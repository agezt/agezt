// SPDX-License-Identifier: MIT

package codeexec

import (
	"archive/tar"
	"bytes"
	"compress/gzip"
	"context"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/agezt/agezt/kernel/artifact"
	"github.com/agezt/agezt/kernel/bus"
	"github.com/agezt/agezt/kernel/creds"
	"github.com/agezt/agezt/kernel/executionprofile"
	"github.com/agezt/agezt/kernel/warden"
)

// fakeWarden captures every Spec it was handed and returns scripted Results, so
// the tool's argv/env/workdir building and result rendering are testable without
// actually running an interpreter. Satisfies warden.Engine.
type fakeWarden struct {
	last      warden.Spec
	all       []warden.Spec
	result    warden.Result   // default result
	results   []warden.Result // consumed in order (per call), then falls back to result
	calls     int
	inspect   func(warden.Spec)
	resultFor func(warden.Spec) (warden.Result, bool)
}

func (f *fakeWarden) Run(_ context.Context, s warden.Spec) (*warden.Result, error) {
	f.last = s
	f.all = append(f.all, s)
	if f.inspect != nil {
		f.inspect(s)
	}
	// Simulate `pip install --target <dir>` creating its target dir, so the
	// PYTHONPATH wiring (which keys on the dir existing) is exercised.
	for i, a := range s.Argv {
		if a == "--target" && i+1 < len(s.Argv) {
			_ = os.MkdirAll(s.Argv[i+1], 0o755)
		}
	}
	var r warden.Result
	if f.resultFor != nil {
		if scripted, ok := f.resultFor(s); ok {
			r = scripted
		} else if f.calls < len(f.results) {
			r = f.results[f.calls]
		} else {
			r = f.result
		}
	} else if f.calls < len(f.results) {
		r = f.results[f.calls]
	} else {
		r = f.result
	}
	f.calls++
	r.RequestedProfile = s.Profile
	if r.EffectiveProfile == "" {
		r.EffectiveProfile = warden.ProfileNone
	}
	return &r, nil
}
func (f *fakeWarden) EffectiveProfile(warden.Profile) warden.Profile { return warden.ProfileNone }
func (f *fakeWarden) SetBus(*bus.Bus)                                {}

func newTool(t *testing.T, runtimes map[string]string, net bool) (*Tool, *fakeWarden) {
	t.Helper()
	root := t.TempDir()
	fw := &fakeWarden{result: warden.Result{ExitCode: 0, Stdout: []byte("ok-output")}}
	tl := &Tool{Warden: fw, SandboxRoot: root, Runtimes: runtimes, NetEnabled: net, Profile: warden.ProfileNamespace}
	return tl, fw
}

type fakeCodeExecIndex struct {
	entries []artifact.Entry
	data    [][]byte
}

func (f *fakeCodeExecIndex) PutEntry(meta artifact.Entry, data []byte, createdMs int64) (artifact.Entry, error) {
	meta.ID = fmt.Sprintf("art-%d", len(f.entries)+1)
	meta.Ref = fmt.Sprintf("ref-%d", len(f.entries)+1)
	meta.Size = int64(len(data))
	meta.CreatedMs = createdMs
	f.entries = append(f.entries, meta)
	f.data = append(f.data, append([]byte(nil), data...))
	return meta, nil
}

func run(t *testing.T, tl *Tool, in map[string]any) (string, bool) {
	return runWithContext(t, tl, context.Background(), in)
}

func runWithContext(t *testing.T, tl *Tool, ctx context.Context, in map[string]any) (string, bool) {
	t.Helper()
	raw, _ := json.Marshal(in)
	res, err := tl.Invoke(ctx, raw)
	if err != nil {
		t.Fatalf("Invoke hard error: %v", err)
	}
	return res.Output, res.IsError
}

func TestPython_Argv_And_EphemeralDir(t *testing.T) {
	tl, fw := newTool(t, map[string]string{LangPython: "/usr/bin/python3"}, true)
	out, isErr := run(t, tl, map[string]any{"language": "python", "code": "print(1)"})
	if isErr {
		t.Fatalf("unexpected error result: %s", out)
	}
	if got := fw.last.Argv; len(got) != 2 || got[0] != "/usr/bin/python3" || got[1] != "main.py" {
		t.Errorf("python argv = %v, want [/usr/bin/python3 main.py]", got)
	}
	// Ephemeral run dir is under sandbox root and removed after the call.
	if !strings.HasPrefix(fw.last.WorkDir, tl.SandboxRoot) {
		t.Errorf("workdir %q not under sandbox root %q", fw.last.WorkDir, tl.SandboxRoot)
	}
	if _, err := os.Stat(fw.last.WorkDir); !os.IsNotExist(err) {
		t.Errorf("ephemeral dir %q should be removed after the run", fw.last.WorkDir)
	}
	if !strings.Contains(out, "isolation=") || !strings.Contains(out, "ok-output") {
		t.Errorf("result missing header/output: %s", out)
	}
}

