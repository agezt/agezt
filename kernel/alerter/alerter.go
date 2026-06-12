// SPDX-License-Identifier: MIT

// Package alerter pushes warning/critical alerts to the configured channels
// (M782). It watches the bus for proactive-signal event kinds (run failures,
// blocked egress, budget/rate trips, halts, and a pending approval — M922) and
// delivers a short brief through the existing Pulse channel sinks — so the
// operator hears about problems, and is asked to approve a blocked run, without
// the console open. (The console surfaces a pending approval through its own
// ApprovalsBell rather than the Alerts view, so the two stay in sync on intent
// even though only the daemon classifies approvals here.)
//
// Pulse-originated kinds (observer.delta, briefing.sent) are deliberately NOT
// handled here: the Pulse engine already delivers its own briefs through the
// same sinks, and notifying them twice would double every heartbeat signal.
package alerter

import (
	"context"
	"encoding/json"
	"strings"
	"sync"
	"time"

	"github.com/agezt/agezt/kernel/bus"
	"github.com/agezt/agezt/kernel/event"
	"github.com/agezt/agezt/kernel/pulse"
)

// Level ranks alert severity. Mirrors the console's classifier
// (frontend/src/lib/alerts.ts): info signals exist but never notify.
type Level int

const (
	LevelInfo Level = iota
	LevelWarning
	LevelCritical
)

// String renders the level for status lines and tests.
func (l Level) String() string {
	switch l {
	case LevelCritical:
		return "critical"
	case LevelWarning:
		return "warning"
	default:
		return "info"
	}
}

// ParseLevel reads a minimum-level config value. Only "critical" narrows the
// gate; anything else (including empty) means warning-and-up, the default.
func ParseLevel(s string) Level {
	if strings.EqualFold(strings.TrimSpace(s), "critical") {
		return LevelCritical
	}
	return LevelWarning
}

// Alert is one classified proactive signal.
type Alert struct {
	Kind   event.Kind
	Level  Level
	Title  string
	Detail string
	Source string
}

// Classify maps an event to an Alert, or ok=false when the event is not a
// notify-worthy proactive signal. The kinds and severities mirror the console's
// Alerts view exactly (warning/critical only — the badge-worthy set).
func Classify(ev *event.Event) (Alert, bool) {
	if ev == nil {
		return Alert{}, false
	}
	p := payloadMap(ev.Payload)
	switch ev.Kind {
	case event.KindTaskFailed:
		return Alert{Kind: ev.Kind, Level: LevelWarning, Title: "run failed",
			Detail: firstStr(p, "reason", "error"), Source: "run"}, true
	case event.KindNetguardBlocked:
		src := str(p, "tool")
		if src == "" {
			src = "egress"
		}
		return Alert{Kind: ev.Kind, Level: LevelWarning, Title: "egress blocked",
			Detail: joinNonEmpty(" — ", str(p, "ip"), str(p, "reason")), Source: src}, true
	case event.KindBudgetExceeded:
		return Alert{Kind: ev.Kind, Level: LevelCritical, Title: "budget ceiling exceeded",
			Source: "budget"}, true
	case event.KindRateLimited:
		return Alert{Kind: ev.Kind, Level: LevelWarning, Title: "provider rate-limited",
			Detail: str(p, "provider"), Source: "provider"}, true
	case event.KindHalt:
		return Alert{Kind: ev.Kind, Level: LevelCritical, Title: "daemon halted",
			Detail: str(p, "reason"), Source: "kernel"}, true
	case event.KindApprovalRequested:
		// A run is BLOCKED on the operator's decision (M922). Easy to miss on the
		// Approvals tab, so push it to the channel like any warning — the agent
		// stalls until you grant or deny.
		return Alert{Kind: ev.Kind, Level: LevelWarning, Title: "approval needed",
			Detail: joinNonEmpty(" — ", firstStr(p, "capability", "tool_name"), str(p, "reason")), Source: "approval"}, true
	}
	return Alert{}, false
}

// Config tunes the notifier. Zero values get safe defaults (see normalize).
type Config struct {
	// MinLevel gates delivery: LevelWarning (default) sends warnings and
	// criticals; LevelCritical sends criticals only.
	MinLevel Level
	// Cooldown suppresses repeats of the same alert (kind + correlation)
	// within the window. Default 5m.
	Cooldown time.Duration
	// MaxPerWindow caps total deliveries inside Window — a flood of distinct
	// failures must not turn a channel into a siren. Default 12.
	MaxPerWindow int
	// Window is the trailing window MaxPerWindow is measured over. Default 10m.
	Window time.Duration
	// Mute (M815) is a daily quiet window during which alerts are suppressed
	// (the operator silences pings overnight even though they're high
	// priority). Disabled by default. CRITICAL alerts still break through —
	// a budget blowout or a halt at 3am is exactly what the owner DOES want
	// to hear; only warnings honour the mute.
	Mute pulse.QuietHours
	// MuteSources (M815) is the set of alert sources to drop entirely
	// (per-category routing: e.g. mute noisy "provider" rate-limit pings
	// while keeping "budget" and "kernel"). Matched case-insensitively
	// against Alert.Source. A critical from a muted source is still
	// suppressed — muting a category means "I never want these".
	MuteSources map[string]bool
}

func (c Config) normalize() Config {
	if c.Cooldown <= 0 {
		c.Cooldown = 5 * time.Minute
	}
	if c.MaxPerWindow <= 0 {
		c.MaxPerWindow = 12
	}
	if c.Window <= 0 {
		c.Window = 10 * time.Minute
	}
	if c.MinLevel < LevelWarning {
		c.MinLevel = LevelWarning
	}
	return c
}

