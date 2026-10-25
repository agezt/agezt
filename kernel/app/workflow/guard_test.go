// SPDX-License-Identifier: MIT

package workflow

import (
	"context"
	"errors"
	"testing"

	graphs "github.com/agezt/agezt/kernel/modules/workflows/workflow"
)

// TestTheAppLayerRefusesWithoutALibrary: the workflow library is the DAEMON's
// file (W3.48) — the app layer holds the contract's port, and an app layer
// without one says so instead of dereferencing nothing.
//
// The refusal is the same answer the kernel gives, and it names the reason:
// "the workflow library is not available on this daemon". An operator must be
// able to tell an unwired daemon from a broken one.
func TestTheAppLayerRefusesWithoutALibrary(t *testing.T) {
	ctx := context.Background()

	// The read surface: the list is EMPTY (there is nothing to show), and the
	// show refuses.
	listing, err := NewReads(nil, nil, nil).PrepareList().List(ctx, ListInput{})
	if err != nil {
		t.Fatalf("the empty list must not fail: %v", err)
	}
	if listing.Count != 0 || len(listing.Workflows) != 0 {
		t.Fatalf("a library-less daemon has no workflows: %+v", listing)
	}

	if _, err := NewReads(nil, nil, nil).Show(ctx, ShowInput{Ref: "any"}); !errors.Is(err, errNoWorkflows) {
		t.Fatalf("show must refuse without a library, got %v", err)
	}

	// The execution surface: the run and the webhook both refuse.
	exec := NewExecution(nil, ExecutionHost{
		Correlation: func() string { return "corr" },
		Run: func(context.Context, string, string, any) (RunResult, error) {
			t.Fatal("the run must not reach the host without a library")
			return RunResult{}, nil
		},
	})
	if _, err := exec.Run(ctx, RunInput{Ref: "any"}); !errors.Is(err, errNoWorkflows) {
		t.Fatalf("run must refuse without a library, got %v", err)
	}
	if _, err := exec.Webhook(ctx, WebhookInput{Ref: "any", Secret: "s"}); !errors.Is(err, errNoWorkflows) {
		t.Fatalf("webhook must refuse without a library, got %v", err)
	}

	// The copilot surface: the refinement refuses.
	if _, err := NewCopilot(nil, nil, nil).PrepareRefine(RefineInput{Ref: "any"}); !errors.Is(err, errNoWorkflows) {
		t.Fatalf("refine must refuse without a library, got %v", err)
	}
}

// TestTheRefusalNamesItsReason: the refusal must say WHY — a bare error would
// leave the operator guessing between an unwired daemon, an empty library and
// a missing workflow.
func TestTheRefusalNamesItsReason(t *testing.T) {
	if errNoWorkflows.Error() != "the workflow library is not available on this daemon" {
		t.Fatalf("the refusal says %q", errNoWorkflows.Error())
	}
}

// TestTheLibraryPortIsTheReadSide: the app layer reads through the contract's
// READ port — Get and List. It never holds the lifecycle verbs, because the
// app layer is a reader of the daemon's library, not its editor.
func TestTheLibraryPortIsTheReadSide(t *testing.T) {
	// The app layer's own Reader interface is exactly the read side.
	var reader Reader = &graphPort{items: []graphs.Workflow{{Name: "owned"}}}
	if got := reader.List(); len(got) != 1 {
		t.Fatalf("the reader must see the library: %+v", got)
	}
	if _, found := reader.Get("owned"); !found {
		t.Fatal("the reader must find the stored workflow")
	}
}
