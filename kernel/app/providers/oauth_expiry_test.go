// SPDX-License-Identifier: MIT

package providers

import (
	"context"
	"strings"
	"testing"
	"time"
)

func expiryLogin() *providerLogin {
	return &providerLogin{state: "fixture", status: "pending", expiryStop: make(chan struct{}), expiryDone: make(chan struct{})}
}

func waitExpiry(t *testing.T, login *providerLogin) {
	t.Helper()
	select {
	case <-login.expiryDone:
	case <-time.After(2 * time.Second):
		t.Fatal("expiry worker retained after owned lifecycle ended")
	}
}

func TestOAuthStartStopWiresOwnedExpiryCancellation(t *testing.T) {
	auth := NewOAuth(nil, t.TempDir(), nil)
	if _, err := auth.Start(context.Background(), OAuthStartInput{}); err != nil {
		if strings.Contains(err.Error(), "cannot bind") {
			t.Skip("callback port is owned by another fixture")
		}
		t.Fatal(err)
	}
	defer auth.stopProviderLogin()
	login := auth.provLogin
	if login == nil || login.expiryStop == nil || login.expiryDone == nil {
		t.Fatal("Start omitted expiry ownership")
	}
	auth.stopProviderLogin()
	waitExpiry(t, login)
}

func TestOAuthExpiryStopReleasesWorkerWithoutTimeoutMutation(t *testing.T) {
	auth := NewOAuth(nil, t.TempDir(), nil)
	login := expiryLogin()
	auth.provLogin = login
	go auth.expireProviderLogin(login, time.Hour)
	auth.stopProviderLogin()
	waitExpiry(t, login)
	if login.status != "pending" || login.errMsg != "" || auth.provLogin != nil {
		t.Fatal("canceled expiry changed retired login state")
	}
	auth.closeProviderLogin(login)
	auth.closeProviderLogin(login)
}

func TestOAuthExpiryCallbackCloseReleasesWorker(t *testing.T) {
	auth := NewOAuth(nil, t.TempDir(), nil)
	login := expiryLogin()
	login.status = "done"
	auth.provLogin = login
	go auth.expireProviderLogin(login, time.Hour)
	go auth.deferredClose(login)
	waitExpiry(t, login)
	if login.status != "done" || login.errMsg != "" {
		t.Fatal("callback close changed completed status")
	}
}

func TestOAuthExpiryRetainsTimeoutAndIgnoresReplacedLogin(t *testing.T) {
	for _, replaced := range []bool{false, true} {
		auth := NewOAuth(nil, t.TempDir(), nil)
		login := expiryLogin()
		auth.provLogin = login
		if replaced {
			auth.provLogin = expiryLogin()
		}
		go auth.expireProviderLogin(login, time.Millisecond)
		waitExpiry(t, login)
		if replaced {
			if login.status != "pending" || auth.provLogin.status != "pending" {
				t.Fatal("retired timer changed login state")
			}
		} else if login.status != "error" || login.errMsg != "sign-in timed out" {
			t.Fatalf("expiry state = %s %q", login.status, login.errMsg)
		}
	}
}
