// SPDX-License-Identifier: MIT

package controlplane_test

import (
	"context"
	"encoding/base64"
	"encoding/json"
	"net/url"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/agezt/agezt/kernel/chatgptauth"
	"github.com/agezt/agezt/kernel/controlplane"
	"github.com/agezt/agezt/plugins/providers/mock"
)

func makeJWT(claims map[string]any) string {
	p, _ := json.Marshal(claims)
	return "h." + base64.RawURLEncoding.EncodeToString(p) + ".s"
}

// TestProviderOAuthStartStatus checks the ChatGPT sign-in start handler returns a
// well-formed authorize URL + state, status reports pending, and an unsupported
// provider is refused.
func TestProviderOAuthStartStatus(t *testing.T) {
	t.Setenv("AGEZT_VAULT_PASSPHRASE", "test-pass")
	_, _, c, _ := startPair(t, mock.New(mock.FinalText("ok")))
	ctx := context.Background()

	res, err := c.Call(ctx, controlplane.CmdProviderOAuthStart, map[string]any{"provider": "chatgpt"})
	if err != nil {
		t.Fatalf("start: %v", err)
	}
	authorize, _ := res["authorize_url"].(string)
	state, _ := res["state"].(string)
	if state == "" || !strings.HasPrefix(authorize, chatgptauth.AuthorizeEP+"?") {
		t.Fatalf("authorize=%q state=%q", authorize, state)
	}
	u, _ := url.Parse(authorize)
	if u.Query().Get("client_id") != chatgptauth.ClientID || u.Query().Get("state") != state {
		t.Fatalf("authorize query = %v", u.Query())
	}

	st, err := c.Call(ctx, controlplane.CmdProviderOAuthStatus, map[string]any{"state": state})
	if err != nil || st["status"] != "pending" {
		t.Fatalf("status = %v err=%v", st["status"], err)
	}

	// Clean up the 1455 listener the start opened.
	_, _ = c.Call(ctx, controlplane.CmdProviderOAuthLogout, nil)

	if _, err := c.Call(ctx, controlplane.CmdProviderOAuthStart, map[string]any{"provider": "telegram"}); err == nil {
		t.Fatal("telegram should not support provider oauth")
	}
}

// TestProviderOAuthImport drives the Codex-CLI import path end-to-end.
func TestProviderOAuthImport(t *testing.T) {
	t.Setenv("AGEZT_VAULT_PASSPHRASE", "test-pass")
	_, _, c, _ := startPair(t, mock.New(mock.FinalText("ok")))
	ctx := context.Background()

	dir := t.TempDir()
	authPath := filepath.Join(dir, "auth.json")
	id := makeJWT(map[string]any{
		"email":                       "owner@example.com",
		"https://api.openai.com/auth": map[string]any{"chatgpt_account_id": "acc-x"},
	})
	blob := `{"tokens":{"access_token":"at","refresh_token":"rt","id_token":"` + id + `","account_id":"acc-x"}}`
	if err := os.WriteFile(authPath, []byte(blob), 0o600); err != nil {
		t.Fatal(err)
	}

	res, err := c.Call(ctx, controlplane.CmdProviderOAuthImport, map[string]any{"path": authPath})
	if err != nil {
		t.Fatalf("import: %v", err)
	}
	if res["connected"] != true || res["email"] != "owner@example.com" {
		t.Fatalf("import result = %v", res)
	}

	// Logout clears it.
	if _, err := c.Call(ctx, controlplane.CmdProviderOAuthLogout, nil); err != nil {
		t.Fatalf("logout: %v", err)
	}
	st, _ := c.Call(ctx, controlplane.CmdProviderOAuthStatus, map[string]any{"state": ""})
	if st["connected"] != false {
		t.Fatalf("still connected after logout: %v", st)
	}
}
