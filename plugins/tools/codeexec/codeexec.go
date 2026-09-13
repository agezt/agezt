// SPDX-License-Identifier: MIT

// Package codeexec is the in-process code-execution tool: it lets the agent
// WRITE a real program (Python, JavaScript/Node, or TypeScript/Deno), RUN it,
// see the output, and ITERATE — the "build whatever's needed" primitive
// (M683). Compute, scraping, data wrangling, one-off scripts and multi-call
// projects all go through here.
//
// Execution is delegated to the kernel/warden Engine (the same path the shell
// tool uses) so timeout, output truncation, exit-code propagation, working-
// directory scoping, best-effort Linux resource limits, and audit events are
// handled in one place. On top of that this tool adds, for EVERY run:
//
//   - a per-call ephemeral scratch dir (or a named persistent project dir) under
//     <baseDir>/sandbox, so one run can't see or clobber another agent's work;
//   - a SCRUBBED environment — the daemon's secrets (API keys, provider creds,
//     the whole AGEZT_* namespace) are never forwarded into model-written code;
//   - for Deno, an OS-level filesystem jail confined to the work dir (real on
//     every platform, Windows included), with network granted by default;
//   - honest reporting of the EFFECTIVE isolation profile (Python/Node get the
//     warden's profile — real on Linux+namespace, workdir/env/limits-only
//     elsewhere; the result and events never overstate containment).
//
// SECURITY NOTE: running arbitrary code is a high-blast-radius capability. It is
// gated by the `code.exec` Edict capability and every run is journaled
// (code.executed + warden.exec) so the operator can see, govern, and revert.
package codeexec

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/agezt/agezt/kernel/agent"
	"github.com/agezt/agezt/kernel/artifact"
	"github.com/agezt/agezt/kernel/bus"
	"github.com/agezt/agezt/kernel/edict"
	"github.com/agezt/agezt/kernel/executionprofile"
	"github.com/agezt/agezt/kernel/warden"
)

const (
	// DefaultTimeout caps one run when the model omits timeout_ms.
	DefaultTimeout = 120 * time.Second
	// MaxTimeout is the hard ceiling — a model can ask for less, never more.
	MaxTimeout = 600 * time.Second
	// MaxOutputBytes caps captured stdout+stderr (matches warden's own default).
	MaxOutputBytes = 256 * 1024

	// Best-effort resource caps (real teeth only on Linux+ProfileNamespace; a
	// harmless no-op elsewhere). They guard against an accidental runaway pegging
	// the host, not against a determined escape.
	limitCPUSeconds           = 120
	limitAddressSpaceByte     = 2 << 30   // 2 GiB — Go-runtime children need ≥1 GiB; Python/Node fit
	limitMaxOpenFiles         = 512       //
	limitMaxFileSizeBytes     = 256 << 20 // 256 MiB per file
	modalArtifactArchiveBytes = 8 << 20
)

// Tool implements agent.Tool. Construct with New (tests) or NewWithWarden
// (production); Bind wires the bus so each run journals a code.executed event.
type Tool struct {
	// Warden is the isolation engine code runs through. Nil → a no-bus engine.
	Warden warden.Engine
	// SandboxRoot is <baseDir>/sandbox: ephemeral runs land in run-* tempdirs
	// here, named projects in <SandboxRoot>/projects/<slug>.
	SandboxRoot string
	// BaseDir points at the AGEZT home/base dir so profile-specific vault-backed
	// secret file mounts can resolve creds.json.
	BaseDir string
	// Runtimes maps a language id to its resolved interpreter absolute path. Only
	// languages present here can run.
	Runtimes map[string]string
	// NetEnabled is the master network switch (AGEZT_SANDBOX_NO_NET=1 → false);
	// when false, no run gets network regardless of allow_net.
	NetEnabled bool
	// Profile is the isolation profile requested of the warden (default
	// ProfileNamespace, downgraded + journaled where unavailable).
	Profile warden.Profile
	// Now returns wall-clock millis for exported artifact metadata.
	Now func() int64

	bus   *bus.Bus
	index artifactIndexer
}

