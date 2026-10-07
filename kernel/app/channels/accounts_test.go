// SPDX-License-Identifier: MIT
package channels

import (
	"context"
	"errors"
	"reflect"
	"strings"
	"testing"

	"github.com/agezt/agezt/kernel/channel"
	"github.com/agezt/agezt/kernel/settings"
)

type accountProbe struct {
	manifest      channel.Manifest
	found         bool
	sections      []settings.Section
	envs          []string
	config, vault *accountStoreProbe
	calls         []string
	kind, section string
}
type accountStoreProbe struct {
	owner            *accountProbe
	kind             string
	values           map[string]string
	loadErr, saveErr error
	key, value       string
}

func (s *accountStoreProbe) Load() error {
	s.owner.calls = append(s.owner.calls, s.kind+".load")
	return s.loadErr
}
func (s *accountStoreProbe) Set(key, value string) {
	s.owner.calls = append(s.owner.calls, s.kind+".set")
	s.key, s.value = key, value
	s.values[key] = value
}
func (s *accountStoreProbe) Remove(key string) bool {
	s.owner.calls = append(s.owner.calls, s.kind+".remove")
	s.key = key
	_, ok := s.values[key]
	delete(s.values, key)
	return ok
}
func (s *accountStoreProbe) Save() error {
	s.owner.calls = append(s.owner.calls, s.kind+".save")
	return s.saveErr
}
func (p *accountProbe) LookupManifest(kind string) (channel.Manifest, bool) {
	p.calls = append(p.calls, "manifest")
	p.kind = kind
	return p.manifest, p.found
}
func (p *accountProbe) Sections() []settings.Section {
	p.calls = append(p.calls, "sections")
	return p.sections
}
func (p *accountProbe) SectionEnvs(section string) []string {
	p.calls = append(p.calls, "envs")
	p.section = section
	return p.envs
}
func (p *accountProbe) ConfigStore() AccountStore {
	p.calls = append(p.calls, "config.factory")
	return p.config
}
func (p *accountProbe) VaultStore() AccountStore {
	p.calls = append(p.calls, "vault.factory")
	return p.vault
}
func newAccountProbe(secret bool) *accountProbe {
	p := &accountProbe{manifest: channel.Manifest{Kind: "owned-kind", ConfigSection: "owned-section"}, found: true, sections: []settings.Section{{ID: "other-section", Fields: []settings.Field{{Env: "AGEZT_PUBLIC", ReadOnly: true}}}, {ID: "owned-section", Fields: []settings.Field{{Env: "AGEZT_PUBLIC", Label: "Owned", Secret: secret, Type: settings.TypeText}}}}, envs: []string{"AGEZT_PUBLIC", "AGEZT_SECOND"}}
	p.config = &accountStoreProbe{owner: p, kind: "config", values: map[string]string{}}
	p.vault = &accountStoreProbe{owner: p, kind: "vault", values: map[string]string{}}
	return p
}
func accountCalls(t *testing.T, p *accountProbe, want ...string) {
	t.Helper()
	if !reflect.DeepEqual(p.calls, want) {
		t.Fatal(p.calls, want)
	}
}

