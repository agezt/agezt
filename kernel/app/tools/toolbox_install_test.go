// SPDX-License-Identifier: MIT
package tools

import (
	"context"
	"encoding/json"
	"errors"
	"github.com/agezt/agezt/kernel/event"
	"github.com/agezt/agezt/kernel/toolbox"
	"reflect"
	"testing"
)

type toolboxInstallProbe struct {
	results  map[string]toolbox.InstallResult
	names    []string
	contexts []context.Context
	trace    *[]string
	after    func(string)
}

func (p *toolboxInstallProbe) Install(ctx context.Context, name string) toolbox.InstallResult {
	p.names = append(p.names, name)
	p.contexts = append(p.contexts, ctx)
	if p.trace != nil {
		*p.trace = append(*p.trace, "install:"+name)
	}
	if p.after != nil {
		p.after(name)
	}
	return p.results[name]
}
func TestToolboxInstallOrderRequestedNamesProgressAndSummary(t *testing.T) {
	trace := []string{}
	p := &toolboxInstallProbe{trace: &trace, results: map[string]toolbox.InstallResult{" requested ": {Tool: "reported", OK: true, Skipped: true, Manager: " raw manager ", Command: " raw command ", Version: " raw version ", OutputTail: " output ö ", Error: " raw error "}, "skip": {Tool: "reported-skip", Skipped: true, Error: "unavailable"}, "fail": {Tool: "reported-fail", Error: "failure"}}}
	var specs []struct {
		kind    event.Kind
		payload map[string]any
	}
	var progress []event.Event
	s := NewToolboxInstall(p, func(kind event.Kind, payload map[string]any) error {
		trace = append(trace, "publish:"+string(kind))
		specs = append(specs, struct {
			kind    event.Kind
			payload map[string]any
		}{kind, payload})
		return nil
	})
	ctx := context.WithValue(context.Background(), struct{ owned bool }{true}, "owned-value")
	out, err := s.Install(ctx, ToolboxInstallInput{Names: []string{" requested ", "skip", "fail", " requested "}}, func(e event.Event) error {
		trace = append(trace, "emit:"+string(e.Kind))
		progress = append(progress, e)
		return nil
	})
	if err != nil || !reflect.DeepEqual(out.Installed, []string{" requested ", " requested "}) || !reflect.DeepEqual(out.Skipped, []string{"skip"}) || !reflect.DeepEqual(out.Failed, []string{"fail"}) || len(specs) != 5 || len(progress) != 4 {
		t.Fatal(out, err, specs, progress)
	}
	want := []string{"publish:toolbox.install.requested"}
	for _, name := range []string{" requested ", "skip", "fail", " requested "} {
		want = append(want, "install:"+name, "publish:toolbox.installed", "emit:toolbox.progress")
	}
	if !reflect.DeepEqual(trace, want) {
		t.Fatal(trace, want)
	}
	if !reflect.DeepEqual(specs[0].payload["tools"], p.names) {
		t.Fatal(specs[0], p.names)
	}
	for i, e := range progress {
		if p.contexts[i] != ctx || e.Kind != event.KindToolboxProgress || e.Subject != "toolbox.install" || e.Actor != "toolbox" || e.Seq != 0 || e.ID != "" || e.CorrelationID != "" || e.TSUnixMS != 0 {
			t.Fatal(i, e, p.contexts[i])
		}
		var res toolbox.InstallResult
		if err := json.Unmarshal(e.Payload, &res); err != nil || res != p.results[p.names[i]] {
			t.Fatal(res, err)
		}
		payload := specs[i+1].payload
		original := p.results[p.names[i]]
		if len(payload) != 7 || payload["tool"] != original.Tool || payload["ok"] != original.OK || payload["skipped"] != original.Skipped || payload["manager"] != original.Manager || payload["command"] != original.Command || payload["version"] != original.Version || payload["error"] != original.Error {
			t.Fatal(payload, original)
		}
	}
}
func TestToolboxInstallEmptyInputAndRequiredEmptySummaryArrays(t *testing.T) {
	p := &toolboxInstallProbe{results: map[string]toolbox.InstallResult{"owned": {Tool: "owned", OK: true}}}
	published := 0
	s := NewToolboxInstall(p, func(event.Kind, map[string]any) error { published++; return nil })
	for _, names := range [][]string{nil, {}} {
		out, err := s.Install(context.Background(), ToolboxInstallInput{Names: names}, nil)
		if err == nil || err.Error() != "args.names (non-empty list) required" || !reflect.DeepEqual(out, ToolboxInstallOutput{}) {
			t.Fatal(out, err)
		}
	}
	if published != 0 || len(p.names) != 0 {
		t.Fatal(published, p.names)
	}
	out, err := s.Install(context.Background(), ToolboxInstallInput{Names: []string{"owned"}}, nil)
	if err != nil || out.Failed == nil || out.Skipped == nil || out.Installed == nil {
		t.Fatal(out, err)
	}
	m := forgeJSON(t, out)
	if len(m) != 3 || !reflect.DeepEqual(m["failed"], []any{}) || !reflect.DeepEqual(m["skipped"], []any{}) || !reflect.DeepEqual(m["installed"], []any{"owned"}) {
		t.Fatal(m)
	}
	for _, result := range []toolbox.InstallResult{{Tool: "owned", Skipped: true}, {Tool: "owned"}} {
		p.results["owned"] = result
		out, err = s.Install(context.Background(), ToolboxInstallInput{Names: []string{"owned"}}, nil)
		if err != nil || out.Installed == nil || out.Failed == nil || out.Skipped == nil {
			t.Fatal(out, err)
		}
	}
}
func TestToolboxInstallErrorsStopLaterEffectsWithOriginalCause(t *testing.T) {
	cause := errors.New("owned boundary failure")
	for _, mode := range []string{"requested", "outcome", "emit"} {
		p := &toolboxInstallProbe{results: map[string]toolbox.InstallResult{"one": {OK: true}, "two": {OK: true}}}
		published, emitted := 0, 0
		s := NewToolboxInstall(p, func(event.Kind, map[string]any) error {
			published++
			if (mode == "requested" && published == 1) || (mode == "outcome" && published == 2) {
				return cause
			}
			return nil
		})
		out, err := s.Install(context.Background(), ToolboxInstallInput{Names: []string{"one", "two"}}, func(event.Event) error {
			emitted++
			if mode == "emit" {
				return cause
			}
			return nil
		})
		if err != cause || !reflect.DeepEqual(out, ToolboxInstallOutput{}) {
			t.Fatal(mode, out, err)
		}
		want := 1
		if mode == "requested" {
			want = 0
		}
		if len(p.names) != want || emitted != map[string]int{"requested": 0, "outcome": 0, "emit": 1}[mode] {
			t.Fatal(mode, p.names, emitted)
		}
	}
}
func TestToolboxInstallCancellationRetainsRequestedAndCompletedAttemptOrder(t *testing.T) {
	for _, mode := range []string{"before", "during"} {
		ctx, cancel := context.WithCancel(context.Background())
		p := &toolboxInstallProbe{results: map[string]toolbox.InstallResult{"one": {OK: true}, "two": {OK: true}}}
		published, emitted := 0, 0
		if mode == "before" {
			cancel()
		} else {
			p.after = func(string) { cancel() }
		}
		s := NewToolboxInstall(p, func(event.Kind, map[string]any) error { published++; return nil })
		out, err := s.Install(ctx, ToolboxInstallInput{Names: []string{"one", "two"}}, func(event.Event) error { emitted++; return nil })
		cancel()
		if err != context.Canceled || !reflect.DeepEqual(out, ToolboxInstallOutput{}) {
			t.Fatal(mode, out, err)
		}
		if mode == "before" {
			if len(p.names) != 0 || published != 1 || emitted != 0 {
				t.Fatal(p.names, published, emitted)
			}
		} else if len(p.names) != 1 || published != 2 || emitted != 1 {
			t.Fatal(p.names, published, emitted)
		}
	}
}
