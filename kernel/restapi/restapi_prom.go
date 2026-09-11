// SPDX-License-Identifier: MIT

// REST API helpers: promName + promHelp + methodNotAllowed + writeJSON + writeErr + tokenText.
// Code extracted from restapi.go during the Day-66 god-file split. Public API unchanged.
package restapi


import (
	"encoding/json"
	"github.com/agezt/agezt/kernel/event"
	"net/http"
	"strings"
)


func promName(s string) string {
	if s == "" {
		return "_"
	}
	b := []byte(s)
	for i := 0; i < len(b); i++ {
		c := b[i]
		switch {
		case c >= 'a' && c <= 'z', c >= 'A' && c <= 'Z', c == '_', c == ':':
			// valid anywhere
		case c >= '0' && c <= '9':
			// valid except as the first character (handled below)
		default:
			b[i] = '_'
		}
	}
	if b[0] >= '0' && b[0] <= '9' {
		return "_" + string(b) // a leading digit isn't allowed; prefix '_'
	}
	return string(b)
}

// promHelp escapes a metric HELP string per the Prometheus exposition format:
// backslash and newline are the two characters that would otherwise break the
// single-line `# HELP <name> <text>` record (a newline ends the line; an
// unescaped backslash is invalid). Today's HELP strings are all single-line, so
// this is robustness against a future multi-line/backslashed description.
func promHelp(s string) string {
	s = strings.ReplaceAll(s, `\`, `\\`)
	s = strings.ReplaceAll(s, "\n", `\n`)
	return s
}

func methodNotAllowed(w http.ResponseWriter, allow string) {
	w.Header().Set("Allow", allow)
	writeErr(w, http.StatusMethodNotAllowed, "method_not_allowed", allow+" required")
}

func writeJSON(w http.ResponseWriter, code int, v any) {
	w.Header().Set("Content-Type", "application/json")
	w.Header().Set("Referrer-Policy", "no-referrer")
	w.WriteHeader(code)
	_ = json.NewEncoder(w).Encode(v)
}

func writeErr(w http.ResponseWriter, code int, typ, msg string) {
	writeJSON(w, code, map[string]any{"error": map[string]any{"type": typ, "message": msg}})
}

// tokenText returns the streamed text delta carried by an llm.token event, or
// "" for any other event.
func tokenText(ev *event.Event) string {
	if ev == nil || ev.Kind != event.KindLLMToken || len(ev.Payload) == 0 {
		return ""
	}
	var p struct {
		Text string `json:"text"`
	}
	if json.Unmarshal(ev.Payload, &p) != nil {
		return ""
	}
	return p.Text
}
