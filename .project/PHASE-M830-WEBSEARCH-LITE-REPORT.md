# Phase M830 — fix web_search (DuckDuckGo endpoint moved/blocked)

**Date:** 2026-06-11 · **Status:** DONE · **Trigger:** owner: "google search
duck duck go vs ne lazımsa çalışmıyor" — web_search returned nothing.

## Root cause

The tool scraped `https://html.duckduckgo.com/html/`. That endpoint now answers
bot GETs with **HTTP 202 + an anti-bot challenge page** (no results) — confirmed
live (14 KB page, zero `result__a` anchors). So every search came back empty.

## Fix (`plugins/tools/websearch/websearch.go`)

- Switch the engine to the **lite** endpoint `https://lite.duckduckgo.com/lite/`,
  which still serves a plain, parseable results page for a GET (verified live:
  HTTP 200, 10 results).
- Update the result regexes to the lite markup: the anchor carries `href` BEFORE a
  (single- or double-quoted) `class='result-link'`, and the snippet is a
  `<td class='result-snippet'>`. `cleanURL` (uddg redirect unwrap) + fail-soft
  behaviour are unchanged.

## Verification

- Unit: the parser test fixture updated to the lite markup; tool tests green.
- **Live** (real deepseek agent run): `web_search "playwright end to end testing"`
  → real results — playwright.dev, developer.microsoft.com, medium.com — with
  titles. Previously: zero results.

## Gate

websearch tests green; vet + staticcheck + linux clean; gofmt clean. Engine host
still fixed (no operator-controlled target); SSRF guard unchanged.
