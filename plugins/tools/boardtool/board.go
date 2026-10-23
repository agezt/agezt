// SPDX-License-Identifier: MIT

package boardtool

// Package documentation lives in doc.go.

import (
	"sync"
	"time"

	"github.com/agezt/agezt/kernel/contract/boardapi"
)

// DefaultReadLimit / MaxReadLimit bound op=read.
const (
	DefaultReadLimit = 20
	MaxReadLimit     = 100
)

// boardStore is the board surface the tool needs — the contract's port, so
// tests can inject a fake without a real on-disk store, and so the plugin
// never names the board module (it is a plugin: it may reach the contract and
// the platform layers and nothing else).
type boardStore = boardapi.Store

// PostNotifier is called after a successful post/send so the host can journal a
// board.posted event (M656/M788) — the primitive that lets one agent's message
// wake another (a standing order triggered on "board.<topic>", or on
// "board.dm.<slug>" for an addressed message). Kept as a plain closure so this
// plugin stays free of the kernel bus/event packages and is trivially testable.
// corr is the posting run's correlation id (may be empty).
type PostNotifier func(m boardapi.Message, corr string)

// Tool implements agent.Tool. Created unbound via New(); BindStore wires the store.
type Tool struct {
	mu     sync.RWMutex
	store  boardStore
	now    func() int64
	notify PostNotifier
}

// New returns an unbound board tool.
func New() *Tool { return &Tool{now: func() int64 { return time.Now().UnixMilli() }} }

// BindStore wires the shared board store (M937): the daemon opens ONE
// kernel/board store and hands the same instance to this tool, the control
// plane, and the REST mailbox — separate instances would silently clobber each
// other's last write (each holds the full message list in memory and saves it
// whole).
//
// The store arrives as the contract's PORT: the tool is a plugin, so it may
// not name the board module — and it may not open the store itself either.
// The daemon opens it and hands it over.
func (t *Tool) BindStore(st boardStore) { t.bindStore(st) }

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
