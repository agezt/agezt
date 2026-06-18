// SPDX-License-Identifier: MIT

// Package config is the in-process `config` agent tool: it lets the agent (and
// the skills it runs) read, write, and register Config Center settings directly,
// without shelling out. It is the tool half of the Config Center's skill-facing
// surface (the other halves are the `agt config` CLI and the /api/config/* HTTP
// routes). All three go through the same kernel/settings Registry + creds vault,
// so behaviour — namespacing, secret handling, live-vs-restart — is identical.
package config

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"strings"

	"github.com/agezt/agezt/kernel/agent"
	"github.com/agezt/agezt/kernel/creds"
	"github.com/agezt/agezt/kernel/roster"
	kernelruntime "github.com/agezt/agezt/kernel/runtime"
	"github.com/agezt/agezt/kernel/settings"
)

// Tool implements agent.Tool. It is constructed in buildTools() before the kernel
// exists; SetKernel binds the kernel afterwards so live-apply fields (provider /
// model) can rebuild the provider in place via Reload().
type Tool struct {
	baseDir string
	kernel  *kernelruntime.Kernel
}

// New returns a config Tool scoped to baseDir.
func New(baseDir string) *Tool { return &Tool{baseDir: baseDir} }

// SetKernel binds the kernel for live reloads (called once the kernel is open).
func (t *Tool) SetKernel(k *kernelruntime.Kernel) { t.kernel = k }

// Definition implements agent.Tool.
func (t *Tool) Definition() agent.ToolDef {
	return agent.ToolDef{
		Name: "config",
		Description: "Read, write, and register configuration in the Config Center. " +
			"Ops: schema (list editable settings), get (read one — secrets report presence only; scope can be effective/global/agent), " +
			"set (write one; empty value clears it; scope=agent writes to the acting agent's private override map, scope=global writes the daemon config; provider/model apply live only for global writes), " +
			"register (add your own schema section — fields must be namespaced AGEZT_* and cannot shadow a built-in), " +
			"unregister (remove a registered section). Use this to let a skill configure itself.",
		Effect: agent.ToolEffect{
			Class: agent.EffectReversible,
			PredictedEffects: []string{
				"read Config Center schema or setting presence for schema/get operations",
				"write, clear, register, or unregister settings for set/register/unregister operations",
			},
			AffectedResources: []string{"Config Center", "AGEZT_* settings", "credentials vault for secret settings"},
			RollbackNotes:     "Most changes can be reverted by setting the previous value, clearing the setting, or unregistering the custom section; provider/model changes may apply live.",
			Confidence:        0.8,
		},
		InputSchema: json.RawMessage(`{
  "type": "object",
  "required": ["op"],
  "properties": {
    "op": {"type": "string", "enum": ["schema", "get", "set", "register", "unregister"], "description": "The operation to perform."},
    "name": {"type": "string", "description": "For get/set: the AGEZT_* env-var name (e.g. AGEZT_X_WEATHER_API_KEY)."},
    "value": {"type": "string", "description": "For set: the value to store (empty string clears the setting)."},
    "scope": {"type": "string", "enum": ["effective", "global", "agent"], "description": "For get/set: read or write the effective value (default), daemon-global config, or the acting agent's private override."},
    "section": {"type": "object", "description": "For register: a schema section {id, name, help?, locked?, fields:[{env, label, type, secret?, required?, help?, options?, read_only?, locked?}]}. Field env names must match AGEZT_[A-Z0-9_]+. read_only fields can't be set; locked fields can't be cleared; a locked section can't be unregistered without force."},
    "id": {"type": "string", "description": "For unregister: the registered section id."},
    "force": {"type": "boolean", "description": "For unregister: remove even a locked (system-approved) section."}
  }
}`),
	}
}

type input struct {
	Op      string          `json:"op"`
	Name    string          `json:"name,omitempty"`
	Value   string          `json:"value,omitempty"`
	Scope   string          `json:"scope,omitempty"`
	Section json.RawMessage `json:"section,omitempty"`
	ID      string          `json:"id,omitempty"`
	Force   bool            `json:"force,omitempty"`
}

