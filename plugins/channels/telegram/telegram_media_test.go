// SPDX-License-Identifier: MIT

package telegram

import (
	"context"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/agezt/agezt/kernel/channel"
)

func TestSendVoiceAttachment(t *testing.T) {
	var path, ctype string
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		path = r.URL.Path
		ctype = r.Header.Get("Content-Type")
		_, _ = w.Write([]byte(`{"ok":true}`))
	}))
	defer srv.Close()
	c := New(Config{Token: "T", BaseURL: srv.URL, HTTPClient: srv.Client()})
	err := c.Send(context.Background(), channel.Outbound{
		ChannelID:   "42",
		Attachments: []channel.Attachment{{Kind: "audio", Data: []byte("OggS-fake"), MIME: "audio/ogg", Filename: "reply.ogg"}},
	})
	if err != nil {
		t.Fatal(err)
	}
	if !strings.HasSuffix(path, "/sendVoice") {
		t.Fatalf("path = %q, want .../sendVoice", path)
	}
	if !strings.HasPrefix(ctype, "multipart/form-data") {
		t.Fatalf("content-type = %q, want multipart", ctype)
	}
}

func TestSendPhotoAttachment(t *testing.T) {
	var path string
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		path = r.URL.Path
		_, _ = w.Write([]byte(`{"ok":true}`))
	}))
	defer srv.Close()
	c := New(Config{Token: "T", BaseURL: srv.URL, HTTPClient: srv.Client()})
	if err := c.Send(context.Background(), channel.Outbound{
		ChannelID:   "42",
		Attachments: []channel.Attachment{{Kind: "image", Data: []byte("PNG-fake"), MIME: "image/png", Filename: "x.png"}},
	}); err != nil {
		t.Fatal(err)
	}
	if !strings.HasSuffix(path, "/sendPhoto") {
		t.Fatalf("path = %q, want .../sendPhoto", path)
	}
}
