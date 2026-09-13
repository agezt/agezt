// SPDX-License-Identifier: MIT

package main

// MCP content flatteners + response writer + tiny helpers:
// flattenContent + flattenResourceContents + writeAgezt + getenvDefault.
// Carved out of main.go during the Day 189 god-file split so the
// main file can stay focused on the lifecycle and the handlers file
// can stay focused on the request handlers.
// Public API unchanged.

import (
	"bufio"
	"encoding/json"
	"fmt"
	"os"
	"strings"
)

func flattenContent(items []mcpContentItem) string {
	if len(items) == 0 {
		return ""
	}
	var sb strings.Builder
	for i, c := range items {
		if i > 0 {
			sb.WriteByte('\n')
		}
		switch c.Type {
		case "text":
			sb.WriteString(c.Text)
		case "":
			// Defensive: some servers omit type for text blocks.
			sb.WriteString(c.Text)
		default:
			fmt.Fprintf(&sb, "[mcp:%s content omitted]", c.Type)
		}
	}
	return sb.String()
}

// flattenResourceContents collapses MCP resource-read output into
// a single string (M1.ww). Mirrors flattenContent's contract — the
// agent loop consumes the Output field as plain text — but the
// shape of the input is different: each block carries URI +
// mimeType + (text or blob), so we annotate per-block to preserve
// provenance.
func flattenResourceContents(items []mcpResourceContent) string {
	if len(items) == 0 {
		return ""
	}
	var sb strings.Builder
	for i, c := range items {
		if i > 0 {
			sb.WriteString("\n\n")
		}
		if c.URI != "" {
			fmt.Fprintf(&sb, "--- %s", c.URI)
			if c.MimeType != "" {
				fmt.Fprintf(&sb, " (%s)", c.MimeType)
			}
			sb.WriteByte('\n')
		}
		switch {
		case c.Text != "":
			sb.WriteString(c.Text)
		case c.Blob != "":
			fmt.Fprintf(&sb, "[mcp:blob omitted, %d base64 chars]", len(c.Blob))
		}
	}
	return sb.String()
}

func writeAgezt(w *bufio.Writer, r ageztResponse) {
	raw, err := json.Marshal(r)
	if err != nil {
		// Should be unreachable: every field is plain JSON. If it
		// happens, drop the response — the host will time out the
		// pending request rather than the bridge crashing.
		fmt.Fprintln(os.Stderr, "mcpbridge: encode response:", err)
		return
	}
	_, _ = w.Write(raw)
	_ = w.WriteByte('\n')
	// Flush per response — the host reads line-by-line and a buffered
	// response would deadlock both sides until the buffer filled.
	_ = w.Flush()
}

func getenvDefault(name, fallback string) string {
	if v := strings.TrimSpace(os.Getenv(name)); v != "" {
		return v
	}
	return fallback
}

