// SPDX-License-Identifier: MIT

package providers

import (
	"context"
	"net"
	"os"
	"path/filepath"
	"testing"

	"github.com/agezt/agezt/kernel/creds"
	"github.com/agezt/agezt/kernel/platform/browsercallback"
)

func TestOAuthLogoutClosesPendingLoginAndReleasesCallbackAddress(t *testing.T) {
	t.Setenv(creds.PassphraseEnvVar, "isolated-oauth-fixture")
	reservation, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	addr := reservation.Addr().String()
	_ = reservation.Close()
	listener, err := browsercallback.Prepare(addr, func(context.Context, string, string, string) (bool, string, bool) { return false, "", false }, nil)
	if err != nil {
		t.Fatal(err)
	}
	defer listener.Close()
	auth := NewOAuth(nil, t.TempDir(), nil)
	auth.provLogin = &providerLogin{state: "fixture-state", status: "pending", srv: listener}
	out, err := auth.Logout(context.Background(), OAuthLogoutInput{})
	if err != nil || !out.OK || out.Connected {
		t.Fatalf("logout = %+v, %v", out, err)
	}
	if auth.provLogin != nil {
		t.Fatalf("EXPECTED: successful logout clears pending callback ownership; ACTUAL: login remains %s", auth.provLogin.status)
	}
	rebound, err := net.Listen("tcp", addr)
	if err != nil {
		t.Fatalf("successful logout retained callback port %s: %v", addr, err)
	}
	_ = rebound.Close()
}

func TestOAuthLogoutVaultFailureKeepsPendingLogin(t *testing.T) {
	t.Setenv(creds.PassphraseEnvVar, "isolated-oauth-fixture")
	blocked := filepath.Join(t.TempDir(), "not-a-directory")
	if err := os.WriteFile(blocked, []byte("fixture"), 0600); err != nil {
		t.Fatal(err)
	}
	auth := NewOAuth(nil, blocked, nil)
	login := &providerLogin{state: "fixture-state", status: "pending"}
	auth.provLogin = login
	if _, err := auth.Logout(context.Background(), OAuthLogoutInput{}); err == nil {
		t.Fatal("fixture vault failure did not propagate")
	}
	if auth.provLogin != login || login.status != "pending" {
		t.Fatal("failed token cleanup discarded the pending login")
	}
}
