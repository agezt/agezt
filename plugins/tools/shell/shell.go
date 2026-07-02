// SPDX-License-Identifier: MIT

// Package shell is the in-process shell tool. It runs commands via the
// platform's default shell ("cmd /C" on Windows, "sh -c" elsewhere) and
// returns combined stdout+stderr to the model.
//
// Execution is delegated to the kernel/warden Engine so timeout,
// output truncation, exit code propagation, and audit events
// (warden.executed, warden.profile_downgraded, warden.limit_exceeded)
// are all handled in one place — including the future Linux
// namespace+cgroups isolation when that backend ships in M1.d.
//
// SECURITY NOTE (M1.c): the cross-platform Warden Engine runs commands
// with the kernel's full privileges (ProfileNone). Edict's trust ladder
// + hard-deny rules are still the only gate on what the shell tool may
// execute. A request for ProfileNamespace is honoured *as a request*
// and journaled as a downgrade so audits stay honest.
package shell

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"runtime"
	"time"

	"github.com/agezt/agezt/kernel/agent"
	"github.com/agezt/agezt/kernel/executionprofile"
	"github.com/agezt/agezt/kernel/warden"
)

// DefaultTimeout caps a single command's wall time when the model
// omits timeout_ms and the Tool has no explicit Timeout set.
const DefaultTimeout = 30 * time.Second

// MaxOutputBytes truncates command output so a runaway command does not
// blow the journal/context budget. 64 KiB is the model-facing budget;
// Warden's own cap is 256 KiB (SPEC-02 §5) and we tighten it here.
const MaxOutputBytes = 64 * 1024

// Tool is the in-process shell tool implementation of agent.Tool.
type Tool struct {
	// Warden is the isolation engine commands run through. If nil, a
	// process-default engine (warden.New(nil)) is used — events go
	// nowhere in that case, suitable for unit tests.
	Warden warden.Engine
	// Shell overrides the default shell binary (mainly for tests). Empty
	// means use the platform default.
	Shell string
	// ShellArg is the flag passed before the command (default "/C" on
	// Windows, "-c" elsewhere).
	ShellArg string
	// Timeout overrides DefaultTimeout when > 0.
	Timeout time.Duration
	// Profile is the isolation profile the shell tool requests of
	// Warden. Defaults to ProfileNamespace (shell is the canonical
	// "needs isolation" tool per SPEC-06 §2). On non-Linux this
	// downgrades to ProfileNone with a journal event.
	Profile warden.Profile
	// WorkDir is the working directory commands run in. Empty inherits the
	// daemon's process CWD. The daemon sets this to the file tool's workspace
	// root so the shell and file tools agree on what "here" is — otherwise an
	// agent's `dir`/`ls` (shell, daemon CWD) and `file read x` (file tool,
	// workspace root) see different directories, which is deeply confusing
	// (M609).
	WorkDir string
	// BaseDir points at the AGEZT home/base dir so profile-specific vault-backed
	// secret file mounts can resolve creds.json. Empty disables that feature
	// unless no mounts are configured.
	BaseDir string
}

// NewWithWarden returns a Tool that routes through the supplied Warden
// engine — the path the daemon uses.
func NewWithWarden(w warden.Engine) *Tool {
	return &Tool{Warden: w, Profile: warden.ProfileNamespace}
}

// Name returns the tool's canonical name.
func (t *Tool) Name() string { return "shell" }

// Definition implements agent.Tool.
func (t *Tool) Definition() agent.ToolDef {
	workDir := t.WorkDir
	if workDir == "" {
		workDir = "process working directory"
	}
	return agent.ToolDef{
		Name: "shell",
		Description: "Run a command in the operating system's default shell. " +
			"Returns combined stdout+stderr. Output is truncated to 64 KiB.",
		Effect: agent.ToolEffect{
			Class: agent.EffectIrreversible,
			PredictedEffects: []string{
				"execute an operating-system command in the configured working directory",
				"may read, write, start processes, or contact the network depending on the command",
			},
			AffectedResources: []string{"host shell", "working directory: " + workDir},
			RollbackNotes:     "No reliable generic rollback exists for arbitrary shell commands; require command-specific rollback or restore from backups/version control.",
			Confidence:        0.45,
		},
		InputSchema: json.RawMessage(`{
  "type": "object",
  "required": ["command"],
  "properties": {
    "command": {"type": "string", "description": "The shell command to run (a single line; use && or ; to chain)."},
    "timeout_ms": {"type": "integer", "description": "Per-call timeout in milliseconds. Default 30000."}
  }
}`),
	}
}

