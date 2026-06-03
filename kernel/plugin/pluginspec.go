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
// A path or argument that itself contains spaces can be wrapped in
// single or double quotes — necessary for the common Windows case of a
// plugin under `C:/Program Files/...`:
//
//	tool="C:/Program Files/agezt-tool.exe" --verbose
//
// The quotes are stripped and the spaces inside preserved. Unquoted
// input splits on whitespace exactly as before, so this is purely
// additive. (A path containing a comma still cannot be expressed — the
// comma is the entry separator.)
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
		parts, err := splitFields(cmdLine)
		if err != nil {
			return nil, fmt.Errorf("plugin: spec entry %q: %w", entry, err)
		}
		if len(parts) == 0 || parts[0] == "" {
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

// splitFields splits a command line into whitespace-separated fields,
// honoring single and double quotes so a single field (typically a
// path) may contain spaces. The quote characters are removed and the
// run between matching quotes is taken literally; quoting can start and
// stop mid-field (`a" "b` is one field "a b"). Unquoted input behaves
// exactly like strings.Fields. An unterminated quote is an error.
func splitFields(s string) ([]string, error) {
	var fields []string
	var cur strings.Builder
	inField := false
	var quote rune // 0 when not inside a quote, else the opening quote rune
	for _, r := range s {
		switch {
		case quote != 0:
			if r == quote {
				quote = 0 // closing quote — the field continues
			} else {
				cur.WriteRune(r)
			}
			inField = true
		case r == '"' || r == '\'':
			quote = r
			inField = true
		case r == ' ' || r == '\t' || r == '\n' || r == '\r':
			if inField {
				fields = append(fields, cur.String())
				cur.Reset()
				inField = false
			}
		default:
			cur.WriteRune(r)
			inField = true
		}
	}
	if quote != 0 {
		return nil, fmt.Errorf("unterminated %c quote", quote)
	}
	if inField {
		fields = append(fields, cur.String())
	}
	return fields, nil
}
