// SPDX-License-Identifier: MIT

// Package boardtool is the in-process shared message board: a persistent,
// topic-addressed space every agent — the lead, its sub-agents, scheduled and
// standing-order agents, and the continuous loops — can post to and read from,
// so they can coordinate and talk to each other (M647).
//
// It is the common ground that complements memory (shared durable FACTS) and
// world (shared ENTITIES): the board carries shared MESSAGES — "I found X",
// "needs follow-up", a note an agent leaves for its next cycle or for a peer.
// One store, shared across every run on the daemon; survives restarts.
package boardtool

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"sync"
	"time"
)

// maxMessages bounds the on-disk board so it can't grow without limit; the
// oldest are dropped first.
const maxMessages = 1000

// DefaultReadLimit / MaxReadLimit bound op=read.
const (
	DefaultReadLimit = 20
	MaxReadLimit     = 100
)

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

// Post appends a message and persists, dropping the oldest past maxMessages.
func (s *Store) Post(topic, from, text string, nowMS int64) (Message, error) {
	m := Message{Topic: strings.TrimSpace(topic), From: strings.TrimSpace(from), Text: text, TSMS: nowMS}
	s.mu.Lock()
	defer s.mu.Unlock()
	s.msgs = append(s.msgs, m)
	if len(s.msgs) > maxMessages {
		s.msgs = s.msgs[len(s.msgs)-maxMessages:]
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
	if len(out) > limit {
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

// --- tool ---

// board is the store subset the tool needs (an interface for tests).
type board interface {
	Post(topic, from, text string, nowMS int64) (Message, error)
	Read(topic string, limit int) []Message
	Topics() map[string]int
}

// Tool implements agent.Tool. Created unbound via New(); Bind opens the store.
type Tool struct {
	mu    sync.RWMutex
	store board
	now   func() int64
}

// New returns an unbound board tool.
func New() *Tool { return &Tool{now: func() int64 { return time.Now().UnixMilli() }} }

// Bind opens the shared board store under dir and wires it. Called once after
// the daemon knows its base dir. Returns an error only if the store can't open.
func (t *Tool) Bind(dir string) error {
	st, err := Open(dir)
	if err != nil {
		return err
	}
	t.mu.Lock()
	t.store = st
	t.mu.Unlock()
	return nil
}

// bindStore wires a pre-built store (used by tests).
func (t *Tool) bindStore(b board) {
	t.mu.Lock()
	t.store = b
	t.mu.Unlock()
}

func (t *Tool) current() (board, func() int64) {
	t.mu.RLock()
	defer t.mu.RUnlock()
	now := t.now
	if now == nil {
		now = func() int64 { return time.Now().UnixMilli() }
	}
	return t.store, now
}