type shellInput struct {
	Command   string `json:"command"`
	TimeoutMS int64  `json:"timeout_ms,omitempty"`
}

// Invoke implements agent.Tool. It builds a Warden Spec and delegates;
// the Result is rendered into the model's tool_result text.
func (t *Tool) Invoke(ctx context.Context, raw json.RawMessage) (agent.Result, error) {
	var in shellInput
	if err := json.Unmarshal(raw, &in); err != nil {
		return agent.Result{}, fmt.Errorf("shell: parse input: %w", err)
	}
	if in.Command == "" {
		return agent.Result{Output: "command is required", IsError: true}, nil
	}

	timeout := DefaultTimeout
	if t.Timeout > 0 {
		timeout = t.Timeout
	}
	if in.TimeoutMS > 0 {
		timeout = time.Duration(in.TimeoutMS) * time.Millisecond
	}

	w := t.Warden
	if w == nil {
		w = warden.New(nil)
	}
	if sshCfg, ok := executionprofile.SSHOverrideFrom(ctx); ok {
		res, err := w.Run(ctx, warden.Spec{
			Profile: warden.ProfileNone,
			Argv:    sshCfg.ShellCommandArgv(in.Command),
			Env:     sshEnv(),
			Limits: warden.Limits{
				Timeout:        timeout,
				MaxOutputBytes: MaxOutputBytes,
			},
			Actor:         "tool.shell.ssh",
			CorrelationID: warden.CorrelationFrom(ctx),
		})
		if err != nil {
			return agent.Result{
				Output:  fmt.Sprintf("ssh run failed: %v", err),
				IsError: true,
			}, nil
		}
		return renderResult(timeout, res), nil
	}
	if k8sCfg, ok := executionprofile.K8sOverrideFrom(ctx); ok {
		res, err := w.Run(ctx, warden.Spec{
			Profile: warden.ProfileNone,
			Argv:    k8sCfg.ShellCommandArgv(in.Command),
			Env:     kubectlEnv(),
			Limits: warden.Limits{
				Timeout:        timeout,
				MaxOutputBytes: MaxOutputBytes,
			},
			Actor:         "tool.shell.k8s",
			CorrelationID: warden.CorrelationFrom(ctx),
		})
		if err != nil {
			return agent.Result{
				Output:  fmt.Sprintf("kubectl run failed: %v", err),
				IsError: true,
			}, nil
		}
		return renderResult(timeout, res), nil
	}
	if modalCfg, ok := executionprofile.ModalOverrideFrom(ctx); ok {
		res, err := w.Run(ctx, warden.Spec{
			Profile: warden.ProfileNone,
			Argv:    modalCfg.ShellCommandArgv(in.Command),
			Env:     cloudCLIEnv(),
			Limits: warden.Limits{
				Timeout:        timeout,
				MaxOutputBytes: MaxOutputBytes,
			},
			Actor:         "tool.shell.modal",
			CorrelationID: warden.CorrelationFrom(ctx),
		})
		if err != nil {
			return agent.Result{
				Output:  fmt.Sprintf("modal run failed: %v", err),
				IsError: true,
			}, nil
		}
		return renderResult(timeout, res), nil
	}
	if daytonaCfg, ok := executionprofile.DaytonaOverrideFrom(ctx); ok {
		timeoutSeconds := int((timeout + time.Second - 1) / time.Second)
		res, err := w.Run(ctx, warden.Spec{
			Profile: warden.ProfileNone,
			Argv:    daytonaCfg.ShellCommandArgv(in.Command, timeoutSeconds),
			Env:     cloudCLIEnv(),
			Limits: warden.Limits{
				Timeout:        timeout,
				MaxOutputBytes: MaxOutputBytes,
			},
			Actor:         "tool.shell.daytona",
			CorrelationID: warden.CorrelationFrom(ctx),
		})
		if err != nil {
			return agent.Result{
				Output:  fmt.Sprintf("daytona run failed: %v", err),
				IsError: true,
			}, nil
		}
		return renderResult(timeout, res), nil
	}
	profile := t.Profile
	if profile == "" {
		profile = warden.ProfileNamespace
	}
	if override, ok := warden.ProfileOverrideFrom(ctx); ok {
		profile = override
	}
	profileID := executionprofile.ProfileIDForWardenProfile(profile)

	// Per-agent workdir (M792): a named agent's commands run inside its
	// workspace subdirectory (created lazily on first use). Anchored under the
	// tool's configured workspace WorkDir — without one there is nothing safe
	// to anchor to, so the ctx workdir is ignored. The ctx value is
	// escape-proofed at the setter (and by profile validation upstream).
	workDir := t.WorkDir
	if wd := agent.WorkdirFromContext(ctx); wd != "" && t.WorkDir != "" {
		workDir = filepath.Join(t.WorkDir, filepath.FromSlash(wd))
		_ = os.MkdirAll(workDir, 0o755)
	}

	shellBin, shellArg := t.resolveShell()
	env := executionprofile.AppendEnvPassthrough(scrubEnv(workDir), profileID)
	secretEnv, cleanupSecrets, _, serr := executionprofile.PrepareSecretFileMounts(t.BaseDir, profileID, workDir)
	if serr != nil {
		return agent.Result{Output: "shell: secret file mounts: " + serr.Error(), IsError: true}, nil
	}
	defer cleanupSecrets()
	env = append(env, secretEnv...)
	res, err := w.Run(ctx, warden.Spec{
		Profile: profile,
		Argv:    []string{shellBin, shellArg, in.Command},
		// A scrubbed host environment (M957): PATH + the OS vars a shell needs,
		// secrets dropped. Warden defaults a nil Env to EMPTY (anti-leak), but an
		// empty env breaks cmd.exe on Windows (no PATH/SystemRoot → "not
		// recognized" / "syntax is incorrect"), which was crippling the shell tool.
		Env:     env,
		WorkDir: workDir, // M609 workspace coherence + M792 per-agent subdir
		Limits: warden.Limits{
			Timeout:        timeout,
			MaxOutputBytes: MaxOutputBytes,
		},
		Actor: "tool.shell",
		// Stamp the run correlation (set by the runtime on the tool ctx) so the
		// warden.executed / profile_downgraded events land in this run's timeline
		// and `agt why` can walk to them. Empty when run outside a kernel run.
		CorrelationID: warden.CorrelationFrom(ctx),
	})
	if err != nil {
		return agent.Result{
			Output:  fmt.Sprintf("warden run failed: %v", err),
			IsError: true,
		}, nil
	}

	return renderResult(timeout, res), nil
}

