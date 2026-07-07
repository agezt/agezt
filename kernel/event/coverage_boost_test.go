// SPDX-License-Identifier: MIT

package event

import (
	"errors"
	"strings"
	"testing"
	"time"
)

// TestDecodeInvalidJSON covers Decode's unmarshal-error branch.
func TestDecodeInvalidJSON(t *testing.T) {
	if _, err := Decode([]byte("{not json")); err == nil {
		t.Fatalf("Decode(invalid) err = nil, want error")
	}
	// Valid round-trip: encode then decode.
	e := mustNew(t, Spec{Subject: "x", Kind: KindHalt, Actor: "kernel"}, GenesisHash)
	canon, err := e.Canonical()
	if err != nil {
		t.Fatalf("Canonical: %v", err)
	}
	// Canonical drops Hash; append a full JSON line via Decode of the full event.
	if _, err := Decode(canon); err != nil {
		t.Fatalf("Decode(valid) err = %v", err)
	}
}

// TestVerifyHashMismatchAndBadPrev covers VerifyHash's mismatch branch and
// computeHash's non-hex PrevHash branch.
func TestVerifyHashMismatchAndBadPrev(t *testing.T) {
	e := mustNew(t, Spec{Subject: "x", Kind: KindHalt, Actor: "kernel"}, GenesisHash)

	// A correctly-hashed event verifies.
	if err := e.VerifyHash(); err != nil {
		t.Fatalf("VerifyHash(valid) err = %v", err)
	}

	// Tamper the hash -> mismatch.
	tampered := *e
	tampered.Hash = strings.Repeat("0", HashHexLen)
	if err := tampered.VerifyHash(); err == nil || !errors.Is(err, ErrHashMismatch) {
		t.Fatalf("VerifyHash(tampered) err = %v, want ErrHashMismatch", err)
	}

	// Non-hex PrevHash makes computeHash (via VerifyHash) fail.
	badPrev := *e
	badPrev.PrevHash = strings.Repeat("Z", HashHexLen)
	if err := badPrev.VerifyHash(); err == nil || !strings.Contains(err.Error(), "not hex") {
		t.Fatalf("VerifyHash(bad prev) err = %v, want not-hex error", err)
	}
}

// TestNewEphemeralValidation exercises NewEphemeral's success path and its
// validation error branches (subject/kind/actor required).
func TestNewEphemeralValidation(t *testing.T) {
	ok, err := NewEphemeral(Spec{Subject: "s", Kind: KindHalt, Actor: "kernel"})
	if err != nil {
		t.Fatalf("NewEphemeral(valid) err = %v", err)
	}
	if !ok.IsEphemeral() {
		t.Fatalf("NewEphemeral result IsEphemeral = false, want true")
	}
	if ok.Seq != 0 || ok.PrevHash != "" || ok.Hash != "" {
		t.Fatalf("NewEphemeral has non-zero chain fields: %+v", ok)
	}

	for _, c := range []struct {
		name string
		spec Spec
	}{
		{"no subject", Spec{Kind: KindHalt, Actor: "kernel"}},
		{"no kind", Spec{Subject: "s", Actor: "kernel"}},
		{"no actor", Spec{Subject: "s", Kind: KindHalt}},
	} {
		t.Run(c.name, func(t *testing.T) {
			if _, err := NewEphemeral(c.spec); err == nil {
				t.Fatalf("NewEphemeral(%s) err = nil, want error", c.name)
			}
		})
	}
}

// TestNewPayloadMarshalError covers the payload-marshal error branch in New:
// a value that cannot be JSON-encoded (a channel) must surface an error.
func TestNewPayloadMarshalError(t *testing.T) {
	_, err := New(
		Spec{Subject: "s", Kind: KindHalt, Actor: "kernel", Payload: make(chan int)},
		fixedID, 1, time.Now(), GenesisHash,
	)
	if err == nil || !strings.Contains(err.Error(), "payload") {
		t.Fatalf("New(unmarshalable payload) err = %v, want payload error", err)
	}
}
