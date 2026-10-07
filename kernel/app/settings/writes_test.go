// SPDX-License-Identifier: MIT
package settings

import (
	"context"
	"encoding/json"
	"errors"
	core "github.com/agezt/agezt/kernel/settings"
	"reflect"
	"strings"
	"testing"
)

type settingsWriteProbe struct {
	field                                                   core.Field
	found, pinned, existed                                  bool
	loadErr, saveErr, reloadErr, registerErr, unregisterErr error
	calls                                                   []string
	section                                                 Section
	id                                                      string
	force                                                   bool
}

func (p *settingsWriteProbe) FieldByEnv(name string) (core.Field, bool) {
	p.calls = append(p.calls, "field:"+name)
	return p.field, p.found
}
func (p *settingsWriteProbe) ConfigStore() ValueStore { p.calls = append(p.calls, "config"); return p }
func (p *settingsWriteProbe) VaultStore() ValueStore  { p.calls = append(p.calls, "vault"); return p }
func (p *settingsWriteProbe) Load() error             { p.calls = append(p.calls, "load"); return p.loadErr }
func (p *settingsWriteProbe) Set(name, value string) {
	p.calls = append(p.calls, "set:"+name+"="+value)
}
func (p *settingsWriteProbe) Remove(name string) { p.calls = append(p.calls, "remove:"+name) }
func (p *settingsWriteProbe) Save() error        { p.calls = append(p.calls, "save"); return p.saveErr }
func (p *settingsWriteProbe) EnvPinned(name string) bool {
	p.calls = append(p.calls, "pin:"+name)
	return p.pinned
}
func (p *settingsWriteProbe) SetLiveEnv(name, value string) {
	p.calls = append(p.calls, "env:"+name+"="+value)
}
func (p *settingsWriteProbe) Reload() error { p.calls = append(p.calls, "reload"); return p.reloadErr }
func (p *settingsWriteProbe) Register(sec Section) error {
	p.calls = append(p.calls, "register")
	p.section = sec
	return p.registerErr
}
func (p *settingsWriteProbe) Unregister(id string, force bool) (bool, error) {
	p.calls = append(p.calls, "unregister")
	p.id = id
	p.force = force
	return p.existed, p.unregisterErr
}
func TestSettingsWritesSetValidationBeforeEffectsAndExactCauses(t *testing.T) {
	marker := errors.New("owned cause")
	for _, tc := range []struct {
		name       string
		field      core.Field
		found      bool
		value      string
		load, save error
		want       string
		calls      []string
	}{
		{"unknown", core.Field{}, false, "v", nil, nil, "unknown setting owned", []string{"field:owned"}},
		{"readonly", core.Field{ReadOnly: true}, true, "v", nil, nil, "owned is read-only and cannot be changed from the Config Center", []string{"field:owned"}},
		{"invalid", core.Field{Type: core.TypeNumber, Label: "Count"}, true, "no", nil, nil, "Count must be a whole number", []string{"field:owned"}},
		{"locked", core.Field{Locked: true}, true, "  ", nil, nil, "owned is locked and cannot be cleared", []string{"field:owned"}},
		{"config-load", core.Field{}, true, "v", marker, nil, "load config: owned cause", []string{"field:owned", "config", "load"}},
		{"vault-load", core.Field{Secret: true}, true, "v", marker, nil, "load vault: owned cause", []string{"field:owned", "vault", "load"}},
		{"config-save", core.Field{}, true, " raw ", nil, marker, "save config: owned cause", []string{"field:owned", "config", "load", "set:owned=raw", "save"}},
		{"vault-save", core.Field{Secret: true}, true, "  ", nil, marker, "save vault: owned cause", []string{"field:owned", "vault", "load", "remove:owned", "save"}},
	} {
		t.Run(tc.name, func(t *testing.T) {
			p := &settingsWriteProbe{field: tc.field, found: tc.found, loadErr: tc.load, saveErr: tc.save}
			out, err := NewWrites(p).Set(context.Background(), SetInput{Name: " owned ", Value: tc.value})
			if err == nil || err.Error() != tc.want || !reflect.DeepEqual(out, SetOutput{}) || !reflect.DeepEqual(p.calls, tc.calls) {
				t.Fatal(out, err, p.calls)
			}
			if (tc.load != nil || tc.save != nil) && !errors.Is(err, marker) {
				t.Fatal("cause lost", err)
			}
		})
	}
}
func TestSettingsWritesSetPinnedLiveReloadClearOrderAndLegacyCanceled(t *testing.T) {
	for _, tc := range []struct {
		name    string
		field   core.Field
		pinned  bool
		value   string
		reload  error
		applied string
		tail    []string
	}{
		{"restart", core.Field{Apply: core.ApplyRestart}, false, " raw ", nil, "restart", nil},
		{"pinned-secret", core.Field{Apply: core.ApplyLive, Secret: true}, true, " raw ", nil, "restart", nil},
		{"secret-live", core.Field{Apply: core.ApplyLive, Secret: true}, false, " raw ", nil, "live", []string{"env:owned=raw"}},
		{"lazy-live", core.Field{Apply: core.ApplyLive}, false, " raw ", nil, "live", []string{"env:owned=raw"}},
		{"lazy-clear", core.Field{Apply: core.ApplyLive}, false, "  ", nil, "live", []string{"env:owned="}},
		{"model", core.Field{Apply: core.ApplyLive}, false, " raw ", nil, "live", []string{"env:AGEZT_MODEL=raw", "reload"}},
		{"provider-error", core.Field{Apply: core.ApplyLive}, false, " raw ", errors.New("reload cause"), "restart", []string{"env:AGEZT_PROVIDER=raw", "reload"}},
		{"provider-empty-error", core.Field{Apply: core.ApplyLive}, false, " raw ", errors.New(""), "restart", []string{"env:AGEZT_PROVIDER=raw", "reload"}},
	} {
		t.Run(tc.name, func(t *testing.T) {
			name := "owned"
			if tc.name == "model" {
				name = "AGEZT_MODEL"
			}
			if strings.HasPrefix(tc.name, "provider-") {
				name = "AGEZT_PROVIDER"
			}
			p := &settingsWriteProbe{field: tc.field, found: true, pinned: tc.pinned, reloadErr: tc.reload}
			ctx, cancel := context.WithCancel(context.Background())
			cancel()
			out, err := NewWrites(p).Set(ctx, SetInput{Name: " " + name + " ", Value: tc.value})
			if err != nil || out.Env != name || out.Saved != true || out.Applied != tc.applied {
				t.Fatal(out, err)
			}
			want := []string{"field:" + name, "config", "load", "set:" + name + "=raw", "save", "pin:" + name}
			if tc.field.Secret {
				want[1] = "vault"
			}
			if strings.TrimSpace(tc.value) == "" {
				want[3] = "remove:" + name
			}
			want = append(want, tc.tail...)
			if !reflect.DeepEqual(p.calls, want) {
				t.Fatal(p.calls, want)
			}
			expectedFields := 3
			if tc.pinned {
				expectedFields++
				if out.EnvPinned != true {
					t.Fatal(out)
				}
			}
			if tc.reload != nil {
				expectedFields++
				value := out.ReloadError
				ok := value != nil
				if !ok || *value != tc.reload.Error() {
					t.Fatal(out)
				}
			}
			if len(settingsWriterJSON(t, out)) != expectedFields {
				t.Fatal(out)
			}
		})
	}
}
func TestSettingsWritesRegistryRawSectionTrimForcePresenceAndCause(t *testing.T) {
	marker := errors.New("registry cause")
	sec := Section{ID: " raw id ", Name: " raw name ", Locked: true, Fields: []core.Field{{Env: " raw env ", Secret: true, Apply: core.ApplyLive}}}
	p := &settingsWriteProbe{}
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	out, err := NewWrites(p).Register(ctx, RegisterInput{Section: sec})
	if err != nil || len(settingsWriterJSON(t, out)) != 3 || out.ID != sec.ID || out.Registered != true || out.Applied != "restart" || !reflect.DeepEqual(p.section, sec) || !reflect.DeepEqual(p.calls, []string{"register"}) {
		t.Fatal(out, err, p)
	}
	p.registerErr = marker
	if out, err := NewWrites(p).Register(ctx, RegisterInput{Section: sec}); !reflect.DeepEqual(out, RegisterOutput{}) || !errors.Is(err, marker) {
		t.Fatal(out, err)
	}
	for _, existed := range []bool{false, true} {
		for _, force := range []bool{false, true} {
			p := &settingsWriteProbe{existed: existed}
			out, err := NewWrites(p).Unregister(ctx, UnregisterInput{ID: " raw-id ", Force: force})
			if err != nil || len(settingsWriterJSON(t, out)) != 2 || out.ID != "raw-id" || out.Removed != existed || p.id != "raw-id" || p.force != force || !reflect.DeepEqual(p.calls, []string{"unregister"}) {
				t.Fatal(out, err, p)
			}
		}
	}
	p.unregisterErr = marker
	if out, err := NewWrites(p).Unregister(ctx, UnregisterInput{ID: " id "}); !reflect.DeepEqual(out, UnregisterOutput{}) || !errors.Is(err, marker) {
		t.Fatal(out, err)
	}
}

func settingsWriterJSON(t *testing.T, value any) map[string]any {
	t.Helper()
	raw, err := json.Marshal(value)
	if err != nil {
		t.Fatal(err)
	}
	var out map[string]any
	if err := json.Unmarshal(raw, &out); err != nil {
		t.Fatal(err)
	}
	return out
}