func TestCodeExec_UsesExecutionProfileOverride(t *testing.T) {
	tl, fw := newTool(t, map[string]string{LangPython: "/usr/bin/python3"}, true)
	ctx := warden.WithProfileOverride(context.Background(), warden.ProfileNone)
	out, isErr := runWithContext(t, tl, ctx, map[string]any{"language": "python", "code": "print(1)", "packages": []any{"requests"}})
	if isErr {
		t.Fatalf("unexpected error result: %s", out)
	}
	if len(fw.all) != 2 {
		t.Fatalf("want pip install + program run, got %d warden calls", len(fw.all))
	}
	for i, spec := range fw.all {
		if spec.Profile != warden.ProfileNone {
			t.Errorf("call %d Profile = %q, want override %q", i, spec.Profile, warden.ProfileNone)
		}
	}
}

func TestCodeExec_UsesSSHExecutionProfileRemoteWorkspace(t *testing.T) {
	tl, fw := newTool(t, map[string]string{LangPython: "/usr/bin/python3"}, true)
	ctx := executionprofile.WithSSHOverride(context.Background(), executionprofile.SSHConfig{
		Enabled: true,
		Target:  "deploy@example.com",
		WorkDir: "/srv/agezt",
	})
	out, isErr := runWithContext(t, tl, ctx, map[string]any{"language": "python", "code": "print(1)"})
	if isErr || !strings.Contains(out, "isolation=ssh") || !strings.Contains(out, "remote_dir=/srv/agezt/runs/") {
		t.Fatalf("ssh profile should run remotely, isErr=%v out=%q", isErr, out)
	}
	if fw.calls != 4 {
		t.Fatalf("want prepare + upload + run + cleanup, got %d calls: %+v", fw.calls, fw.all)
	}
	if fw.all[0].Argv[0] != "ssh" || !strings.Contains(strings.Join(fw.all[0].Argv, " "), "mkdir -p") {
		t.Fatalf("prepare argv wrong: %+v", fw.all[0].Argv)
	}
	if fw.all[1].Argv[0] != "scp" || !strings.Contains(strings.Join(fw.all[1].Argv, " "), "deploy@example.com:") {
		t.Fatalf("upload argv wrong: %+v", fw.all[1].Argv)
	}
	if fw.all[2].Argv[0] != "ssh" || !strings.Contains(strings.Join(fw.all[2].Argv, " "), "python3") {
		t.Fatalf("run argv wrong: %+v", fw.all[2].Argv)
	}
	for i, spec := range fw.all {
		if spec.Profile != warden.ProfileNone {
			t.Fatalf("ssh call %d profile = %q, want none", i, spec.Profile)
		}
	}
}

func TestCodeExec_ExportsSSHArtifacts(t *testing.T) {
	idx := &fakeCodeExecIndex{}
	tl, fw := newTool(t, map[string]string{LangPython: "/usr/bin/python3"}, true)
	tl.SetIndex(idx)
	tl.Now = func() int64 { return 1234 }
	fw.inspect = func(s warden.Spec) {
		if s.Actor != "tool.code_exec.ssh.artifacts.download" {
			return
		}
		dest := s.Argv[len(s.Argv)-1]
		if err := os.WriteFile(filepath.Join(dest, "report.txt"), []byte("remote report"), 0o600); err != nil {
			t.Fatalf("write fake ssh artifact: %v", err)
		}
	}
	ctx := warden.WithCorrelation(context.Background(), "run-ssh-art")
	ctx = executionprofile.WithSSHOverride(ctx, executionprofile.SSHConfig{
		Enabled: true,
		Target:  "deploy@example.com",
		WorkDir: "/srv/agezt",
	})
	out, isErr := runWithContext(t, tl, ctx, map[string]any{"language": "python", "code": "print(1)"})
	if isErr || !strings.Contains(out, "[artifact_export]") || !strings.Contains(out, `"name": "report.txt"`) {
		t.Fatalf("ssh artifact export missing, isErr=%v out=%q", isErr, out)
	}
	if fw.calls != 6 {
		t.Fatalf("want prepare + upload + run + artifact check + download + cleanup, got %d calls: %+v", fw.calls, fw.all)
	}
	if got := idx.entries[0]; got.Source != "code_exec" || got.Corr != "run-ssh-art" || got.Name != "report.txt" || got.Kind != "file" {
		t.Fatalf("indexed ssh artifact wrong: %+v", got)
	}
	if string(idx.data[0]) != "remote report" {
		t.Fatalf("ssh artifact bytes = %q", string(idx.data[0]))
	}
}

