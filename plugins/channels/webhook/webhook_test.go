// SPDX-License-Identifier: MIT

package webhook

import (
	"context"
	"crypto/hmac"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/agezt/agezt/kernel/channel"
)

func hmacSig(secret string, body []byte) string {
	mac := hmac.New(sha256.New, []byte(secret))
	mac.Write(body)
	return "sha256=" + hex.EncodeToString(mac.Sum(nil))
}

// post issues a signed inbound request to the channel's handler and returns the
// recorder. sign controls whether a (valid) signature header is attached.
func post(t *testing.T, c *Channel, body map[string]any, secret string, signed bool) *httptest.ResponseRecorder {
	t.Helper()
	raw, _ := json.Marshal(body)
	req := httptest.NewRequest(http.MethodPost, c.path, strings.NewReader(string(raw)))
	if signed {
		req.Header.Set("X-Agezt-Signature", hmacSig(secret, raw))
	}
	rec := httptest.NewRecorder()
	c.Handler().ServeHTTP(rec, req)
	return rec
}

// TestInbound_SignedDrivesHandlerAndReplies (M334): a correctly-signed,
// allowlisted message runs the handler and returns its reply synchronously.
func TestInbound_SignedDrivesHandlerAndReplies(t *testing.T) {
	const secret = "s3cr3t"
	var gotText string
	c := New(Config{
		Secret:    secret,
		Allowlist: channel.NewAllowlist([]string{"room1"}),
		Handler: func(_ context.Context, m channel.UnifiedMessage, _ string) (string, error) {
			gotText = m.Text
			return "echo: " + m.Text, nil
		},
	})
	rec := post(t, c, map[string]any{"channel_id": "room1", "sender": "u1", "text": "hi there"}, secret, true)
	if rec.Code != http.StatusOK {
		t.Fatalf("status=%d body=%s", rec.Code, rec.Body.String())
	}
	if gotText != "hi there" {
		t.Errorf("handler saw text=%q", gotText)
	}
	var out struct {
		Reply string `json:"reply"`
		Corr  string `json:"correlation_id"`
	}
	_ = json.Unmarshal(rec.Body.Bytes(), &out)
	if out.Reply != "echo: hi there" {
		t.Errorf("reply=%q", out.Reply)
	}
	if !strings.HasPrefix(out.Corr, "chan-") {
		t.Errorf("correlation_id=%q should start with chan-", out.Corr)
	}
}

// TestInbound_BadSignatureRejected: a wrong/missing signature fails closed (401).
func TestInbound_BadSignatureRejected(t *testing.T) {
	c := New(Config{Secret: "right", Allowlist: channel.NewAllowlist([]string{"room1"}),
		Handler: func(context.Context, channel.UnifiedMessage, string) (string, error) { return "ran", nil }})

	// Signed with the WRONG secret.
	rec := post(t, c, map[string]any{"channel_id": "room1", "text": "x"}, "wrong", true)
	if rec.Code != http.StatusUnauthorized {
		t.Errorf("wrong-secret status=%d want 401", rec.Code)
	}
	// No signature header at all.
	rec = post(t, c, map[string]any{"channel_id": "room1", "text": "x"}, "right", false)
	if rec.Code != http.StatusUnauthorized {
		t.Errorf("unsigned status=%d want 401", rec.Code)
	}
}

// TestInbound_EmptySecretFailsClosed: with no secret configured, inbound is fully
// disabled — even a body that hashes to the empty-key HMAC is rejected.
func TestInbound_EmptySecretFailsClosed(t *testing.T) {
	ran := false
	c := New(Config{Allowlist: channel.NewAllowlist([]string{"room1"}),
		Handler: func(context.Context, channel.UnifiedMessage, string) (string, error) { ran = true; return "", nil }})
	rec := post(t, c, map[string]any{"channel_id": "room1", "text": "x"}, "", true)
	if rec.Code != http.StatusUnauthorized {
		t.Errorf("status=%d want 401 (empty secret must reject)", rec.Code)
	}
	if ran {
		t.Error("handler must not run when inbound is disabled")
	}
}

