// SPDX-License-Identifier: MIT
package workboard_test

import (
	"context"
	"errors"
	appwork "github.com/agezt/agezt/kernel/app/workboard"
	"github.com/agezt/agezt/kernel/event"
	"github.com/agezt/agezt/kernel/journal"
	tasks "github.com/agezt/agezt/kernel/workboard"
	"os"
	"path/filepath"
	"reflect"
	"testing"
	"time"
)

func TestWorkboardWatchRejectsActualJournalReadFailure(t *testing.T) {
	for _, prefix := range []bool{false, true} {
		t.Run(map[bool]string{false: "empty", true: "partial"}[prefix], func(t *testing.T) {
			root := t.TempDir()
			store, err := tasks.OpenStore(filepath.Join(root, "work"))
			if err != nil {
				t.Fatal(err)
			}
			task, _, err := store.Create(tasks.CreateSpec{Title: "owned"}, time.UnixMilli(100))
			if err != nil {
				t.Fatal(err)
			}
			dir := filepath.Join(root, "journal")
			log, err := journal.Open(dir, journal.Options{})
			if err != nil {
				t.Fatal(err)
			}
			if prefix {
				if _, err := log.Append(event.Spec{Kind: event.KindWorkboardTaskUpdated, Subject: "workboard." + task.ID, Actor: "fixture", Payload: map[string]any{"id": task.ID}}); err != nil {
					t.Fatal(err)
				}
			}
			if err := log.Close(); err != nil {
				t.Fatal(err)
			}
			file, err := os.OpenFile(filepath.Join(dir, "00000001.jsonl"), os.O_WRONLY|os.O_APPEND, 0600)
			if err != nil {
				t.Fatal(err)
			}
			_, writeErr := file.WriteString("{broken owned fixture}\n")
			closeErr := file.Close()
			if writeErr != nil || closeErr != nil {
				t.Fatal(writeErr, closeErr)
			}
			cause := log.Range(func(*event.Event) error { return nil })
			if cause == nil {
				t.Fatal("invalid corruption fixture")
			}
			out, err := appwork.NewWatch(store, log).Watch(context.Background(), appwork.WatchInput{ID: task.ID})
			if err == nil || err.Error() != cause.Error() || !reflect.DeepEqual(out, appwork.WatchOutput{}) {
				t.Fatalf("EXPECTED: original actual journal read cause and zero output; ACTUAL: err=%v out=%+v cause=%v", err, out, cause)
			}
		})
	}
}
func TestWorkboardWatchRejectsDependencyReadFailureWithoutPartialOutput(t *testing.T) {
	cause := errors.New("owned dependency read cause")
	store := &watchStore{found: true, task: tasks.Task{ID: "owned"}, blocked: []tasks.DependencyState{{ID: "parent", Status: tasks.StatusBlocked}}, cause: cause}
	reader := watchJournal{rows: []event.Event{{Seq: 1, Subject: "workboard.owned", Kind: event.KindWorkboardTaskUpdated}}}
	out, err := appwork.NewWatch(store, reader).Watch(context.Background(), appwork.WatchInput{ID: "owned"})
	if err != cause || !reflect.DeepEqual(out, appwork.WatchOutput{}) {
		t.Fatalf("EXPECTED: exact dependency read cause and zero output; ACTUAL: err=%v out=%+v", err, out)
	}
}
