// SPDX-License-Identifier: MIT

//go:build windows

package pulse

import (
	"os"
	"testing"
)

func TestDiskUsageWindows(t *testing.T) {
	free, total, err := DiskUsage(t.TempDir())
	if err != nil {
		t.Fatalf("DiskUsage(temp): %v", err)
	}
	if free == 0 || total == 0 || free > total {
		t.Fatalf("invalid disk usage: free=%d total=%d", free, total)
	}

	if _, _, err := DiskUsage("bad\x00path"); err == nil {
		t.Fatal("embedded NUL path must fail UTF-16 conversion")
	}
	missing := os.TempDir() + `\agezt-definitely-missing-volume\child`
	if _, _, err := DiskUsage(missing); err == nil {
		t.Fatal("missing path must fail")
	}
}
