// SPDX-License-Identifier: MIT

package main

import (
	"bytes"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

func TestIsHTTPURL(t *testing.T) {
	for _, c := range []struct {
		in   string
		want bool
	}{
		{"http://x", true}, {"https://x/y", true},
		{"./dir", false}, {"/abs/dir", false}, {"ftp://x", false},
	} {
		if got := isHTTPURL(c.in); got != c.want {
			t.Errorf("isHTTPURL(%q) = %v, want %v", c.in, got, c.want)
		}
	}
}

// A registry index file name that escapes the base (traversal or a separator) is
// refused before any fetch — a malicious index cannot redirect the download
// (M274).
func TestFetchRegistryFile_RejectsUnsafePath(t *testing.T) {
	for _, bad := range []string{"../secret", "a/b.skill.json", "x\\y", ".."} {
		var errb bytes.Buffer
		if _, ok := fetchRegistryFile("http://example.test", bad, &errb); ok {
			t.Errorf("fetchRegistryFile accepted unsafe path %q", bad)
		}
		if !strings.Contains(errb.String(), "unsafe") {
			t.Errorf("path %q: stderr = %q, want an unsafe-path notice", bad, errb.String())
		}
	}
}

// remoteRegistry lists the bundles from a served index.json (the list path needs
// no daemon).
func TestRemoteRegistry_ListsFromIndex(t *testing.T) {
	idx := registryIndex{
		Tool: "agt", FormatVersion: 1,
		Skills: []indexSkill{
			{Name: "diagnose-ci", Version: "0.1.0", ID: "abc123", Description: "find why CI broke", File: "diagnose-ci-abc.skill.json"},
			{Name: "deploy", Version: "0.2.0", ID: "def456", Description: "ship it", File: "deploy-def.skill.json"},
		},
	}
	body, _ := json.Marshal(idx)
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/index.json" {
			_, _ = w.Write(body)
			return
		}
		http.NotFound(w, r)
	}))
	defer srv.Close()

	var out, errb bytes.Buffer
	if code := remoteRegistry(srv.URL, "", false, &out, &errb); code != 0 {
		t.Fatalf("exit = %d, err = %s", code, errb.String())
	}
	got := out.String()
	if !strings.Contains(got, "diagnose-ci") || !strings.Contains(got, "deploy") {
		t.Errorf("listing = %q, want both skills", got)
	}
	if !strings.Contains(got, "--install deploy") {
		t.Errorf("listing = %q, want per-skill install hints", got)
	}
}

// A missing index.json (404) is a clean error, not a crash.
func TestRemoteRegistry_MissingIndex(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		http.NotFound(w, r)
	}))
	defer srv.Close()

	var out, errb bytes.Buffer
	if code := remoteRegistry(srv.URL, "", false, &out, &errb); code != 1 {
		t.Errorf("exit = %d, want 1 for a missing index", code)
	}
}

// Installing a name absent from the index errors before fetching a bundle or
// dialing the daemon.
func TestRemoteInstall_NameNotFound(t *testing.T) {
	idx := registryIndex{Skills: []indexSkill{{Name: "deploy", File: "deploy.skill.json"}}}
	var out, errb bytes.Buffer
	if code := remoteInstall("http://example.test", idx, "missing", false, &out, &errb); code != 1 {
		t.Errorf("exit = %d, want 1 for an absent name", code)
	}
	if !strings.Contains(errb.String(), "no skill named") {
		t.Errorf("stderr = %q, want a not-found message", errb.String())
	}
}
