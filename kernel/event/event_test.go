// SPDX-License-Identifier: MIT

package event

import (
	"bytes"
	"encoding/json"
	"errors"
	"strings"
	"testing"
	"time"
)

const fixedID = "01HQRSTUVWXYZ0123456789ABCD"

func mustNew(t *testing.T, spec Spec, prev string) *Event {
	t.Helper()
	ts := time.UnixMilli(1_700_000_000_000)
	e, err := New(spec, fixedID, 1, ts, prev)
	if err != nil {
		t.Fatalf("New: %v", err)
	}
	return e
}

func TestNew_RequiredFields(t *testing.T) {
	cases := []struct {
		name string
		spec Spec
		want string
	}{
		{"no subject", Spec{Kind: KindHalt, Actor: "kernel"}, "subject required"},
		{"no kind", Spec{Subject: "x", Actor: "kernel"}, "kind required"},
		{"no actor", Spec{Subject: "x", Kind: KindHalt}, "actor required"},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			_, err := New(c.spec, fixedID, 0, time.Now(), GenesisHash)
			if err == nil || !strings.Contains(err.Error(), c.want) {
				t.Errorf("got err=%v, want substring %q", err, c.want)
			}
		})
	}
}

func TestNew_BadPrevHash(t *testing.T) {
	_, err := New(Spec{Subject: "x", Kind: KindHalt, Actor: "kernel"}, fixedID, 0, time.Now(), "too-short")
	if err == nil || !strings.Contains(err.Error(), "prev_hash") {
		t.Errorf("got err=%v, want prev_hash error", err)
	}

	_, err = New(Spec{Subject: "x", Kind: KindHalt, Actor: "kernel"}, fixedID, 0, time.Now(), strings.Repeat("Z", HashHexLen))
	if err == nil || !strings.Contains(err.Error(), "not hex") {
		t.Errorf("got err=%v, want not-hex error", err)
	}
}

func TestCanonical_Deterministic(t *testing.T) {
	// Same Spec → same canonical bytes → same hash. Critical for replay.
	spec := Spec{
		Subject: "agent.01H.tool",
		Kind:    KindToolInvoked,
		Actor:   "agent-01H",
		Payload: map[string]any{"b": 2, "a": 1, "c": "three"},
		Tags:    map[string]string{"z": "9", "a": "1"},
	}
	a := mustNew(t, spec, GenesisHash)
	b := mustNew(t, spec, GenesisHash)
	ca, err := a.Canonical()
	if err != nil {
		t.Fatalf("a.Canonical: %v", err)
	}
	cb, err := b.Canonical()
	if err != nil {
		t.Fatalf("b.Canonical: %v", err)
	}
	if !bytes.Equal(ca, cb) {
		t.Fatalf("canonical bytes differ:\n a=%s\n b=%s", ca, cb)
	}
	if a.Hash != b.Hash {
		t.Errorf("hash mismatch for identical spec: a=%s b=%s", a.Hash, b.Hash)
	}
}

func TestCanonical_ExcludesHash(t *testing.T) {
	e := mustNew(t, Spec{Subject: "x", Kind: KindHalt, Actor: "kernel"}, GenesisHash)
	if e.Hash == "" {
		t.Fatal("hash should be populated by New")
	}
	c, err := e.Canonical()
	if err != nil {
		t.Fatal(err)
	}
	if bytes.Contains(c, []byte(e.Hash)) {
		t.Errorf("canonical bytes contain the hash itself, breaking the chain invariant:\n%s", c)
	}
	if !bytes.Contains(c, []byte(`"prev_hash":"`+GenesisHash+`"`)) {
		t.Errorf("canonical bytes missing prev_hash:\n%s", c)
	}
}

func TestHash_Chains(t *testing.T) {
	// Two events: second.PrevHash = first.Hash → second.Hash differs from
	// a "same spec but genesis prev_hash" event. Proves chaining.
	first := mustNew(t, Spec{Subject: "x", Kind: KindHalt, Actor: "kernel"}, GenesisHash)
	second := mustNew(t, Spec{Subject: "x", Kind: KindHalt, Actor: "kernel"}, first.Hash)
	parallel := mustNew(t, Spec{Subject: "x", Kind: KindHalt, Actor: "kernel"}, GenesisHash)

	if second.Hash == parallel.Hash {
		t.Errorf("chained event has same hash as genesis-prev event; chain ignored?")
	}
	if first.Hash == second.Hash {
		t.Errorf("two events with same content but different prev_hash hashed identically")
	}
}

