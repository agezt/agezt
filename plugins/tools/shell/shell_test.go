// SPDX-License-Identifier: MIT

package shell

import (
	"context"
	"encoding/json"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"
	"time"

	"github.com/agezt/agezt/kernel/bus"
	"github.com/agezt/agezt/kernel/creds"
	"github.com/agezt/agezt/kernel/executionprofile"
	"github.com/agezt/agezt/kernel/warden"
)

// fakeWarden returns a canned Result so the shell tool's output-combination
// logic (stdout+stderr ordering, truncation marker, exit-code suffix) can be
// exercised deterministically — a real command can't reliably produce a
// truncated stream or a specific exit code with both stdout and stderr.
type fakeWarden struct{ res *warden.Result }

func (f *fakeWarden) Run(context.Context, warden.Spec) (*warden.Result, error) { return f.res, nil }
func (f *fakeWarden) EffectiveProfile(p warden.Profile) warden.Profile         { return p }
func (f *fakeWarden) SetBus(*bus.Bus)                                          {}

// capturingWarden records the Spec it was asked to run, so a test can assert
// what the shell tool put on it (e.g. the correlation id read from ctx).
type capturingWarden struct {
	got     warden.Spec
	inspect func(warden.Spec)
}

func (c *capturingWarden) Run(_ context.Context, s warden.Spec) (*warden.Result, error) {
	c.got = s
	if c.inspect != nil {
		c.inspect(s)
	}
	return &warden.Result{ExitCode: 0, Stdout: []byte("ok")}, nil
}
func (c *capturingWarden) EffectiveProfile(p warden.Profile) warden.Profile { return p }
func (c *capturingWarden) SetBus(*bus.Bus)                                  {}

// TestShell_StampsRunCorrelationOnSpec: the shell tool must copy the run
// correlation from its ctx (set by the runtime via warden.WithCorrelation) onto
// the warden Spec, so warden.executed lands in the run timeline and `agt why`
// reaches it. Without it the isolation events are orphaned from the run.
func TestShell_StampsRunCorrelationOnSpec(t *testing.T) {
	cw := &capturingWarden{}
	sh := NewWithWarden(cw)
	in, _ := json.Marshal(shellInput{Command: "x"})

	ctx := warden.WithCorrelation(context.Background(), "run-CORR-123")
	if _, err := sh.Invoke(ctx, in); err != nil {
		t.Fatalf("Invoke: %v", err)
	}
	if cw.got.CorrelationID != "run-CORR-123" {
		t.Errorf("Spec.CorrelationID = %q, want the run correlation from ctx", cw.got.CorrelationID)
	}

	// No correlation on the ctx → empty, never a panic (e.g. a tool run outside a
	// kernel run, like the CLI smoke path).
	cw.got = warden.Spec{}
	if _, err := sh.Invoke(context.Background(), in); err != nil {
		t.Fatalf("Invoke (no corr): %v", err)
	}
	if cw.got.CorrelationID != "" {
		t.Errorf("Spec.CorrelationID = %q, want empty with no ctx correlation", cw.got.CorrelationID)
	}
}

func TestShell_UsesExecutionProfileOverride(t *testing.T) {
	cw := &capturingWarden{}
	sh := NewWithWarden(cw)
	in, _ := json.Marshal(shellInput{Command: "x"})

	ctx := warden.WithProfileOverride(context.Background(), warden.ProfileNone)
	if _, err := sh.Invoke(ctx, in); err != nil {
		t.Fatalf("Invoke: %v", err)
	}
	if cw.got.Profile != warden.ProfileNone {
		t.Errorf("Spec.Profile = %q, want per-run override %q", cw.got.Profile, warden.ProfileNone)
	}
}

