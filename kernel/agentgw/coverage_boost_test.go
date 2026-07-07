// SPDX-License-Identifier: MIT

package agentgw

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/agezt/agezt/kernel/configcenter"
)

// doAuthedGet issues a Bearer-authenticated GET and returns the response.
// The caller is responsible for closing resp.Body.
func doAuthedGet(t *testing.T, url, token string) *http.Response {
	t.Helper()
	req, err := http.NewRequest(http.MethodGet, url, nil)
	if err != nil {
		t.Fatalf("NewRequest: %v", err)
	}
	req.Header.Set("Authorization", "Bearer "+token)
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatalf("Do: %v", err)
	}
	return resp
}

func newTestCtx() context.Context { return context.Background() }

// newGatewayWithConfig builds a gateway wired to an all-auto config center
// and returns the gateway plus a helper to mint tokens with given caps.
func newGatewayWithConfig(t *testing.T) *Gateway {
	t.Helper()
	gw := NewGateway(DefaultGatewayConfig(t.TempDir()))
	cfg := configcenter.DefaultConfig(t.TempDir())
	cfg.AccessPolicies = map[configcenter.Rating]configcenter.Policy{
		configcenter.RatingPublic:     configcenter.PolicyAuto,
		configcenter.RatingInternal:   configcenter.PolicyAuto,
		configcenter.RatingSecret:     configcenter.PolicyAuto,
		configcenter.RatingRestricted: configcenter.PolicyAuto,
	}
	center, err := configcenter.New(cfg)
	if err != nil {
		t.Fatalf("configcenter.New: %v", err)
	}
	t.Cleanup(func() { _ = center.Close() })
	gw.SetConfigCenter(center)
	return gw
}

func mkToken(t *testing.T, gw *Gateway, agent string, caps ...string) string {
	t.Helper()
	token, err := gw.tokenMgr.CreateToken(&TokenClaims{
		RunID:        "run-" + agent,
		SubprocessID: agent,
		Caps:         caps,
		ExpiresAt:    time.Now().Add(time.Hour),
	})
	if err != nil {
		t.Fatalf("CreateToken: %v", err)
	}
	return token
}

// TestHandleConfigGet exercises the handleConfigGet success path (value
// returned), the missing-key branch, and the not-found branch.
func TestHandleConfigGet(t *testing.T) {
	gw := newGatewayWithConfig(t)

	entry := configcenter.NewConfigEntry("svc:endpoint", "https://api.example.com")
	entry.Rating = configcenter.RatingPublic
	if err := gw.configCenter.Set(entry); err != nil {
		t.Fatalf("Set: %v", err)
	}

	token := mkToken(t, gw, "agent-1", "config.access")

	mux := http.NewServeMux()
	mux.HandleFunc("GET /v1/config/", gw.withAuth(gw.configHandler.handleConfigGet))
	srv := httptest.NewServer(mux)
	t.Cleanup(srv.Close)

	// Success: existing public key.
	resp := doAuthedGet(t, srv.URL+"/v1/config/svc:endpoint", token)
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("Get existing key status = %d, want 200", resp.StatusCode)
	}
	var body map[string]any
	_ = json.NewDecoder(resp.Body).Decode(&body)
	resp.Body.Close()
	if body["value"] != "https://api.example.com" {
		t.Fatalf("Get value = %v, want stored endpoint", body["value"])
	}

	// Not found: missing key -> 404.
	nf := doAuthedGet(t, srv.URL+"/v1/config/does:not:exist", token)
	if nf.StatusCode != http.StatusNotFound {
		t.Fatalf("Get missing key status = %d, want 404", nf.StatusCode)
	}
	nf.Body.Close()

	// Missing key (empty after trim) -> 400.
	empty := doAuthedGet(t, srv.URL+"/v1/config/", token)
	if empty.StatusCode != http.StatusBadRequest {
		t.Fatalf("Get empty key status = %d, want 400", empty.StatusCode)
	}
	empty.Body.Close()

	// Forbidden: token without config.access cap.
	noCap := mkToken(t, gw, "agent-2", "config.list")
	forbidden := doAuthedGet(t, srv.URL+"/v1/config/svc:endpoint", noCap)
	if forbidden.StatusCode != http.StatusForbidden {
		t.Fatalf("Get without cap status = %d, want 403", forbidden.StatusCode)
	}
	forbidden.Body.Close()
}

