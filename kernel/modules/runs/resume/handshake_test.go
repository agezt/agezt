// SPDX-License-Identifier: MIT

package resume

// The resume HANDSHAKE's test — the module's own vocabulary.
//
// The owned marker and the seeded conversation used to live in the daemon
// (kernel/runtime/resume.go), which made the run engine ask the host for a
// copy of the module's words. They are the module's now, and the test that
// pins them moved with them.

import (
	"context"
	"testing"

	"github.com/agezt/agezt/kernel/contract/llm"
)

// TestTheHandshakeCarriesTheOwnedMarkerAndTheSeed pins the handshake: an
// outer frame's ownership and the seeded conversation travel in the context,
// and a bare context carries neither.
func TestTheHandshakeCarriesTheOwnedMarkerAndTheSeed(t *testing.T) {
	owned := WithOwned(context.Background(), KindRun)
	kind, ok := OwnedKindFromCtx(owned)
	if !ok || kind != KindRun {
		t.Fatalf("owned kind = %q/%v, want %q", kind, ok, KindRun)
	}
	if _, ok := OwnedKindFromCtx(context.Background()); ok {
		t.Fatal("a bare context owns no ticket")
	}

	seeded := WithSeed(context.Background(), []llm.Message{{Role: llm.RoleUser, Content: "hi"}}, 3)
	msgs, iter, ok := SeedFromCtx(seeded)
	if !ok || iter != 3 || len(msgs) != 1 || msgs[0].Content != "hi" {
		t.Fatalf("seed = %+v/%v/%v, want the seeded conversation", msgs, iter, ok)
	}
	if _, _, ok := SeedFromCtx(context.Background()); ok {
		t.Fatal("a bare context carries no seed")
	}
	// An EMPTY conversation is not a seed: the run starts fresh rather than
	// resuming with nothing to say.
	if _, _, ok := SeedFromCtx(WithSeed(context.Background(), nil, 3)); ok {
		t.Fatal("an empty seed must not resume")
	}
}
