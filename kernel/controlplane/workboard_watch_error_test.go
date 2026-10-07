// SPDX-License-Identifier: MIT

package controlplane

import (
	"github.com/agezt/agezt/kernel/event"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestWorkboardAppWatchRejectsActualJournalFailureWithOneErrorFrame(t *testing.T) {
	for _, prefix := range []bool{false, true} {
		t.Run(map[bool]string{false: "empty", true: "partial"}[prefix], func(t *testing.T) {
			k, s, task, _, provider := workboardAppFixture(t)
			if prefix {
				if _, err := k.Bus().Publish(event.Spec{Kind: event.KindWorkboardTaskUpdated, Subject: "workboard." + task.ID, Actor: "fixture", Payload: map[string]any{"id": task.ID}}); err != nil {
					t.Fatal(err)
				}
			}
			if err := k.Journal().Close(); err != nil {
				t.Fatal(err)
			}
			file, err := os.OpenFile(filepath.Join(k.BaseDir(), "journal", "00000001.jsonl"), os.O_WRONLY|os.O_APPEND, 0600)
			if err != nil {
				t.Fatal(err)
			}
			_, writeErr := file.WriteString("{broken owned fixture}\n")
			closeErr := file.Close()
			if writeErr != nil || closeErr != nil {
				t.Fatal(writeErr, closeErr)
			}
			responses := callAppHost(t, s, Request{ID: "watch-failure", Cmd: CmdWorkboardWatch, Token: "primary", Args: map[string]any{"id": task.ID}})
			if len(responses) != 1 || responses[0].Type != RespError || responses[0].Result != nil || responses[0].Event != nil || !strings.Contains(responses[0].Error, "journal: decode") || provider.CallCount() != 0 {
				t.Fatalf("EXPECTED: one error frame, original read cause, no snapshot/provider effect; ACTUAL: responses=%+v calls=%d", responses, provider.CallCount())
			}
		})
	}
}
