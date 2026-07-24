/*
Copyright 2026.

Licensed under the Apache License, Version 2.0 (the "License");
you may not use this file except in compliance with the License.
You may obtain a copy of the License at

    http://www.apache.org/licenses/LICENSE-2.0

Unless required by applicable law or agreed to in writing, software
distributed under the License is distributed on an "AS IS" BASIS,
WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
See the License for the specific language governing permissions and
limitations under the License.
*/

package console

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"runtime"
	"testing"

	"k8s.io/client-go/dynamic"
	"sigs.k8s.io/controller-runtime/pkg/envtest"
)

// This test exercises the console REST API end-to-end against a REAL
// Kubernetes API server (envtest: real kube-apiserver + etcd binaries, no
// mocks), the same way internal/controller's suite does. It proves the
// console can create, list, read, update and delete a CRD through plain HTTP
// calls — exactly what the graphical UI will do from the browser.
func TestConsoleAPIEndToEnd(t *testing.T) {
	_, thisFile, _, _ := runtime.Caller(0)
	repoRoot := filepath.Clean(filepath.Join(filepath.Dir(thisFile), "..", ".."))
	binaryAssetsDir := filepath.Join(repoRoot, "bin", "k8s", fmt.Sprintf("1.31.0-%s-%s", runtime.GOOS, runtime.GOARCH))
	if _, err := os.Stat(binaryAssetsDir); err != nil {
		t.Skipf("envtest binaries not found at %s (run `make envtest` first): %v", binaryAssetsDir, err)
	}

	testEnv := &envtest.Environment{
		CRDDirectoryPaths:     []string{filepath.Join(repoRoot, "config", "crd", "bases")},
		ErrorIfCRDPathMissing: true,
		BinaryAssetsDirectory: binaryAssetsDir,
	}
	cfg, err := testEnv.Start()
	if err != nil {
		t.Fatalf("starting envtest: %v", err)
	}
	t.Cleanup(func() {
		if err := testEnv.Stop(); err != nil {
			t.Errorf("stopping envtest: %v", err)
		}
	})

	dyn, err := dynamic.NewForConfig(cfg)
	if err != nil {
		t.Fatalf("building dynamic client: %v", err)
	}

	srv := NewServer(dyn, nil)
	ts := httptest.NewServer(srv)
	defer ts.Close()

	t.Run("lists the 11 registered resource types", func(t *testing.T) {
		var got []map[string]any
		mustGetJSON(t, ts.URL+"/api/resources", &got)
		if len(got) != 11 {
			t.Fatalf("expected 11 resource types, got %d: %+v", len(got), got)
		}
	})

	t.Run("lists namespaces including default", func(t *testing.T) {
		var got []string
		mustGetJSON(t, ts.URL+"/api/namespaces", &got)
		if !contains(got, "default") {
			t.Fatalf("expected \"default\" in namespaces, got %v", got)
		}
	})

	t.Run("fetches the AIProvider schema from the live CRD", func(t *testing.T) {
		var got map[string]any
		mustGetJSON(t, ts.URL+"/api/resources/AIProvider/schema", &got)
		props, ok := got["properties"].(map[string]any)
		if !ok {
			t.Fatalf("expected top-level properties in schema, got %+v", got)
		}
		if _, ok := props["spec"]; !ok {
			t.Fatalf("expected schema.properties.spec, got keys %v", keysOf(props))
		}
	})

	const ns = "default"
	const name = "console-test-provider"

	t.Run("creates an AIProvider via POST", func(t *testing.T) {
		body := map[string]any{
			"metadata": map[string]any{"name": name, "namespace": ns},
			"spec": map[string]any{
				"type":          "mistral",
				"dataResidency": "france",
				"managed":       true,
				"pricing": map[string]any{
					"inputTokenPricePerMillion":  "2.00",
					"outputTokenPricePerMillion": "6.00",
				},
			},
		}
		resp := mustPostJSON(t, ts.URL+"/api/resources/AIProvider", body)
		if resp.StatusCode != http.StatusCreated {
			t.Fatalf("expected 201, got %d: %s", resp.StatusCode, readBody(t, resp))
		}
	})

	t.Run("lists it back under the namespace", func(t *testing.T) {
		var list map[string]any
		mustGetJSON(t, ts.URL+"/api/resources/AIProvider?namespace="+ns, &list)
		items, _ := list["items"].([]any)
		if len(items) != 1 {
			t.Fatalf("expected 1 item, got %d: %+v", len(items), list)
		}
	})

	t.Run("gets it directly by namespace/name", func(t *testing.T) {
		var obj map[string]any
		mustGetJSON(t, ts.URL+"/api/resources/AIProvider/"+ns+"/"+name, &obj)
		spec, _ := obj["spec"].(map[string]any)
		if spec["type"] != "mistral" {
			t.Fatalf("expected spec.type=mistral, got %+v", spec)
		}
	})

	t.Run("updates it via PUT", func(t *testing.T) {
		var current map[string]any
		mustGetJSON(t, ts.URL+"/api/resources/AIProvider/"+ns+"/"+name, &current)
		spec := current["spec"].(map[string]any)
		spec["dataResidency"] = "eu"

		req, _ := http.NewRequest(http.MethodPut, ts.URL+"/api/resources/AIProvider/"+ns+"/"+name, jsonBody(current))
		resp, err := http.DefaultClient.Do(req)
		if err != nil {
			t.Fatalf("PUT request: %v", err)
		}
		if resp.StatusCode != http.StatusOK {
			t.Fatalf("expected 200, got %d: %s", resp.StatusCode, readBody(t, resp))
		}

		var updated map[string]any
		mustGetJSON(t, ts.URL+"/api/resources/AIProvider/"+ns+"/"+name, &updated)
		if updated["spec"].(map[string]any)["dataResidency"] != "eu" {
			t.Fatalf("expected dataResidency=eu after update, got %+v", updated["spec"])
		}
	})

	t.Run("deletes it, then a get 404s", func(t *testing.T) {
		req, _ := http.NewRequest(http.MethodDelete, ts.URL+"/api/resources/AIProvider/"+ns+"/"+name, nil)
		resp, err := http.DefaultClient.Do(req)
		if err != nil {
			t.Fatalf("DELETE request: %v", err)
		}
		if resp.StatusCode != http.StatusNoContent {
			t.Fatalf("expected 204, got %d", resp.StatusCode)
		}

		getResp, err := http.Get(ts.URL + "/api/resources/AIProvider/" + ns + "/" + name)
		if err != nil {
			t.Fatalf("GET request: %v", err)
		}
		if getResp.StatusCode != http.StatusNotFound {
			t.Fatalf("expected 404 after delete, got %d", getResp.StatusCode)
		}
	})
}

