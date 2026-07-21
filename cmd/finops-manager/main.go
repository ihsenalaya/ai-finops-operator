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

// Command finops-manager is the standalone controller manager of the
// ai-finops-operator: it reconciles only the FinOps & sovereignty CRDs
// (catalog, budgets, sovereignty, break-even, reports, quality gates,
// routing, overrides and change requests). Confidential attestation and
// GOV-AR admission are owned by their dedicated operators.
package main

import (
	"context"
	"crypto/tls"
	"flag"
	"fmt"
	"log"
	"os"
	"time"

	// Import all Kubernetes client auth plugins (e.g. Azure, GCP, OIDC, etc.)
	// to ensure that exec-entrypoint and run can make use of them.
	_ "k8s.io/client-go/plugin/pkg/client/auth"

	"k8s.io/apimachinery/pkg/runtime"
	utilruntime "k8s.io/apimachinery/pkg/util/runtime"
	clientgoscheme "k8s.io/client-go/kubernetes/scheme"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/healthz"
	"sigs.k8s.io/controller-runtime/pkg/log/zap"
	metricsserver "sigs.k8s.io/controller-runtime/pkg/metrics/server"

	aiopsv1alpha1 "github.com/ihsenalaya/ai-finops-operator/api/v1alpha1"
	"github.com/ihsenalaya/ai-finops-operator/internal/controller"
	"github.com/ihsenalaya/ai-finops-operator/internal/qualityeval"
)

var (
	scheme   = runtime.NewScheme()
	setupLog = ctrl.Log.WithName("setup")
)

func init() {
	utilruntime.Must(clientgoscheme.AddToScheme(scheme))
	utilruntime.Must(aiopsv1alpha1.AddToScheme(scheme))
}

func main() {
	if len(os.Args) > 1 && os.Args[1] == "quality-eval" {
		if err := runQualityEval(os.Args[2:]); err != nil {
			log.Printf("quality-eval failed: %v", err)
			os.Exit(1)
		}
		return
	}

	var metricsAddr string
	var enableLeaderElection bool
	var probeAddr string
	var secureMetrics bool
	var enableHTTP2 bool
	flag.StringVar(&metricsAddr, "metrics-bind-address", ":8080", "The address the metric endpoint binds to.")
	flag.StringVar(&probeAddr, "health-probe-bind-address", ":8081", "The address the probe endpoint binds to.")
	flag.BoolVar(&enableLeaderElection, "leader-elect", false,
		"Enable leader election for controller manager. "+
			"Enabling this will ensure there is only one active controller manager.")
	flag.BoolVar(&secureMetrics, "metrics-secure", false,
		"If set the metrics endpoint is served securely")
	flag.BoolVar(&enableHTTP2, "enable-http2", false,
		"If set, HTTP/2 will be enabled for the metrics and webhook servers")
	opts := zap.Options{
		Development: true,
	}
	opts.BindFlags(flag.CommandLine)
	flag.Parse()

	ctrl.SetLogger(zap.New(zap.UseFlagOptions(&opts)))

	// Disable http/2 by default (Rapid Reset / Stream Cancellation CVEs).
	disableHTTP2 := func(c *tls.Config) {
		setupLog.Info("disabling http/2")
		c.NextProtos = []string{"http/1.1"}
	}

	tlsOpts := []func(*tls.Config){}
	if !enableHTTP2 {
		tlsOpts = append(tlsOpts, disableHTTP2)
	}

	mgr, err := ctrl.NewManager(ctrl.GetConfigOrDie(), ctrl.Options{
		Scheme: scheme,
		Metrics: metricsserver.Options{
			BindAddress:   metricsAddr,
			SecureServing: secureMetrics,
			TLSOpts:       tlsOpts,
		},
		HealthProbeBindAddress: probeAddr,
		LeaderElection:         enableLeaderElection,
		LeaderElectionID:       "finops.2d92db7d.imperium.io",
	})
	if err != nil {
		setupLog.Error(err, "unable to start manager")
		os.Exit(1)
	}

	if err = (&controller.AIGatewayReconciler{
		Client:   mgr.GetClient(),
		Scheme:   mgr.GetScheme(),
		Recorder: mgr.GetEventRecorderFor("aigateway-controller"),
	}).SetupWithManager(mgr); err != nil {
		setupLog.Error(err, "unable to create controller", "controller", "AIGateway")
		os.Exit(1)
	}
	if err = (&controller.AIProviderReconciler{
		Client:   mgr.GetClient(),
		Scheme:   mgr.GetScheme(),
		Recorder: mgr.GetEventRecorderFor("aiprovider-controller"),
	}).SetupWithManager(mgr); err != nil {
		setupLog.Error(err, "unable to create controller", "controller", "AIProvider")
		os.Exit(1)
	}
	if err = (&controller.AIModelReconciler{
		Client:   mgr.GetClient(),
		Scheme:   mgr.GetScheme(),
		Recorder: mgr.GetEventRecorderFor("aimodel-controller"),
	}).SetupWithManager(mgr); err != nil {
		setupLog.Error(err, "unable to create controller", "controller", "AIModel")
		os.Exit(1)
	}
	if err = (&controller.AIBudgetPolicyReconciler{
		Client:   mgr.GetClient(),
		Scheme:   mgr.GetScheme(),
		Recorder: mgr.GetEventRecorderFor("aibudgetpolicy-controller"),
	}).SetupWithManager(mgr); err != nil {
		setupLog.Error(err, "unable to create controller", "controller", "AIBudgetPolicy")
		os.Exit(1)
	}
	if err = (&controller.AISovereigntyPolicyReconciler{
		Client:   mgr.GetClient(),
		Scheme:   mgr.GetScheme(),
		Recorder: mgr.GetEventRecorderFor("aisovereigntypolicy-controller"),
	}).SetupWithManager(mgr); err != nil {
		setupLog.Error(err, "unable to create controller", "controller", "AISovereigntyPolicy")
		os.Exit(1)
	}
	if err = (&controller.AIBreakEvenAnalysisReconciler{
		Client:   mgr.GetClient(),
		Scheme:   mgr.GetScheme(),
		Recorder: mgr.GetEventRecorderFor("aibreakevenanalysis-controller"),
	}).SetupWithManager(mgr); err != nil {
		setupLog.Error(err, "unable to create controller", "controller", "AIBreakEvenAnalysis")
		os.Exit(1)
	}
	if err = (&controller.AIFinOpsReportReconciler{
		Client:   mgr.GetClient(),
		Scheme:   mgr.GetScheme(),
		Recorder: mgr.GetEventRecorderFor("aifinopsreport-controller"),
	}).SetupWithManager(mgr); err != nil {
		setupLog.Error(err, "unable to create controller", "controller", "AIFinOpsReport")
		os.Exit(1)
	}
	if err = (&controller.AIQualityGateReconciler{
		Client:   mgr.GetClient(),
		Scheme:   mgr.GetScheme(),
		Recorder: mgr.GetEventRecorderFor("aiqualitygate-controller"),
	}).SetupWithManager(mgr); err != nil {
		setupLog.Error(err, "unable to create controller", "controller", "AIQualityGate")
		os.Exit(1)
	}
	if err = (&controller.AIRouteOverrideReconciler{
		Client:   mgr.GetClient(),
		Scheme:   mgr.GetScheme(),
		Recorder: mgr.GetEventRecorderFor("airouteoverride-controller"),
	}).SetupWithManager(mgr); err != nil {
		setupLog.Error(err, "unable to create controller", "controller", "AIRouteOverride")
		os.Exit(1)
	}
	if err = (&controller.AIRoutingPolicyReconciler{
		Client:   mgr.GetClient(),
		Scheme:   mgr.GetScheme(),
		Recorder: mgr.GetEventRecorderFor("airoutingpolicy-controller"),
	}).SetupWithManager(mgr); err != nil {
		setupLog.Error(err, "unable to create controller", "controller", "AIRoutingPolicy")
		os.Exit(1)
	}
	if err = (&controller.AIChangeRequestReconciler{
		Client:   mgr.GetClient(),
		Scheme:   mgr.GetScheme(),
		Recorder: mgr.GetEventRecorderFor("aichangerequest-controller"),
	}).SetupWithManager(mgr); err != nil {
		setupLog.Error(err, "unable to create controller", "controller", "AIChangeRequest")
		os.Exit(1)
	}

	if err := mgr.AddHealthzCheck("healthz", healthz.Ping); err != nil {
		setupLog.Error(err, "unable to set up health check")
		os.Exit(1)
	}
	if err := mgr.AddReadyzCheck("readyz", healthz.Ping); err != nil {
		setupLog.Error(err, "unable to set up ready check")
		os.Exit(1)
	}

	setupLog.Info("starting finops manager")
	if err := mgr.Start(ctrl.SetupSignalHandler()); err != nil {
		setupLog.Error(err, "problem running manager")
		os.Exit(1)
	}
}

