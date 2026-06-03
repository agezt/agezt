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

// TestPeers_Route_ChoosesSortedFirstServer: with several peers serving the model,
// `route` names the sorted-first as chosen and the rest as fallback — mirroring the
// remote_run auto-router (M203).
func TestPeers_Route_ChoosesSortedFirstServer(t *testing.T) {
	srvA := modelsServer(t, "", "opus", []string{"opus", "haiku"})
	defer srvA.Close()
	srvB := modelsServer(t, "", "opus", []string{"opus"})
	defer srvB.Close()
	t.Setenv("AGEZT_PEERS", "alpha="+srvA.URL+",bravo="+srvB.URL)

	var out, errb bytes.Buffer
	if code := cmdPeers([]string{"route", "opus", "--json"}, &out, &errb); code != 0 {
		t.Fatalf("exit=%d stderr=%s", code, errb.String())
	}
	var arr []peerRoute
	if err := json.Unmarshal(out.Bytes(), &arr); err != nil {
		t.Fatalf("json: %v\n%s", err, out.String())
	}
	byName := map[string]peerRoute{}
	for _, e := range arr {
		byName[e.Name] = e
	}
	if !byName["alpha"].Chosen || !byName["alpha"].Serves {
		t.Errorf("alpha should be chosen+serves: %+v", byName["alpha"])
	}
	if byName["bravo"].Chosen || !byName["bravo"].Serves {
		t.Errorf("bravo should serve but not be chosen: %+v", byName["bravo"])
	}
}

func TestPeers_Route_Text(t *testing.T) {
	srvA := modelsServer(t, "", "haiku", []string{"haiku"})
	defer srvA.Close()
	srvB := modelsServer(t, "", "opus", []string{"opus"})
	defer srvB.Close()
	t.Setenv("AGEZT_PEERS", "alpha="+srvA.URL+",bravo="+srvB.URL)

	var out, errb bytes.Buffer
	if code := cmdPeers([]string{"route", "opus"}, &out, &errb); code != 0 {
		t.Fatalf("exit=%d", code)
	}
	s := out.String()
	if !strings.Contains(s, `would route to: bravo`) {
		t.Errorf("missing route header:\n%s", s)
	}
	if !strings.Contains(s, "chosen") || !strings.Contains(s, "does not serve") {
		t.Errorf("missing per-peer status:\n%s", s)
	}
}

// TestPeers_Route_NoServerExits1: a model no reachable peer serves exits non-zero.
func TestPeers_Route_NoServerExits1(t *testing.T) {
	srv := modelsServer(t, "", "haiku", []string{"haiku"})
	defer srv.Close()
	t.Setenv("AGEZT_PEERS", "only="+srv.URL)

	var out, errb bytes.Buffer
	if code := cmdPeers([]string{"route", "gpt-4o"}, &out, &errb); code != 1 {
		t.Fatalf("unserved model should exit 1, got %d", code)
	}
	if !strings.Contains(out.String(), "no reachable peer serves it") {
		t.Errorf("output = %s", out.String())
	}
}

// TestPeers_Route_SkipsUnreachable: an unreachable peer is shown but the first
// reachable server is chosen.
func TestPeers_Route_SkipsUnreachable(t *testing.T) {
	// alpha (sorted first) is a server that 500s; bravo serves opus.
	dead := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusInternalServerError)
	}))
	defer dead.Close()
	srvB := modelsServer(t, "", "opus", []string{"opus"})
	defer srvB.Close()
	t.Setenv("AGEZT_PEERS", "alpha="+dead.URL+",bravo="+srvB.URL)

	var out, errb bytes.Buffer
	if code := cmdPeers([]string{"route", "opus", "--json"}, &out, &errb); code != 0 {
		t.Fatalf("exit=%d stderr=%s", code, errb.String())
	}
	var arr []peerRoute
	if err := json.Unmarshal(out.Bytes(), &arr); err != nil {
		t.Fatalf("json: %v", err)
	}
	byName := map[string]peerRoute{}
	for _, e := range arr {
		byName[e.Name] = e
	}
	if byName["alpha"].Reachable {
		t.Errorf("alpha should be unreachable: %+v", byName["alpha"])
	}
	if !byName["bravo"].Chosen {
		t.Errorf("bravo should be chosen after skipping alpha: %+v", byName["bravo"])
	}
}

// TestPeers_Route_RequiresModel: `route` with no model is a usage error.
func TestPeers_Route_RequiresModel(t *testing.T) {
	var out, errb bytes.Buffer
	if code := cmdPeers([]string{"route"}, &out, &errb); code != 2 {
		t.Fatalf("route without a model should exit 2, got %d", code)
	}
	if !strings.Contains(errb.String(), "a <model> is required") {
		t.Errorf("stderr = %q", errb.String())
	}
}
