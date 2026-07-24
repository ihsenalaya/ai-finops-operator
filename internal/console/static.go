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
	"net/http"
	"os"
	"path/filepath"
)

const notBuiltPlaceholder = `<!doctype html>
<html><head><meta charset="utf-8"><title>console not built yet</title></head>
<body style="font-family: sans-serif; max-width: 40rem; margin: 4rem auto;">
<h1>Console not built yet</h1>
<p>Run <code>cd ui/console &amp;&amp; npm install &amp;&amp; npm run build</code>, then restart
console-api.</p>
</body></html>`

// StaticDir serves the built frontend (ui/console/dist) directly from disk —
// this tool runs locally next to the repo using the caller's own kubeconfig,
// so there is no need to embed the UI into the binary for portability. If the
// frontend hasn't been built yet, a short placeholder page is served instead
// of a 404, so `go run ./cmd/console-api` before `npm run build` is still
// self-explanatory.
func StaticDir(dir string) StaticFS {
	return &diskUI{dir: dir, fallback: http.FileServer(http.Dir(dir))}
}

type diskUI struct {
	dir      string
	fallback http.Handler
}

func (d *diskUI) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	if _, err := os.Stat(filepath.Join(d.dir, "index.html")); err != nil {
		w.Header().Set("Content-Type", "text/html; charset=utf-8")
		_, _ = w.Write([]byte(notBuiltPlaceholder))
		return
	}
	d.fallback.ServeHTTP(w, r)
}
