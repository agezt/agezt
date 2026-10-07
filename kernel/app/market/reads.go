// SPDX-License-Identifier: MIT
package market

import (
	"context"
	"errors"
	core "github.com/agezt/agezt/kernel/market"
	"maps"
	"slices"
	"strings"
)

// Reader retains the selected manager's catalogue, provenance and source reads.
type Reader interface {
	List(string) ([]core.Listing, error)
	Show(string, string) (core.Pack, core.InstalledPack, bool, error)
	Sources() ([]core.Source, error)
}
type Reads struct{ reader Reader }

func NewReads(reader Reader) *Reads { return &Reads{reader: reader} }

type ListInput struct{ Query string }
type ShowInput struct{ Marketplace, Name string }
type SourcesInput struct{}

func (s *Reads) List(_ context.Context, in ListInput) (ListOutput, error) {
	if s.reader == nil {
		return ListOutput{}, errors.New("marketplace not available on this daemon")
	}
	listings, err := s.reader.List(in.Query)
	if err != nil {
		return ListOutput{}, err
	}
	rows := make([]core.Listing, 0, len(listings))
	for _, row := range listings {
		row.Tags = slices.Clone(row.Tags)
		rows = append(rows, row)
	}
	return ListOutput{Packs: rows, Count: len(rows)}, nil
}
func (s *Reads) Show(_ context.Context, in ShowInput) (ShowOutput, error) {
	if s.reader == nil {
		return ShowOutput{}, errors.New("marketplace not available on this daemon")
	}
	name := strings.TrimSpace(in.Name)
	if name == "" {
		return ShowOutput{}, errors.New("args.name required")
	}
	pack, installed, found, err := s.reader.Show(in.Marketplace, name)
	if err != nil {
		return ShowOutput{}, err
	}
	skills, mcps, tools := pack.Counts()
	skillRows := make([]SkillView, 0, len(pack.Skills))
	for _, ps := range pack.Skills {
		row := SkillView{SkillMD: ps.SkillMD}
		if summary, serr := core.SkillSummary(ps); serr == nil {
			name, desc, _ := strings.Cut(summary, " — ")
			row.Name = &name
			row.Description = &desc
		}
		skillRows = append(skillRows, row)
	}
	names := make([]string, 0, len(pack.MCPServers))
	for _, srv := range pack.MCPServers {
		names = append(names, srv.Name)
	}
	return ShowOutput{Pack: cloneReadPack(pack), SkillCount: skills, MCPCount: mcps, ToolCount: tools, Skills: skillRows, MCPServers: names, Tools: slices.Clone(pack.ToolRequirements), Installed: found, InstalledAt: installed.InstalledMS, Vet: core.VetPack(pack)}, nil
}
func (s *Reads) Sources(_ context.Context, _ SourcesInput) (SourcesOutput, error) {
	if s.reader == nil {
		return SourcesOutput{}, errors.New("marketplace not available on this daemon")
	}
	sources, err := s.reader.Sources()
	if err != nil {
		return SourcesOutput{}, err
	}
	rows := make([]core.Source, len(sources))
	copy(rows, sources)
	return SourcesOutput{Sources: rows, Count: len(rows)}, nil
}

// The former JSON projection owned nested manifest values. Retain that ownership
// without converting int64 values through float64; preserve nil/empty collections.
func cloneReadPack(pack core.Pack) core.Pack {
	pack.Tags = slices.Clone(pack.Tags)
	pack.Keywords = slices.Clone(pack.Keywords)
	pack.ToolRequirements = slices.Clone(pack.ToolRequirements)
	pack.Skills = slices.Clone(pack.Skills)
	for i := range pack.Skills {
		resources := maps.Clone(pack.Skills[i].Resources)
		for key, data := range resources {
			resources[key] = slices.Clone(data)
		}
		pack.Skills[i].Resources = resources
	}
	pack.MCPServers = slices.Clone(pack.MCPServers)
	for i := range pack.MCPServers {
		srv := &pack.MCPServers[i]
		srv.Args = slices.Clone(srv.Args)
		srv.ToolAllow = slices.Clone(srv.ToolAllow)
		srv.Env = maps.Clone(srv.Env)
		srv.Headers = maps.Clone(srv.Headers)
	}
	if pack.Signature != nil {
		signature := *pack.Signature
		pack.Signature = &signature
	}
	return pack
}
