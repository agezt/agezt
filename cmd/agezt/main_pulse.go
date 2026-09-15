// SPDX-License-Identifier: MIT

// Pulse/setup/transcriber helpers extracted from main.go during Day 211
// god-file refactor (#43). Public API unchanged.
package main

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"time"

	"github.com/agezt/agezt/internal/brand"
	"github.com/agezt/agezt/kernel/artifact"
	"github.com/agezt/agezt/kernel/bus"
	"github.com/agezt/agezt/kernel/creds"
	"github.com/agezt/agezt/kernel/edict"
	"github.com/agezt/agezt/kernel/event"
	"github.com/agezt/agezt/kernel/pulse"
	kernelruntime "github.com/agezt/agezt/kernel/runtime"
	"github.com/agezt/agezt/kernel/settings"
	"github.com/agezt/agezt/kernel/state"
	"github.com/agezt/agezt/kernel/stt"
	"github.com/agezt/agezt/kernel/ulid"
	"github.com/agezt/agezt/kernel/warden"
)

func startReflectTicker(ctx context.Context, k *kernelruntime.Kernel, stdout io.Writer) string {
	raw := os.Getenv(brand.EnvPrefix + "REFLECT_EVERY")
	if raw == "" {
		return ""
	}
	every, err := time.ParseDuration(raw)
	if err != nil || every <= 0 {
		fmt.Fprintf(stdout, "  reflection       : invalid AGEZT_REFLECT_EVERY %q (%v) — on-demand only\n", raw, err)
		return ""
	}
	go func() {
		ticker := time.NewTicker(every)
		defer ticker.Stop()
		for {
			select {
			case <-ctx.Done():
				return
			case <-ticker.C:
				corr := "reflect-" + ulid.New()
				if _, err := k.Reflect().Reflect(ctx, corr); err != nil {
					fmt.Fprintf(stdout, "reflection pass failed: %v\n", err)
				}
			}
		}
	}()
	return "every " + every.String()
}

func startWorkboardSweepTicker(ctx context.Context, k *kernelruntime.Kernel, stdout io.Writer) string {
	raw := os.Getenv(brand.EnvPrefix + "WORKBOARD_SWEEP_EVERY")
	if raw == "" {
		return ""
	}
	every, err := time.ParseDuration(raw)
	if err != nil || every <= 0 {
		fmt.Fprintf(stdout, "  workboard sweep  : invalid AGEZT_WORKBOARD_SWEEP_EVERY %q (%v) - on-demand only\n", raw, err)
		return ""
	}
	staleAfter := 10 * time.Minute
	if spec := strings.TrimSpace(os.Getenv(brand.EnvPrefix + "WORKBOARD_STALE_AFTER")); spec != "" {
		if parsed, perr := time.ParseDuration(spec); perr == nil && parsed > 0 {
			staleAfter = parsed
		} else {
			fmt.Fprintf(stdout, "  workboard sweep  : invalid AGEZT_WORKBOARD_STALE_AFTER %q (%v) - using %s\n", spec, perr, staleAfter)
		}
	}
	go func() {
		ticker := time.NewTicker(every)
		defer ticker.Stop()
		for {
			select {
			case <-ctx.Done():
				return
			case <-ticker.C:
				corr := "workboard-sweep-" + ulid.New()
				tasks, err := k.SweepStaleWorkboardClaims(corr, "workboard-sweeper", staleAfter, 100)
				if err != nil {
					fmt.Fprintf(stdout, "workboard sweep failed: %v\n", err)
					continue
				}
				if len(tasks) > 0 {
					fmt.Fprintf(stdout, "workboard sweep reclaimed %d stale claim(s)\n", len(tasks))
				}
			}
		}
	}()
	return "every " + every.String() + " (stale after " + staleAfter.String() + ")"
}

