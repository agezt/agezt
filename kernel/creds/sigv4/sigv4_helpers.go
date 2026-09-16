// SPDX-License-Identifier: MIT
//
// kernel/creds/sigv4 canonicalisation + URI encoding helpers (CanonicalQuery,
// canonicalHeaders, collapseSpaces, AWSURIEncode).
// Extracted from sigv4.go during Day 211 god-file refactor (#91).
// Public API unchanged.
package sigv4

import (
	"fmt"
	"net/http"
	"sort"
	"strings"
)

func CanonicalQuery(q map[string][]string) string {
	if len(q) == 0 {
		return ""
	}
	type kv struct{ k, v string }
	var pairs []kv
	for k, vs := range q {
		for _, v := range vs {
			pairs = append(pairs, kv{AWSURIEncode(k, true), AWSURIEncode(v, true)})
		}
	}
	sort.Slice(pairs, func(i, j int) bool {
		if pairs[i].k != pairs[j].k {
			return pairs[i].k < pairs[j].k
		}
		return pairs[i].v < pairs[j].v
	})
	var parts []string
	for _, p := range pairs {
		parts = append(parts, p.k+"="+p.v)
	}
	return strings.Join(parts, "&")
}
func canonicalHeaders(h http.Header) (canonical, signedHeaders string) {
	const (
		hHost         = "host"
		hXAmzDate     = "x-amz-date"
		hXAmzContent  = "x-amz-content-sha256"
		hContentType  = "content-type"
		hXAmzSecurity = "x-amz-security-token"
	)
	include := []string{hHost, hXAmzDate, hXAmzContent}
	if h.Get("Content-Type") != "" {
		include = append(include, hContentType)
	}
	if h.Get("X-Amz-Security-Token") != "" {
		include = append(include, hXAmzSecurity)
	}
	sort.Strings(include)
	var lines []string
	for _, name := range include {
		val := strings.TrimSpace(h.Get(name))
		val = collapseSpaces(val)
		lines = append(lines, name+":"+val+"\n")
	}
	return strings.Join(lines, ""), strings.Join(include, ";")
}
func collapseSpaces(s string) string {
	if !strings.Contains(s, "  ") {
		return s
	}
	var sb strings.Builder
	sb.Grow(len(s))
	prevSpace := false
	for _, r := range s {
		if r == ' ' {
			if !prevSpace {
				sb.WriteRune(r)
			}
			prevSpace = true
		} else {
			sb.WriteRune(r)
			prevSpace = false
		}
	}
	return sb.String()
}
func AWSURIEncode(s string, encodeSlash bool) string {
	var sb strings.Builder
	sb.Grow(len(s))
	for i := 0; i < len(s); i++ {
		c := s[i]
		switch {
		case c >= 'A' && c <= 'Z',
			c >= 'a' && c <= 'z',
			c >= '0' && c <= '9',
			c == '-', c == '_', c == '.', c == '~':
			sb.WriteByte(c)
		case c == '/' && !encodeSlash:
			sb.WriteByte(c)
		default:
			fmt.Fprintf(&sb, "%%%02X", c)
		}
	}
	return sb.String()
}
