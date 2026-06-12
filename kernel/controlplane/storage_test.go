// SPDX-License-Identifier: MIT

package controlplane_test

import (
	"context"
	"testing"

	"github.com/agezt/agezt/kernel/controlplane"
	"github.com/agezt/agezt/kernel/event"
	"github.com/agezt/agezt/plugins/providers/mock"
)

// TestStorageStats_BreaksDownHomeDir — storage_stats inventories the home dir
// per top-level subdirectory with bytes + file counts, and the journal (which
// has real bytes after an event) appears in the breakdown (M927).
func TestStorageStats_BreaksDownHomeDir(t *testing.T) {
	k, srv, c, _ := startPair(t, mock.New(mock.FinalText("ok")))
	srv.SetDiskFree(func(string) (free, total uint64, err error) {
		return 25 << 30, 100 << 30, nil
	})
	if _, err := k.Bus().Publish(event.Spec{
		Subject: "task", Kind: event.KindTaskReceived, Actor: "a",
		CorrelationID: "r1", Payload: map[string]string{"intent": "grow the journal"},
	}); err != nil {
		t.Fatalf("Publish: %v", err)
	}

	res, err := c.Call(context.Background(), controlplane.CmdStorageStats, nil)
	if err != nil {
		t.Fatalf("Call: %v", err)
	}
	if tb := intOf(res["total_bytes"]); tb <= 0 {
		t.Errorf("total_bytes = %d, want > 0", tb)
	}
	dirs, ok := res["dirs"].([]any)
	if !ok || len(dirs) == 0 {
		t.Fatalf("dirs = %#v, want non-empty list", res["dirs"])
	}
	var journalBytes int64
	var journalLabel string
	for _, d := range dirs {
		m, _ := d.(map[string]any)
		if m["name"] == "journal" {
			journalBytes = int64(intOf(m["bytes"]))
			journalLabel, _ = m["label"].(string)
		}
	}
	if journalBytes <= 0 {
		t.Errorf("journal dir bytes = %d, want > 0 after an event", journalBytes)
	}
	if journalLabel == "" {
		t.Errorf("journal dir should carry a human label")
	}
	if avail, _ := res["disk_available"].(bool); !avail {
		t.Errorf("disk_available = false, want true (probe injected)")
	}
}

// TestStorageStats_NoProbe — without a disk-free probe the breakdown still
// works and free space is just flagged unavailable, mirroring disk_stats.
func TestStorageStats_NoProbe(t *testing.T) {
	_, _, c, _ := startPair(t, mock.New(mock.FinalText("ok")))
	res, err := c.Call(context.Background(), controlplane.CmdStorageStats, nil)
	if err != nil {
		t.Fatalf("Call: %v", err)
	}
	if avail, _ := res["disk_available"].(bool); avail {
		t.Errorf("disk_available = true, want false when no probe is injected")
	}
	if _, ok := res["total_bytes"]; !ok {
		t.Errorf("total_bytes should always be present")
	}
}
