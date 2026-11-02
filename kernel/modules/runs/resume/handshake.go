// SPDX-License-Identifier: MIT

package resume

// The resume HANDSHAKE: the two facts a resumed run carries in its context.
//
// The owned marker and the seeded conversation are the ticket protocol's
// VOCABULARY: what a ticket means for the run that holds it. The rules are
// the module's (the ticket's kinds, its dispositions), so the words that
// carry them are the module's too.
//
// They used to live in the daemon (kernel/runtime/resume.go), which made the
// run engine — the reader — ask the host for a copy of the module's own
// vocabulary. The run engine lives in the runs module, right next to this
// package, so it reads the handshake from here now.
//
// The package doc used to say "the resumer (in the daemon, where the context
// builders live) rebuilds a run context from the flattened fields". The
// builders live HERE now; the resumer (cmd/agezt) calls this package.

import (
	"context"

	"github.com/agezt/agezt/kernel/contract/llm"
)

// handshakeCtxKey keeps the handshake's two facts apart from every other
// context value: they are the module's own keys, not the run context's.
type handshakeCtxKey int

const (
	ctxKeyOwned handshakeCtxKey = iota // a resume ticket for this corr is already owned by an outer frame
	ctxKeySeed                         // prior conversation + iter to seed a resumed run
)

// WithSeed carries a suspended run's prior conversation and iteration into a
// resume dispatch, so the run continues the interrupted loop instead of
// starting fresh.
//
// Set by the resumer; read only by the run engine.
func WithSeed(ctx context.Context, messages []llm.Message, iter int) context.Context {
	if len(messages) == 0 {
		return ctx
	}
	return context.WithValue(ctx, ctxKeySeed, seed{messages: messages, iter: iter})
}

// WithOwned marks that a ticket for this corr already exists and is owned by
// an outer frame — a governed wrapper (RunAssured/RunWithRetry) or the
// resumer. The run then neither creates nor deletes the ticket; it only
// refreshes the snapshot for a message-bearing (Kind=run) resume.
func WithOwned(ctx context.Context, kind string) context.Context {
	return context.WithValue(ctx, ctxKeyOwned, kind)
}

// seed is the seeded conversation and the iteration to continue from.
type seed struct {
	messages []llm.Message
	iter     int
}

// SeedFromCtx returns the seeded conversation and the iteration to continue
// from, when the context carries one.
func SeedFromCtx(ctx context.Context) ([]llm.Message, int, bool) {
	s, ok := ctx.Value(ctxKeySeed).(seed)
	if !ok || len(s.messages) == 0 {
		return nil, 0, false
	}
	return s.messages, s.iter, true
}

// OwnedKindFromCtx returns the kind of the ticket an outer frame owns, when
// the context carries one.
func OwnedKindFromCtx(ctx context.Context) (string, bool) {
	kind, ok := ctx.Value(ctxKeyOwned).(string)
	return kind, ok
}