// startBrainDistillTicker starts a periodic brain-distillation pass when
// AGEZT_BRAIN_DISTILL_EVERY is a valid positive duration, on the daemon ctx
// (so halt/shutdown stop it). Returns a banner description, or "" when no
// timer is configured. Mirrors the reflection ticker.
func startBrainDistillTicker(ctx context.Context, k *kernelruntime.Kernel, stdout io.Writer) string {
	raw := os.Getenv(brand.EnvPrefix + "BRAIN_DISTILL_EVERY")
	if raw == "" {
		return ""
	}
	every, err := time.ParseDuration(raw)
	if err != nil || every <= 0 {
		fmt.Fprintf(stdout, "  brain distill    : invalid AGEZT_BRAIN_DISTILL_EVERY %q (%v) — on-demand only\n", raw, err)
		return ""
	}
	go func() {
		ticker := time.NewTicker(every)
		defer ticker.Stop()
		for {
			select {
			case <-ctx.Done():
				return
			case <-ticker.C:
				corr := "brain-distill-" + ulid.New()
				if _, err := k.DistillBrain(ctx, corr); err != nil {
					fmt.Fprintf(stdout, "brain-distill pass failed: %v\n", err)
				}
			}
		}
	}()
	return "every " + every.String()
}

// startProfileDistillTicker runs the operator-profile synthesis on a low daily
// cadence (M1000) when the profile feature is on, so AGEZT learns who the
// operator is without being asked. Default 24h; AGEZT_USER_PROFILE_EVERY overrides
// (operators can also schedule the profile_distill system task at a custom cadence
// or run `agt memory profile`). Returns a banner description, or "" when off.
func startProfileDistillTicker(ctx context.Context, k *kernelruntime.Kernel, on bool, stdout io.Writer) string {
	if !on {
		return ""
	}
	every := 24 * time.Hour
	if raw := strings.TrimSpace(os.Getenv(brand.EnvPrefix + "USER_PROFILE_EVERY")); raw != "" {
		if d, err := time.ParseDuration(raw); err == nil && d > 0 {
			every = d
		} else {
			fmt.Fprintf(stdout, "  user profile     : invalid AGEZT_USER_PROFILE_EVERY %q (%v) — using 24h\n", raw, err)
		}
	}
	go func() {
		ticker := time.NewTicker(every)
		defer ticker.Stop()
		for {
			select {
			case <-ctx.Done():
				return
			case <-ticker.C:
				corr := "profile-distill-" + ulid.New()
				if _, err := k.DistillProfile(ctx, corr); err != nil {
					fmt.Fprintf(stdout, "profile-distill pass failed: %v\n", err)
				}
			}
		}
	}()
	return "every " + every.String()
}

// onOff renders a boolean as a banner-friendly enabled/disabled token.
func onOff(b bool) string {
	if b {
		return "on"
	}
	return "off"
}

// replayPolicyOverlay reads the journal, decodes every policy.changed event
// (runtime deny-rule add/rm + trust-level changes, M18/M19), and projects
// them into the net overlay to restore onto the engine (M20). The journal is
// the source of truth; the engine overlay is a projection. Order is preserved
// by Range (append-only journal), which ProjectPolicyChanges relies on for
// last-wins level semantics and add/rm rule bookkeeping.
type bootStep struct {
	name  string
	run   func() (desc string, err error)
	fatal bool
}

// pulseObserverAdmin adapts the live pulse engine to controlplane.PulseObservers
// (Phase 2.6 3b-i): the daemon owns the DiskUsage func, the warden and the state
// store the observer constructors need, so the control plane stays decoupled
// from kernel/pulse. Wired only when the engine is running (pulse enabled).
type pulseObserverAdmin struct {
	eng  *pulse.Engine
	ward warden.Engine
	st   *state.FileStore
}

// AddDiskObserver registers a runtime disk-space watch (M767).
func (a pulseObserverAdmin) AddDiskObserver(path string, minPct float64) (string, bool) {
	return a.eng.AddObserver(pulse.NewDiskObserver(path, minPct, pulse.DiskUsage)), true
}

// AddProbeObserver registers a runtime command-probe watch (M768) — the command
// runs through the warden each beat, like any agent shell call.
func (a pulseObserverAdmin) AddProbeObserver(name string, argv []string) (string, bool) {
	return a.eng.AddObserver(pulse.NewProbeObserver(name, argv, a.ward, a.st)), true
}

