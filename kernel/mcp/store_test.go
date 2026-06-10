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
	}
	for _, tc := range cases {
		s := ok
		tc.mutate(&s)
		if err := Validate(s); err == nil {
			t.Errorf("%s: accepted", tc.label)
		}
	}
}
