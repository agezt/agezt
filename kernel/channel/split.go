// SPDX-License-Identifier: MIT

package channel

import "unicode/utf16"

// SplitText breaks text into pieces each at most limit UTF-16 code units long,
// so an over-long message can be delivered as a sequence of platform-accepted
// chunks instead of being rejected wholesale. Chat platforms cap message
// length — Telegram at 4096 UTF-16 code units, Discord at 2000 characters, Slack
// far higher — and a single oversize send is dropped by the API, losing the
// agent's answer entirely.
//
// The unit is UTF-16 code units because that is what Telegram counts; it is also
// a safe bound for platforms that count runes or Unicode code points (Discord,
// Slack), since a rune is never more than its UTF-16 length. Pass each platform's
// own limit.
//
// Splitting prefers to break just after the last newline or space that fits, so
// words and lines stay intact; a run with no such boundary (one very long
// "word") is hard-cut at the limit. No characters are added or dropped:
// concatenating the result always reproduces text exactly. A limit <= 0, or text
// already within the limit, returns the text as a single piece.
func SplitText(text string, limit int) []string {
	if limit <= 0 || utf16Len(text) <= limit {
		return []string{text}
	}
	var out []string
	cur := make([]rune, 0, limit)
	units := 0
	breakAfter := -1 // rune index in cur just past the last newline/space, or -1

	for _, r := range text {
		ru := runeUnits(r)
		if units+ru > limit && len(cur) > 0 {
			cut := len(cur)
			var carry []rune
			if breakAfter > 0 && breakAfter < len(cur) {
				cut = breakAfter
				carry = append(carry, cur[breakAfter:]...)
			}
			out = append(out, string(cur[:cut]))
			cur = append(cur[:0], carry...)
			units = utf16RunesLen(cur)
			breakAfter = -1
		}
		cur = append(cur, r)
		units += ru
		if r == '\n' || r == ' ' {
			breakAfter = len(cur)
		}
	}
	if len(cur) > 0 {
		out = append(out, string(cur))
	}
	return out
}

func runeUnits(r rune) int {
	if u := utf16.RuneLen(r); u > 0 {
		return u
	}
	return 1 // invalid rune → encoded as the replacement char (1 unit)
}

func utf16Len(s string) int {
	n := 0
	for _, r := range s {
		n += runeUnits(r)
	}
	return n
}

func utf16RunesLen(rs []rune) int {
	n := 0
	for _, r := range rs {
		n += runeUnits(r)
	}
	return n
}
