// SPDX-License-Identifier: MIT

// Policy-overlay + orphan-run-reconciliation + anomaly/notify helpers
// extracted from main.go during Day 211 god-file refactor (#43).
// Public API unchanged.
package main

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"sort"
	"strconv"
	"strings"
	"time"

	"github.com/agezt/agezt/internal/brand"
	"github.com/agezt/agezt/kernel/alerter"
	"github.com/agezt/agezt/kernel/anomaly"
	"github.com/agezt/agezt/kernel/catalog"
	"github.com/agezt/agezt/kernel/creds"
	"github.com/agezt/agezt/kernel/edict"
	"github.com/agezt/agezt/kernel/event"
	"github.com/agezt/agezt/kernel/pulse"
	kernelruntime "github.com/agezt/agezt/kernel/runtime"
	"github.com/agezt/agezt/kernel/standing"
)

func replayPolicyOverlay(k *kernelruntime.Kernel) (edict.PolicyOverlay, error) {
	// Compaction (M95): if a snapshot exists, seed the fold with its collapsed
	// changes and replay only the journal events recorded AFTER it. ProjectPolicyChanges
	// is resumable (snapshot.ToChanges + later changes folds to the same overlay as the
	// full history), so this is equivalent to the uncompacted replay.
	//
	// Integrity (M176): the snapshot is trusted ONLY when its content hash equals the
	// latest journaled policy.compacted hash, binding it to the tamper-evident journal.
	// A corrupt snapshot, one edited on disk to loosen policy, or one predating the
	// binding fails this check and is ignored — the journal (the source of truth) is
	// folded in full instead.
	snap, serr := edict.LoadOverlaySnapshot(overlaySnapshotPath(k))
	if serr != nil {
		snap = nil
	}

	type seqChange struct {
		seq int64
		ch  edict.PolicyChange
	}
	var all []seqChange
	var journaledHash string // latest policy.compacted content hash
	err := k.Journal().Range(func(ev *event.Event) error {
		switch ev.Kind {
		case event.KindPolicyChanged:
			var ch edict.PolicyChange
			// A single malformed historical payload must not wedge boot; skip it
			// (ProjectPolicyChanges also skips malformed content).
			if json.Unmarshal(ev.Payload, &ch) == nil {
				all = append(all, seqChange{ev.Seq, ch})
			}
		case event.KindPolicyCompacted:
			var p struct {
				ContentHash string `json:"content_hash"`
			}
			if json.Unmarshal(ev.Payload, &p) == nil {
				journaledHash = p.ContentHash // last one wins
			}
		}
		return nil
	})
	if err != nil {
		return edict.PolicyOverlay{}, err
	}

	var changes []edict.PolicyChange
	fromSeq := int64(-1)
	if snap != nil && journaledHash != "" && snap.ContentHash() == journaledHash {
		changes = append(changes, snap.Changes...)
		fromSeq = snap.ThroughSeq
	}
	for _, sc := range all {
		if sc.seq > fromSeq {
			changes = append(changes, sc.ch)
		}
	}
	return edict.ProjectPolicyChanges(changes), nil
}

// overlaySnapshotPath is the per-kernel durable-policy snapshot location (M95),
// under the kernel's own base dir so each tenant snapshots independently.
func overlaySnapshotPath(k *kernelruntime.Kernel) string {
	return filepath.Join(k.BaseDir(), "runtime", edict.OverlaySnapshotFile)
}

// orphanRun is a run that was received but never completed in a prior
// session — found at boot by runScan.
type orphanRun struct {
	Corr      string
	Intent    string
	StartedMS int64
}

// runScan folds the journal's task.* events to find orphaned runs (M28). A
// run is orphaned when it has a task.received but no terminal event:
// neither a task.completed (it finished), a task.failed (it errored out
// live — M30), nor a task.abandoned (we already reconciled it on an
// earlier boot — the idempotency guard). Pure and fed one event at a
// time, so it's unit-testable without a kernel.
type runScan struct {
	received  map[string]*orphanRun
	completed map[string]bool
	failed    map[string]bool
	abandoned map[string]bool
}

