// SPDX-License-Identifier: MIT

package codeexec

import (
	"context"
	"encoding/base64"
	"fmt"
	"os"
	"path"
	"path/filepath"
	"strings"
	"time"

	"github.com/agezt/agezt/kernel/agent"
	"github.com/agezt/agezt/kernel/executionprofile"
	"github.com/agezt/agezt/kernel/warden"
)

const (
	daytonaMaterializeChunkBytes = 12 * 1024
	daytonaMaxWorkspaceBytes     = 4 << 20
	daytonaArtifactArchiveBytes  = 8 << 20
)

func (t *Tool) invokeDaytona(
	ctx context.Context,
	cfg executionprofile.DaytonaConfig,
	w warden.Engine,
	lang, interp, entry, dir string,
	ephemeral bool,
	projectSlug string,
	packages []string,
	allowNet bool,
	timeout time.Duration,
	codeBytes int,
) agent.Result {
	remoteDir := daytonaWorkDir(cfg, dir, projectSlug)
	if strings.TrimSpace(remoteDir) == "" {
		return errResult("daytona remote workdir is empty")
	}
	if len(packages) > 0 {
		if lang != LangPython {
			return errResult(`packages are only supported for python; for deno/JS, import npm packages inline instead, e.g. import x from "npm:cheerio"`)
		}
		if !t.NetEnabled {
			return errResult("cannot install packages: network is disabled on this daemon (AGEZT_SANDBOX_NO_NET=1)")
		}
	}
	if r, err := runDaytonaCommand(ctx, w, cfg, "mkdir -p "+executionprofile.ShellQuote(remoteDir), timeout, "tool.code_exec.daytona.prepare", ""); err != nil {
		return errResult("daytona prepare failed: " + err.Error())
	} else if r.ExitCode != 0 {
		return errResult(fmt.Sprintf("daytona prepare failed (exit %d):\n%s", r.ExitCode, installTail(r)))
	}
	if err := t.materializeDaytonaWorkspace(ctx, w, cfg, dir, remoteDir, timeout); err != nil {
		if ephemeral {
			_, _ = runDaytonaCommand(ctx, w, cfg, "rm -rf "+executionprofile.ShellQuote(remoteDir), 30*time.Second, "tool.code_exec.daytona.cleanup", "")
		}
		return errResult("daytona workspace upload failed: " + err.Error())
	}

	remoteRuntime := remoteRuntimeCommand(lang, interp)
	if len(packages) > 0 {
		pkgs, perr := validatePackages(packages)
		if perr != nil {
			return errResult(perr.Error())
		}
		if len(pkgs) > 0 {
			cmd := "cd " + executionprofile.ShellQuote(remoteDir) + " && " + remotePipInstallCommand(remoteRuntime, pkgs)
			if r, err := runDaytonaCommand(ctx, w, cfg, cmd, pipInstallTimeout, "tool.code_exec.daytona.install", remoteDir); err != nil {
				return errResult("pip install failed: " + err.Error())
			} else if r.ExitCode != 0 {
				return errResult(fmt.Sprintf("pip install failed (exit %d):\n%s", r.ExitCode, installTail(r)))
			}
		}
	}

	runCmd := "cd " + executionprofile.ShellQuote(remoteDir) + " && " + remoteRunCommand(lang, remoteRuntime, entry, allowNet, len(packages) > 0)
	res, err := runDaytonaCommand(ctx, w, cfg, runCmd, timeout, "tool.code_exec.daytona.run", remoteDir)
	var artifacts []artifactExportRecord
	var artifactErr error
	if err == nil {
		artifacts, artifactErr = t.exportDaytonaArtifacts(ctx, w, cfg, remoteDir)
	}
	if ephemeral {
		_, _ = runDaytonaCommand(ctx, w, cfg, "rm -rf "+executionprofile.ShellQuote(remoteDir), 30*time.Second, "tool.code_exec.daytona.cleanup", "")
	}
	if err != nil {
		return errResult(fmt.Sprintf("daytona run failed: %v", err))
	}
	t.publish(ctx, lang, projectSlug, codeBytes, allowNet, res)
	return appendArtifactExport(renderRemoteProfile("daytona", lang, projectSlug, remoteDir, timeout, res), "daytona", artifacts, artifactErr)
}

func (t *Tool) exportDaytonaArtifacts(ctx context.Context, w warden.Engine, cfg executionprofile.DaytonaConfig, remoteDir string) ([]artifactExportRecord, error) {
	if t.index == nil {
		return nil, nil
	}
	remoteExportDir := path.Join(remoteDir, artifactExportDir)
	cmd := "if [ -d " + executionprofile.ShellQuote(remoteExportDir) + " ]; then tar -C " + executionprofile.ShellQuote(remoteExportDir) + " -czf - . | base64; fi"
	res, err := w.Run(ctx, warden.Spec{
		Profile: warden.ProfileNone,
		Argv:    cfg.CommandArgv(cmd, "", int(artifactExportCopyTimeout/time.Second)),
		Env:     daytonaClientEnv(),
		Limits: warden.Limits{
			Timeout:        artifactExportCopyTimeout,
			MaxOutputBytes: daytonaArtifactArchiveBytes,
		},
		Actor:         "tool.code_exec.daytona.artifacts.download",
		CorrelationID: warden.CorrelationFrom(ctx),
	})
	if err != nil {
		return nil, fmt.Errorf("download remote artifacts: %w", err)
	}
	if res.ExitCode != 0 {
		return nil, fmt.Errorf("download remote artifacts failed (exit %d): %s", res.ExitCode, installTail(res))
	}
	if res.Truncated {
		return nil, fmt.Errorf("download remote artifacts exceeded %d MiB encoded archive limit", daytonaArtifactArchiveBytes>>20)
	}
	return t.exportTarGzBase64Artifacts(ctx, string(res.Stdout), "daytona")
}

