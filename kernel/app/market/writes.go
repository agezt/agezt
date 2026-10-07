// SPDX-License-Identifier: MIT
package market

import (
	"context"
	"errors"
	"github.com/agezt/agezt/kernel/contract/opapi"
	"github.com/agezt/agezt/kernel/event"
	core "github.com/agezt/agezt/kernel/market"
	"slices"
	"strings"
)

// Writer retains core materialization, provenance, optional reverse APIs and sync.
type Writer interface {
	InstallContext(context.Context, string, string, string, string, func(core.Event) error) (core.InstalledPack, error)
	UninstallContext(context.Context, string, string, func(core.Event) error) error
	AddSource(string, string, string) (core.Source, error)
	RemoveSource(string) (bool, error)
	Sync(context.Context, string) ([]core.SyncResult, error)
}
type Writes struct {
	writer  Writer
	publish func(event.Kind, map[string]any) error
}

func NewWrites(writer Writer, publish func(event.Kind, map[string]any) error) *Writes {
	return &Writes{writer: writer, publish: publish}
}

type InstallInput struct{ CorrelationID, Marketplace, Name, Version string }
type UninstallInput struct{ CorrelationID, Name string }
type AddSourceInput struct{ Name, URL, PubKey string }
type RemoveSourceInput struct{ Name string }
type SyncInput struct{ Name string }

func writeCorrelation(ctx context.Context, explicit string) string {
	if owned := opapi.CorrelationFromContext(ctx); owned != "" {
		return owned
	}
	return explicit
}
func (s *Writes) published(kind event.Kind, payload map[string]any) error {
	if s.publish == nil {
		return nil
	}
	return s.publish(kind, payload)
}
func (s *Writes) Install(ctx context.Context, in InstallInput, emit func(core.Event) error) (InstallOutput, error) {
	if err := ctx.Err(); err != nil {
		return InstallOutput{}, err
	}
	if s.writer == nil {
		return InstallOutput{}, errors.New("marketplace not available on this daemon")
	}
	name := strings.TrimSpace(in.Name)
	if name == "" {
		return InstallOutput{}, errors.New("args.name required")
	}
	rec, err := s.writer.InstallContext(ctx, writeCorrelation(ctx, in.CorrelationID), in.Marketplace, name, in.Version, emit)
	if err != nil {
		return InstallOutput{}, err
	}
	if err := s.published(event.KindMarketPackInstalled, map[string]any{"pack": rec.Name, "version": rec.Version, "marketplace": rec.Marketplace, "skills": rec.SkillIDs, "mcp": rec.MCPServers, "tools": rec.ToolReqs, "unsigned": rec.Unsigned}); err != nil {
		return InstallOutput{}, err
	}
	rec.SkillIDs = slices.Clone(rec.SkillIDs)
	rec.MCPServers = slices.Clone(rec.MCPServers)
	rec.ToolReqs = slices.Clone(rec.ToolReqs)
	return rec, nil
}
func (s *Writes) Uninstall(ctx context.Context, in UninstallInput, emit func(core.Event) error) (UninstallOutput, error) {
	if err := ctx.Err(); err != nil {
		return UninstallOutput{}, err
	}
	if s.writer == nil {
		return UninstallOutput{}, errors.New("marketplace not available on this daemon")
	}
	name := strings.TrimSpace(in.Name)
	if name == "" {
		return UninstallOutput{}, errors.New("args.name required")
	}
	if err := s.writer.UninstallContext(ctx, writeCorrelation(ctx, in.CorrelationID), name, emit); err != nil {
		return UninstallOutput{}, err
	}
	if err := s.published(event.KindMarketPackUninstalled, map[string]any{"pack": name}); err != nil {
		return UninstallOutput{}, err
	}
	return UninstallOutput{Uninstalled: name}, nil
}
func (s *Writes) AddSource(ctx context.Context, in AddSourceInput) (AddSourceOutput, error) {
	if err := ctx.Err(); err != nil {
		return AddSourceOutput{}, err
	}
	if s.writer == nil {
		return AddSourceOutput{}, errors.New("marketplace not available on this daemon")
	}
	if in.URL == "" {
		return AddSourceOutput{}, errors.New("args.url required")
	}
	src, err := s.writer.AddSource(in.Name, in.URL, in.PubKey)
	if err != nil {
		return AddSourceOutput{}, err
	}
	if err := s.published(event.KindMarketSourceAdded, map[string]any{"source": src.Name, "url": src.URL}); err != nil {
		return AddSourceOutput{}, err
	}
	return src, nil
}
func (s *Writes) RemoveSource(ctx context.Context, in RemoveSourceInput) (RemoveSourceOutput, error) {
	if err := ctx.Err(); err != nil {
		return RemoveSourceOutput{}, err
	}
	if s.writer == nil {
		return RemoveSourceOutput{}, errors.New("marketplace not available on this daemon")
	}
	if in.Name == "" {
		return RemoveSourceOutput{}, errors.New("args.name required")
	}
	found, err := s.writer.RemoveSource(in.Name)
	if err != nil {
		return RemoveSourceOutput{}, err
	}
	if err := s.published(event.KindMarketSourceRemoved, map[string]any{"source": in.Name}); err != nil {
		return RemoveSourceOutput{}, err
	}
	return RemoveSourceOutput{Removed: found, Name: in.Name}, nil
}
func (s *Writes) Sync(ctx context.Context, in SyncInput) (SyncOutput, error) {
	if err := ctx.Err(); err != nil {
		return SyncOutput{}, err
	}
	if s.writer == nil {
		return SyncOutput{}, errors.New("marketplace not available on this daemon")
	}
	results, err := s.writer.Sync(ctx, in.Name)
	if err != nil && len(results) == 0 {
		return SyncOutput{}, err
	}
	rows := make([]core.SyncResult, len(results))
	copy(rows, results)
	total := 0
	for _, row := range rows {
		total += row.Packs
	}
	if pubErr := s.published(event.KindMarketSynced, map[string]any{"sources": len(rows), "packs": total}); pubErr != nil {
		return SyncOutput{}, pubErr
	}
	out := SyncOutput{Results: rows, Synced: len(rows), Packs: total}
	if err != nil {
		message := err.Error()
		out.PartialError = &message
	}
	return out, nil
}
