// SPDX-License-Identifier: MIT

package agentgw

import (
	"bytes"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"sort"
	"strings"
	"testing"
	"time"

	"github.com/agezt/agezt/kernel/configcenter"
)

func TestConfigGatewayListAndSearchRespectAgentVisibility(t *testing.T) {
	gw := NewGateway(DefaultGatewayConfig(t.TempDir()))
	center, err := configcenter.New(configcenter.DefaultConfig(t.TempDir()))
	if err != nil {
		t.Fatalf("configcenter.New: %v", err)
	}
	t.Cleanup(func() { center.Close() })
	gw.SetConfigCenter(center)

	public := configcenter.NewConfigEntry("public:value", "public-content")
	opsOnly := configcenter.NewConfigEntry("agent/ops/runtime", "mode=careful")
	opsOnly.AllowedAgents = []string{"ops"}
	blocked := configcenter.NewConfigEntry("agent/blocked/runtime", "mode=blocked")
	blocked.ExcludedAgents = []string{"ops"}
	plannerOnly := configcenter.NewConfigEntry("agent/planner/runtime", "mode=plan")
	plannerOnly.AllowedAgents = []string{"planner"}
	for _, entry := range []*configcenter.ConfigEntry{public, opsOnly, blocked, plannerOnly} {
		if err := center.Set(entry); err != nil {
			t.Fatalf("Set(%s): %v", entry.Key, err)
		}
	}

	token, err := gw.tokenMgr.CreateToken(&TokenClaims{
		RunID:        "run-ops",
		SubprocessID: "ops",
		Caps:         []string{"config.list", "config.search"},
		ExpiresAt:    time.Now().Add(time.Hour),
	})
	if err != nil {
		t.Fatalf("CreateToken: %v", err)
	}

	mux := http.NewServeMux()
	mux.HandleFunc("GET /v1/config/search", gw.withAuth(gw.configHandler.handleConfigSearch))
	mux.HandleFunc("GET /v1/config", gw.withAuth(gw.configHandler.handleConfigList))
	srv := httptest.NewServer(mux)
	t.Cleanup(srv.Close)

	listResp := doGatewayJSON(t, srv.URL+"/v1/config", token)
	if got := sortedStrings(listResp["keys"]); strings.Join(got, ",") != "agent/ops/runtime,public:value" {
		t.Fatalf("list keys = %v", got)
	}

	searchResp := doGatewayJSON(t, srv.URL+"/v1/config/search?q=agent/", token)
	rawResults, _ := searchResp["results"].([]any)
	keys := make([]string, 0, len(rawResults))
	for _, raw := range rawResults {
		row, _ := raw.(map[string]any)
		if key, _ := row["key"].(string); key != "" {
			keys = append(keys, key)
		}
	}
	sort.Strings(keys)
	if strings.Join(keys, ",") != "agent/ops/runtime" {
		t.Fatalf("search keys = %v", keys)
	}
}