// Invoke implements agent.Tool.
func (t *Tool) Invoke(ctx context.Context, raw json.RawMessage) (agent.Result, error) {
	var in input
	if err := json.Unmarshal(raw, &in); err != nil {
		return agent.Result{}, fmt.Errorf("config: parse input: %w", err)
	}
	switch in.Op {
	case "schema":
		return t.doSchema()
	case "get":
		return t.doGet(ctx, in)
	case "set":
		return t.doSet(ctx, in)
	case "register":
		return t.doRegister(in)
	case "unregister":
		return t.doUnregister(in)
	default:
		return errf("unknown op %q (schema|get|set|register|unregister)", in.Op), nil
	}
}

func (t *Tool) registry() *settings.Registry { return settings.NewRegistry(t.baseDir) }

func (t *Tool) doSchema() (agent.Result, error) {
	var b strings.Builder
	for _, sec := range t.registry().Sections() {
		tag := ""
		if sec.Source != "" && sec.Source != settings.SourceBuiltin {
			tag = " (registered)"
		}
		fmt.Fprintf(&b, "[%s] %s%s\n", sec.ID, sec.Name, tag)
		for _, f := range sec.Fields {
			secret := ""
			if f.Secret {
				secret = " (secret)"
			}
			typ := string(f.Type)
			if len(f.Options) > 0 {
				typ += ":" + strings.Join(f.Options, "|")
			}
			fmt.Fprintf(&b, "  %s (%s)%s — %s\n", f.Env, typ, secret, f.Label)
		}
	}
	return agent.Result{Output: strings.TrimRight(b.String(), "\n")}, nil
}

func (t *Tool) doGet(ctx context.Context, in input) (agent.Result, error) {
	name := strings.TrimSpace(in.Name)
	if name == "" {
		return errf("name required"), nil
	}
	scope, err := configScope(in.Scope, "effective")
	if err != nil {
		return errf("%s", err.Error()), nil
	}
	field, ok := t.registry().FieldByEnv(name)
	if !ok {
		return errf("unknown setting %q", name), nil
	}
	agentSlug := agent.AgentFromContext(ctx)
	agentOverrides := kernelruntime.AgentConfigOverrides(ctx)
	if scope == "agent" {
		if agentSlug == "" {
			return errf("scope=agent requires a named agent run"), nil
		}
		if field.Secret {
			return errf("agent-scoped overrides do not support secret fields"), nil
		}
		if value, ok := agentOverrides[name]; ok {
			return agent.Result{Output: fmt.Sprintf("%s=%s (agent override: %s)", name, value, agentSlug)}, nil
		}
		return agent.Result{Output: fmt.Sprintf("%s: not set for agent %s", name, agentSlug)}, nil
	}
	if scope != "global" && !field.Secret {
		if value, ok := agentOverrides[name]; ok {
			return agent.Result{Output: fmt.Sprintf("%s=%s (agent override: %s)", name, value, agentSlug)}, nil
		}
	}
	if field.Secret {
		vault := creds.NewStore(t.baseDir)
		_ = vault.Load()
		if vault.Has(name) {
			return agent.Result{Output: name + ": set (secret, value not shown)"}, nil
		}
		return agent.Result{Output: name + ": not set"}, nil
	}
	val := os.Getenv(name)
	if val == "" {
		store := settings.NewStore(t.baseDir)
		_ = store.Load()
		val, _ = store.Get(name)
	}
	return agent.Result{Output: fmt.Sprintf("%s=%s", name, val)}, nil
}