func buildPulse(k *kernelruntime.Kernel, ward warden.Engine, model string, stdout io.Writer, extraSink pulse.BriefSink) (*pulse.Engine, string) {
	if strings.EqualFold(os.Getenv(brand.EnvPrefix+"PULSE"), "off") {
		return nil, ""
	}
	cadence := 60 * time.Second
	if v := os.Getenv(brand.EnvPrefix + "PULSE_CADENCE"); v != "" {
		if d, err := time.ParseDuration(v); err == nil && d > 0 {
			cadence = d
		}
	}
	dial := pulse.ParseDial(os.Getenv(brand.EnvPrefix + "PULSE_DIAL"))
	qh := pulse.ParseQuietHours(os.Getenv(brand.EnvPrefix + "PULSE_QUIET_HOURS"))

	var obs []pulse.Observer
	var parts []string
	if spec := os.Getenv(brand.EnvPrefix + "PULSE_PROBE"); spec != "" {
		if name, argv, ok := pulse.ParseProbeSpec(spec); ok {
			obs = append(obs, pulse.NewProbeObserver(name, argv, ward, k.State()))
			parts = append(parts, "probe:"+name)
		}
	}
	if spec := os.Getenv(brand.EnvPrefix + "PULSE_DISK"); spec != "" {
		if path, pctStr, ok := strings.Cut(spec, ":"); ok {
			if pct, err := strconv.ParseFloat(pctStr, 64); err == nil && pct > 0 {
				obs = append(obs, pulse.NewDiskObserver(path, pct, pulse.DiskUsage))
				parts = append(parts, "disk:"+path)
			}
		}
	}
	// Self-health observer (M628): the daemon watches its OWN run/tool
	// reliability and briefs the operator when its health transitions
	// (healthy↔degraded↔critical) — proactive self-monitoring, not just the
	// reactive Analyst. On by default (the whole point is to watch unprompted);
	// AGEZT_PULSE_HEALTH=off disables it, =<float> overrides the tool-error-rate
	// degrade threshold (default 0.30).
	if hv := os.Getenv(brand.EnvPrefix + "PULSE_HEALTH"); !strings.EqualFold(hv, "off") {
		degradeAt := 0.0 // observer falls back to its default
		if f, err := strconv.ParseFloat(hv, 64); err == nil && f > 0 {
			degradeAt = f
		}
		obs = append(obs, pulse.NewHealthObserver(healthStatFromJournal(k), degradeAt, 0))
		parts = append(parts, "self:health")
	}
	useLLM := strings.EqualFold(os.Getenv(brand.EnvPrefix+"PULSE_LLM"), "on")
	// Autonomy level (M999): off|ask|act, default act. `act` makes Pulse EMIT a
	// pulse.initiative.act event on actionable observations; an autonomous run still
	// requires an ENABLED standing order bound to it (the seeded responder ships
	// disabled), so a fresh install is bold-by-default yet dormant until opt-in.
	initiative := pulse.ParseInitiative(os.Getenv(brand.EnvPrefix + "PULSE_INITIATIVE"))

	eng := pulse.New(pulse.Config{
		Bus:        k.Bus(),
		State:      k.State(),
		Warden:     ward,
		Provider:   k.Provider(),
		Model:      model,
		Relevance:  k.World(), // world-model relevance signal (SPEC-05 §3.4)
		Observers:  obs,
		Dial:       dial,
		Initiative: initiative,
		Cadence:    cadence,
		QuietHours: qh,
		UseLLM:     useLLM,
		Sink:       briefSink(stdout, extraSink),
	})
	observers := "no observers configured"
	if len(parts) > 0 {
		observers = strings.Join(parts, ",")
	}
	return eng, fmt.Sprintf("dial=%s initiative=%s cadence=%s observers=[%s]", dial, initiative, cadence, observers)
}

