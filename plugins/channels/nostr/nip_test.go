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

func TestBech32NpubRoundsToHex(t *testing.T) {
	// Known NIP-19 test vector (from the NIP-19 spec).
	const npub = "npub180cvv07tjdrrgpa0j7j7tmnyl2yr6yr7l8j4s3evf6u64th6gkwsyjh6w6"
	const wantHex = "3bf0c63fcb93463407af97a5e5ee64fa883d107ef9e558472c4eb9aaaefa459d"
	got, err := DecodePubkey(npub)
	if err != nil {
		t.Fatalf("decode npub: %v", err)
	}
	if got != wantHex {
		t.Fatalf("npub decoded to %s, want %s", got, wantHex)
	}
	// Plain hex passes through unchanged.
	if h, _ := DecodePubkey(wantHex); h != wantHex {
		t.Fatalf("hex passthrough = %s", h)
	}
}

func TestNewAcceptsNsec(t *testing.T) {
	priv, _ := btcec.NewPrivateKey()
	hexKey := hex.EncodeToString(priv.Serialize())
	nsec, err := EncodeNsec(hexKey)
	if err != nil {
		t.Fatal(err)
	}
	// New must accept the nsec and derive the same pubkey as the hex form.
	cHex, err := New(Config{PrivKeyHex: hexKey})
	if err != nil {
		t.Fatal(err)
	}
	cNsec, err := New(Config{PrivKeyHex: nsec})
	if err != nil {
		t.Fatalf("New with nsec: %v", err)
	}
	if cNsec.PubHex() != cHex.PubHex() || len(cNsec.PubHex()) != 64 {
		t.Fatalf("nsec pubkey %s != hex pubkey %s", cNsec.PubHex(), cHex.PubHex())
	}
	// npub of that pubkey round-trips back to the hex pubkey.
	npub, _ := EncodeNpub(cHex.PubHex())
	if got, _ := DecodePubkey(npub); got != cHex.PubHex() {
		t.Fatalf("npub round-trip = %s", got)
	}
}

func TestNIP04RoundTrip(t *testing.T) {
	alice, _ := btcec.NewPrivateKey()
	bob, _ := btcec.NewPrivateKey()
	// Alice encrypts to Bob; Bob decrypts using Alice's pubkey.
	ct, err := nip04Encrypt(alice, bob.PubKey(), "secret hello 🤫")
	if err != nil {
		t.Fatal(err)
	}
	pt, err := nip04Decrypt(bob, alice.PubKey(), ct)
	if err != nil {
		t.Fatal(err)
	}
	if pt != "secret hello 🤫" {
		t.Fatalf("decrypted = %q", pt)
	}
	// A wrong key must not decrypt to the plaintext.
	mallory, _ := btcec.NewPrivateKey()
	if got, err := nip04Decrypt(mallory, alice.PubKey(), ct); err == nil && got == "secret hello 🤫" {
		t.Fatal("wrong key should not recover the plaintext")
	}
}

// TestDispatchDMDecryptsAndRepliesEncrypted exercises the full kind-4 inbound
// path: an allowlisted sender DMs the agent; the handler sees plaintext.
func TestDispatchDMDecryptsAndReplies(t *testing.T) {
	self, _ := btcec.NewPrivateKey()
	selfHex := hex.EncodeToString(schnorr.SerializePubKey(self.PubKey()))
	sender, _ := btcec.NewPrivateKey()
	senderHex := hex.EncodeToString(schnorr.SerializePubKey(sender.PubKey()))

	var seen string
	c, err := New(Config{
		PrivKeyHex: hex.EncodeToString(self.Serialize()),
		Allowlist:  channel.NewAllowlist([]string{senderHex}),
		Handler: func(_ context.Context, msg channel.UnifiedMessage, _ string) (channel.Reply, error) {
			seen = msg.Text
			return channel.Reply{}, nil // empty → no publish (no relay in test)
		},
	})
	if err != nil {
		t.Fatal(err)
	}

	// Sender builds an encrypted kind-4 DM to the agent.
	enc, err := nip04Encrypt(sender, c.priv.PubKey(), "private question")
	if err != nil {
		t.Fatal(err)
	}
	ev := nostrEvent{
		Pubkey:    senderHex,
		CreatedAt: 1700000000,
		Kind:      4,
		Tags:      [][]string{{"p", selfHex}},
		Content:   enc,
	}
	if err := ev.sign(sender); err != nil {
		t.Fatal(err)
	}
	frame, _ := json.Marshal([]any{"EVENT", "sub", ev})
	c.handleFrame(context.Background(), frame)
	if seen != "private question" {
		t.Fatalf("handler should see decrypted DM, got %q", seen)
	}
}