// artifactIndexer is the slice of *artifact.Index code_exec needs for files a
// script intentionally exports under .agezt-artifacts/.
type artifactIndexer interface {
	PutEntry(meta artifact.Entry, data []byte, createdMs int64) (artifact.Entry, error)
}

// NewWithWarden returns a Tool routed through the supplied warden engine — the
// path the daemon uses so audit events land on the kernel bus.
func NewWithWarden(w warden.Engine, sandboxRoot string, runtimes map[string]string, netEnabled bool) *Tool {
	return &Tool{
		Warden:      w,
		SandboxRoot: sandboxRoot,
		BaseDir:     filepath.Dir(sandboxRoot),
		Runtimes:    runtimes,
		NetEnabled:  netEnabled,
		Profile:     warden.ProfileNamespace,
	}
}

// Bind wires the live bus so each run publishes a code.executed event. Called
// once after the kernel opens.
func (t *Tool) Bind(b *bus.Bus) { t.bus = b }

// SetIndex injects the artifact index so code can export files by writing them
// under .agezt-artifacts/ in the workspace.
func (t *Tool) SetIndex(idx artifactIndexer) { t.index = idx }

// Languages returns the available language ids (sorted) — for the daemon banner.
func (t *Tool) Languages() []string { return sortedLangs(t.Runtimes) }

// Definition implements agent.Tool. The language enum and description reflect
// exactly the runtimes detected on this host.
func (t *Tool) Definition() agent.ToolDef {
	langs := sortedLangs(t.Runtimes)
	enum, _ := json.Marshal(langs)
	netLine := "Network is ON by default"
	if !t.NetEnabled {
		netLine = "Network is DISABLED on this daemon"
	}
	desc := "Write and run code, then read its output. Languages: " + strings.Join(langs, ", ") +
		". Each call runs in its own scratch directory; pass a `project` name to keep a " +
		"persistent directory you can revisit and extend across calls (write more files, re-run). " +
		"Use `files` to drop extra source files alongside the entrypoint, and `packages` to pip-install " +
		"Python dependencies (they persist in a project). Save files you want to keep under `.agezt-artifacts/`; " +
		"they are copied into the artifact store when artifact storage is available. " + netLine +
		". The daemon's secrets are never visible to your code. Good for computation, scraping, " +
		"data processing, and building small programs. Returns combined stdout+stderr (truncated to 256 KiB)."

	schema := `{
  "type": "object",
  "required": ["language", "code"],
  "properties": {
    "language": {"type":"string", "enum": ` + string(enum) + `, "description":"Which runtime to use."},
    "code": {"type":"string", "description":"The program source to run (the entrypoint)."},
    "stdin": {"type":"string", "description":"Optional input; written to stdin.txt in the working dir for your code to read."},
    "project": {"type":"string", "description":"Optional persistent project name. Reuse the same name across calls to keep and extend a working directory."},
    "files": {"type":"object", "description":"Optional extra files to write before running, as {\"relative/name\": \"content\"}."},
    "packages": {"type":"array", "items":{"type":"string"}, "description":"Python only: pip packages to install before running, e.g. [\"requests\",\"beautifulsoup4\"]. In a project they persist across calls. For Deno/JS import npm packages inline instead: import x from \"npm:cheerio\"."},
    "timeout_ms": {"type":"integer", "description":"Per-call timeout in ms (default 120000, max 600000)."},
    "allow_net": {"type":"boolean", "description":"Deno only: grant network (default true). Ignored if the daemon has network disabled."}
  }
}`

	return agent.ToolDef{
		Name:        "code_exec",
		Capability:  agent.ToolCapability{Name: string(edict.CapCodeExec)},
		Description: desc,
		Effect: agent.ToolEffect{
			Class: agent.EffectIrreversible,
			PredictedEffects: []string{
				"write and execute model-provided code in the sandbox workspace",
				"may create project files, install packages, consume compute, and contact the network when enabled",
			},
			AffectedResources: []string{"sandbox root: " + t.SandboxRoot, "available runtimes: " + strings.Join(langs, ", ")},
			RollbackNotes:     "Ephemeral scratch runs are discarded after use. Persistent project files/packages must be deleted from the sandbox project or restored from a clean workspace.",
			Confidence:        0.55,
		},
		InputSchema: json.RawMessage(schema),
	}
}

