// SPDX-License-Identifier: MIT

package runtime_test

import (
	"context"
	"strings"
	"testing"
	"time"

	"github.com/agezt/agezt/kernel/approval"
	"github.com/agezt/agezt/kernel/contract/llm"
	"github.com/agezt/agezt/kernel/contract/toolapi"
	"github.com/agezt/agezt/kernel/edict"
	"github.com/agezt/agezt/kernel/runtime"
	"github.com/agezt/agezt/plugins/providers/mock"
)

// TestApprovalWaitSurvivesShutdownAsReAsk pins what a daemon restart does to a
// run that is waiting on an operator's approval (architecture/21 decision 5.5).
//
// Shutdown suspends then cancels the run. The cancelled approval must NOT be
// checkpointed as a denial: the loop returns on ctx.Err() before the next
// checkpoint, so the resume ticket ends before the gated tool call and the
// resumed run asks again. The operator's queue is re-created, not silently
// denied — what a restart does lose is the approval's ID and its timer.
func TestApprovalWaitSurvivesShutdownAsReAsk(t *testing.T) {
	var invoked int32
	prov := mock.New(testToolUse("c1", "approvalprobe", map[string]any{}), mock.FinalText("done"))
	eng := edict.New(edict.Options{
		Levels:    map[edict.Capability]edict.TrustLevel{"approvalprobe": edict.LevelAsk},
		AskPolicy: edict.AskPrompt,
	})
	reg := approval.New(approval.Config{Timeout: time.Minute})
	k, err := runtime.Open(runtime.Config{
		BaseDir:       t.TempDir(),
		Provider:      prov,
		Tools:         map[string]toolapi.Tool{"approvalprobe": probeTool{invoked: &invoked}},
		Edict:         eng,
		Approvals:     reg,
		ResumeEnabled: true,
	})
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = k.Close() })

	ctx, cancel := context.WithCancel(context.Background())
	done := make(chan error, 1)
	go func() {
		_, e := k.RunWith(ctx, "corr-appr", "do the probe")
		done <- e
	}()
	waitForPending(t, reg)

	k.Suspend("restart")
	cancel()
	<-done

	tk, ok, err := k.ResumeStore().Get("corr-appr")
	if err != nil || !ok {
		t.Fatalf("a run waiting on approval must be resumable after a restart: ok=%v err=%v", ok, err)
	}
	for _, m := range tk.Messages {
		if m.Role == llm.RoleTool && strings.Contains(strings.ToLower(m.Content), "cancel") {
			t.Fatalf("the shutdown's cancelled approval was checkpointed as a tool result: %q", m.Content)
		}
		if len(m.ToolCalls) > 0 {
			t.Fatalf("the ticket includes the gated tool call; resume would skip the approval: %+v", m)
		}
	}
	if invoked != 0 {
		t.Fatalf("the probe ran %d times without approval", invoked)
	}
}
