// SPDX-License-Identifier: MIT
package tools

import (
	"context"
	"encoding/json"
	"errors"
	"github.com/agezt/agezt/kernel/event"
	"github.com/agezt/agezt/kernel/toolbox"
)

type ToolboxInstaller interface {
	Install(context.Context, string) toolbox.InstallResult
}
type ToolboxInstall struct {
	installer ToolboxInstaller
	publish   func(event.Kind, map[string]any) error
}

func NewToolboxInstall(installer ToolboxInstaller, publish func(event.Kind, map[string]any) error) *ToolboxInstall {
	return &ToolboxInstall{installer: installer, publish: publish}
}

type ToolboxInstallInput struct{ Names []string }
type ToolboxInstallOutput struct {
	Installed []string `json:"installed"`
	Failed    []string `json:"failed"`
	Skipped   []string `json:"skipped"`
}

func (s *ToolboxInstall) publication(kind event.Kind, payload map[string]any) error {
	if s.publish == nil {
		return nil
	}
	return s.publish(kind, payload)
}

// Install preserves ordering and the requested-name summary axis. Publication or
// emission errors stop later installs; native compatibility callbacks may remain
// best effort until their typed transport binding.
func (s *ToolboxInstall) Install(ctx context.Context, in ToolboxInstallInput, emit func(event.Event) error) (ToolboxInstallOutput, error) {
	if len(in.Names) == 0 {
		return ToolboxInstallOutput{}, errors.New("args.names (non-empty list) required")
	}
	if err := s.publication(event.KindToolboxInstallRequested, map[string]any{"tools": in.Names}); err != nil {
		return ToolboxInstallOutput{}, err
	}
	installed := make([]string, 0, len(in.Names))
	failed := make([]string, 0)
	skipped := make([]string, 0)
	for _, name := range in.Names {
		if err := ctx.Err(); err != nil {
			return ToolboxInstallOutput{}, err
		}
		res := s.installer.Install(ctx, name)
		switch {
		case res.OK:
			installed = append(installed, name)
		case res.Skipped:
			skipped = append(skipped, name)
		default:
			failed = append(failed, name)
		}
		if err := s.publication(event.KindToolboxInstalled, map[string]any{"tool": res.Tool, "ok": res.OK, "skipped": res.Skipped, "manager": res.Manager, "command": res.Command, "version": res.Version, "error": res.Error}); err != nil {
			return ToolboxInstallOutput{}, err
		}
		payload, _ := json.Marshal(res)
		if emit != nil {
			if err := emit(event.Event{Kind: event.KindToolboxProgress, Subject: "toolbox.install", Actor: "toolbox", Payload: payload}); err != nil {
				return ToolboxInstallOutput{}, err
			}
		}
	}
	return ToolboxInstallOutput{Installed: installed, Failed: failed, Skipped: skipped}, nil
}
