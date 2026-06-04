// SPDX-License-Identifier: MIT

package governor_test

import (
	"context"
	"encoding/json"
	"testing"

	"github.com/agezt/agezt/kernel/agent"
	"github.com/agezt/agezt/kernel/event"
	"github.com/agezt/agezt/kernel/governor"
)

// jsonNativeLookup builds a ModelJSONNative func from a fixed map of
// model→native; any model absent from the map reports known=false.
func jsonNativeLookup(m map[string]bool) func(string) (bool, bool) {
	return func(model string) (bool, bool) {
		n, ok := m[model]
		return n, ok
	}
}

func countKind(j interface {
	Range(func(*event.Event) error) error
}, kind event.Kind) int {
	n := 0
	_ = j.Range(func(e *event.Event) error {
		if e.Kind == kind {
			n++
		}
		return nil
	})
	return n
}

// newGovWithJSONNative wires a governor with a single provider and a
// ModelJSONNative lookup, returning the governor + journal for assertions.
func newGovWithJSONNative(t *testing.T, native map[string]bool) (*governor.Governor, interface {
	Range(func(*event.Event) error) error
}, *fakeProvider) {
	t.Helper()
	b, j := newBus(t)
	r := governor.NewRegistry()
	prov := &fakeProvider{name: "p", resp: okResp("mini", 1, 1)}
	mustRegister(t, r, &governor.ProviderInfo{Name: "p", Provider: prov, AuthMode: governor.AuthAPIKey})
	g, err := governor.New(governor.Config{
		Registry:        r,
		Bus:             b,
		ModelJSONNative: jsonNativeLookup(native),
	})
	if err != nil {
		t.Fatal(err)
	}
	return g, j, prov
}

// TestCapabilityDegraded_JSONModeOnNonNativeModel: a JSON-mode request to a model
// the catalog knows is non-native journals capability.degraded — and the request
// still PROCEEDS (degradation, not rejection): the provider is called.
func TestCapabilityDegraded_JSONModeOnNonNativeModel(t *testing.T) {
	g, j, prov := newGovWithJSONNative(t, map[string]bool{"mini": false})
	if _, err := g.Complete(context.Background(), agent.CompletionRequest{
		Model:    "mini",
		JSONMode: true,
	}); err != nil {
		t.Fatalf("Complete: %v", err)
	}
	if got := countKind(j, event.KindCapabilityDegraded); got != 1 {
		t.Errorf("capability.degraded count = %d, want 1", got)
	}
	if prov.calls.Load() != 1 {
		t.Errorf("provider calls = %d, want 1 (degradation must not block the request)", prov.calls.Load())
	}
	// The payload must name the json_mode capability and the model.
	var deg *event.Event
	_ = j.Range(func(e *event.Event) error {
		if e.Kind == event.KindCapabilityDegraded {
			deg = e
		}
		return nil
	})
	if deg == nil {
		t.Fatal("no capability.degraded event captured")
	}
	var p struct {
		Model      string `json:"model"`
		Capability string `json:"capability"`
		Reason     string `json:"reason"`
	}
	if err := json.Unmarshal(deg.Payload, &p); err != nil {
		t.Fatalf("payload: %v", err)
	}
	if p.Model != "mini" || p.Capability != "json_mode" || p.Reason == "" {
		t.Errorf("payload = %+v", p)
	}
}

// TestCapabilityDegraded_CarriesRunCorrelation: the degradation event must carry
// the request's correlation id, so it lands in the run timeline and `agt why`
// reaches it (rather than being orphaned).
func TestCapabilityDegraded_CarriesRunCorrelation(t *testing.T) {
	g, j, _ := newGovWithJSONNative(t, map[string]bool{"mini": false})
	if _, err := g.Complete(context.Background(), agent.CompletionRequest{
		Model:         "mini",
		JSONMode:      true,
		CorrelationID: "run-CORR-9",
	}); err != nil {
		t.Fatalf("Complete: %v", err)
	}
	var deg *event.Event
	_ = j.Range(func(e *event.Event) error {
		if e.Kind == event.KindCapabilityDegraded {
			deg = e
		}
		return nil
	})
	if deg == nil {
		t.Fatal("no capability.degraded event captured")
	}
	if deg.CorrelationID != "run-CORR-9" {
		t.Errorf("CorrelationID = %q, want run-CORR-9 (degradation must link to its run)", deg.CorrelationID)
	}
}

// TestCapabilityDegraded_NativeModelNotFlagged: JSON mode on a native-JSON model
// is the happy path — no degradation event.
func TestCapabilityDegraded_NativeModelNotFlagged(t *testing.T) {
	g, j, _ := newGovWithJSONNative(t, map[string]bool{"mini": true})
	if _, err := g.Complete(context.Background(), agent.CompletionRequest{Model: "mini", JSONMode: true}); err != nil {
		t.Fatalf("Complete: %v", err)
	}
	if got := countKind(j, event.KindCapabilityDegraded); got != 0 {
		t.Errorf("capability.degraded count = %d, want 0 (native model)", got)
	}
}

// TestCapabilityDegraded_UnknownModelNotFlagged: an unknown model is never flagged
// — we don't journal a degradation we can't confirm (fail-safe).
func TestCapabilityDegraded_UnknownModelNotFlagged(t *testing.T) {
	g, j, _ := newGovWithJSONNative(t, map[string]bool{"mini": false})
	if _, err := g.Complete(context.Background(), agent.CompletionRequest{Model: "stranger", JSONMode: true}); err != nil {
		t.Fatalf("Complete: %v", err)
	}
	if got := countKind(j, event.KindCapabilityDegraded); got != 0 {
		t.Errorf("capability.degraded count = %d, want 0 (unknown model)", got)
	}
}

// TestCapabilityDegraded_NoJSONModeNoEvent: without JSON mode there is nothing to
// degrade, even on a non-native model.
func TestCapabilityDegraded_NoJSONModeNoEvent(t *testing.T) {
	g, j, _ := newGovWithJSONNative(t, map[string]bool{"mini": false})
	if _, err := g.Complete(context.Background(), agent.CompletionRequest{Model: "mini", JSONMode: false}); err != nil {
		t.Fatalf("Complete: %v", err)
	}
	if got := countKind(j, event.KindCapabilityDegraded); got != 0 {
		t.Errorf("capability.degraded count = %d, want 0 (no JSON mode)", got)
	}
}
