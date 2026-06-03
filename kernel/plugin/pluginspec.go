// SPDX-License-Identifier: MIT

package plugin

import (
	"fmt"
	"strings"
)

// PluginSpecEntry is one parsed `AGEZT_PLUGINS` entry: a tool-name
// prefix plus the executable (and its args) to spawn for it.
type PluginSpecEntry struct {
	// Prefix namespaces the plugin's tools (a plugin advertising
	// "search" registers as "<Prefix>.search").
	Prefix string
	// Path is the plugin executable.
	Path string
	// Args are passed to the executable after Path. May be nil when
	// none were given; always safe to range over.
	Args []string
}

// ParsePluginSpec decodes an `AGEZT_PLUGINS`-style spec:
//
//	prefix1=path1 arg1 arg2,prefix2=path2
//
// Each comma-separated entry is `<prefix>=<path> [args...]`; whitespace
// around tokens is trimmed and the path is split on spaces into the
// executable and its arguments.
//
// Malformed entries are a hard error so the operator gets startup-time
// feedback rather than a daemon that silently runs with fewer tools
// than configured — matching the sibling ParsePinSpec /
// ParseToolAllowlistSpec semantics:
//
//   - an entry missing '=' is rejected;
//   - an empty prefix is rejected;
//   - an empty path is rejected;
//   - a duplicate prefix is rejected. (Previously the boot loop spawned
//     *both* processes; the second's tools then lost a name conflict to
//     the first and produced a misleading "conflicts with in-process
//     version" warning. A repeated prefix is a config typo, not a
//     request to run two plugins under one namespace — surface it.)
//
// Empty / whitespace-only spec → nil slice + nil error.
func ParsePluginSpec(spec string) ([]PluginSpecEntry, error) {
	var out []PluginSpecEntry
	seen := map[string]struct{}{}
	for entry := range strings.SplitSeq(spec, ",") {
		entry = strings.TrimSpace(entry)
		if entry == "" {
			continue
		}
		prefix, cmdLine, ok := strings.Cut(entry, "=")
		if !ok {
			return nil, fmt.Errorf("plugin: spec entry %q missing '=' (expected '<prefix>=<path>')", entry)
		}
		prefix = strings.TrimSpace(prefix)
		if prefix == "" {
			return nil, fmt.Errorf("plugin: spec entry %q has empty prefix", entry)
		}
		parts := strings.Fields(cmdLine)
		if len(parts) == 0 {
			return nil, fmt.Errorf("plugin: spec entry %q has empty path", entry)
		}
		if _, dup := seen[prefix]; dup {
			return nil, fmt.Errorf("plugin: prefix %q is defined more than once", prefix)
		}
		seen[prefix] = struct{}{}
		out = append(out, PluginSpecEntry{
			Prefix: prefix,
			Path:   parts[0],
			Args:   append([]string(nil), parts[1:]...),
		})
	}
	return out, nil
}
