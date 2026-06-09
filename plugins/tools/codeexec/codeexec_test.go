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

// fakeWarden captures the Spec it was handed and returns a scripted Result, so
// the tool's argv/env/workdir building and result rendering are testable without
// actually running an interpreter. Satisfies warden.Engine.
type fakeWarden struct {
	last   warden.Spec
	result warden.Result
}

func (f *fakeWarden) Run(_ context.Context, s warden.Spec) (*warden.Result, error) {
	f.last = s
	r := f.result
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