func TestChannelAccountsSetValidationBeforeEffectsAndSelectedSection(t *testing.T) {
	for _, tc := range []struct {
		in   SetAccountInput
		want string
	}{
		{SetAccountInput{}, "args.kind and args.name required"},
		{SetAccountInput{Kind: "owned", Name: "AGEZT_PUBLIC", Label: "Bad Label"}, "label must be a slug: lowercase letters/digits/-/_, max 32"},
		{SetAccountInput{Kind: "owned", Name: "AGEZT_PUBLIC", Label: strings.Repeat("a", 33)}, "label must be a slug: lowercase letters/digits/-/_, max 32"},
	} {
		p := newAccountProbe(false)
		out, err := NewAccounts(p).Set(context.Background(), tc.in)
		if out != (SetAccountOutput{}) || err == nil || err.Error() != tc.want || len(p.calls) != 0 {
			t.Fatal(out, err, p.calls)
		}
	}
	p := newAccountProbe(false)
	p.found = false
	if _, err := NewAccounts(p).Set(context.Background(), SetAccountInput{Kind: " unknown ", Name: "AGEZT_PUBLIC"}); err == nil || err.Error() != "AGEZT_PUBLIC is not a field of channel unknown" {
		t.Fatal(err)
	}
	accountCalls(t, p, "manifest")
	p = newAccountProbe(false)
	if _, err := NewAccounts(p).Set(context.Background(), SetAccountInput{Kind: "owned", Name: "AGEZT_OTHER"}); err == nil || err.Error() != "AGEZT_OTHER is not a field of channel owned" {
		t.Fatal(err)
	}
	accountCalls(t, p, "manifest", "sections")
	for _, kind := range []string{"readonly", "invalid-number", "invalid-select"} {
		p = newAccountProbe(false)
		field := &p.sections[1].Fields[0]
		field.ReadOnly = kind == "readonly"
		if kind == "invalid-number" {
			field.Type = settings.TypeNumber
		}
		if kind == "invalid-select" {
			field.Type = settings.TypeSelect
			field.Options = []string{"allowed"}
		}
		out, err := NewAccounts(p).Set(context.Background(), SetAccountInput{Kind: "owned", Name: "AGEZT_PUBLIC", Value: "invalid"})
		if out != (SetAccountOutput{}) || err == nil {
			t.Fatal(out, err)
		}
		accountCalls(t, p, "manifest", "sections")
	}
}

func TestChannelAccountsSetSuffixTrimClearRoutingLegacyCancelAndErrorText(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	for _, secret := range []bool{false, true} {
		for _, label := range []string{"", " work ", strings.Repeat("a", 32)} {
			for _, value := range []string{" raw value ", " "} {
				p := newAccountProbe(secret)
				out, err := NewAccounts(p).Set(ctx, SetAccountInput{Kind: " OWNED-KIND ", Name: " AGEZT_PUBLIC ", Label: label, Value: value})
				key := settings.SuffixEnv("AGEZT_PUBLIC", strings.TrimSpace(label))
				if err != nil || !reflect.DeepEqual(out, SetAccountOutput{Kind: "owned-kind", Label: strings.TrimSpace(label), Env: key, Saved: true, Applied: "restart"}) || p.kind != "owned-kind" {
					t.Fatal(out, err, p)
				}
				store := p.config
				name := "config"
				if secret {
					store = p.vault
					name = "vault"
				}
				verb := "set"
				if strings.TrimSpace(value) == "" {
					verb = "remove"
				} else if store.value != "raw value" {
					t.Fatal(store)
				}
				if store.key != key {
					t.Fatal(store.key, key)
				}
				accountCalls(t, p, "manifest", "sections", name+".factory", name+".load", name+"."+verb, name+".save")
			}
		}
	}
	for _, secret := range []bool{false, true} {
		for _, phase := range []string{"load", "save"} {
			p := newAccountProbe(secret)
			sentinel := errors.New("owned cause")
			store := p.config
			name := "config"
			if secret {
				store = p.vault
				name = "vault"
			}
			if phase == "load" {
				store.loadErr = sentinel
			} else {
				store.saveErr = sentinel
			}
			out, err := NewAccounts(p).Set(context.Background(), SetAccountInput{Kind: "owned", Name: "AGEZT_PUBLIC", Label: "work", Value: "raw"})
			if out != (SetAccountOutput{}) || err == nil || err.Error() != phase+" "+name+": owned cause" || errors.Is(err, sentinel) {
				t.Fatal("legacy error text/cause translation changed", out, err)
			}
			if phase == "load" {
				accountCalls(t, p, "manifest", "sections", name+".factory", name+".load")
				if len(store.values) != 0 {
					t.Fatal("load failure mutated")
				}
			} else {
				accountCalls(t, p, "manifest", "sections", name+".factory", name+".load", name+".set", name+".save")
				if store.values["AGEZT_PUBLIC#work"] != "raw" {
					t.Fatal("save failure rolled back")
				}
			}
		}
	}
}

