// SPDX-License-Identifier: MIT

package controlplane_test

import (
	"context"
	"strings"
	"testing"

	"github.com/agezt/agezt/kernel/controlplane"
	"github.com/agezt/agezt/kernel/event"
	"github.com/agezt/agezt/plugins/providers/mock"
)

// TestRun_VisionGate_RejectsImageOnNonVisionModel — a run carrying image
// attachments is rejected pre-flight when the active model isn't confirmed
// vision-capable, and a capability.rejected event is journaled (M91).
func TestRun_VisionGate_RejectsImageOnNonVisionModel(t *testing.T) {
	k, _, c, _ := startPair(t, mock.New(mock.FinalText("hi")))

	_, err := c.Stream(context.Background(), controlplane.CmdRun,
		map[string]any{"intent": "describe this", "images": []any{"photo.png"}},
		func(e *event.Event) {})
	if err == nil {
		t.Fatalf("expected rejection for image on a non-vision model, got nil")
	}
	if !strings.Contains(err.Error(), "vision") {
		t.Errorf("error = %q; want it to mention vision", err.Error())
	}

	// The rejection is journaled as capability.rejected with capability=vision.
	var found bool
	_ = k.Journal().Range(func(e *event.Event) error {
		if e.Kind == event.KindCapabilityRejected {
			if strings.Contains(string(e.Payload), `"vision"`) {
				found = true
			}
		}
		return nil
	})
	if !found {
		t.Errorf("no capability.rejected{capability:vision} event journaled")
	}
}

// TestRun_NoImage_Unaffected — an ordinary run (no images) is not gated (M91).
func TestRun_NoImage_Unaffected(t *testing.T) {
	_, _, c, _ := startPair(t, mock.New(mock.FinalText("ok")))
	res, err := c.Stream(context.Background(), controlplane.CmdRun,
		map[string]any{"intent": "hello"}, func(e *event.Event) {})
	if err != nil {
		t.Fatalf("plain run errored: %v", err)
	}
	if res["answer"] != "ok" {
		t.Errorf("answer = %v want ok", res["answer"])
	}
}
