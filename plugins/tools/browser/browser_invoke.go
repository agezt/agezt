// SPDX-License-Identifier: MIT

// Package browser: Invoke (the SSRF-guarded fetch → strip → decode → truncate
// pipeline) + hostAllowed (the AllowedHosts matcher with "*.example.com"
// wildcard support). Extracted from browser.go during the Day-211 god-file
// split. Public API unchanged.
package browser


import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	stdhttp "net/http"
	"net/url"
	"strings"
	"unicode/utf8"

	"github.com/agezt/agezt/kernel/agent"
)
func (t *Tool) Invoke(ctx context.Context, raw json.RawMessage) (agent.Result, error) {
	var in browserInput
	if err := json.Unmarshal(raw, &in); err != nil {
		return agent.Result{}, fmt.Errorf("browser: parse input: %w", err)
	}
	if strings.TrimSpace(in.URL) == "" {
		return agent.Result{}, errors.New("browser: url required")
	}

	u, err := url.Parse(in.URL)
	if err != nil {
		return agent.Result{}, fmt.Errorf("browser: parse url: %w", err)
	}
	if u.Scheme != "http" && u.Scheme != "https" {
		return agent.Result{}, fmt.Errorf("browser: scheme %q not allowed (only http/https)", u.Scheme)
	}
	if !t.AllowAll && !hostAllowed(u.Host, t.AllowedHosts) {
		return agent.Result{}, fmt.Errorf("%w: %s", ErrHostDenied, u.Host)
	}

	req, err := stdhttp.NewRequestWithContext(ctx, "GET", in.URL, nil)
	if err != nil {
		return agent.Result{}, fmt.Errorf("browser: build request: %w", err)
	}
	req.Header.Set("User-Agent", t.UserAgent)
	// Accept text/html primarily; some sites send JSON when they
	// see only application/* and that's harder to extract from.
	req.Header.Set("Accept", "text/html,application/xhtml+xml,*/*;q=0.8")

	client := t.client()
	// Per-Invoke cookie jar attach. Mutating the shared client's
	// Jar would be a data race across concurrent Invokes; instead
	// we make a shallow copy of the client when a jar is configured
	// and the client doesn't already carry one. The copy reuses
	// the same Transport so connection pooling carries over.
	if t.Cookies != nil && client.Jar == nil {
		shim := *client
		shim.Jar = t.Cookies
		client = &shim
	}
	resp, err := client.Do(req)
	if err != nil {
		return agent.Result{}, fmt.Errorf("browser: fetch: %w", err)
	}
	defer resp.Body.Close()

	// Cap raw download. Read up to MaxFetchBytes+1 to detect overflow,
	// then truncate cleanly.
	rawBody, err := io.ReadAll(io.LimitReader(resp.Body, MaxFetchBytes+1))
	if err != nil {
		return agent.Result{}, fmt.Errorf("browser: read body: %w", err)
	}
	truncatedRaw := false
	if len(rawBody) > MaxFetchBytes {
		rawBody = rawBody[:MaxFetchBytes]
		truncatedRaw = true
	}

	if resp.StatusCode/100 != 2 {
		// Non-2xx: surface the status as a tool error rather than
		// returning the error body as content; the agent should
		// react to the failure, not quote the error page.
		return agent.Result{}, fmt.Errorf("browser: HTTP %d from %s", resp.StatusCode, in.URL)
	}

	text := HTMLToText(string(rawBody))

	maxChars := t.MaxChars
	if maxChars <= 0 {
		maxChars = DefaultMaxChars
	}
	if in.MaxChars > 0 && in.MaxChars < maxChars {
		maxChars = in.MaxChars
	}
	truncatedText := false
	if len(text) > maxChars {
		// Back the cut up to a UTF-8 rune boundary so a multi-byte rune straddling
		// maxChars — common in non-English page text — is dropped whole rather than
		// split into invalid UTF-8 sent to the model.
		cut := maxChars
		for cut > 0 && !utf8.RuneStart(text[cut]) {
			cut--
		}
		text = text[:cut]
		truncatedText = true
	}

	// Bundle metadata + text into a single JSON object — the http
	// tool does the same so the model has a consistent shape across
	// network tools (status, content-type, body all in one place).
	if truncatedText {
		text += "\n\n…[truncated]"
	}
	out := map[string]any{
		"url":          in.URL,
		"status":       resp.StatusCode,
		"content_type": resp.Header.Get("Content-Type"),
		"raw_bytes":    len(rawBody),
		"text_chars":   len(text),
		"text":         text,
	}
	if truncatedRaw {
		out["truncated_raw"] = true
	}
	if truncatedText {
		out["truncated_text"] = true
	}
	enc, err := json.MarshalIndent(out, "", "  ")
	if err != nil {
		return agent.Result{}, fmt.Errorf("browser: marshal result: %w", err)
	}
	return agent.Result{
		Output:            string(enc),
		ObservationTrust:  agent.ObservationUntrusted,
		ObservationSource: in.URL,
	}, nil
}

// hostAllowed reports whether host (with optional :port) matches
// any entry in allowed. Each entry may be a bare hostname or a
// "*.example.com" one-level wildcard. Case-insensitive. Duplicated
// from plugins/tools/http because keeping each tool self-contained
// is cheaper than building shared allowlist infrastructure for
// what's effectively the same eight-line check.
func hostAllowed(host string, allowed []string) bool {
	if len(allowed) == 0 {
		return false
	}
	// Drop port from host for matching.
	if i := strings.IndexByte(host, ':'); i >= 0 {
		host = host[:i]
	}
	host = strings.ToLower(host)
	for _, a := range allowed {
		a = strings.ToLower(strings.TrimSpace(a))
		if a == "" {
			continue
		}
		if strings.HasPrefix(a, "*.") {
			suffix := a[1:] // ".example.com"
			if strings.HasSuffix(host, suffix) && strings.Count(host, ".") == strings.Count(a, ".") {
				return true
			}
		} else if a == host {
			return true
		}
	}
	return false
}
