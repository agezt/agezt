// SPDX-License-Identifier: MIT

package main

import (
	"testing"

	"github.com/agezt/agezt/kernel/contract/updateapi"
	"github.com/agezt/agezt/kernel/update"
)

// TestTheNilEngineBecomesANilPort: the daemon's conversion is what keeps the
// "update is disabled" answer reachable.
//
// A nil *update.Service handed to an interface parameter is a NON-nil
// interface — the holder would hold an engine that is not there, and every
// nil check downstream would be false.
func TestTheNilEngineBecomesANilPort(t *testing.T) {
	var engine *update.Service
	port := updatePort(engine)
	if port != nil {
		t.Fatalf("EXPECTED:a nil port for a nil engine ACTUAL:%v", port)
	}
}

// TestTheEngineBecomesThePort: a wired engine is handed over as the
// contract's port.
func TestTheEngineBecomesThePort(t *testing.T) {
	engine := update.New(update.Config{Source: update.SourceGitHub, GitHubOwner: "agezt", GitHubRepo: "agezt"})
	port := updatePort(engine)
	if port == nil {
		t.Fatal("EXPECTED:the wired engine as a port")
	}
	var _ updateapi.Backend = port
}