func TestConfigGatewaySetRejectsACLFieldsButPersistsRest(t *testing.T) {
	gw := NewGateway(DefaultGatewayConfig(t.TempDir()))
	center, err := configcenter.New(configcenter.DefaultConfig(t.TempDir()))
	if err != nil {
		t.Fatalf("configcenter.New: %v", err)
	}
	t.Cleanup(func() { center.Close() })
	gw.SetConfigCenter(center)

	token, err := gw.tokenMgr.CreateToken(&TokenClaims{
		RunID:        "run-ops",
		SubprocessID: "ops",
		Caps:         []string{"config.write"},
		ExpiresAt:    time.Now().Add(time.Hour),
	})
	if err != nil {
		t.Fatalf("CreateToken: %v", err)
	}

	mux := http.NewServeMux()
	mux.HandleFunc("POST /v1/config", gw.withAuth(gw.configHandler.handleConfigSet))
	srv := httptest.NewServer(mux)
	t.Cleanup(srv.Close)

	// SECURITY (CWE-862/CWE-269): a config.write token MUST NOT be able to set
	// the per-key access-control grant (allowed_agents/excluded_agents) — that
	// would let it self-escalate read access. Such writes are rejected with 403
	// and must not persist anything.
	if status := gatewayPostStatus(t, srv.URL+"/v1/config", token, map[string]any{
		"key":            "agent/ops/runtime",
		"value":          "mode=careful",
		"allowed_agents": []string{"ops", "planner"},
	}); status != http.StatusForbidden {
		t.Fatalf("write with allowed_agents: status = %d, want 403", status)
	}
	if status := gatewayPostStatus(t, srv.URL+"/v1/config", token, map[string]any{
		"key":             "agent/ops/runtime",
		"value":           "mode=careful",
		"excluded_agents": []string{"blocked"},
	}); status != http.StatusForbidden {
		t.Fatalf("write with excluded_agents: status = %d, want 403", status)
	}
	if _, err := center.GetEntry("agent/ops/runtime"); err == nil {
		t.Fatal("rejected ACL write should not have persisted the entry")
	}

	// A write WITHOUT the ACL fields still persists value/rating/description/tags.
	doGatewayPostJSON(t, srv.URL+"/v1/config", token, map[string]any{
		"key":         "agent/ops/runtime",
		"value":       "mode=careful",
		"rating":      "restricted",
		"description": "ops-local runtime config",
		"tags":        []string{"agent", "runtime"},
	})

	entry, err := center.GetEntry("agent/ops/runtime")
	if err != nil {
		t.Fatalf("GetEntry: %v", err)
	}
	if entry.Rating != configcenter.RatingRestricted {
		t.Fatalf("rating = %q", entry.Rating)
	}
	if len(entry.AllowedAgents) != 0 || len(entry.ExcludedAgents) != 0 {
		t.Fatalf("ACL fields must stay empty via the gateway: allowed=%v excluded=%v",
			entry.AllowedAgents, entry.ExcludedAgents)
	}
}

func gatewayPostStatus(t *testing.T, url, token string, payload any) int {
	t.Helper()
	body, err := json.Marshal(payload)
	if err != nil {
		t.Fatalf("Marshal: %v", err)
	}
	req, err := http.NewRequest(http.MethodPost, url, bytes.NewReader(body))
	if err != nil {
		t.Fatalf("NewRequest: %v", err)
	}
	req.Header.Set("Authorization", "Bearer "+token)
	req.Header.Set("Content-Type", "application/json")
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatalf("Do: %v", err)
	}
	defer resp.Body.Close()
	return resp.StatusCode
}

func doGatewayJSON(t *testing.T, url, token string) map[string]any {
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
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("GET %s status = %d", url, resp.StatusCode)
	}
	var out map[string]any
	if err := json.NewDecoder(resp.Body).Decode(&out); err != nil {
		t.Fatalf("Decode: %v", err)
	}
	return out
}

func doGatewayPostJSON(t *testing.T, url, token string, payload any) map[string]any {
	t.Helper()
	body, err := json.Marshal(payload)
	if err != nil {
		t.Fatalf("Marshal: %v", err)
	}
	req, err := http.NewRequest(http.MethodPost, url, bytes.NewReader(body))
	if err != nil {
		t.Fatalf("NewRequest: %v", err)
	}
	req.Header.Set("Authorization", "Bearer "+token)
	req.Header.Set("Content-Type", "application/json")
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatalf("Do: %v", err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusCreated {
		t.Fatalf("POST %s status = %d", url, resp.StatusCode)
	}
	var out map[string]any
	if err := json.NewDecoder(resp.Body).Decode(&out); err != nil {
		t.Fatalf("Decode: %v", err)
	}
	return out
}

func sortedStrings(raw any) []string {
	xs, _ := raw.([]any)
	out := make([]string, 0, len(xs))
	for _, x := range xs {
		if s, ok := x.(string); ok {
			out = append(out, s)
		}
	}
	sort.Strings(out)
	return out
}
