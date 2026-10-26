// SPDX-License-Identifier: MIT

package codeexec

// Provenance: Package codeexec: Invoke (the execution router — language detection,
//             SSH/ K8s/Modal/Daytona overrides, sandbox dir setup, code write, pip
//             install for Python, the warden.run call, artifact registration).
//             Extracted from codeexec.go during the Day-211 god-file split. Public
//             API unchanged.

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/agezt/agezt/internal/atomicfile"
	"github.com/agezt/agezt/kernel/contract/toolapi"
	"github.com/agezt/agezt/kernel/executionprofile"
	"github.com/agezt/agezt/kernel/warden"
)

func (t *Tool) Invoke(ctx context.Context, raw json.RawMessage) (toolapi.Result, error) {
	var in input
	if err := json.Unmarshal(raw, &in); err != nil {
		return toolapi.Result{}, fmt.Errorf("code_exec: parse input: %w", err)
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
	//
	// These files are READ BY THE CHILD PROCESS — the interpreter that runs
	// them, or (for the SSH/K8s/Modal/Daytona backends) the machine that
	// receives them. A write that truncates in place leaves a window in
	// which the file is short, and a short script is not an error the
	// interpreter can report: it may be a valid prefix that runs and stops
	// early. The platform's atomic writer (internal/atomicfile) replaces the
	// file instead, so the child always reads a WHOLE file — the old one or
	// the new one — and the mode is re-asserted on every write, so a file
	// left behind by an earlier run (a project directory keeps its files)
	// cannot keep a wider mode or stop the write outright.
	entry := entryName(lang)
	if err := atomicfile.WriteFile(filepath.Join(dir, entry), []byte(in.Code), 0o600); err != nil {
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
		if err := atomicfile.WriteFile(full, []byte(content), 0o600); err != nil {
			return errResult("write " + rel + ": " + err.Error()), nil
		}
	}
	if in.Stdin != "" {
		// The program reads this file, so a failed write is reported: a
		// program that silently receives no stdin reads EOF and behaves as
		// if the operator had typed nothing.
		if err := atomicfile.WriteFile(filepath.Join(dir, "stdin.txt"), []byte(in.Stdin), 0o600); err != nil {
			return errResult("write stdin: " + err.Error()), nil
		}
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
