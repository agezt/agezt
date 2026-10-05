// SPDX-License-Identifier: MIT

package providers

import (
	"context"
	"testing"
	"time"

	"github.com/agezt/agezt/kernel/chatgptauth"
	"github.com/agezt/agezt/kernel/creds"
)

func TestOAuthLogoutPreventsLateCallbackPersistence(t *testing.T) {
	t.Setenv(creds.PassphraseEnvVar, "isolated-oauth-fixture")
	auth := NewOAuth(nil, t.TempDir(), nil)
	login := expiryLogin()
	login.verifier = "fixture-verifier"
	auth.provLogin = login
	started := make(chan struct{})
	release := make(chan struct{})
	auth.fetchTokens = func(context.Context, string, string) (chatgptauth.Tokens, error) {
		close(started)
		<-release
		return chatgptauth.Tokens{AccessToken: "late-fixture-access", RefreshToken: "late-fixture-refresh"}, nil
	}
	result := make(chan bool, 1)
	go func() {
		success, _, _ := auth.providerCompletion(login)(context.Background(), "fixture-code", login.state, "")
		result <- success
	}()
	select {
	case <-started:
	case <-time.After(time.Second):
		t.Fatal("controlled token fetch did not begin")
	}
	if _, err := auth.Logout(context.Background(), OAuthLogoutInput{}); err != nil {
		close(release)
		t.Fatal(err)
	}
	close(release)
	select {
	case success := <-result:
		if success || auth.chatgptMgr().HasTokens() {
			t.Fatalf("EXPECTED: logout rejects late callback persistence; ACTUAL: success=%v tokens=%v", success, auth.chatgptMgr().HasTokens())
		}
	case <-time.After(time.Second):
		t.Fatal("callback did not settle after released fetch")
	}
}

func TestOAuthReplacementPreventsLateCallbackPersistenceAndModelEffects(t *testing.T) {
	t.Setenv(creds.PassphraseEnvVar, "isolated-oauth-fixture")
	models := 0
	auth := NewOAuth(nil, t.TempDir(), func() ([]string, string) { models++; return nil, "" })
	old := expiryLogin()
	auth.provLogin = old
	started, release := make(chan struct{}), make(chan struct{})
	auth.fetchTokens = func(context.Context, string, string) (chatgptauth.Tokens, error) {
		close(started)
		<-release
		return chatgptauth.Tokens{AccessToken: "retired-fixture-access"}, nil
	}
	done := make(chan bool, 1)
	go func() {
		success, _, _ := auth.providerCompletion(old)(context.Background(), "fixture-code", old.state, "")
		done <- success
	}()
	<-started
	auth.stopProviderLogin()
	replacement := expiryLogin()
	replacement.state = "replacement-state"
	auth.provLoginMu.Lock()
	auth.provLogin = replacement
	auth.provLoginMu.Unlock()
	close(release)
	select {
	case success := <-done:
		if success || auth.chatgptMgr().HasTokens() || models != 0 || replacement.status != "pending" {
			t.Fatalf("retired callback reached new session/effects: success=%v tokens=%v models=%d new=%s", success, auth.chatgptMgr().HasTokens(), models, replacement.status)
		}
	case <-time.After(time.Second):
		t.Fatal("retired callback did not settle")
	}
}

func TestOAuthPersistenceRejectsCanceledExpiredAndTerminalOwners(t *testing.T) {
	t.Setenv(creds.PassphraseEnvVar, "isolated-oauth-fixture")
	for _, mode := range []string{"canceled", "stopped", "terminal", "retired"} {
		t.Run(mode, func(t *testing.T) {
			auth := NewOAuth(nil, t.TempDir(), nil)
			login := expiryLogin()
			auth.provLogin = login
			ctx, cancel := context.WithCancel(context.Background())
			defer cancel()
			switch mode {
			case "canceled":
				cancel()
			case "stopped":
				auth.closeProviderLogin(login)
			case "terminal":
				login.status = "error"
			case "retired":
				auth.provLogin = expiryLogin()
			}
			if err := auth.persistProviderTokens(ctx, login, chatgptauth.Tokens{AccessToken: "rejected-fixture-access"}); err == nil {
				t.Fatal("inadmissible candidate was persisted")
			}
			if auth.chatgptMgr().HasTokens() {
				t.Fatal("inadmissible candidate changed the vault")
			}
		})
	}
}

func TestOAuthCurrentSessionPersistsCandidateAndCompletes(t *testing.T) {
	t.Setenv(creds.PassphraseEnvVar, "isolated-oauth-fixture")
	models := 0
	auth := NewOAuth(nil, t.TempDir(), func() ([]string, string) { models++; return []string{}, "" })
	login := expiryLogin()
	login.verifier = "owned-verifier"
	auth.provLogin = login
	auth.fetchTokens = func(_ context.Context, code, verifier string) (chatgptauth.Tokens, error) {
		if code != "owned-code" || verifier != "owned-verifier" {
			t.Fatal("owned exchange identity changed")
		}
		return chatgptauth.Tokens{AccessToken: "owned-fixture-access", RefreshToken: "owned-fixture-refresh"}, nil
	}
	success, message, closeListener := auth.providerCompletion(login)(context.Background(), "owned-code", login.state, "")
	if !success || message != "" || !closeListener || !auth.chatgptMgr().HasTokens() || login.status != "done" || models != 1 {
		t.Fatalf("owned completion failed: %v %q %v status=%s models=%d", success, message, closeListener, login.status, models)
	}
}

func TestOAuthCallbackRetiredAfterExchangeSkipsCompletionEffects(t *testing.T) {
	models := 0
	auth := NewOAuth(nil, t.TempDir(), func() ([]string, string) { models++; return nil, "" })
	login := expiryLogin()
	auth.provLogin = login
	result := auth.completeProviderLogin(context.Background(), login, providerCallbackInput{Code: "fixture-code", State: login.state}, func(context.Context, string, string) error {
		auth.stopProviderLogin()
		return nil
	})
	if result.Success || models != 0 || login.status == "done" {
		t.Fatalf("retired exchange reached completion effects: %+v models=%d status=%s", result, models, login.status)
	}
}