func TestChannelAccountsRemoveRequiredGlobalEnvsBothStoresCountsAndIgnoredLoads(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	for _, in := range []RemoveAccountInput{{}, {Kind: "owned"}, {Label: "work"}} {
		p := newAccountProbe(false)
		out, err := NewAccounts(p).Remove(ctx, in)
		if out != (RemoveAccountOutput{}) || err == nil || err.Error() != "args.kind and a non-empty args.label required" || len(p.calls) != 0 {
			t.Fatal(out, err, p.calls)
		}
	}
	p := newAccountProbe(false)
	p.found = false
	if _, err := NewAccounts(p).Remove(ctx, RemoveAccountInput{Kind: " Unknown ", Label: "work"}); err == nil || err.Error() != "unknown channel unknown" {
		t.Fatal(err)
	}
	accountCalls(t, p, "manifest")
	for _, label := range []string{" work ", " Bad Label "} {
		p = newAccountProbe(false)
		trimmed := strings.TrimSpace(label)
		p.config.values = map[string]string{"AGEZT_PUBLIC#" + trimmed: "public", "AGEZT_SECOND#" + trimmed: "second", "UNRELATED#" + trimmed: "keep"}
		p.vault.values = map[string]string{"AGEZT_PUBLIC#" + trimmed: "secret"}
		p.config.loadErr = errors.New("ignored config load")
		p.vault.loadErr = errors.New("ignored vault load")
		out, err := NewAccounts(p).Remove(ctx, RemoveAccountInput{Kind: " OWNED-KIND ", Label: label})
		if err != nil || !reflect.DeepEqual(out, RemoveAccountOutput{Kind: "owned-kind", Label: trimmed, Removed: 3, Applied: "restart"}) || p.section != "owned-section" || p.kind != "owned-kind" || len(p.config.values) != 1 || p.config.values["UNRELATED#"+trimmed] != "keep" || len(p.vault.values) != 0 {
			t.Fatal(out, err, p)
		}
		accountCalls(t, p, "manifest", "envs", "config.factory", "config.load", "vault.factory", "vault.load", "config.remove", "vault.remove", "config.remove", "vault.remove", "config.save", "vault.save")
	}
	p = newAccountProbe(false)
	p.envs = nil
	out, err := NewAccounts(p).Remove(ctx, RemoveAccountInput{Kind: "owned", Label: "work"})
	if err != nil || out.Removed != 0 {
		t.Fatal(out, err)
	}
	accountCalls(t, p, "manifest", "envs", "config.factory", "config.load", "vault.factory", "vault.load", "config.save", "vault.save")
}

func TestChannelAccountsRemoveSaveOrderAndPartialEffects(t *testing.T) {
	for _, phase := range []string{"config", "vault"} {
		p := newAccountProbe(false)
		sentinel := errors.New("owned failure")
		p.config.values["AGEZT_PUBLIC#work"] = "raw"
		p.vault.values["AGEZT_PUBLIC#work"] = "secret"
		if phase == "config" {
			p.config.saveErr = sentinel
		} else {
			p.vault.saveErr = sentinel
		}
		out, err := NewAccounts(p).Remove(context.Background(), RemoveAccountInput{Kind: "owned", Label: "work"})
		if out != (RemoveAccountOutput{}) || err == nil || err.Error() != "save "+phase+": owned failure" || errors.Is(err, sentinel) || len(p.config.values) != 0 || len(p.vault.values) != 0 {
			t.Fatal(out, err, p)
		}
		want := []string{"manifest", "envs", "config.factory", "config.load", "vault.factory", "vault.load", "config.remove", "vault.remove", "config.remove", "vault.remove", "config.save"}
		if phase == "vault" {
			want = append(want, "vault.save")
		}
		if !reflect.DeepEqual(p.calls, want) {
			t.Fatal(p.calls)
		}
	}
}
