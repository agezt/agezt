// SPDX-License-Identifier: MIT
package market

import (
	"context"
	"errors"
	"github.com/agezt/agezt/kernel/contract/opapi"
	"github.com/agezt/agezt/kernel/event"
	core "github.com/agezt/agezt/kernel/market"
	"reflect"
	"testing"
)

type writeCallerKey struct{}

type writeProbe struct {
	calls                                      []string
	corr, marketplace, name, version, url, key string
	ctx                                        context.Context
	record                                     core.InstalledPack
	source                                     core.Source
	results                                    []core.SyncResult
	found                                      bool
	err                                        error
	progress                                   []core.Event
	trace                                      *[]string
	nilEmit                                    bool
}

func (p *writeProbe) called(name string) {
	p.calls = append(p.calls, name)
	if p.trace != nil {
		*p.trace = append(*p.trace, "writer:"+name)
	}
}
func (p *writeProbe) InstallContext(ctx context.Context, corr, marketplace, name, version string, emit func(core.Event) error) (core.InstalledPack, error) {
	p.called("install")
	p.ctx = ctx
	p.corr = corr
	p.marketplace = marketplace
	p.name = name
	p.version = version
	p.nilEmit = emit == nil
	if emit != nil {
		for _, e := range p.progress {
			if err := emit(e); err != nil {
				return p.record, err
			}
		}
	}
	return p.record, p.err
}
func (p *writeProbe) UninstallContext(ctx context.Context, corr, name string, emit func(core.Event) error) error {
	p.called("uninstall")
	p.ctx = ctx
	p.corr = corr
	p.name = name
	p.nilEmit = emit == nil
	if emit != nil {
		for _, e := range p.progress {
			if err := emit(e); err != nil {
				return err
			}
		}
	}
	return p.err
}
func (p *writeProbe) AddSource(name, url, key string) (core.Source, error) {
	p.called("add")
	p.name = name
	p.url = url
	p.key = key
	return p.source, p.err
}
func (p *writeProbe) RemoveSource(name string) (bool, error) {
	p.called("remove")
	p.name = name
	return p.found, p.err
}
func (p *writeProbe) Sync(ctx context.Context, name string) ([]core.SyncResult, error) {
	p.called("sync")
	p.ctx = ctx
	p.name = name
	return p.results, p.err
}

