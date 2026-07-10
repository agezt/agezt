// SPDX-License-Identifier: MIT

package codeexec

import (
	"strings"
	"testing"
	"time"

	"github.com/agezt/agezt/kernel/executionprofile"
)

func TestCodeexecCoverageSmallHelpers(t *testing.T) {
	// stripBase64Whitespace: spaces, tabs, newlines, CR removed.
	in := "AB CD\nEF\rG\tHI"
	if got := stripBase64Whitespace(in); got != "ABCDEFGHI" {
		t.Fatalf("stripBase64Whitespace = %q", got)
	}
	if got := stripBase64Whitespace(""); got != "" {
		t.Fatalf("empty strip = %q", got)
	}

	// artifactMime: known extension returns a mime.
	if got := artifactMime("foo.png", nil); got != "image/png" {
		t.Fatalf("png mime = %q", got)
	}
	if got := artifactMime("foo.txt", []byte("hi")); !strings.HasPrefix(got, "text/plain") {
		t.Fatalf("text mime = %q", got)
	}
	if got := artifactMime("noext", []byte("hi")); !strings.HasPrefix(got, "text/plain") {
		t.Fatalf("noext should default to text/plain by sniffing: %q", got)
	}
	if got := artifactMime("foo.bin", []byte{0, 1, 2, 3, 4}); !strings.HasPrefix(got, "application/octet-stream") {
		t.Fatalf("binary mime = %q", got)
	}

	// splitArtifactEnvelope branches.
	clean, payload, found, err := splitArtifactEnvelope([]byte("hello"), "BEGIN", "END")
	if err != nil || found || clean == nil || payload != "" {
		t.Fatalf("no begin = %+v %+v", clean, payload)
	}
	clean, _, found, err = splitArtifactEnvelope([]byte("BEGIN data"), "BEGIN", "END")
	if err == nil || !found || string(clean) != "BEGIN data" {
		t.Fatalf("no end = %+v err %v", clean, err)
	}
	_, payload, found, err = splitArtifactEnvelope([]byte("a BEGIN payload END b"), "BEGIN", "END")
	if err != nil || !found || payload != "payload" {
		t.Fatalf("both ends err=%v found=%v payload=%q", err, found, payload)
	}
	_, payload, found, err = splitArtifactEnvelope([]byte("a BEGIN payload END"), "BEGIN", "END")
	if err != nil || !found || payload != "payload" {
		t.Fatalf("only left err=%v found=%v payload=%q", err, found, payload)
	}
	_, payload, found, err = splitArtifactEnvelope([]byte("BEGIN payload END b"), "BEGIN", "END")
	if err != nil || !found || payload != "payload" {
		t.Fatalf("only right err=%v found=%v payload=%q", err, found, payload)
	}
}

