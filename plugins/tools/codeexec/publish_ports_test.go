// SPDX-License-Identifier: MIT

package codeexec

import (
	"context"
	"encoding/json"
	"testing"
	"time"

	"github.com/agezt/agezt/kernel/contract/eventapi"
	"github.com/agezt/agezt/kernel/event"
	"github.com/agezt/agezt/kernel/warden"
)

// fakePublisher records what the tool published through the contract's port.
type fakePublisher struct {
	specs []eventapi.Spec
	err   error
}

func (f *fakePublisher) Publish(spec eventapi.Spec) error {
	f.specs = append(f.specs, spec)
	return f.err
}

// TestTheToolPublishesTheCodeExecutedEventThroughThePort: the wire of the
// tool side.
//
// The tool publishes through the CONTRACT's port (kernel/contract/eventapi),
// and the spec it hands over is the same row the bus takes — the platform's
// event package keeps it as an alias. The event the operator reads in the
// run timeline is built here, so the fields are pinned: a dropped subject, a
// rewritten kind or a lost payload would put a different event in the
// timeline than the one the run actually produced.
func TestTheToolPublishesTheCodeExecutedEventThroughThePort(t *testing.T) {
	pub := &fakePublisher{}
	tool := NewWithWarden(nil, t.TempDir(), nil, false)
	tool.Bind(pub)

	tool.publish(context.Background(), "python", "myproj", 128, true, &warden.Result{
		ExitCode:         0,
		EffectiveProfile: warden.ProfileNone,
		Duration:         1500 * time.Millisecond,
	})

	if len(pub.specs) != 1 {
		t.Fatalf("the tool must publish exactly one event, got %d", len(pub.specs))
	}
	got := pub.specs[0]
	if got.Subject != "code.exec" {
		t.Errorf("the subject drifted: %q", got.Subject)
	}
	if got.Kind != event.KindCodeExecuted {
		t.Errorf("the kind drifted: %q", got.Kind)
	}
	if got.Actor != "tool.code_exec" {
		t.Errorf("the actor drifted: %q", got.Actor)
	}

	// The payload travels as the bus will carry it: JSON.
	raw, err := json.Marshal(got.Payload)
	if err != nil {
		t.Fatalf("marshal the payload: %v", err)
	}
	var body map[string]any
	if err := json.Unmarshal(raw, &body); err != nil {
		t.Fatalf("unmarshal the payload: %v (%s)", err, raw)
	}
	for key, want := range map[string]any{
		"language":          "python",
		"code_bytes":        float64(128),
		"exit_code":         float64(0),
		"net":               true,
		"project":           "myproj",
		"profile_effective": string(warden.ProfileNone),
		"duration_ms":       float64(1500),
	} {
		if body[key] != want {
			t.Errorf("the payload's %q = %v, want %v", key, body[key], want)
		}
	}
	if _, ok := body["timed_out"]; !ok {
		t.Errorf("the payload lost the timed_out flag: %v", body)
	}
}

// TestTheToolPublishesNothingWithoutAPort: a daemon with no publisher must
// not crash the tool — the run still happened, only the timeline entry is
// missing. The port is a nil interface here, so the check is on the port
// itself, not on what it wraps.
func TestTheToolPublishesNothingWithoutAPort(t *testing.T) {
	tool := NewWithWarden(nil, t.TempDir(), nil, false)
	tool.publish(context.Background(), "python", "", 0, false, &warden.Result{})
	// No publisher, no panic: the run's own result is unaffected.
}
