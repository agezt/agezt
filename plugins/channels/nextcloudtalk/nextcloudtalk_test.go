// SPDX-License-Identifier: MIT

package nextcloudtalk

import (
	"context"
	"crypto/hmac"
	"crypto/sha256"
	"encoding/hex"
	"io"
	"net/http"
	"net/http/httptest"
	"net/url"
	"strings"
	"testing"

	"github.com/agezt/agezt/kernel/channel"
)

const secret = "sh4r3d-s3cr3t"

func sigFor(random string, body []byte) string {
	mac := hmac.New(sha256.New, []byte(secret))
	mac.Write([]byte(random))
	mac.Write(body)
	return hex.EncodeToString(mac.Sum(nil))
}

func TestVerify(t *testing.T) {
	c := New(Config{Secret: secret})
	body := []byte(`{"type":"Create"}`)
	if !c.verify("rnd1", sigFor("rnd1", body), body) {
		t.Fatal("valid signature rejected")
	}
	if c.verify("rnd1", sigFor("rnd2", body), body) {
		t.Fatal("signature must depend on the random value")
	}
	if c.verify("", "", body) {
		t.Fatal("empty random/sig must fail closed")
	}
	// Empty secret fails closed even with a 'matching' empty-key HMAC.
	c2 := New(Config{})
	if c2.verify("r", sigFor("r", body), body) {
		t.Fatal("empty secret must fail closed")
	}
}

func TestParseActivity(t *testing.T) {
	body := []byte(`{"type":"Create","actor":{"id":"users/alice"},"object":{"id":"msg-1","content":"{\"message\":\"hello bot\"}"},"target":{"id":"tok123"}}`)
	m, ok := parseActivity(body)
	if !ok || m.token != "tok123" || m.sender != "users/alice" || m.text != "hello bot" || m.id != "msg-1" {
		t.Fatalf("parse = %+v ok=%v", m, ok)
	}
	// Non-Create events (reactions, joins) are dropped.
	if _, ok := parseActivity([]byte(`{"type":"Like"}`)); ok {
		t.Fatal("Like should be dropped")
	}
	// Plain (non-JSON) content falls back to the raw string.
	m2, ok := parseActivity([]byte(`{"type":"Create","object":{"content":"raw text"},"target":{"id":"t"}}`))
	if !ok || m2.text != "raw text" {
		t.Fatalf("raw content = %+v", m2)
	}
}

func TestSendSignsAndPosts(t *testing.T) {
	var gotPath, gotRandom, gotSig, gotMsg, gotOCS string
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		gotPath = r.URL.Path
		gotRandom = r.Header.Get("X-Nextcloud-Talk-Bot-Random")
		gotSig = r.Header.Get("X-Nextcloud-Talk-Bot-Signature")
		gotOCS = r.Header.Get("OCS-APIRequest")
		b, _ := io.ReadAll(r.Body)
		vals, _ := url.ParseQuery(string(b))
		gotMsg = vals.Get("message")
		_, _ = io.WriteString(w, `{"ocs":{}}`)
	}))
	defer srv.Close()

	c := New(Config{ServerURL: srv.URL, Secret: secret, HTTPClient: srv.Client()})
	if err := c.Send(context.Background(), channel.Outbound{ChannelID: "tok9", Text: "hi there"}); err != nil {
		t.Fatal(err)
	}
	if gotPath != "/ocs/v2.php/apps/spreed/api/v1/bot/tok9/message" {
		t.Fatalf("path = %q", gotPath)
	}
	if gotOCS != "true" {
		t.Fatalf("OCS-APIRequest = %q", gotOCS)
	}
	if gotMsg != "hi there" {
		t.Fatalf("message = %q", gotMsg)
	}
	// The signature must match HMAC(secret, random || message).
	if gotSig != sigFor(gotRandom, []byte("hi there")) {
		t.Fatalf("signature mismatch: got %q", gotSig)
	}
}

func TestSendRequiresServerAndSecret(t *testing.T) {
	if err := (New(Config{Secret: secret})).Send(context.Background(), channel.Outbound{ChannelID: "t", Text: "x"}); err == nil {
		t.Fatal("send without ServerURL should error")
	}
	if err := (New(Config{ServerURL: "http://x"})).Send(context.Background(), channel.Outbound{ChannelID: "t", Text: "x"}); err == nil {
		t.Fatal("send without secret should error")
	}
}

func TestDispatchAllowlistAndReply(t *testing.T) {
	var replied string
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		b, _ := io.ReadAll(r.Body)
		vals, _ := url.ParseQuery(string(b))
		replied = vals.Get("message")
		_, _ = io.WriteString(w, `{"ocs":{}}`)
	}))
	defer srv.Close()

	c := New(Config{
		ServerURL: srv.URL,
		Secret:    secret,
		Allowlist: channel.NewAllowlist([]string{"tokOK"}),
		Handler: func(_ context.Context, msg channel.UnifiedMessage, _ string) (channel.Reply, error) {
			return channel.Reply{Text: "echo: " + msg.Text}, nil
		},
	})

	// Allowlisted conversation → handler runs, reply posted.
	c.dispatch(context.Background(), inbound{token: "tokOK", sender: "users/a", text: "ping", id: "m1"})
	if replied != "echo: ping" {
		t.Fatalf("reply = %q", replied)
	}
	// Non-allowlisted conversation → no reply.
	replied = ""
	c.dispatch(context.Background(), inbound{token: "tokNO", sender: "users/a", text: "ping", id: "m2"})
	if replied != "" {
		t.Fatalf("non-allowlisted should not reply, got %q", replied)
	}
	// Duplicate id → skipped.
	replied = ""
	c.dispatch(context.Background(), inbound{token: "tokOK", sender: "users/a", text: "again", id: "m1"})
	if replied != "" {
		t.Fatalf("duplicate id should be skipped, got %q", replied)
	}
}

func TestHandleInboundRejectsBadSignature(t *testing.T) {
	c := New(Config{Secret: secret})
	body := `{"type":"Create","object":{"content":"{\"message\":\"x\"}"},"target":{"id":"t"}}`
	req := httptest.NewRequest(http.MethodPost, "/nextcloudtalk", strings.NewReader(body))
	req.Header.Set("X-Nextcloud-Talk-Random", "r")
	req.Header.Set("X-Nextcloud-Talk-Signature", "deadbeef")
	w := httptest.NewRecorder()
	c.handleInbound(w, req)
	if w.Code != http.StatusUnauthorized {
		t.Fatalf("bad signature status = %d, want 401", w.Code)
	}
}
