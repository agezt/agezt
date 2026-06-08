// SPDX-License-Identifier: MIT

// Package boardtool is the agent-facing wrapper over kernel/board: the shared,
// persistent, topic-addressed message board every agent — the lead, its
// sub-agents, scheduled and standing-order agents, and the continuous loops —
// can post to and read from, so they can coordinate and talk to each other
// (M647).
//
// The store itself lives in kernel/board (so the control plane can read it to
// surface the conversation in the Web UI without importing a plugin); this
// package is just the Tool that lets an agent use it, mirroring how the
// schedule/standing tools wrap kernel/cadence and kernel/standing.
package boardtool

import (
	"sync"
	"time"

	"github.com/agezt/agezt/kernel/board"
)

// DefaultReadLimit / MaxReadLimit bound op=read.
const (
	DefaultReadLimit = 20
	MaxReadLimit     = 100
)

// boardStore is the kernel/board subset the tool needs — an interface so tests
// can inject a fake without a real on-disk store.
type boardStore interface {
	Post(topic, from, text string, nowMS int64) (board.Message, error)
	Read(topic string, limit int) []board.Message
	Topics() map[string]int
}

// Tool implements agent.Tool. Created unbound via New(); Bind opens the store.
type Tool struct {
	mu    sync.RWMutex
	store boardStore
	now   func() int64
}

// New returns an unbound board tool.
func New() *Tool { return &Tool{now: func() int64 { return time.Now().UnixMilli() }} }

// Bind opens the shared board store under dir and wires it. Called once after
// the daemon knows its base dir. Returns an error only if the store can't open.
func (t *Tool) Bind(dir string) error {
	st, err := board.Open(dir)
	if err != nil {
		return err
	}
	t.mu.Lock()
	t.store = st
	t.mu.Unlock()
	return nil
}

// bindStore wires a pre-built store (used by tests).
func (t *Tool) bindStore(b boardStore) {
	t.mu.Lock()
	t.store = b
	t.mu.Unlock()
}

func (t *Tool) current() (boardStore, func() int64) {
	t.mu.RLock()
	defer t.mu.RUnlock()
	now := t.now
	if now == nil {
		now = func() int64 { return time.Now().UnixMilli() }
	}
	return t.store, now
}
