// SPDX-License-Identifier: MIT
package controlplane

import (
	"context"
	"encoding/json"
	"github.com/agezt/agezt/kernel/app"
	"github.com/agezt/agezt/kernel/contract/opapi"
	"github.com/agezt/agezt/kernel/event"
	"github.com/agezt/agezt/kernel/platform/schema"
	"github.com/agezt/agezt/kernel/runtime"
	"github.com/agezt/agezt/kernel/settings"
	"github.com/agezt/agezt/plugins/providers/mock"
	"reflect"
	"strings"
	"testing"
	"time"
)

type pulseAppPort struct {
	calls      []string
	values     []any
	observerOK bool
}

func (p *pulseAppPort) StatusMap() map[string]any {
	p.calls = append(p.calls, "status")
	return map[string]any{"enabled": false, "opaque": map[string]any{"raw": true}}
}
func (p *pulseAppPort) Pause()  { p.calls = append(p.calls, "pause") }
func (p *pulseAppPort) Resume() { p.calls = append(p.calls, "resume") }
func (p *pulseAppPort) Beat()   { p.calls = append(p.calls, "beat") }
func (p *pulseAppPort) SetCadence(d time.Duration) time.Duration {
	p.calls = append(p.calls, "cadence")
	p.values = append(p.values, d)
	return 5 * time.Second
}
func (p *pulseAppPort) SetDial(v string) string {
	p.calls = append(p.calls, "dial")
	p.values = append(p.values, v)
	return "applied-dial"
}
func (p *pulseAppPort) SetQuietHours(v string) string {
	p.calls = append(p.calls, "quiet")
	p.values = append(p.values, v)
	return "applied-quiet"
}
func (p *pulseAppPort) FlushDigest() int { p.calls = append(p.calls, "flush"); return 0 }
func (p *pulseAppPort) RemoveObserver(v string) int {
	p.calls = append(p.calls, "unwatch")
	p.values = append(p.values, v)
	return 0
}
func (p *pulseAppPort) PendingAsks() []map[string]any { p.calls = append(p.calls, "asks"); return nil }
func (p *pulseAppPort) ResolveAsk(v string, approve bool) (bool, bool) {
	p.calls = append(p.calls, "resolve")
	p.values = append(p.values, v, approve)
	return v != "missing", false
}
func (p *pulseAppPort) AddDiskObserver(path string, pct float64) (string, bool) {
	p.calls = append(p.calls, "watch")
	p.values = append(p.values, path, pct)
	return "disk-result", p.observerOK
}
func (p *pulseAppPort) AddProbeObserver(name string, argv []string) (string, bool) {
	p.calls = append(p.calls, "probe")
	p.values = append(p.values, name, argv)
	return "probe-result", p.observerOK
}

