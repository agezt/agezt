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

// fetchPhotoDataURL bounds an inbound photo download so a hostile/buggy peer
// can't stream an unbounded body into memory. The guard reads MaxRaw+1 bytes and
// rejects when len > MaxRaw. Two things must hold at the boundary, neither of
// which the happy-path image test exercises:
//   - a photo of EXACTLY tgPhotoMaxRaw bytes is accepted (the cap is inclusive);
//   - a photo of tgPhotoMaxRaw+1 bytes is rejected — which only works because the
//     LimitReader caps at MaxRaw+1, letting len(data) reach MaxRaw+1 so `> MaxRaw`
//     fires. Were the reader capped at MaxRaw, an oversized body would read as
//     exactly MaxRaw and slip through truncated.
func TestInbound_PhotoSizeCapBoundary(t *testing.T) {
	serveN := func(n int) *httptest.Server {
		body := make([]byte, n)
		for i := range body {
			body[i] = 'a'
		}
		return httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			switch {
			case strings.Contains(r.URL.Path, "/getFile"):
				_, _ = w.Write([]byte(`{"ok":true,"result":{"file_path":"photos/f.jpg"}}`))
			case strings.Contains(r.URL.Path, "/file/bot"):
				_, _ = w.Write(body)
			default:
				w.WriteHeader(404)
			}
		}))
	}

	t.Run("exactly max accepted", func(t *testing.T) {
		srv := serveN(tgPhotoMaxRaw)
		defer srv.Close()
		c, _ := newTestChannel(t, srv, channel.NewAllowlist([]string{"42"}), nil)
		got, err := c.fetchPhotoDataURL(context.Background(), "big")
		if err != nil {
			t.Fatalf("a photo of exactly tgPhotoMaxRaw bytes must be accepted, got error: %v", err)
		}
		if !strings.HasPrefix(got, "data:image/jpeg;base64,") {
			t.Errorf("want a data URL, got %.40q…", got)
		}
	})

	t.Run("one past max rejected", func(t *testing.T) {
		srv := serveN(tgPhotoMaxRaw + 1)
		defer srv.Close()
		c, _ := newTestChannel(t, srv, channel.NewAllowlist([]string{"42"}), nil)
		_, err := c.fetchPhotoDataURL(context.Background(), "big")
		if err == nil || !strings.Contains(err.Error(), "exceeds") {
			t.Fatalf("a photo of tgPhotoMaxRaw+1 bytes must be rejected as oversize, got err=%v", err)
		}
	})
}
