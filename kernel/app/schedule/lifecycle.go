// SPDX-License-Identifier: MIT

package schedule

import (
	"context"
	"github.com/agezt/agezt/kernel/cadence"
)

type LifecycleStore interface {
	Get(string) (cadence.Entry, bool)
	Remove(string) (bool, error)
	RunNow(string) (bool, error)
	SetEnabled(string, bool) (bool, error)
}
type Lifecycle struct {
	store    LifecycleStore
	validate func(cadence.Entry) error
	publish  func(string, bool, string, cadence.Entry)
}

func NewLifecycle(store LifecycleStore, validate func(cadence.Entry) error, publish func(string, bool, string, cadence.Entry)) *Lifecycle {
	return &Lifecycle{store: store, validate: validate, publish: publish}
}

type IDInput struct {
	ID string `json:"id"`
}
type RemoveOutput struct {
	Removed bool `json:"removed"`
}
type RunOutput struct {
	Triggered bool `json:"triggered"`
}
type EnableInput struct {
	ID      string `json:"id"`
	Enabled bool   `json:"enabled"`
}
type EnableOutput struct {
	Updated bool   `json:"updated"`
	Enabled bool   `json:"enabled"`
	ID      string `json:"id"`
	Action  string `json:"action"`
}

func (s *Lifecycle) Remove(_ context.Context, in IDInput) (RemoveOutput, error) {
	removed, err := s.store.Remove(in.ID)
	if err != nil {
		return RemoveOutput{}, err
	}
	return RemoveOutput{Removed: removed}, nil
}
func (s *Lifecycle) Run(_ context.Context, in IDInput) (RunOutput, error) {
	if current, found := s.store.Get(in.ID); found && s.validate != nil {
		if err := s.validate(current); err != nil {
			return RunOutput{}, err
		}
	}
	triggered, err := s.store.RunNow(in.ID)
	if err != nil {
		return RunOutput{}, err
	}
	return RunOutput{Triggered: triggered}, nil
}
func (s *Lifecycle) Enable(_ context.Context, in EnableInput) (EnableOutput, error) {
	current, found := s.store.Get(in.ID)
	if in.Enabled && found && s.validate != nil {
		if err := s.validate(current); err != nil {
			return EnableOutput{}, err
		}
	}
	updated, err := s.store.SetEnabled(in.ID, in.Enabled)
	if err != nil {
		return EnableOutput{}, err
	}
	action := "paused"
	if in.Enabled {
		action = "resumed"
	}
	if updated && s.publish != nil {
		s.publish(in.ID, in.Enabled, action, current)
	}
	return EnableOutput{Updated: updated, Enabled: in.Enabled, ID: in.ID, Action: action}, nil
}