func TestShell_AddsProfileEnvPassthrough(t *testing.T) {
	t.Setenv("SAFE_TOOL_ENV", "visible")
	t.Setenv("OPENAI_API_KEY", "sk-test")
	t.Setenv("AGEZT_WEB_PASSWORD", "internal")
	t.Setenv(executionprofile.EnvLocal, "SAFE_TOOL_ENV,OPENAI_API_KEY")
	t.Setenv(executionprofile.SecretEnvLocal, "OPENAI_API_KEY,AGEZT_WEB_PASSWORD")

	cw := &capturingWarden{}
	sh := NewWithWarden(cw)
	in, _ := json.Marshal(shellInput{Command: "x"})
	ctx := warden.WithProfileOverride(context.Background(), warden.ProfileNone)
	if _, err := sh.Invoke(ctx, in); err != nil {
		t.Fatalf("Invoke: %v", err)
	}
	joined := strings.Join(cw.got.Env, "\n")
	for _, want := range []string{"SAFE_TOOL_ENV=visible", "OPENAI_API_KEY=sk-test"} {
		if !strings.Contains(joined, want) {
			t.Fatalf("env missing %q:\n%s", want, joined)
		}
	}
	for _, leak := range []string{"AGEZT_WEB_PASSWORD", "internal"} {
		if strings.Contains(joined, leak) {
			t.Fatalf("env leaked %q:\n%s", leak, joined)
		}
	}
}

func TestShell_AddsVaultSecretFileMounts(t *testing.T) {
	baseDir := t.TempDir()
	workDir := t.TempDir()
	vault := creds.NewStore(baseDir)
	if err := vault.Load(); err != nil {
		t.Fatal(err)
	}
	if err := vault.Set("OPENAI_API_KEY", "sk-test-secret"); err != nil {
		t.Fatal(err)
	}
	if err := vault.Save(); err != nil {
		t.Fatal(err)
	}
	t.Setenv(executionprofile.SecretFilesLocal, "OPENAI_API_KEY:openai.key")

	var mountedPath string
	cw := &capturingWarden{inspect: func(s warden.Spec) {
		for _, kv := range s.Env {
			if v, ok := strings.CutPrefix(kv, "SECRET_FILE_OPENAI_API_KEY="); ok {
				mountedPath = v
			}
		}
		if mountedPath == "" {
			t.Fatalf("missing secret file env in %+v", s.Env)
		}
		data, err := os.ReadFile(mountedPath)
		if err != nil {
			t.Fatalf("secret file should exist while command runs: %v", err)
		}
		if string(data) != "sk-test-secret" {
			t.Fatalf("secret file content = %q", data)
		}
	}}
	sh := NewWithWarden(cw)
	sh.BaseDir = baseDir
	sh.WorkDir = workDir
	in, _ := json.Marshal(shellInput{Command: "x"})
	ctx := warden.WithProfileOverride(context.Background(), warden.ProfileNone)
	if _, err := sh.Invoke(ctx, in); err != nil {
		t.Fatalf("Invoke: %v", err)
	}
	if _, err := os.Stat(mountedPath); !os.IsNotExist(err) {
		t.Fatalf("secret file should be cleaned after command, stat err=%v", err)
	}
}

func TestShell_UsesSSHExecutionProfileOverride(t *testing.T) {
	cw := &capturingWarden{}
	sh := NewWithWarden(cw)
	in, _ := json.Marshal(shellInput{Command: "echo 'remote ok'"})

	ctx := executionprofile.WithSSHOverride(context.Background(), executionprofile.SSHConfig{
		Enabled:               true,
		Target:                "deploy@example.com",
		WorkDir:               "/srv/app dir",
		Port:                  "2222",
		IdentityFile:          "/keys/id_ed25519",
		StrictHostKeyChecking: "accept-new",
	})
	if _, err := sh.Invoke(ctx, in); err != nil {
		t.Fatalf("Invoke: %v", err)
	}
	got := strings.Join(cw.got.Argv, "\n")
	for _, want := range []string{
		"ssh",
		"BatchMode=yes",
		"StrictHostKeyChecking=accept-new",
		"/keys/id_ed25519",
		"2222",
		"deploy@example.com",
		"sh -lc",
		"/srv/app dir",
		"remote ok",
	} {
		if !strings.Contains(got, want) {
			t.Fatalf("ssh argv missing %q:\n%v", want, cw.got.Argv)
		}
	}
	if cw.got.Profile != warden.ProfileNone || cw.got.Actor != "tool.shell.ssh" {
		t.Fatalf("ssh spec profile/actor = %q/%q", cw.got.Profile, cw.got.Actor)
	}
}

