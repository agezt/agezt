// SPDX-License-Identifier: MIT

// Nostr event type + serialize + sign + verify.
// Code extracted from nostr.go during the Day-107 god-file split.
// Public API unchanged.
package nostr


import (
	"bytes"

	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	btcec "github.com/btcsuite/btcd/btcec/v2"
	"github.com/btcsuite/btcd/btcec/v2/schnorr"
)

type nostrEvent struct {
	ID        string     `json:"id"`
	Pubkey    string     `json:"pubkey"`
	CreatedAt int64      `json:"created_at"`
	Kind      int        `json:"kind"`
	Tags      [][]string `json:"tags"`
	Content   string     `json:"content"`
	Sig       string     `json:"sig"`
}

// serialize produces the NIP-01 canonical form whose sha256 is the event id:
// [0,pubkey,created_at,kind,tags,content] with no extra whitespace and HTML
// escaping disabled (Nostr uses standard minimal JSON string escapes).
func (e *nostrEvent) serialize() []byte {
	tags := e.Tags
	if tags == nil {
		tags = [][]string{}
	}
	arr := []any{0, e.Pubkey, e.CreatedAt, e.Kind, tags, e.Content}
	var buf bytes.Buffer
	enc := json.NewEncoder(&buf)
	enc.SetEscapeHTML(false)
	_ = enc.Encode(arr)
	return bytes.TrimRight(buf.Bytes(), "\n")
}

// sign computes the id and schnorr signature, filling ID and Sig.
func (e *nostrEvent) sign(priv *btcec.PrivateKey) error {
	sum := sha256.Sum256(e.serialize())
	sig, err := schnorr.Sign(priv, sum[:])
	if err != nil {
		return err
	}
	e.ID = hex.EncodeToString(sum[:])
	e.Sig = hex.EncodeToString(sig.Serialize())
	return nil
}

// verify recomputes the id, checks it matches e.ID, and verifies the schnorr
// signature against the author pubkey. False on any malformed field.
func (e *nostrEvent) verify() bool {
	sum := sha256.Sum256(e.serialize())
	if hex.EncodeToString(sum[:]) != e.ID {
		return false
	}
	pkBytes, err := hex.DecodeString(e.Pubkey)
	if err != nil || len(pkBytes) != 32 {
		return false
	}
	pub, err := schnorr.ParsePubKey(pkBytes)
	if err != nil {
		return false
	}
	sigBytes, err := hex.DecodeString(e.Sig)
	if err != nil {
		return false
	}
	sig, err := schnorr.ParseSignature(sigBytes)
	if err != nil {
		return false
	}
	return sig.Verify(sum[:], pub)
}

// seenBefore records an event id and reports whether it was already processed.
func (c *Channel) seenBefore(id string) bool {
	c.dmu.Lock()
	defer c.dmu.Unlock()
	if _, ok := c.seen[id]; ok {
		return true
	}
	c.seen[id] = struct{}{}
	c.ring = append(c.ring, id)
	if len(c.ring) > dedupCap {
		old := c.ring[0]
		c.ring = c.ring[1:]
		delete(c.seen, old)
	}
	return false
}