func pulseAppFixture(t *testing.T) (*runtime.Kernel, *Server, *pulseAppPort, *mock.Provider) {
	t.Helper()
	dir := t.TempDir()
	provider := mock.New()
	k, err := runtime.Open(runtime.Config{BaseDir: dir, Provider: provider})
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { k.Close() })
	s := NewServer(k, dir)
	s.token = "primary"
	port := &pulseAppPort{observerOK: true}
	s.SetPulse(port)
	s.SetPulseObservers(port)
	return k, s, port, provider
}
func pulseAppArgs() map[string]any {
	return map[string]any{"issue_key": "owned", "approve": false, "seconds": float64(30), "path": " owned path ", "min_pct": float64(20), "name": "owned", "command": " echo owned ", "dial": "quiet", "hours": "22-7", "unused": true}
}
func TestPulseControlNativeMetadataComesFromThirteenTypedSpecs(t *testing.T) {
	want := map[string]bool{CmdPulseStatus: true, CmdPulseAsks: true, CmdPulseAskResolve: false, CmdPulsePause: false, CmdPulseResume: false, CmdPulseBeat: false, CmdPulseCadence: false, CmdPulseDial: false, CmdPulseQuiet: false, CmdPulseFlush: false, CmdPulseWatch: false, CmdPulseProbe: false, CmdPulseUnwatch: false}
	seen := map[string]bool{}
	for _, operation := range registeredAppOperations() {
		spec := operation.Spec()
		if !strings.HasPrefix(spec.Name, "pulse_") || spec.Name == CmdPulseSubscribe {
			continue
		}
		read, known := want[spec.Name]
		wire, exists := commandRegistry[spec.Name]
		if !known || seen[spec.Name] || !exists || !wire.AppOwned || wire.ReadOnly != read || wire.TenantAllowed || wire.TenantRouted || wire.Streaming != StreamNone || spec.ReadOnly != read || spec.Authz != opapi.PrimaryOnly || spec.Tenancy != opapi.Primary || spec.Stream != opapi.StreamNone || spec.Input == nil || spec.Output == nil || len(spec.InputSchema) == 0 || len(spec.OutputSchema) == 0 || !spec.AllowUnknownInput {
			t.Fatal(spec, wire)
		}
		seen[spec.Name] = true
	}
	if len(seen) != 13 || len(pulseControlOperations) != 13 {
		t.Fatal(seen, len(pulseControlOperations))
	}
}
func TestPulseControlAppRequiresActualAuditBeforeElevenMutations(t *testing.T) {
	for _, cmd := range []string{CmdPulseAskResolve, CmdPulsePause, CmdPulseResume, CmdPulseBeat, CmdPulseCadence, CmdPulseDial, CmdPulseQuiet, CmdPulseFlush, CmdPulseWatch, CmdPulseProbe, CmdPulseUnwatch} {
		t.Run(cmd, func(t *testing.T) {
			k, s, port, provider := pulseAppFixture(t)
			if err := k.Journal().Close(); err != nil {
				t.Fatal(err)
			}
			out := callAppHost(t, s, Request{ID: cmd, Cmd: cmd, Token: "primary", Args: pulseAppArgs()})
			store := settings.NewStore(s.baseDir)
			if err := store.Load(); err != nil {
				t.Fatal(err)
			}
			for _, key := range []string{"AGEZT_PULSE_CADENCE", "AGEZT_PULSE_DIAL", "AGEZT_PULSE_QUIET_HOURS"} {
				if value, ok := store.Get(key); ok {
					t.Fatal(key, value)
				}
			}
			if len(out) != 1 || out[0].Type != RespError || !strings.Contains(out[0].Error, "journal") || len(port.calls) != 0 || len(port.values) != 0 || provider.CallCount() != 0 {
				t.Fatalf("EXPECTED: failed audit blocks controller, observers, settings, provider; ACTUAL: %s %+v calls=%v values=%v", cmd, out, port.calls, port.values)
			}
		})
	}
}
func TestPulseControlAppReadPolicyAndPairedMutationAudit(t *testing.T) {
	k, s, port, provider := pulseAppFixture(t)
	for _, operation := range pulseControlOperations {
		spec := operation.Spec()
		out := callAppHost(t, s, Request{ID: spec.Name, Cmd: spec.Name, Token: "primary", Args: pulseAppArgs()})
		if len(out) != 1 || out[0].Type != RespResult {
			t.Fatal(spec.Name, out)
		}
		raw, err := json.Marshal(out[0].Result)
		if err != nil {
			t.Fatal(err)
		}
		if err := schema.ValidateJSON(spec.OutputSchema, raw); err != nil {
			t.Fatal(spec.Name, err)
		}
	}
	spans := map[string]map[event.Kind]int{}
	correlations := map[string]string{}
	if err := k.Journal().Range(func(e *event.Event) error {
		if !strings.HasPrefix(e.Subject, "op.pulse_") {
			return nil
		}
		if e.Subject == "op.pulse_status" || e.Subject == "op.pulse_asks" {
			t.Fatal("read audited", e)
		}
		if e.CorrelationID == "" {
			t.Fatal("audit identity missing", e)
		}
		if spans[e.Subject] == nil {
			spans[e.Subject] = map[event.Kind]int{}
			correlations[e.Subject] = e.CorrelationID
		}
		if e.CorrelationID != correlations[e.Subject] {
			t.Fatal("split audit identity", e)
		}
		spans[e.Subject][e.Kind]++
		return nil
	}); err != nil {
		t.Fatal(err)
	}
	if len(spans) != 11 || len(port.calls) != 13 || provider.CallCount() != 0 {
		t.Fatal(spans, port.calls, provider.CallCount())
	}
	for name, kinds := range spans {
		if kinds[event.KindOpInvoked] != 1 || kinds[event.KindOpCompleted] != 1 || kinds[event.KindOpFailed] != 0 {
			t.Fatal(name, kinds)
		}
	}
	// Resolve failure keeps the same paired audit lifecycle and no provider work.
	out := callAppHost(t, s, Request{ID: "missing", Cmd: CmdPulseAskResolve, Token: "primary", Args: map[string]any{"issue_key": "missing"}})
	if len(out) != 1 || out[0].Type != RespError || out[0].Error != "no pending ask with that issue_key (already resolved?)" {
		t.Fatal(out)
	}
	failed := 0
	if err := k.Journal().Range(func(e *event.Event) error {
		if e.Subject == "op.pulse_ask_resolve" && e.Kind == event.KindOpFailed {
			failed++
		}
		return nil
	}); err != nil || failed != 1 {
		t.Fatal(err, failed)
	}
}
func TestPulseControlAppAvailabilityIsSelectedAtEachDispatch(t *testing.T) {
	_, s, port, provider := pulseAppFixture(t)
	s.SetPulse(nil)
	s.SetPulseObservers(nil)
	for _, cmd := range []string{CmdPulseAskResolve, CmdPulseCadence, CmdPulseDial, CmdPulseQuiet, CmdPulseUnwatch, CmdPulseWatch, CmdPulseProbe} {
		out := callAppHost(t, s, Request{ID: cmd, Cmd: cmd, Token: "primary", Args: map[string]any{"issue_key": true, "dial": true, "name": true, "path": true, "command": true, "hours": true}})
		want := "pulse is disabled (AGEZT_PULSE=off)"
		if cmd == CmdPulseWatch || cmd == CmdPulseProbe {
			want = "watches are unavailable (pulse is disabled)"
		}
		if len(out) != 1 || out[0].Type != RespError || out[0].Error != want {
			t.Fatal(cmd, out)
		}
	}
	s.SetPulse(port)
	s.SetPulseObservers(port)
	out := callAppHost(t, s, Request{ID: "live", Cmd: CmdPulseBeat, Token: "primary"})
	if len(out) != 1 || !reflect.DeepEqual(out[0].Result, map[string]any{"triggered": true}) || !reflect.DeepEqual(port.calls, []string{"beat"}) || provider.CallCount() != 0 {
		t.Fatal(out, port.calls, provider.CallCount())
	}
}
func TestPulseControlAppCanceledAdmissionHasNoEffects(t *testing.T) {
	_, s, port, provider := pulseAppFixture(t)
	s.operationOnce.Do(func() {
		s.operations, s.operationErr = app.NewDispatcher(registeredAppOperations(), app.Dependencies{Auth: appAuthenticator{s}, Router: appTenantRouter{s}, Audit: appAuditor{}})
	})
	if s.operationErr != nil {
		t.Fatal(s.operationErr)
	}
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	for _, operation := range pulseControlOperations {
		raw, _ := json.Marshal(pulseAppArgs())
		if _, err := s.operations.Dispatch(ctx, opapi.Caller{Credential: "primary"}, operation.Spec().Name, raw, nil); err != context.Canceled {
			t.Fatal(operation.Spec().Name, err)
		}
	}
	if len(port.calls) != 0 || provider.CallCount() != 0 {
		t.Fatal(port.calls, provider.CallCount())
	}
}

