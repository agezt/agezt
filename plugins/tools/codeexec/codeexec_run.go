// SPDX-License-Identifier: MIT

package codeexec

// Script-run side of the codeexec tool: RunScript + workDir +
// resolveTimeout + render + renderRemote + renderRemoteProfile +
// publish + errResult. Carved out of codeexec.go during the Day 182
// god-file split so the main file can stay focused on constants +
// Tool struct + lifecycle + Invoke.
// Public API unchanged.

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/agezt/agezt/kernel/agent"
	"github.com/agezt/agezt/kernel/event"
	"github.com/agezt/agezt/kernel/warden"
)

func (t *Tool) RunScript(ctx context.Context, language, code, inputJSON string) (string, bool, error) {
	raw, err := json.Marshal(input{Language: language, Code: code, Stdin: inputJSON})
	if err != nil {
		return "", false, fmt.Errorf("code_exec: marshal script input: %w", err)
	}
	res, err := t.Invoke(ctx, raw)
	if err != nil {
		return "", false, err
	}
	return res.Output, res.IsError, nil
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

func resolveTimeout(timeoutMS int64) time.Duration {
	timeout := DefaultTimeout
	if timeoutMS > 0 {
		timeout = time.Duration(timeoutMS) * time.Millisecond
	}
	if timeout > MaxTimeout {
		timeout = MaxTimeout
	}
	return timeout
}

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

func renderRemote(lang, projectSlug, remoteDir string, timeout time.Duration, res *warden.Result) agent.Result {
	return renderRemoteProfile("ssh", lang, projectSlug, remoteDir, timeout, res)
}

func renderRemoteProfile(profile, lang, projectSlug, remoteDir string, timeout time.Duration, res *warden.Result) agent.Result {
	var head strings.Builder
	fmt.Fprintf(&head, "[code_exec] language=%s isolation=%s remote_dir=%s", lang, profile, remoteDir)
	if projectSlug != "" {
		fmt.Fprintf(&head, " project=%s", projectSlug)
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