func newRunScan() *runScan {
	return &runScan{
		received:  map[string]*orphanRun{},
		completed: map[string]bool{},
		failed:    map[string]bool{},
		abandoned: map[string]bool{},
	}
}

func (s *runScan) observe(e *event.Event) {
	switch e.Kind {
	case event.KindTaskReceived:
		o := &orphanRun{Corr: e.CorrelationID, StartedMS: e.TSUnixMS}
		var p struct {
			Intent string `json:"intent"`
		}
		_ = json.Unmarshal(e.Payload, &p)
		o.Intent = p.Intent
		s.received[e.CorrelationID] = o
	case event.KindTaskCompleted:
		s.completed[e.CorrelationID] = true
	case event.KindTaskFailed:
		s.failed[e.CorrelationID] = true
	case event.KindTaskAbandoned:
		s.abandoned[e.CorrelationID] = true
	}
}

// orphans returns the orphaned runs, sorted by start time then correlation
// id for deterministic output (and stable abandon-event ordering).
func (s *runScan) orphans() []orphanRun {
	var out []orphanRun
	for corr, o := range s.received {
		if !s.completed[corr] && !s.failed[corr] && !s.abandoned[corr] {
			out = append(out, *o)
		}
	}
	sort.Slice(out, func(i, j int) bool {
		if out[i].StartedMS != out[j].StartedMS {
			return out[i].StartedMS < out[j].StartedMS
		}
		return out[i].Corr < out[j].Corr
	})
	return out
}

// reconcileOrphanRuns scans the journal at boot for runs that were in-flight
// when a prior daemon exited and publishes a task.abandoned event for each,
// so `agt runs` shows them as "abandoned" rather than "running" forever
// (M28). Idempotent: a run already carrying task.abandoned is skipped, so
// repeated restarts don't re-abandon. Returns the count reconciled. MUST run
// before any new Run is dispatched (so the scan can't see a live run).
func reconcileOrphanRuns(k *kernelruntime.Kernel) (int, error) {
	scan := newRunScan()
	if err := k.Journal().Range(func(e *event.Event) error {
		scan.observe(e)
		return nil
	}); err != nil {
		return 0, err
	}
	orphans := scan.orphans()
	for _, o := range orphans {
		_, _ = k.Bus().Publish(event.Spec{
			Subject:       "task",
			Kind:          event.KindTaskAbandoned,
			Actor:         "kernel",
			CorrelationID: o.Corr,
			Payload: map[string]any{
				"intent":          o.Intent,
				"reason":          "daemon restart: run was in-flight and never completed",
				"started_unix_ms": o.StartedMS,
			},
		})
	}
	return len(orphans), nil
}

// modelAdvisory returns a one-line agent-readiness advisory for the selected
// primary model (M24), or "" when the model is unknown to the catalog or has no
// concerns. It surfaces the same catalog.Model.AgentWarnings that
// `agt provider check --caps` reports, but at boot — the moment an operator
// would want to know the headline gap: a model that doesn't advertise tool-use,
// which the tool-driven agent loop relies on. Unknown models (the offline mock,
// a model absent from the catalog) yield no advisory rather than a false alarm.
func modelAdvisory(cat *catalog.Catalog, model string) string {
	if cat == nil || model == "" {
		return ""
	}
	_, m := cat.FindModel(model)
	if m == nil {
		return ""
	}
	return strings.Join(m.AgentWarnings(), "; ")
}

// credSecrets returns the non-empty values of every vault entry plus any extra
// operator-supplied literals, for seeding the secret redactor (M15). Values, not
// names — the redactor scrubs the actual secret strings wherever they appear in
// event payloads. Extra literals (AGEZT_REDACT_EXTRA, ';'-separated) cover
// site-specific secrets not in the provider vault and not matching a built-in
// pattern (internal API tokens, DB passwords, …).
func credSecrets(store *creds.Store) []string {
	names := store.Names()
	vals := make([]string, 0, len(names))
	for _, n := range names {
		if v := store.Get(n); v != "" {
			vals = append(vals, v)
		}
	}
	vals = append(vals, extraRedactLiterals()...)
	return vals
}

