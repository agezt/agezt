// SPDX-License-Identifier: MIT

package plugin

import (
	"fmt"
	"strings"
)

// PinSpec maps plugin prefixes to the BLAKE3-256 hash their binary
// must hash to. Parsed from the operator's `AGEZT_PLUGIN_PINS` env
// var (M1.ff) by ParsePinSpec; consumed by the daemon's plugin
// boot loop when constructing each plugin.Config.
//
// Map shape rather than per-plugin field on Config:
//   - Plugin Config is built per-entry from the AGEZT_PLUGINS spec.
//   - Pins are configured *separately* so an operator can turn pins
//     on/off without re-listing every plugin path.
//   - One env var to parse keeps the daemon's bootstrap simple.
type PinSpec map[string]string

// ParsePinSpec decodes a `AGEZT_PLUGIN_PINS`-style spec:
//
//	prefix1=hash1,prefix2=hash2
//
// Each entry must be `<prefix>=<64-char-hex-digest>`. Whitespace
// around tokens is trimmed; entries with malformed pins are a hard
// error (operator gets startup-time feedback instead of a
// surprising "plugin failed to start" mid-run).
//
// Unknown prefixes (a pin for a plugin not present in
// AGEZT_PLUGINS) are tolerated and reported via UnusedPins so the
// daemon can warn the operator at startup without failing — a
// pin-for-removed-plugin should be a typo to fix, not a daemon
// outage.
//
// Empty / whitespace-only spec → empty PinSpec + nil error.
func ParsePinSpec(spec string) (PinSpec, error) {
	out := PinSpec{}
	for entry := range strings.SplitSeq(spec, ",") {
		entry = strings.TrimSpace(entry)
		if entry == "" {
			continue
		}
		prefix, pin, ok := strings.Cut(entry, "=")
		if !ok {
			return nil, fmt.Errorf("plugin: pin entry %q missing '='", entry)
		}
		prefix = strings.TrimSpace(prefix)
		pin = strings.ToLower(strings.TrimSpace(pin))
		if prefix == "" {
			return nil, fmt.Errorf("plugin: pin entry %q has empty prefix", entry)
		}
		if !looksLikeBLAKE3Pin(pin) {
			return nil, fmt.Errorf("plugin: pin for %q is not a 64-char lowercase hex BLAKE3-256 digest (got %q)", prefix, pin)
		}
		out[prefix] = pin
	}
	return out, nil
}

// UnusedPins returns prefixes in spec that didn't match any in
// usedPrefixes. The daemon calls this after spawning every plugin
// to warn the operator about pins for plugins that aren't loaded
// — usually a stale entry the operator forgot to remove.
func (s PinSpec) UnusedPins(usedPrefixes []string) []string {
	seen := make(map[string]struct{}, len(usedPrefixes))
	for _, p := range usedPrefixes {
		seen[p] = struct{}{}
	}
	var stale []string
	for prefix := range s {
		if _, used := seen[prefix]; !used {
			stale = append(stale, prefix)
		}
	}
	return stale
}

// ToolAllowlistSpec maps plugin prefix → list of tool names the
// plugin is permitted to advertise (M1.hh).
type ToolAllowlistSpec map[string][]string

// ParseToolAllowlistSpec decodes a `AGEZT_PLUGIN_TOOLS`-style spec:
//
//	prefix1=tool1+tool2,prefix2=toolA+toolB+toolC
//
// `+` separates tools inside a prefix's list (chosen over `,`
// because `,` is already the entry separator and `;` is hostile
// to URL-style env-var habits). Whitespace tolerated; empty value
// `prefix=` is a hard error (operators meaning "deny everything"
// should just unset the plugin, not register an empty allowlist).
//
// Unknown prefixes (allowlist for a plugin not present in
// AGEZT_PLUGINS) are tolerated and surfaced via Unused — same
// pattern as PinSpec.
func ParseToolAllowlistSpec(spec string) (ToolAllowlistSpec, error) {
	out := ToolAllowlistSpec{}
	for entry := range strings.SplitSeq(spec, ",") {
		entry = strings.TrimSpace(entry)
		if entry == "" {
			continue
		}
		prefix, list, ok := strings.Cut(entry, "=")
		if !ok {
			return nil, fmt.Errorf("plugin: tool-allowlist entry %q missing '='", entry)
		}
		prefix = strings.TrimSpace(prefix)
		if prefix == "" {
			return nil, fmt.Errorf("plugin: tool-allowlist entry %q has empty prefix", entry)
		}
		var tools []string
		for t := range strings.SplitSeq(list, "+") {
			if t = strings.TrimSpace(t); t != "" {
				tools = append(tools, t)
			}
		}
		if len(tools) == 0 {
			return nil, fmt.Errorf("plugin: tool-allowlist entry %q has empty tool list (unset the plugin instead of allow-listing nothing)", entry)
		}
		out[prefix] = tools
	}
	return out, nil
}

// Unused returns prefixes in the spec that didn't match any in
// usedPrefixes — same diff helper as PinSpec.UnusedPins.
func (s ToolAllowlistSpec) Unused(usedPrefixes []string) []string {
	seen := make(map[string]struct{}, len(usedPrefixes))
	for _, p := range usedPrefixes {
		seen[p] = struct{}{}
	}
	var stale []string
	for prefix := range s {
		if _, used := seen[prefix]; !used {
			stale = append(stale, prefix)
		}
	}
	return stale
}
