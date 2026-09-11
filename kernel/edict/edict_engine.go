// SPDX-License-Identifier: MIT

package edict

// Engine methods + factory + PolicyOverlay helpers: Levels + HardDenyRules
// + AskPolicy + SetAskPolicy + New + DefaultLevels + DefaultHardDeny +
// AllCapabilities + knownCapability + KnownCapability + ParseDenyRules +
// Decide + DecideWithCeiling + SetLevel + AddHardDeny + RemoveHardDeny +
// PolicyOverlay.IsEmpty + ProjectPolicyChanges + ApplyOverlay + Level.
// Carved out of edict.go during the Day 35 god file split #1.

import (
	"fmt"
	"maps"
	"slices"
	"strings"
)

func (e *Engine) Levels() map[Capability]TrustLevel {
	e.mu.RLock()
	defer e.mu.RUnlock()
	out := make(map[Capability]TrustLevel, len(e.levels))
	maps.Copy(out, e.levels)
	return out
}

// HardDenyRules returns a snapshot of the hard-deny rule set.
// Returned slice is a copy — same rationale as Levels().
func (e *Engine) HardDenyRules() []HardDenyRule {
	e.mu.RLock()
	defer e.mu.RUnlock()
	out := make([]HardDenyRule, len(e.hardDeny))
	copy(out, e.hardDeny)
	return out
}

// AskPolicy returns the configured AskPolicy. Useful for the
// control plane's `agt edict show` so operators can confirm
// whether the daemon is currently in allow/deny/prompt mode.
func (e *Engine) AskPolicy() AskPolicy {
	e.mu.RLock()
	defer e.mu.RUnlock()
	return e.askPolicy
}

// SetAskPolicy changes the engine-wide approval mode at runtime. The
// hard-deny floor is unaffected (it fires before AskPolicy is consulted),
// so even AskAllow can't relax a hard-deny. Safe for concurrent use.
func (e *Engine) SetAskPolicy(p AskPolicy) {
	e.mu.Lock()
	defer e.mu.Unlock()
	e.askPolicy = p
}

// New builds an Engine. Unset Options fall back to DefaultLevels +
// DefaultHardDeny + AskAllow.
func New(opt Options) *Engine {
	defaults := DefaultLevels()
	levels := make(map[Capability]TrustLevel, len(defaults)+len(opt.Levels))
	maps.Copy(levels, defaults)
	maps.Copy(levels, opt.Levels)
	hd := opt.HardDeny
	if hd == nil {
		hd = DefaultHardDeny()
	}
	return &Engine{
		levels:       levels,
		hardDeny:     hd,
		askPolicy:    opt.AskPolicy,
		unknownAllow: opt.UnknownAllow,
	}
}

// DefaultLevels is the MAX-AUTONOMY posture (M814, owner's law: "has
// permission for everything unless you turn it off by default" — everything is
// allowed unless the operator turns it off). Every capability defaults to LevelAllow;
// restriction is the operator's opt-OUT, applied per capability through the
// Policy center, `agt edict`, AGEZT_EDICT_DENY, or a durable overlay.
//
// What this deliberately does NOT relax (they are guards, not permissions):
//   - the F4 hard-deny strings (fork bombs, rm -rf /, raw-device writes);
//   - the http/browser SSRF guards (loopback/private-net egress);
//   - Governor budget ceilings and per-agent daily caps;
//   - EXPLICIT HITL surfaces the operator wires on purpose (the workflow
//     approval node, the forge promotion queue) — those block on the
//     approval registry regardless of capability levels.
//
// History: the pre-M814 ladder mixed Allow/AskFirst/Ask per DECISIONS F3,
// but the owner ran AskPolicy=AskAllow, which folded every ask to allow in
// practice — this makes the real posture the explicit one. New capabilities
// ship at LevelAllow unless the owner says otherwise.
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

// Decide returns the engine's verdict for one (capability, input) pair.
// input is the stringified tool input — the engine treats it as opaque
// text for hard-deny substring matching only. The runtime is responsible
// for journaling the Outcome.
func (e *Engine) Decide(cap Capability, input string) Outcome {
	// No ceiling: LevelAllow (the max) clamps nothing, so behaviour is unchanged.
	return e.DecideWithCeiling(cap, input, LevelAllow)
}