func TestShell_UsesK8sExecutionProfileOverride(t *testing.T) {
	t.Setenv("KUBECONFIG", "/tmp/kubeconfig")
	t.Setenv("KUBE_TOKEN", "must-not-forward")
	t.Setenv("AGEZT_API_TOKEN", "must-not-forward")

	cw := &capturingWarden{}
	sh := NewWithWarden(cw)
	in, _ := json.Marshal(shellInput{Command: "echo 'cluster ok'"})

	ctx := executionprofile.WithK8sOverride(context.Background(), executionprofile.K8sConfig{
		Enabled:   true,
		Context:   "prod",
		Namespace: "agents",
		Pod:       "runner-0",
		Container: "worker",
		WorkDir:   "/workspace app",
	})
	if _, err := sh.Invoke(ctx, in); err != nil {
		t.Fatalf("Invoke: %v", err)
	}
	got := strings.Join(cw.got.Argv, "\n")
	for _, want := range []string{
		"kubectl",
		"--context",
		"prod",
		"-n",
		"agents",
		"exec",
		"runner-0",
		"-c",
		"worker",
		"sh",
		"-lc",
		"/workspace app",
		"cluster ok",
	} {
		if !strings.Contains(got, want) {
			t.Fatalf("k8s argv missing %q:\n%v", want, cw.got.Argv)
		}
	}
	if cw.got.Profile != warden.ProfileNone || cw.got.Actor != "tool.shell.k8s" {
		t.Fatalf("k8s spec profile/actor = %q/%q", cw.got.Profile, cw.got.Actor)
	}
	env := strings.Join(cw.got.Env, "\n")
	if !strings.Contains(env, "KUBECONFIG=/tmp/kubeconfig") {
		t.Fatalf("k8s env missing KUBECONFIG:\n%s", env)
	}
	for _, leak := range []string{"KUBE_TOKEN", "AGEZT_API_TOKEN", "must-not-forward"} {
		if strings.Contains(env, leak) {
			t.Fatalf("k8s env leaked %q:\n%s", leak, env)
		}
	}
}

func TestShell_UsesModalExecutionProfileOverride(t *testing.T) {
	t.Setenv("MODAL_CONFIG_PATH", "/tmp/modal.toml")
	t.Setenv("MODAL_TOKEN_SECRET", "must-not-forward")
	t.Setenv("AGEZT_API_TOKEN", "must-not-forward")

	cw := &capturingWarden{}
	sh := NewWithWarden(cw)
	in, _ := json.Marshal(shellInput{Command: "echo 'modal ok'"})

	ctx := executionprofile.WithModalOverride(context.Background(), executionprofile.ModalConfig{
		Enabled:     true,
		Ref:         "app.py::main",
		Environment: "prod",
		WorkDir:     "/workspace app",
	})
	if _, err := sh.Invoke(ctx, in); err != nil {
		t.Fatalf("Invoke: %v", err)
	}
	got := strings.Join(cw.got.Argv, "\n")
	for _, want := range []string{"modal", "shell", "--env", "prod", "app.py::main", "--cmd", "/workspace app", "modal ok", "--no-pty"} {
		if !strings.Contains(got, want) {
			t.Fatalf("modal argv missing %q:\n%v", want, cw.got.Argv)
		}
	}
	if cw.got.Profile != warden.ProfileNone || cw.got.Actor != "tool.shell.modal" {
		t.Fatalf("modal spec profile/actor = %q/%q", cw.got.Profile, cw.got.Actor)
	}
	env := strings.Join(cw.got.Env, "\n")
	if !strings.Contains(env, "MODAL_CONFIG_PATH=/tmp/modal.toml") {
		t.Fatalf("modal env missing MODAL_CONFIG_PATH:\n%s", env)
	}
	for _, leak := range []string{"MODAL_TOKEN_SECRET", "AGEZT_API_TOKEN", "must-not-forward"} {
		if strings.Contains(env, leak) {
			t.Fatalf("modal env leaked %q:\n%s", leak, env)
		}
	}
}