// healthStatFromJournal returns a pulse.HealthStatFunc that samples the
// daemon's recent reliability from the tail of its own journal: tool.invoked /
// tool.result(error) for tool reliability, and task.completed / task.failed for
// run reliability. It reads only the last healthWindow events so the scan is
// cheap and the assessment reflects RECENT behaviour, not all-time history.
func healthStatFromJournal(k *kernelruntime.Kernel) pulse.HealthStatFunc {
	const healthWindow = 2000
	return func(context.Context) (pulse.HealthStat, error) {
		j := k.Journal()
		if j == nil {
			return pulse.HealthStat{}, nil
		}
		evs, err := j.Tail(healthWindow)
		if err != nil {
			return pulse.HealthStat{}, err
		}
		var st pulse.HealthStat
		for _, e := range evs {
			switch e.Kind {
			case event.KindToolInvoked:
				st.ToolCalls++
			case event.KindToolResult:
				var p struct {
					Error bool `json:"error"`
				}
				_ = json.Unmarshal(e.Payload, &p)
				if p.Error {
					st.ToolErrors++
				}
			case event.KindTaskCompleted:
				st.Runs++
			case event.KindTaskFailed:
				st.Runs++
				st.FailedRuns++
			}
		}
		return st, nil
	}
}

// wireArtifactIndexer subscribes to the bus and indexes every offloaded tool
// output (M827): a tool.result event with a raw_ref means the agent stored a
// large output in the blob store, so we add a metadata index entry pointing at
// that ref (kind=tool-output, source=run, the tool name, the run correlation).
// The file manager then lists run outputs alongside inbound images. Best-effort:
// an index failure is silently skipped — it must never disturb a run. The
// subscription lives on the daemon ctx and ends when the daemon stops.
func wireArtifactIndexer(ctx context.Context, k *kernelruntime.Kernel) {
	idx := k.ArtifactIndex()
	if idx == nil {
		return
	}
	sub, err := k.Bus().Subscribe(">", 256)
	if err != nil {
		return
	}
	go func() {
		defer sub.Cancel()
		for {
			select {
			case <-ctx.Done():
				return
			case ev, ok := <-sub.C:
				if !ok {
					return
				}
				if ev.Kind != event.KindToolResult {
					continue
				}
				var p struct {
					RawRef      string `json:"raw_ref"`
					Tool        string `json:"tool"`
					OutputBytes int64  `json:"output_bytes"`
				}
				if json.Unmarshal(ev.Payload, &p) != nil || p.RawRef == "" {
					continue
				}
				name := p.Tool
				if name == "" {
					name = "tool"
				}
				_, _ = idx.IndexRef(p.RawRef, artifact.Entry{
					Kind:   "tool-output",
					Source: "run",
					Name:   fmt.Sprintf("%s-output.txt", name),
					Mime:   "text/plain",
					Corr:   ev.CorrelationID,
					Size:   p.OutputBytes,
				}, time.Now().UnixMilli())
			}
		}
	}()
}

// netguardPublish returns the per-tool egress-block audit publisher handed to
// toolreg.KernelDeps.NetguardPublish: Set.Configure calls it once per
// netguard-guarded tool instance, and the returned callback journals a refused
// dial (SSRF / metadata attempt) as a netguard.blocked event (M109). A nil bus
// returns nil so Configure skips the wiring (harmless no-op, e.g. in tests).
func netguardPublish(b *bus.Bus) func(tool string) func(ip, reason string) {
	if b == nil {
		return nil
	}
	return func(tool string) func(ip, reason string) {
		return func(ip, reason string) {
			_, _ = b.Publish(event.Spec{
				Subject: "netguard.block",
				Kind:    event.KindNetguardBlocked,
				Actor:   tool,
				Payload: map[string]any{"ip": ip, "reason": reason, "tool": tool},
			})
		}
	}
}

// boardSubjectSlug sanitises a board topic into one subject segment (M656):
// lowercased, with any run of characters that aren't [a-z0-9_-] collapsed to a
// single dash, so "Acil Müdahale!" → "acil-m-dahale" and the event subject
// "board.<slug>" stays a single, well-formed segment a standing trigger can match.
// An empty/all-symbol topic degrades to "untopiced" so the subject is never
// "board." with a trailing dot.
func boardSubjectSlug(topic string) string {
	var b strings.Builder
	lastDash := false
	for _, r := range strings.ToLower(strings.TrimSpace(topic)) {
		if (r >= 'a' && r <= 'z') || (r >= '0' && r <= '9') || r == '_' || r == '-' {
			b.WriteRune(r)
			lastDash = false
		} else if !lastDash {
			b.WriteByte('-')
			lastDash = true
		}
	}
	s := strings.Trim(b.String(), "-")
	if s == "" {
		return "untopiced"
	}
	return s
}

