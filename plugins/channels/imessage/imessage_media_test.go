// SPDX-License-Identifier: MIT

package imessage

import (
	"context"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/agezt/agezt/kernel/channel"
)

func TestSendAttachment(t *testing.T) {
	var path, ctype string
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		path = r.URL.Path
		ctype = r.Header.Get("Content-Type")
	}))
	defer srv.Close()
	c := New(Config{BaseURL: srv.URL, Password: "pw", HTTPClient: srv.Client()})
	err := c.Send(context.Background(), channel.Outbound{
		ChannelID:   "iMessage;-;+15551234567",
		Attachments: []channel.Attachment{{Kind: "audio", Data: []byte("fake"), MIME: "audio/ogg", Filename: "reply.ogg"}},
	})
	if err != nil {
		t.Fatal(err)
	}
	if path != "/api/v1/message/attachment" {
		t.Fatalf("path = %q", path)
	}
	if !strings.HasPrefix(ctype, "multipart/form-data") {
		t.Fatalf("content-type = %q", ctype)
	}
}
