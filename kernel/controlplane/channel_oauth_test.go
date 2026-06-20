// SPDX-License-Identifier: MIT

package controlplane_test

import (
	"context"
	"net/url"
	"strings"
	"testing"

	"github.com/agezt/agezt/kernel/controlplane"
	"github.com/agezt/agezt/plugins/builtinchannels"
	"github.com/agezt/agezt/plugins/providers/mock"
)

// TestChannelOAuthStartStatus drives the OAuth start/status handlers through the
// control plane: a Slack flow yields a well-formed authorize URL + state, the
// state reports pending, and unsupported kinds / missing creds are refused.
func TestChannelOAuthStartStatus(t *testing.T) {
	builtinchannels.RegisterAll()
	_, _, c, _ := startPair(t, mock.New(mock.FinalText("ok")))
	ctx := context.Background()

	res, err := c.Call(ctx, controlplane.CmdChannelOAuthStart, map[string]any{
		"kind": "slack", "label": "team-b",
		"client_id": "cid123", "client_secret": "shh",
		"redirect_uri": "https://console.example/oauth/callback",
	})
	if err != nil {
		t.Fatalf("start: %v", err)
	}
	authorize, _ := res["authorize_url"].(string)
	state, _ := res["state"].(string)
	if state == "" {
		t.Fatal("start returned no state")
	}
	u, err := url.Parse(authorize)
	if err != nil || !strings.HasPrefix(authorize, "https://slack.com/oauth/v2/authorize?") {
		t.Fatalf("bad authorize_url: %q (%v)", authorize, err)
	}
	q := u.Query()
	if q.Get("client_id") != "cid123" || q.Get("state") != state || q.Get("response_type") != "code" {
		t.Fatalf("authorize query missing fields: %v", q)
	}
	if q.Get("redirect_uri") != "https://console.example/oauth/callback" {
		t.Fatalf("redirect not echoed: %q", q.Get("redirect_uri"))
	}

	// Status reports pending before any callback.
	st, err := c.Call(ctx, controlplane.CmdChannelOAuthStatus, map[string]any{"state": state})
	if err != nil || st["status"] != "pending" {
		t.Fatalf("status = %v err=%v, want pending", st["status"], err)
	}
	// Unknown state → "unknown".
	st2, _ := c.Call(ctx, controlplane.CmdChannelOAuthStatus, map[string]any{"state": "nope"})
	if st2["status"] != "unknown" {
		t.Fatalf("unknown state status = %v", st2["status"])
	}

	// Unsupported kind refused.
	if _, err := c.Call(ctx, controlplane.CmdChannelOAuthStart, map[string]any{
		"kind": "telegram", "client_id": "a", "client_secret": "b",
		"redirect_uri": "https://x/cb",
	}); err == nil {
		t.Fatal("telegram should not support oauth")
	}
	// Missing client creds refused.
	if _, err := c.Call(ctx, controlplane.CmdChannelOAuthStart, map[string]any{
		"kind": "slack", "redirect_uri": "https://x/cb",
	}); err == nil {
		t.Fatal("missing client_id/secret should be refused")
	}
	// Mastodon requires an instance URL.
	if _, err := c.Call(ctx, controlplane.CmdChannelOAuthStart, map[string]any{
		"kind": "mastodon", "client_id": "a", "client_secret": "b",
		"redirect_uri": "https://x/cb",
	}); err == nil {
		t.Fatal("mastodon without instance_url should be refused")
	}
}
