// SPDX-License-Identifier: MIT

package codeexec

// Remote/backend transport for the codeexec tool: invokeSSH + invokeK8s +
// invokeModal + runSSHCommand + runK8sCommand + the remote workdir /
// mount-dir / runtime / pip-install / run-command helpers, plus the
// ssh/kubectl/modal/daytona client env builders. Carved out of codeexec.go
// during the Day 156 god-file split so the main file can stay focused on
// lifecycle + local Invoke + render/publish.
// Public API unchanged.

import (
	"context"
	"fmt"
	"os"
	"path"
	"path/filepath"
	"strings"
	"time"

	"github.com/agezt/agezt/kernel/agent"
	"github.com/agezt/agezt/kernel/warden"
	"github.com/agezt/agezt/kernel/executionprofile"
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

func remoteWorkDir(cfg executionprofile.SSHConfig, localDir, projectSlug string) string {
	root := strings.Trim(strings.TrimSpace(cfg.WorkDir), "/")
	if root == "" {
		root = ".agezt/code_exec"
	}
	if strings.HasPrefix(strings.TrimSpace(cfg.WorkDir), "/") {
		root = "/" + root
	}
	if projectSlug != "" {
		return path.Join(root, "projects", projectSlug)
	}
	base := filepath.Base(localDir)
	if base == "." || base == string(filepath.Separator) || strings.TrimSpace(base) == "" {
		base = fmt.Sprintf("run-%d", time.Now().UnixNano())
	}
	return path.Join(root, "runs", base)
}

func modalMountDir(localDir string) string {
	base := filepath.Base(localDir)
	if base == "." || base == string(filepath.Separator) || strings.TrimSpace(base) == "" {
		base = fmt.Sprintf("run-%d", time.Now().UnixNano())
	}
	return path.Join("/mnt", base)
}

func k8sWorkDir(cfg executionprofile.K8sConfig, localDir, projectSlug string) string {
	root := strings.Trim(strings.TrimSpace(cfg.WorkDir), "/")
	if root == "" {
		root = ".agezt/code_exec"
	}
	if strings.HasPrefix(strings.TrimSpace(cfg.WorkDir), "/") {
		root = "/" + root
	}
	if projectSlug != "" {
		return path.Join(root, "projects", projectSlug)
	}
	base := filepath.Base(localDir)
	if base == "." || base == string(filepath.Separator) || strings.TrimSpace(base) == "" {
		base = fmt.Sprintf("run-%d", time.Now().UnixNano())
	}
	return path.Join(root, "runs", base)
}

func remoteRuntimeCommand(lang, interp string) string {
	switch lang {
	case LangPython:
		base := strings.ToLower(filepath.Base(interp))
		if strings.HasPrefix(base, "python3") {
			return "python3"
		}
		return "python3"
	case LangNode:
		return "node"
	case LangDeno:
		return "deno"
	default:
		return filepath.Base(interp)
	}
}

func remotePipInstallCommand(runtime string, pkgs []string) string {
	args := []string{runtime, "-m", "pip", "install", "--target", pyDepsName, "--no-input", "--disable-pip-version-check", "--no-warn-script-location"}
	args = append(args, pkgs...)
	return quoteCommand(args)
}

func remoteRunCommand(lang, runtime, entry string, allowNet bool, hasDeps bool) string {
	switch lang {
	case LangPython:
		args := []string{runtime, entry}
		cmd := quoteCommand(args)
		if hasDeps {
			cmd = "PYTHONPATH=" + executionprofile.ShellQuote(pyDepsName) + " " + cmd
		}
		return cmd
	case LangDeno:
		args := []string{runtime, "run", "--quiet", "--no-prompt", "--allow-read=.", "--allow-write=.", "--allow-env"}
		if allowNet {
			args = append(args, "--allow-net")
		}
		args = append(args, entry)
		return quoteCommand(args)
	default:
		return quoteCommand([]string{runtime, entry})
	}
}

func quoteCommand(args []string) string {
	out := make([]string, 0, len(args))
	for _, a := range args {
		out = append(out, executionprofile.ShellQuote(a))
	}
	return strings.Join(out, " ")
}

func sshClientEnv() []string {
	allow := map[string]bool{
		"PATH": true, "PATHEXT": true, "COMSPEC": true,
		"SYSTEMROOT": true, "SYSTEMDRIVE": true, "WINDIR": true,
		"HOME": true, "USERPROFILE": true, "SSH_AUTH_SOCK": true,
		"LANG": true,
	}
	var out []string
	for _, kv := range os.Environ() {
		name, _, ok := strings.Cut(kv, "=")
		if !ok {
			continue
		}
		up := strings.ToUpper(name)
		if isSecretName(up) {
			continue
		}
		if allow[up] || strings.HasPrefix(up, "LC_") {
			out = append(out, kv)
		}
	}
	return out
}

func kubectlClientEnv() []string {
	allow := map[string]bool{
		"PATH": true, "PATHEXT": true, "COMSPEC": true,
		"SYSTEMROOT": true, "SYSTEMDRIVE": true, "WINDIR": true,
		"HOME": true, "USERPROFILE": true, "KUBECONFIG": true,
		"LANG": true,
	}
	var out []string
	for _, kv := range os.Environ() {
		name, _, ok := strings.Cut(kv, "=")
		if !ok {
			continue
		}
		up := strings.ToUpper(name)
		if isSecretName(up) {
			continue
		}
		if allow[up] || strings.HasPrefix(up, "LC_") {
			out = append(out, kv)
		}
	}
	return out
}

func modalClientEnv() []string {
	allow := map[string]bool{
		"PATH": true, "PATHEXT": true, "COMSPEC": true,
		"SYSTEMROOT": true, "SYSTEMDRIVE": true, "WINDIR": true,
		"HOME": true, "USERPROFILE": true, "MODAL_CONFIG_PATH": true,
		"LANG": true,
	}
	var out []string
	for _, kv := range os.Environ() {
		name, _, ok := strings.Cut(kv, "=")
		if !ok {
			continue
		}
		up := strings.ToUpper(name)
		if isSecretName(up) {
			continue
		}
		if allow[up] || strings.HasPrefix(up, "LC_") {
			out = append(out, kv)
		}
	}
	return out
}

func daytonaClientEnv() []string {
	allow := map[string]bool{
		"PATH": true, "PATHEXT": true, "COMSPEC": true,
		"SYSTEMROOT": true, "SYSTEMDRIVE": true, "WINDIR": true,
		"HOME": true, "USERPROFILE": true, "DAYTONA_CONFIG_PATH": true,
		"LANG": true,
	}
	var out []string
	for _, kv := range os.Environ() {
		name, _, ok := strings.Cut(kv, "=")
		if !ok {
			continue
		}
		up := strings.ToUpper(name)
		if isSecretName(up) {
			continue
		}
		if allow[up] || strings.HasPrefix(up, "LC_") {
			out = append(out, kv)
		}
	}
	return out
}

// render builds the model-facing Result: a one-line header (language / project /
// effective isolation profile / dir) followed by combined output, with the same
// truncation / timeout / non-zero-exit semantics as the shell tool.
