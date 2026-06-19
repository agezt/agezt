// SPDX-License-Identifier: MIT

package slack

import (
	"context"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/agezt/agezt/kernel/channel"
)

func TestSendFileExternalUpload(t *testing.T) {
	var steps []string
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch {
		case strings.HasSuffix(r.URL.Path, "/files.getUploadURLExternal"):
			steps = append(steps, "geturl")
			_, _ = io.WriteString(w, `{"ok":true,"upload_url":"`+baseURL(r)+`/upload-here","file_id":"F1"}`)
		case strings.HasSuffix(r.URL.Path, "/upload-here"):
			steps = append(steps, "upload")
			w.WriteHeader(200)
		case strings.HasSuffix(r.URL.Path, "/files.completeUploadExternal"):
			steps = append(steps, "complete")
			_, _ = io.WriteString(w, `{"ok":true}`)
		}
	}))
	defer srv.Close()
	c := New(Config{Token: "xoxb-t", BaseURL: srv.URL, HTTPClient: srv.Client()})
	err := c.Send(context.Background(), channel.Outbound{
		ChannelID:   "C1",
		Attachments: []channel.Attachment{{Kind: "audio", Data: []byte("fake"), MIME: "audio/ogg", Filename: "reply.ogg"}},
	})
	if err != nil {
		t.Fatal(err)
	}
	if strings.Join(steps, ",") != "geturl,upload,complete" {
		t.Fatalf("steps = %v, want geturl,upload,complete", steps)
	}
}

func baseURL(r *http.Request) string {
	return "http://" + r.Host
}
