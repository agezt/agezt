// SPDX-License-Identifier: MIT

package toolreg

// Built-tool set: BuildAll + Set methods + sortedKeys. Carved out
// of toolreg.go during the Day 201 god-file split so the main
// file can stay focused on types + the package-level Register +
// Names + snapshot.
// Public API unchanged.

import (
	"fmt"
	"sort"
	"strings"

	"github.com/agezt/agezt/kernel/agent"
	"github.com/agezt/agezt/kernel/runtime"
)

func BuildAll(d BuildDeps) (*Set, error) {
	s := &Set{tools: map[string]agent.Tool{}, claim: map[string]string{}, dropped: map[string]bool{}}
	for _, sp := range snapshot() {
		if sp.Build == nil {
			continue
		}
		b, err := sp.Build(d)
		if err != nil {
			return nil, fmt.Errorf("tool %s: %w", sp.Name, err)
		}
		if b.Tool == nil && len(b.Extra) == 0 && len(b.Infos) == 0 {
			continue // env-gated off
		}
		if b.Tool != nil {
			if err := s.add(b.Tool.Definition().Name, b.Tool, sp, d); err != nil {
				return nil, err
			}
		}
		for _, name := range sortedKeys(b.Extra) {
			if err := s.add(name, b.Extra[name], sp, d); err != nil {
				return nil, err
			}
		}
		s.pairs = append(s.pairs, pair{spec: sp, built: b})
	}
	return s, nil
}

func (s *Set) add(name string, tl agent.Tool, sp Spec, d BuildDeps) error {
	if prev, dup := s.claim[name]; dup {
		if sp.YieldOnConflict {
			// In-process wins: drop the later (plugin) claimant, keep the
			// earlier registration, and record the drop so PluginManifest /
			// ToolCapabilities stay post-conflict-accurate.
			s.dropped[name] = true
			if d.Stderr != nil {
				fmt.Fprintf(d.Stderr, "WARNING: spec %q tool %q conflicts with existing tool (spec %s) — keeping the in-process version\n", sp.Name, name, prev)
			}
			return nil
		}
		return fmt.Errorf("tool name %q claimed by both spec %s and spec %s", name, prev, sp.Name)
	}
	s.claim[name] = sp.Name
	s.tools[name] = tl
	return nil
}

// Tools returns the merged name → instance map (a copy; instances are shared).
func (s *Set) Tools() map[string]agent.Tool {
	out := make(map[string]agent.Tool, len(s.tools))
	for name, tl := range s.tools {
		out[name] = tl
	}
	return out
}

// ApplyPreOpen runs every spec's PreOpen hook against cfg.
func (s *Set) ApplyPreOpen(cfg *runtime.Config) {
	for _, p := range s.pairs {
		if p.spec.PreOpen != nil {
			p.spec.PreOpen(p.built.Tool, cfg)
		}
	}
}

// Configure runs the post-Open phase: for every Netguard spec it wires
// d.NetguardPublish into each instance implementing NetguardAware (primary
// first, then Extras in sorted-name order), then invokes the spec's Configure
// hook. Pairs run in registration order.
func (s *Set) Configure(d KernelDeps) error {
	for _, p := range s.pairs {
		if p.spec.Netguard && d.NetguardPublish != nil {
			if na, ok := p.built.Tool.(NetguardAware); ok {
				na.SetOnBlock(d.NetguardPublish(p.built.Tool.Definition().Name))
			}
			for _, name := range sortedKeys(p.built.Extra) {
				if na, ok := p.built.Extra[name].(NetguardAware); ok {
					na.SetOnBlock(d.NetguardPublish(name))
				}
			}
		}
		if p.spec.Configure != nil {
			if err := p.spec.Configure(p.built.Tool, d); err != nil {
				return fmt.Errorf("configure %s: %w", p.spec.Name, err)
			}
		}
	}
	return nil
}

// ConfigureLate runs the live-channel/board phase in registration order.
func (s *Set) ConfigureLate(d LateDeps) error {
	for _, p := range s.pairs {
		if p.spec.Late != nil {
			if err := p.spec.Late(p.built.Tool, d); err != nil {
				return fmt.Errorf("late-configure %s: %w", p.spec.Name, err)
			}
		}
	}
	return nil
}

// Descs returns each built spec's banner line, in registration order,
// skipping empties.
func (s *Set) Descs() []string {
	var out []string
	for _, p := range s.pairs {
		if p.built.Desc != "" {
			out = append(out, p.built.Desc)
		}
	}
	return out
}

// PluginManifest collects the external-plugin manifest entries. Each entry's
// ToolCount is adjusted down by the conflict drops recorded for its prefix, so
// the manifest reports the post-conflict count — what the model actually sees —
// not the raw plugin advertisement (the operator can spot when a conflict
// shadowed a tool they expected).
func (s *Set) PluginManifest() []runtime.PluginInfo {
	var out []runtime.PluginInfo
	for _, p := range s.pairs {
		for _, info := range p.built.Infos {
			for name := range s.dropped {
				if strings.HasPrefix(name, info.Prefix+".") && info.ToolCount > 0 {
					info.ToolCount--
				}
			}
			out = append(out, info)
		}
	}
	return out
}

// ToolCapabilities merges every built spec's declared capability map (M900).
// Capabilities declared for a name a YieldOnConflict spec lost are skipped — a
// plugin's declared cap must never re-scope the in-process tool that shadowed
// it.
func (s *Set) ToolCapabilities() map[string]string {
	var out map[string]string
	for _, p := range s.pairs {
		for name, cap := range p.built.Caps {
			if s.dropped[name] {
				continue
			}
			if out == nil {
				out = map[string]string{}
			}
			out[name] = cap
		}
	}
	return out
}

// NetguardGaps returns the names of built Netguard specs none of whose
// instances implement NetguardAware — i.e. egress-guarded tools whose SSRF
// refusals would go unjournaled. A registry-driven guard test asserts this is
// empty, replacing the old hand-listed type switch. Specs that skipped
// themselves at Build are not gaps (nothing was built to wire).
func (s *Set) NetguardGaps() []string {
	var gaps []string
	for _, p := range s.pairs {
		if !p.spec.Netguard {
			continue
		}
		aware := false
		if _, ok := p.built.Tool.(NetguardAware); ok {
			aware = true
		}
		for _, tl := range p.built.Extra {
			if _, ok := tl.(NetguardAware); ok {
				aware = true
				break
			}
		}
		if !aware {
			gaps = append(gaps, p.spec.Name)
		}
	}
	return gaps
}

func sortedKeys(m map[string]agent.Tool) []string {
	if len(m) == 0 {
		return nil
	}
	out := make([]string, 0, len(m))
	for k := range m {
		out = append(out, k)
	}
	sort.Strings(out)
	return out
}