// Notifier classifies events and delivers gated briefs to a sink. Safe for
// concurrent use; Start runs one goroutine but tests drive Handle directly.
type Notifier struct {
	cfg  Config
	sink pulse.BriefSink
	now  func() time.Time

	mu       sync.Mutex
	lastSent map[string]time.Time // dedup: kind+correlation → last delivery
	recent   []time.Time          // rate cap: deliveries inside the window
}

// New builds a Notifier. sink must be non-nil.
func New(sink pulse.BriefSink, cfg Config) *Notifier {
	return &Notifier{cfg: cfg.normalize(), sink: sink, now: time.Now,
		lastSent: map[string]time.Time{}}
}

// Handle classifies one event and, when it passes the level/source/mute/
// dedup/rate gates, delivers it. Returns true when a brief was delivered.
func (n *Notifier) Handle(ev *event.Event) bool {
	a, ok := Classify(ev)
	if !ok || a.Level < n.cfg.MinLevel {
		return false
	}
	// Per-source routing (M815): a muted category never notifies, at any level.
	if n.cfg.MuteSources != nil && a.Source != "" &&
		n.cfg.MuteSources[strings.ToLower(a.Source)] {
		return false
	}
	now := n.now()
	// Mute window (M815): warnings are held during the quiet window; criticals
	// always break through — that's the whole point of a critical.
	if a.Level < LevelCritical && n.cfg.Mute.Active(now) {
		return false
	}
	key := string(a.Kind) + "/" + ev.CorrelationID
	n.mu.Lock()
	if last, seen := n.lastSent[key]; seen && now.Sub(last) < n.cfg.Cooldown {
		n.mu.Unlock()
		return false
	}
	// Prune the rate window, then check the cap.
	cut := now.Add(-n.cfg.Window)
	kept := n.recent[:0]
	for _, t := range n.recent {
		if t.After(cut) {
			kept = append(kept, t)
		}
	}
	n.recent = kept
	if len(n.recent) >= n.cfg.MaxPerWindow {
		n.mu.Unlock()
		return false
	}
	n.recent = append(n.recent, now)
	n.lastSent[key] = now
	if len(n.lastSent) > 4096 { // bound the dedup map across a long daemon life
		for k, t := range n.lastSent {
			if now.Sub(t) >= n.cfg.Cooldown {
				delete(n.lastSent, k)
			}
		}
	}
	n.mu.Unlock()
	_ = n.sink.Deliver(brief(a, ev.CorrelationID)) // a channel outage must not wedge the watcher
	return true
}

// ParseMuteSources reads a comma/space-separated source list ("provider,
// egress") into the lowercase set Config.MuteSources expects. Empty → nil.
func ParseMuteSources(s string) map[string]bool {
	out := map[string]bool{}
	for _, f := range strings.FieldsFunc(s, func(r rune) bool { return r == ',' || r == ' ' }) {
		out[strings.ToLower(strings.TrimSpace(f))] = true
	}
	if len(out) == 0 {
		return nil
	}
	return out
}

// brief renders an Alert as a Pulse brief. DispAlert is the "send now, high
// priority" disposition — these are exactly the signals that should break
// through digests and quiet hours.
func brief(a Alert, corr string) pulse.Brief {
	title := "⚠ " + a.Title
	if a.Level == LevelCritical {
		title = "🚨 " + a.Title
	}
	body := a.Detail
	if a.Source != "" {
		body = joinNonEmpty("\n", body, "source: "+a.Source)
	}
	return pulse.Brief{
		Title:         title,
		Body:          body,
		Disposition:   pulse.DispAlert,
		IssueKey:      "alert/" + string(a.Kind),
		CorrelationID: corr,
		Items:         1,
	}
}

// Start wires the notifier onto the bus: subscribe to everything, classify,
// gate, deliver. Returns false (nothing started) when bus or sink is missing.
// The goroutine stops on ctx cancellation or bus close; a panic in the loop is
// recovered so a notifier bug can never crash the daemon (anomaly pattern).
func Start(ctx context.Context, b *bus.Bus, sink pulse.BriefSink, cfg Config) bool {
	if b == nil || sink == nil {
		return false
	}
	sub, err := b.Subscribe(">", 256)
	if err != nil {
		return false
	}
	n := New(sink, cfg)
	go func() {
		defer func() {
			sub.Cancel()
			_ = recover() // a watcher panic must never take down the daemon
		}()
		for {
			select {
			case <-ctx.Done():
				return
			case ev, ok := <-sub.C:
				if !ok {
					return
				}
				n.Handle(ev)
			}
		}
	}()
	return true
}

// payloadMap decodes an event payload object, tolerating nil/non-object
// payloads (→ nil map, so every field lookup just comes back empty).
func payloadMap(raw json.RawMessage) map[string]any {
	if len(raw) == 0 {
		return nil
	}
	var m map[string]any
	if err := json.Unmarshal(raw, &m); err != nil {
		return nil
	}
	return m
}

func str(p map[string]any, key string) string {
	if p == nil {
		return ""
	}
	if v, ok := p[key]; ok && v != nil {
		if s, ok := v.(string); ok {
			return s
		}
	}
	return ""
}

func firstStr(p map[string]any, keys ...string) string {
	for _, k := range keys {
		if s := str(p, k); s != "" {
			return s
		}
	}
	return ""
}

func joinNonEmpty(sep string, parts ...string) string {
	var out []string
	for _, p := range parts {
		if p != "" {
			out = append(out, p)
		}
	}
	return strings.Join(out, sep)
}
