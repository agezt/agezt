// SPDX-License-Identifier: MIT
package settings

import (
	"context"
	"fmt"
	core "github.com/agezt/agezt/kernel/settings"
	"strings"
)

type ValueStore interface {
	Load() error
	Set(string, string)
	Remove(string)
	Save() error
}
type Writer interface {
	FieldByEnv(string) (core.Field, bool)
	ConfigStore() ValueStore
	VaultStore() ValueStore
	EnvPinned(string) bool
	SetLiveEnv(string, string)
	Reload() error
	Register(Section) error
	Unregister(string, bool) (bool, error)
}
type Writes struct{ writer Writer }

func NewWrites(writer Writer) *Writes { return &Writes{writer: writer} }

type SetInput struct{ Name, Value string }
type RegisterInput struct{ Section Section }
type UnregisterInput struct {
	ID    string
	Force bool
}

func (s *Writes) Set(_ context.Context, in SetInput) (SetOutput, error) {
	name := strings.TrimSpace(in.Name)
	value := in.Value
	field, ok := s.writer.FieldByEnv(name)
	if !ok {
		return SetOutput{}, fmt.Errorf("unknown setting %s", name)
	}
	if field.ReadOnly {
		return SetOutput{}, fmt.Errorf("%s is read-only and cannot be changed from the Config Center", name)
	}
	if err := core.Validate(field, value); err != nil {
		return SetOutput{}, err
	}
	value = strings.TrimSpace(value)
	if field.Locked && value == "" {
		return SetOutput{}, fmt.Errorf("%s is locked and cannot be cleared", name)
	}
	var store ValueStore
	kind := "config"
	if field.Secret {
		store = s.writer.VaultStore()
		kind = "vault"
	} else {
		store = s.writer.ConfigStore()
	}
	if err := store.Load(); err != nil {
		return SetOutput{}, fmt.Errorf("load %s: %w", kind, err)
	}
	if value == "" {
		store.Remove(name)
	} else {
		store.Set(name, value)
	}
	if err := store.Save(); err != nil {
		return SetOutput{}, fmt.Errorf("save %s: %w", kind, err)
	}
	result := SetOutput{Env: name, Saved: true}
	if s.writer.EnvPinned(name) {
		result.Applied = "restart"
		result.EnvPinned = true
		return result, nil
	}
	switch {
	case field.Apply == core.ApplyLive && field.Secret:
		s.writer.SetLiveEnv(name, value)
		result.Applied = "live"
	case field.Apply == core.ApplyLive && !configFieldNeedsKernelReload(name):
		s.writer.SetLiveEnv(name, value)
		result.Applied = "live"
	case field.Apply == core.ApplyLive:
		s.writer.SetLiveEnv(name, value)
		if err := s.writer.Reload(); err != nil {
			result.Applied = "restart"
			message := err.Error()
			result.ReloadError = &message
		} else {
			result.Applied = "live"
		}
	default:
		result.Applied = "restart"
	}
	return result, nil
}
func (s *Writes) Register(_ context.Context, in RegisterInput) (RegisterOutput, error) {
	if err := s.writer.Register(in.Section); err != nil {
		return RegisterOutput{}, err
	}
	return RegisterOutput{ID: in.Section.ID, Registered: true, Applied: "restart"}, nil
}
func (s *Writes) Unregister(_ context.Context, in UnregisterInput) (UnregisterOutput, error) {
	id := strings.TrimSpace(in.ID)
	existed, err := s.writer.Unregister(id, in.Force)
	if err != nil {
		return UnregisterOutput{}, err
	}
	return UnregisterOutput{ID: id, Removed: existed}, nil
}
func configFieldNeedsKernelReload(name string) bool {
	switch name {
	case "AGEZT_PROVIDER", "AGEZT_MODEL":
		return true
	default:
		return false
	}
}
