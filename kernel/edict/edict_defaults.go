// SPDX-License-Identifier: MIT

// Package edict: default level map + default hard-deny rules + capability list
// + the deny-rule parser (DefaultLevels + DefaultHardDeny + AllCapabilities +
// knownCapability + KnownCapability + ParseDenyRules). Extracted from
// edict_engine.go during the Day-211 god-file split. Public API unchanged.
package edict


import (
	"fmt"
	"slices"
	"strings"
)
func DefaultLevels() map[Capability]TrustLevel {
	levels := make(map[Capability]TrustLevel, len(AllCapabilities()))
	for _, c := range AllCapabilities() {
		levels[c] = LevelAllow
	}
	return levels
}

// DefaultHardDeny is the immutable hard-deny set from DECISIONS F4. The
// substrings are case-insensitive and only checked for the specified
// capabilities so they don't false-positive on unrelated tool input.
func DefaultHardDeny() []HardDenyRule {
	return []HardDenyRule{
		{Name: "fork-bomb", Substring: ":(){:|:&};:", AppliesTo: []Capability{CapShell}},
		{Name: "rm-rf-root", Substring: "rm -rf /", AppliesTo: []Capability{CapShell}},
		{Name: "rm-rf-root-flag", Substring: "rm -rf --no-preserve-root", AppliesTo: []Capability{CapShell}},
		{Name: "mkfs", Substring: "mkfs", AppliesTo: []Capability{CapShell}},
		{Name: "wipefs", Substring: "wipefs", AppliesTo: []Capability{CapShell}},    // wipe FS signatures
		{Name: "dd-of-dev", Substring: "dd if=", AppliesTo: []Capability{CapShell}}, // dd reading a source (usual disk-write shape)
		// dd/redirect writing to a RAW BLOCK DEVICE (M175). Keyed on of=/dev/<dev>
		// for the common device families so a `dd of=/dev/sdb` with no `if=` is also
		// caught — while the safe pseudo-devices (/dev/null, /dev/zero, /dev/random)
		// are deliberately NOT matched, so benign `dd of=/dev/null` stays allowed.
		{Name: "dd-of-sd", Substring: "of=/dev/sd", AppliesTo: []Capability{CapShell}},
		{Name: "dd-of-nvme", Substring: "of=/dev/nvme", AppliesTo: []Capability{CapShell}},
		{Name: "dd-of-vd", Substring: "of=/dev/vd", AppliesTo: []Capability{CapShell}},
		{Name: "dd-of-xvd", Substring: "of=/dev/xvd", AppliesTo: []Capability{CapShell}}, // AWS Xen
		{Name: "dd-of-mmcblk", Substring: "of=/dev/mmcblk", AppliesTo: []Capability{CapShell}},
		{Name: "shutdown", Substring: "shutdown -", AppliesTo: []Capability{CapShell}},
		{Name: "poweroff", Substring: "poweroff", AppliesTo: []Capability{CapShell}},
		{Name: "reboot", Substring: "reboot", AppliesTo: []Capability{CapShell}},
		{Name: "powershell-format", Substring: "format-volume", AppliesTo: []Capability{CapShell}},
	}
}

// AllCapabilities returns every governed capability, sorted, for validation and
// operator-facing listings.
func AllCapabilities() []Capability {
	caps := []Capability{
		CapShell, CapFileRead, CapFileWrite, CapFileDelete, CapFileList,
		CapHTTPGet, CapHTTPPost, CapProviderCall, CapDelegate, CapCoding,
		CapACPAgent, CapRemoteRun, CapNotify,
		CapHomeAssistantRead, CapHomeAssistantCall,
		CapBrowserRead, CapBrowserAction, CapMemory, CapWorld, CapWebSearch, CapResearch, CapSchedule, CapRunsRead, CapStanding, CapBoard, CapWorkboard, CapSkill,
		CapIntrospect, CapOversee, CapCodeExec, CapToolForge, CapMCPInstall, CapMCP, CapConfigRead, CapConfigWrite,
		CapWorkflow, CapMarket,
	}
	slices.Sort(caps)
	return caps
}

// knownCapability reports whether s names a governed capability.
func knownCapability(s string) bool {
	return slices.Contains(AllCapabilities(), Capability(s))
}

// KnownCapability is the exported form of knownCapability (M900): the kernel
// uses it to validate a plugin manifest's DECLARED tool capability — a plugin
// may join an existing policy axis but never invent a new one.
func KnownCapability(s string) bool { return knownCapability(s) }

// ParseDenyRules parses operator-supplied hard-deny rules from a ';'-separated
// spec, for AGEZT_EDICT_DENY. Each entry is either:
//
//   - "substring"               — denies that substring for EVERY capability
//   - "<capability>:substring"  — denies it only for that capability, when the
//     text before the first ':' is a known capability (e.g. "shell:rm -rf",
//     "http.post:169.254", "file.delete:/etc"). If the prefix is not a known
//     capability the whole entry is treated as an all-capability substring (so
//     "https://evil.example" works verbatim).
//
// A blank substring is rejected — a hard-deny rule matching the empty string
// would deny every action. Returned rules are meant to be appended to
// DefaultHardDeny. Entry order is preserved; rules are named "operator[N]".
func ParseDenyRules(spec string) ([]HardDenyRule, error) {
	spec = strings.TrimSpace(spec)
	if spec == "" {
		return nil, nil
	}
	var out []HardDenyRule
	for raw := range strings.SplitSeq(spec, ";") {
		entry := strings.TrimSpace(raw)
		if entry == "" {
			continue
		}
		var applies []Capability
		substr := entry
		if i := strings.IndexByte(entry, ':'); i > 0 {
			if prefix := entry[:i]; knownCapability(prefix) {
				applies = []Capability{Capability(prefix)}
				substr = strings.TrimSpace(entry[i+1:])
			}
		}
		if substr == "" {
			return nil, fmt.Errorf("edict: deny rule %q has an empty substring (would deny everything)", entry)
		}
		out = append(out, HardDenyRule{
			Name:      fmt.Sprintf("operator[%d]", len(out)+1),
			Substring: substr,
			AppliesTo: applies,
		})
	}
	return out, nil
}
