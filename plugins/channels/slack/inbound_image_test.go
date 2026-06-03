// SPDX-License-Identifier: MIT

package slack

import (
	"context"
	"encoding/base64"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/agezt/agezt/kernel/channel"
)

// An inbound image file is downloaded (url_private, with the bot token) and
// handed to the handler as a data: URL so a vision model can see it (M248).
func TestSlack_InboundImageFileBecomesDataURL(t *testing.T) {
	raw := []byte{0x89, 0x50, 0x4e, 0x47, 1, 2, 3, 4}
	posted := make(chan map[string]any, 1)
	gotImages := make(chan []string, 1)
	var gotAuth string
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch {
		case strings.HasSuffix(r.URL.Path, "/chat.postMessage"):
			posted <- map[string]any{"ok": true}
			w.Header().Set("Content-Type", "application/json")
			_, _ = w.Write([]byte(`{"ok":true}`))
		case strings.Contains(r.URL.Path, "/files/"):
			gotAuth = r.Header.Get("Authorization")
			_, _ = w.Write(raw)
		default:
			http.NotFound(w, r)
		}
	}))
	defer srv.Close()

	c := New(Config{
		SigningSecret: secret, Token: "xoxb-test", BaseURL: srv.URL,
		HTTPClient: srv.Client(),
		Allowlist:  channel.NewAllowlist([]string{"C1"}),
		Handler: func(_ context.Context, msg channel.UnifiedMessage, _ string) (string, error) {
			gotImages <- msg.Images
			return "seen", nil
		},
	})

	fileURL := srv.URL + "/files/pic.png"
	body := []byte(`{"type":"event_callback","event":{"type":"message","channel":"C1","user":"U1","text":"what is this?","ts":"1700000000.000100","files":[{"url_private":"` + fileURL + `","mimetype":"image/png","name":"pic.png"}]}}`)
	if rec := postEvent(t, c, body, false, ""); rec.Code != 200 {
		t.Fatalf("event ACK = %d, want 200", rec.Code)
	}

	select {
	case imgs := <-gotImages:
		want := "data:image/png;base64," + base64.StdEncoding.EncodeToString(raw)
		if len(imgs) != 1 || imgs[0] != want {
			t.Errorf("Images=%v\nwant [%s]", imgs, want)
		}
	case <-time.After(2 * time.Second):
		t.Fatal("timed out waiting for the handler")
	}
	<-posted
	if !strings.Contains(gotAuth, "xoxb-test") {
		t.Errorf("file download missing bot-token auth header: %q", gotAuth)
	}
}

// A non-image file (e.g. a text document) is not fetched or attached.
func TestSlack_InboundNonImageFileSkipped(t *testing.T) {
	posted := make(chan map[string]any, 1)
	gotImages := make(chan []string, 1)
	fetched := false
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if strings.Contains(r.URL.Path, "/files/") {
			fetched = true
		}
		if strings.HasSuffix(r.URL.Path, "/chat.postMessage") {
			posted <- map[string]any{"ok": true}
		}
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"ok":true}`))
	}))
	defer srv.Close()

	c := New(Config{
		SigningSecret: secret, Token: "xoxb-test", BaseURL: srv.URL,
		HTTPClient: srv.Client(),
		Allowlist:  channel.NewAllowlist([]string{"C1"}),
		Handler: func(_ context.Context, msg channel.UnifiedMessage, _ string) (string, error) {
			gotImages <- msg.Images
			return "ok", nil
		},
	})

	fileURL := srv.URL + "/files/notes.txt"
	body := []byte(`{"type":"event_callback","event":{"type":"message","channel":"C1","user":"U1","text":"hi","ts":"1700000000.000200","files":[{"url_private":"` + fileURL + `","mimetype":"text/plain","name":"notes.txt"}]}}`)
	if rec := postEvent(t, c, body, false, ""); rec.Code != 200 {
		t.Fatalf("event ACK = %d, want 200", rec.Code)
	}

	select {
	case imgs := <-gotImages:
		if len(imgs) != 0 {
			t.Errorf("Images=%v, want none for a non-image file", imgs)
		}
	case <-time.After(2 * time.Second):
		t.Fatal("timed out waiting for the handler")
	}
	<-posted
	if fetched {
		t.Error("a non-image file was downloaded")
	}
}
