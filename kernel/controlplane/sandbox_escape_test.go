// SPDX-License-Identifier: MIT

package controlplane

// Round-4 elite-bug-hunter reproduction, promoted to a durable containment
// pin. handleSandboxFile confined paths lexically (confineUnder: Clean +
// prefix check) but read them with follow-opens (os.Stat + os.ReadFile). The
// projects tree is agent-built content, so a planted link — a POSIX symlink,
// or a Windows junction (os.Lstat reports ModeIrregular, invisible to
// ModeSymlink checks, and filepath.EvalSymlinks does not resolve through it)
// — turned this operator inspection endpoint into an arbitrary-file read
// crossing the projects root. Deterministic; no swap race required.

import (
	"encoding/json"
	"net"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"
	"testing"
	"time"

	"github.com/agezt/agezt/kernel/agent"
	ageruntime "github.com/agezt/agezt/kernel/runtime"
)

// nopProvider satisfies runtime.Config's non-nil Provider requirement without
// supplying behavior; Open only stores the provider during init.
type nopProvider struct {
	agent.Provider
}

func newSandboxTestServer(t *testing.T) *Server {
	t.Helper()
	base := t.TempDir()
	k, err := ageruntime.Open(ageruntime.Config{BaseDir: base, Provider: nopProvider{}})
	if err != nil {
		t.Fatalf("runtime.Open: %v", err)
	}
	t.Cleanup(func() { _ = k.Close() })
	return &Server{k: k, baseDir: base}
}

func callSandboxFile(t *testing.T, s *Server, project, file string) Response {
	t.Helper()
	client, server := net.Pipe()
	go func() {
		defer server.Close()
		s.handleSandboxFile(server, Request{ID: "r4", Args: map[string]any{"project": project, "file": file}})
	}()
	defer client.Close()
	_ = client.SetReadDeadline(time.Now().Add(10 * time.Second))
	var resp Response
	if err := json.NewDecoder(client).Decode(&resp); err != nil {
		t.Fatalf("decode response: %v", err)
	}
	return resp
}

func sandboxContent(t *testing.T, resp Response) string {
	t.Helper()
	if resp.Result == nil {
		return resp.Error
	}
	if c, ok := resp.Result["content"].(string); ok {
		return c
	}
	b, err := json.Marshal(resp.Result)
	if err != nil {
		t.Fatalf("marshal result: %v", err)
	}
	return string(b)
}

func TestSandboxFile_PlantedLinkEscapeFailsClosed(t *testing.T) {
	s := newSandboxTestServer(t)
	base := s.baseDir
	proj := filepath.Join(base, "sandbox", "projects", "demo")
	if err := os.MkdirAll(proj, 0o755); err != nil {
		t.Fatalf("mkdir: %v", err)
	}
	outside := filepath.Join(base, "outside")
	if err := os.MkdirAll(outside, 0o755); err != nil {
		t.Fatalf("mkdir: %v", err)
	}
	if err := os.WriteFile(filepath.Join(proj, "innocent.txt"), []byte("hello"), 0o644); err != nil {
		t.Fatalf("write: %v", err)
	}
	secret := filepath.Join(outside, "secret.txt")
	if err := os.WriteFile(secret, []byte("TOPSECRET"), 0o644); err != nil {
		t.Fatalf("write: %v", err)
	}

	// Positive control: a regular in-project file must read normally.
	resp := callSandboxFile(t, s, "demo", "innocent.txt")
	if resp.Error != "" || !strings.Contains(sandboxContent(t, resp), "hello") {
		t.Fatalf("positive control failed: %+v (%s)", resp, sandboxContent(t, resp))
	}

	// Escape attempt 1: POSIX file symlink planted in the projects tree.
	link := filepath.Join(proj, "leak.txt")
	if err := os.Symlink(secret, link); err == nil {
		resp := callSandboxFile(t, s, "demo", "leak.txt")
		if strings.Contains(sandboxContent(t, resp), "TOPSECRET") {
			t.Fatalf("symlink leak: sandbox read crossed the projects root: %+v (%s)", resp, sandboxContent(t, resp))
		}
	} else {
		t.Logf("os.Symlink unavailable here (%v); the junction attempt covers this platform", err)
	}

	// Escape attempt 2: Windows junction (creatable without admin).
	if runtime.GOOS == "windows" {
		junc := filepath.Join(proj, "out")
		if out, err := exec.Command("cmd", "/c", "mklink", "/J", junc, outside).CombinedOutput(); err != nil {
			t.Skipf("mklink /J unavailable: %v: %s", err, out)
		}
		resp := callSandboxFile(t, s, "demo", "out/secret.txt")
		if strings.Contains(sandboxContent(t, resp), "TOPSECRET") {
			t.Fatalf("junction leak: sandbox read crossed the projects root: %+v (%s)", resp, sandboxContent(t, resp))
		}
	}
}
