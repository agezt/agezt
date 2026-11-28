// SPDX-License-Identifier: MIT

package edict

import (
	"context"
	"errors"
	"fmt"
	"slices"
	"strings"

	"github.com/agezt/agezt/kernel/app"
	"github.com/agezt/agezt/kernel/contract/opapi"
	core "github.com/agezt/agezt/kernel/edict"
	"github.com/agezt/agezt/kernel/event"
)

// WriteEngine is the routed kernel's policy engine as the writes change it.
type WriteEngine interface {
	HardDenyRules() []core.HardDenyRule
	AskPolicy() core.AskPolicy
	Level(core.Capability) (core.TrustLevel, bool)
	SetLevel(core.Capability, core.TrustLevel)
	SetAskPolicy(core.AskPolicy)
	AddHardDeny(core.HardDenyRule) (core.HardDenyRule, error)
	RemoveHardDeny(string) (bool, error)
}

// Publish journals a policy change on the routed kernel; its failure does not
// undo the change.
type Publish func(event.Spec)

type DenyAddOutput struct {
	Name      string   `json:"name"`
	Substring string   `json:"substring"`
	AppliesTo []string `json:"applies_to"`
	Count     int      `json:"count"`
}

type DenyRemoveOutput struct {
	Removed bool `json:"removed"`
	Count   int  `json:"count"`
}

type SetLevelOutput struct {
	Capability string `json:"capability"`
	From       string `json:"from"`
	To         string `json:"to"`
}

type SetModeOutput struct {
	From string `json:"from"`
	To   string `json:"to"`
}

// requiredText rejects a blank value but returns it untrimmed.
func requiredText(text, key string) (string, error) {
	if strings.TrimSpace(text) == "" {
		return "", fmt.Errorf("args.%s required", key)
	}
	return text, nil
}

// Writes changes the routed kernel's policy and journals each change as
// policy.changed from the operator.
type Writes struct {
	engine  WriteEngine
	publish Publish
}

func NewWrites(engine WriteEngine, publish Publish) *Writes {
	return &Writes{engine: engine, publish: publish}
}

func (w *Writes) changed(payload map[string]any) {
	w.publish(event.Spec{Subject: "kernel.policy", Kind: event.KindPolicyChanged, Actor: "operator", Payload: payload})
}

// DenyAdd adds exactly one runtime hard-deny rule, written in the boot-time
// deny syntax.
func (w *Writes) DenyAdd(_ context.Context, in opapi.EdictDenyAddRequest) (DenyAddOutput, error) {
	parsed, err := core.ParseDenyRules(in.Rule)
	if err != nil {
		return DenyAddOutput{}, err
	}
	if len(parsed) != 1 {
		return DenyAddOutput{}, errors.New("args.rule must specify exactly one deny rule (no ';' separators)")
	}
	added, err := w.engine.AddHardDeny(parsed[0])
	if err != nil {
		return DenyAddOutput{}, err
	}
	out := DenyAddOutput{Name: added.Name, Substring: added.Substring, AppliesTo: make([]string, 0, len(added.AppliesTo)), Count: len(w.engine.HardDenyRules())}
	for _, c := range added.AppliesTo {
		out.AppliesTo = append(out.AppliesTo, string(c))
	}
	w.changed(map[string]any{"action": "deny.add", "name": out.Name, "substring": out.Substring, "applies_to": out.AppliesTo, "count": out.Count})
	return out, nil
}

// DenyRemove removes one runtime rule; the boot-time floor refuses removal.
// Only an actual removal is journaled.
func (w *Writes) DenyRemove(_ context.Context, in opapi.EdictDenyRemoveRequest) (DenyRemoveOutput, error) {
	name, err := requiredText(in.Name, "name")
	if err != nil {
		return DenyRemoveOutput{}, err
	}
	removed, err := w.engine.RemoveHardDeny(name)
	if err != nil {
		return DenyRemoveOutput{}, err
	}
	out := DenyRemoveOutput{Removed: removed, Count: len(w.engine.HardDenyRules())}
	if removed {
		w.changed(map[string]any{"action": "deny.rm", "name": name, "count": out.Count})
	}
	return out, nil
}

// SetLevel sets one governed capability's trust level.
func (w *Writes) SetLevel(_ context.Context, in opapi.EdictSetLevelRequest) (SetLevelOutput, error) {
	name, err := requiredText(in.Capability, "capability")
	if err != nil {
		return SetLevelOutput{}, err
	}
	capability := core.Capability(name)
	if !slices.Contains(core.AllCapabilities(), capability) {
		return SetLevelOutput{}, errors.New("unknown capability " + name + " (see `edict show` for the governed set)")
	}
	level, err := core.ParseTrustLevel(in.Level)
	if err != nil {
		return SetLevelOutput{}, err
	}
	out := SetLevelOutput{Capability: name, From: "unset", To: level.String()}
	if prev, ok := w.engine.Level(capability); ok {
		out.From = prev.String()
	}
	w.engine.SetLevel(capability, level)
	w.changed(map[string]any{"action": "level.set", "capability": out.Capability, "from": out.From, "to": out.To})
	return out, nil
}

// SetMode sets the ask policy.
func (w *Writes) SetMode(_ context.Context, in opapi.EdictSetModeRequest) (SetModeOutput, error) {
	mode, err := core.ParseAskPolicy(in.Mode)
	if err != nil {
		return SetModeOutput{}, err
	}
	out := SetModeOutput{From: w.engine.AskPolicy().String(), To: mode.String()}
	w.engine.SetAskPolicy(mode)
	w.changed(map[string]any{"action": "mode.set", "from": out.From, "to": out.To})
	return out, nil
}

// WriteOperations declares the four audited policy writes. Shared dispatch
// journals the operation before the engine changes, and each routes to the
// caller's tenant kernel.
func WriteOperations(provider func(context.Context) *Writes) ([]app.Operation, error) {
	if provider == nil {
		return nil, errors.New("edict writes provider required")
	}
	var ops []app.Operation
	for _, b := range []func() error{
		func() error {
			return bind(&ops, opapi.Spec{Name: opapi.EdictDenyAdd}, func(ctx context.Context, in opapi.EdictDenyAddRequest) (DenyAddOutput, error) {
				return provider(ctx).DenyAdd(ctx, in)
			})
		},
		func() error {
			return bind(&ops, opapi.Spec{Name: opapi.EdictDenyRemove}, func(ctx context.Context, in opapi.EdictDenyRemoveRequest) (DenyRemoveOutput, error) {
				return provider(ctx).DenyRemove(ctx, in)
			})
		},
		func() error {
			return bind(&ops, opapi.Spec{Name: opapi.EdictSetLevel}, func(ctx context.Context, in opapi.EdictSetLevelRequest) (SetLevelOutput, error) {
				return provider(ctx).SetLevel(ctx, in)
			})
		},
		func() error {
			return bind(&ops, opapi.Spec{Name: opapi.EdictSetMode}, func(ctx context.Context, in opapi.EdictSetModeRequest) (SetModeOutput, error) {
				return provider(ctx).SetMode(ctx, in)
			})
		},
	} {
		if err := b(); err != nil {
			return nil, err
		}
	}
	return ops, nil
}
