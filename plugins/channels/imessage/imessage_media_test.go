// SPDX-License-Identifier: MIT

package imessage

import (
	"context"
	"fmt"
	"net/http"
	"net/http/httptest"
	"net/url"
	"strings"
	"testing"

	"github.com/agezt/agezt/kernel/channel"
)

func TestScrubURLError(t *testing.T) {
	raw := &url.Error{Op: "Get", URL: "http://host:1234/api/v1/message/text?password=s3cr3t", Err: fmt.Errorf("dial tcp: timeout")}
	scrubbed := scrubURLError(raw)
	if strings.Contains(scrubbed.Error(), "s3cr3t") || strings.Contains(scrubbed.Error(), "password") {
		t.Fatalf("password leaked in scrubbed error: %v", scrubbed)
	}
	// A non-URL error passes through unchanged.
	plain := fmt.Errorf("boom")
	if scrubURLError(plain).Error() != "boom" {
		t.Fatal("non-url error should pass through")
	}
}

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
