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
	"sync"

	"k8s.io/apimachinery/pkg/types"

	aiopsv1alpha1 "github.com/ihsenalaya/ai-finops-operator/api/v1alpha1"
	"github.com/ihsenalaya/ai-finops-operator/internal/metrics"
)

// qualityGateSeriesSet accumulates the exact label-tuples one AIQualityGate emits
// into each gauge vector during a single reconcile. The order of values in each
// tuple matches the vector's label names.
type qualityGateSeriesSet struct {
	passed       [][]string // {namespace, quality_gate, target_namespace, application, source_model, candidate_model}
	failedChecks [][]string // {namespace, quality_gate, target_namespace, application}
	score        [][]string // {namespace, quality_gate, target_namespace, application}
	dimensions   [][]string // {namespace, app, provider, model, dimension}
}

func (s *qualityGateSeriesSet) addGate(gate *aiopsv1alpha1.AIQualityGate) {
	s.passed = append(s.passed, []string{
		gate.Namespace, gate.Name, gate.Spec.Target.Namespace, gate.Spec.Target.Application,
		gate.Spec.SourceModel, gate.Spec.CandidateModel,
	})
	scoped := []string{gate.Namespace, gate.Name, gate.Spec.Target.Namespace, gate.Spec.Target.Application}
	s.failedChecks = append(s.failedChecks, scoped)
	s.score = append(s.score, scoped)
}

func (s *qualityGateSeriesSet) addDimension(targetNamespace, application, provider, model, dimension string) {
	s.dimensions = append(s.dimensions, []string{targetNamespace, application, provider, model, dimension})
}

// qualityGateMetricTracker remembers, per gate UID, the series each AIQualityGate
// last wrote into the four gauge vectors this controller is the sole writer of.
// It prunes exactly that gate's series instead of Reset()-ing the whole vector,
// which would wipe every other gate's series on each reconcile.
//
// Without it a gate's series outlived the object in two ways: editing the spec
// (a new candidate model, a retargeted application) left the previous tuple
// behind, and deleting the gate left every tuple behind until the operator
// restarted — so a dashboard kept reporting gates that no longer exist.
//
// ai_finops_quality_score is included even though its labels do not carry the
// gate name: this controller is its only writer, and two gates sharing a target
// application, provider and candidate model would be duplicate configuration.
// Should that happen, the surviving gate re-Set()s the tuple on its next
// reconcile.
type qualityGateMetricTracker struct {
	mu    sync.Mutex
	byUID map[types.UID]*qualityGateSeriesSet
}

var qualityGateMetrics = &qualityGateMetricTracker{byUID: map[types.UID]*qualityGateSeriesSet{}}

// retire prunes the tuples this gate emitted on a previous reconcile but not in
// `now`, then records `now` as the gate's current series. Callers must already
// have Set() every series in `now`, so pruning removes only genuinely-disappeared
// series and persistent ones never flap.
func (t *qualityGateMetricTracker) retire(uid types.UID, now *qualityGateSeriesSet) {
	t.mu.Lock()
	defer t.mu.Unlock()
	if prev := t.byUID[uid]; prev != nil {
		deleteStale(metrics.QualityGatePassed, prev.passed, now.passed)
		deleteStale(metrics.QualityGateFailedChecks, prev.failedChecks, now.failedChecks)
		deleteStale(metrics.QualityGateScore, prev.score, now.score)
		deleteStale(metrics.QualityScore, prev.dimensions, now.dimensions)
	}
	t.byUID[uid] = now
}

// forget deletes all of a gate's series and drops it from the tracker. Called by
// the finalizer when an AIQualityGate is deleted so no dead series persist.
func (t *qualityGateMetricTracker) forget(uid types.UID) {
	t.mu.Lock()
	defer t.mu.Unlock()
	prev := t.byUID[uid]
	if prev == nil {
		return
	}
	deleteStale(metrics.QualityGatePassed, prev.passed, nil)
	deleteStale(metrics.QualityGateFailedChecks, prev.failedChecks, nil)
	deleteStale(metrics.QualityGateScore, prev.score, nil)
	deleteStale(metrics.QualityScore, prev.dimensions, nil)
	delete(t.byUID, uid)
}
