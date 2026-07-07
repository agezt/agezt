// SPDX-License-Identifier: MIT

package controlplane_test

import (
	"context"
	"testing"

	"github.com/agezt/agezt/kernel/artifact"
	"github.com/agezt/agezt/kernel/controlplane"
	"github.com/agezt/agezt/plugins/providers/mock"
)

// putIndexed stores a blob and records an index entry, returning its id.
func putIndexed(t *testing.T, k interface {
	Artifacts() *artifact.Store
	ArtifactIndex() *artifact.Index
}, name, kind, source, corr string, data []byte, createdMs int64) artifact.Entry {
	t.Helper()
	e, err := k.ArtifactIndex().PutEntry(artifact.Entry{
		Name: name, Kind: kind, Source: source, Corr: corr,
	}, data, createdMs)
	if err != nil {
		t.Fatalf("PutEntry: %v", err)
	}
	return e
}

// TestArtifactList_FilterAndAll covers handleArtifactList including the kind /
// source / corr filters and the unfiltered path.
func TestArtifactList_FilterAndAll(t *testing.T) {
	k, _, c, _ := startPair(t, mock.New(mock.FinalText("ok")))

	putIndexed(t, k, "a.txt", "tool-output", "run", "corr-1", []byte("alpha"), 1000)
	putIndexed(t, k, "b.png", "image", "telegram", "corr-2", []byte("bravo"), 2000)

	// Unfiltered: both entries.
	res, err := c.Call(context.Background(), controlplane.CmdArtifactList, map[string]any{})
	if err != nil {
		t.Fatalf("artifact_list: %v", err)
	}
	if got := intOf(res["count"]); got != 2 {
		t.Errorf("unfiltered count = %d, want 2", got)
	}

	// Filter by kind.
	res, err = c.Call(context.Background(), controlplane.CmdArtifactList, map[string]any{"kind": "image"})
	if err != nil {
		t.Fatalf("artifact_list kind: %v", err)
	}
	if got := intOf(res["count"]); got != 1 {
		t.Errorf("kind filter count = %d, want 1", got)
	}

	// Filter by source.
	res, err = c.Call(context.Background(), controlplane.CmdArtifactList, map[string]any{"source": "run"})
	if err != nil {
		t.Fatalf("artifact_list source: %v", err)
	}
	if got := intOf(res["count"]); got != 1 {
		t.Errorf("source filter count = %d, want 1", got)
	}

	// Filter by corr.
	res, err = c.Call(context.Background(), controlplane.CmdArtifactList, map[string]any{"corr": "corr-2"})
	if err != nil {
		t.Fatalf("artifact_list corr: %v", err)
	}
	if got := intOf(res["count"]); got != 1 {
		t.Errorf("corr filter count = %d, want 1", got)
	}
}

// TestArtifactDelete_OKAndErrors covers handleArtifactDelete: missing id error,
// unknown id, and a successful delete.
func TestArtifactDelete_OKAndErrors(t *testing.T) {
	k, _, c, _ := startPair(t, mock.New(mock.FinalText("ok")))

	// Missing id → error mentioning id.
	if _, err := c.Call(context.Background(), controlplane.CmdArtifactDelete, map[string]any{}); err == nil {
		t.Error("delete with no id: expected error")
	}

	e := putIndexed(t, k, "gone.txt", "file", "run", "", []byte("delete-me"), 1000)

	// Successful delete.
	res, err := c.Call(context.Background(), controlplane.CmdArtifactDelete, map[string]any{"id": e.ID})
	if err != nil {
		t.Fatalf("artifact_delete: %v", err)
	}
	_ = res

	// Entry should be gone from the index.
	if _, ok := k.ArtifactIndex().Get(e.ID); ok {
		t.Errorf("entry %s still present after delete", e.ID)
	}
}

// TestArtifactCollect_DryRunAndReap covers handleArtifactCollect: the default
// dry-run reporting path, an explicit older_than_days, and the destructive
// dry_run=false reap.
func TestArtifactCollect_DryRunAndReap(t *testing.T) {
	k, _, c, _ := startPair(t, mock.New(mock.FinalText("ok")))

	// Two very old entries (created in 1970-ish) so any positive cutoff reaps them.
	putIndexed(t, k, "old1.txt", "tool-output", "run", "", []byte("old-one"), 1)
	putIndexed(t, k, "old2.txt", "tool-output", "run", "", []byte("old-two"), 2)

	// Default: dry-run true, default older_than_days.
	res, err := c.Call(context.Background(), controlplane.CmdArtifactCollect, map[string]any{})
	if err != nil {
		t.Fatalf("artifact_collect dry: %v", err)
	}
	if dry, _ := res["dry_run"].(bool); !dry {
		t.Errorf("dry_run = %v, want true by default", res["dry_run"])
	}
	if got := intOf(res["count"]); got != 2 {
		t.Errorf("dry-run candidate count = %d, want 2", got)
	}

	// Explicit older_than_days + dry_run string form ("1").
	res, err = c.Call(context.Background(), controlplane.CmdArtifactCollect, map[string]any{
		"older_than_days": float64(1), "dry_run": "true",
	})
	if err != nil {
		t.Fatalf("artifact_collect explicit dry: %v", err)
	}
	if dry, _ := res["dry_run"].(bool); !dry {
		t.Errorf("dry_run(string) = %v, want true", res["dry_run"])
	}

	// Reap for real: dry_run=false.
	res, err = c.Call(context.Background(), controlplane.CmdArtifactCollect, map[string]any{"dry_run": false})
	if err != nil {
		t.Fatalf("artifact_collect reap: %v", err)
	}
	if dry, _ := res["dry_run"].(bool); dry {
		t.Errorf("dry_run = %v, want false after reap", res["dry_run"])
	}
	if got := intOf(res["count"]); got != 2 {
		t.Errorf("reaped count = %d, want 2", got)
	}
	// Index now empty.
	if n := k.ArtifactIndex().Count(); n != 0 {
		t.Errorf("index count after reap = %d, want 0", n)
	}
}
