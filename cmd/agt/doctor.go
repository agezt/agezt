// SPDX-License-Identifier: MIT

package main

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/agezt/agezt/internal/brand"
	"github.com/agezt/agezt/internal/paths"
	"github.com/agezt/agezt/kernel/catalog"
	"github.com/agezt/agezt/kernel/controlplane"
)

// cmdDoctor implements `agt doctor` — the zero-config-first-run preflight
// (ROADMAP §2.1 "always-on essential"; SPEC-08 §3.3 version-skew check).
//
// It runs a checklist and prints each result as OK / WARN / FAIL with a
// remediation hint. Local checks (base dir) always run; daemon checks
// (reachability, version skew, journal integrity, tools, halt state) run when
// the daemon is up, and collapse to a single FAIL on "daemon" when it isn't —
// so `agt doctor` is the first thing to run when something feels wrong, and it
// degrades honestly rather than erroring out.
//
// Exit: 0 when nothing FAILed (warnings don't fail — they're advisories); 1
// when any check FAILed; 2 on bad args. `--json` emits the machine form for CI.
//
// Reuses existing surfaces only: paths.BaseDir, controlplane.NewClient/Call,
// CmdStatus, CmdJournalVerify. No new control-plane command, no new event kind.
func cmdDoctor(args []string, stdout, stderr io.Writer) int {
	asJSON := false
	for _, a := range args {
		switch a {
		case "--json":
			asJSON = true
		case "-h", "--help":
			fmt.Fprintf(stdout, "usage: %s doctor [--json]\n", brand.CLI)
			fmt.Fprintf(stdout, "preflight health check: base dir, daemon, version skew, journal, tools\n")
			return 0
		default:
			fmt.Fprintf(stderr, "%s doctor: unexpected arg %q\n", brand.CLI, a)
			return 2
		}
	}

	checks := runDoctorChecks()

	if asJSON {
		return renderDoctorJSON(checks, stdout)
	}
	return renderDoctorText(checks, stdout)
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
func runDoctorChecks() []doctorCheck {
	var checks []doctorCheck

	base, baseErr := paths.BaseDir()
	checks = append(checks, checkBaseDir(base, baseErr))

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
	checks = append(checks, checkHalt(status))

	return checks
}

// checkModelReadiness reports whether the daemon's configured model is fit
// for the tool-driven agent loop, surfacing the same catalog.Model.AgentWarnings
// as `agt provider check --caps` / the boot advisory (M23–M25), now inside the
// operator's go-to diagnostic. Conservative: WARN only on a known capability
// gap; an offline/mock model or a model the catalog doesn't list is an
// informational OK (capabilities unknown), never a FAIL.
func checkModelReadiness(status map[string]any, cat *catalog.Catalog) doctorCheck {
	const name = "model readiness"
	model, _ := status["model"].(string)
	if model == "" || model == "mock" {
		return ok(name, "offline/mock model (no catalog capabilities to assess)")
	}
	if cat == nil {
		return ok(name, fmt.Sprintf("%s (catalog not synced — capabilities unknown)", model))
	}
	_, m := cat.FindModel(model)
	if m == nil {
		return ok(name, fmt.Sprintf("%s (not in catalog — capabilities unknown)", model))
	}
	if w := m.AgentWarnings(); len(w) > 0 {
		return warn(name, fmt.Sprintf("%s — %s", model, strings.Join(w, "; ")),
			"pick a tool-capable model (AGEZT_MODEL) or set AGEZT_MODEL_STRICT=on to fail fast")
	}
	return ok(name, model+" (agent-ready: advertises tool-use)")
}

func checkBaseDir(base string, baseErr error) doctorCheck {
	const name = "base directory"
	if baseErr != nil {
		return fail(name, baseErr.Error(), "set AGEZT_HOME or check filesystem permissions")
	}
	info, err := os.Stat(base)
	if os.IsNotExist(err) {
		return warn(name, base+" (not created yet)",
			fmt.Sprintf("run `%s` once to initialise it", brand.Binary))
	}
	if err != nil {
		return fail(name, err.Error(), "check filesystem permissions")
	}
	if !info.IsDir() {
		return fail(name, base+" exists but is not a directory", "remove the file or set AGEZT_HOME")
	}
	// Prove writability rather than guessing from mode bits.
	probe := filepath.Join(base, ".doctor-probe")
	if werr := os.WriteFile(probe, []byte("ok"), 0o600); werr != nil {
		return fail(name, base+" (not writable: "+werr.Error()+")", "fix ownership/permissions on the base dir")
	}
	_ = os.Remove(probe)
	return ok(name, base+" (writable)")
}

func checkVersionSkew(status map[string]any) doctorCheck {
	const name = "version skew"
	daemonVer, _ := status["daemon"].(string)
	daemonProto := intOfStatus(status["protocol"])
	if daemonVer == brand.Version && daemonProto == int64(brand.ProtocolVersion) {
		return ok(name, fmt.Sprintf("client and daemon aligned (%s, protocol v%d)", brand.Version, brand.ProtocolVersion))
	}
	return warn(name,
		fmt.Sprintf("client %s/v%d vs daemon %s/v%d", brand.Version, brand.ProtocolVersion, daemonVer, daemonProto),
		fmt.Sprintf("restart the daemon to align (`%s shutdown` then `%s`)", brand.CLI, brand.Binary))
}

func checkJournal(ctx context.Context, client *controlplane.Client, status map[string]any) doctorCheck {
	const name = "journal"
	if _, err := client.Call(ctx, controlplane.CmdJournalVerify, nil); err != nil {
		return fail(name, "hash chain verification failed: "+err.Error(),
			"the audit log may be tampered or truncated — investigate before trusting it")
	}
	head := intOfStatus(status["journal_head"])
	return ok(name, fmt.Sprintf("BLAKE3 hash chain verified (head seq=%d)", head))
}

func checkTools(status map[string]any) doctorCheck {
	const name = "tools"
	n := intOfStatus(status["tools"])
	if n == 0 {
		return warn(name, "0 registered", "no capabilities available — check tool plugins / AGEZT_TOOLS")
	}
	return ok(name, fmt.Sprintf("%d registered", n))
}

func checkHalt(status map[string]any) doctorCheck {
	const name = "halt state"
	if halted, _ := status["halted"].(bool); halted {
		return warn(name, "system is HALTED", fmt.Sprintf("resume work with `%s resume`", brand.CLI))
	}
	return ok(name, "running")
}

func renderDoctorText(checks []doctorCheck, stdout io.Writer) int {
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
	if worst == statusFail {
		return 1
	}
	return 0
}

func renderDoctorJSON(checks []doctorCheck, stdout io.Writer) int {
	worst := statusOK
	for _, c := range checks {
		if c.Status > worst {
			worst = c.Status
		}
	}
	out := map[string]any{
		"checks":  checks,
		"healthy": worst != statusFail,
		"worst":   worst.label(),
	}
	enc := json.NewEncoder(stdout)
	enc.SetIndent("", "  ")
	_ = enc.Encode(out)
	if worst == statusFail {
		return 1
	}
	return 0
}
