// SPDX-License-Identifier: MIT

package matrix

import (
	"context"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/agezt/agezt/kernel/channel"
)

func TestSendMediaUploadsThenPosts(t *testing.T) {
	var uploaded bool
	var eventMsgType string
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch {
		case strings.Contains(r.URL.Path, "/_matrix/media/v3/upload"):
			uploaded = true
			_, _ = io.WriteString(w, `{"content_uri":"mxc://test/abc"}`)
		case strings.Contains(r.URL.Path, "/send/m.room.message/"):
			b, _ := io.ReadAll(r.Body)
			var m map[string]any
			_ = json.Unmarshal(b, &m)
			eventMsgType, _ = m["msgtype"].(string)
			_, _ = io.WriteString(w, `{"event_id":"$e"}`)
		}
	}))
	defer srv.Close()
	c := New(Config{Homeserver: srv.URL, Token: "tok", HTTPClient: srv.Client()})
	err := c.Send(context.Background(), channel.Outbound{
		ChannelID:   "!room:test",
		Attachments: []channel.Attachment{{Kind: "audio", Data: []byte("fake"), MIME: "audio/ogg", Filename: "reply.ogg"}},
	})
	if err != nil {
		t.Fatal(err)
	}
	if !uploaded {
		t.Fatal("media was not uploaded")
	}
	if eventMsgType != "m.audio" {
		t.Fatalf("msgtype = %q, want m.audio", eventMsgType)
	}
}

func TestFetchMXC(t *testing.T) {
	var gotPath, gotAuth string
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		gotPath, gotAuth = r.URL.Path, r.Header.Get("Authorization")
		w.Header().Set("Content-Type", "image/png")
		_, _ = w.Write([]byte("\x89PNGdata"))
	}))
	defer srv.Close()
	c := New(Config{Homeserver: srv.URL, Token: "tok", HTTPClient: srv.Client()})

	du := c.fetchMXC(context.Background(), "mxc://home.test/MEDIA1", "")
	if !strings.HasPrefix(du, "data:image/png;base64,") {
		t.Fatalf("data URL = %q", du)
	}
	if gotPath != "/_matrix/media/v3/download/home.test/MEDIA1" {
		t.Fatalf("download path = %q", gotPath)
	}
	if gotAuth != "Bearer tok" {
		t.Fatalf("auth = %q", gotAuth)
	}
	// A malformed mxc URI yields "" without a request.
	if du := c.fetchMXC(context.Background(), "not-an-mxc", ""); du != "" {
		t.Fatalf("malformed mxc should yield empty, got %q", du)
	}
}
