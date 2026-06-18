// SPDX-License-Identifier: MIT

package controlplane

// Per-directory storage breakdown of the daemon's home dir (M927). disk_stats
// (M131) answers "how big is the journal and is the disk filling"; this answers
// WHAT under ~/.agezt is taking the space — the read side of a cleanup
// decision. Every subsystem owns one top-level subdirectory, so a per-subdir
// walk with a label map is a faithful inventory. Read-only; the collectors
// (artifact_collect, memory_prune, reaper_scan) do the actual reclaiming,
// each dry-run-first.

import (
	"io/fs"
	"net"
	"os"
	"path/filepath"
	"sort"
)

// storageLabels names what each well-known home subdirectory holds, so the
// Storage view can explain the breakdown instead of showing bare dir names.
// Unknown dirs (a plugin's, a future subsystem's) simply carry no label.
var storageLabels = map[string]string{
	"journal":    "Append-only event log — full retention, segments rotate but are never deleted",
	"state":      "Durable kernel state (roster, standing orders, projections)",
	"memory":     "Personal knowledge store (facts, preferences, observations)",
	"artifacts":  "Content-addressed blob store (inbound files, tool outputs) + metadata index",
	"worldmodel": "Entity/relation knowledge graph",
	"skills":     "Skill registry + agentskills.io bundles",
	"cadence":    "Typed schedules (agent/workflow/system-task/tool jobs)",
	"standing":   "Standing orders",
	"roster":     "Agent roster profiles",
	"toolforge":  "Custom forged tool definitions",
	"mcp":        "MCP server registry",
	"workflows":  "Workflow definitions + run state",
	"datalake":   "Personal Data Lake collections",
	"board":      "Shared agent message board",
	"catalog":    "Skill/tool catalog store",
	"tenants":    "Per-tenant homes (each with its own journal/state/memory)",
	"bin":        "Self-update staging binaries",
	"sandbox":    "Code-execution sandbox projects (scratch)",
	"workspace":  "Agent tool workspace (scratch files)",
	"runtime":    "Runtime ephemera (socket, token, policy overlay)",
	"convo":      "Conversation transcripts",
	"sessions":   "Channel session state",
}

type storageDir struct {
	Name  string `json:"name"`
	Bytes int64  `json:"bytes"`
	Files int64  `json:"files"`
	Label string `json:"label,omitempty"`
}

func (s *Server) handleStorageStats(conn net.Conn, req Request) {
	base := s.k.BaseDir()
	entries, _ := os.ReadDir(base)

	var dirs []storageDir
	var rootBytes, rootFiles, totalBytes, totalFiles int64
	for _, e := range entries {
		if e.IsDir() {
			b, f := dirUsage(filepath.Join(base, e.Name()))
			dirs = append(dirs, storageDir{Name: e.Name(), Bytes: b, Files: f, Label: storageLabels[e.Name()]})
			totalBytes += b
			totalFiles += f
			continue
		}
		if info, err := e.Info(); err == nil {
			rootBytes += info.Size()
			rootFiles++
		}
	}
	if rootFiles > 0 {
		dirs = append(dirs, storageDir{Name: "(home root)", Bytes: rootBytes, Files: rootFiles, Label: "Loose files at the home root"})
		totalBytes += rootBytes
		totalFiles += rootFiles
	}
	sort.Slice(dirs, func(i, j int) bool {
		if dirs[i].Bytes != dirs[j].Bytes {
			return dirs[i].Bytes > dirs[j].Bytes
		}
		return dirs[i].Name < dirs[j].Name
	})

	result := map[string]any{
		"base_dir":       base,
		"total_bytes":    totalBytes,
		"total_files":    totalFiles,
		"dirs":           dirs,
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

// dirUsage sums sizes and counts of regular files under dir. Best-effort like
// dirSize: a missing dir or unreadable entry contributes 0 — an inventory must
// never be the reason a diagnostic errors.
func dirUsage(dir string) (bytes, files int64) {
	_ = filepath.WalkDir(dir, func(_ string, d fs.DirEntry, err error) error {
		if err != nil || d.IsDir() {
			return nil
		}
		if info, err := d.Info(); err == nil {
			bytes += info.Size()
			files++
		}
		return nil
	})
	return bytes, files
}
