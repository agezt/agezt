// SPDX-License-Identifier: MIT

// Package shell: Invoke (the execution router — SSH/K8s/Modal/Daytona overrides
// plus the default profile path). Extracted from shell.go during the
// Day-211 god-file split. Public API unchanged.
package shell


import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"time"

	"github.com/agezt/agezt/kernel/agent"
	"github.com/agezt/agezt/kernel/executionprofile"
	"github.com/agezt/agezt/kernel/warden"
)
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
	// Key the credential bucket off what will ACTUALLY run, not what we asked
	// for (RCE-001). The requested profile defaults to ProfileNamespace, but
	// resolveEffectiveProfile returns ProfileNone on every non-Linux host — so
	// keying on the request handed secrets an operator had scoped to the isolated
	// "warden" tier straight into a completely un-isolated `cmd /S /C` child.
	// Windows and macOS hit that on the DEFAULT path, not an exotic one.
	//
	// The engine is still asked for the ORIGINAL profile below: it does its own
	// downgrade and emits warden.profile_downgraded, and that honest-downgrade
	// signal is the operator's notice. This only decides which bucket's secrets
	// are willing to travel — and an isolated-tier secret must not follow a
	// request that silently became un-isolated.
	profileID := executionprofile.ProfileIDForWardenProfile(w.EffectiveProfile(profile))

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
