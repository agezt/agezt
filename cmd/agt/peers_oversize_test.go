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

// TestPeers_OversizedHealthBody proves `agt peers` bounds a peer's health
// response: a peer that returns a health doc whose JSON value runs past
// maxPeerHealthBytes is treated as unreachable (the read is cut off and the decode
// fails) instead of being ingested unbounded into the CLI's memory.
//
// The body is a single, valid-prefix JSON object with a giant `version` string. A
// decoder reading the WHOLE value would succeed and report the peer reachable with
// a multi-megabyte version; with the cap, the read stops mid-string and Decode
// errors — so the observable outcome (unreachable) distinguishes capped from not.
func TestPeers_OversizedHealthBody(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/api/v1/health" {
			w.WriteHeader(http.StatusNotFound)
			return
		}
		w.Header().Set("Content-Type", "application/json")
		// {"status":"ok","version":"xxxx…(2 MiB)…","model_count":1}
		_, _ = w.Write([]byte(`{"status":"ok","version":"`))
		huge := bytes.Repeat([]byte("x"), maxPeerHealthBytes+(1<<20))
		_, _ = w.Write(huge)
		_, _ = w.Write([]byte(`","model_count":1}`))
	}))
	defer srv.Close()
	t.Setenv("AGEZT_PEERS", "big="+srv.URL)

	var out, errb bytes.Buffer
	// --json always exits 0 (the result detail is in the body); the source of truth
	// is the parsed reachability below.
	_ = cmdPeers([]string{"--json"}, &out, &errb)
	var arr []peerHealth
	if err := json.Unmarshal(out.Bytes(), &arr); err != nil {
		t.Fatalf("json: %v\n%s", err, out.String())
	}
	if len(arr) != 1 {
		t.Fatalf("want 1 result, got %d", len(arr))
	}
	if arr[0].Reachable {
		t.Errorf("oversized health body must be rejected, got reachable=%+v", arr[0])
	}
	if !strings.Contains(arr[0].Error, "bad health response") {
		t.Errorf("want a decode error, got %q", arr[0].Error)
	}
}

// TestPeers_NormalHealthBodyUnaffected confirms the cap is regression-free: an
// ordinary small health doc still parses and reports the peer reachable.
func TestPeers_NormalHealthBodyUnaffected(t *testing.T) {
	srv := healthServer(t, "", "3.2.1", 4)
	defer srv.Close()
	t.Setenv("AGEZT_PEERS", "ok="+srv.URL)

	var out, errb bytes.Buffer
	if code := cmdPeers([]string{"--json"}, &out, &errb); code != 0 {
		t.Fatalf("healthy peer should exit 0, got stderr=%s", errb.String())
	}
	var arr []peerHealth
	if err := json.Unmarshal(out.Bytes(), &arr); err != nil {
		t.Fatalf("json: %v", err)
	}
	if len(arr) != 1 || !arr[0].Reachable || arr[0].Version != "3.2.1" || arr[0].ModelCount != 4 {
		t.Errorf("normal health body result = %+v", arr)
	}
}
