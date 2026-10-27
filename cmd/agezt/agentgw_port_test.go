// SPDX-License-Identifier: MIT

package main

// Provenance: the daemon's contract test for the agent gateway (W3.67) — the
//             gateway is the daemon's TRANSPORT (an adapter, L5), so the
//             daemon builds it over the kernel's ports, starts it and closes
//             it.
//
//             These tests are the PROOF, and they exist because the wiring was
//             never exercised: before W3.67 the gateway was built inside
//             runtime.Open, where no test could reach it — nobody had shown
//             that it listened, that a token the `agt` CLI mints is accepted,
//             or that its audit trail ever reached the journal.

import (
	"encoding/json"
	"io"
	"net"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/agezt/agezt/kernel/agentgw"
	kernelruntime "github.com/agezt/agezt/kernel/runtime"
	"github.com/agezt/agezt/plugins/providers/mock"
)

// agentgwFixture is a running daemon gateway over a real kernel.
type agentgwFixture struct {
	dir     string
	kernel  *kernelruntime.Kernel
	gateway *agentgw.Gateway
	base    string
	closed  bool
}

// shutdown closes the fixture the way the daemon does — the gateway first,
// then the kernel — and is idempotent so a test may close it early.
func (f *agentgwFixture) shutdown() {
	if f.closed {
		return
	}
	f.closed = true
	shutdownAgentGateway(f.gateway, f.kernel)
}

// token mints a subprocess token the way the `agt` CLI does: from the
// per-install secret under the base dir, so the daemon and the CLI agree.
func (f *agentgwFixture) token(t *testing.T, caps ...string) string {
	t.Helper()
	secret, err := agentgw.ResolveTokenSecret(f.dir)
	if err != nil {
		t.Fatalf("resolve the install token secret: %v", err)
	}
	token, err := agentgw.NewTokenManager(secret).CreateToken(&agentgw.TokenClaims{
		RunID:     "run-agentgw-test",
		Caps:      caps,
		MaxRate:   600,
		MaxBurst:  60,
		ExpiresAt: time.Now().Add(10 * time.Minute),
	})
	if err != nil {
		t.Fatalf("mint a subprocess token: %v", err)
	}
	return token
}

// agentgwTestServer starts the daemon's agent gateway over a real kernel.
//
// The socket path is overridden with a TCP address on a free local port: the
// default is an abstract unix socket ("@agezt/agentgw-<rand>.sock"), which is
// Linux-only and undiscoverable by an HTTP client — so a test that wants to
// PROVE the listener is alive has to speak to it.
func agentgwTestServer(t *testing.T) *agentgwFixture {
	t.Helper()
	dir := t.TempDir()
	k, err := openAppKernel(withMemory(t, withRoster(t, kernelruntime.Config{
		BaseDir:  dir,
		Provider: mock.New(),
	})))
	if err != nil {
		t.Fatalf("openAppKernel: %v", err)
	}
	addr := reserveLocalAddr(t)
	t.Setenv(agentgwSocketEnv, addr)
	gw, err := startAgentGateway(k, dir)
	if err != nil {
		_ = k.Close()
		t.Fatalf("startAgentGateway: %v", err)
	}
	f := &agentgwFixture{dir: dir, kernel: k, gateway: gw, base: "http://" + addr}
	t.Cleanup(f.shutdown)

	// The listener is up when it answers — not when it was constructed.
	client := &http.Client{Timeout: 2 * time.Second}
	deadline := time.Now().Add(10 * time.Second)
	for time.Now().Before(deadline) {
		resp, err := client.Get(f.base + "/health")
		if err == nil {
			_ = resp.Body.Close()
			return f
		}
		time.Sleep(25 * time.Millisecond)
	}
	t.Fatalf("the daemon's agent gateway never answered on %s", addr)
	return nil
}

// reserveLocalAddr returns a free loopback address. The port is released
// before the gateway binds it, so the window is small but real.
func reserveLocalAddr(t *testing.T) string {
	t.Helper()
	ln, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatalf("reserve a local port: %v", err)
	}
	addr := ln.Addr().String()
	_ = ln.Close()
	return addr
}

// agentgwDo performs one request against the gateway.
func agentgwDo(t *testing.T, method, url, token string, body string) (int, string) {
	t.Helper()
	var rdr io.Reader
	if body != "" {
		rdr = strings.NewReader(body)
	}
	req, err := http.NewRequest(method, url, rdr)
	if err != nil {
		t.Fatalf("build the request: %v", err)
	}
	if token != "" {
		req.Header.Set("Authorization", "Bearer "+token)
	}
	resp, err := (&http.Client{Timeout: 10 * time.Second}).Do(req)
	if err != nil {
		t.Fatalf("%s %s: %v", method, url, err)
	}
	defer resp.Body.Close()
	raw, err := io.ReadAll(resp.Body)
	if err != nil {
		t.Fatalf("read the response: %v", err)
	}
	return resp.StatusCode, string(raw)
}

// TestTheDaemonWiresTheAgentGatewayAndItListens: the gateway is the daemon's
// transport, so the daemon must start it — and a listener that was never
// started answers nothing at all.
func TestTheDaemonWiresTheAgentGatewayAndItListens(t *testing.T) {
	f := agentgwTestServer(t)
	code, body := agentgwDo(t, http.MethodGet, f.base+"/health", "", "")
	if code != http.StatusOK {
		t.Fatalf("the health endpoint answered %d, want 200 — the daemon's gateway is not listening", code)
	}
	var payload map[string]string
	if err := json.Unmarshal([]byte(body), &payload); err != nil {
		t.Fatalf("the health body %q is not JSON: %v", body, err)
	}
	if payload["status"] != "healthy" {
		t.Fatalf("the health body = %q, want a healthy status", body)
	}
}

