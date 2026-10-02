// SPDX-License-Identifier: MIT

// Package websearch is the in-process web-search tool. It runs a keyword
// query against a public search engine (DuckDuckGo's no-JS HTML endpoint)
// and returns the top results as structured {title, url, snippet} records —
// the capability that lets the agent *discover* a URL, not just fetch one it
// was already given (M627).
//
// Design notes:
//   - Keyless: DuckDuckGo's html endpoint needs no API key, so the tool works
//     out of the box with no operator secret.
//   - SSRF-guarded: the request goes through a netguard-protected client that
//     refuses internal/metadata addresses, exactly like the http/browser tools.
//   - Fail-soft: a network error or an unparseable page returns an empty result
//     set with a note, never a hard error — a flaky search must not fail a run.
//
// The engine host is fixed (the operator cannot point it at an arbitrary
// host), so the only operator-controlled input is the query string; that is
// why its capability (edict.CapWebSearch) is a low-risk network read.
package websearch