// voiceTranscriberShim adapts the runtime Voice adapter (Transcribe(ctx, audio,
// filename)) to the webui.Transcriber seam (Transcribe(ctx, filename, audio)) so
// the console mic and Voice-mode STT flow through whatever provider the voice
// adapter is configured for — ElevenLabs / Deepgram included, not just OpenAI.
type voiceTranscriberShim struct{ v kernelruntime.Voice }

func (s voiceTranscriberShim) Transcribe(ctx context.Context, filename string, audio []byte) (string, error) {
	return s.v.Transcribe(ctx, audio, filename)
}

// sttTranscriberFromEnv builds the speech-to-text client from AGEZT_STT_* (or a
// fallback OPENAI_API_KEY), or returns nil when no STT endpoint is configured.
// Shared by the Web UI mic button (/api/transcribe, M689) and the OpenAI-
// compatible /v1/audio/transcriptions route — one place decides "is STT on?".
// Returns the concrete *stt.Client so callers can nil-check the pointer before
// handing it to a Set*Transcriber (avoiding a typed-nil interface).
func sttTranscriberFromEnv() *stt.Client {
	key := strings.TrimSpace(os.Getenv(brand.EnvPrefix + "STT_API_KEY"))
	if key == "" {
		key = strings.TrimSpace(os.Getenv("OPENAI_API_KEY"))
	}
	url := strings.TrimSpace(os.Getenv(brand.EnvPrefix + "STT_API_URL"))
	if key == "" && url == "" {
		return nil
	}
	return stt.New(stt.Config{
		APIURL: url,
		APIKey: key,
		Model:  strings.TrimSpace(os.Getenv(brand.EnvPrefix + "STT_MODEL")),
	})
}

// injectConfig bridges the Config Center's config store + vault into the process
// environment at startup so the existing os.Getenv consumers read operator edits
// unchanged (M693). Precedence: a value already in the real environment WINS
// (operator's .env/shell); the store/vault only fill gaps. Returns the schema env
// vars that were pinned by the real environment (computed BEFORE injection, so our
// own Setenv calls aren't mistaken for operator pins) for the Config Center to
// show read-only. AGEZT_CONFIG=off disables the bridge entirely.
func injectConfig(baseDir string, vault *creds.Store, stdout io.Writer) map[string]bool {
	pinned := map[string]bool{}
	// Pin across the FULL merged surface (built-in + registered) so a skill's
	// registered field is also marked read-only when the operator pins it in .env.
	for _, sec := range settings.NewRegistry(baseDir).Sections() {
		for _, f := range sec.Fields {
			if os.Getenv(f.Env) != "" {
				pinned[f.Env] = true
			}
		}
	}
	if strings.EqualFold(strings.TrimSpace(os.Getenv(brand.EnvPrefix+"CONFIG")), "off") {
		return pinned
	}
	store := settings.NewStore(baseDir)
	if err := store.Load(); err != nil {
		fmt.Fprintf(stdout, "  config store     : load failed (%v) — environment only\n", err)
		return pinned
	}
	injected := 0
	for name, val := range store.All() {
		if val != "" && os.Getenv(name) == "" {
			_ = os.Setenv(name, val)
			injected++
		}
	}
	// Channel/config SECRETS live in the vault under their AGEZT_* name; inject
	// those too. Provider API keys are NON-AGEZT_ and resolved via the cred chain,
	// so they need no env injection.
	for _, name := range vault.Names() {
		if strings.HasPrefix(name, brand.EnvPrefix) && os.Getenv(name) == "" {
			if v := vault.Get(name); v != "" {
				_ = os.Setenv(name, v)
				injected++
			}
		}
	}
	if injected > 0 {
		fmt.Fprintf(stdout, "  config store     : %d setting(s) applied from %s\n", injected, store.Path)
	}
	return pinned
}

