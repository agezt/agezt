// SPDX-License-Identifier: MIT

package slack

import (
	"context"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

// fetchFileDataURL bounds an inbound url_private download (an untrusted file body)
// so a hostile/oversized file can't be streamed unbounded into memory. The guard
// reads MaxRaw+1 and rejects when len > MaxRaw. The happy-path image test uses a
// tiny body, so neither boundary is exercised:
//   - exactly slackFileMaxRaw bytes must be accepted (inclusive cap);
//   - slackFileMaxRaw+1 must be rejected — which depends on the LimitReader reading
//     MaxRaw+1; capping at MaxRaw would let an oversized body read as exactly MaxRaw
//     and pass `> MaxRaw` truncated.
func TestSlack_FileSizeCapBoundary(t *testing.T) {
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
		srv := serveN(slackFileMaxRaw)
		defer srv.Close()
		c := New(Config{Token: "xoxb-test", HTTPClient: srv.Client()})
		got, err := c.fetchFileDataURL(context.Background(), srv.URL+"/f.png", "image/png")
		if err != nil {
			t.Fatalf("a file of exactly slackFileMaxRaw bytes must be accepted, got: %v", err)
		}
		if !strings.HasPrefix(got, "data:image/png;base64,") {
			t.Errorf("want a data URL, got %.40q…", got)
		}
	})

	t.Run("one past max rejected", func(t *testing.T) {
		srv := serveN(slackFileMaxRaw + 1)
		defer srv.Close()
		c := New(Config{Token: "xoxb-test", HTTPClient: srv.Client()})
		_, err := c.fetchFileDataURL(context.Background(), srv.URL+"/f.png", "image/png")
		if err == nil || !strings.Contains(err.Error(), "exceeds") {
			t.Fatalf("a file of slackFileMaxRaw+1 bytes must be rejected, got err=%v", err)
		}
	})
}
