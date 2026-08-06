// SPDX-License-Identifier: MIT

// External-plugin host spec (Phase 2.2 PR 6): the AGEZT_PLUGINS spawn loop
// that used to close cmd/agezt's buildTools, migrated as ONE spec whose Built
// carries every plugin's tools (Extra), the plugin manifest (Infos) and the
// M900 declared capabilities (Caps). The spec is registered LAST and marked
// YieldOnConflict, so a plugin tool whose prefixed name collides with an
// in-process tool is dropped by toolreg.BuildAll with a warning — in-process
// wins, never a boot error (the historical semantic, made explicit).
package builtintools

import (
	"context"
	"fmt"
	"strings"
	"time"

	"github.com/agezt/agezt/internal/brand"
	"github.com/agezt/agezt/kernel/agent"
	"github.com/agezt/agezt/kernel/plugin"
	"github.com/agezt/agezt/kernel/redact"
	"github.com/agezt/agezt/kernel/runtime"
	"github.com/agezt/agezt/kernel/toolreg"
)

// specPlugins — external plugins via the AGEZT_PLUGINS env var (M1.y). Format:
//
//	AGEZT_PLUGINS="<prefix>=<path> <args...>"[,...]
//
// e.g. AGEZT_PLUGINS="search=/usr/local/bin/agezt-search,scrape=/opt/scraper".
// Each plugin is spawned at daemon start; its tools register under the given
// prefix. A plugin that fails to initialize is logged to stderr and skipped;
// the daemon continues with non-plugin tools so a broken plugin can't take
// down the kernel. Malformed AGEZT_PLUGIN_PINS / AGEZT_PLUGIN_TOOLS /
// AGEZT_PLUGINS specs are hard startup errors (fast operator feedback on a
// security setting beats a silently-broken pin).
func specPlugins() toolreg.Spec {
	return toolreg.Spec{
		Name:            "plugins",
		YieldOnConflict: true, // plugin tools always lose to in-process names
		Build:           buildPlugins,
	}
}

