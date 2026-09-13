// SPDX-License-Identifier: MIT

package codeexec

import (
	"context"
	"fmt"
	"path"
	"path/filepath"
	"strings"
	"time"

	"github.com/agezt/agezt/kernel/agent"
	"github.com/agezt/agezt/kernel/executionprofile"
	"github.com/agezt/agezt/kernel/warden"
)

func (t *Tool) invokeSSH(
	ctx context.Context,
	cfg executionprofile.SSHConfig,
	w warden.Engine,
	lang, interp, entry, dir string,
	ephemeral bool,
	projectSlug string,
	packages []string,
	allowNet bool,
	timeout time.Duration,
	codeBytes int,
) agent.Result {
	remoteDir := remoteWorkDir(cfg, dir, projectSlug)
	if strings.TrimSpace(remoteDir) == "" {
		return errResult("ssh remote workdir is empty")
	}
	if len(packages) > 0 {
		if lang != LangPython {
			return errResult(`packages are only supported for python; for deno/JS, import npm packages inline instead, e.g. import x from "npm:cheerio"`)
		}
		if !t.NetEnabled {
			return errResult("cannot install packages: network is disabled on this daemon (AGEZT_SANDBOX_NO_NET=1)")
		}
	}
	if r, err := runSSHCommand(ctx, w, cfg, "mkdir -p "+executionprofile.ShellQuote(remoteDir), timeout, "tool.code_exec.ssh.prepare"); err != nil {
		return errResult("ssh prepare failed: " + err.Error())
	} else if r.ExitCode != 0 {
		return errResult(fmt.Sprintf("ssh prepare failed (exit %d):\n%s", r.ExitCode, installTail(r)))
	}
	src := filepath.Join(dir, ".")
	if r, err := w.Run(ctx, warden.Spec{
		Profile: warden.ProfileNone,
		Argv:    cfg.SCPToArgv(src, remoteDir),
		Env:     sshClientEnv(),
		Limits: warden.Limits{
			Timeout:        timeout,
			MaxOutputBytes: MaxOutputBytes,
		},
		Actor:         "tool.code_exec.ssh.upload",
		CorrelationID: warden.CorrelationFrom(ctx),
	}); err != nil {
		return errResult("ssh upload failed: " + err.Error())
	} else if r.ExitCode != 0 {
		return errResult(fmt.Sprintf("ssh upload failed (exit %d):\n%s", r.ExitCode, installTail(r)))
	}

	remoteRuntime := remoteRuntimeCommand(lang, interp)
	if len(packages) > 0 {
		pkgs, perr := validatePackages(packages)
		if perr != nil {
			return errResult(perr.Error())
		}
		if len(pkgs) > 0 {
			cmd := "cd " + executionprofile.ShellQuote(remoteDir) + " && " + remotePipInstallCommand(remoteRuntime, pkgs)
			if r, err := runSSHCommand(ctx, w, cfg, cmd, pipInstallTimeout, "tool.code_exec.ssh.install"); err != nil {
				return errResult("pip install failed: " + err.Error())
			} else if r.ExitCode != 0 {
				return errResult(fmt.Sprintf("pip install failed (exit %d):\n%s", r.ExitCode, installTail(r)))
			}
		}
	}

	runCmd := "cd " + executionprofile.ShellQuote(remoteDir) + " && " + remoteRunCommand(lang, remoteRuntime, entry, allowNet, len(packages) > 0)
	res, err := runSSHCommand(ctx, w, cfg, runCmd, timeout, "tool.code_exec.ssh.run")
	var artifacts []artifactExportRecord
	var artifactErr error
	if err == nil {
		artifacts, artifactErr = t.exportSSHArtifacts(ctx, w, cfg, remoteDir)
	}
	if ephemeral {
		_, _ = runSSHCommand(ctx, w, cfg, "rm -rf "+executionprofile.ShellQuote(remoteDir), 30*time.Second, "tool.code_exec.ssh.cleanup")
	}
	if err != nil {
		return errResult(fmt.Sprintf("remote run failed: %v", err))
	}
	t.publish(ctx, lang, projectSlug, codeBytes, allowNet, res)
	return appendArtifactExport(renderRemote(lang, projectSlug, remoteDir, timeout, res), "ssh", artifacts, artifactErr)
}