func TestCodeExec_UsesK8sExecutionProfileRemoteWorkspace(t *testing.T) {
	t.Setenv("KUBECONFIG", "/tmp/kubeconfig")
	t.Setenv("KUBE_TOKEN", "must-not-forward")
	t.Setenv("AGEZT_API_TOKEN", "must-not-forward")

	tl, fw := newTool(t, map[string]string{LangPython: "/usr/bin/python3"}, true)
	ctx := executionprofile.WithK8sOverride(context.Background(), executionprofile.K8sConfig{
		Enabled:   true,
		Context:   "prod",
		Namespace: "agents",
		Pod:       "runner-0",
		Container: "worker",
		WorkDir:   "/workspace",
	})
	out, isErr := runWithContext(t, tl, ctx, map[string]any{"language": "python", "code": "print(1)"})
	if isErr || !strings.Contains(out, "isolation=k8s") || !strings.Contains(out, "remote_dir=/workspace/runs/") {
		t.Fatalf("k8s profile should run remotely, isErr=%v out=%q", isErr, out)
	}
	if fw.calls != 4 {
		t.Fatalf("want prepare + upload + run + cleanup, got %d calls: %+v", fw.calls, fw.all)
	}
	if fw.all[0].Argv[0] != "kubectl" || !strings.Contains(strings.Join(fw.all[0].Argv, " "), "mkdir -p") {
		t.Fatalf("prepare argv wrong: %+v", fw.all[0].Argv)
	}
	if fw.all[1].Argv[0] != "kubectl" || !strings.Contains(strings.Join(fw.all[1].Argv, " "), " cp ") || !strings.Contains(strings.Join(fw.all[1].Argv, " "), "runner-0:/workspace/runs/") {
		t.Fatalf("upload argv wrong: %+v", fw.all[1].Argv)
	}
	if fw.all[2].Argv[0] != "kubectl" || !strings.Contains(strings.Join(fw.all[2].Argv, " "), "python3") {
		t.Fatalf("run argv wrong: %+v", fw.all[2].Argv)
	}
	for i, spec := range fw.all {
		if spec.Profile != warden.ProfileNone {
			t.Fatalf("k8s call %d profile = %q, want none", i, spec.Profile)
		}
		joinedArgv := strings.Join(spec.Argv, " ")
		for _, want := range []string{"--context prod", "-n agents", "runner-0", "-c worker"} {
			if !strings.Contains(joinedArgv, want) {
				t.Fatalf("k8s call %d argv missing %q: %+v", i, want, spec.Argv)
			}
		}
	}
	env := strings.Join(fw.all[0].Env, "\n")
	if !strings.Contains(env, "KUBECONFIG=/tmp/kubeconfig") {
		t.Fatalf("k8s env missing KUBECONFIG:\n%s", env)
	}
	for _, leak := range []string{"KUBE_TOKEN", "AGEZT_API_TOKEN", "must-not-forward"} {
		if strings.Contains(env, leak) {
			t.Fatalf("k8s env leaked %q:\n%s", leak, env)
		}
	}
}

func TestCodeExec_ExportsK8sArtifacts(t *testing.T) {
	idx := &fakeCodeExecIndex{}
	tl, fw := newTool(t, map[string]string{LangPython: "/usr/bin/python3"}, true)
	tl.SetIndex(idx)
	tl.Now = func() int64 { return 5678 }
	fw.inspect = func(s warden.Spec) {
		if s.Actor != "tool.code_exec.k8s.artifacts.download" {
			return
		}
		var dest string
		for i, arg := range s.Argv {
			if arg == "cp" && i+2 < len(s.Argv) {
				dest = s.Argv[i+2]
				break
			}
		}
		if dest == "" {
			t.Fatalf("could not find kubectl cp destination in %+v", s.Argv)
		}
		if err := os.WriteFile(filepath.Join(dest, "plot.png"), []byte("PNGDATA"), 0o600); err != nil {
			t.Fatalf("write fake k8s artifact: %v", err)
		}
	}
	ctx := warden.WithCorrelation(context.Background(), "run-k8s-art")
	ctx = executionprofile.WithK8sOverride(ctx, executionprofile.K8sConfig{
		Enabled: true,
		Pod:     "runner-0",
		WorkDir: "/workspace",
	})
	out, isErr := runWithContext(t, tl, ctx, map[string]any{"language": "python", "code": "print(1)"})
	if isErr || !strings.Contains(out, "[artifact_export]") || !strings.Contains(out, `"name": "plot.png"`) {
		t.Fatalf("k8s artifact export missing, isErr=%v out=%q", isErr, out)
	}
	if fw.calls != 6 {
		t.Fatalf("want prepare + upload + run + artifact check + download + cleanup, got %d calls: %+v", fw.calls, fw.all)
	}
	if got := idx.entries[0]; got.Source != "code_exec" || got.Corr != "run-k8s-art" || got.Name != "plot.png" || got.Kind != "image" {
		t.Fatalf("indexed k8s artifact wrong: %+v", got)
	}
	if string(idx.data[0]) != "PNGDATA" {
		t.Fatalf("k8s artifact bytes = %q", string(idx.data[0]))
	}
}

func TestCodeExec_UsesModalExecutionProfileLocalMount(t *testing.T) {
	t.Setenv("MODAL_CONFIG_PATH", "/tmp/modal.toml")
	t.Setenv("MODAL_TOKEN_SECRET", "must-not-forward")
	t.Setenv("AGEZT_API_TOKEN", "must-not-forward")

	tl, fw := newTool(t, map[string]string{LangPython: "/usr/bin/python3"}, true)
	ctx := executionprofile.WithModalOverride(context.Background(), executionprofile.ModalConfig{
		Enabled:     true,
		Image:       "python:3.12",
		Environment: "prod",
		AddPython:   "3.12",
	})
	out, isErr := runWithContext(t, tl, ctx, map[string]any{"language": "python", "code": "print(1)"})
	if isErr || !strings.Contains(out, "isolation=modal") || !strings.Contains(out, "remote_dir=/mnt/") {
		t.Fatalf("modal profile should run remotely, isErr=%v out=%q", isErr, out)
	}
	if fw.calls != 1 {
		t.Fatalf("want one modal shell run, got %d calls: %+v", fw.calls, fw.all)
	}
	argv := strings.Join(fw.last.Argv, " ")
	for _, want := range []string{"modal", "shell", "--env prod", "--image python:3.12", "--add-python 3.12", "--add-local", "--cmd", "python3", "main.py", "--no-pty"} {
		if !strings.Contains(argv, want) {
			t.Fatalf("modal argv missing %q: %+v", want, fw.last.Argv)
		}
	}
	if fw.last.Profile != warden.ProfileNone || fw.last.Actor != "tool.code_exec.modal.run" {
		t.Fatalf("modal spec profile/actor = %q/%q", fw.last.Profile, fw.last.Actor)
	}
	env := strings.Join(fw.last.Env, "\n")
	if !strings.Contains(env, "MODAL_CONFIG_PATH=/tmp/modal.toml") {
		t.Fatalf("modal env missing MODAL_CONFIG_PATH:\n%s", env)
	}
	for _, leak := range []string{"MODAL_TOKEN_SECRET", "AGEZT_API_TOKEN", "must-not-forward"} {
		if strings.Contains(env, leak) {
			t.Fatalf("modal env leaked %q:\n%s", leak, env)
		}
	}
}

