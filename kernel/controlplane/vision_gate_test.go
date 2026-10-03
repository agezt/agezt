// SPDX-License-Identifier: MIT

package controlplane_test

import (
	"context"
	"strings"
	"testing"

	"github.com/agezt/agezt/kernel/contract/llm"
	"github.com/agezt/agezt/kernel/controlplane"
	"github.com/agezt/agezt/kernel/event"
	"github.com/agezt/agezt/kernel/runtime"
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

func TestRun_VisionGate_CaptionsForRequestedTextModel(t *testing.T) {
	prov := mock.New()
	var sidecar, primary int
	prov.Responder = func(req llm.CompletionRequest) llm.CompletionResponse {
		if req.TaskType == "vision" {
			sidecar++
			if req.Model != "vision" || len(req.Messages[0].Images) != 1 {
				t.Errorf("sidecar request=%+v", req)
			}
			return mock.FinalText("a red square")
		}
		primary++
		if req.Model != "requested-text" {
			t.Errorf("primary model=%q", req.Model)
		}
		var captioned bool
		for _, msg := range req.Messages {
			captioned = captioned || strings.Contains(msg.Content, "a red square")
			if len(msg.Images) != 0 {
				t.Error("captioned run kept raw images")
			}
		}
		if !captioned {
			t.Error("primary lost caption")
		}
		return mock.FinalText("handled")
	}
	_, _, c, _ := startPairWithConfig(t, runtime.Config{
		Provider: prov, Model: "default-text", VisionModel: func() (string, bool) { return "vision", true },
	})
	res, err := c.Stream(context.Background(), controlplane.CmdRun,
		map[string]any{"intent": "describe", "model": "requested-text", "images": []any{"photo"}}, func(*event.Event) {})
	if err != nil || res["answer"] != "handled" {
		t.Fatalf("response=%v error=%v", res, err)
	}
	if sidecar != 1 || primary != 1 {
		t.Errorf("sidecar=%d primary=%d", sidecar, primary)
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