func runSSHCommand(ctx context.Context, w warden.Engine, cfg executionprofile.SSHConfig, command string, timeout time.Duration, actor string) (*warden.Result, error) {
	return w.Run(ctx, warden.Spec{
		Profile: warden.ProfileNone,
		Argv:    cfg.CommandArgv(command),
		Env:     sshClientEnv(),
		Limits: warden.Limits{
			Timeout:        timeout,
			MaxOutputBytes: MaxOutputBytes,
		},
		Actor:         actor,
		CorrelationID: warden.CorrelationFrom(ctx),
	})
}

func (t *Tool) invokeK8s(
	ctx context.Context,
	cfg executionprofile.K8sConfig,
	w warden.Engine,
	lang, interp, entry, dir string,
	ephemeral bool,
	projectSlug string,
	packages []string,
	allowNet bool,
	timeout time.Duration,
	codeBytes int,
) agent.Result {
	remoteDir := k8sWorkDir(cfg, dir, projectSlug)
	if strings.TrimSpace(remoteDir) == "" {
		return errResult("k8s remote workdir is empty")
	}
	if len(packages) > 0 {
		if lang != LangPython {
			return errResult(`packages are only supported for python; for deno/JS, import npm packages inline instead, e.g. import x from "npm:cheerio"`)
		}
		if !t.NetEnabled {
			return errResult("cannot install packages: network is disabled on this daemon (AGEZT_SANDBOX_NO_NET=1)")
		}
	}
	if r, err := runK8sCommand(ctx, w, cfg, "mkdir -p "+executionprofile.ShellQuote(remoteDir), timeout, "tool.code_exec.k8s.prepare"); err != nil {
		return errResult("k8s prepare failed: " + err.Error())
	} else if r.ExitCode != 0 {
		return errResult(fmt.Sprintf("k8s prepare failed (exit %d):\n%s", r.ExitCode, installTail(r)))
	}
	src := filepath.Join(dir, ".")
	if r, err := w.Run(ctx, warden.Spec{
		Profile: warden.ProfileNone,
		Argv:    cfg.CopyToArgv(src, remoteDir),
		Env:     kubectlClientEnv(),
		Limits: warden.Limits{
			Timeout:        timeout,
			MaxOutputBytes: MaxOutputBytes,
		},
		Actor:         "tool.code_exec.k8s.upload",
		CorrelationID: warden.CorrelationFrom(ctx),
	}); err != nil {
		return errResult("k8s upload failed: " + err.Error())
	} else if r.ExitCode != 0 {
		return errResult(fmt.Sprintf("k8s upload failed (exit %d):\n%s", r.ExitCode, installTail(r)))
	}

	remoteRuntime := remoteRuntimeCommand(lang, interp)
	if len(packages) > 0 {
		pkgs, perr := validatePackages(packages)
		if perr != nil {
			return errResult(perr.Error())
		}
		if len(pkgs) > 0 {
			cmd := "cd " + executionprofile.ShellQuote(remoteDir) + " && " + remotePipInstallCommand(remoteRuntime, pkgs)
			if r, err := runK8sCommand(ctx, w, cfg, cmd, pipInstallTimeout, "tool.code_exec.k8s.install"); err != nil {
				return errResult("pip install failed: " + err.Error())
			} else if r.ExitCode != 0 {
				return errResult(fmt.Sprintf("pip install failed (exit %d):\n%s", r.ExitCode, installTail(r)))
			}
		}
	}

	runCmd := "cd " + executionprofile.ShellQuote(remoteDir) + " && " + remoteRunCommand(lang, remoteRuntime, entry, allowNet, len(packages) > 0)
	res, err := runK8sCommand(ctx, w, cfg, runCmd, timeout, "tool.code_exec.k8s.run")
	var artifacts []artifactExportRecord
	var artifactErr error
	if err == nil {
		artifacts, artifactErr = t.exportK8sArtifacts(ctx, w, cfg, remoteDir)
	}
	if ephemeral {
		_, _ = runK8sCommand(ctx, w, cfg, "rm -rf "+executionprofile.ShellQuote(remoteDir), 30*time.Second, "tool.code_exec.k8s.cleanup")
	}
	if err != nil {
		return errResult(fmt.Sprintf("k8s run failed: %v", err))
	}
	t.publish(ctx, lang, projectSlug, codeBytes, allowNet, res)
	return appendArtifactExport(renderRemoteProfile("k8s", lang, projectSlug, remoteDir, timeout, res), "k8s", artifacts, artifactErr)
}

