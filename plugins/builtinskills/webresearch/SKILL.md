---
name: web-research
description: Research a question across the open web — gather from several sources, pull the readable text out of pages, keep track of every URL as a citation, and synthesize a sourced answer instead of trusting one page — when a task needs current facts, comparisons, or anything you should verify against real sources
triggers: [research, web, search, source, cite, fact, compare, look up, gather, article, news, documentation]
tools: [fetch, code_exec, shell, browser]
---

# web-research — gather, extract, cite, synthesize

When a task needs facts you should verify — current events, library docs, a
product comparison, "is X still true" — don't answer from memory and don't trust
a single page. Gather from **several** sources, pull out the real text, keep
every URL, and synthesize an answer that says where each claim came from.

## The discipline

1. **Frame the question** into 2–4 concrete sub-questions before searching.
2. **Diversify sources** — at least 2–3 independent pages per claim; prefer
   primary/official sources (docs, the project itself, the standards body) over
   aggregators.
3. **Extract, don't skim** — pull the readable text so you quote accurately.
4. **Cite as you go** — keep a list of `{claim → url}`. Every non-obvious fact in
   the final answer must trace to a URL.
5. **Note disagreement** — if sources conflict, say so and prefer the more
   authoritative/recent one rather than silently picking one.

## Fetching pages

- A single known page → the **`fetch`** tool (it returns readable content directly).
- A batch of URLs, or when you want title + clean main text as JSON →
  `scripts/extract.py` (below).
- A **JavaScript-heavy or login-gated** page that returns little text → switch to
  the **browser-use** skill (real browser, sees rendered content). Don't fight a
  SPA with raw fetch.

## One-time setup

Run `scripts/setup.sh` once (via code_exec or shell): installs `requests` and
`beautifulsoup4` (and `trafilatura` if available, for better main-text
extraction). Use `skill op=files web-research` to find the bundle directory.

## Extract with the helper

`scripts/extract.py` fetches one or more URLs and prints, per URL, the title and
the main readable text as JSON — so you can quote and cite precisely:

```
python scripts/extract.py '{"urls":["https://example.com/article"],"max_chars":4000}'
```

### Spec fields
- `urls` (required) — one URL string or a list of them.
- `max_chars` (optional, default 6000) — truncate each page's text.
- `timeout` (optional, default 20) — per-request seconds.

### Output (JSON on stdout)
```
{ "ok": true, "results": [ { "url": "...", "status": 200, "title": "...",
  "text": "...", "chars": 1234 } ], "errors": [ {"url":"...","error":"..."} ] }
```

## Synthesizing

Write the answer first, sources second. Lead with the conclusion, then back each
claim inline or in a short **Sources** list of the URLs you actually used. If you
couldn't verify something, say so — an honest gap beats a confident guess.

See `reference/recipes.md` for search strategies, a citation format, handling
paywalls/PDFs, and when to escalate to the browser.
