// SPDX-License-Identifier: MIT

package controlplane

// Disk-space observability (M131). The journal is append-only and grows forever
// (segments rotate but are never deleted), so on a small host the #1 silent
// failure mode is a full disk: once it fills, the journal can no longer write
// and the daemon stops recording what it does. `agt doctor` and `agt disk` fold
// this into a first-class surface so an operator sees it coming — the daemon
// reports its own journal size and the free space on the filesystem it lives on
// (it knows its base dir; the client may be on another host).

import (
	"io/fs"
	"net"
	"path/filepath"
)

func (s *Server) handleDiskStats(conn net.Conn, req Request) {
	base := s.k.BaseDir()
	journalBytes := dirSize(filepath.Join(base, "journal"))

	result := map[string]any{
		"base_dir":       base,
		"journal_bytes":  journalBytes,
		"disk_available": false,
	}
	if s.diskFree != nil {
		if free, total, err := s.diskFree(base); err == nil && total > 0 {
			result["disk_free_bytes"] = free
			result["disk_total_bytes"] = total
			result["disk_free_pct"] = float64(free) / float64(total) * 100
			result["disk_available"] = true
		}
	}
	s.writeResp(conn, Response{ID: req.ID, Type: RespResult, Result: result})
}

// dirSize sums the sizes of all regular files under dir (the journal's rotated
// segments). Best-effort: a missing directory or an unreadable entry contributes
// 0 rather than failing the whole call — disk stats must never be the reason a
// diagnostic errors.
func dirSize(dir string) int64 {
	var total int64
	_ = filepath.WalkDir(dir, func(_ string, d fs.DirEntry, err error) error {
		if err != nil || d.IsDir() {
			return nil
		}
		if info, err := d.Info(); err == nil {
			total += info.Size()
		}
		return nil
	})
	return total
}