// TestTheDaemonGatewayAcceptsTheInstallSecretTheCLIMints: the daemon signs
// with the per-install secret under the base dir and the `agt` CLI mints the
// subprocess tokens from the SAME file. An ephemeral per-process key would
// sign tokens the CLI cannot verify — the SDK path would fail closed with a
// 401 that reads like a bug.
func TestTheDaemonGatewayAcceptsTheInstallSecretTheCLIMints(t *testing.T) {
	f := agentgwTestServer(t)
	token := f.token(t, string(agentgw.CapMemoryWrite), string(agentgw.CapMemorySearch))

	code, body := agentgwDo(t, http.MethodPost, f.base+"/v1/memory/write", token,
		`{"type":"fact","subject":"gateway","content":"the daemon owns the gateway"}`)
	if code != http.StatusCreated {
		t.Fatalf("the memory write answered %d, want 201 — the CLI-minted token was not accepted, or the memory is not attached: %s", code, body)
	}

	code, body = agentgwDo(t, http.MethodGet, f.base+"/v1/memory/search?q=daemon+owns", token, "")
	if code != http.StatusOK {
		t.Fatalf("the memory search answered %d, want 200: %s", code, body)
	}
	if !strings.Contains(body, "the daemon owns the gateway") {
		t.Fatalf("the memory search body %q does not carry the record the gateway wrote", body)
	}
}

// TestTheDaemonGatewayPublishesOnTheDaemonsBus: the gateway speaks FOR the
// daemon, so it publishes on the daemon's bus — and the event lands in the
// daemon's journal.
func TestTheDaemonGatewayPublishesOnTheDaemonsBus(t *testing.T) {
	f := agentgwTestServer(t)
	token := f.token(t, string(agentgw.CapEventbusPublish))

	code, body := agentgwDo(t, http.MethodPost, f.base+"/v1/eventbus/publish", token,
		`{"event":"agentgw.test.published","payload":{"hello":"gateway"}}`)
	if code != http.StatusAccepted {
		t.Fatalf("the publish answered %d, want 202 — the bus is not attached: %s", code, body)
	}

	deadline := time.Now().Add(10 * time.Second)
	for time.Now().Before(deadline) {
		events, err := f.kernel.Journal().Tail(200)
		if err == nil {
			for _, ev := range events {
				if ev.Subject == "agentgw.test.published" {
					return
				}
			}
		}
		time.Sleep(25 * time.Millisecond)
	}
	t.Fatal("the event published through the gateway never reached the daemon's journal")
}

// TestTheDaemonGatewayServesTheConfigCenterAndTheRoster: the config and agent
// endpoints exist ONLY when the gateway was given the daemon's config center
// and roster. Without them the gateway answers 503 / 404 — the routes are not
// registered at all.
func TestTheDaemonGatewayServesTheConfigCenterAndTheRoster(t *testing.T) {
	f := agentgwTestServer(t)
	token := f.token(t, string(agentgw.CapConfigList), string(agentgw.CapAgentList))

	code, body := agentgwDo(t, http.MethodGet, f.base+"/v1/config", token, "")
	if code != http.StatusOK {
		t.Fatalf("the config endpoint answered %d, want 200 — the config center is not attached: %s", code, body)
	}

	code, body = agentgwDo(t, http.MethodGet, f.base+"/v1/agent/list", token, "")
	if code != http.StatusOK {
		t.Fatalf("the agent list answered %d, want 200 — the roster is not attached: %s", code, body)
	}
}

// TestTheDaemonClosesTheGatewayBeforeTheKernel: the gateway's audit trail is
// buffered and flushed on Close, and the flush appends to the kernel's
// journal. A kernel closed first would take the journal with it and the
// buffered audit entries — the record of who asked for what — would be
// lost. So the daemon closes the gateway first, and the flushed audit record
// is on disk afterwards.
func TestTheDaemonClosesTheGatewayBeforeTheKernel(t *testing.T) {
	f := agentgwTestServer(t)
	token := f.token(t, string(agentgw.CapMemorySearch))

	code, body := agentgwDo(t, http.MethodGet, f.base+"/v1/memory/search?q=audit", token, "")
	if code != http.StatusOK {
		t.Fatalf("the memory search answered %d, want 200: %s", code, body)
	}

	f.shutdown()

	entries, err := os.ReadDir(filepath.Join(f.dir, "journal"))
	if err != nil {
		t.Fatalf("read the journal directory: %v", err)
	}
	found := false
	for _, entry := range entries {
		if entry.IsDir() {
			continue
		}
		raw, err := os.ReadFile(filepath.Join(f.dir, "journal", entry.Name()))
		if err != nil {
			t.Fatalf("read the journal segment %s: %v", entry.Name(), err)
		}
		if strings.Contains(string(raw), "agentgw.audit") {
			found = true
			break
		}
	}
	if !found {
		t.Fatal("the gateway's audit trail never reached the journal — the kernel was closed before the gateway flushed it")
	}
}