func (t *Tool) materializeDaytonaWorkspace(ctx context.Context, w warden.Engine, cfg executionprofile.DaytonaConfig, localDir, remoteDir string, timeout time.Duration) error {
	var total int64
	return filepath.WalkDir(localDir, func(p string, d os.DirEntry, walkErr error) error {
		if walkErr != nil {
			return walkErr
		}
		if p == localDir {
			return nil
		}
		rel, err := filepath.Rel(localDir, p)
		if err != nil {
			return err
		}
		rel = filepath.ToSlash(rel)
		if rel == artifactExportDir || strings.HasPrefix(rel, artifactExportDir+"/") {
			if d.IsDir() {
				return filepath.SkipDir
			}
			return nil
		}
		if rel == pyDepsName || strings.HasPrefix(rel, pyDepsName+"/") {
			if d.IsDir() {
				return filepath.SkipDir
			}
			return nil
		}
		if d.Type()&os.ModeSymlink != 0 {
			return nil
		}
		if d.IsDir() {
			remoteSubdir := path.Join(remoteDir, rel)
			res, err := runDaytonaCommand(ctx, w, cfg, "mkdir -p "+executionprofile.ShellQuote(remoteSubdir), timeout, "tool.code_exec.daytona.mkdir", "")
			if err != nil {
				return err
			}
			if res.ExitCode != 0 {
				return fmt.Errorf("mkdir %s failed (exit %d): %s", rel, res.ExitCode, installTail(res))
			}
			return nil
		}
		clean, ok := sanitizeRelFile(rel)
		if !ok {
			return fmt.Errorf("illegal workspace file path %q", rel)
		}
		info, err := d.Info()
		if err != nil {
			return err
		}
		total += info.Size()
		if total > daytonaMaxWorkspaceBytes {
			return fmt.Errorf("workspace exceeds Daytona materialization limit of %d MiB", daytonaMaxWorkspaceBytes>>20)
		}
		data, err := os.ReadFile(p)
		if err != nil {
			return err
		}
		return writeDaytonaFile(ctx, w, cfg, path.Join(remoteDir, clean), data, timeout)
	})
}

func writeDaytonaFile(ctx context.Context, w warden.Engine, cfg executionprofile.DaytonaConfig, remotePath string, data []byte, timeout time.Duration) error {
	parent := path.Dir(remotePath)
	cmd := "mkdir -p " + executionprofile.ShellQuote(parent) + " && : > " + executionprofile.ShellQuote(remotePath)
	if r, err := runDaytonaCommand(ctx, w, cfg, cmd, timeout, "tool.code_exec.daytona.write", ""); err != nil {
		return err
	} else if r.ExitCode != 0 {
		return fmt.Errorf("prepare file %s failed (exit %d): %s", remotePath, r.ExitCode, installTail(r))
	}
	if len(data) == 0 {
		return nil
	}
	encoded := base64.StdEncoding.EncodeToString(data)
	for start := 0; start < len(encoded); start += daytonaMaterializeChunkBytes {
		end := start + daytonaMaterializeChunkBytes
		if end > len(encoded) {
			end = len(encoded)
		}
		chunk := encoded[start:end]
		cmd := "printf %s " + executionprofile.ShellQuote(chunk) + " | base64 -d >> " + executionprofile.ShellQuote(remotePath)
		if r, err := runDaytonaCommand(ctx, w, cfg, cmd, timeout, "tool.code_exec.daytona.write", ""); err != nil {
			return err
		} else if r.ExitCode != 0 {
			return fmt.Errorf("write file %s failed (exit %d): %s", remotePath, r.ExitCode, installTail(r))
		}
	}
	return nil
}

func runDaytonaCommand(ctx context.Context, w warden.Engine, cfg executionprofile.DaytonaConfig, command string, timeout time.Duration, actor, workDir string) (*warden.Result, error) {
	return w.Run(ctx, warden.Spec{
		Profile: warden.ProfileNone,
		Argv:    cfg.CommandArgv(command, workDir, durationSeconds(timeout)),
		Env:     daytonaClientEnv(),
		Limits: warden.Limits{
			Timeout:        timeout,
			MaxOutputBytes: MaxOutputBytes,
		},
		Actor:         actor,
		CorrelationID: warden.CorrelationFrom(ctx),
	})
}

func daytonaWorkDir(cfg executionprofile.DaytonaConfig, localDir, projectSlug string) string {
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

func durationSeconds(d time.Duration) int {
	if d <= 0 {
		return 0
	}
	return int((d + time.Second - 1) / time.Second)
}
