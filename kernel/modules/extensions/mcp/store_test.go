// SPDX-License-Identifier: MIT

package mcp

import (
	"testing"
	"time"
)

func openTestStore(t *testing.T) *Store {
	t.Helper()
	s, err := OpenStore(t.TempDir())
	if err != nil {
		t.Fatalf("OpenStore: %v", err)
	}
	tick := int64(0)
	s.now = func() time.Time {
		tick++
		return time.UnixMilli(1_700_000_000_000 + tick)
	}
	return s
}

func TestStore_AddGetRemove(t *testing.T) {
	s := openTestStore(t)
	srv, err := s.Add(Server{Name: "everything", Command: "npx", Args: []string{"-y", "@modelcontextprotocol/server-everything"}})
	if err != nil {
		t.Fatalf("Add: %v", err)
	}
	if !srv.Enabled || srv.ID == "" {
		t.Fatalf("new server = %+v, want enabled with id", srv)
	}
	if _, err := s.Add(Server{Name: "everything", Command: "x"}); err == nil {
		t.Fatal("duplicate name accepted")
	}
	if got, found := s.Get("everything"); !found || got.ID != srv.ID {
		t.Fatalf("Get by name = %+v/%v", got, found)
	}
	if _, err := s.SetEnabled(srv.ID, false); err != nil {
		t.Fatalf("SetEnabled: %v", err)
	}
	if got, _ := s.Get(srv.ID); got.Enabled {
		t.Fatal("disable did not stick")
	}
	gone, ok, err := s.Remove("everything")
	if err != nil || !ok || gone.ID != srv.ID {
		t.Fatalf("Remove = %+v/%v/%v", gone, ok, err)
	}
	if s.Count() != 0 {
		t.Fatalf("Count = %d", s.Count())
	}
}

func TestStore_Persistence(t *testing.T) {
	dir := t.TempDir()
	s, err := OpenStore(dir)
	if err != nil {
		t.Fatalf("OpenStore: %v", err)
	}
	if _, err := s.Add(Server{Name: "fake", Command: "python", Args: []string{"server.py"}}); err != nil {
		t.Fatalf("Add: %v", err)
	}
	re, err := OpenStore(dir)
	if err != nil {
		t.Fatalf("reopen: %v", err)
	}
	got, found := re.Get("fake")
	if !found || got.Command != "python" || len(got.Args) != 1 {
		t.Fatalf("reloaded = %+v/%v", got, found)
	}
}

func TestValidateServer(t *testing.T) {
	ok := Server{Name: "fake9", Command: "python"}
	if err := Validate(ok); err != nil {
		t.Fatalf("valid server rejected: %v", err)
	}
	cases := []struct {
		label  string
		mutate func(*Server)
	}{
		// The name becomes the mcp_<name>_* prefix segment — underscores and
		// dashes would make the Edict toolmap's parse ambiguous.
		{"underscore in name", func(s *Server) { s.Name = "my_server" }},
		{"dash in name", func(s *Server) { s.Name = "my-server" }},
		{"uppercase name", func(s *Server) { s.Name = "Fake" }},
		{"leading digit", func(s *Server) { s.Name = "9fake" }},
		{"empty command", func(s *Server) { s.Command = "  " }},
		{"empty arg", func(s *Server) { s.Args = []string{"x", " "} }},
		{"bad env key", func(s *Server) { s.Env = map[string]string{"BAD-KEY": "v"} }},
		{"env key leading digit", func(s *Server) { s.Env = map[string]string{"1KEY": "v"} }},
		{"empty tool in allowlist", func(s *Server) { s.ToolAllow = []string{"greet", "  "} }},
	}
	for _, tc := range cases {
		s := ok
		tc.mutate(&s)
		if err := Validate(s); err == nil {
			t.Errorf("%s: accepted", tc.label)
		}
	}

	// A well-formed env (e.g. an API token) + tool allowlist are accepted.
	good := ok
	good.Env = map[string]string{"GITHUB_PERSONAL_ACCESS_TOKEN": "ghp_x", "FOO_BAR": "1"}
	good.ToolAllow = []string{"create_issue", "search_code"}
	if err := Validate(good); err != nil {
		t.Errorf("valid env/tool-allow rejected: %v", err)
	}
}

func TestValidateServer_Transport(t *testing.T) {
	// A remote (URL) server with no command is valid.
	remote := Server{Name: "remote9", URL: "https://mcp.example.com/v1"}
	if err := Validate(remote); err != nil {
		t.Fatalf("valid remote server rejected: %v", err)
	}
	// With opt-in auth headers, still valid.
	remote.Headers = map[string]string{"Authorization": "Bearer x", "X-Trace-Id": "abc"}
	if err := Validate(remote); err != nil {
		t.Fatalf("remote + headers rejected: %v", err)
	}

	bad := []struct {
		label string
		srv   Server
	}{
		{"both command and url", Server{Name: "both9", Command: "python", URL: "https://x.example"}},
		{"neither command nor url", Server{Name: "none9"}},
		{"non-http scheme", Server{Name: "ftp9", URL: "ftp://x.example/v1"}},
		{"url without host", Server{Name: "nohost9", URL: "http:///v1"}},
		{"bad header name", Server{Name: "badhdr9", URL: "https://x.example", Headers: map[string]string{"Bad Header": "v"}}},
	}
	for _, tc := range bad {
		if err := Validate(tc.srv); err == nil {
			t.Errorf("%s: accepted, want rejected", tc.label)
		}
	}
}

func TestAppendEnv(t *testing.T) {
	base := []string{"PATH=/bin", "HOME=/home/me"}
	out := appendEnv(base, map[string]string{"TOKEN": "secret"})
	var got string
	for _, kv := range out {
		if kv == "TOKEN=secret" {
			got = kv
		}
	}
	if got == "" {
		t.Errorf("appendEnv did not inject TOKEN=secret: %v", out)
	}
	if len(out) != len(base)+1 {
		t.Errorf("appendEnv len = %d, want %d", len(out), len(base)+1)
	}
	// Nil/empty extra returns the base unchanged.
	if same := appendEnv(base, nil); len(same) != len(base) {
		t.Errorf("appendEnv(nil) changed the base: %v", same)
	}
}