func TestCodeExec_ExportsModalArtifacts(t *testing.T) {
	idx := &fakeCodeExecIndex{}
	tl, fw := newTool(t, map[string]string{LangPython: "/usr/bin/python3"}, true)
	tl.SetIndex(idx)
	tl.Now = func() int64 { return 2468 }
	fw.resultFor = func(s warden.Spec) (warden.Result, bool) {
		if s.Actor == "tool.code_exec.modal.run" {
			out := "ok-output\n" + modalArtifactBegin + "\n" + testTarGzB64(t, map[string]string{"chart.txt": "modal artifact"}) + "\n" + modalArtifactEnd + "\n"
			return warden.Result{ExitCode: 0, Stdout: []byte(out)}, true
		}
		return warden.Result{}, false
	}
	ctx := warden.WithCorrelation(context.Background(), "run-modal-art")
	ctx = executionprofile.WithModalOverride(ctx, executionprofile.ModalConfig{
		Enabled: true,
		Image:   "python:3.12",
	})
	out, isErr := runWithContext(t, tl, ctx, map[string]any{"language": "python", "code": "print(1)"})
	if isErr || !strings.Contains(out, "[artifact_export]") || !strings.Contains(out, `"name": "chart.txt"`) {
		t.Fatalf("modal artifact export missing, isErr=%v out=%q", isErr, out)
	}
	if strings.Contains(out, modalArtifactBegin) || strings.Contains(out, modalArtifactEnd) {
		t.Fatalf("modal artifact envelope leaked into rendered output: %q", out)
	}
	if !strings.Contains(strings.Join(fw.last.Argv, " "), "tar -C") {
		t.Fatalf("modal command did not include artifact archive wrapper: %+v", fw.last.Argv)
	}
	if got := idx.entries[0]; got.Source != "code_exec" || got.Corr != "run-modal-art" || got.Name != "chart.txt" || got.Kind != "file" {
		t.Fatalf("indexed modal artifact wrong: %+v", got)
	}
	if string(idx.data[0]) != "modal artifact" {
		t.Fatalf("modal artifact bytes = %q", string(idx.data[0]))
	}
}

func TestCodeExec_UsesDaytonaExecutionProfileMaterializedWorkspace(t *testing.T) {
	t.Setenv("DAYTONA_CONFIG_PATH", "/tmp/daytona.toml")
	t.Setenv("DAYTONA_TOKEN_SECRET", "must-not-forward")
	t.Setenv("AGEZT_API_TOKEN", "must-not-forward")

	tl, fw := newTool(t, map[string]string{LangPython: "/usr/bin/python3"}, true)
	ctx := executionprofile.WithDaytonaOverride(context.Background(), executionprofile.DaytonaConfig{
		Enabled: true,
		Sandbox: "sandbox-1",
		WorkDir: "/workspace",
	})
	out, isErr := runWithContext(t, tl, ctx, map[string]any{
		"language": "python",
		"code":     "import util\nprint('daytona')",
		"stdin":    "hello",
		"files": map[string]string{
			"util.py": "VALUE = 1\n",
		},
	})
	if isErr || !strings.Contains(out, "isolation=daytona") || !strings.Contains(out, "remote_dir=/workspace/runs/") {
		t.Fatalf("daytona profile should run remotely, isErr=%v out=%q", isErr, out)
	}
	var runSpec *warden.Spec
	var sawWrite bool
	for i := range fw.all {
		spec := &fw.all[i]
		if spec.Actor == "tool.code_exec.daytona.run" {
			runSpec = spec
		}
		if spec.Actor == "tool.code_exec.daytona.write" && strings.Contains(strings.Join(spec.Argv, " "), "base64 -d") {
			sawWrite = true
		}
		if spec.Profile != warden.ProfileNone {
			t.Fatalf("daytona call %d profile = %q, want none", i, spec.Profile)
		}
	}
	if runSpec == nil {
		t.Fatalf("missing daytona run spec: %+v", fw.all)
	}
	runArgv := strings.Join(runSpec.Argv, " ")
	for _, want := range []string{"daytona", "exec", "sandbox-1", "--cwd", "/workspace/runs/", "--timeout", "120", "--", "sh", "-lc", "python3", "main.py"} {
		if !strings.Contains(runArgv, want) {
			t.Fatalf("daytona run argv missing %q: %+v", want, runSpec.Argv)
		}
	}
	if !sawWrite {
		t.Fatalf("daytona workspace was not materialized with base64 writes: %+v", fw.all)
	}
	env := strings.Join(runSpec.Env, "\n")
	if !strings.Contains(env, "DAYTONA_CONFIG_PATH=/tmp/daytona.toml") {
		t.Fatalf("daytona env missing DAYTONA_CONFIG_PATH:\n%s", env)
	}
	for _, leak := range []string{"DAYTONA_TOKEN_SECRET", "AGEZT_API_TOKEN", "must-not-forward"} {
		if strings.Contains(env, leak) {
			t.Fatalf("daytona env leaked %q:\n%s", leak, env)
		}
	}
}

