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

const clientsFixture = `---
title: "Clients"
---

## Editors and IDEs

- [Zed](https://zed.dev/docs/ai/external-agents)
- Emacs via [agent-shell.el](https://github.com/xenodium/agent-shell)
- Visual Studio Code
  - [ACP Client](https://github.com/formulahendry/vscode-acp) extension

## CLI and TUI

- [acpx (CLI)](https://github.com/openclaw/acpx)

## Connectors

- DuckDB — through the [sidequery/duckdb-acp](https://github.com/sidequery/duckdb-acp) extension
`

func TestParseClientsMDXPreservesOfficialCategoriesAndNestedClients(t *testing.T) {
	entries, err := ParseClientsMDX(clientsFixture)
	if err != nil {
		t.Fatalf("parse: %v", err)
	}
	if len(entries) != 6 {
		t.Fatalf("entries=%d want 6: %+v", len(entries), entries)
	}
	want := map[string]string{
		"Zed":                "Editors and IDEs",
		"Emacs":              "Editors and IDEs",
		"Visual Studio Code": "Editors and IDEs",
		"ACP Client":         "Editors and IDEs",
		"acpx (CLI)":         "CLI and TUI",
		"DuckDB":             "Connectors",
	}
	for _, entry := range entries {
		if want[entry.Name] != entry.Category {
			t.Errorf("entry=%+v want category %q", entry, want[entry.Name])
		}
		delete(want, entry.Name)
	}
	if len(want) != 0 {
		t.Fatalf("missing entries: %v", want)
	}
}

func TestClientsClientCachesAndKeepsStaleSnapshot(t *testing.T) {
	calls := 0
	fail := false
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		calls++
		if fail {
			http.Error(w, "down", http.StatusBadGateway)
			return
		}
		_, _ = w.Write([]byte(clientsFixture))
	}))
	defer srv.Close()
	c := NewClientsClient(srv.URL)
	c.HTTP = srv.Client()
	c.TTL = time.Hour

	entries, revision, fetched, cached, err := c.Fetch(context.Background(), false)
	if err != nil || cached || len(entries) != 6 || revision == "" || fetched.IsZero() {
		t.Fatalf("first fetch entries=%d revision=%q fetched=%v cached=%v err=%v", len(entries), revision, fetched, cached, err)
	}
	entries[0].Name = "mutated by caller"
	cachedEntries, revision2, _, cached, err := c.Fetch(context.Background(), false)
	if err != nil || !cached || calls != 1 || revision2 != revision || cachedEntries[0].Name == "mutated by caller" {
		t.Fatalf("cache isolation entries=%+v revision=%q cached=%v calls=%d err=%v", cachedEntries, revision2, cached, calls, err)
	}

	fail = true
	stale, _, _, cached, err := c.Fetch(context.Background(), true)
	if err == nil || !cached || len(stale) != 6 || calls != 2 {
		t.Fatalf("stale fallback entries=%d cached=%v calls=%d err=%v", len(stale), cached, calls, err)
	}
}

func TestClientsClientRejectsEmptyAndOversizedSources(t *testing.T) {
	for name, body := range map[string]string{
		"empty":     "# no clients here",
		"oversized": strings.Repeat("x", clientsMaxBytes+1),
	} {
		t.Run(name, func(t *testing.T) {
			srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) { _, _ = w.Write([]byte(body)) }))
			defer srv.Close()
			c := NewClientsClient(srv.URL)
			c.HTTP = srv.Client()
			if _, _, _, _, err := c.Fetch(context.Background(), false); err == nil {
				t.Fatal("expected source rejection")
			}
		})
	}
}

func TestLiveOfficialClientsSource(t *testing.T) {
	if os.Getenv("AGEZT_ACP_LIVE_SOURCES") != "1" {
		t.Skip("set AGEZT_ACP_LIVE_SOURCES=1 to fetch the official ACP clients source")
	}
	ctx, cancel := context.WithTimeout(context.Background(), 15*time.Second)
	defer cancel()
	entries, revision, _, cached, err := NewClientsClient(OfficialClientsURL).Fetch(ctx, true)
	if err != nil {
		t.Fatalf("fetch official ACP clients source: %v", err)
	}
	if cached || revision == "" || len(entries) < 10 {
		t.Fatalf("unexpected official source result: entries=%d revision=%q cached=%v", len(entries), revision, cached)
	}
	categories := map[string]bool{}
	for _, entry := range entries {
		categories[entry.Category] = true
	}
	if len(categories) < 3 {
		t.Fatalf("official source yielded only %d categories: %v", len(categories), categories)
	}
	t.Logf("official ACP clients source: %d entries, %d categories, revision %s", len(entries), len(categories), revision)
}