func TestShell_UsesDaytonaExecutionProfileOverride(t *testing.T) {
	t.Setenv("DAYTONA_CONFIG_PATH", "/tmp/daytona.toml")
	t.Setenv("DAYTONA_API_KEY", "must-not-forward")
	t.Setenv("AGEZT_API_TOKEN", "must-not-forward")

	cw := &capturingWarden{}
	sh := NewWithWarden(cw)
	in, _ := json.Marshal(shellInput{Command: "echo 'daytona ok'", TimeoutMS: 1500})

	ctx := executionprofile.WithDaytonaOverride(context.Background(), executionprofile.DaytonaConfig{
		Enabled: true,
		Sandbox: "sandbox-1",
		WorkDir: "/workspace app",
	})
	if _, err := sh.Invoke(ctx, in); err != nil {
		t.Fatalf("Invoke: %v", err)
	}
	got := strings.Join(cw.got.Argv, "\n")
	for _, want := range []string{"daytona", "exec", "sandbox-1", "--cwd", "/workspace app", "--timeout", "2", "--", "sh", "-lc", "daytona ok"} {
		if !strings.Contains(got, want) {
			t.Fatalf("daytona argv missing %q:\n%v", want, cw.got.Argv)
		}
	}
	if cw.got.Profile != warden.ProfileNone || cw.got.Actor != "tool.shell.daytona" {
		t.Fatalf("daytona spec profile/actor = %q/%q", cw.got.Profile, cw.got.Actor)
	}
	env := strings.Join(cw.got.Env, "\n")
	if !strings.Contains(env, "DAYTONA_CONFIG_PATH=/tmp/daytona.toml") {
		t.Fatalf("daytona env missing DAYTONA_CONFIG_PATH:\n%s", env)
	}
	for _, leak := range []string{"DAYTONA_API_KEY", "AGEZT_API_TOKEN", "must-not-forward"} {
		if strings.Contains(env, leak) {
			t.Fatalf("daytona env leaked %q:\n%s", leak, env)
		}
	}
}

func TestShell_CombinesStdoutThenStderr(t *testing.T) {
	sh := NewWithWarden(&fakeWarden{res: &warden.Result{
		ExitCode: 0, Stdout: []byte("out-data"), Stderr: []byte("err-data"),
	}})
	in, _ := json.Marshal(shellInput{Command: "x"})
	r, err := sh.Invoke(context.Background(), in)
	if err != nil || r.IsError {
		t.Fatalf("unexpected: err=%v isErr=%v out=%q", err, r.IsError, r.Output)
	}
	if r.Output != "out-data\nerr-data" {
		t.Errorf("combined output = %q, want stdout then stderr %q", r.Output, "out-data\nerr-data")
	}
}

func TestShell_PrependsTruncationMarker(t *testing.T) {
	sh := NewWithWarden(&fakeWarden{res: &warden.Result{
		ExitCode: 0, Stdout: []byte("partial output"), Truncated: true,
	}})
	in, _ := json.Marshal(shellInput{Command: "x"})
	r, _ := sh.Invoke(context.Background(), in)
	if !strings.HasPrefix(r.Output, "[truncated to last 64 KiB]") {
		t.Errorf("truncated output should be prefixed with the marker, got %q", r.Output)
	}
	if !strings.Contains(r.Output, "partial output") {
		t.Errorf("truncated output should still carry the retained data, got %q", r.Output)
	}
}

func TestShell_NonzeroExitAppendsCode(t *testing.T) {
	sh := NewWithWarden(&fakeWarden{res: &warden.Result{
		ExitCode: 3, Stdout: []byte("boom"),
	}})
	in, _ := json.Marshal(shellInput{Command: "x"})
	r, _ := sh.Invoke(context.Background(), in)
	if !r.IsError {
		t.Error("nonzero exit should set IsError")
	}
	if !strings.Contains(r.Output, "[exit code 3]") {
		t.Errorf("output should include the exit-code suffix, got %q", r.Output)
	}
}

func TestShell_RunsCommand(t *testing.T) {
	sh := NewWithWarden(warden.New(nil))
	cmd := "echo hello"
	if runtime.GOOS == "windows" {
		cmd = "echo hello"
	}
	in, _ := json.Marshal(shellInput{Command: cmd})
	r, err := sh.Invoke(context.Background(), in)
	if err != nil {
		t.Fatalf("Invoke: %v", err)
	}
	if r.IsError {
		t.Errorf("unexpected IsError; output=%s", r.Output)
	}
	if !strings.Contains(r.Output, "hello") {
		t.Errorf("output missing 'hello': %q", r.Output)
	}
}

// TestShell_WorkDir runs in a configured directory so the shell and file tools
// can be made to agree on "here" (M609). Uses the platform's print-CWD command.
func TestShell_WorkDir(t *testing.T) {
	dir := t.TempDir()
	sh := NewWithWarden(warden.New(nil))
	sh.WorkDir = dir
	cmd := "pwd"
	if runtime.GOOS == "windows" {
		cmd = "cd"
	}
	in, _ := json.Marshal(shellInput{Command: cmd})
	r, err := sh.Invoke(context.Background(), in)
	if err != nil {
		t.Fatalf("Invoke: %v", err)
	}
	if r.IsError {
		t.Fatalf("unexpected IsError; output=%s", r.Output)
	}
	// Resolve both sides through EvalSymlinks: t.TempDir on macOS is under
	// /var → /private/var, and `pwd` reports the resolved form.
	wantAbs, _ := filepath.EvalSymlinks(dir)
	got := strings.TrimSpace(r.Output)
	gotAbs, _ := filepath.EvalSymlinks(got)
	if !strings.EqualFold(gotAbs, wantAbs) && !strings.EqualFold(got, dir) {
		t.Errorf("shell ran in %q, want WorkDir %q", got, dir)
	}
}

