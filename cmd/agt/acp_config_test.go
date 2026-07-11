// SPDX-License-Identifier: MIT

package main

import (
	"bytes"
	"encoding/json"
	"strings"
	"testing"
)

func TestACPConfigEmitsClientReadyJSONWithoutDaemon(t *testing.T) {
	var stdout, stderr bytes.Buffer
	if code := cmdACP([]string{"config", "--tenant", "team-a", "--json"}, &stdout, &stderr); code != 0 {
		t.Fatalf("code=%d stderr=%s", code, stderr.String())
	}
	var got struct {
		Name    string   `json:"name"`
		Command string   `json:"command"`
		Args    []string `json:"args"`
	}
	if err := json.Unmarshal(stdout.Bytes(), &got); err != nil {
		t.Fatalf("decode config: %v\n%s", err, stdout.String())
	}
	if got.Name != "Agezt" || got.Command == "" || strings.Join(got.Args, " ") != "acp --tenant team-a" {
		t.Fatalf("config = %+v", got)
	}
}

func TestACPConfigRejectsMissingTenant(t *testing.T) {
	var stdout, stderr bytes.Buffer
	if code := cmdACP([]string{"config", "--tenant"}, &stdout, &stderr); code != 2 {
		t.Fatalf("code=%d stdout=%s stderr=%s", code, stdout.String(), stderr.String())
	}
	if !strings.Contains(stderr.String(), "--tenant requires an id") {
		t.Fatalf("stderr=%q", stderr.String())
	}
}