func (t *Tool) doSet(ctx context.Context, in input) (agent.Result, error) {
	name := strings.TrimSpace(in.Name)
	if name == "" {
		return errf("name required"), nil
	}
	scope, err := configScope(in.Scope, "global")
	if err != nil {
		return errf("%s", err.Error()), nil
	}
	field, ok := t.registry().FieldByEnv(name)
	if !ok {
		return errf("unknown setting %q", name), nil
	}
	if field.ReadOnly {
		return errf("%s is read-only and cannot be changed", name), nil
	}
	value := strings.TrimSpace(in.Value)
	if err := settings.Validate(field, value); err != nil {
		return errf("%s", err.Error()), nil
	}
	if field.Locked && value == "" {
		return errf("%s is locked and cannot be cleared", name), nil
	}
	if scope == "agent" {
		agentSlug := agent.AgentFromContext(ctx)
		if agentSlug == "" {
			return errf("scope=agent requires a named agent run"), nil
		}
		if field.Secret {
			return errf("agent-scoped overrides do not support secret fields"), nil
		}
		if t.kernel == nil {
			return errf("agent-scoped config needs a live kernel"), nil
		}
		if _, found, err := t.kernel.UpdateProfile(agentSlug, func(p *roster.Profile) {
			next := make(map[string]string, len(p.ConfigOverrides)+1)
			for key, existing := range p.ConfigOverrides {
				next[key] = existing
			}
			if value == "" {
				delete(next, name)
			} else {
				next[name] = value
			}
			if len(next) == 0 {
				p.ConfigOverrides = nil
			} else {
				p.ConfigOverrides = next
			}
		}); err != nil {
			return errf("save agent override: %v", err), nil
		} else if !found {
			return errf("unknown agent %q", agentSlug), nil
		}
		if value == "" {
			return agent.Result{Output: fmt.Sprintf("%s cleared from agent override (%s)", name, agentSlug)}, nil
		}
		return agent.Result{Output: fmt.Sprintf("%s saved in agent override (%s)", name, agentSlug)}, nil
	}
	if field.Secret {
		vault := creds.NewStore(t.baseDir)
		if err := vault.Load(); err != nil {
			return errf("load vault: %v", err), nil
		}
		if value == "" {
			vault.Remove(name)
		} else {
			vault.Set(name, value)
		}
		if err := vault.Save(); err != nil {
			return errf("save vault: %v", err), nil
		}
	} else {
		store := settings.NewStore(t.baseDir)
		if err := store.Load(); err != nil {
			return errf("load config: %v", err), nil
		}
		if value == "" {
			store.Remove(name)
		} else {
			store.Set(name, value)
		}
		if err := store.Save(); err != nil {
			return errf("save config: %v", err), nil
		}
	}
	// Live-apply provider/model in place; everything else takes effect on restart.
	if field.Apply == settings.ApplyLive && !field.Secret && t.kernel != nil {
		_ = os.Setenv(name, value)
		if _, _, err := t.kernel.Reload(); err != nil {
			return agent.Result{Output: fmt.Sprintf("%s saved, but live reload failed: %v", name, err), IsError: true}, nil
		}
		return agent.Result{Output: name + " applied live"}, nil
	}
	return agent.Result{Output: name + " saved — restart to apply"}, nil
}

func configScope(raw, def string) (string, error) {
	scope := strings.TrimSpace(strings.ToLower(raw))
	if scope == "" {
		return def, nil
	}
	switch scope {
	case "effective", "global", "agent":
		return scope, nil
	default:
		return "", fmt.Errorf("scope must be effective, global, or agent")
	}
}

func (t *Tool) doRegister(in input) (agent.Result, error) {
	if len(in.Section) == 0 {
		return errf("section required"), nil
	}
	var sec settings.Section
	if err := json.Unmarshal(in.Section, &sec); err != nil {
		return errf("decode section: %v", err), nil
	}
	if err := t.registry().Register(sec); err != nil {
		return errf("%s", err.Error()), nil
	}
	return agent.Result{Output: fmt.Sprintf("registered schema section %q (restart to apply its values)", sec.ID)}, nil
}

func (t *Tool) doUnregister(in input) (agent.Result, error) {
	id := strings.TrimSpace(in.ID)
	if id == "" {
		return errf("id required"), nil
	}
	removed, err := t.registry().Unregister(id, in.Force)
	if err != nil {
		return errf("%s", err.Error()), nil
	}
	if removed {
		return agent.Result{Output: "unregistered " + id}, nil
	}
	return agent.Result{Output: id + " was not registered"}, nil
}

func errf(format string, a ...any) agent.Result {
	return agent.Result{Output: fmt.Sprintf(format, a...), IsError: true}
}
