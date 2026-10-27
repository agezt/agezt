// SPDX-License-Identifier: MIT

package main

// Provenance: the daemon's contract test for the pulse (W3.66) — that the
//             composition root injects the structured-output path the salience
//             refine needs, because the path lives in another module.

import (
	"context"
	"encoding/json"
	"io"
	"strings"
	"testing"
	"time"

	"github.com/agezt/agezt/internal/brand"
	"github.com/agezt/agezt/kernel/event"
	"github.com/agezt/agezt/kernel/pulse"
	kernelruntime "github.com/agezt/agezt/kernel/runtime"
	"github.com/agezt/agezt/plugins/providers/mock"
)

// pulseTestObserver reports one delta per poll — enough for the engine to
// score something.
type pulseTestObserver struct{}

func (pulseTestObserver) Name() string { return "test-observer" }

func (pulseTestObserver) Poll(ctx context.Context) ([]pulse.Delta, error) {
	return []pulse.Delta{{
		Source:  "test",
		Kind:    "usage",
		Summary: "the disk is filling up",
		Hints:   map[string]string{"severity": "medium"},
	}}, nil
}

// TestTheDaemonWiresTheStructuredOutputPathIntoPulse: the salience refine
// needs the structured-output path, and it lives in ANOTHER module — so the
// daemon injects it rather than letting the pulse module reach for it.
//
// The path is the only way a model's answer becomes a typed score. Without it
// the refine is skipped and the pulse falls back to the severity score — the
// operator would never learn that the LLM refine is dead, because nothing
// fails: the pulse simply gets quieter.
func TestTheDaemonWiresTheStructuredOutputPathIntoPulse(t *testing.T) {
	t.Setenv(brand.EnvPrefix+"PULSE_LLM", "on")
	dir := t.TempDir()
	prov := mock.New(mock.FinalText(`{"score":0.87,"reason":"the disk is about to fill"}`))
	k, err := openAppKernel(kernelruntime.Config{BaseDir: dir, Provider: prov})
	if err != nil {
		t.Fatalf("openAppKernel: %v", err)
	}
	t.Cleanup(func() { _ = k.Close() })

	eng, _ := buildPulse(k, nil, "test-model", io.Discard, nil)
	eng.AddObserver(pulseTestObserver{})
	ctx, cancel := context.WithCancel(context.Background())
	t.Cleanup(cancel)
	eng.Start(ctx)
	eng.Beat()

	deadline := time.Now().Add(10 * time.Second)
	for time.Now().Before(deadline) {
		events, err := k.Journal().Tail(200)
		if err == nil {
			for _, ev := range events {
				if ev.Kind != event.KindSalienceScored {
					continue
				}
				var payload struct {
					Reason string  `json:"reason"`
					Score  float64 `json:"score"`
				}
				if err := json.Unmarshal(ev.Payload, &payload); err != nil {
					continue
				}
				if !strings.HasPrefix(payload.Reason, "llm: ") {
					continue
				}
				if payload.Reason != "llm: the disk is about to fill" {
					t.Fatalf("the refine reason = %q, want the model's reason", payload.Reason)
				}
				if payload.Score != 0.87 {
					t.Fatalf("the refine score = %v, want the model's 0.87", payload.Score)
				}
				return
			}
		}
		time.Sleep(50 * time.Millisecond)
	}
	t.Fatal("the daemon's structured-output path must drive the pulse's salience refine")
}
