// SPDX-License-Identifier: MIT

package discord

import (
	"context"
	"crypto/ed25519"
	"crypto/rand"
	"encoding/hex"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"strconv"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	"github.com/agezt/agezt/kernel/channel"
)

// keypair returns a fresh Ed25519 keypair and the public key as hex (the form
// Config.PublicKey takes).
func keypair(t *testing.T) (ed25519.PrivateKey, string) {
	t.Helper()
	pub, priv, err := ed25519.GenerateKey(rand.Reader)
	if err != nil {
		t.Fatalf("GenerateKey: %v", err)
	}
	return priv, hex.EncodeToString(pub)
}

// postInteraction signs body with priv (unless badSig) and delivers it to the
// channel's interactions endpoint, returning the recorder.
func postInteraction(t *testing.T, c *Channel, priv ed25519.PrivateKey, body []byte, badSig bool, ts string) *httptest.ResponseRecorder {
	t.Helper()
	if ts == "" {
		ts = strconv.FormatInt(time.Now().Unix(), 10)
	}
	req := httptest.NewRequest(http.MethodPost, InteractionsPath, strings.NewReader(string(body)))
	req.Header.Set("X-Signature-Timestamp", ts)
	if badSig {
		req.Header.Set("X-Signature-Ed25519", hex.EncodeToString(make([]byte, ed25519.SignatureSize)))
	} else {
		sig := ed25519.Sign(priv, append([]byte(ts), body...))
		req.Header.Set("X-Signature-Ed25519", hex.EncodeToString(sig))
	}
	rec := httptest.NewRecorder()
	c.Handler().ServeHTTP(rec, req)
	return rec
}

func TestDiscord_PingPong(t *testing.T) {
	priv, pub := keypair(t)
	c := New(Config{PublicKey: pub})
	rec := postInteraction(t, c, priv, []byte(`{"type":1}`), false, "")
	if rec.Code != 200 {
		t.Fatalf("ping code = %d want 200", rec.Code)
	}
	var resp map[string]any
	if err := json.Unmarshal(rec.Body.Bytes(), &resp); err != nil {
		t.Fatalf("bad json: %v", err)
	}
	if resp["type"] != float64(responsePong) {
		t.Errorf("ping reply type = %v want %d (PONG)", resp["type"], responsePong)
	}
}

func TestDiscord_BadSignatureRejected(t *testing.T) {
	priv, pub := keypair(t)
	c := New(Config{PublicKey: pub})
	if rec := postInteraction(t, c, priv, []byte(`{"type":1}`), true, ""); rec.Code != 401 {
		t.Errorf("bad signature code = %d want 401", rec.Code)
	}
	// A stale timestamp (> 5 min) is rejected even with an otherwise-valid sig.
	old := strconv.FormatInt(time.Now().Add(-10*time.Minute).Unix(), 10)
	if rec := postInteraction(t, c, priv, []byte(`{"type":1}`), false, old); rec.Code != 401 {
		t.Errorf("stale timestamp code = %d want 401", rec.Code)
	}
	// A different keypair's signature must not verify against our public key.
	other, _ := keypair(t)
	if rec := postInteraction(t, c, other, []byte(`{"type":1}`), false, ""); rec.Code != 401 {
		t.Errorf("wrong-key signature code = %d want 401", rec.Code)
	}
}

// discordAPI is an httptest stand-in for Discord's HTTP API: captures follow-up
// webhook bodies and returns 200.
func discordAPI(t *testing.T, posted chan<- map[string]any) *httptest.Server {
	t.Helper()
	return httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if !strings.Contains(r.URL.Path, "/webhooks/") && !strings.HasSuffix(r.URL.Path, "/messages") {
			http.NotFound(w, r)
			return
		}
		b, _ := io.ReadAll(r.Body)
		var m map[string]any
		_ = json.Unmarshal(b, &m)
		posted <- m
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte(`{}`))
	}))
}

