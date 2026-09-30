// SPDX-License-Identifier: MIT

// Package browser is the in-process web-reader tool. It fetches a URL
// over HTTP/HTTPS, strips scripts/styles, decodes HTML entities, and
// returns the visible text content to the model — turning "raw HTML
// the model has to parse" into "readable prose the model can quote."
//
// **Scope (M1.x).** `browser.read` is stdlib-only. No headless-browser binary,
// no JavaScript execution. This is a pragmatic "user agent" tool:
//
//   - Server-rendered pages (news articles, docs sites, GitHub,
//     blogs, Wikipedia) read cleanly.
//   - Single-page apps that defer rendering to client-side
//     JavaScript come back as a near-empty shell with a `<noscript>`
//     hint. The agent sees that and knows to fall back.
//
// The sibling `browser.action` tool is the opt-in Playwright bridge for
// JS-rendered pages and user-like actions. Keeping it separate preserves
// `browser.read` as the cheap, SSRF-guarded default fetch path while letting
// operators explicitly enable browser automation when they have Node/Playwright
// installed.
//
// **Tool name.** Exposed as `browser.read` (not just `browser`) so
// the namespace stays open for verbs like `browser.action`.
package browser
