// SPDX-License-Identifier: MIT

package controlplane

// Prompt library (M713): the owner's saved, named chat prompts — reusable
// workflows ("draft my standup", "review the diff", …) defined once and launched
// from the Chat view. Stored daemon-side (a small JSON file under the base dir) so
// they're the same from any browser or access point, not stuck in one localStorage.
// Purely a UI convenience: the agent loop never reads these; they only seed the
// composer.

import (
	"encoding/json"
	"net"
	"os"
	"path/filepath"
	"strings"
)

const (
	promptsFile    = "chat_prompts.json"
	maxPrompts     = 100
	maxPromptTitle = 120
	maxPromptText  = 8000
)

type promptItem struct {
	Title string `json:"title"`
	Text  string `json:"text"`
}

func (s *Server) promptsPath() string { return filepath.Join(s.baseDir, promptsFile) }

// handlePromptsGet returns the saved prompt library (empty list if none yet).
func (s *Server) handlePromptsGet(conn net.Conn, req Request) {
	items := s.loadPrompts()
	out := make([]map[string]any, 0, len(items))
	for _, p := range items {
		out = append(out, map[string]any{"title": p.Title, "text": p.Text})
	}
	s.writeResp(conn, Response{ID: req.ID, Type: RespResult, Result: map[string]any{"prompts": out}})
}

// handlePromptsSet replaces the whole prompt library. args.prompts is an array of
// {title, text}; blank entries are dropped, fields trimmed and length-capped.
func (s *Server) handlePromptsSet(conn net.Conn, req Request) {
	raw, ok := req.Args["prompts"]
	if !ok {
		s.writeResp(conn, Response{ID: req.ID, Type: RespError, Error: "args.prompts required (array of {title, text})"})
		return
	}
	arr, ok := raw.([]any)
	if !ok {
		s.writeResp(conn, Response{ID: req.ID, Type: RespError, Error: "args.prompts must be an array"})
		return
	}
	items := make([]promptItem, 0, len(arr))
	for _, e := range arr {
		m, ok := e.(map[string]any)
		if !ok {
			continue
		}
		title, _ := m["title"].(string)
		text, _ := m["text"].(string)
		title = strings.TrimSpace(title)
		text = strings.TrimSpace(text)
		if title == "" || text == "" {
			continue // a prompt needs both a label and a body
		}
		if len(title) > maxPromptTitle {
			title = title[:maxPromptTitle]
		}
		if len(text) > maxPromptText {
			text = text[:maxPromptText]
		}
		items = append(items, promptItem{Title: title, Text: text})
		if len(items) >= maxPrompts {
			break
		}
	}

	data, err := json.MarshalIndent(items, "", "  ")
	if err != nil {
		s.writeResp(conn, Response{ID: req.ID, Type: RespError, Error: "encode prompts: " + err.Error()})
		return
	}
	if err := os.WriteFile(s.promptsPath(), data, 0600); err != nil {
		s.writeResp(conn, Response{ID: req.ID, Type: RespError, Error: "save prompts: " + err.Error()})
		return
	}
	s.writeResp(conn, Response{ID: req.ID, Type: RespResult, Result: map[string]any{"saved": true, "count": len(items)}})
}

// loadPrompts reads the prompt library file, returning an empty slice if it's
// missing or unreadable (a corrupt file shouldn't break the Chat view).
func (s *Server) loadPrompts() []promptItem {
	data, err := os.ReadFile(s.promptsPath())
	if err != nil {
		return nil
	}
	var items []promptItem
	if json.Unmarshal(data, &items) != nil {
		return nil
	}
	return items
}
