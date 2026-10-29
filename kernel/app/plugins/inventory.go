// SPDX-License-Identifier: MIT
package plugins

import (
	"context"
	"sort"

	"github.com/agezt/agezt/kernel/contract/types"
)

// Registration is the daemon-supplied manifest projected by plugin_list. Spawning,
// hashes, allowlist enforcement and process lifecycle stay outside this read port.
// It is the contract type itself — not a copy of it — so the host hands its
// manifest over without a field-by-field projection and the two sides cannot
// drift.
type Registration = types.PluginInfo

// Reader is the plugins family's own narrow view of the manifest.
//
// The method is named "Manifest" — the daemon's own word for what it holds —
// and not "Plugins": the family's own port may not carry the name of the
// kernel verb it stands for.
type Reader interface{ Manifest() []Registration }

type Service struct{ reader Reader }

func New(reader Reader) *Service { return &Service{reader: reader} }

type ListInput struct{}
type Row struct {
	Prefix       string   `json:"prefix"`
	Path         string   `json:"path"`
	Args         []string `json:"args"`
	ToolCount    int      `json:"tool_count"`
	HashPinned   bool     `json:"hash_pinned"`
	AllowedTools []string `json:"allowed_tools"`
}
type ListOutput struct {
	Plugins []Row `json:"plugins"`
	Count   int   `json:"count"`
}

func (s *Service) List(_ context.Context, _ ListInput) (ListOutput, error) {
	plugins := s.reader.Manifest()
	rows := make([]Row, 0, len(plugins))
	for _, p := range plugins {
		args := make([]string, len(p.Args))
		copy(args, p.Args)
		var allowed []string
		if p.AllowedTools != nil {
			allowed = make([]string, len(p.AllowedTools))
			copy(allowed, p.AllowedTools)
		}
		rows = append(rows, Row{Prefix: p.Prefix, Path: p.Path, Args: args, ToolCount: p.ToolCount, HashPinned: p.HashPinned, AllowedTools: allowed})
	}
	sort.Slice(rows, func(i, j int) bool { return rows[i].Prefix < rows[j].Prefix })
	return ListOutput{Plugins: rows, Count: len(rows)}, nil
}
