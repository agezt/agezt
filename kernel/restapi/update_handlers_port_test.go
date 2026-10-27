// SPDX-License-Identifier: MIT
package restapi

import (
	"context"
	"net/http"
	"strings"
	"testing"
	"time"

	"github.com/agezt/agezt/internal/brand"
	"github.com/agezt/agezt/kernel/contract/updateapi"
)

// shortHashBackend is the contract's port with a fixed answer: a checksum
// mismatch whose WANTED hash is shorter than eight characters — a manifest may
// carry such a hash.
type shortHashBackend struct {
	applied *updateapi.UpdateInfo
}

func (b *shortHashBackend) Check(context.Context) (*updateapi.CheckResult, error) {
	return &updateapi.CheckResult{Current: brand.Version}, nil
}

func (b *shortHashBackend) Apply(_ context.Context, info *updateapi.UpdateInfo, _ func(context.Context, time.Duration) updateapi.DrainResult) error {
	b.applied = info
	return &updateapi.ErrChecksumMismatch{Have: "deadbeefcafef00d", Want: "abc"}
}

// TestTheApplyAnswersAChecksumMismatchWithAShortHash: the mismatch message
// TRUNCATES the wanted hash instead of slicing it.
//
// The old message did `e.Want[:8]` — and a manifest may carry a shorter hash,
// so the handler PANICKED while reporting a failed checksum. A crash in the
// error path is worse than the error.
func TestTheApplyAnswersAChecksumMismatchWithAShortHash(t *testing.T) {
	s := newServer(t, &fakeEngine{model: "m"}, "secret")
	backend := &shortHashBackend{}
	s.SetUpdateService(backend)

	rec := do(t, s, http.MethodPost, "/api/v1/update/apply", `{"version":"1.0.0","sha256":"abc","url":"https://example.com/agezt"}`, "secret")
	if rec.Code != http.StatusBadRequest {
		t.Fatalf("EXPECTED:400 for a checksum mismatch ACTUAL:%d body=%s", rec.Code, rec.Body.String())
	}
	body := rec.Body.String()
	if !strings.Contains(body, "checksum_mismatch") {
		t.Fatalf("EXPECTED:the mismatch named ACTUAL:%s", body)
	}
	if !strings.Contains(body, "want=abc") {
		t.Fatalf("EXPECTED:the wanted hash in the message ACTUAL:%s", body)
	}
	if backend.applied == nil || backend.applied.Version != "1.0.0" {
		t.Fatalf("EXPECTED:the request reached the port ACTUAL:%+v", backend.applied)
	}
}

// TestTheUpdateCheckWithoutAnEngineReportsTheRunningVersion: a daemon with
// no update engine answers "update is disabled" and still names the version
// it is running.
func TestTheUpdateCheckWithoutAnEngineReportsTheRunningVersion(t *testing.T) {
	s := newServer(t, &fakeEngine{model: "m"}, "secret")
	rec := do(t, s, http.MethodGet, "/api/v1/update", "", "secret")
	if rec.Code != http.StatusOK {
		t.Fatalf("EXPECTED:200 ACTUAL:%d body=%s", rec.Code, rec.Body.String())
	}
	body := rec.Body.String()
	if !strings.Contains(body, `"status":"update is disabled"`) {
		t.Fatalf("EXPECTED:update is disabled ACTUAL:%s", body)
	}
	if !strings.Contains(body, brand.Version) {
		t.Fatalf("EXPECTED:the running version ACTUAL:%s", body)
	}
}