func TestCodeExec_ExportsDaytonaArtifacts(t *testing.T) {
	idx := &fakeCodeExecIndex{}
	tl, fw := newTool(t, map[string]string{LangPython: "/usr/bin/python3"}, true)
	tl.SetIndex(idx)
	tl.Now = func() int64 { return 6789 }
	fw.resultFor = func(s warden.Spec) (warden.Result, bool) {
		if s.Actor == "tool.code_exec.daytona.artifacts.download" {
			return warden.Result{ExitCode: 0, Stdout: []byte(testTarGzB64(t, map[string]string{"result.txt": "daytona artifact"}))}, true
		}
		return warden.Result{}, false
	}
	ctx := warden.WithCorrelation(context.Background(), "run-daytona-art")
	ctx = executionprofile.WithDaytonaOverride(ctx, executionprofile.DaytonaConfig{
		Enabled: true,
		Sandbox: "sandbox-1",
		WorkDir: "/workspace",
	})
	out, isErr := runWithContext(t, tl, ctx, map[string]any{"language": "python", "code": "print(1)"})
	if isErr || !strings.Contains(out, "[artifact_export]") || !strings.Contains(out, `"name": "result.txt"`) {
		t.Fatalf("daytona artifact export missing, isErr=%v out=%q", isErr, out)
	}
	var sawDownload bool
	for _, spec := range fw.all {
		if spec.Actor == "tool.code_exec.daytona.artifacts.download" {
			sawDownload = true
			joined := strings.Join(spec.Argv, " ")
			if !strings.Contains(joined, "tar -C") || !strings.Contains(joined, ".agezt-artifacts") {
				t.Fatalf("daytona artifact download argv wrong: %+v", spec.Argv)
			}
		}
	}
	if !sawDownload {
		t.Fatalf("missing daytona artifact download call: %+v", fw.all)
	}
	if got := idx.entries[0]; got.Source != "code_exec" || got.Corr != "run-daytona-art" || got.Name != "result.txt" || got.Kind != "file" {
		t.Fatalf("indexed daytona artifact wrong: %+v", got)
	}
	if string(idx.data[0]) != "daytona artifact" {
		t.Fatalf("daytona artifact bytes = %q", string(idx.data[0]))
	}
}

func testTarGzB64(t *testing.T, files map[string]string) string {
	t.Helper()
	var buf bytes.Buffer
	gz := gzip.NewWriter(&buf)
	tw := tar.NewWriter(gz)
	for name, body := range files {
		data := []byte(body)
		if err := tw.WriteHeader(&tar.Header{Name: name, Mode: 0o600, Size: int64(len(data))}); err != nil {
			t.Fatalf("tar header: %v", err)
		}
		if _, err := tw.Write(data); err != nil {
			t.Fatalf("tar write: %v", err)
		}
	}
	if err := tw.Close(); err != nil {
		t.Fatalf("tar close: %v", err)
	}
	if err := gz.Close(); err != nil {
		t.Fatalf("gzip close: %v", err)
	}
	return base64.StdEncoding.EncodeToString(buf.Bytes())
}

func TestCodeExec_ExportsLocalArtifacts(t *testing.T) {
	idx := &fakeCodeExecIndex{}
	tl, fw := newTool(t, map[string]string{LangPython: "/usr/bin/python3"}, true)
	tl.SetIndex(idx)
	tl.Now = func() int64 { return 42 }
	fw.inspect = func(s warden.Spec) {
		if s.Actor != "tool.code_exec" {
			return
		}
		dir := filepath.Join(s.WorkDir, artifactExportDir, "nested")
		if err := os.MkdirAll(dir, 0o700); err != nil {
			t.Fatalf("mkdir fake artifact dir: %v", err)
		}
		if err := os.WriteFile(filepath.Join(dir, "result.txt"), []byte("local result"), 0o600); err != nil {
			t.Fatalf("write fake local artifact: %v", err)
		}
	}
	ctx := warden.WithCorrelation(context.Background(), "run-local-art")
	out, isErr := runWithContext(t, tl, ctx, map[string]any{"language": "python", "code": "print(1)"})
	if isErr || !strings.Contains(out, "[artifact_export]") || !strings.Contains(out, `"name": "nested/result.txt"`) {
		t.Fatalf("local artifact export missing, isErr=%v out=%q", isErr, out)
	}
	if got := idx.entries[0]; got.Source != "code_exec" || got.Corr != "run-local-art" || got.Name != "nested/result.txt" || got.Kind != "file" {
		t.Fatalf("indexed local artifact wrong: %+v", got)
	}
	if string(idx.data[0]) != "local result" {
		t.Fatalf("local artifact bytes = %q", string(idx.data[0]))
	}
}