func renderResult(timeout time.Duration, res *warden.Result) agent.Result {
	// Combine streams the way the previous implementation did
	// (CombinedOutput). Stderr appended after stdout keeps the order
	// stable across shells.
	combined := append([]byte{}, res.Stdout...)
	if len(res.Stderr) > 0 {
		if len(combined) > 0 {
			combined = append(combined, '\n')
		}
		combined = append(combined, res.Stderr...)
	}
	if res.Truncated {
		combined = append([]byte("[truncated to last 64 KiB]\n"), combined...)
	}

	if res.TimedOut {
		return agent.Result{
			Output:  fmt.Sprintf("timed out after %s\n%s", timeout, combined),
			IsError: true,
		}
	}
	if res.ExitCode != 0 {
		return agent.Result{
			Output:  fmt.Sprintf("%s\n[exit code %d]", combined, res.ExitCode),
			IsError: true,
		}
	}
	return agent.Result{Output: string(combined)}
}

// ShellHint reports the shell binary and command flag this tool will use on
// the current host (e.g. "cmd","/C" on Windows, "sh","-c" elsewhere). The
// runtime's host-environment preamble (M609) calls it so the model is told the
// EXACT shell — including an operator's override — rather than guessing from
// GOOS. Part of the implicit "shell hinter" interface the runtime looks for.
func (t *Tool) ShellHint() (string, string) { return t.resolveShell() }

func (t *Tool) resolveShell() (string, string) {
	if t.Shell != "" {
		arg := t.ShellArg
		if arg == "" {
			arg = "-c"
		}
		return t.Shell, arg
	}
	if runtime.GOOS == "windows" {
		return "cmd", "/C"
	}
	return "sh", "-c"
}