func TestCodeexecCoverageCommandAndPathHelpers(t *testing.T) {
	if got := entryName(LangPython); got != "main.py" {
		t.Fatalf("python entry = %q", got)
	}
	if got := entryName(LangNode); got != "main.js" {
		t.Fatalf("node entry = %q", got)
	}
	if got := entryName(LangDeno); got != "main.ts" {
		t.Fatalf("deno entry = %q", got)
	}
	if got := entryName("rust"); got != "main.txt" {
		t.Fatalf("rust entry = %q", got)
	}

	if got := remoteRuntimeCommand(LangPython, "/usr/bin/python3.11"); got != "python3" {
		t.Fatalf("python3.11 = %q", got)
	}
	if got := remoteRuntimeCommand(LangPython, "/usr/bin/python"); got != "python3" {
		t.Fatalf("python (apex) = %q", got)
	}
	if got := remoteRuntimeCommand(LangNode, "/usr/bin/node"); got != "node" {
		t.Fatalf("node = %q", got)
	}
	if got := remoteRuntimeCommand(LangDeno, "/usr/bin/deno"); got != "deno" {
		t.Fatalf("deno = %q", got)
	}
	if got := remoteRuntimeCommand("ruby", "/usr/bin/ruby"); got != "ruby" {
		t.Fatalf("ruby fallback = %q", got)
	}

	if got := remotePipInstallCommand("python3", []string{"requests", "bs4"}); !strings.Contains(got, "requests") || !strings.Contains(got, "bs4") || !strings.Contains(got, pyDepsName) {
		t.Fatalf("remotePipInstallCommand = %q", got)
	}

	if got := remoteRunCommand(LangPython, "python3", "main.py", false, true); !strings.Contains(got, "PYTHONPATH=") {
		t.Fatalf("python remoteRun = %q", got)
	}
	if got := remoteRunCommand(LangDeno, "deno", "main.ts", true, false); !strings.Contains(got, "--allow-net") {
		t.Fatalf("deno remoteRun = %q", got)
	}
	if got := remoteRunCommand("rust", "cargo", "main.rs", true, false); !strings.Contains(got, "cargo") {
		t.Fatalf("rust remoteRun = %q", got)
	}

	if got := quoteCommand([]string{"a", "b", "c"}); !strings.Contains(got, "a") || !strings.Contains(got, "b") || !strings.Contains(got, "c") {
		t.Fatalf("quoteCommand = %q", got)
	}

	if got := resolveTimeout(0); got != DefaultTimeout {
		t.Fatalf("resolveTimeout(0) = %v, want %v", got, DefaultTimeout)
	}
	if got := resolveTimeout(-5); got != DefaultTimeout {
		t.Fatalf("resolveTimeout(-5) = %v", got)
	}
	if got := resolveTimeout(1000); got != 1000*time.Millisecond {
		t.Fatalf("resolveTimeout(1000) = %v", got)
	}
	// Huge value should clamp to MaxTimeout.
	if got := resolveTimeout(int64(10 * time.Hour / time.Millisecond)); got != MaxTimeout {
		t.Fatalf("resolveTimeout(huge) = %v, want %v (cap)", got, MaxTimeout)
	}
}

func TestCodeexecCoverageRemoteWorkDir(t *testing.T) {
	// remoteWorkDir: SSHConfig.WorkDir + localDir basename (project).
	cfg := executionprofile.SSHConfig{WorkDir: "/work"}
	if got := remoteWorkDir(cfg, "/local", ""); got != "/work/runs/local" {
		t.Fatalf("remoteWorkDir = %q", got)
	}
	if got := remoteWorkDir(cfg, "/local", "myproj"); got != "/work/projects/myproj" {
		t.Fatalf("remoteWorkDir project = %q", got)
	}
	// modalMountDir: prepends /mnt/ to the base.
	if got := modalMountDir("/local"); got != "/mnt/local" {
		t.Fatalf("modalMountDir = %q", got)
	}
	if got := modalMountDir("/local/"); got != "/mnt/local" {
		t.Fatalf("modalMountDir trailing / = %q", got)
	}
}

func TestCodeexecCoverageDefinition(t *testing.T) {
	tool := NewWithWarden(nil, "/sandbox", map[string]string{LangPython: "/usr/bin/python3", LangNode: "node"}, true)
	def := tool.Definition()
	if def.Name != "code_exec" {
		t.Fatalf("Name = %q", def.Name)
	}
	schema := string(def.InputSchema)
	for _, want := range []string{`"language"`, `"code"`, `"enum"`, `"python"`, `"node"`} {
		if !strings.Contains(schema, want) {
			t.Fatalf("schema missing %q, got %s", want, schema)
		}
	}
	if !strings.Contains(def.Description, "Languages: node, python") {
		t.Fatalf("description should list sorted languages: %q", def.Description)
	}
	if !strings.Contains(def.Description, "Network is ON by default") {
		t.Fatalf("description should say network on by default: %q", def.Description)
	}

	tool2 := NewWithWarden(nil, "/sandbox", map[string]string{LangPython: "py"}, false)
	def2 := tool2.Definition()
	if !strings.Contains(def2.Description, "Network is DISABLED") {
		t.Fatalf("description should say network disabled: %q", def2.Description)
	}

	tool3 := NewWithWarden(nil, "/sandbox", map[string]string{LangNode: "node", LangDeno: "deno", LangPython: "py"}, true)
	def3 := tool3.Definition()
	if !strings.Contains(def3.Description, "Languages: deno, node, python") {
		t.Fatalf("expected sorted languages, got: %q", def3.Description)
	}
}
