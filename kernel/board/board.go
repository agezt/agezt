// SPDX-License-Identifier: MIT

// Package board is the in-process shared message board: a persistent,
// topic-addressed space every agent — the lead, its sub-agents, scheduled and
// standing-order agents, and the continuous loops — can post to and read from,
// so they can coordinate and talk to each other (M647).
//
// It is the common ground that complements memory (shared durable FACTS) and
// world (shared ENTITIES): the board carries shared MESSAGES — "I found X",
// "needs follow-up", a note an agent leaves for its next cycle or for a peer.
// One store, shared across every run on the daemon; survives restarts.
//
// Like kernel/cadence and kernel/standing, this is the pure store; the agent-
// facing `board` tool (plugins/tools/boardtool) wraps it, and the control plane
// reads it to surface the conversation in the Web UI. The store is a plain
// JSON file so a reader can Open it fresh and see the latest committed writes
// (writes are atomic: tmp + rename).
package board

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"sync"
)

// MaxMessages bounds the on-disk board so it can't grow without limit; the
// oldest are dropped first.
const MaxMessages = 1000

// Message is one board post.
type Message struct {
	Topic string `json:"topic"`
	From  string `json:"from,omitempty"`
	Text  string `json:"text"`
	TSMS  int64  `json:"ts_ms"`
}

// Store is the persistent, concurrency-safe message board. Many agents (and
// their goroutines) post concurrently, so every access is mutex-guarded.
type Store struct {
	mu   sync.Mutex
	path string
	msgs []Message
}

// Open loads (or creates) the board under dir/board.json.
func Open(dir string) (*Store, error) {
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return nil, fmt.Errorf("board: mkdir: %w", err)
	}
	s := &Store{path: filepath.Join(dir, "board.json")}
	b, err := os.ReadFile(s.path)
	if err == nil {
		_ = json.Unmarshal(b, &s.msgs)
	} else if !os.IsNotExist(err) {
		return nil, fmt.Errorf("board: read: %w", err)
	}
	return s, nil
}

// Post appends a message and persists, dropping the oldest past MaxMessages.
func (s *Store) Post(topic, from, text string, nowMS int64) (Message, error) {
	m := Message{Topic: strings.TrimSpace(topic), From: strings.TrimSpace(from), Text: text, TSMS: nowMS}
	s.mu.Lock()
	defer s.mu.Unlock()
	s.msgs = append(s.msgs, m)
	if len(s.msgs) > MaxMessages {
		s.msgs = s.msgs[len(s.msgs)-MaxMessages:]
	}
	return m, s.save()
}

// Read returns up to limit messages, newest first, optionally filtered to a
// topic (case-insensitive exact match).
func (s *Store) Read(topic string, limit int) []Message {
	s.mu.Lock()
	defer s.mu.Unlock()
	topic = strings.ToLower(strings.TrimSpace(topic))
	out := make([]Message, 0, len(s.msgs))
	for _, m := range s.msgs {
		if topic == "" || strings.ToLower(m.Topic) == topic {
			out = append(out, m)
		}
	}
	sort.SliceStable(out, func(i, j int) bool { return out[i].TSMS > out[j].TSMS })
	if limit > 0 && len(out) > limit {
		out = out[:limit]
	}
	return out
}

// Topics returns the distinct topics on the board with their message counts.
func (s *Store) Topics() map[string]int {
	s.mu.Lock()
	defer s.mu.Unlock()
	counts := map[string]int{}
	for _, m := range s.msgs {
		counts[m.Topic]++
	}
	return counts
}

func (s *Store) save() error {
	b, err := json.MarshalIndent(s.msgs, "", "  ")
	if err != nil {
		return err
	}
	tmp := s.path + ".tmp"
	if err := os.WriteFile(tmp, b, 0o644); err != nil {
		return err
	}
	return os.Rename(tmp, s.path)
}
