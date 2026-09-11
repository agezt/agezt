// SPDX-License-Identifier: MIT

package webui

// OAuth callback handler (the post-`agt webui login` redirect endpoint)
// plus the htmlEscape helper. Carved out of webui.go during the Day 26
// god file split #1 so the main file can focus on auth + routing.

import (
	"context"
	"fmt"
	"net/http"
	"strings"
	"time"

	"github.com/agezt/agezt/kernel/controlplane"
)

func (s *Server) handleOAuthCallback(w http.ResponseWriter, r *http.Request) {
	q := r.URL.Query()
	code := q.Get("code")
	state := q.Get("state")
	if e := q.Get("error"); e != "" {
		oauthResultPage(w, false, "Authorization was denied: "+e)
		return
	}
	if code == "" || state == "" {
		oauthResultPage(w, false, "Missing code or state in the redirect.")
		return
	}
	ctx, cancel := context.WithTimeout(r.Context(), 30*time.Second)
	defer cancel()
	if _, err := s.client.Call(ctx, controlplane.CmdChannelOAuthCallback, map[string]any{
		"code": code, "state": state,
	}); err != nil {
		oauthResultPage(w, false, err.Error())
		return
	}
	oauthResultPage(w, true, "")
}

// oauthResultPage renders a minimal terminal page for the OAuth redirect. The
// console polls /api/channel/oauth/status for the real outcome, so this is just
// operator-facing confirmation they can close.
func oauthResultPage(w http.ResponseWriter, ok bool, msg string) {
	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	title, detail := "Connected ✓", "You can close this window and return to the console."
	if !ok {
		title, detail = "Connection failed", htmlEscape(msg)
	}
	w.WriteHeader(http.StatusOK)
	fmt.Fprintf(w, `<!doctype html><meta charset="utf-8"><title>%s</title>`+
		`<body style="font:16px system-ui;display:grid;place-items:center;height:100vh;margin:0;background:#0b1020;color:#e6e8f0">`+
		`<div style="text-align:center;max-width:32rem;padding:2rem"><h1 style="font-size:1.4rem">%s</h1>`+
		`<p style="opacity:.8">%s</p></div><script>setTimeout(function(){window.close()},1500)</script>`,
		title, title, detail)
}

func htmlEscape(s string) string {
	r := strings.NewReplacer("&", "&amp;", "<", "&lt;", ">", "&gt;", `"`, "&quot;")
	return r.Replace(s)
}

// webhookBodyCap bounds an inbound hook body — payloads are trigger inputs,
// not file uploads.
const webhookBodyCap = 256 * 1024

// Hook rate-limit tuning (VULN-007). A legitimate webhook fires far below this;
// the cap exists to stop a tight abuse loop on a leaked secret, not to meter
// callers. Fixed 60s window with a burst headroom, per workflow+source bucket.
const (
	hookRatePerMin    = 60   // sustained fires/min per workflow+source
	hookRateBurst     = 30   // extra fires allowed within a window
	hookRLMaxBuckets  = 4096 // memory bound across distinct workflow+source keys
	hookRLIdleEvictMs = 5 * 60_000
)

// hookBucket is a fixed-window counter for one workflow+source key. On window
// rollover the count resets; a request is refused once count exceeds max+burst.
type hookBucket struct {
	count     int
	windowEnd int64
	lastSeen  int64
}

// allowHook reports whether a /hooks/ request for key may proceed, counting it
// when so. Buckets are created on demand and evicted when idle; the map is bounded
// so a flood of distinct keys can't grow it without limit.