// extraRedactLiterals parses AGEZT_REDACT_EXTRA into a list of additional literal
// secrets to scrub. Entries are ';'-separated and trimmed; empties are dropped.
func extraRedactLiterals() []string {
	spec := strings.TrimSpace(os.Getenv(brand.EnvPrefix + "REDACT_EXTRA"))
	if spec == "" {
		return nil
	}
	var out []string
	for _, part := range strings.Split(spec, ";") {
		if v := strings.TrimSpace(part); v != "" {
			out = append(out, v)
		}
	}
	return out
}

// buildAnomaly starts the anomaly auto-halt circuit breaker (SPEC-06 §5). It
// watches the global tool-call rate and auto-halts the kernel on a runaway
// spike — a safety backstop above the per-run loop guard (M116). On by default;
// AGEZT_ANOMALY_MAX_TOOLCALLS sets the ceiling (0 disables),
// AGEZT_ANOMALY_WINDOW the measurement window. Returns a banner description.
func buildAnomaly(ctx context.Context, k *kernelruntime.Kernel, stdout io.Writer) string {
	max := 120 // ~12 tool calls/sec sustained — only a tight loop hits this
	if v := strings.TrimSpace(os.Getenv(brand.EnvPrefix + "ANOMALY_MAX_TOOLCALLS")); v != "" {
		if n, err := strconv.Atoi(v); err == nil && n >= 0 {
			max = n
		}
	}
	window := 10 * time.Second
	if v := strings.TrimSpace(os.Getenv(brand.EnvPrefix + "ANOMALY_WINDOW")); v != "" {
		if d, err := time.ParseDuration(v); err == nil && d > 0 {
			window = d
		}
	}
	if max <= 0 {
		return "disabled (AGEZT_ANOMALY_MAX_TOOLCALLS=0)"
	}
	started := anomaly.Start(ctx, k.Bus(), anomaly.Config{MaxToolCalls: max, Window: window}, func(reason string) {
		fmt.Fprintf(stdout, "  ⚠ anomaly auto-halt engaged: %s\n", reason)
		k.HaltWith(reason)
	})
	if !started {
		return "disabled"
	}
	return fmt.Sprintf("on (halt if >%d tool calls / %s; set AGEZT_ANOMALY_MAX_TOOLCALLS=0 to disable)", max, window)
}

