// SPDX-License-Identifier: MIT
package tools

import (
	"context"
	"github.com/agezt/agezt/kernel/toolbox"
	"reflect"
	"sort"
	"testing"
	"time"
)

type toolboxReadProbe struct {
	inventory                  toolbox.Inventory
	outdated                   map[string]bool
	detectCtx, outdatedCtx     context.Context
	detectCalls, outdatedCalls int
}

func (p *toolboxReadProbe) Detect(ctx context.Context) toolbox.Inventory {
	p.detectCtx = ctx
	p.detectCalls++
	return p.inventory
}
func (p *toolboxReadProbe) Outdated(ctx context.Context) map[string]bool {
	p.outdatedCtx = ctx
	p.outdatedCalls++
	return p.outdated
}
func TestToolboxReadsDetectPreservesHostSnapshotAndContext(t *testing.T) {
	inv := toolbox.Inventory{OS: "windows", Managers: []string{"winget", "cargo"}, Tools: []toolbox.ToolStatus{{Name: "owned", Category: "raw category", Description: " raw description ö ", Installed: true, Version: " raw version ", Path: " raw path ", Installable: true, Manager: "winget", Command: " raw command "}, {Name: "missing", Installed: false, Installable: false}}, InstalledCount: 1, MissingCount: 1}
	p := &toolboxReadProbe{inventory: inv}
	s := NewToolboxReads(p)
	ctx, cancel := context.WithTimeout(context.Background(), time.Second)
	defer cancel()
	out, err := s.Detect(ctx, ToolboxDetectInput{})
	if err != nil || !reflect.DeepEqual(out, inv) || p.detectCtx != ctx || p.detectCalls != 1 || p.outdatedCalls != 0 {
		t.Fatal(out, err, p)
	}
	value := forgeJSON(t, out)
	if len(value) != 5 || value["os"] != "windows" || value["installed_count"] != float64(1) || value["missing_count"] != float64(1) {
		t.Fatal(value)
	}
	row := value["tools"].([]any)[1].(map[string]any)
	if len(row) != 5 || row["installed"] != false || row["installable"] != false || row["category"] != "" || row["description"] != "" {
		t.Fatal(row)
	}
	p.inventory = toolbox.Inventory{}
	fresh, err := s.Detect(ctx, ToolboxDetectInput{})
	if err != nil || fresh.OS != "" || fresh.Managers != nil || fresh.Tools != nil || fresh.InstalledCount != 0 || fresh.MissingCount != 0 || p.detectCalls != 2 {
		t.Fatal(fresh, err, p)
	}
	empty := forgeJSON(t, fresh)
	if len(empty) != 5 || empty["managers"] != nil || empty["tools"] != nil || empty["os"] != "" || empty["installed_count"] != float64(0) || empty["missing_count"] != float64(0) {
		t.Fatal(empty)
	}
}
func TestToolboxReadsOutdatedKeepsAllKeysAndFreshEmptyArray(t *testing.T) {
	p := &toolboxReadProbe{outdated: map[string]bool{"zeta": true, " raw name ": false, "ö": true}}
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	s := NewToolboxReads(p)
	out, err := s.Outdated(ctx, ToolboxOutdatedInput{})
	if err != nil || out.Count != 3 || p.detectCalls != 0 || p.outdatedCalls != 1 || p.outdatedCtx != ctx {
		t.Fatal(out, err, p)
	}
	sort.Strings(out.Outdated)
	if !reflect.DeepEqual(out.Outdated, []string{" raw name ", "zeta", "ö"}) {
		t.Fatal(out)
	}
	if len(p.outdated) != 3 {
		t.Fatal(p.outdated)
	}
	for _, next := range []map[string]bool{nil, {}} {
		p.outdated = next
		out, err = s.Outdated(ctx, ToolboxOutdatedInput{})
		if err != nil || out.Outdated == nil || out.Count != 0 || len(out.Outdated) != 0 {
			t.Fatal(out, err)
		}
		m := forgeJSON(t, out)
		if len(m) != 2 || !reflect.DeepEqual(m["outdated"], []any{}) || m["count"] != float64(0) {
			t.Fatal(m)
		}
	}
	p.outdated = map[string]bool{"fresh": false}
	out, err = s.Outdated(ctx, ToolboxOutdatedInput{})
	if err != nil || out.Count != 1 || !reflect.DeepEqual(out.Outdated, []string{"fresh"}) || p.outdatedCalls != 4 {
		t.Fatal(out, err, p)
	}
}
func TestToolboxReadsDirectCanceledContextStillReachesBestEffortHost(t *testing.T) {
	p := &toolboxReadProbe{inventory: toolbox.Inventory{OS: "owned"}, outdated: map[string]bool{"owned": true}}
	s := NewToolboxReads(p)
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	out, err := s.Detect(ctx, ToolboxDetectInput{})
	names, other := s.Outdated(ctx, ToolboxOutdatedInput{})
	if err != nil || other != nil || out.OS != "owned" || names.Count != 1 || p.detectCtx != ctx || p.outdatedCtx != ctx || p.detectCalls != 1 || p.outdatedCalls != 1 {
		t.Fatal(out, names, err, other, p)
	}
}
