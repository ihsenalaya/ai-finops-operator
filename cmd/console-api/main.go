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
// on a single port.
//
// It is meant to run two ways:
//   - Deployed by the Helm chart, inside the cluster, next to the operator
//     (console.enabled=true, the default) — it then uses its own dedicated
//     ServiceAccount and in-cluster RBAC (see charts/.../templates/console-*),
//     scoped to only this operator's CRDs. This is how the console travels
//     with the operator into any environment it's installed in.
//   - Run locally against a kubeconfig (`go run ./cmd/console-api`, or
//     --kubeconfig/--context flags) for local development against a cluster
//     you already have `kubectl` access to.
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
	kubeconfigPath := flag.String("kubeconfig", "", "path to kubeconfig; forces local-kubeconfig mode even inside a cluster")
	kubeContext := flag.String("context", "", "kubeconfig context to use (defaults to current-context); only used in local-kubeconfig mode")
	devCORS := flag.Bool("dev-cors", false, "enable permissive CORS (only needed when running the Vite dev server separately)")
	uiDir := flag.String("ui-dir", "ui/console/dist", "path to the built frontend (npm run build in ui/console), relative to the current directory")
	flag.Parse()

	cfg, source, err := loadConfig(*kubeconfigPath, *kubeContext)
	if err != nil {
		log.Fatalf("console-api: loading Kubernetes config: %v", err)
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

	log.Printf("console-api: connected via %s, listening on %s", source, *addr)
	if err := http.ListenAndServe(*addr, handler); err != nil {
		log.Fatal(err)
	}
}

// loadConfig picks the right Kubernetes config for however this binary is
// currently running:
//  1. --kubeconfig explicitly given: always honor it (explicit override wins,
//     even inside a cluster — useful for pointing a locally-run console at a
//     remote cluster).
//  2. Otherwise, try in-cluster config first (this succeeds only when running
//     inside a pod with a mounted ServiceAccount token — i.e. deployed by the
//     Helm chart). This is the "attached to the operator" path.
//  3. Otherwise, fall back to the caller's own local kubeconfig
//     ($KUBECONFIG / ~/.kube/config / current-context, optionally overridden
//     by --context) — the local-development path.
func loadConfig(explicitKubeconfig, contextName string) (*rest.Config, string, error) {
	if explicitKubeconfig != "" {
		cfg, ctxName, err := loadKubeconfig(explicitKubeconfig, contextName)
		return cfg, "kubeconfig " + explicitKubeconfig + " (context " + ctxName + ")", err
	}

	if cfg, err := rest.InClusterConfig(); err == nil {
		return cfg, "in-cluster ServiceAccount", nil
	}

	cfg, ctxName, err := loadKubeconfig("", contextName)
	return cfg, "kubeconfig context " + ctxName, err
}

// loadKubeconfig resolves a kubeconfig using the same rules as kubectl:
// --kubeconfig flag > $KUBECONFIG > ~/.kube/config, and the requested (or
// current) context within it.
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