func TestVerifyHash(t *testing.T) {
	e := mustNew(t, Spec{Subject: "x", Kind: KindHalt, Actor: "kernel"}, GenesisHash)
	if err := e.VerifyHash(); err != nil {
		t.Fatalf("VerifyHash fresh: %v", err)
	}
	// Tamper.
	e.Actor = "attacker"
	err := e.VerifyHash()
	if err == nil {
		t.Fatal("VerifyHash should fail on tampered event")
	}
	if !errors.Is(err, ErrHashMismatch) {
		t.Errorf("got %v, want ErrHashMismatch wrapped", err)
	}
}

func TestDecode_Roundtrip(t *testing.T) {
	orig := mustNew(t, Spec{
		Subject: "task.intent",
		Kind:    KindTaskReceived,
		Actor:   "kernel",
		Payload: map[string]string{"intent": "list files"},
		Tags:    map[string]string{"src": "cli"},
	}, GenesisHash)

	encoded, err := json.Marshal(orig)
	if err != nil {
		t.Fatal(err)
	}
	got, err := Decode(encoded)
	if err != nil {
		t.Fatalf("Decode: %v", err)
	}
	if got.Hash != orig.Hash {
		t.Errorf("hash drift: got %s want %s", got.Hash, orig.Hash)
	}
	if err := got.VerifyHash(); err != nil {
		t.Errorf("roundtripped event fails VerifyHash: %v", err)
	}
}

func TestKinds_Known(t *testing.T) {
	for _, k := range []Kind{
		KindAgentSpawned, KindAgentDied,
		KindTaskReceived, KindTaskCompleted,
		KindToolInvoked, KindToolResult,
		KindLLMRequest, KindLLMResponse,
		KindHalt, KindResume,
	} {
		if !IsKnown(k) {
			t.Errorf("Kind %q should be in knownKinds map", k)
		}
	}
	if IsKnown(Kind("nonexistent.kind")) {
		t.Error("unknown kind reported as known")
	}
}

func TestNewEphemeral_HasNoChainFields(t *testing.T) {
	e, err := NewEphemeral(Spec{
		Subject: "agent.01H.llm",
		Kind:    KindLLMToken,
		Actor:   "agent-01H",
		Payload: map[string]any{"text": "pong"},
	})
	if err != nil {
		t.Fatalf("NewEphemeral: %v", err)
	}
	if e.Seq != 0 {
		t.Errorf("Seq=%d, want 0 (ephemeral signal)", e.Seq)
	}
	if e.Hash != "" {
		t.Errorf("Hash=%q, want empty (ephemeral signal)", e.Hash)
	}
	if e.PrevHash != "" {
		t.Errorf("PrevHash=%q, want empty (ephemeral signal)", e.PrevHash)
	}
	if !e.IsEphemeral() {
		t.Error("IsEphemeral=false on freshly-NewEphemeral'd event")
	}
	if e.Kind != KindLLMToken || e.Subject != "agent.01H.llm" || e.Actor != "agent-01H" {
		t.Errorf("metadata wrong: %+v", e)
	}
	if e.TSUnixMS == 0 {
		t.Error("TSUnixMS not set")
	}
	if len(e.Payload) == 0 {
		t.Error("payload empty")
	}
}

func TestNewEphemeral_ValidatesRequiredFields(t *testing.T) {
	cases := []struct {
		name string
		spec Spec
		want string
	}{
		{"no subject", Spec{Kind: KindLLMToken, Actor: "a"}, "subject"},
		{"no kind", Spec{Subject: "s", Actor: "a"}, "kind"},
		{"no actor", Spec{Subject: "s", Kind: KindLLMToken}, "actor"},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			_, err := NewEphemeral(c.spec)
			if err == nil {
				t.Fatal("want error")
			}
			if !strings.Contains(err.Error(), c.want) {
				t.Errorf("err=%v, want substring %q", err, c.want)
			}
		})
	}
}

func TestIsEphemeral_DurableIsFalse(t *testing.T) {
	// Durable events (built via New) must NOT report IsEphemeral.
	// Otherwise the bus's PublishStreaming/Publish split would
	// silently misclassify chain-linked events as display-only.
	e, err := New(Spec{Subject: "s", Kind: KindTaskReceived, Actor: "a"},
		fixedID, 1, time.UnixMilli(1), GenesisHash)
	if err != nil {
		t.Fatal(err)
	}
	if e.IsEphemeral() {
		t.Error("durable event reported IsEphemeral=true")
	}
}
