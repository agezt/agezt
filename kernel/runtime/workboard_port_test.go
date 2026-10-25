// SPDX-License-Identifier: MIT

package runtime_test

import (
	"context"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/agezt/agezt/kernel/contract/workboardapi"
	"github.com/agezt/agezt/kernel/modules/work/workboard"
	"github.com/agezt/agezt/kernel/runtime"
	"github.com/agezt/agezt/plugins/providers/mock"
)

// TestTheBoardRefusesWithoutAStore: the task board used to be opened by the
// kernel, so the field was never nil and the code never had to say what
// happens without one. Now the store file is the host's (W3.47) and a host
// may open a kernel without a board — and EVERY board operation must refuse
// instead of panicking.
//
// The refusal is the same answer everywhere: "the task board is not
// available on this daemon".
func TestTheBoardRefusesWithoutAStore(t *testing.T) {
	k, err := runtime.Open(runtime.Config{BaseDir: t.TempDir(), Provider: mock.New()})
	if err != nil {
		t.Fatalf("Open: %v", err)
	}
	t.Cleanup(func() { k.Close() })

	if k.Workboard() != nil {
		t.Fatal("the kernel must hold no board until the daemon wires one")
	}

	policy := &workboardapi.RetryPolicy{MaxAttempts: 2}
	for _, c := range []struct {
		name string
		run  func() error
	}{
		{"create", func() error {
			_, _, err := k.CreateWorkboardTask("corr", workboardapi.CreateSpec{Title: "no board"})
			return err
		}},
		{"claim", func() error {
			_, err := k.ClaimWorkboardTask("corr", "id", "agent", "run")
			return err
		}},
		{"heartbeat", func() error {
			_, err := k.HeartbeatWorkboardTask("corr", "id", "agent", "run")
			return err
		}},
		{"comment", func() error {
			_, err := k.CommentWorkboardTask("corr", "id", "author", "body")
			return err
		}},
		{"block", func() error {
			_, err := k.BlockWorkboardTask("corr", "id", "actor", "reason")
			return err
		}},
		{"fail", func() error {
			_, _, err := k.FailWorkboardTask("corr", "id", "actor", "reason")
			return err
		}},
		{"unblock", func() error {
			_, err := k.UnblockWorkboardTask("corr", "id", "actor")
			return err
		}},
		{"retry policy", func() error {
			_, err := k.SetWorkboardRetryPolicy("corr", "id", "actor", policy)
			return err
		}},
		{"complete", func() error {
			_, err := k.CompleteWorkboardTask("corr", "id", "actor")
			return err
		}},
		{"review", func() error {
			_, err := k.ReviewWorkboardTask("corr", "id", "actor", "summary")
			return err
		}},
		{"archive", func() error {
			_, err := k.ArchiveWorkboardTask("corr", "id", "actor")
			return err
		}},
		{"link", func() error {
			_, err := k.LinkWorkboardTask("corr", "id", "type", "target")
			return err
		}},
		{"dependency", func() error {
			_, err := k.AddWorkboardDependency("corr", "id", "other")
			return err
		}},
		{"reclaim", func() error {
			_, err := k.ReclaimStaleWorkboardTask("corr", "id", "actor", time.Minute)
			return err
		}},
		{"sweep", func() error {
			_, err := k.SweepStaleWorkboardClaims("corr", "actor", time.Minute, 10)
			return err
		}},
		{"prove", func() error {
			_, err := k.ProveTask(context.Background(), "corr", "id", "answer")
			return err
		}},
	} {
		err := c.run()
		if err == nil {
			t.Errorf("%s: the operation reported success without a board", c.name)
			continue
		}
		if !strings.Contains(err.Error(), "not available") {
			t.Errorf("%s: the refusal = %v, want the board's plain refusal", c.name, err)
		}
	}
}

// TestTheKernelDrivesTheBoardThroughThePort: the kernel holds the CONTRACT's
// host port, and it drives the whole lifecycle through it — the port is not a
// read-only window onto somebody else's store.
func TestTheKernelDrivesTheBoardThroughThePort(t *testing.T) {
	dir := t.TempDir()
	store, err := workboard.OpenStore(filepath.Join(dir, "workboard"))
	if err != nil {
		t.Fatalf("open the workboard store: %v", err)
	}
	k, err := runtime.Open(runtime.Config{BaseDir: dir, Provider: mock.New(), Workboard: store})
	if err != nil {
		t.Fatalf("Open: %v", err)
	}
	t.Cleanup(func() { k.Close() })

	if k.Workboard() == nil {
		t.Fatal("the kernel must hold the board it was given")
	}
	task, created, err := k.CreateWorkboardTask("corr", workboardapi.CreateSpec{Title: "driven"})
	if err != nil {
		t.Fatalf("CreateWorkboardTask: %v", err)
	}
	if !created || task.Title != "driven" {
		t.Fatalf("the board did not take the task: %+v", task)
	}
	// The port must reach the LIVE store — a task the kernel stored has to be
	// visible through it.
	got, ok := k.Workboard().Get(task.ID)
	if !ok || got.Title != "driven" {
		t.Fatalf("the port does not answer the live board: %+v", got)
	}
}