func runK8sCommand(ctx context.Context, w warden.Engine, cfg executionprofile.K8sConfig, command string, timeout time.Duration, actor string) (*warden.Result, error) {
	return w.Run(ctx, warden.Spec{
		Profile: warden.ProfileNone,
		Argv:    cfg.CommandArgv(command),
		Env:     kubectlClientEnv(),
		Limits: warden.Limits{
			Timeout:        timeout,
			MaxOutputBytes: MaxOutputBytes,
		},
		Actor:         actor,
		CorrelationID: warden.CorrelationFrom(ctx),
	})
}

func (t *Tool) invokeModal(
	ctx context.Context,
	cfg executionprofile.ModalConfig,
	w warden.Engine,
	lang, interp, entry, dir string,
	projectSlug string,
	packages []string,
	allowNet bool,
	timeout time.Duration,
	codeBytes int,
) agent.Result {
	if len(packages) > 0 {
		if lang != LangPython {
			return errResult(`packages are only supported for python; for deno/JS, import npm packages inline instead, e.g. import x from "npm:cheerio"`)
		}
		if !t.NetEnabled {
			return errResult("cannot install packages: network is disabled on this daemon (AGEZT_SANDBOX_NO_NET=1)")
		}
	}
	remoteDir := modalMountDir(dir)
	remoteRuntime := remoteRuntimeCommand(lang, interp)
	runCmd := remoteRunCommand(lang, remoteRuntime, entry, allowNet, len(packages) > 0)
	if len(packages) > 0 {
		pkgs, perr := validatePackages(packages)
		if perr != nil {
			return errResult(perr.Error())
		}
		if len(pkgs) > 0 {
			runCmd = remotePipInstallCommand(remoteRuntime, pkgs) + " && " + runCmd
		}
	}
	cmd := "cd " + executionprofile.ShellQuote(remoteDir) + " && " + runCmd
	cmd = wrapModalArtifactExport(cmd, path.Join(remoteDir, artifactExportDir))
	res, err := w.Run(ctx, warden.Spec{
		Profile: warden.ProfileNone,
		Argv:    cfg.CodeExecArgv(dir, cmd),
		Env:     modalClientEnv(),
		Limits: warden.Limits{
			Timeout:        timeout,
			MaxOutputBytes: MaxOutputBytes + modalArtifactArchiveBytes,
		},
		Actor:         "tool.code_exec.modal.run",
		CorrelationID: warden.CorrelationFrom(ctx),
	})
	if err != nil {
		return errResult(fmt.Sprintf("modal run failed: %v", err))
	}
	var artifacts []artifactExportRecord
	var artifactErr error
	if t.index != nil {
		var payload string
		var found bool
		var splitErr error
		res.Stdout, payload, found, splitErr = splitArtifactEnvelope(res.Stdout, modalArtifactBegin, modalArtifactEnd)
		if splitErr != nil {
			artifactErr = splitErr
		} else if found {
			artifacts, artifactErr = t.exportTarGzBase64Artifacts(ctx, payload, "modal")
		}
	}
	t.publish(ctx, lang, projectSlug, codeBytes, allowNet, res)
	return appendArtifactExport(renderRemoteProfile("modal", lang, projectSlug, remoteDir, timeout, res), "modal", artifacts, artifactErr)
}

func wrapModalArtifactExport(runCmd, artifactDir string) string {
	qDir := executionprofile.ShellQuote(artifactDir)
	return "(" + runCmd + "); status=$?; " +
		"if [ -d " + qDir + " ] && command -v tar >/dev/null 2>&1 && command -v base64 >/dev/null 2>&1; then " +
		"printf '\\n" + modalArtifactBegin + "\\n'; " +
		"tar -C " + qDir + " -czf - . | base64; " +
		"printf '\\n" + modalArtifactEnd + "\\n'; " +
		"fi; exit $status"
}