func buildPlugins(d toolreg.BuildDeps) (toolreg.Built, error) {
	spec := strings.TrimSpace(d.Get(brand.EnvPrefix + "PLUGINS"))
	if spec == "" {
		return toolreg.Built{}, nil
	}
	// Parse pin spec first (M1.ff). A syntactically-bad pin is a hard startup
	// error — operators want fast feedback on a security setting, not a
	// silently-broken pin that lets a modified binary slip through next
	// reboot. Unknown pins (for plugins not in AGEZT_PLUGINS) become warnings
	// after the plugin loop runs.
	var pins plugin.PinSpec
	if pinSpec := strings.TrimSpace(d.Get(brand.EnvPrefix + "PLUGIN_PINS")); pinSpec != "" {
		parsed, err := plugin.ParsePinSpec(pinSpec)
		if err != nil {
			return toolreg.Built{}, fmt.Errorf("%sPLUGIN_PINS: %w", brand.EnvPrefix, err)
		}
		pins = parsed
	}
	// Tool allowlist (M1.hh) — same hard-error semantics as pins.
	var allowedTools plugin.ToolAllowlistSpec
	if allowSpec := strings.TrimSpace(d.Get(brand.EnvPrefix + "PLUGIN_TOOLS")); allowSpec != "" {
		parsed, err := plugin.ParseToolAllowlistSpec(allowSpec)
		if err != nil {
			return toolreg.Built{}, fmt.Errorf("%sPLUGIN_TOOLS: %w", brand.EnvPrefix, err)
		}
		allowedTools = parsed
	}
	// Parse the spec up front (M223). A malformed entry — missing '=', empty
	// path, or a duplicate prefix — is a hard startup error, matching the
	// pin/allowlist specs parsed just above. A repeated prefix used to spawn
	// two processes whose tools then collided with a misleading "in-process
	// version" warning; rejecting it surfaces the typo instead.
	entries, err := plugin.ParsePluginSpec(spec)
	if err != nil {
		return toolreg.Built{}, fmt.Errorf("%sPLUGINS: %w", brand.EnvPrefix, err)
	}
	usedPrefixes := make([]string, 0, len(entries))
	// Scrub secrets of known formats from plugin stderr before it reaches the
	// daemon log (M229). Pattern-only (no literals) — a plugin leaks its own
	// secrets, not the daemon's.
	pluginRedactor := redact.New()

	var built toolreg.Built
	var registered []string

	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()
	for _, e := range entries {
		prefix := e.Prefix
		usedPrefixes = append(usedPrefixes, prefix)
		cfg := plugin.Config{
			Path: e.Path,
			Args: e.Args,
			Logger: func(line string) {
				fmt.Fprintln(d.Stderr, pluginLogLine(pluginRedactor, prefix, line))
			},
			PinnedHash:   pins[prefix],         // empty if no pin configured for this prefix
			AllowedTools: allowedTools[prefix], // nil if no allowlist for this prefix
		}
		p, err := plugin.Spawn(ctx, cfg)
		if err != nil {
			fmt.Fprintf(d.Stderr, "WARNING: plugin %q (%s) failed to start: %v\n", prefix, e.Path, err)
			continue
		}
		pluginTools := p.Tools(prefix + ".")
		declaredCaps := p.ToolCapabilities(prefix + ".") // M900: manifest-declared policy axes
		for name, tool := range pluginTools {
			if built.Extra == nil {
				built.Extra = map[string]agent.Tool{}
			}
			built.Extra[name] = tool
			if cap, ok := declaredCaps[name]; ok {
				if built.Caps == nil {
					built.Caps = map[string]string{}
				}
				built.Caps[name] = cap
			}
		}
		registered = append(registered, fmt.Sprintf("plugin:%s(%d tools)", prefix, len(pluginTools)))
		// Record manifest entry. ToolCount here is the loaded count; a name
		// later dropped by BuildAll's in-process-wins conflict handling is
		// subtracted by Set.PluginManifest, so the served tool_count stays
		// the post-conflict number — what the model actually sees — and the
		// operator can spot when a conflict shadowed a tool they expected.
		built.Infos = append(built.Infos, runtime.PluginInfo{
			Prefix:       prefix,
			Path:         e.Path,
			Args:         append([]string(nil), e.Args...),
			ToolCount:    len(pluginTools),
			HashPinned:   pins[prefix] != "",
			AllowedTools: append([]string(nil), allowedTools[prefix]...),
		})
	}
	// Warn about pin entries that didn't match any spawned plugin (typo,
	// removed plugin, etc.) — surfaces stale config without failing the daemon.
	for _, stale := range pins.UnusedPins(usedPrefixes) {
		fmt.Fprintf(d.Stderr, "WARNING: %sPLUGIN_PINS has entry for %q but no plugin with that prefix was loaded\n", brand.EnvPrefix, stale)
	}
	for _, stale := range allowedTools.Unused(usedPrefixes) {
		fmt.Fprintf(d.Stderr, "WARNING: %sPLUGIN_TOOLS has entry for %q but no plugin with that prefix was loaded\n", brand.EnvPrefix, stale)
	}
	built.Desc = strings.Join(registered, ", ")
	return built, nil
}

// pluginLogLine formats a plugin's stderr line for the daemon log, scrubbing
// any secret of a known format it may have printed (M229). A third-party
// plugin's stderr is untrusted output that lands directly in the operator's
// logs — a path the bus redactor (which only covers journaled events) does not
// touch. Pattern-based redaction is the right fit here: a plugin leaks its OWN
// secrets, which the daemon doesn't hold as literals but whose formats (sk-,
// Telegram, Groq, …) the built-in detectors catch.
func pluginLogLine(r *redact.Redactor, prefix, line string) string {
	return fmt.Sprintf("[plugin:%s] %s", prefix, r.Redact(line))
}
