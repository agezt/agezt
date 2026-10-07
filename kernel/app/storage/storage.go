// SPDX-License-Identifier: MIT

package storage

import (
	"context"
	"io/fs"
	"path/filepath"
	"sort"
)

type Filesystem interface {
	ReadDir(string) ([]fs.DirEntry, error)
	Usage(string) (int64, int64)
}
type Service struct {
	base     string
	files    Filesystem
	diskFree func(string) (uint64, uint64, error)
}

func New(base string, files Filesystem, diskFree func(string) (uint64, uint64, error)) *Service {
	return &Service{base: base, files: files, diskFree: diskFree}
}

type Directory struct {
	Name  string `json:"name"`
	Bytes int64  `json:"bytes"`
	Files int64  `json:"files"`
	Label string `json:"label,omitempty"`
}
type StatsInput struct{}
type StatsOutput struct {
	BaseDir         string      `json:"base_dir"`
	TotalBytes      int64       `json:"total_bytes"`
	TotalFiles      int64       `json:"total_files"`
	Directories     []Directory `json:"dirs"`
	DiskAvailable   bool        `json:"disk_available"`
	DiskFreeBytes   *uint64     `json:"disk_free_bytes,omitempty"`
	DiskTotalBytes  *uint64     `json:"disk_total_bytes,omitempty"`
	DiskFreePercent *float64    `json:"disk_free_pct,omitempty"`
}

var labels = map[string]string{
	"journal":    "Append-only event log — full retention, segments rotate but are never deleted",
	"state":      "Durable kernel state (roster, standing orders, projections)",
	"memory":     "Personal knowledge store (facts, preferences, observations)",
	"artifacts":  "Content-addressed blob store (inbound files, tool outputs) + metadata index",
	"worldmodel": "Entity/relation knowledge graph",
	"skills":     "Skill registry + agentskills.io bundles",
	"cadence":    "Typed schedules (agent/workflow/system-task/tool jobs)",
	"standing":   "Standing orders",
	"resume":     "Durable in-flight-run tickets for restart resume",
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

func (s *Service) Stats(_ context.Context, _ StatsInput) (StatsOutput, error) {
	base := s.base
	entries, _ := s.files.ReadDir(base)

	var dirs []Directory
	var rootBytes, rootFiles, totalBytes, totalFiles int64
	for _, e := range entries {
		if e.IsDir() {
			b, f := s.files.Usage(filepath.Join(base, e.Name()))
			dirs = append(dirs, Directory{Name: e.Name(), Bytes: b, Files: f, Label: labels[e.Name()]})
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
		dirs = append(dirs, Directory{Name: "(home root)", Bytes: rootBytes, Files: rootFiles, Label: "Loose files at the home root"})
		totalBytes += rootBytes
		totalFiles += rootFiles
	}
	sort.Slice(dirs, func(i, j int) bool {
		if dirs[i].Bytes != dirs[j].Bytes {
			return dirs[i].Bytes > dirs[j].Bytes
		}
		return dirs[i].Name < dirs[j].Name
	})

	out := StatsOutput{BaseDir: base, TotalBytes: totalBytes, TotalFiles: totalFiles, Directories: dirs}
	if s.diskFree != nil {
		if free, total, err := s.diskFree(base); err == nil && total > 0 {
			pct := float64(free) / float64(total) * 100
			out.DiskFreeBytes = &free
			out.DiskTotalBytes = &total
			out.DiskFreePercent = &pct
			out.DiskAvailable = true
		}
	}
	return out, nil
}
