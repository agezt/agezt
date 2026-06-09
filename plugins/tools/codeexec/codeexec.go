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
	"github.com/agezt/agezt/kernel/bus"
	"github.com/agezt/agezt/kernel/event"
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
	limitCPUSeconds       = 120
	limitAddressSpaceByte = 2 << 30   // 2 GiB — Go-runtime children need ≥1 GiB; Python/Node fit
	limitMaxOpenFiles     = 512       //
	limitMaxFileSizeBytes = 256 << 20 // 256 MiB per file
)

// Tool implements agent.Tool. Construct with New (tests) or NewWithWarden
// (production); Bind wires the bus so each run journals a code.executed event.
type Tool struct {
	// Warden is the isolation engine code runs through. Nil → a no-bus engine.
	Warden warden.Engine
	// SandboxRoot is <baseDir>/sandbox: ephemeral runs land in run-* tempdirs
	// here, named projects in <SandboxRoot>/projects/<slug>.
	SandboxRoot string
	// Runtimes maps a language id to its resolved interpreter absolute path. Only
	// languages present here can run.
	Runtimes map[string]string
	// NetEnabled is the master network switch (AGEZT_SANDBOX_NO_NET=1 → false);
	// when false, no run gets network regardless of allow_net.
	NetEnabled bool
	// Profile is the isolation profile requested of the warden (default
	// ProfileNamespace, downgraded + journaled where unavailable).
	Profile warden.Profile

	bus *bus.Bus
}

// New returns a Tool with a no-bus warden — suitable for unit tests.
func New(sandboxRoot string, runtimes map[string]string) *Tool {
	return &Tool{
		Warden:      warden.New(nil),
		SandboxRoot: sandboxRoot,
		Runtimes:    runtimes,
		NetEnabled:  true,
		Profile:     warden.ProfileNamespace,
	}
}

// NewWithWarden returns a Tool routed through the supplied warden engine — the
// path the daemon uses so audit events land on the kernel bus.
func NewWithWarden(w warden.Engine, sandboxRoot string, runtimes map[string]string, netEnabled bool) *Tool {
	return &Tool{
		Warden:      w,
		SandboxRoot: sandboxRoot,
		Runtimes:    runtimes,
		NetEnabled:  netEnabled,
		Profile:     warden.ProfileNamespace,
	}
}

// Bind wires the live bus so each run publishes a code.executed event. Called
// once after the kernel opens.
func (t *Tool) Bind(b *bus.Bus) { t.bus = b }

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
		"Use `files` to drop extra source files alongside the entrypoint. " + netLine +
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
    "timeout_ms": {"type":"integer", "description":"Per-call timeout in ms (default 120000, max 600000)."},
    "allow_net": {"type":"boolean", "description":"Deno only: grant network (default true). Ignored if the daemon has network disabled."}
  }
}`

	return agent.ToolDef{
		Name:        "code_exec",
		Description: desc,
		InputSchema: json.RawMessage(schema),
	}
}

type input struct {
	Language  string            `json:"language"`
	Code      string            `json:"code"`
	Stdin     string            `json:"stdin,omitempty"`
	Project   string            `json:"project,omitempty"`
	Files     map[string]string `json:"files,omitempty"`
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

	timeout := DefaultTimeout
	if in.TimeoutMS > 0 {
		timeout = time.Duration(in.TimeoutMS) * time.Millisecond
	}
	if timeout > MaxTimeout {
		timeout = MaxTimeout
	}

	profile := t.Profile
	if profile == "" {
		profile = warden.ProfileNamespace
	}
	w := t.Warden
	if w == nil {
		w = warden.New(nil)
	}

	res, err := w.Run(ctx, warden.Spec{
		Profile: profile,
		Argv:    buildArgv(interp, lang, entry, dir, allowNet),
		WorkDir: dir,
		Env:     scrubEnv(dir),
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
	return render(lang, projectSlug, dir, ephemeral, timeout, res), nil
}

// workDir resolves where this run executes. With no project name it returns a
// fresh ephemeral temp dir (caller removes it). With a project name it returns
// a stable per-project dir that persists across calls.
func (t *Tool) workDir(project string) (dir string, ephemeral bool, projectSlug string, err error) {
	if strings.TrimSpace(project) == "" {
		if err = os.MkdirAll(t.SandboxRoot, 0o700); err != nil {
			return "", false, "", err
		}
		dir, err = os.MkdirTemp(t.SandboxRoot, "run-")
		return dir, true, "", err
	}
	projectSlug = slug(project)
	dir = filepath.Join(t.SandboxRoot, "projects", projectSlug)
	if err = os.MkdirAll(dir, 0o700); err != nil {
		return "", false, "", err
	}
	return dir, false, projectSlug, nil
}

// render builds the model-facing Result: a one-line header (language / project /
// effective isolation profile / dir) followed by combined output, with the same
// truncation / timeout / non-zero-exit semantics as the shell tool.
func render(lang, projectSlug, dir string, ephemeral bool, timeout time.Duration, res *warden.Result) agent.Result {
	var head strings.Builder
	fmt.Fprintf(&head, "[code_exec] language=%s", lang)
	if projectSlug != "" {
		fmt.Fprintf(&head, " project=%s", projectSlug)
	}
	fmt.Fprintf(&head, " isolation=%s", res.EffectiveProfile)
	if res.Downgraded {
		fmt.Fprintf(&head, " (requested %s, downgraded on this host)", res.RequestedProfile)
	}
	if !ephemeral {
		fmt.Fprintf(&head, " dir=%s", dir)
	}
	header := head.String()

	combined := append([]byte{}, res.Stdout...)
	if len(res.Stderr) > 0 {
		if len(combined) > 0 {
			combined = append(combined, '\n')
		}
		combined = append(combined, res.Stderr...)
	}
	if res.Truncated {
		combined = append([]byte("[truncated to last 256 KiB]\n"), combined...)
	}

	body := strings.TrimRight(string(combined), "\n")
	if res.TimedOut {
		return agent.Result{Output: fmt.Sprintf("%s\ntimed out after %s\n%s", header, timeout, body), IsError: true}
	}
	if res.ExitCode != 0 {
		return agent.Result{Output: fmt.Sprintf("%s\n%s\n[exit code %d]", header, body, res.ExitCode), IsError: true}
	}
	if body == "" {
		body = "(no output)"
	}
	return agent.Result{Output: header + "\n" + body}
}

// publish journals one code.executed event per run so `agt why` and the run
// timeline show what code ran and how it ended. No-op without a bus.
func (t *Tool) publish(ctx context.Context, lang, projectSlug string, codeBytes int, net bool, res *warden.Result) {
	if t.bus == nil {
		return
	}
	payload := map[string]any{
		"language":          lang,
		"code_bytes":        codeBytes,
		"exit_code":         res.ExitCode,
		"timed_out":         res.TimedOut,
		"net":               net,
		"profile_effective": string(res.EffectiveProfile),
		"duration_ms":       res.Duration.Milliseconds(),
	}
	if projectSlug != "" {
		payload["project"] = projectSlug
	}
	_, _ = t.bus.Publish(event.Spec{
		Subject:       "code.exec",
		Kind:          event.KindCodeExecuted,
		Actor:         "tool.code_exec",
		CorrelationID: warden.CorrelationFrom(ctx),
		Payload:       payload,
	})
}

func errResult(msg string) agent.Result {
	return agent.Result{Output: "code_exec: " + msg, IsError: true}
}