func TestPulseControlAppArgumentPresenceAndAppliedPersistence(t *testing.T) {
	_, s, port, _ := pulseAppFixture(t)
	cases := []struct {
		cmd    string
		args   map[string]any
		error  string
		calls  []string
		values []any
		result map[string]any
	}{
		{CmdPulseAskResolve, map[string]any{"issue_key": " raw ", "approve": "TRUE"}, "", []string{"resolve"}, []any{" raw ", false}, map[string]any{"resolved": true, "approved": false, "acted": false}},
		{CmdPulseAskResolve, map[string]any{"issue_key": "owned", "approve": "1"}, "", []string{"resolve"}, []any{"owned", true}, map[string]any{"resolved": true, "approved": true, "acted": false}},
		{CmdPulseAskResolve, map[string]any{"issue_key": "owned", "approve": float64(1)}, "", []string{"resolve"}, []any{"owned", false}, map[string]any{"resolved": true, "approved": false, "acted": false}},
		{CmdPulseAskResolve, map[string]any{"issue_key": nil}, "args.issue_key must be a string", nil, nil, nil},
		{CmdPulseUnwatch, map[string]any{"name": "  "}, "args.name required", nil, nil, nil},
		{CmdPulseCadence, map[string]any{"seconds": "0.1234567899"}, "", []string{"cadence"}, []any{time.Duration(123456789)}, map[string]any{"cadence_ms": float64(5000)}},
		{CmdPulseCadence, map[string]any{"seconds": "wrong"}, "args.seconds must be > 0", nil, nil, nil},
		{CmdPulseCadence, map[string]any{"seconds": true}, "args.seconds must be > 0", nil, nil, nil},
		{CmdPulseDial, map[string]any{"dial": nil}, "args.dial must be a string", nil, nil, nil},
		{CmdPulseDial, nil, "", []string{"dial"}, []any{""}, map[string]any{"dial": "applied-dial"}},
		{CmdPulseQuiet, map[string]any{"hours": " raw "}, "", []string{"quiet"}, []any{" raw "}, map[string]any{"quiet": "applied-quiet"}},
		{CmdPulseQuiet, map[string]any{"hours": true}, "args.hours must be a string", nil, nil, nil},
		{CmdPulseWatch, map[string]any{"path": " raw path ", "min_pct": "17.25"}, "", []string{"watch"}, []any{" raw path ", 17.25}, map[string]any{"added": true, "observer": "disk-result"}},
		{CmdPulseWatch, map[string]any{"path": "owned", "min_pct": float64(100)}, "args.min_pct must be between 0 and 100", nil, nil, nil},
		{CmdPulseWatch, map[string]any{"path": nil, "min_pct": "wrong"}, "args.path must be a string", nil, nil, nil},
		{CmdPulseProbe, map[string]any{"name": " raw ", "command": " echo 'two words' --flag "}, "", []string{"probe"}, []any{" raw ", []string{"echo", "'two", "words'", "--flag"}}, map[string]any{"added": true, "observer": "probe-result"}},
		{CmdPulseProbe, map[string]any{"name": true, "command": true}, "args.name must be a string", nil, nil, nil},
		{CmdPulseProbe, map[string]any{"name": "owned", "command": nil}, "args.command must be a string", nil, nil, nil},
		{CmdPulseProbe, map[string]any{"name": "owned", "command": "  "}, "args.name and args.command required", nil, nil, nil},
	}
	for _, tc := range cases {
		port.calls = nil
		port.values = nil
		out := callAppHost(t, s, Request{ID: tc.cmd, Cmd: tc.cmd, Token: "primary", Args: tc.args})
		if len(out) != 1 || out[0].Error != tc.error || !reflect.DeepEqual(port.calls, tc.calls) || !reflect.DeepEqual(port.values, tc.values) || !reflect.DeepEqual(out[0].Result, tc.result) {
			t.Fatal(tc, out, port.calls, port.values)
		}
	}
	store := settings.NewStore(s.baseDir)
	if err := store.Load(); err != nil {
		t.Fatal(err)
	}
	for key, want := range map[string]string{"AGEZT_PULSE_CADENCE": "5s", "AGEZT_PULSE_DIAL": "applied-dial", "AGEZT_PULSE_QUIET_HOURS": "applied-quiet"} {
		got, ok := store.Get(key)
		if !ok || got != want {
			t.Fatal(key, got, ok)
		}
	}
}
