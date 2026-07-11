// SPDX-License-Identifier: MIT

package acpcatalog

import (
	"context"
	"net/http"
	"net/http/httptest"
	"os"
	"strings"
	"testing"
	"time"
)

func TestLiveOfficialRegistry(t *testing.T) {
	if os.Getenv("AGEZT_ACP_LIVE_SOURCES") != "1" {
		t.Skip("set AGEZT_ACP_LIVE_SOURCES=1 to fetch the official ACP agent registry")
	}
	ctx, cancel := context.WithTimeout(context.Background(), 15*time.Second)
	defer cancel()
	reg, _, cached, err := NewRegistryClient(OfficialRegistryURL).Fetch(ctx, true)
	if err != nil {
		t.Fatalf("fetch official ACP registry: %v", err)
	}
	if cached || len(reg.Agents) == 0 {
		t.Fatalf("unexpected official registry result: agents=%d cached=%v", len(reg.Agents), cached)
	}
	t.Logf("official ACP registry: schema %s, %d agents", reg.Version, len(reg.Agents))
}

const registryFixture = `{
  "version":"1.0.0",
  "agents":[
    {
      "id":"gemini","name":"Gemini CLI","version":"9.8.7",
      "description":"Gemini over ACP","repository":"https://example.test/gemini",
      "authors":["Google"],"license":"Apache-2.0",
      "distribution":{"npx":{"package":"@google/gemini-cli@9.8.7","args":["--acp"]}}
    },
    {
      "id":"binary-only","name":"Binary Only","version":"1.2.3",
      "description":"Binary fixture",
      "distribution":{"binary":{"linux-x86_64":{"archive":"https://example.test/a.tgz","cmd":"./binary-only"}}}
    }
  ]
}`

func fixtureRegistryClient(t *testing.T, status int, body string, calls *int) (*RegistryClient, *httptest.Server) {
	t.Helper()
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		*calls++
		w.WriteHeader(status)
		_, _ = w.Write([]byte(body))
	}))
	c := NewRegistryClient(srv.URL)
	c.HTTP = srv.Client()
	c.TTL = time.Hour
	return c, srv
}

func TestRegistryFetchCachesValidatedIndex(t *testing.T) {
	calls := 0
	c, srv := fixtureRegistryClient(t, http.StatusOK, registryFixture, &calls)
	defer srv.Close()

	reg, fetched, cached, err := c.Fetch(context.Background(), false)
	if err != nil {
		t.Fatalf("first fetch: %v", err)
	}
	if cached || fetched.IsZero() || len(reg.Agents) != 2 || reg.Version != "1.0.0" {
		t.Fatalf("unexpected first fetch: cached=%v fetched=%v registry=%+v", cached, fetched, reg)
	}
	_, fetched2, cached, err := c.Fetch(context.Background(), false)
	if err != nil || !cached || !fetched2.Equal(fetched) || calls != 1 {
		t.Fatalf("cached fetch = cached:%v calls:%d fetched:%v err:%v", cached, calls, fetched2, err)
	}
}

func TestRegistryFetchForcedFailureKeepsStaleSnapshot(t *testing.T) {
	calls := 0
	fail := false
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		calls++
		if fail {
			http.Error(w, "down", http.StatusBadGateway)
			return
		}
		_, _ = w.Write([]byte(registryFixture))
	}))
	defer srv.Close()
	c := NewRegistryClient(srv.URL)
	c.HTTP = srv.Client()
	if _, _, _, err := c.Fetch(context.Background(), false); err != nil {
		t.Fatalf("warm cache: %v", err)
	}
	fail = true
	reg, _, cached, err := c.Fetch(context.Background(), true)
	if err == nil || !cached || len(reg.Agents) != 2 || calls != 2 {
		t.Fatalf("stale fallback = agents:%d cached:%v calls:%d err:%v", len(reg.Agents), cached, calls, err)
	}
}

func TestDiscoverWithMergesRegistryAndLocalDetection(t *testing.T) {
	calls := 0
	c, srv := fixtureRegistryClient(t, http.StatusOK, registryFixture, &calls)
	defer srv.Close()

	inv := DiscoverWith(context.Background(), "gemini --acp", false, c)
	if inv.RegistryVersion != "1.0.0" || inv.RegisteredCount != 2 || inv.RegistryURL != srv.URL {
		t.Fatalf("inventory metadata = %+v", inv)
	}
	var gemini, binary *AgentStatus
	for i := range inv.Agents {
		switch inv.Agents[i].Slug {
		case "gemini":
			gemini = &inv.Agents[i]
		case "binary-only":
			binary = &inv.Agents[i]
		}
	}
	if gemini == nil || !gemini.Registered || !gemini.Compatible || gemini.Version != "9.8.7" || !gemini.Active {
		t.Fatalf("merged gemini = %+v", gemini)
	}
	if binary == nil {
		t.Fatal("binary-only registry entry missing")
	}
	if runtimePlatform := platformID(); runtimePlatform != "linux-x86_64" && binary.Compatible {
		t.Fatalf("binary-only should be incompatible on %s: %+v", runtimePlatform, binary)
	}
}

func TestRegistryRejectsInvalidAndOversizedDocuments(t *testing.T) {
	for name, body := range map[string]string{
		"bad schema": `{"version":"2.0.0","agents":[{"id":"x","name":"x","version":"1.0.0","description":"x","distribution":{"npx":{"package":"x"}}}]}`,
		"duplicate":  `{"version":"1.0.0","agents":[{"id":"x","name":"x","version":"1.0.0","description":"x","distribution":{"npx":{"package":"x"}}},{"id":"x","name":"y","version":"1.0.0","description":"y","distribution":{"npx":{"package":"y"}}}]}`,
		"oversized":  strings.Repeat("x", registryMaxBytes+1),
	} {
		t.Run(name, func(t *testing.T) {
			calls := 0
			c, srv := fixtureRegistryClient(t, http.StatusOK, body, &calls)
			defer srv.Close()
			if _, _, _, err := c.Fetch(context.Background(), false); err == nil {
				t.Fatal("expected registry validation/fetch error")
			}
		})
	}
}

func TestCommandMatchesLaunchDistinguishesNPXPackages(t *testing.T) {
	launch := registryLaunch{
		Program: "npx", Runner: "npx",
		Args: []string{"--yes", "@agentclientprotocol/codex-acp@1.1.2"},
	}
	if !commandMatchesLaunch("npx --yes @agentclientprotocol/codex-acp@1.1.2", launch) {
		t.Fatal("matching pinned npx package should be active")
	}
	if commandMatchesLaunch("npx --yes @agentclientprotocol/claude-agent-acp@0.58.1", launch) {
		t.Fatal("a different npx package must not mark this agent active")
	}
}

func TestResolveLaunchRejectsSelectorCommandsAndKeepsFallbackOperatorOnly(t *testing.T) {
	if _, err := ResolveLaunch(context.Background(), `codex-acp && whoami`, ""); err == nil {
		t.Fatal("selector-shaped command injection must be rejected")
	}
	launch, err := ResolveLaunch(context.Background(), "", "custom-acp --stdio")
	if err != nil {
		t.Fatalf("operator fallback: %v", err)
	}
	if !launch.Shell || launch.Program != "custom-acp --stdio" {
		t.Fatalf("operator fallback launch = %+v", launch)
	}
}
