// SPDX-License-Identifier: MIT

package whatsapp

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

func TestSendMediaTwoStep(t *testing.T) {
	var uploadHit, msgType string
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if strings.HasSuffix(r.URL.Path, "/media") {
			uploadHit = r.Header.Get("Content-Type")
			_, _ = io.WriteString(w, `{"id":"media-99"}`)
			return
		}
		// /messages
		b, _ := io.ReadAll(r.Body)
		var m map[string]any
		_ = json.Unmarshal(b, &m)
		if t, _ := m["type"].(string); t != "" {
			msgType = t
		}
		if a, ok := m["audio"].(map[string]any); ok {
			if a["id"] != "media-99" {
				t.Errorf("audio id = %v", a["id"])
			}
		}
		_, _ = io.WriteString(w, `{"messages":[{"id":"wamid.out"}]}`)
	}))
	defer srv.Close()
	c := New(Config{AccessToken: "tok", PhoneNumberID: "PN1", GraphBase: srv.URL, HTTPClient: srv.Client()})
	err := c.Send(context.Background(), channel.Outbound{
		ChannelID:   "+15551230001",
		Attachments: []channel.Attachment{{Kind: "audio", Data: []byte("fake-ogg"), MIME: "audio/ogg", Filename: "reply.ogg"}},
	})
	if err != nil {
		t.Fatal(err)
	}
	if !strings.HasPrefix(uploadHit, "multipart/form-data") {
		t.Fatalf("upload content-type = %q", uploadHit)
	}
	if msgType != "audio" {
		t.Fatalf("message type = %q, want audio", msgType)
	}
}