func TestDeno_PermissionFlags_NetOnOff(t *testing.T) {
	// Net on (default): --allow-net present, fs confined to the work dir.
	tl, fw := newTool(t, map[string]string{LangDeno: "/usr/bin/deno"}, true)
	run(t, tl, map[string]any{"language": "deno", "code": "console.log(1)"})
	argv := strings.Join(fw.last.Argv, " ")
	for _, want := range []string{"run", "--quiet", "--no-prompt", "--allow-read=", "--allow-write=", "--allow-env", "--allow-net", "main.ts"} {
		if !strings.Contains(argv, want) {
			t.Errorf("deno argv missing %q: %s", want, argv)
		}
	}
	if strings.Contains(argv, "--allow-run") {
		t.Errorf("deno must NOT get --allow-run (subprocess escape): %s", argv)
	}

	// allow_net:false → no --allow-net.
	f := false
	tl2, fw2 := newTool(t, map[string]string{LangDeno: "/usr/bin/deno"}, true)
	run(t, tl2, map[string]any{"language": "deno", "code": "x", "allow_net": f})
	if strings.Contains(strings.Join(fw2.last.Argv, " "), "--allow-net") {
		t.Errorf("allow_net:false must drop --allow-net: %v", fw2.last.Argv)
	}

	// Daemon-level net disabled → no --allow-net even if the call asks.
	tl3, fw3 := newTool(t, map[string]string{LangDeno: "/usr/bin/deno"}, false)
	run(t, tl3, map[string]any{"language": "deno", "code": "x", "allow_net": true})
	if strings.Contains(strings.Join(fw3.last.Argv, " "), "--allow-net") {
		t.Errorf("NetEnabled=false must drop --allow-net regardless of allow_net: %v", fw3.last.Argv)
	}
}

func TestPythonCandidates_PrefersRealPythonOnWindows(t *testing.T) {
	// On Windows, `python` (a real install) must be tried before `python3` (the
	// Store shim that can trigger the auto-installer and pollute output).
	if got := pythonCandidates("windows"); len(got) != 2 || got[0] != "python" || got[1] != "python3" {
		t.Errorf("windows candidates = %v, want [python python3]", got)
	}
	// Elsewhere `python3` is canonical and tried first.
	if got := pythonCandidates("linux"); len(got) != 2 || got[0] != "python3" {
		t.Errorf("linux candidates = %v, want [python3 python]", got)
	}
}

func TestEnv_DisablesPyLauncherAutoInstall(t *testing.T) {
	t.Setenv("PATH", "/usr/bin")
	tl, fw := newTool(t, map[string]string{LangPython: "/usr/bin/python3"}, true)
	run(t, tl, map[string]any{"language": "python", "code": "x"})
	if !strings.Contains(strings.Join(fw.last.Env, "\n"), "PYLAUNCHER_ALLOW_INSTALL=0") {
		t.Errorf("env should disable the py-launcher auto-installer:\n%s", strings.Join(fw.last.Env, "\n"))
	}
}

func TestEnv_ScrubsSecrets(t *testing.T) {
	t.Setenv("AGEZT_API_KEY", "super-secret")
	t.Setenv("DEEPSEEK_API_KEY", "sk-leak")
	t.Setenv("MY_TOKEN", "tok")
	t.Setenv("AWS_SECRET_ACCESS_KEY", "aws")
	t.Setenv("PATH", "/usr/bin")

	tl, fw := newTool(t, map[string]string{LangPython: "/usr/bin/python3"}, true)
	run(t, tl, map[string]any{"language": "python", "code": "x"})
	joined := strings.Join(fw.last.Env, "\n")
	for _, leak := range []string{"super-secret", "sk-leak", "AGEZT_API_KEY", "DEEPSEEK_API_KEY", "MY_TOKEN", "AWS_SECRET_ACCESS_KEY"} {
		if strings.Contains(joined, leak) {
			t.Errorf("scrubbed env leaked %q:\n%s", leak, joined)
		}
	}
	// HOME/TMP repointed at the work dir; PATH preserved.
	if !strings.Contains(joined, "HOME="+fw.last.WorkDir) {
		t.Errorf("HOME not repointed at work dir:\n%s", joined)
	}
	if !strings.Contains(joined, "PATH=") {
		t.Errorf("PATH should be preserved:\n%s", joined)
	}
}

