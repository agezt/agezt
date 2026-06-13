// SPDX-License-Identifier: MIT

package update

import (
	"context"
	"crypto/ed25519"
	"crypto/rand"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"
)

// withKey installs a fresh Ed25519 keypair for the duration of the test and
// returns the private key (to sign with) and the hex public key. It clears the
// trusted key on cleanup so package globals stay pristine across tests.
func withKey(t *testing.T) (priv ed25519.PrivateKey, pubHex string) {
	t.Helper()
	pub, priv, err := ed25519.GenerateKey(rand.Reader)
	if err != nil {
		t.Fatalf("GenerateKey: %v", err)
	}
	pubHex = hex.EncodeToString(pub)
	if err := SetPublicKey(pubHex); err != nil {
		t.Fatalf("SetPublicKey: %v", err)
	}
	// Mask any build-time default for the test and restore state on exit.
	prev := DefaultPublicKeyHex
	DefaultPublicKeyHex = ""
	t.Cleanup(func() {
		_ = SetPublicKey("")
		DefaultPublicKeyHex = prev
	})
	return priv, pubHex
}

// signHex signs "<version>\n<sumHex>" with priv and returns hex.
func signHex(t *testing.T, priv ed25519.PrivateKey, version, sumHex string) string {
	t.Helper()
	return hex.EncodeToString(ed25519.Sign(priv, signedMessage(version, sumHex)))
}

const fakeSum = "0000000000000000000000000000000000000000000000000000000000000001"

// TestVerifySignature_NoKeyConfigured — with no trusted key, verification is a
// no-op (SHA256-only mode). The backward-compatible default must never reject
// an unsigned release on its own.
func TestVerifySignature_NoKeyConfigured(t *testing.T) {
	_ = SetPublicKey("")
	DefaultPublicKeyHex = ""
	svc := New(Config{})
	info := &UpdateInfo{Version: "1.2.3", SHA256: fakeSum} // no Signature
	if err := svc.verifySignature(info); err != nil {
		t.Errorf("verifySignature with no key: got %v, want nil", err)
	}
}

// TestVerifySignature_Valid — a correctly signed release verifies.
func TestVerifySignature_Valid(t *testing.T) {
	priv, _ := withKey(t)
	svc := New(Config{})
	info := &UpdateInfo{
		Version:   "1.2.3",
		SHA256:    fakeSum,
		Signature: signHex(t, priv, "1.2.3", fakeSum),
	}
	if err := svc.verifySignature(info); err != nil {
		t.Errorf("verifySignature(valid): got %v, want nil", err)
	}
}

// TestVerifySignature_Missing — a configured key rejects an unsigned release.
func TestVerifySignature_Missing(t *testing.T) {
	withKey(t)
	svc := New(Config{})
	info := &UpdateInfo{Version: "1.2.3", SHA256: fakeSum}
	if err := svc.verifySignature(info); !errors.Is(err, ErrSignatureMissing) {
		t.Errorf("verifySignature(missing sig): got %v, want ErrSignatureMissing", err)
	}
}

// TestVerifySignature_TamperedHash — a signature over hash A does not validate
// hash B (the hash is bound to the key, not self-supplied).
func TestVerifySignature_TamperedHash(t *testing.T) {
	priv, _ := withKey(t)
	svc := New(Config{})
	const other = "0000000000000000000000000000000000000000000000000000000000000002"
	info := &UpdateInfo{
		Version:   "1.2.3",
		SHA256:    other, // differs from what was signed
		Signature: signHex(t, priv, "1.2.3", fakeSum),
	}
	err := svc.verifySignature(info)
	var invalid *ErrSignatureInvalid
	if !errors.As(err, &invalid) {
		t.Errorf("verifySignature(tampered hash): got %v, want *ErrSignatureInvalid", err)
	}
}