// DecideWithCeiling is Decide with a per-call trust ceiling (SPEC-16 §4
// initiative.max_trust): the looked-up capability level is clamped to at most
// `ceiling` before the level→decision mapping, so a normally auto-allowed (L4)
// capability is downgraded to Ask (or, at ceiling L0, Deny) within a bounded
// context like a standing order. The hard-deny floor and unknown-capability
// default-deny are unaffected — a ceiling can only TIGHTEN, never loosen.
func (e *Engine) DecideWithCeiling(cap Capability, input string, ceiling TrustLevel) Outcome {
	e.mu.RLock()
	defer e.mu.RUnlock()

	// 1. Hard-deny always wins. Match against the decoded, normalized action — not
	// just the raw JSON tool-arg text. The model picks the command string, so it
	// could otherwise evade a floor rule by JSON-escaping a banned token
	// (`{"command":"rm -rf /"}`) or by padding whitespace (`rm  -rf /`); both
	// decode/normalize back to the banned form. denyCandidates returns the raw
	// input (no regression) plus each JSON string value with whitespace collapsed
	// (M173). A rule firing on ANY candidate denies.
	candidates := denyCandidates(input)
	for _, r := range e.hardDeny {
		for _, c := range candidates {
			if r.matches(cap, c) {
				return Outcome{
					Decision:     DecisionDeny,
					Capability:   cap,
					Level:        LevelDeny,
					Reason:       "hard-deny rule matched: " + r.Name,
					HardDenied:   true,
					HardDenyRule: r.Name,
				}
			}
		}
	}

	// 2. Look up the trust level for this capability.
	lvl, ok := e.levels[cap]
	if !ok {
		// Unknown capability. Default-deny is the strict, secure default (the
		// user must explicitly grant a level). But under UnknownAllow (M613, set
		// by AGEZT_ALLOW_ALL) an unconfigured capability is treated as L4 so
		// "allow everything" covers tools whose capability isn't in DefaultLevels
		// — including future plugin tools. Hard-deny already ran above, so the
		// catastrophe rails still hold.
		if !e.unknownAllow {
			return Outcome{
				Decision:   DecisionDeny,
				Capability: cap,
				Level:      LevelDeny,
				Reason:     fmt.Sprintf("no trust level configured for %q (default-deny)", cap),
			}
		}
		lvl = LevelAllow
	}

	// 2b. Clamp to the per-call ceiling (SPEC-16 §4): autonomy within this context
	// can be capped below the capability's configured level. Only ever tightens.
	ceilNote := ""
	if ceiling < lvl {
		lvl = ceiling
		ceilNote = fmt.Sprintf(" (clamped to ceiling %s)", ceiling)
	}

	// 3. Apply level → decision.
	switch lvl {
	case LevelDeny:
		return Outcome{
			Decision:   DecisionDeny,
			Capability: cap,
			Level:      lvl,
			Reason:     "capability set to L0 (deny)" + ceilNote,
		}
	case LevelAllow:
		return Outcome{
			Decision:   DecisionAllow,
			Capability: cap,
			Level:      lvl,
			Reason:     "capability set to L4 (allow)",
		}
	default: // L1..L3 — Ask
		switch e.askPolicy {
		case AskDeny:
			return Outcome{
				Decision:   DecisionDeny,
				Capability: cap,
				Level:      lvl,
				Reason:     fmt.Sprintf("level %s requires approval; AskPolicy=AskDeny", lvl) + ceilNote,
			}
		case AskPrompt:
			return Outcome{
				// Fail-closed default for callers that don't honour
				// RequiresApproval — the runtime overrides this after
				// a real grant.
				Decision:         DecisionDeny,
				Capability:       cap,
				Level:            lvl,
				Reason:           fmt.Sprintf("level %s; AskPolicy=AskPrompt → operator approval required", lvl) + ceilNote,
				WouldAsk:         true,
				RequiresApproval: true,
			}
		default: // AskAllow
			return Outcome{
				Decision:   DecisionAllow,
				Capability: cap,
				Level:      lvl,
				Reason:     fmt.Sprintf("level %s; AskPolicy=AskAllow (would prompt in MVP)", lvl) + ceilNote,
				WouldAsk:   true,
			}
		}
	}
}

// SetLevel changes the trust level for a capability at runtime. Useful
// for the future `agt trust` CLI; safe for concurrent use.
func (e *Engine) SetLevel(cap Capability, lvl TrustLevel) {
	e.mu.Lock()
	defer e.mu.Unlock()
	e.levels[cap] = lvl
}

// AddHardDeny appends a hard-deny rule at runtime and returns the stored
// rule (with its engine-assigned name). The caller supplies Substring and
// AppliesTo (e.g. from ParseDenyRules); the Name is always overwritten
// with a fresh "runtime[N]" so the rule is removable via RemoveHardDeny
// and can never be confused with a built-in or operator[N] floor rule.
//
// A blank substring is rejected for the same reason ParseDenyRules rejects
// it: a rule matching the empty string would deny every action. Safe for
// concurrent use; the change takes effect on the next Decide.
func (e *Engine) AddHardDeny(rule HardDenyRule) (HardDenyRule, error) {
	if strings.TrimSpace(rule.Substring) == "" {
		return HardDenyRule{}, fmt.Errorf("edict: hard-deny rule has an empty substring (would deny everything)")
	}
	e.mu.Lock()
	defer e.mu.Unlock()
	e.rtSeq++
	rule.Name = fmt.Sprintf("%s%d]", RuntimeRulePrefix, e.rtSeq)
	e.hardDeny = append(e.hardDeny, rule)
	return rule, nil
}

