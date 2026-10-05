// SPDX-License-Identifier: MIT

// Package browsercallback owns OAuth browser callback query/result presentation.
// Callers supply admission/effects and listener cleanup independently of HTTP.
package browsercallback

import (
	"context"
	"fmt"
	"net/http"
	"strings"
)

type Complete func(context.Context, string, string, string) (bool, string, bool)

// Handle projects query fields and request context into the caller's completion,
// renders its legacy result page, then schedules requested listener cleanup.
func Handle(w http.ResponseWriter, r *http.Request, complete Complete, closeListener func()) {
	q := r.URL.Query()
	success, message, close := complete(r.Context(), q.Get("code"), q.Get("state"), q.Get("error"))
	Render(w, success, message)
	if close && closeListener != nil {
		go closeListener()
	}
}

// Render writes the existing success/failure browser page, escaping failure text.
func Render(w http.ResponseWriter, ok bool, msg string) {
	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	title, detail := "Signed in ✓", "You can close this window and return to the console."
	if !ok {
		title, detail = "Sign-in failed", escape(msg)
	}
	w.WriteHeader(http.StatusOK)
	fmt.Fprintf(w, `<!doctype html><meta charset="utf-8"><title>%s</title>`+
		`<body style="font:16px system-ui;display:grid;place-items:center;height:100vh;margin:0;background:#0b1020;color:#e6e8f0">`+
		`<div style="text-align:center;max-width:32rem;padding:2rem"><h1 style="font-size:1.4rem">%s</h1>`+
		`<p style="opacity:.8">%s</p></div><script>setTimeout(function(){window.close()},1500)</script>`,
		title, title, detail)
}

func escape(s string) string {
	r := strings.NewReplacer("&", "&amp;", "<", "&lt;", ">", "&gt;", `"`, "&quot;")
	return r.Replace(s)
}
