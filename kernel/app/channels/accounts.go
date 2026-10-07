// SPDX-License-Identifier: MIT
package channels

import (
	"context"
	"fmt"
	"strings"

	"github.com/agezt/agezt/kernel/channel"
	"github.com/agezt/agezt/kernel/settings"
)

type AccountStore interface {
	Load() error
	Set(string, string)
	Remove(string) bool
	Save() error
}
type AccountWriter interface {
	LookupManifest(string) (channel.Manifest, bool)
	Sections() []settings.Section
	SectionEnvs(string) []string
	ConfigStore() AccountStore
	VaultStore() AccountStore
}
type Accounts struct{ writer AccountWriter }

func NewAccounts(writer AccountWriter) *Accounts { return &Accounts{writer: writer} }

type SetAccountInput struct{ Kind, Label, Name, Value string }
type RemoveAccountInput struct{ Kind, Label string }
type SetAccountOutput struct {
	Kind    string `json:"kind"`
	Label   string `json:"label"`
	Env     string `json:"env"`
	Saved   bool   `json:"saved"`
	Applied string `json:"applied"`
}
type RemoveAccountOutput struct {
	Kind    string `json:"kind"`
	Label   string `json:"label"`
	Removed int    `json:"removed"`
	Applied string `json:"applied"`
}

func (s *Accounts) fieldFor(kind, name string) (settings.Field, bool) {
	m, ok := s.writer.LookupManifest(kind)
	if !ok {
		return settings.Field{}, false
	}
	for _, section := range s.writer.Sections() {
		if section.ID != m.ConfigSection {
			continue
		}
		for _, field := range section.Fields {
			if field.Env == name {
				return field, true
			}
		}
	}
	return settings.Field{}, false
}

func (s *Accounts) Set(_ context.Context, in SetAccountInput) (SetAccountOutput, error) {
	kind := strings.TrimSpace(strings.ToLower(in.Kind))
	label := strings.TrimSpace(in.Label)
	name := strings.TrimSpace(in.Name)
	value := in.Value
	if kind == "" || name == "" {
		return SetAccountOutput{}, fmt.Errorf("args.kind and args.name required")
	}
	if label != "" && !settings.ValidAccountLabel(label) {
		return SetAccountOutput{}, fmt.Errorf("label must be a slug: lowercase letters/digits/-/_, max 32")
	}
	field, ok := s.fieldFor(kind, name)
	if !ok {
		return SetAccountOutput{}, fmt.Errorf("%s is not a field of channel %s", name, kind)
	}
	if field.ReadOnly {
		return SetAccountOutput{}, fmt.Errorf("%s is read-only", name)
	}
	if err := settings.Validate(field, value); err != nil {
		return SetAccountOutput{}, err
	}
	value = strings.TrimSpace(value)
	key := settings.SuffixEnv(name, label)
	if field.Secret {
		vault := s.writer.VaultStore()
		if err := vault.Load(); err != nil {
			return SetAccountOutput{}, fmt.Errorf("load vault: %s", err)
		}
		if value == "" {
			vault.Remove(key)
		} else {
			vault.Set(key, value)
		}
		if err := vault.Save(); err != nil {
			return SetAccountOutput{}, fmt.Errorf("save vault: %s", err)
		}
	} else {
		store := s.writer.ConfigStore()
		if err := store.Load(); err != nil {
			return SetAccountOutput{}, fmt.Errorf("load config: %s", err)
		}
		if value == "" {
			store.Remove(key)
		} else {
			store.Set(key, value)
		}
		if err := store.Save(); err != nil {
			return SetAccountOutput{}, fmt.Errorf("save config: %s", err)
		}
	}
	return SetAccountOutput{Kind: kind, Label: label, Env: key, Saved: true, Applied: "restart"}, nil
}

func (s *Accounts) Remove(_ context.Context, in RemoveAccountInput) (RemoveAccountOutput, error) {
	kind := strings.TrimSpace(strings.ToLower(in.Kind))
	label := strings.TrimSpace(in.Label)
	if kind == "" || label == "" {
		return RemoveAccountOutput{}, fmt.Errorf("args.kind and a non-empty args.label required")
	}
	m, ok := s.writer.LookupManifest(kind)
	if !ok {
		return RemoveAccountOutput{}, fmt.Errorf("unknown channel %s", kind)
	}
	baseEnvs := s.writer.SectionEnvs(m.ConfigSection)
	store := s.writer.ConfigStore()
	_ = store.Load()
	vault := s.writer.VaultStore()
	_ = vault.Load()
	removed := 0
	for _, base := range baseEnvs {
		key := settings.SuffixEnv(base, label)
		if store.Remove(key) {
			removed++
		}
		if vault.Remove(key) {
			removed++
		}
	}
	if err := store.Save(); err != nil {
		return RemoveAccountOutput{}, fmt.Errorf("save config: %s", err)
	}
	if err := vault.Save(); err != nil {
		return RemoveAccountOutput{}, fmt.Errorf("save vault: %s", err)
	}
	return RemoveAccountOutput{Kind: kind, Label: label, Removed: removed, Applied: "restart"}, nil
}