// RemoveHardDeny removes the runtime-added hard-deny rule named name and
// reports whether a rule was removed. It refuses to touch the boot-time
// floor: removing a built-in or operator[N] rule returns an error, never a
// silent success — the floor stays put. Safe for concurrent use.
func (e *Engine) RemoveHardDeny(name string) (bool, error) {
	if !IsRuntimeRule(name) {
		return false, fmt.Errorf("edict: %q is not a runtime-added rule; the boot-time deny floor cannot be removed at runtime", name)
	}
	e.mu.Lock()
	defer e.mu.Unlock()
	for i, r := range e.hardDeny {
		if r.Name == name {
			e.hardDeny = slices.Delete(e.hardDeny, i, i+1)
			return true, nil
		}
	}
	return false, nil
}

// PolicyChange is one decoded policy.changed event — the input to
// ProjectPolicyChanges. Field names mirror the control plane's
// policy.changed payloads (actions "level.set", "deny.add", "deny.rm"),
// so a journal payload unmarshals straight into this struct.
type PolicyChange struct {
	Action     string   `json:"action"`
	Capability string   `json:"capability"`
	To         string   `json:"to"`
	Name       string   `json:"name"`
	Substring  string   `json:"substring"`
	AppliesTo  []string `json:"applies_to"`
}

// PolicyOverlay is the net effect of a sequence of PolicyChanges: the
// per-capability level overrides plus the surviving runtime deny rules.
// It is what a daemon replays onto a freshly-booted engine to make
// runtime policy changes durable across a restart.
type PolicyOverlay struct {
	Levels    map[Capability]TrustLevel
	DenyRules []HardDenyRule
	// Mode, when non-nil, is the net runtime approval-mode override. A
	// pointer so "no mode change in history" is distinct from AskAllow (0).
	Mode *AskPolicy
}

// IsEmpty reports whether the overlay carries no changes — lets a caller
// skip the apply (and a banner line) when there's nothing to restore.
func (o PolicyOverlay) IsEmpty() bool {
	return len(o.Levels) == 0 && len(o.DenyRules) == 0 && o.Mode == nil
}

// ProjectPolicyChanges folds an ordered sequence of policy.changed events
// into the net overlay. It is pure (no engine, no I/O) so it is trivially
// testable and the daemon's only job is to decode the journal and apply
// the result.
//
// Semantics: level.set is last-wins per capability; deny.add/deny.rm are
// tracked by the rule's journaled name so an add later removed leaves no
// trace, and surviving rules keep their original add order. Malformed
// entries (blank capability/substring, unparseable level) are skipped
// rather than failing the whole replay — one bad historical event must
// not wedge a restart.
func ProjectPolicyChanges(changes []PolicyChange) PolicyOverlay {
	levels := map[Capability]TrustLevel{}
	denyByName := map[string]HardDenyRule{}
	var order []string // surviving deny names, in add order
	var mode *AskPolicy
	for _, ch := range changes {
		switch ch.Action {
		case "mode.set":
			if p, err := ParseAskPolicy(ch.To); err == nil {
				mode = &p // last-wins
			}
		case "level.set":
			if ch.Capability == "" {
				continue
			}
			lvl, err := ParseTrustLevel(ch.To)
			if err != nil {
				continue
			}
			levels[Capability(ch.Capability)] = lvl
		case "deny.add":
			if ch.Name == "" || strings.TrimSpace(ch.Substring) == "" {
				continue
			}
			var caps []Capability
			for _, c := range ch.AppliesTo {
				if c != "" {
					caps = append(caps, Capability(c))
				}
			}
			if _, seen := denyByName[ch.Name]; !seen {
				order = append(order, ch.Name)
			}
			denyByName[ch.Name] = HardDenyRule{Name: ch.Name, Substring: ch.Substring, AppliesTo: caps}
		case "deny.rm":
			if _, ok := denyByName[ch.Name]; ok {
				delete(denyByName, ch.Name)
				for i, n := range order {
					if n == ch.Name {
						order = slices.Delete(order, i, i+1)
						break
					}
				}
			}
		}
	}
	overlay := PolicyOverlay{Mode: mode}
	if len(levels) > 0 {
		overlay.Levels = levels
	}
	for _, n := range order {
		overlay.DenyRules = append(overlay.DenyRules, denyByName[n])
	}
	return overlay
}

// ApplyOverlay applies a projected overlay onto the engine: level
// overrides via SetLevel, surviving runtime deny rules via AddHardDeny
// (each re-assigned a fresh runtime[N] name). Returns the counts applied,
// for an operator banner. Used by the daemon when AGEZT_EDICT_DURABLE is
// on to restore runtime policy from the journal.
func (e *Engine) ApplyOverlay(o PolicyOverlay) (levels, rules int) {
	if o.Mode != nil {
		e.SetAskPolicy(*o.Mode)
	}
	for cap, lvl := range o.Levels {
		e.SetLevel(cap, lvl)
		levels++
	}
	for _, r := range o.DenyRules {
		if _, err := e.AddHardDeny(r); err == nil {
			rules++
		}
	}
	return levels, rules
}

// Level returns the current trust level for a capability, and a bool
// indicating whether it was explicitly configured (vs. default-deny).
func (e *Engine) Level(cap Capability) (TrustLevel, bool) {
	e.mu.RLock()
	defer e.mu.RUnlock()
	lvl, ok := e.levels[cap]
	return lvl, ok
}