func runQualityEval(args []string) error {
	fs := flag.NewFlagSet("quality-eval", flag.ContinueOnError)
	var opts qualityeval.Options
	var timeoutSeconds int
	var terminationLog string
	fs.StringVar(&opts.Endpoint, "endpoint", "", "OpenAI-compatible gateway chat completions endpoint")
	fs.StringVar(&opts.PromptsDir, "prompts-dir", "", "directory containing prompts.yaml/prompts.json")
	fs.StringVar(&opts.PromptsFile, "prompts-file", "", "explicit golden dataset file")
	fs.StringVar(&opts.Namespace, "namespace", "", "application namespace attribution header")
	fs.StringVar(&opts.Application, "application", "", "application attribution header")
	fs.StringVar(&opts.SourceModel, "source-model", "", "source model name")
	fs.StringVar(&opts.CandidateModel, "candidate-model", "", "candidate model name")
	fs.IntVar(&opts.MaxTokens, "max-tokens", 96, "max completion tokens per prompt")
	fs.IntVar(&timeoutSeconds, "timeout-seconds", 60, "HTTP timeout per gateway request")
	fs.StringVar(&terminationLog, "termination-log", "/dev/termination-log", "path for Kubernetes termination message evidence")
	if err := fs.Parse(args); err != nil {
		return err
	}
	opts.Timeout = time.Duration(timeoutSeconds) * time.Second
	raw, err := qualityeval.Run(context.Background(), opts)
	if err != nil {
		if terminationLog != "" {
			_ = writeTerminationError(terminationLog, err)
		}
		return err
	}
	if terminationLog != "" {
		if err := writeTerminationMessage(terminationLog, raw); err != nil {
			return err
		}
	}
	log.Printf("quality-eval completed: wrote %d bytes of evidence", len(raw))
	return nil
}

func writeTerminationMessage(path string, raw []byte) error {
	if err := os.WriteFile(path, raw, 0o600); err != nil {
		return fmt.Errorf("write termination log %s: %w", path, err)
	}
	return nil
}

func writeTerminationError(path string, cause error) error {
	const maxTerminationLogBytes = 4096
	raw := []byte("ERROR: " + cause.Error())
	if len(raw) > maxTerminationLogBytes {
		raw = raw[:maxTerminationLogBytes]
	}
	return writeTerminationMessage(path, raw)
}