// TestInbound_AllowlistGates: a signed message from a non-allowlisted channel id
// is authenticated but not authorized — 403, handler not run.
func TestInbound_AllowlistGates(t *testing.T) {
	const secret = "s"
	ran := false
	c := New(Config{Secret: secret, Allowlist: channel.NewAllowlist([]string{"room1"}),
		Handler: func(context.Context, channel.UnifiedMessage, string) (string, error) { ran = true; return "", nil }})
	rec := post(t, c, map[string]any{"channel_id": "intruder", "text": "x"}, secret, true)
	if rec.Code != http.StatusForbidden {
		t.Errorf("status=%d want 403", rec.Code)
	}
	if ran {
		t.Error("handler must not run for a non-allowlisted channel")
	}
}

// TestInbound_StaleTimestampRejected: a signed body whose ts_ms is outside the
// freshness window is rejected (replay protection).
func TestInbound_StaleTimestampRejected(t *testing.T) {
	const secret = "s"
	c := New(Config{Secret: secret, Allowlist: channel.NewAllowlist([]string{"room1"}),
		Handler: func(context.Context, channel.UnifiedMessage, string) (string, error) { return "ok", nil }})
	old := time.Now().Add(-10 * time.Minute).UnixMilli()
	rec := post(t, c, map[string]any{"channel_id": "room1", "text": "x", "ts_ms": old}, secret, true)
	if rec.Code != http.StatusUnauthorized {
		t.Errorf("status=%d want 401 for stale ts_ms", rec.Code)
	}
}

// TestInbound_DedupesRepeatedID: a repeated message id within the window is not
// re-run — the second delivery returns duplicate:true without invoking the agent.
func TestInbound_DedupesRepeatedID(t *testing.T) {
	const secret = "s"
	runs := 0
	c := New(Config{Secret: secret, Allowlist: channel.NewAllowlist([]string{"room1"}),
		Handler: func(context.Context, channel.UnifiedMessage, string) (string, error) { runs++; return "ok", nil }})
	msg := map[string]any{"channel_id": "room1", "text": "x", "id": "msg-1"}
	if rec := post(t, c, msg, secret, true); rec.Code != http.StatusOK {
		t.Fatalf("first delivery status=%d", rec.Code)
	}
	rec := post(t, c, msg, secret, true)
	if rec.Code != http.StatusOK {
		t.Fatalf("second delivery status=%d", rec.Code)
	}
	var out struct {
		Duplicate bool `json:"duplicate"`
	}
	_ = json.Unmarshal(rec.Body.Bytes(), &out)
	if !out.Duplicate {
		t.Error("repeated id should be flagged duplicate")
	}
	if runs != 1 {
		t.Errorf("handler ran %d times, want 1 (dedup)", runs)
	}
}

// TestSend_PostsSignedOutbound: Send POSTs a signed message to the configured
// OutboundURL; with no URL it errors.
func TestSend_PostsSignedOutbound(t *testing.T) {
	const secret = "s"
	var gotSig string
	var gotBody map[string]any
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		gotSig = r.Header.Get("X-Agezt-Signature")
		raw, _ := io.ReadAll(r.Body)
		_ = json.Unmarshal(raw, &gotBody)
		// Verify the signature the receiver would check.
		if gotSig != hmacSig(secret, raw) {
			t.Errorf("outbound signature mismatch")
		}
		w.WriteHeader(http.StatusOK)
	}))
	defer srv.Close()

	c := New(Config{Secret: secret, OutboundURL: srv.URL})
	if err := c.Send(context.Background(), channel.Outbound{ChannelID: "room1", Text: "ping", Priority: channel.PriorityNotify}); err != nil {
		t.Fatalf("Send: %v", err)
	}
	if gotBody["channel_id"] != "room1" || gotBody["text"] != "ping" {
		t.Errorf("outbound body=%+v", gotBody)
	}

	// No OutboundURL → error.
	c2 := New(Config{Secret: secret})
	if err := c2.Send(context.Background(), channel.Outbound{ChannelID: "x", Text: "y"}); err == nil {
		t.Error("Send with no OutboundURL should error")
	}
}
