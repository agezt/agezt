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
