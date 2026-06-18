// SPDX-License-Identifier: MIT

package runtime_test

import (
	"context"
	"encoding/json"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	"github.com/agezt/agezt/kernel/agent"
	"github.com/agezt/agezt/kernel/approval"
	"github.com/agezt/agezt/kernel/edict"
	"github.com/agezt/agezt/kernel/event"
	"github.com/agezt/agezt/kernel/runtime"
	"github.com/agezt/agezt/plugins/providers/mock"
)

func TestRunWith_IntentRegretGateRoutesAmbiguousHighRegretActionToApproval(t *testing.T) {
	var invoked int32
	reg := approval.New(approval.Config{Timeout: 5 * time.Second})
	prov := mock.New(
		mock.ToolUse("c1", "approvalprobe", map[string]any{"path": "legacy/2023"}),
		mock.FinalText("done"),
	)
	k, err := runtime.Open(runtime.Config{
		BaseDir:                t.TempDir(),
		Provider:               prov,
		Tools:                  map[string]agent.Tool{"approvalprobe": probeTool{invoked: &invoked}},
		Edict:                  edict.New(edict.Options{UnknownAllow: true}),
		Approvals:              reg,
		IntentRegretGating:     true,
		DisableHeuristicBypass: true,
	})
	if err != nil {
		t.Fatalf("Open: %v", err)
	}
	t.Cleanup(func() { k.Close() })

	done := make(chan error, 1)
	go func() {
		_, _, err := k.Run(context.Background(), "clean files")
		done <- err
	}()

	req := waitForPending(t, reg)
	if req.CanonicalIntent != "clean files" {
		t.Fatalf("canonical intent = %q, want clean files", req.CanonicalIntent)
	}
	if req.AmbiguityScore < 0.6 || req.HarmfulInterpretation == "" {
		t.Fatalf("intent metadata incomplete: ambiguity=%v harmful=%q", req.AmbiguityScore, req.HarmfulInterpretation)
	}
	if req.RegretAxes["informational"] < 0.75 {
		t.Fatalf("regret axes = %+v, want high informational risk", req.RegretAxes)
	}
	if !strings.Contains(req.ConfirmationPrompt, "approvalprobe") || !strings.Contains(req.ConfirmationPrompt, "Confirm") {
		t.Fatalf("confirmation prompt is not targeted: %q", req.ConfirmationPrompt)
	}
	if err := reg.Resolve(req.ID, approval.DecisionDeny, "scope is ambiguous", "operator"); err != nil {
		t.Fatalf("Resolve: %v", err)
	}

	select {
	case err := <-done:
		if err != nil {
			t.Fatalf("Run: %v", err)
		}
	case <-time.After(3 * time.Second):
		t.Fatal("run did not finish after denial")
	}
	if atomic.LoadInt32(&invoked) != 0 {
		t.Fatalf("intent-gated tool executed %d times, want 0", invoked)
	}
}

func TestRunWith_IntentInterpretationIsJournaledWithoutRawUtterance(t *testing.T) {
	k, err := runtime.Open(runtime.Config{
		BaseDir:                t.TempDir(),
		Provider:               mock.New(mock.FinalText("done")),
		DisableHeuristicBypass: true,
	})
	if err != nil {
		t.Fatalf("Open: %v", err)
	}
	t.Cleanup(func() { k.Close() })

	if _, _, err := k.Run(context.Background(), "dosyaları temizle"); err != nil {
		t.Fatalf("Run: %v", err)
	}

	var found bool
	_ = k.Journal().Range(func(e *event.Event) error {
		if e.Kind != event.KindIntentInterpreted {
			return nil
		}
		var p struct {
			Hash            string  `json:"user_utterance_hash"`
			CanonicalIntent string  `json:"canonical_intent"`
			AmbiguityScore  float64 `json:"ambiguity_score"`
			Underdetermined bool    `json:"underdetermined"`
			HarmfulReading  string  `json:"harmful_reading"`
		}
		if err := json.Unmarshal(e.Payload, &p); err != nil {
			t.Fatalf("intent.interpreted payload: %v", err)
		}
		found = true
		if p.Hash == "" || strings.Contains(p.Hash, "dosya") {
			t.Fatalf("hash should not expose raw utterance: %q", p.Hash)
		}
		if p.CanonicalIntent != "dosyaları temizle" || !p.Underdetermined || p.AmbiguityScore < 0.6 || p.HarmfulReading == "" {
			t.Fatalf("intent payload incomplete: %+v", p)
		}
		return nil
	})
	if !found {
		t.Fatal("no intent.interpreted event")
	}
}