// TestVerifySignature_WrongKey — a signature from a different key is rejected.
func TestVerifySignature_WrongKey(t *testing.T) {
	priv, _ := withKey(t)
	otherPub, _, _ := ed25519.GenerateKey(rand.Reader)
	if err := SetPublicKey(hex.EncodeToString(otherPub)); err != nil {
		t.Fatal(err)
	}
	svc := New(Config{})
	info := &UpdateInfo{
		Version:   "1.2.3",
		SHA256:    fakeSum,
		Signature: signHex(t, priv, "1.2.3", fakeSum), // signed by the first key
	}
	if err := svc.verifySignature(info); err == nil {
		t.Error("verifySignature(wrong key): got nil, want *ErrSignatureInvalid")
	}
}

// TestVerifySignature_BadHex — a non-hex signature is rejected cleanly.
func TestVerifySignature_BadHex(t *testing.T) {
	withKey(t)
	svc := New(Config{})
	info := &UpdateInfo{Version: "1.2.3", SHA256: fakeSum, Signature: "not-hex!!"}
	if err := svc.verifySignature(info); err == nil {
		t.Error("verifySignature(bad hex): got nil, want *ErrSignatureInvalid")
	}
}

// TestSetPublicKey_Validation guards key parsing: only a 32-byte Ed25519 key
// in hex is accepted; junk and wrong lengths are rejected; "" clears.
func TestSetPublicKey_Validation(t *testing.T) {
	if err := SetPublicKey("zz"); err == nil {
		t.Error(`SetPublicKey("zz"): got nil, want error`)
	}
	if err := SetPublicKey("abcd"); err == nil { // decodes but wrong length
		t.Error(`SetPublicKey("abcd"): got nil, want error (wrong length)`)
	}
	pub, _, _ := ed25519.GenerateKey(rand.Reader)
	if err := SetPublicKey(hex.EncodeToString(pub)); err != nil {
		t.Errorf("SetPublicKey(valid): %v", err)
	}
	if err := SetPublicKey(""); err != nil {
		t.Errorf(`SetPublicKey(""): %v`, err)
	}
}

// noDrain is a drainFunc that reports immediate completion. Apply skips the
// drain phase entirely when Config.DrainTimeout is zero (these Configs leave it
// zero), so this is never actually invoked — it only satisfies the signature.
var noDrain = func(context.Context, time.Duration) DrainResult { return DrainResult{} }

// TestApply_SignatureGate proves the signature gate is wired into the real
// Apply path: a validly-signed release applies, but a release whose signature
// was made over a different hash is refused at Apply time (and staging removed).
func TestApply_SignatureGate(t *testing.T) {
	priv, _ := withKey(t)

	bin := []byte("this is the real agezt binary payload")
	sumArr := sha256.Sum256(bin)
	sum := hex.EncodeToString(sumArr[:])

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_, _ = w.Write(bin)
	}))
	defer srv.Close()

	t.Run("valid signature applies", func(t *testing.T) {
		svc := New(Config{Source: SourceEndpoint, BaseDir: t.TempDir()})
		info := &UpdateInfo{
			Version: "9.9.9", SHA256: sum, URL: srv.URL,
			Signature: signHex(t, priv, "9.9.9", sum),
		}
		if err := svc.Apply(context.Background(), info, noDrain); err != nil {
			t.Fatalf("Apply(valid sig): %v", err)
		}
	})

	t.Run("tampered signature refused", func(t *testing.T) {
		svc := New(Config{Source: SourceEndpoint, BaseDir: t.TempDir()})
		// Signature over fakeSum, but the binary's real hash is `sum`.
		bad := &UpdateInfo{
			Version: "9.9.9", SHA256: sum, URL: srv.URL,
			Signature: signHex(t, priv, "9.9.9", fakeSum),
		}
		err := svc.Apply(context.Background(), bad, noDrain)
		if err == nil {
			t.Fatal("Apply(tampered sig): got nil, want signature error")
		}
	})

	t.Run("missing signature refused", func(t *testing.T) {
		svc := New(Config{Source: SourceEndpoint, BaseDir: t.TempDir()})
		info := &UpdateInfo{Version: "9.9.9", SHA256: sum, URL: srv.URL} // no Signature
		err := svc.Apply(context.Background(), info, noDrain)
		if err == nil {
			t.Fatal("Apply(missing sig): got nil, want signature error")
		}
	})
}