type input struct {
	Language  string            `json:"language"`
	Code      string            `json:"code"`
	Stdin     string            `json:"stdin,omitempty"`
	Project   string            `json:"project,omitempty"`
	Files     map[string]string `json:"files,omitempty"`
	Packages  []string          `json:"packages,omitempty"`
	TimeoutMS int64             `json:"timeout_ms,omitempty"`
	AllowNet  *bool             `json:"allow_net,omitempty"`
}

// Invoke implements agent.Tool.
func (t *Tool) Invoke(ctx context.Context, raw json.RawMessage) (agent.Result, error) {
	var in input
	if err := json.Unmarshal(raw, &in); err != nil {
		return agent.Result{}, fmt.Errorf("code_exec: parse input: %w", err)
	}
	lang := strings.TrimSpace(in.Language)
	interp, ok := t.Runtimes[lang]
	if !ok {
		avail := strings.Join(sortedLangs(t.Runtimes), ", ")
		if avail == "" {
			avail = "(none installed)"
		}
		return errResult(fmt.Sprintf("language %q is not available; installed: %s", in.Language, avail)), nil
	}
	if strings.TrimSpace(in.Code) == "" {
		return errResult("code is required"), nil
	}
	sshCfg, sshMode := executionprofile.SSHOverrideFrom(ctx)
	k8sCfg, k8sMode := executionprofile.K8sOverrideFrom(ctx)
	modalCfg, modalMode := executionprofile.ModalOverrideFrom(ctx)
	daytonaCfg, daytonaMode := executionprofile.DaytonaOverrideFrom(ctx)

	// Network: default on for Deno; honored only when the daemon allows it.
	allowNet := true
	if in.AllowNet != nil {
		allowNet = *in.AllowNet
	}
	allowNet = allowNet && t.NetEnabled

	// Resolve the work directory: ephemeral (removed after) or a kept project dir.
	dir, ephemeral, projectSlug, err := t.workDir(in.Project)
	if err != nil {
		return errResult("prepare workspace: " + err.Error()), nil
	}
	if ephemeral {
		defer os.RemoveAll(dir)
	}

	// Write the entrypoint, any extra files, and optional stdin.
	entry := entryName(lang)
	if err := os.WriteFile(filepath.Join(dir, entry), []byte(in.Code), 0o600); err != nil {
		return errResult("write entrypoint: " + err.Error()), nil
	}
	for name, content := range in.Files {
		rel, ok := sanitizeRelFile(name)
		if !ok {
			return errResult(fmt.Sprintf("illegal file name %q (must be a relative path inside the workspace)", name)), nil
		}
		full := filepath.Join(dir, filepath.FromSlash(rel))
		if err := os.MkdirAll(filepath.Dir(full), 0o700); err != nil {
			return errResult("create dir for " + rel + ": " + err.Error()), nil
		}
		if err := os.WriteFile(full, []byte(content), 0o600); err != nil {
			return errResult("write " + rel + ": " + err.Error()), nil
		}
	}
	if in.Stdin != "" {
		_ = os.WriteFile(filepath.Join(dir, "stdin.txt"), []byte(in.Stdin), 0o600)
	}

	profile := t.Profile
	if profile == "" {
		profile = warden.ProfileNamespace
	}
	if override, ok := warden.ProfileOverrideFrom(ctx); ok {
		profile = override
	}
	w := t.Warden
	if w == nil {
		w = warden.New(nil)
	}
	// Key the credential bucket off what will ACTUALLY run, not what we asked
	// for (RCE-001). The requested profile defaults to ProfileNamespace, but
	// resolveEffectiveProfile returns ProfileNone on every non-Linux host — so
	// keying on the request handed secrets an operator had scoped to the isolated
	// "warden" tier straight into an un-isolated child. Windows and macOS hit
	// that on the DEFAULT path. See the matching comment in plugins/tools/shell.
	profileID := executionprofile.ProfileIDForWardenProfile(w.EffectiveProfile(profile))

	timeout := resolveTimeout(in.TimeoutMS)
	if sshMode {
		return t.invokeSSH(ctx, sshCfg, w, lang, interp, entry, dir, ephemeral, projectSlug, in.Packages, allowNet, timeout, len(in.Code)), nil
	}
	if k8sMode {
		return t.invokeK8s(ctx, k8sCfg, w, lang, interp, entry, dir, ephemeral, projectSlug, in.Packages, allowNet, timeout, len(in.Code)), nil
	}
	if modalMode {
		return t.invokeModal(ctx, modalCfg, w, lang, interp, entry, dir, projectSlug, in.Packages, allowNet, timeout, len(in.Code)), nil
	}
	if daytonaMode {
		return t.invokeDaytona(ctx, daytonaCfg, w, lang, interp, entry, dir, ephemeral, projectSlug, in.Packages, allowNet, timeout, len(in.Code)), nil
	}

	// Install dependencies before running (Python only). They land in <dir>/.deps,
	// so a project's installs persist across calls and an ephemeral run's are
	// discarded with it. A failed install short-circuits — we never run code
	// against a half-installed environment.
	env := executionprofile.AppendEnvPassthrough(scrubEnv(dir), profileID)
	secretEnv, cleanupSecrets, _, serr := executionprofile.PrepareSecretFileMounts(t.BaseDir, profileID, dir)
	if serr != nil {
		return errResult("secret file mounts: " + serr.Error()), nil
	}
	defer cleanupSecrets()
	env = append(env, secretEnv...)
	depsDir := filepath.Join(dir, pyDepsName)
	if len(in.Packages) > 0 {
		if lang != LangPython {
			return errResult(`packages are only supported for python; for deno/JS, import npm packages inline instead, e.g. import x from "npm:cheerio"`), nil
		}
		if !t.NetEnabled {
			return errResult("cannot install packages: network is disabled on this daemon (AGEZT_SANDBOX_NO_NET=1)"), nil
		}
		pkgs, perr := validatePackages(in.Packages)
		if perr != nil {
			return errResult(perr.Error()), nil
		}
		if len(pkgs) > 0 {
			ires, ierr := pipInstall(ctx, w, interp, dir, depsDir, pkgs, profile, env)
			if ierr != nil {
				return errResult("pip install failed: " + ierr.Error()), nil
			}
			if ires.ExitCode != 0 {
				return errResult(fmt.Sprintf("pip install failed (exit %d):\n%s", ires.ExitCode, installTail(ires))), nil
			}
		}
	}

	// The program's environment: scrubbed, plus PYTHONPATH pointing at any
	// installed deps (present whenever a project — or this call — installed
	// packages), so `import requests` resolves.
	if lang == LangPython {
		if _, serr := os.Stat(depsDir); serr == nil {
			env = append(env, "PYTHONPATH="+depsDir)
		}
	}

	res, err := w.Run(ctx, warden.Spec{
		Profile: profile,
		Argv:    buildArgv(interp, lang, entry, dir, allowNet),
		WorkDir: dir,
		Env:     env,
		Limits: warden.Limits{
			Timeout:           timeout,
			MaxOutputBytes:    MaxOutputBytes,
			CPUSeconds:        limitCPUSeconds,
			AddressSpaceBytes: limitAddressSpaceByte,
			MaxOpenFiles:      limitMaxOpenFiles,
			MaxFileSizeBytes:  limitMaxFileSizeBytes,
		},
		Actor:         "tool.code_exec",
		CorrelationID: warden.CorrelationFrom(ctx),
	})
	if err != nil {
		return errResult(fmt.Sprintf("run failed: %v", err)), nil
	}

	t.publish(ctx, lang, projectSlug, len(in.Code), allowNet, res)
	artifacts, artifactErr := t.exportArtifactsFromDir(ctx, dir, "local")
	return appendArtifactExport(render(lang, projectSlug, dir, ephemeral, timeout, res), "local", artifacts, artifactErr), nil
}

// RunScript executes a stored script once in the sandbox: an ephemeral work
// dir, the daemon's scrubbed env, and inputJSON surfaced to the script as
// ./stdin.txt. It is the execution backend of the script-tool forge (M794) —
// structurally satisfying the kernel's toolforge.Runner contract without the
// kernel importing this plugin. isError mirrors the tool's own verdict
// (unavailable language, non-zero exit, timeout), so a failed forged call
// reads exactly like a failed code_exec call.
