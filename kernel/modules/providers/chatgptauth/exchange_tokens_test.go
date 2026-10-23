// SPDX-License-Identifier: MIT

package chatgptauth

import (
	"context"
	"net/http"
	"net/http/httptest"
	"net/url"
	"reflect"
	"testing"
)

func TestExchangeTokensPreservesFormCandidateAndDefersPersistence(t *testing.T) {
	withLoopbackClient(t)
	var form url.Values
	id := makeJWT(map[string]any{"https://api.openai.com/auth": map[string]any{"chatgpt_account_id": "fixture-account"}})
	endpoint := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPost || r.Header.Get("Content-Type") != "application/x-www-form-urlencoded" {
			t.Error("exchange request shape changed")
		}
		_ = r.ParseForm()
		form = r.Form
		_, _ = w.Write([]byte(`{"access_token":"fixture-access","refresh_token":"fixture-refresh","id_token":"` + id + `"}`))
	}))
	defer endpoint.Close()
	previous := tokenEP
	tokenEP = endpoint.URL
	t.Cleanup(func() { tokenEP = previous })
	manager := newTestManager(t)
	if err := manager.StoreTokens(Tokens{AccessToken: "previous-access", RefreshToken: "previous-refresh"}); err != nil {
		t.Fatal(err)
	}
	before := manager.toks
	candidate, err := manager.ExchangeTokens(context.Background(), "fixture-code", "fixture-verifier")
	if err != nil {
		t.Fatal(err)
	}
	if candidate != (Tokens{AccessToken: "fixture-access", RefreshToken: "fixture-refresh", IDToken: id}) || !reflect.DeepEqual(manager.toks, before) {
		t.Fatalf("candidate fetch changed manager or fields: candidate=%+v manager=%+v", candidate, manager.toks)
	}
	if form.Get("code") != "fixture-code" || form.Get("code_verifier") != "fixture-verifier" || form.Get("grant_type") != "authorization_code" || form.Get("redirect_uri") != RedirectURI || form.Get("client_id") != ClientID {
		t.Fatalf("exchange form changed: %v", form)
	}
	reopened := NewManager(manager.baseDir)
	if !reopened.HasTokens() || reopened.toks.AccessToken != "previous-access" {
		t.Fatal("candidate fetch wrote the vault")
	}
	if err := manager.StoreTokens(candidate); err != nil {
		t.Fatal(err)
	}
	if _, account := manager.Account(); account != "fixture-account" {
		t.Fatalf("explicit persist lost account derivation: %q", account)
	}
}

func TestExchangeTokensFailureKeepsExistingTokens(t *testing.T) {
	withLoopbackClient(t)
	endpoint := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusUnauthorized)
		_, _ = w.Write([]byte(`{"error_description":"fixture rejected"}`))
	}))
	defer endpoint.Close()
	previous := tokenEP
	tokenEP = endpoint.URL
	t.Cleanup(func() { tokenEP = previous })
	manager := newTestManager(t)
	if err := manager.StoreTokens(Tokens{AccessToken: "previous-access"}); err != nil {
		t.Fatal(err)
	}
	before := manager.toks
	candidate, err := manager.ExchangeTokens(context.Background(), "fixture-code", "fixture-verifier")
	if err == nil || candidate != (Tokens{}) || !reflect.DeepEqual(manager.toks, before) {
		t.Fatalf("failed fetch changed tokens: %+v %v", candidate, err)
	}
	if err := manager.ExchangeCode(context.Background(), "fixture-code", "fixture-verifier"); err == nil || !reflect.DeepEqual(manager.toks, before) {
		t.Fatal("legacy failed exchange changed existing tokens")
	}
}