func TestEnv_AddsProfileEnvPassthrough(t *testing.T) {
	t.Setenv("SAFE_DOCKER_ENV", "visible")
	t.Setenv("OPENAI_API_KEY", "sk-test")
	t.Setenv("AGEZT_WEB_PASSWORD", "internal")
	t.Setenv(executionprofile.EnvDocker, "SAFE_DOCKER_ENV,OPENAI_API_KEY")
	t.Setenv(executionprofile.SecretEnvDocker, "OPENAI_API_KEY,AGEZT_WEB_PASSWORD")
	t.Setenv("PATH", "/usr/bin")

	tl, fw := newTool(t, map[string]string{LangPython: "/usr/bin/python3"}, true)
	ctx := warden.WithProfileOverride(context.Background(), warden.ProfileContainer)
	runWithContext(t, tl, ctx, map[string]any{"language": "python", "code": "x"})
	joined := strings.Join(fw.last.Env, "\n")
	for _, want := range []string{"SAFE_DOCKER_ENV=visible", "OPENAI_API_KEY=sk-test"} {
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

func TestEnv_AddsVaultSecretFileMounts(t *testing.T) {
	baseDir := t.TempDir()
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
	t.Setenv(executionprofile.SecretFilesDocker, "OPENAI_API_KEY:openai.key")

	tl, fw := newTool(t, map[string]string{LangPython: "/usr/bin/python3"}, true)
	tl.BaseDir = baseDir
	var hostPath string
	fw.inspect = func(s warden.Spec) {
		for _, kv := range s.Env {
			if v, ok := strings.CutPrefix(kv, "SECRET_FILE_OPENAI_API_KEY="); ok && v != "/workspace/.agezt-secrets/openai.key" {
				t.Fatalf("docker secret file env path = %q, want /workspace/.agezt-secrets/openai.key", v)
			}
		}
		hostPath = filepath.Join(s.WorkDir, ".agezt-secrets", "openai.key")
		data, err := os.ReadFile(hostPath)
		if err != nil {
			t.Fatalf("secret file should exist while code runs: %v", err)
		}
		if string(data) != "sk-test-secret" {
			t.Fatalf("secret file content = %q", data)
		}
	}
	ctx := warden.WithProfileOverride(context.Background(), warden.ProfileContainer)
	out, isErr := runWithContext(t, tl, ctx, map[string]any{"language": "python", "code": "x", "project": "secret project"})
	if isErr {
		t.Fatalf("unexpected error result: %s", out)
	}
	if _, err := os.Stat(hostPath); !os.IsNotExist(err) {
		t.Fatalf("secret file should be cleaned after run, stat err=%v", err)
	}
}

func TestProject_PersistsAcrossCalls(t *testing.T) {
	tl, fw := newTool(t, map[string]string{LangPython: "/usr/bin/python3"}, true)

	run(t, tl, map[string]any{"language": "python", "code": "print(1)", "project": "My Scraper!"})
	projDir := fw.last.WorkDir
	wantDir := filepath.Join(tl.SandboxRoot, "projects", "my-scraper")
	if projDir != wantDir {
		t.Fatalf("project dir = %q, want %q", projDir, wantDir)
	}
	// The entrypoint persists (not removed like an ephemeral run).
	if _, err := os.Stat(filepath.Join(projDir, "main.py")); err != nil {
		t.Errorf("project entrypoint should persist: %v", err)
	}
	// A second call with extra files lands in the SAME dir.
	run(t, tl, map[string]any{"language": "python", "code": "print(2)", "project": "my-scraper",
		"files": map[string]string{"util.py": "def f(): return 1"}})
	if fw.last.WorkDir != wantDir {
		t.Errorf("second call dir = %q, want same %q", fw.last.WorkDir, wantDir)
	}
	if _, err := os.Stat(filepath.Join(wantDir, "util.py")); err != nil {
		t.Errorf("extra file should be written into the project: %v", err)
	}
}

func TestFiles_RejectTraversal(t *testing.T) {
	tl, _ := newTool(t, map[string]string{LangPython: "/usr/bin/python3"}, true)
	for _, bad := range []string{"../escape.py", "/etc/passwd", "a/../../b", "C:\\evil"} {
		out, isErr := run(t, tl, map[string]any{"language": "python", "code": "x",
			"files": map[string]string{bad: "pwned"}})
		if !isErr || !strings.Contains(out, "illegal file name") {
			t.Errorf("file %q should be rejected, got isErr=%v out=%q", bad, isErr, out)
		}
	}
}

func TestRendering_NonZeroAndTimeout(t *testing.T) {
	// Non-zero exit → IsError with [exit code N].
	tl, fw := newTool(t, map[string]string{LangNode: "/usr/bin/node"}, true)
	fw.result = warden.Result{ExitCode: 3, Stdout: []byte("boom")}
	out, isErr := run(t, tl, map[string]any{"language": "node", "code": "process.exit(3)"})
	if !isErr || !strings.Contains(out, "[exit code 3]") || !strings.Contains(out, "boom") {
		t.Errorf("non-zero render wrong: isErr=%v out=%q", isErr, out)
	}

	// Timed out → IsError with a timeout note.
	tl2, fw2 := newTool(t, map[string]string{LangNode: "/usr/bin/node"}, true)
	fw2.result = warden.Result{ExitCode: -1, TimedOut: true}
	out2, isErr2 := run(t, tl2, map[string]any{"language": "node", "code": "while(true){}"})
	if !isErr2 || !strings.Contains(out2, "timed out") {
		t.Errorf("timeout render wrong: isErr=%v out=%q", isErr2, out2)
	}
}

func TestPackages_InstallThenRunWithPythonpath(t *testing.T) {
	tl, fw := newTool(t, map[string]string{LangPython: "/usr/bin/python3"}, true)
	out, isErr := run(t, tl, map[string]any{
		"language": "python", "code": "import requests", "packages": []any{"requests", "beautifulsoup4"},
	})
	if isErr {
		t.Fatalf("unexpected error: %s", out)
	}
	if len(fw.all) != 2 {
		t.Fatalf("want 2 warden runs (install + program), got %d", len(fw.all))
	}
	// First run is the pip install into <dir>/.deps with both packages.
	inst := strings.Join(fw.all[0].Argv, " ")
	for _, want := range []string{"/usr/bin/python3", "-m pip install", "--target", ".deps", "requests", "beautifulsoup4"} {
		if !strings.Contains(inst, want) {
			t.Errorf("install argv missing %q: %s", want, inst)
		}
	}
	// Second run is the program, with PYTHONPATH pointing at the deps dir.
	prog := fw.all[1]
	if prog.Argv[len(prog.Argv)-1] != "main.py" {
		t.Errorf("second run should be the program: %v", prog.Argv)
	}
	if !strings.Contains(strings.Join(prog.Env, "\n"), "PYTHONPATH=") {
		t.Errorf("program env should carry PYTHONPATH:\n%s", strings.Join(prog.Env, "\n"))
	}
}

func TestPackages_RejectedForNonPython(t *testing.T) {
	tl, fw := newTool(t, map[string]string{LangDeno: "/usr/bin/deno"}, true)
	out, isErr := run(t, tl, map[string]any{"language": "deno", "code": "x", "packages": []any{"cheerio"}})
	if !isErr || !strings.Contains(out, "only supported for python") {
		t.Errorf("packages on deno should be rejected: isErr=%v out=%q", isErr, out)
	}
	if len(fw.all) != 0 {
		t.Errorf("nothing should run when packages are rejected, got %d runs", len(fw.all))
	}
}

func TestPackages_InstallFailureShortCircuits(t *testing.T) {
	tl, fw := newTool(t, map[string]string{LangPython: "/usr/bin/python3"}, true)
	// First run (install) fails; the program must NOT run.
	fw.results = []warden.Result{{ExitCode: 1, Stderr: []byte("No matching distribution found for nope")}}
	out, isErr := run(t, tl, map[string]any{"language": "python", "code": "print(1)", "packages": []any{"nope"}})
	if !isErr || !strings.Contains(out, "pip install failed") {
		t.Errorf("install failure should surface: isErr=%v out=%q", isErr, out)
	}
	if len(fw.all) != 1 {
		t.Errorf("program must not run after a failed install; got %d runs", len(fw.all))
	}
}

func TestPackages_RejectsFlagInjection(t *testing.T) {
	tl, fw := newTool(t, map[string]string{LangPython: "/usr/bin/python3"}, true)
	for _, bad := range []any{"--index-url=http://evil", "-rrequirements.txt", "foo bar"} {
		out, isErr := run(t, tl, map[string]any{"language": "python", "code": "x", "packages": []any{bad}})
		if !isErr || !strings.Contains(out, "illegal package") {
			t.Errorf("package %v should be rejected: isErr=%v out=%q", bad, isErr, out)
		}
	}
	if len(fw.all) != 0 {
		t.Errorf("a rejected package list must run nothing, got %d", len(fw.all))
	}
}

func TestPackages_NetDisabledBlocksInstall(t *testing.T) {
	tl, _ := newTool(t, map[string]string{LangPython: "/usr/bin/python3"}, false) // NetEnabled=false
	out, isErr := run(t, tl, map[string]any{"language": "python", "code": "x", "packages": []any{"requests"}})
	if !isErr || !strings.Contains(out, "network is disabled") {
		t.Errorf("install with net disabled should error: isErr=%v out=%q", isErr, out)
	}
}

func TestProject_DepsPersistAcrossCalls(t *testing.T) {
	tl, fw := newTool(t, map[string]string{LangPython: "/usr/bin/python3"}, true)
	// Pre-create the project's .deps (as a prior install would have).
	depsDir := filepath.Join(tl.SandboxRoot, "projects", "scraper", pyDepsName)
	if err := os.MkdirAll(depsDir, 0o755); err != nil {
		t.Fatal(err)
	}
	// A later call with NO packages still gets PYTHONPATH for the persisted deps.
	run(t, tl, map[string]any{"language": "python", "code": "import requests", "project": "scraper"})
	if len(fw.all) != 1 {
		t.Fatalf("want 1 run (no install), got %d", len(fw.all))
	}
	if !strings.Contains(strings.Join(fw.all[0].Env, "\n"), "PYTHONPATH="+depsDir) {
		t.Errorf("persisted project deps should be on PYTHONPATH:\n%s", strings.Join(fw.all[0].Env, "\n"))
	}
}

func TestUnavailableLanguage(t *testing.T) {
	tl, _ := newTool(t, map[string]string{LangPython: "/usr/bin/python3"}, true)
	out, isErr := run(t, tl, map[string]any{"language": "ruby", "code": "puts 1"})
	if !isErr || !strings.Contains(out, "not available") {
		t.Errorf("unavailable language should error: isErr=%v out=%q", isErr, out)
	}
}

func TestDefinition_ReflectsDetectedLanguages(t *testing.T) {
	tl, _ := newTool(t, map[string]string{LangPython: "/p", LangDeno: "/d"}, true)
	def := tl.Definition()
	if def.Name != "code_exec" {
		t.Fatalf("name = %q", def.Name)
	}
	schema := string(def.InputSchema)
	if !strings.Contains(schema, `"python"`) || !strings.Contains(schema, `"deno"`) {
		t.Errorf("schema enum should list detected languages: %s", schema)
	}
	if strings.Contains(schema, `"node"`) {
		t.Errorf("schema enum should NOT list node (not detected): %s", schema)
	}
}
