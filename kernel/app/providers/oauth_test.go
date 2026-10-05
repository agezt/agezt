// SPDX-License-Identifier: MIT

package providers

import (
	"context"
	"encoding/base64"
	"encoding/json"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"sync/atomic"
	"testing"

	"github.com/agezt/agezt/kernel/creds"
	"github.com/agezt/agezt/kernel/runtime"
	"github.com/agezt/agezt/plugins/providers/mock"
)

func TestOAuthImportStatusLogoutRemainSocketFreeAndUseLiveModels(t *testing.T) {
	t.Setenv(creds.PassphraseEnvVar, "isolated-oauth-fixture")
	dir := t.TempDir()
	var reloads, syncs atomic.Int32
	k, err := runtime.Open(runtime.Config{BaseDir: dir, Provider: mock.New(), OnReload: func() error { reloads.Add(1); return nil }})
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { k.Close() })
	auth := NewOAuth(k, dir, func() ([]string, string) {
		syncs.Add(1)
		return []string{"fixture-current-model"}, "fixture-current-model"
	})
	claims, _ := json.Marshal(map[string]any{"email": "fixture@example.invalid"})
	id := "h." + base64.RawURLEncoding.EncodeToString(claims) + ".s"
	path := filepath.Join(t.TempDir(), "auth.json")
	raw, _ := json.Marshal(map[string]any{"tokens": map[string]any{"access_token": "fixture-access", "refresh_token": "fixture-refresh", "id_token": id, "account_id": "fixture-account"}})
	if err := os.WriteFile(path, raw, 0600); err != nil {
		t.Fatal(err)
	}
	imported, err := auth.Import(context.Background(), OAuthImportInput{Path: path})
	if err != nil {
		t.Fatal(err)
	}
	if imported["connected"] != true || imported["email"] != "fixture@example.invalid" || imported["default_model"] != "fixture-current-model" || reloads.Load() != 1 {
		t.Fatalf("import=%v reloads=%d", imported, reloads.Load())
	}
	status, err := auth.Status(context.Background(), OAuthStatusInput{})
	if err != nil {
		t.Fatal(err)
	}
	if status["connected"] != true || status["default_model"] != "fixture-current-model" || syncs.Load() != 2 {
		t.Fatalf("status/sync=%v %d", status, syncs.Load())
	}
	encoded, err := json.Marshal(status)
	if err != nil {
		t.Fatal(err)
	}
	if strings.Contains(string(encoded), "fixture-access") || strings.Contains(string(encoded), "fixture-refresh") {
		t.Fatal("OAuth tokens reached response")
	}
	if _, err := auth.Logout(context.Background(), OAuthLogoutInput{}); err != nil {
		t.Fatal(err)
	}
	status, err = auth.Status(context.Background(), OAuthStatusInput{})
	if err != nil {
		t.Fatal(err)
	}
	if status["connected"] != false || status["default_model"] != "" || syncs.Load() != 2 || reloads.Load() != 2 {
		t.Fatalf("logout/status=%v sync/reload=%d/%d", status, syncs.Load(), reloads.Load())
	}
}

func TestOAuthCallbackDenialStateFilteringAndConcurrentStatus(t *testing.T) {
	t.Setenv(creds.PassphraseEnvVar, "isolated-oauth-fixture")
	auth := NewOAuth(nil, t.TempDir(), nil)
	login := &providerLogin{state: "fixture-state", status: "pending"}
	auth.provLogin = login
	response := httptest.NewRecorder()
	auth.providerCallback(response, httptest.NewRequest("GET", "http://callback.invalid/?error=denied", nil), login)
	status, err := auth.Status(context.Background(), OAuthStatusInput{State: "fixture-state"})
	if err != nil {
		t.Fatal(err)
	}
	if status["status"] != "error" || status["error"] != "authorization denied: denied" || !strings.Contains(response.Body.String(), "Sign-in failed") {
		t.Fatalf("denial=%v page=%s", status, response.Body.String())
	}
	status, err = auth.Status(context.Background(), OAuthStatusInput{State: "different"})
	if err != nil {
		t.Fatal(err)
	}
	if status["status"] != "unknown" || status["error"] != "" {
		t.Fatalf("unmatched state exposed login=%v", status)
	}
	var wg sync.WaitGroup
	for i := 0; i < 10; i++ {
		wg.Add(2)
		go func() { defer wg.Done(); auth.setProviderLoginStatus(login, "pending", "") }()
		go func() {
			defer wg.Done()
			if _, err := auth.Status(context.Background(), OAuthStatusInput{}); err != nil {
				t.Error(err)
			}
		}()
	}
	wg.Wait()
}

func TestOAuthFailurePageEscapesUntrustedText(t *testing.T) {
	response := httptest.NewRecorder()
	providerLoginPage(response, false, `<script>"fixture"</script>`)
	if strings.Contains(response.Body.String(), `<script>"fixture"</script>`) {
		t.Fatal("callback text remained HTML")
	}
	if !strings.Contains(response.Body.String(), "&lt;script&gt;&quot;fixture&quot;&lt;/script&gt;") {
		t.Fatalf("callback escape=%s", response.Body.String())
	}
}