// buildAlertNotify starts the alert → channel notifier (M782) when
// AGEZT_ALERT_NOTIFY is on and at least one channel is configured. Knobs:
//
//	AGEZT_ALERT_NOTIFY           1/on/true enables (default off — opt-in)
//	AGEZT_ALERT_NOTIFY_LEVEL     "critical" = criticals only; default warning+
//	AGEZT_ALERT_NOTIFY_COOLDOWN  per-alert (kind+run) repeat suppression, default 5m
//	AGEZT_ALERT_NOTIFY_MAX       global cap per 10-minute window, default 12
func buildAlertNotify(ctx context.Context, k *kernelruntime.Kernel, sink pulse.BriefSink) string {
	switch strings.ToLower(strings.TrimSpace(os.Getenv(brand.EnvPrefix + "ALERT_NOTIFY"))) {
	case "1", "on", "true", "yes":
	default:
		return "disabled (set AGEZT_ALERT_NOTIFY=1 to push warning/critical alerts to channels)"
	}
	if sink == nil {
		return "enabled but NO channel configured — configure Telegram/Slack/… first"
	}
	cfg := alerter.Config{MinLevel: alerter.ParseLevel(os.Getenv(brand.EnvPrefix + "ALERT_NOTIFY_LEVEL"))}
	if v := strings.TrimSpace(os.Getenv(brand.EnvPrefix + "ALERT_NOTIFY_COOLDOWN")); v != "" {
		if d, err := time.ParseDuration(v); err == nil && d > 0 {
			cfg.Cooldown = d
		}
	}
	if v := strings.TrimSpace(os.Getenv(brand.EnvPrefix + "ALERT_NOTIFY_MAX")); v != "" {
		if n, err := strconv.Atoi(v); err == nil && n > 0 {
			cfg.MaxPerWindow = n
		}
	}
	// Mute window (M815): hold warnings during a daily quiet window (criticals
	// always break through). Reuses Pulse's "START-END" 24h form, e.g. "0-7".
	cfg.Mute = pulse.ParseQuietHours(os.Getenv(brand.EnvPrefix + "ALERT_NOTIFY_MUTE"))
	// Per-source routing (M815): drop noisy categories (run/egress/budget/
	// provider/kernel) outright while keeping the rest.
	cfg.MuteSources = alerter.ParseMuteSources(os.Getenv(brand.EnvPrefix + "ALERT_NOTIFY_MUTE_SOURCES"))
	if !alerter.Start(ctx, k.Bus(), sink, cfg) {
		return "disabled"
	}
	extra := ""
	if cfg.Mute.Enabled {
		extra += fmt.Sprintf("; muted %s (criticals still break through)", cfg.Mute.Spec())
	}
	if len(cfg.MuteSources) > 0 {
		srcs := make([]string, 0, len(cfg.MuteSources))
		for s := range cfg.MuteSources {
			srcs = append(srcs, s)
		}
		sort.Strings(srcs)
		extra += "; sources muted: " + strings.Join(srcs, ",")
	}
	return fmt.Sprintf("on (level≥%s → channels; repeats suppressed; flood-capped%s)", cfg.MinLevel, extra)
}

// standingTrustCeiling computes the effective trust ceiling for a standing-order
// firing (M999): the MORE restrictive (lower edict level) of the order's explicit
// max_trust and the level implied by its initiative mode (inform_only→L0, ask→L1).
// Returns (level, false) when neither caps — the firing runs uncapped, the
// pre-M999 default for orders with empty mode and no max_trust.
//
// Fail-safe for act_or_ask (VULN-003): an order whose mode is the explicit
// autonomous dial "act_or_ask" but which leaves max_trust blank would otherwise
// fire UNCAPPED (L4) — the most permissive setting silently meaning "no clamp",
// despite the operator having chosen a mode whose very name implies bounded
// autonomy. Because such a firing is unattended and its trigger payload can be
// attacker-influenced (VULN-004), it must not run uncapped by omission. We default
// it to L2 (ask-first), mirroring the seeded guardian-initiative responder. An
// operator who genuinely wants uncapped act_or_ask autonomy opts in explicitly by
// setting max_trust=L4. Empty/unset mode is left untouched so pre-M999 and
// non-initiative orders keep their existing behaviour.
func standingTrustCeiling(in standing.Initiative) (edict.TrustLevel, bool) {
	ceil := edict.LevelAllow
	have := false
	if lvl, err := edict.ParseTrustLevel(in.MaxTrust); err == nil {
		ceil, have = lvl, true
	}
	if modeLvl, capped := in.Mode.MaxAutonomyTrust(); capped {
		if lvl, err := edict.ParseTrustLevel(modeLvl); err == nil {
			if !have || lvl < ceil {
				ceil = lvl
			}
			have = true
		}
	}
	if !have && in.Mode == standing.InitiativeActOrAsk {
		ceil, have = edict.LevelAskFirst, true
	}
	return ceil, have
}

// buildStandingRunner starts the event-trigger half of standing orders
// (SPEC-16 §4): when a journal event matches an enabled order's event trigger,
// the order's governed plan is launched as a run (bounded by its budget ceiling)
// and a standing.fired event is journaled. Cron triggers are handled by the
// schedule engine, not here.
