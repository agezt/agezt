// SPDX-License-Identifier: MIT

package browser

// Stdlib-only HTML → plain-text extraction.
//
// We can't use `golang.org/x/net/html` (excluded by the lean-deps
// policy). The implementation below is a small state machine over
// the raw bytes that:
//
//  1. Strips entire `<script>`, `<style>`, and `<noscript>` blocks
//     (content AND tags) — these are noise for an agent reader.
//  2. Strips HTML comments (`<!-- ... -->`).
//  3. Replaces block-level tag boundaries (`</p>`, `<br>`, `</li>`,
//     `</h1>` etc.) with newlines so paragraph structure survives.
//  4. Strips all remaining tags.
//  5. Decodes the common named entities + numeric entities.
//  6. Collapses runs of whitespace to a single space, runs of
//     newlines to a maximum of two (preserves paragraph breaks).
//
// **Why not a full HTML parser.** Real parsers handle malformed
// HTML, character encoding declarations, foreign content
// (SVG/MathML), template elements, etc. We don't need any of that
// to surface "the text content of this article." A small state
// machine that gets the common case right beats a fragile half-
// implementation of a real parser.
//
// **Trade-offs (what we lose).**
//
//   - Tables read as a wall of text. Cell boundaries aren't
//     reconstructed; column structure is lost.
//   - Links read as plain text without URLs. The agent can't
//     follow them without a separate `browser.read` call.
//   - Image alt-text is dropped (we strip the whole tag).
//
// These are real limitations but rarely block the "read the article
// to answer a question" use case. A future v2 could surface anchor
// hrefs as inline `[text](url)` and tabularise tables.

import (
	"html"
	"regexp"
	"strings"
)

// Block-level tag pattern. Closing or self-closing variants trigger
// a paragraph break in the output. Lowercased; matching is
// case-insensitive (some HTML in the wild uses `<P>`).
var blockTags = map[string]struct{}{
	"p": {}, "br": {}, "div": {}, "section": {}, "article": {},
	"li": {}, "ul": {}, "ol": {}, "tr": {},
	"h1": {}, "h2": {}, "h3": {}, "h4": {}, "h5": {}, "h6": {},
	"blockquote": {}, "pre": {}, "hr": {},
	"header": {}, "footer": {}, "main": {}, "nav": {}, "aside": {},
	"figure": {}, "figcaption": {}, "dt": {}, "dd": {},
}

// stripBlockREs matches complete `<X>...</X>` blocks for noise tags.
// Go's regexp (RE2) doesn't support backreferences, so we run one
// pattern per tag instead of `</\1>`. (?s)=dot-matches-newline,
// (?i)=case-insensitive, non-greedy .*? so one open tag doesn't
// swallow through every closing tag on the page.
var stripBlockREs = []*regexp.Regexp{
	regexp.MustCompile(`(?is)<script\b[^>]*>.*?</script\s*>`),
	regexp.MustCompile(`(?is)<style\b[^>]*>.*?</style\s*>`),
	regexp.MustCompile(`(?is)<noscript\b[^>]*>.*?</noscript\s*>`),
}

// stripUnclosedREs handle the rarer case where a page has an
// unclosed `<script>` / `<style>` / `<noscript>` (truncated
// download, malformed HTML). Without these we'd leak the source
// into the text. One per tag for the same RE2 reason.
var stripUnclosedREs = []*regexp.Regexp{
	regexp.MustCompile(`(?is)<script\b[^>]*>.*`),
	regexp.MustCompile(`(?is)<style\b[^>]*>.*`),
	regexp.MustCompile(`(?is)<noscript\b[^>]*>.*`),
}

// commentRE matches `<!-- ... -->` including multi-line comments.
var commentRE = regexp.MustCompile(`(?s)<!--.*?-->`)

// tagRE matches any HTML tag (opening, closing, self-closing). We
// extract the tag name to decide whether it's block-level. Doctypes
// and processing instructions are caught by the broader prefix
// match below.
var tagRE = regexp.MustCompile(`</?([a-zA-Z][a-zA-Z0-9]*)\b[^>]*>`)

// multiSpaceRE collapses runs of horizontal whitespace.
var multiSpaceRE = regexp.MustCompile(`[ \t]+`)

// multiNewlineRE collapses runs of newlines to a maximum of two
// (so paragraph breaks survive, but not 12-blank-line gaps).
var multiNewlineRE = regexp.MustCompile(`\n{3,}`)

// HTMLToText converts an HTML string to a readable plain-text form.
// Exported so callers (and future tests at the browser_test level)
// can exercise it directly without going through the network path.
func HTMLToText(htmlStr string) string {
	s := htmlStr

	// 1. Drop script/style/noscript blocks (well-formed).
	for _, re := range stripBlockREs {
		s = re.ReplaceAllString(s, "")
	}
	// 2. Drop any unclosed remainder (truncated download safety net).
	for _, re := range stripUnclosedREs {
		s = re.ReplaceAllString(s, "")
	}
	// 3. Drop comments.
	s = commentRE.ReplaceAllString(s, "")
	// 4. Drop doctype and XML processing instructions (don't capture
	//    them in the block strip — they're not script/style).
	s = regexp.MustCompile(`(?is)<!doctype[^>]*>`).ReplaceAllString(s, "")
	s = regexp.MustCompile(`(?is)<\?[^>]*\?>`).ReplaceAllString(s, "")

	// 5. Replace tags. Block-level tags become newlines so paragraph
	//    structure survives; everything else becomes a space.
	s = tagRE.ReplaceAllStringFunc(s, func(tag string) string {
		// Extract the tag name (already captured by tagRE; re-find to get).
		m := tagRE.FindStringSubmatch(tag)
		if len(m) < 2 {
			return " "
		}
		name := strings.ToLower(m[1])
		if _, isBlock := blockTags[name]; isBlock {
			return "\n"
		}
		return " "
	})

	// 6. Decode HTML entities (stdlib handles named + numeric).
	s = html.UnescapeString(s)

	// 7. Whitespace normalisation. Order matters: collapse horizontal
	//    first (preserves the \n boundaries we just emitted), then
	//    trim trailing spaces per line, then collapse vertical.
	s = multiSpaceRE.ReplaceAllString(s, " ")
	// Strip leading/trailing space on each line. Re-tokenising on \n
	// is cheaper than another regex pass.
	lines := strings.Split(s, "\n")
	for i, line := range lines {
		lines[i] = strings.TrimSpace(line)
	}
	s = strings.Join(lines, "\n")
	s = multiNewlineRE.ReplaceAllString(s, "\n\n")

	return strings.TrimSpace(s)
}