// TestHandleConfigAudit exercises handleConfigAudit: it generates audit
// records via config gets, then reads back the audit log with filters.
func TestHandleConfigAudit(t *testing.T) {
	gw := newGatewayWithConfig(t)

	entry := configcenter.NewConfigEntry("svc:audited", "value-123")
	entry.Rating = configcenter.RatingPublic
	if err := gw.configCenter.Set(entry); err != nil {
		t.Fatalf("Set: %v", err)
	}
	// Produce an audit record.
	if _, err := gw.configCenter.Get(newTestCtx(), configcenter.ConfigAccessRequest{
		AgentID: "agent-1", RunID: "run-1", Key: "svc:audited",
	}); err != nil {
		t.Fatalf("Get: %v", err)
	}

	token := mkToken(t, gw, "agent-1", "config.access")

	mux := http.NewServeMux()
	mux.HandleFunc("GET /v1/config/audit", gw.withAuth(gw.configHandler.handleConfigAudit))
	srv := httptest.NewServer(mux)
	t.Cleanup(srv.Close)

	// Unfiltered audit query.
	resp := doAuthedGet(t, srv.URL+"/v1/config/audit", token)
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("audit status = %d, want 200", resp.StatusCode)
	}
	var body map[string]any
	_ = json.NewDecoder(resp.Body).Decode(&body)
	resp.Body.Close()
	if _, ok := body["entries"]; !ok {
		t.Fatalf("audit response missing entries key: %v", body)
	}

	// Filtered by agent_id, key, and limit.
	filtered := doAuthedGet(t, srv.URL+"/v1/config/audit?agent_id=agent-1&key=svc:audited&limit=5", token)
	if filtered.StatusCode != http.StatusOK {
		t.Fatalf("filtered audit status = %d, want 200", filtered.StatusCode)
	}
	filtered.Body.Close()

	// Forbidden without cap.
	noCap := mkToken(t, gw, "agent-3", "config.list")
	forbidden := doAuthedGet(t, srv.URL+"/v1/config/audit", noCap)
	if forbidden.StatusCode != http.StatusForbidden {
		t.Fatalf("audit without cap status = %d, want 403", forbidden.StatusCode)
	}
	forbidden.Body.Close()
}

// TestCheckAllAndParseCapability covers CapabilityChecker.CheckAll (pass and
// fail) plus ParseCapability across valid inputs and the unknown/default case.
func TestCheckAllAndParseCapability(t *testing.T) {
	cc := NewCapabilityChecker()

	claims := &TokenClaims{
		Caps: []string{string(CapConfigAccess), string(CapConfigList)},
	}

	// CheckAll passes when all required caps are present.
	if err := cc.CheckAll(claims, CapConfigAccess, CapConfigList); err != nil {
		t.Fatalf("CheckAll(present) error = %v, want nil", err)
	}
	// CheckAll fails when one is missing.
	if err := cc.CheckAll(claims, CapConfigAccess, CapConfigWrite); err == nil {
		t.Fatalf("CheckAll(missing) error = nil, want error")
	}

	// ParseCapability: a representative set of valid strings.
	valids := []struct {
		in   string
		want AgentCapability
	}{
		{"eventbus.publish", CapEventbusPublish},
		{"channel.send", CapChannelSend},
		{"  MEMORY.READ  ", CapMemoryRead},
		{"config.access", CapConfigAccess},
		{"config.write", CapConfigWrite},
		{"db.query", CapDBQuery},
		{"agent.list", CapAgentList},
		{"log.write", CapLogWrite},
	}
	for _, tc := range valids {
		got, err := ParseCapability(tc.in)
		if err != nil {
			t.Fatalf("ParseCapability(%q) error = %v", tc.in, err)
		}
		if got != tc.want {
			t.Fatalf("ParseCapability(%q) = %q, want %q", tc.in, got, tc.want)
		}
	}

	// Unknown capability -> error (default branch).
	if _, err := ParseCapability("does.not.exist"); err == nil {
		t.Fatalf("ParseCapability(unknown) error = nil, want error")
	}
}

// TestSetAuditJournal exercises the SetAuditJournal setter on the gateway.
func TestSetAuditJournal(t *testing.T) {
	gw := NewGateway(DefaultGatewayConfig(t.TempDir()))
	// Passing nil should be a safe no-op assignment (setter just stores it).
	gw.SetAuditJournal(nil)
}
