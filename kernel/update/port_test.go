// SPDX-License-Identifier: MIT

// Package update_test is the update module's own test for the CONTRACT's port
// (W5.4c).
//
// The module keeps the work (the check, the verification, the download, the
// atomic swap); the contract keeps the rows and the port. Two things must
// hold:
//
//   - the module's rows ARE the contract's rows (aliases, not copies) — a
//     copy would fork the wire schema and the operation's schema would stop
//     matching the row the daemon answers with;
//   - the module's service IS the contract's port — the daemon hands it out
//     and the adapters never learn which package implemented it.
package update_test

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"reflect"
	"testing"

	"github.com/agezt/agezt/internal/brand"
	"github.com/agezt/agezt/kernel/contract/updateapi"
	"github.com/agezt/agezt/kernel/update"
)

// The module's service satisfies the contract's port.
var _ updateapi.Backend = (*update.Service)(nil)

// TestTheModuleRowsAreTheContractRows: the rows are aliases, not copies.
func TestTheModuleRowsAreTheContractRows(t *testing.T) {
	for _, row := range []struct {
		name       string
		moduleType reflect.Type
		rowType    reflect.Type
	}{
		{"update info", reflect.TypeOf(update.UpdateInfo{}), reflect.TypeOf(updateapi.UpdateInfo{})},
		{"check result", reflect.TypeOf(update.CheckResult{}), reflect.TypeOf(updateapi.CheckResult{})},
		{"drain result", reflect.TypeOf(update.DrainResult{}), reflect.TypeOf(updateapi.DrainResult{})},
		{"manifest", reflect.TypeOf(update.Manifest{}), reflect.TypeOf(updateapi.Manifest{})},
		{"source", reflect.TypeOf(update.SourceGitHub), reflect.TypeOf(updateapi.SourceGitHub)},
		{"provenance", reflect.TypeOf(update.ProvenanceUnverified), reflect.TypeOf(updateapi.ProvenanceUnverified)},
		{"checksum mismatch", reflect.TypeOf(update.ErrChecksumMismatch{}), reflect.TypeOf(updateapi.ErrChecksumMismatch{})},
		{"signature invalid", reflect.TypeOf(update.ErrSignatureInvalid{}), reflect.TypeOf(updateapi.ErrSignatureInvalid{})},
	} {
		if row.moduleType != row.rowType {
			t.Fatalf("the module's %s is %v, want the contract's row %v", row.name, row.moduleType, row.rowType)
		}
	}
}

// TestTheModuleSentinelsAreTheContractSentinels: the sentinels that cross
// the module's boundary are ONE sentinel, not two.
func TestTheModuleSentinelsAreTheContractSentinels(t *testing.T) {
	if update.ErrDrainTimeout != updateapi.ErrDrainTimeout {
		t.Fatal("the module's drain timeout is not the contract's sentinel")
	}
}

// TestTheModuleServiceAnswersThroughTheContractPort: the service answers
// what the contract's port promises, and it reports the running version.
func TestTheModuleServiceAnswersThroughTheContractPort(t *testing.T) {
	orig := brand.Version
	brand.Version = "1.0.0"
	defer func() { brand.Version = orig }()

	svc := update.New(update.Config{Source: update.SourceGitHub, GitHubOwner: "agezt", GitHubRepo: "agezt"})
	var port updateapi.Backend = svc

	// A cancelled context stops the check before any network call.
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	result, err := port.Check(ctx)
	if err == nil {
		t.Fatalf("EXPECTED:the cancelled check fails ACTUAL:%+v", result)
	}
	if result != nil {
		t.Fatalf("EXPECTED:no result on a failed check ACTUAL:%+v", result)
	}
}

// TestTheModuleCarriesTheRunningVersion: the check's row carries the
// BRAND's version — there is one current version in the tree, and the module
// compares the manifest against it.
func TestTheModuleCarriesTheRunningVersion(t *testing.T) {
	orig := brand.Version
	brand.Version = "9.9.9"
	defer func() { brand.Version = orig }()

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		json.NewEncoder(w).Encode(update.Manifest{Version: "9.9.9", SHA256: "abc123", URL: "https://example.com/agezt"})
	}))
	defer srv.Close()

	svc := update.New(update.Config{Source: update.SourceEndpoint, Endpoint: srv.URL})
	result, err := svc.Check(context.Background())
	if err != nil {
		t.Fatalf("the check failed: %v", err)
	}
	if result.Current != "9.9.9" {
		t.Fatalf("EXPECTED:the running version in the row ACTUAL:%q", result.Current)
	}
	if result.Update != nil {
		t.Fatalf("EXPECTED:no update for the running version ACTUAL:%+v", result.Update)
	}
}
