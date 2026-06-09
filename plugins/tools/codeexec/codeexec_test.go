// SPDX-License-Identifier: MIT

package codeexec

import (
	"context"
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/agezt/agezt/kernel/bus"
	"github.com/agezt/agezt/kernel/warden"
)

// fakeWarden captures every Spec it was handed and returns scripted Results, so
// the tool's argv/env/workdir building and result rendering are testable without
// actually running an interpreter. Satisfies warden.Engine.
type fakeWarden struct {
	last    warden.Spec
	all     []warden.Spec
	result  warden.Result   // default result
	results []warden.Result // consumed in order (per call), then falls back to result
	calls   int
}

func (f *fakeWarden) Run(_ context.Context, s warden.Spec) (*warden.Result, error) {
	f.last = s
	f.all = append(f.all, s)
	// Simulate `pip install --target <dir>` creating its target dir, so the
	// PYTHONPATH wiring (which keys on the dir existing) is exercised.
	for i, a := range s.Argv {
		if a == "--target" && i+1 < len(s.Argv) {
			_ = os.MkdirAll(s.Argv[i+1], 0o755)
		}
	}
	var r warden.Result
	if f.calls < len(f.results) {
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

func run(t *testing.T, tl *Tool, in map[string]any) (string, bool) {
	t.Helper()
	raw, _ := json.Marshal(in)
	res, err := tl.Invoke(context.Background(), raw)
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

func TestDeno_PermissionFlags_NetOnOff(t *testing.T) {
	// Net on (default): --allow-net present, fs confined to the work dir.
	tl, fw := newTool(t, map[string]string{LangDeno: "/usr/bin/deno"}, true)
	run(t, tl, map[string]any{"language": "deno", "code": "console.log(1)"})
	argv := strings.Join(fw.last.Argv, " ")
	for _, want := range []string{"run", "--no-prompt", "--allow-read=", "--allow-write=", "--allow-env", "--allow-net", "main.ts"} {
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