func TestDiscord_CommandDrivesAgentAndFollowsUp(t *testing.T) {
	priv, pub := keypair(t)
	posted := make(chan map[string]any, 1)
	api := discordAPI(t, posted)
	defer api.Close()

	var got atomic.Value
	c := New(Config{
		PublicKey: pub, Token: "bot-test", ApplicationID: "APP1",
		BaseURL:    api.URL,
		HTTPClient: api.Client(),
		Allowlist:  channel.NewAllowlist([]string{"C1"}),
		Handler: func(_ context.Context, msg channel.UnifiedMessage, _ string) (string, error) {
			got.Store(msg.Text)
			return "pong", nil
		},
	})

	body := []byte(`{"type":2,"id":"I1","token":"tok-xyz","channel_id":"C1","member":{"user":{"id":"U1"}},"data":{"name":"agezt","options":[{"name":"prompt","type":3,"value":"ping"}]}}`)
	rec := postInteraction(t, c, priv, body, false, "")
	if rec.Code != 200 {
		t.Fatalf("command ACK code = %d want 200", rec.Code)
	}
	var resp map[string]any
	_ = json.Unmarshal(rec.Body.Bytes(), &resp)
	if resp["type"] != float64(responseDeferred) {
		t.Errorf("command ACK type = %v want %d (deferred)", resp["type"], responseDeferred)
	}

	select {
	case m := <-posted:
		if m["content"] != "pong" {
			t.Errorf("follow-up content = %v want pong", m["content"])
		}
	case <-time.After(2 * time.Second):
		t.Fatal("timed out waiting for follow-up webhook")
	}
	if got.Load() != "ping" {
		t.Errorf("handler saw text %v want ping", got.Load())
	}
}

func TestDiscord_IgnoresNonAllowlisted(t *testing.T) {
	priv, pub := keypair(t)
	posted := make(chan map[string]any, 4)
	api := discordAPI(t, posted)
	defer api.Close()

	var calls int32
	c := New(Config{
		PublicKey: pub, Token: "bot", ApplicationID: "APP", BaseURL: api.URL, HTTPClient: api.Client(),
		Allowlist: channel.NewAllowlist([]string{"C1"}),
		Handler: func(_ context.Context, _ channel.UnifiedMessage, _ string) (string, error) {
			atomic.AddInt32(&calls, 1)
			return "reply", nil
		},
	})

	// A non-allowlisted channel: handler not run; the interaction is answered
	// immediately (ephemeral "not authorized"), never deferred.
	body := []byte(`{"type":2,"id":"I2","token":"t","channel_id":"CX","member":{"user":{"id":"U9"}},"data":{"name":"agezt","options":[{"name":"prompt","type":3,"value":"hi"}]}}`)
	rec := postInteraction(t, c, priv, body, false, "")
	if rec.Code != 200 {
		t.Fatalf("code = %d want 200", rec.Code)
	}
	var resp map[string]any
	_ = json.Unmarshal(rec.Body.Bytes(), &resp)
	if resp["type"] != float64(responseMessage) {
		t.Errorf("non-allowlisted reply type = %v want %d (immediate message, not deferred)", resp["type"], responseMessage)
	}
	if data, _ := resp["data"].(map[string]any); data == nil || !strings.Contains(data["content"].(string), "not authorized") {
		t.Errorf("expected 'not authorized' content, got %v", resp["data"])
	}

	time.Sleep(150 * time.Millisecond)
	if n := atomic.LoadInt32(&calls); n != 0 {
		t.Errorf("handler ran %d time(s); a non-allowlisted command must not drive the agent", n)
	}
	select {
	case m := <-posted:
		t.Errorf("no follow-up expected for a denied command, got %v", m)
	default:
	}
}

func TestDiscord_AttachmentClassification(t *testing.T) {
	in := discordInteraction{
		Data: &discordData{
			Options: []discordOption{
				{Type: optionTypeAttachment, Value: json.RawMessage(`"img"`)},
				{Type: optionTypeAttachment, Value: json.RawMessage(`"voice"`)},
				{Type: optionTypeAttachment, Value: json.RawMessage(`"doc"`)},
			},
			Resolved: &discordResolved{Attachments: map[string]discordAttachment{
				"img":   {URL: "https://cdn/x.png", ContentType: "image/png"},
				"voice": {URL: "https://cdn/x.ogg", ContentType: "audio/ogg"},
				"doc":   {URL: "https://cdn/x.pdf", ContentType: "application/pdf"},
			}},
		},
	}
	imgs := in.imageAttachments()
	if len(imgs) != 1 || imgs[0].ContentType != "image/png" {
		t.Fatalf("imageAttachments = %+v", imgs)
	}
	auds := in.audioAttachments()
	if len(auds) != 1 || auds[0].ContentType != "audio/ogg" {
		t.Fatalf("audioAttachments = %+v", auds)
	}
}
