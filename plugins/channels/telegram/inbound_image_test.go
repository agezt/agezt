// SPDX-License-Identifier: MIT

package telegram

import (
	"context"
	"encoding/base64"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/agezt/agezt/kernel/channel"
)

// An inbound photo is fetched (getFile → download) and handed to the handler as
// a data: URL, so a vision model can see it (M247).
func TestInbound_PhotoBecomesImageDataURL(t *testing.T) {
	raw := []byte{0xff, 0xd8, 0xff, 0xe0, 1, 2, 3, 4} // JPEG-ish bytes
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch {
		case strings.Contains(r.URL.Path, "/getFile"):
			_ = json.NewEncoder(w).Encode(map[string]any{
				"ok":     true,
				"result": map[string]any{"file_path": "photos/file_7.jpg"},
			})
		case strings.Contains(r.URL.Path, "/file/bot"):
			w.Write(raw)
		case strings.Contains(r.URL.Path, "/sendMessage"):
			w.WriteHeader(200)
			_, _ = w.Write([]byte(`{"ok":true}`))
		default:
			w.WriteHeader(404)
		}
	}))
	defer srv.Close()

	var got channel.UnifiedMessage
	h := func(_ context.Context, msg channel.UnifiedMessage, _ string) (string, error) {
		got = msg
		return "seen", nil
	}
	c, _ := newTestChannel(t, srv, channel.NewAllowlist([]string{"42"}), h)

	c.handleInbound(context.Background(), &tgMessage{
		MessageID: 1,
		Chat:      tgChat{ID: 42},
		From:      &tgUser{Username: "ersin"},
		Caption:   "what is this?",
		Photo: []tgPhotoSize{
			{FileID: "small", Width: 90, Height: 90},
			{FileID: "big", Width: 1280, Height: 1280}, // largest is last
		},
	})

	want := "data:image/jpeg;base64," + base64.StdEncoding.EncodeToString(raw)
	if len(got.Images) != 1 || got.Images[0] != want {
		t.Errorf("Images=%v\nwant [%s]", got.Images, want)
	}
	// The caption is surfaced as the message text.
	if got.Text != "what is this?" {
		t.Errorf("Text=%q, want caption", got.Text)
	}
}

// A photo from a non-allowlisted sender is never fetched (no file dereference
// for an unauthorized sender).
func TestInbound_PhotoNotFetchedForRejectedSender(t *testing.T) {
	fetched := false
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if strings.Contains(r.URL.Path, "/getFile") || strings.Contains(r.URL.Path, "/file/bot") {
			fetched = true
		}
		w.WriteHeader(200)
		_, _ = w.Write([]byte(`{"ok":true}`))
	}))
	defer srv.Close()

	c, _ := newTestChannel(t, srv, channel.NewAllowlist([]string{"42"}), nil)
	c.handleInbound(context.Background(), &tgMessage{
		Chat:  tgChat{ID: 999}, // not allowlisted
		Photo: []tgPhotoSize{{FileID: "x"}},
	})
	if fetched {
		t.Error("photo was fetched for a non-allowlisted sender")
	}
}

func TestTgMediaType(t *testing.T) {
	cases := map[string]string{
		"photos/a.jpg":  "image/jpeg",
		"photos/a.jpeg": "image/jpeg",
		"x.png":         "image/png",
		"y.gif":         "image/gif",
		"z.webp":        "image/webp",
		"noext":         "image/jpeg",
	}
	for path, want := range cases {
		if got := tgMediaType(path); got != want {
			t.Errorf("tgMediaType(%q)=%q, want %q", path, got, want)
		}
	}
}
