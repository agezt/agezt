// SPDX-License-Identifier: MIT

package storage

import (
	"context"
	"path/filepath"
)

type DiskInput struct{}

// DiskOutput is the journal's size beside the free space on the filesystem the
// daemon's home lives on; the disk fields appear only when that is known.
type DiskOutput struct {
	BaseDir         string   `json:"base_dir"`
	JournalBytes    int64    `json:"journal_bytes"`
	DiskAvailable   bool     `json:"disk_available"`
	DiskFreeBytes   *uint64  `json:"disk_free_bytes,omitempty"`
	DiskTotalBytes  *uint64  `json:"disk_total_bytes,omitempty"`
	DiskFreePercent *float64 `json:"disk_free_pct,omitempty"`
}

// Disk reports the append-only journal's size and the home filesystem's free
// space. A missing or unreadable journal counts as empty, and an unknown or
// zero-sized filesystem leaves the disk fields out: disk stats never fail.
func (s *Service) Disk(_ context.Context, _ DiskInput) (DiskOutput, error) {
	journalBytes, _ := s.files.Usage(filepath.Join(s.base, "journal"))
	out := DiskOutput{BaseDir: s.base, JournalBytes: journalBytes}
	out.DiskFreeBytes, out.DiskTotalBytes, out.DiskFreePercent, out.DiskAvailable = s.freeSpace()
	return out, nil
}