func TestWritesTypedInputsOrderExactNumbersAndOwnership(t *testing.T) {
	ctx := opapi.WithCorrelation(context.Background(), "owned-corr")
	trace := []string{}
	p := &writeProbe{record: core.InstalledPack{Name: "reported", Version: "2.0.0", Marketplace: "reported-market", InstalledMS: 9007199254740993, SkillIDs: []string{"skill"}, MCPServers: []string{"mcp"}, ToolReqs: []string{"tool"}, Unsigned: true, VetVerdict: "danger"}, source: core.Source{Name: "reported", URL: "reported-url", AddedMS: 9007199254740993}, progress: []core.Event{{Stage: "vet", OK: false}, {Stage: "done", OK: true}}, trace: &trace}
	var kinds []event.Kind
	var payloads []map[string]any
	svc := NewWrites(p, func(kind event.Kind, payload map[string]any) error {
		trace = append(trace, "publish")
		kinds = append(kinds, kind)
		payloads = append(payloads, payload)
		return nil
	})
	emit := func(e core.Event) error { trace = append(trace, "emit:"+e.Stage); return nil }
	out, err := svc.Install(ctx, InstallInput{CorrelationID: "poison", Marketplace: " raw-market ", Name: " owned ", Version: " raw-version "}, emit)
	if err != nil || out.InstalledMS != 9007199254740993 || p.ctx != ctx || p.corr != "owned-corr" || p.name != "owned" || p.marketplace != " raw-market " || p.version != " raw-version " || !reflect.DeepEqual(trace, []string{"writer:install", "emit:vet", "emit:done", "publish"}) || kinds[0] != event.KindMarketPackInstalled || payloads[0]["pack"] != "reported" {
		t.Fatal(out, err, p, trace)
	}
	out.SkillIDs[0] = "changed"
	out.MCPServers[0] = "changed"
	out.ToolReqs[0] = "changed"
	if p.record.SkillIDs[0] != "skill" || p.record.MCPServers[0] != "mcp" || p.record.ToolReqs[0] != "tool" {
		t.Fatal("record alias")
	}
	value, err := svc.Uninstall(ctx, UninstallInput{Name: " owned ", CorrelationID: "poison"}, emit)
	if err != nil || value.Uninstalled != "owned" || p.corr != "owned-corr" || p.ctx != ctx || kinds[1] != event.KindMarketPackUninstalled {
		t.Fatal(value, err, p)
	}
	source, err := svc.AddSource(ctx, AddSourceInput{Name: " raw-name ", URL: " raw-url ", PubKey: " raw-key "})
	if err != nil || source.AddedMS != 9007199254740993 || p.name != " raw-name " || p.url != " raw-url " || p.key != " raw-key " || kinds[2] != event.KindMarketSourceAdded {
		t.Fatal(source, err, p)
	}
	removed, err := svc.RemoveSource(ctx, RemoveSourceInput{Name: " raw-name "})
	if err != nil || removed.Removed || removed.Name != " raw-name " || kinds[3] != event.KindMarketSourceRemoved {
		t.Fatal(removed, err)
	}
}
func TestWritesTypedSyncPartialZeroRowsAndContext(t *testing.T) {
	owned := errors.New("owned sync failure")
	ctx := context.WithValue(context.Background(), writeCallerKey{}, "owned")
	for _, tc := range []struct {
		rows []core.SyncResult
		err  error
	}{{nil, nil}, {[]core.SyncResult{}, nil}, {nil, owned}, {[]core.SyncResult{}, owned}, {[]core.SyncResult{{Source: "second", Packs: 2, FetchedMS: 9007199254740993}, {Source: "first", Packs: 5}}, nil}, {[]core.SyncResult{{Source: "owned", Packs: 3}}, owned}} {
		p := &writeProbe{results: tc.rows, err: tc.err}
		pub := 0
		svc := NewWrites(p, func(kind event.Kind, payload map[string]any) error {
			pub++
			if kind != event.KindMarketSynced {
				t.Fatal(kind)
			}
			return nil
		})
		out, err := svc.Sync(ctx, SyncInput{Name: " raw "})
		if p.ctx != ctx || p.name != " raw " {
			t.Fatal(p)
		}
		if tc.err != nil && len(tc.rows) == 0 {
			if err != owned || pub != 0 {
				t.Fatal(out, err, pub)
			}
			continue
		}
		total := 0
		for _, row := range tc.rows {
			total += row.Packs
		}
		if err != nil || out.Results == nil || out.Synced != len(tc.rows) || out.Packs != total || pub != 1 || tc.err == nil && out.PartialError != nil || tc.err != nil && (out.PartialError == nil || *out.PartialError != owned.Error()) {
			t.Fatal(out, err, pub)
		}
		if len(out.Results) > 0 {
			out.Results[0].Source = "changed"
			if p.results[0].Source == "changed" {
				t.Fatal("sync alias")
			}
		}
	}
}
func TestWritesTypedCancellationAndErrorsStopPublication(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	p := &writeProbe{}
	pub := 0
	svc := NewWrites(p, func(event.Kind, map[string]any) error { pub++; return nil })
	svc.Install(ctx, InstallInput{Name: "owned"}, nil)
	svc.Uninstall(ctx, UninstallInput{Name: "owned"}, nil)
	svc.AddSource(ctx, AddSourceInput{URL: "owned"})
	svc.RemoveSource(ctx, RemoveSourceInput{Name: "owned"})
	svc.Sync(ctx, SyncInput{})
	if len(p.calls) != 0 || pub != 0 {
		t.Fatal(p, pub)
	}
	owned := errors.New("owned writer failure")
	p.err = owned
	for _, method := range []string{"install", "uninstall", "add", "remove", "sync"} {
		var err error
		switch method {
		case "install":
			_, err = svc.Install(context.Background(), InstallInput{Name: "owned"}, nil)
		case "uninstall":
			_, err = svc.Uninstall(context.Background(), UninstallInput{Name: "owned"}, nil)
		case "add":
			_, err = svc.AddSource(context.Background(), AddSourceInput{URL: "owned"})
		case "remove":
			_, err = svc.RemoveSource(context.Background(), RemoveSourceInput{Name: "owned"})
		case "sync":
			_, err = svc.Sync(context.Background(), SyncInput{})
		}
		if err != owned || pub != 0 {
			t.Fatal(method, err, pub)
		}
	}
}
func TestWritesTypedSinkAndPublicationCausesPropagate(t *testing.T) {
	sink := errors.New("owned sink failure")
	p := &writeProbe{progress: []core.Event{{Stage: "first"}, {Stage: "second"}}}
	pub := 0
	svc := NewWrites(p, func(event.Kind, map[string]any) error { pub++; return nil })
	emit := func(core.Event) error { return sink }
	if _, err := svc.Install(context.Background(), InstallInput{Name: "owned"}, emit); err != sink || pub != 0 {
		t.Fatal(err, pub)
	}
	if _, err := svc.Uninstall(context.Background(), UninstallInput{Name: "owned"}, emit); err != sink || pub != 0 {
		t.Fatal(err, pub)
	}
	failure := errors.New("owned publication failure")
	svc = NewWrites(p, func(event.Kind, map[string]any) error { return failure })
	for _, method := range []string{"install", "uninstall", "add", "remove", "sync"} {
		var err error
		switch method {
		case "install":
			_, err = svc.Install(context.Background(), InstallInput{Name: "owned"}, nil)
		case "uninstall":
			_, err = svc.Uninstall(context.Background(), UninstallInput{Name: "owned"}, nil)
		case "add":
			_, err = svc.AddSource(context.Background(), AddSourceInput{URL: "owned"})
		case "remove":
			_, err = svc.RemoveSource(context.Background(), RemoveSourceInput{Name: "owned"})
		case "sync":
			_, err = svc.Sync(context.Background(), SyncInput{})
		}
		if err != failure {
			t.Fatal(method, err)
		}
	}
}
