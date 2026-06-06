// SPDX-License-Identifier: MIT

package discord

import (
	"context"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/agezt/agezt/kernel/channel"
)

// fetchAttachmentDataURL bounds an inbound attachment download (an untrusted CDN
// body) so a hostile/oversized file can't be streamed unbounded into memory. The
// guard reads MaxRaw+1 and rejects when len > MaxRaw. The happy-path image test
// uses a tiny body, so neither boundary is exercised:
//   - exactly discordAttachMaxRaw bytes must be accepted (inclusive cap);
//   - discordAttachMaxRaw+1 must be rejected — which depends on the LimitReader
//     reading MaxRaw+1 bytes; capping at MaxRaw would let an oversized body read
//     as exactly MaxRaw and pass `> MaxRaw` truncated.
func TestDiscord_AttachmentSizeCapBoundary(t *testing.T) {
	chanWith := func(cdn *httptest.Server) *Channel {
		return New(Config{
			PublicKey: "00", Token: "bot-test", ApplicationID: "APP1",
			HTTPClient: cdn.Client(),
			Allowlist:  channel.NewAllowlist([]string{"C1"}),
		})
	}
	serveN := func(n int) *httptest.Server {
		body := make([]byte, n)
		for i := range body {
			body[i] = 'a'
		}
		return httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
			_, _ = w.Write(body)
		}))
	}

	t.Run("exactly max accepted", func(t *testing.T) {
		cdn := serveN(discordAttachMaxRaw)
		defer cdn.Close()
		c := chanWith(cdn)
		got, err := c.fetchAttachmentDataURL(context.Background(),
			discordAttachment{URL: cdn.URL + "/a.png", ContentType: "image/png"})
		if err != nil {
			t.Fatalf("an attachment of exactly discordAttachMaxRaw bytes must be accepted, got: %v", err)
		}
		if !strings.HasPrefix(got, "data:image/png;base64,") {
			t.Errorf("want a data URL, got %.40q…", got)
		}
	})

	t.Run("one past max rejected", func(t *testing.T) {
		cdn := serveN(discordAttachMaxRaw + 1)
		defer cdn.Close()
		c := chanWith(cdn)
		_, err := c.fetchAttachmentDataURL(context.Background(),
			discordAttachment{URL: cdn.URL + "/a.png", ContentType: "image/png"})
		if err == nil || !strings.Contains(err.Error(), "exceeds") {
			t.Fatalf("an attachment of discordAttachMaxRaw+1 bytes must be rejected, got err=%v", err)
		}
	})
}
