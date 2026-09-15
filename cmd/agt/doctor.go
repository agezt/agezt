// SPDX-License-Identifier: MIT
//
// cmd/agt doctor entry: cmdDoctor dispatcher + types (doctorOptions,
// checkStatus, doctorCheck) + helpers (ok, warn, fail, doctorExitCode,
// label) + runDoctorChecks orchestrator + renderDoctorText +
// renderDoctorJSON. Split from doctor.go during Day 211 god-file
// refactor (#40). Public API unchanged.
package main

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"os"
	"strings"
	"time"

	"github.com/agezt/agezt/internal/brand"
	"github.com/agezt/agezt/internal/paths"
	"github.com/agezt/agezt/kernel/controlplane"
	"github.com/agezt/agezt/kernel/meshctx"
	"github.com/agezt/agezt/plugins/tools/peer"
)

func cmdDoctor(args []string, stdout, stderr io.Writer) int {
	asJSON := false
	strict := false
	repair := false
	for _, a := range args {
		switch a {
		case "--json":
			asJSON = true
		case "--strict":
			strict = true
		case "--repair":
			repair = true
		case "-h", "--help":
			fmt.Fprintf(stdout, "usage: %s doctor [--json] [--strict] [--repair]\n", brand.CLI)
			fmt.Fprintf(stdout, "preflight health check: base dir, memory store, daemon, version skew, journal, tools\n")
			fmt.Fprintf(stdout, "  --strict  exit non-zero on warnings too (not just failures)\n")
			fmt.Fprintf(stdout, "  --repair  repair startup-blocking local store files and pause noisy schedules\n")
			return 0
		default:
			fmt.Fprintf(stderr, "%s doctor: unexpected arg %q\n", brand.CLI, a)
			return 2
		}
	}

	checks := runDoctorChecks(doctorOptions{Repair: repair})

	if asJSON {
		return renderDoctorJSON(checks, strict, stdout)
	}
	return renderDoctorText(checks, strict, stdout)
}

type doctorOptions struct {
	Repair bool
}

// doctorExitCode maps the worst check status to a process exit code. A FAIL is
// always non-zero; a WARN is non-zero only under --strict (warnings are advisories
// by default, but a monitoring/CI caller can opt into treating them as actionable).
func doctorExitCode(worst checkStatus, strict bool) int {
	if worst == statusFail || (strict && worst == statusWarn) {
		return 1
	}
	return 0
}

// checkStatus is a tri-state result. Order matters: worst wins in a summary.
type checkStatus int

const (
	statusOK checkStatus = iota
	statusWarn
	statusFail
)

func (s checkStatus) label() string {
	switch s {
	case statusWarn:
		return "WARN"
	case statusFail:
		return "FAIL"
	default:
		return "OK"
	}
}

// doctorCheck is one line of the report.
type doctorCheck struct {
	Name   string      `json:"name"`
	Status checkStatus `json:"-"`
	State  string      `json:"status"` // string form for JSON ("OK"/"WARN"/"FAIL")
	Detail string      `json:"detail"`
	Hint   string      `json:"hint,omitempty"`
}

func ok(name, detail string) doctorCheck {
	return doctorCheck{Name: name, Status: statusOK, State: "OK", Detail: detail}
}
func warn(name, detail, hint string) doctorCheck {
	return doctorCheck{Name: name, Status: statusWarn, State: "WARN", Detail: detail, Hint: hint}
}
func fail(name, detail, hint string) doctorCheck {
	return doctorCheck{Name: name, Status: statusFail, State: "FAIL", Detail: detail, Hint: hint}
}

// runDoctorChecks performs the diagnostics and returns them in display order.

