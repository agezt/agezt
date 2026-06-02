// SPDX-License-Identifier: MIT

package controlplane_test

import (
	"context"
	"testing"

	"github.com/agezt/agezt/kernel/controlplane"
	"github.com/agezt/agezt/kernel/event"
	"github.com/agezt/agezt/plugins/providers/mock"
)

// TestDiskStats_ReportsJournalAndFreeSpace — the disk handler reports the
// journal's on-disk size and, when a disk-free probe is injected, the free/total
// bytes and percentage (M131).
func TestDiskStats_ReportsJournalAndFreeSpace(t *testing.T) {
	k, srv, c, _ := startPair(t, mock.New(mock.FinalText("ok")))
	// Inject a deterministic disk-free probe: 25 GB free of 100 GB → 25%.
	srv.SetDiskFree(func(string) (free, total uint64, err error) {
		return 25 << 30, 100 << 30, nil
	})
	// Write an event so the journal segment has real bytes on disk.
	if _, err := k.Bus().Publish(event.Spec{
		Subject: "task", Kind: event.KindTaskReceived, Actor: "a",
		CorrelationID: "r1", Payload: map[string]string{"intent": "grow the journal"},
	}); err != nil {
		t.Fatalf("Publish: %v", err)
	}

	res, err := c.Call(context.Background(), controlplane.CmdDiskStats, nil)
	if err != nil {
		t.Fatalf("Call: %v", err)
	}
	if avail, _ := res["disk_available"].(bool); !avail {
		t.Fatalf("disk_available = false, want true (probe injected)")
	}
	if jb := intOf(res["journal_bytes"]); jb <= 0 {
		t.Errorf("journal_bytes = %d, want > 0 after an event", jb)
	}
	if pct, _ := res["disk_free_pct"].(float64); pct < 24 || pct > 26 {
		t.Errorf("disk_free_pct = %v, want ~25", pct)
	}
	if fb := int64(intOf(res["disk_free_bytes"])); fb != 25<<30 {
		t.Errorf("disk_free_bytes = %d, want %d", fb, int64(25<<30))
	}
}

// TestDiskStats_NoProbeReportsUnavailable — without an injected probe the handler
// still reports the journal size but flags free space as unavailable rather than
// failing (M131).
func TestDiskStats_NoProbeReportsUnavailable(t *testing.T) {
	_, _, c, _ := startPair(t, mock.New(mock.FinalText("ok")))
	res, err := c.Call(context.Background(), controlplane.CmdDiskStats, nil)
	if err != nil {
		t.Fatalf("Call: %v", err)
	}
	if avail, _ := res["disk_available"].(bool); avail {
		t.Errorf("disk_available = true, want false when no probe is injected")
	}
	if _, ok := res["journal_bytes"]; !ok {
		t.Errorf("journal_bytes should always be present")
	}
}