// workspaceRoot resolves the directory the file and shell tools share:
// $AGEZT_WORKSPACE, or <baseDir>/workspace by default. Used by buildTools (to
// scope the tools) and by the kernel Config (to tell the model where it is via
// the M609 environment preamble), so the two never drift.
func workspaceRoot(baseDir string) string {
	if ws := os.Getenv(brand.EnvPrefix + "WORKSPACE"); ws != "" {
		return ws
	}
	return filepath.Join(baseDir, "workspace")
}

// buildTools + councilSeatName → boot_tools.go

func selectAskPolicy() (edict.AskPolicy, string) {
	switch strings.ToLower(strings.TrimSpace(os.Getenv(brand.EnvPrefix + "APPROVAL_MODE"))) {
	case "deny":
		return edict.AskDeny, "AskDeny (strict; only L4 calls run)"
	case "prompt", "ask":
		return edict.AskPrompt, "AskPrompt (live HITL via `agt approve|deny`)"
	case "", "allow":
		return edict.AskAllow, "AskAllow (Ask-class folded to Allow + WouldAsk)"
	default:
		// Unknown values fall back to the safe default; surface the
		// fact in the banner so the operator notices the typo.
		return edict.AskAllow, fmt.Sprintf("AskAllow (unknown %sAPPROVAL_MODE=%q ignored)",
			brand.EnvPrefix, os.Getenv(brand.EnvPrefix+"APPROVAL_MODE"))
	}
}

func wardenOptionsFromEnv() (warden.Options, string) {
	raw := strings.ToLower(strings.TrimSpace(os.Getenv(brand.EnvPrefix + "WARDEN_DOCKER")))
	if raw != "1" && raw != "true" && raw != "yes" && raw != "on" {
		return warden.Options{}, ""
	}
	runtimeName := strings.TrimSpace(os.Getenv(brand.EnvPrefix + "WARDEN_DOCKER_RUNTIME"))
	if runtimeName == "" {
		runtimeName = "docker"
	}
	image := strings.TrimSpace(os.Getenv(brand.EnvPrefix + "WARDEN_DOCKER_IMAGE"))
	if image == "" {
		image = "python:3.12-slim"
	}
	network := strings.TrimSpace(os.Getenv(brand.EnvPrefix + "WARDEN_DOCKER_NETWORK"))
	if network == "" {
		network = "none"
	}
	return warden.Options{
		Container: warden.ContainerOptions{
			Enabled: true,
			Runtime: runtimeName,
			Image:   image,
			Network: network,
		},
	}, fmt.Sprintf("; container=%s image=%s network=%s", runtimeName, image, network)
}

func selectAutoApproveCapabilities() (map[string]bool, string) {
	raw := strings.TrimSpace(os.Getenv(brand.EnvPrefix + "AUTO_APPROVE_CAPS"))
	switch strings.ToLower(raw) {
	case "off", "0", "false", "no", "none":
		return nil, "off (set " + brand.EnvPrefix + "AUTO_APPROVE_CAPS=all or a comma list)"
	case "", "all", "1", "true", "yes", "on":
		caps := map[string]bool{}
		for _, c := range edict.AllCapabilities() {
			caps[string(c)] = true
		}
		return caps, fmt.Sprintf("on (%d known capabilities; hard-deny/SSRF/budget guards still apply)", len(caps))
	default:
		caps := map[string]bool{}
		var unknown []string
		for _, item := range splitNonEmpty(raw) {
			if edict.KnownCapability(item) {
				caps[item] = true
			} else {
				unknown = append(unknown, item)
			}
		}
		if len(caps) == 0 {
			return nil, fmt.Sprintf("off (no known capabilities in %sAUTO_APPROVE_CAPS=%q)", brand.EnvPrefix, raw)
		}
		desc := fmt.Sprintf("on (%d selected capabilities)", len(caps))
		if len(unknown) > 0 {
			desc += fmt.Sprintf("; ignored unknown: %s", strings.Join(unknown, ", "))
		}
		return caps, desc
	}
}

// keep import honest
var _ = event.GenesisHash
