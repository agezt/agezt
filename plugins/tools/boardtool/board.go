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
	Send(m board.Message, nowMS int64) (board.Message, error)
	Broadcast(from, text string, nowMS int64) (board.Message, error)
	HelpRequest(from, to, text string, nowMS int64) (board.Message, error)
	OpenHelp(limit int) []board.Message
	Read(topic string, limit int) []board.Message
	Inbox(to string, limit int, includeAnswered bool) []board.Message
	Replies(id string, limit int) []board.Message
	Get(id string) (board.Message, bool)
	Topics() map[string]int
	Ack(id, by string) (board.Message, bool, error)
}

// PostNotifier is called after a successful post/send so the host can journal a
// board.posted event (M656/M788) — the primitive that lets one agent's message
// wake another (a standing order triggered on "board.<topic>", or on
// "board.dm.<slug>" for an addressed message). Kept as a plain closure so this
// plugin stays free of the kernel bus/event packages and is trivially testable.
// corr is the posting run's correlation id (may be empty).
type PostNotifier func(m board.Message, corr string)

// Tool implements agent.Tool. Created unbound via New(); Bind opens the store.
type Tool struct {
	mu     sync.RWMutex
	store  boardStore
	now    func() int64
	notify PostNotifier
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

// BindStore wires a pre-built store (M937): the daemon opens ONE kernel/board
// store and hands the same instance to this tool, the control plane, and the
// REST mailbox — separate instances would silently clobber each other's last
// write (each holds the full message list in memory and saves it whole).
func (t *Tool) BindStore(st *board.Store) { t.bindStore(st) }

// bindStore wires a pre-built store (used by tests).
func (t *Tool) bindStore(b boardStore) {
	t.mu.Lock()
	t.store = b
	t.mu.Unlock()
}

// OnPost registers a notifier invoked after each successful post (M656). The
// daemon wires this to journal a board.posted event so standing orders can react
// to board messages. Safe to leave unset (posts simply aren't journaled then).
func (t *Tool) OnPost(fn PostNotifier) {
	t.mu.Lock()
	t.notify = fn
	t.mu.Unlock()
}

func (t *Tool) current() (boardStore, func() int64, PostNotifier) {
	t.mu.RLock()
	defer t.mu.RUnlock()
	now := t.now
	if now == nil {
		now = func() int64 { return time.Now().UnixMilli() }
	}
	return t.store, now, t.notify
}
