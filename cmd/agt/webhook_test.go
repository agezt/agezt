// SPDX-License-Identifier: MIT

package main

import (
	"bytes"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/agezt/agezt/internal/brand"
)

func TestCmdWebhookTest_OKAndFail(t *testing.T) {
	// A 2xx sink → exit 0.
	good := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
	}))
	defer good.Close()
	var out, errOut bytes.Buffer
	if code := cmdWebhookTest([]string{good.URL}, &out, &errOut); code != 0 {
		t.Errorf("good sink: exit = %d want 0 (err=%q)", code, errOut.String())
	}
	if !strings.Contains(out.String(), "[OK]") {
		t.Errorf("expected [OK] line, got %q", out.String())
	}

	// A 5xx sink → exit 3.
	bad := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusBadGateway)
	}))
	defer bad.Close()
	out.Reset()
	errOut.Reset()
	if code := cmdWebhookTest([]string{bad.URL}, &out, &errOut); code != 3 {
		t.Errorf("bad sink: exit = %d want 3", code)
	}
	if !strings.Contains(out.String(), "[FAIL]") {
		t.Errorf("expected [FAIL] line, got %q", out.String())
	}
}

func TestCmdWebhookTest_UsageErrors(t *testing.T) {
	var out, errOut bytes.Buffer
	// Non-http URL → usage error (2).
	if code := cmdWebhookTest([]string{"ftp://x/y"}, &out, &errOut); code != 2 {
		t.Errorf("bad scheme: exit = %d want 2", code)
	}
	// No url + WEBHOOKS unset → usage error (2).
	t.Setenv(brand.EnvPrefix+"WEBHOOKS", "")
	out.Reset()
	errOut.Reset()
	if code := cmdWebhookTest(nil, &out, &errOut); code != 2 {
		t.Errorf("no url + no env: exit = %d want 2", code)
	}
}

func TestCmdWebhookTest_FromEnv(t *testing.T) {
	good := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
	}))
	defer good.Close()
	t.Setenv(brand.EnvPrefix+"WEBHOOKS", good.URL+"|agent.>")
	var out, errOut bytes.Buffer
	if code := cmdWebhookTest(nil, &out, &errOut); code != 0 {
		t.Errorf("env sink: exit = %d want 0 (err=%q)", code, errOut.String())
	}
}