func mustGetJSON(t *testing.T, url string, out any) {
	t.Helper()
	resp, err := http.Get(url)
	if err != nil {
		t.Fatalf("GET %s: %v", url, err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("GET %s: expected 200, got %d: %s", url, resp.StatusCode, readBody(t, resp))
	}
	if err := json.NewDecoder(resp.Body).Decode(out); err != nil {
		t.Fatalf("GET %s: decode response: %v", url, err)
	}
}

func mustPostJSON(t *testing.T, url string, body any) *http.Response {
	t.Helper()
	resp, err := http.Post(url, "application/json", jsonBody(body))
	if err != nil {
		t.Fatalf("POST %s: %v", url, err)
	}
	return resp
}

func jsonBody(v any) *bytes.Reader {
	b, err := json.Marshal(v)
	if err != nil {
		panic(err)
	}
	return bytes.NewReader(b)
}

func readBody(t *testing.T, resp *http.Response) string {
	t.Helper()
	b, _ := io.ReadAll(resp.Body)
	return string(b)
}

func contains(list []string, want string) bool {
	for _, v := range list {
		if v == want {
			return true
		}
	}
	return false
}

func keysOf(m map[string]any) []string {
	out := make([]string, 0, len(m))
	for k := range m {
		out = append(out, k)
	}
	return out
}
