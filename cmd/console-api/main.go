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

// Command console-api serves the AI FinOps Operator console: a small REST API
// over this operator's CRDs, plus the built graphical console (ui/console),
// on a single port. It never uses an in-cluster ServiceAccount — it always
// connects to Kubernetes with whatever kubeconfig/context is active for the
// person running it (KUBECONFIG env var, ~/.kube/config, current-context, or
// --kubeconfig/--context flags), so every action in the UI runs under that
// person's own RBAC permissions.
package main

import (
	"flag"
	"log"
	"net/http"

	"k8s.io/client-go/dynamic"
	"k8s.io/client-go/rest"
	"k8s.io/client-go/tools/clientcmd"

	"github.com/ihsenalaya/ai-finops-operator/internal/console"
)

func main() {
	addr := flag.String("addr", ":8090", "address to listen on")
	kubeconfigPath := flag.String("kubeconfig", "", "path to kubeconfig (defaults to $KUBECONFIG or ~/.kube/config)")
	kubeContext := flag.String("context", "", "kubeconfig context to use (defaults to current-context)")
	devCORS := flag.Bool("dev-cors", false, "enable permissive CORS (only needed when running the Vite dev server separately)")
	uiDir := flag.String("ui-dir", "ui/console/dist", "path to the built frontend (npm run build in ui/console), relative to the current directory")
	flag.Parse()

	cfg, contextName, err := loadKubeconfig(*kubeconfigPath, *kubeContext)
	if err != nil {
		log.Fatalf("console-api: loading kubeconfig: %v", err)
	}

	dyn, err := dynamic.NewForConfig(cfg)
	if err != nil {
		log.Fatalf("console-api: building Kubernetes client: %v", err)
	}

	srv := console.NewServer(dyn, console.StaticDir(*uiDir))
	var handler http.Handler = srv
	if *devCORS {
		handler = console.CORS(handler)
	}

	log.Printf("console-api: using kubeconfig context %q, listening on %s", contextName, *addr)
	if err := http.ListenAndServe(*addr, handler); err != nil {
		log.Fatal(err)
	}
}

// loadKubeconfig resolves the caller's own kubeconfig using the same rules as
// kubectl: --kubeconfig flag > $KUBECONFIG > ~/.kube/config, and the requested
// (or current) context within it. It deliberately never falls back to
// in-cluster config: this tool always acts as the human operating it, never
// as a separate service identity.
func loadKubeconfig(explicitPath, contextName string) (*rest.Config, string, error) {
	rules := clientcmd.NewDefaultClientConfigLoadingRules()
	if explicitPath != "" {
		rules.ExplicitPath = explicitPath
	}
	overrides := &clientcmd.ConfigOverrides{}
	if contextName != "" {
		overrides.CurrentContext = contextName
	}
	loader := clientcmd.NewNonInteractiveDeferredLoadingClientConfig(rules, overrides)
	cfg, err := loader.ClientConfig()
	if err != nil {
		return nil, "", err
	}
	raw, err := loader.RawConfig()
	if err != nil {
		return cfg, "", nil //nolint:nilerr // context name is cosmetic only
	}
	resolvedContext := raw.CurrentContext
	if contextName != "" {
		resolvedContext = contextName
	}
	return cfg, resolvedContext, nil
}