func TestShell_MissingCommand_IsErrorNotFatal(t *testing.T) {
	r, err := NewWithWarden(warden.New(nil)).Invoke(context.Background(), json.RawMessage(`{}`))
	if err != nil {
		t.Fatalf("Invoke: %v", err)
	}
	if !r.IsError {
		t.Errorf("expected IsError for missing command; output=%s", r.Output)
	}
}

func TestShell_NonzeroExit_FlaggedNotPanicked(t *testing.T) {
	cmd := "exit 7"
	if runtime.GOOS == "windows" {
		cmd = "exit 7"
	}
	in, _ := json.Marshal(shellInput{Command: cmd})
	r, err := NewWithWarden(warden.New(nil)).Invoke(context.Background(), in)
	if err != nil {
		t.Fatalf("Invoke: %v", err)
	}
	if !r.IsError {
		t.Errorf("non-zero exit should set IsError")
	}
}

func TestShell_Timeout(t *testing.T) {
	// Pick a command that the wrapper shell runs directly (no fork) so
	// the ctx cancellation actually halts within WaitDelay even on Windows
	// where cmd /C can't reap child processes. A busy-loop in cmd itself
	// is reliable.
	var cmd string
	if runtime.GOOS == "windows" {
		// Endless until cmd dies; no child process.
		cmd = "for /L %i in (1,0,2) do @ver >NUL"
	} else {
		// Pure bash loop; no fork.
		cmd = "while :; do :; done"
	}
	in, _ := json.Marshal(shellInput{Command: cmd, TimeoutMS: 150})
	start := time.Now()
	r, err := NewWithWarden(warden.New(nil)).Invoke(context.Background(), in)
	elapsed := time.Since(start)
	if err != nil {
		t.Fatalf("Invoke: %v", err)
	}
	if !r.IsError {
		t.Errorf("expected timeout to set IsError; output=%s", r.Output)
	}
	if !strings.Contains(r.Output, "timed out") {
		t.Errorf("output missing 'timed out': %q", r.Output)
	}
	// Allow generous slack for ctx propagation + WaitDelay (500ms).
	if elapsed > 3*time.Second {
		t.Errorf("timeout took too long: %s", elapsed)
	}
}

func TestShell_BadJSONInput(t *testing.T) {
	_, err := NewWithWarden(warden.New(nil)).Invoke(context.Background(), json.RawMessage(`{bogus`))
	if err == nil {
		t.Errorf("expected error for malformed input JSON")
	}
}

func TestShell_Definition(t *testing.T) {
	def := NewWithWarden(warden.New(nil)).Definition()
	if def.Name != "shell" {
		t.Errorf("Name=%q want shell", def.Name)
	}
	if !strings.Contains(string(def.InputSchema), `"command"`) {
		t.Errorf("schema missing 'command' field")
	}
}

// TestShell_NegativeTimeoutMSFallsBackToDefault pins that only a POSITIVE timeout_ms
// overrides the default — a malformed negative value must NOT be passed through as a
// negative duration to warden (which could disable the timeout runaway-guard). The guard
// `in.TimeoutMS > 0` was unpinned at negatives (mutation M537: `> 0 → != 0` would forward
// a negative timeout_ms).
func TestShell_NegativeTimeoutMSFallsBackToDefault(t *testing.T) {
	cw := &capturingWarden{}
	sh := NewWithWarden(cw)
	in, _ := json.Marshal(shellInput{Command: "x", TimeoutMS: -1})
	if _, err := sh.Invoke(context.Background(), in); err != nil {
		t.Fatalf("Invoke: %v", err)
	}
	if cw.got.Limits.Timeout != DefaultTimeout {
		t.Errorf("a negative timeout_ms must fall back to DefaultTimeout, got %v", cw.got.Limits.Timeout)
	}
}
