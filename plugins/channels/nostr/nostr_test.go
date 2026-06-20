// SPDX-License-Identifier: MIT

package nostr

import (
	"context"
	"encoding/hex"
	"encoding/json"
	"testing"

	btcec "github.com/btcsuite/btcd/btcec/v2"
	"github.com/btcsuite/btcd/btcec/v2/schnorr"

	"github.com/agezt/agezt/kernel/channel"
)

// newSigned builds and signs a kind-1 event from priv that p-tags target.
func newSigned(t *testing.T, priv *btcec.PrivateKey, target, content string) nostrEvent {
	t.Helper()
	ev := nostrEvent{
		Pubkey:    hex.EncodeToString(schnorr.SerializePubKey(priv.PubKey())),
		CreatedAt: 1700000000,
		Kind:      1,
		Tags:      [][]string{{"p", target}},
		Content:   content,
	}
	if err := ev.sign(priv); err != nil {
		t.Fatalf("sign: %v", err)
	}
	return ev
}

func TestNewRejectsBadKey(t *testing.T) {
	if _, err := New(Config{PrivKeyHex: "nsec1abc"}); err == nil {
		t.Fatal("nsec should be rejected")
	}
	if _, err := New(Config{PrivKeyHex: "xyz"}); err == nil {
		t.Fatal("non-hex should be rejected")
	}
	if _, err := New(Config{PrivKeyHex: hex.EncodeToString(make([]byte, 31))}); err == nil {
		t.Fatal("wrong length should be rejected")
	}
}

func TestSignVerifyRoundTrip(t *testing.T) {
	priv, _ := btcec.NewPrivateKey()
	ev := newSigned(t, priv, "deadbeef", "hello nostr")
	if !ev.verify() {
		t.Fatal("freshly signed event must verify")
	}
	// id must equal sha256(serialize).
	if len(ev.ID) != 64 || len(ev.Sig) != 128 {
		t.Fatalf("id/sig length: id=%d sig=%d", len(ev.ID), len(ev.Sig))
	}
	// Tamper the content → id no longer matches → verify false.
	bad := ev
	bad.Content = "tampered"
	if bad.verify() {
		t.Fatal("tampered content must fail verification")
	}
	// Tamper the signature → verify false.
	bad2 := ev
	bad2.Sig = "00" + ev.Sig[2:]
	if bad2.verify() {
		t.Fatal("tampered signature must fail verification")
	}
}

func TestHandleFrameVerifiesBeforeDispatch(t *testing.T) {
	self, _ := btcec.NewPrivateKey()
	selfHex := hex.EncodeToString(schnorr.SerializePubKey(self.PubKey()))
	sender, _ := btcec.NewPrivateKey()
	senderHex := hex.EncodeToString(schnorr.SerializePubKey(sender.PubKey()))

	var got string
	c, err := New(Config{
		PrivKeyHex: hex.EncodeToString(self.Serialize()),
		Allowlist:  channel.NewAllowlist([]string{senderHex}),
		Handler: func(_ context.Context, msg channel.UnifiedMessage, _ string) (channel.Reply, error) {
			got = msg.Text
			return channel.Reply{}, nil // empty reply → no publish needed
		},
	})
	if err != nil {
		t.Fatal(err)
	}

	ev := newSigned(t, sender, selfHex, "ping the agent")
	frame, _ := json.Marshal([]any{"EVENT", "sub", ev})
	c.handleFrame(context.Background(), frame)
	if got != "ping the agent" {
		t.Fatalf("allowlisted signed event should reach handler, got %q", got)
	}

	// A forged event (valid structure, broken signature) must be dropped before dispatch.
	got = ""
	forged := ev
	forged.Sig = "00" + ev.Sig[2:]
	frame2, _ := json.Marshal([]any{"EVENT", "sub", forged})
	c.handleFrame(context.Background(), frame2)
	if got != "" {
		t.Fatalf("forged event must not reach handler, got %q", got)
	}
}

func TestDispatchAllowlistGate(t *testing.T) {
	self, _ := btcec.NewPrivateKey()
	sender, _ := btcec.NewPrivateKey()
	senderHex := hex.EncodeToString(schnorr.SerializePubKey(sender.PubKey()))

	called := false
	c, _ := New(Config{
		PrivKeyHex: hex.EncodeToString(self.Serialize()),
		Allowlist:  channel.NewAllowlist([]string{"someone-else"}),
		Handler: func(_ context.Context, _ channel.UnifiedMessage, _ string) (channel.Reply, error) {
			called = true
			return channel.Reply{}, nil
		},
	})
	c.dispatch(context.Background(), nostrEvent{ID: "id1", Pubkey: senderHex, Content: "hi"})
	if called {
		t.Fatal("non-allowlisted author must not reach the handler")
	}
}

func TestBroadcastNoRelays(t *testing.T) {
	self, _ := btcec.NewPrivateKey()
	c, _ := New(Config{PrivKeyHex: hex.EncodeToString(self.Serialize())})
	if err := c.Send(context.Background(), channel.Outbound{Text: "x"}); err == nil {
		t.Fatal("publish with no connected relay should error")
	}
}
