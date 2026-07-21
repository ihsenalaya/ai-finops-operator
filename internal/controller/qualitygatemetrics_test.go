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

package controller

import (
	"testing"

	"github.com/prometheus/client_golang/prometheus/testutil"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/types"

	aiopsv1alpha1 "github.com/ihsenalaya/ai-finops-operator/api/v1alpha1"
	"github.com/ihsenalaya/ai-finops-operator/internal/metrics"
)

func qualityGateFixture(name, application, candidate string, uid types.UID) *aiopsv1alpha1.AIQualityGate {
	return &aiopsv1alpha1.AIQualityGate{
		ObjectMeta: metav1.ObjectMeta{Name: name, Namespace: "demo", UID: uid},
		Spec: aiopsv1alpha1.AIQualityGateSpec{
			Target:         aiopsv1alpha1.AIQualityGateTarget{Namespace: "demo", Application: application},
			SourceModel:    "source-model",
			CandidateModel: candidate,
		},
	}
}

// emitQualityGateSeries mirrors what the reconciler writes, so the test exercises
// the same tuples the controller registers with the tracker.
func emitQualityGateSeries(gate *aiopsv1alpha1.AIQualityGate, provider string) *qualityGateSeriesSet {
	series := &qualityGateSeriesSet{}
	series.addGate(gate)
	metrics.QualityGatePassed.WithLabelValues(gate.Namespace, gate.Name, gate.Spec.Target.Namespace, gate.Spec.Target.Application, gate.Spec.SourceModel, gate.Spec.CandidateModel).Set(1)
	metrics.QualityGateFailedChecks.WithLabelValues(gate.Namespace, gate.Name, gate.Spec.Target.Namespace, gate.Spec.Target.Application).Set(0)
	metrics.QualityGateScore.WithLabelValues(gate.Namespace, gate.Name, gate.Spec.Target.Namespace, gate.Spec.Target.Application).Set(90)
	gate.Status.Dimensions = []aiopsv1alpha1.AIQualityDimensionStatus{{Name: "overall", Score: 90}}
	emitQualityScoreMetrics(gate, provider, gate.Spec.CandidateModel, series)
	return series
}

func resetQualityGateMetrics() {
	metrics.QualityGatePassed.Reset()
	metrics.QualityGateFailedChecks.Reset()
	metrics.QualityGateScore.Reset()
	metrics.QualityScore.Reset()
	qualityGateMetrics.byUID = map[types.UID]*qualityGateSeriesSet{}
}

// A deleted AIQualityGate must not keep reporting: before the tracker its series
// survived the object and the dashboard kept showing a gate that no longer existed.
func TestQualityGateMetricsForgetDropsDeletedGateSeries(t *testing.T) {
	resetQualityGateMetrics()
	defer resetQualityGateMetrics()

	gate := qualityGateFixture("gate-a", "checkout", "candidate-model", types.UID("uid-a"))
	qualityGateMetrics.retire(gate.UID, emitQualityGateSeries(gate, "provider-a"))

	if got := testutil.CollectAndCount(metrics.QualityGatePassed); got != 1 {
		t.Fatalf("quality_gate_passed series before delete = %d, want 1", got)
	}
	if got := testutil.CollectAndCount(metrics.QualityScore); got != 1 {
		t.Fatalf("quality_score series before delete = %d, want 1", got)
	}

	qualityGateMetrics.forget(gate.UID)

	for name, count := range map[string]int{
		"quality_gate_passed":        testutil.CollectAndCount(metrics.QualityGatePassed),
		"quality_gate_failed_checks": testutil.CollectAndCount(metrics.QualityGateFailedChecks),
		"quality_gate_score":         testutil.CollectAndCount(metrics.QualityGateScore),
		"quality_score":              testutil.CollectAndCount(metrics.QualityScore),
	} {
		if count != 0 {
			t.Errorf("%s still has %d series after delete, want 0", name, count)
		}
	}
}

// Editing the spec must not leave the previous tuple behind: the candidate model
// is a metric label, so a retarget would otherwise double-report the gate.
func TestQualityGateMetricsRetirePrunesRenamedSeries(t *testing.T) {
	resetQualityGateMetrics()
	defer resetQualityGateMetrics()

	gate := qualityGateFixture("gate-a", "checkout", "candidate-v1", types.UID("uid-a"))
	qualityGateMetrics.retire(gate.UID, emitQualityGateSeries(gate, "provider-a"))

	gate.Spec.CandidateModel = "candidate-v2"
	qualityGateMetrics.retire(gate.UID, emitQualityGateSeries(gate, "provider-a"))

	if got := testutil.CollectAndCount(metrics.QualityGatePassed); got != 1 {
		t.Errorf("quality_gate_passed series after retarget = %d, want 1 (stale tuple not pruned)", got)
	}
	if got := testutil.CollectAndCount(metrics.QualityScore); got != 1 {
		t.Errorf("quality_score series after retarget = %d, want 1 (stale tuple not pruned)", got)
	}
}

// One gate's pruning must never remove another gate's series.
func TestQualityGateMetricsForgetLeavesOtherGatesAlone(t *testing.T) {
	resetQualityGateMetrics()
	defer resetQualityGateMetrics()

	first := qualityGateFixture("gate-a", "checkout", "candidate-a", types.UID("uid-a"))
	second := qualityGateFixture("gate-b", "billing", "candidate-b", types.UID("uid-b"))
	qualityGateMetrics.retire(first.UID, emitQualityGateSeries(first, "provider-a"))
	qualityGateMetrics.retire(second.UID, emitQualityGateSeries(second, "provider-b"))

	qualityGateMetrics.forget(first.UID)

	if got := testutil.CollectAndCount(metrics.QualityGatePassed); got != 1 {
		t.Errorf("quality_gate_passed series after deleting one of two gates = %d, want 1", got)
	}
	if got := testutil.CollectAndCount(metrics.QualityScore); got != 1 {
		t.Errorf("quality_score series after deleting one of two gates = %d, want 1", got)
	}
}