func runDoctorChecks(opts doctorOptions) []doctorCheck {
	var checks []doctorCheck

	base, baseErr := paths.BaseDir()
	checks = append(checks, checkBaseDir(base, baseErr))
	if baseErr == nil {
		checks = append(checks, checkMemoryStoreFile(base, opts.Repair))
	}

	// Daemon-dependent checks need a client. If we can't build one (no
	// addr/token files), the daemon isn't running — report that one FAIL and
	// skip the rest (they'd all just say "daemon unreachable").
	if baseErr != nil {
		checks = append(checks, fail("daemon", "cannot resolve base dir", "fix the base dir error above"))
		return checks
	}
	// Probe the recorded control-plane address. This surfaces *which* daemon
	// the CLI reaches (so a stray second instance is visible) and tells a
	// stale socket (recorded, dead) apart from no socket at all.
	addr, alive := controlplane.ProbeExisting(base)
	switch {
	case addr == "":
		checks = append(checks, fail("daemon", "not running (no control-plane socket recorded)",
			fmt.Sprintf("start it: %s", brand.Binary)))
		return checks
	case !alive:
		checks = append(checks, fail("daemon", "recorded at "+addr+" but not responding (stale socket)",
			fmt.Sprintf("a daemon crashed or was killed; start a fresh one: %s", brand.Binary)))
		return checks
	}

	client, err := controlplane.NewClient(base)
	if err != nil {
		checks = append(checks, fail("daemon", "socket recorded but client build failed: "+err.Error(),
			fmt.Sprintf("start it: %s", brand.Binary)))
		return checks
	}
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	status, err := client.Call(ctx, controlplane.CmdStatus, nil)
	if err != nil {
		checks = append(checks, fail("daemon", "control-plane call failed: "+err.Error(),
			fmt.Sprintf("is the daemon healthy? try `%s status`", brand.CLI)))
		return checks
	}
	checks = append(checks, ok("daemon", "running at "+addr))
	checks = append(checks, checkVersionSkew(status))
	checks = append(checks, checkJournal(ctx, client, status))
	checks = append(checks, checkTools(status))
	// Model readiness (M26): is the running model fit for the tool-driven
	// agent loop? Best-effort — the catalog is read from disk (the same one
	// the daemon loaded); a missing catalog or unlisted model yields an
	// informational OK, never a false alarm.
	cat, _ := loadCatalogIfAny(io.Discard)
	checks = append(checks, checkModelReadiness(status, cat))
	checks = append(checks, checkSandbox(ctx, client))
	checks = append(checks, checkProvider(ctx, client))
	checks = append(checks, checkApprovals(ctx, client))
	checks = append(checks, checkBudget(ctx, client))
	checks = append(checks, checkCatalog(ctx, client))
	checks = append(checks, checkWebhooks(ctx, client))
	checks = append(checks, checkSchedules(ctx, client, status, opts.Repair))
	checks = append(checks, checkStanding(ctx, client, opts.Repair))
	checks = append(checks, checkAgentHealth(ctx, client))
	checks = append(checks, checkGuardianNoise(ctx, client, opts.Repair))
	checks = append(checks, checkDisk(ctx, client))
	checks = append(checks, checkExposure(status))
	checks = append(checks, checkNetguard(ctx, client))
	checks = append(checks, checkRateLimit(ctx, client))
	checks = append(checks, checkChannels(status))
	checks = append(checks, checkCredentials(status))
	checks = append(checks, checkPlugins())
	checks = append(checks, checkMesh())
	// Mesh auth posture (M214): only when peers are configured. Flags a peer reached
	// without a token — an unauthenticated cross-node delegation.
	if peers, err := peer.ParsePeers(os.Getenv(brand.EnvPrefix + "PEERS")); err == nil && len(peers) > 0 {
		checks = append(checks, checkMeshAuth(peers))
	}
	// Mesh hop-limit config (M213): only surfaced when AGEZT_MESH_MAX_HOPS is set, so
	// single-node operators see no noise. Flags a typo that would silently fall back.
	if _, raw, _ := meshctx.MaxHopsConfig(); raw != "" {
		checks = append(checks, checkMeshHopLimit())
	}
	// Mesh loop-guard activity (M226): surfaced only when the local node has
	// actually refused a delegation loop, so healthy/single-node output stays
	// quiet. A non-zero count is a real signal worth investigating.
	if c, show := checkMeshLoops(ctx, client); show {
		checks = append(checks, c)
	}
	// Provider fallbacks (M280): a primary provider that errors on every request
	// is masked by the always-on mock fallback — invisible without a journal dig
	// (this is how the M279 dotted-tool-name 400 hid). Surface it here so a
	// silently-degraded provider is caught at preflight.
	if c, show := checkProviderFallbacks(ctx, client); show {
		checks = append(checks, c)
	}
	// Per-tenant peer overrides (M227): only when AGEZT_TENANT_PEERS is set —
	// validates the spec the daemon hard-fails on, so a typo is caught here
	// rather than by a daemon that won't restart.
	if c, show := checkTenantPeers(strings.TrimSpace(os.Getenv(brand.EnvPrefix + "TENANT_PEERS"))); show {
		checks = append(checks, c)
	}
	checks = append(checks, checkHalt(status))

	return checks
}

func renderDoctorText(checks []doctorCheck, strict bool, stdout io.Writer) int {
	worst := statusOK
	var nOK, nWarn, nFail int
	fmt.Fprintf(stdout, "%s doctor:\n", brand.CLI)
	for _, c := range checks {
		fmt.Fprintf(stdout, "  [%-4s] %-16s : %s\n", c.Status.label(), c.Name, c.Detail)
		if c.Hint != "" && c.Status != statusOK {
			fmt.Fprintf(stdout, "           ↳ %s\n", c.Hint)
		}
		switch c.Status {
		case statusOK:
			nOK++
		case statusWarn:
			nWarn++
		case statusFail:
			nFail++
		}
		if c.Status > worst {
			worst = c.Status
		}
	}
	fmt.Fprintf(stdout, "\nsummary: %d ok, %d %s, %d failed\n",
		nOK, nWarn, plural(nWarn, "warning", "warnings"), nFail)
	// Under --strict, point out that the warnings are what produced the non-zero
	// exit, so the operator isn't left wondering why a warning-only run "failed".
	if strict && worst == statusWarn {
		fmt.Fprintf(stdout, "strict: warnings treated as failures (exit 1)\n")
	}
	return doctorExitCode(worst, strict)
}

func renderDoctorJSON(checks []doctorCheck, strict bool, stdout io.Writer) int {
	worst := statusOK
	for _, c := range checks {
		if c.Status > worst {
			worst = c.Status
		}
	}
	exit := doctorExitCode(worst, strict)
	out := map[string]any{
		"checks":  checks,
		"healthy": worst != statusFail,
		"worst":   worst.label(),
		"strict":  strict,
		// ok reflects the exit verdict under the chosen mode: false when this run
		// will exit non-zero (a FAIL, or a WARN under --strict).
		"ok": exit == 0,
	}
	enc := json.NewEncoder(stdout)
	enc.SetIndent("", "  ")
	_ = enc.Encode(out)
	return exit
}
