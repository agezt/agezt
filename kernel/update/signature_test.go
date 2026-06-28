// SPDX-License-Identifier: MIT

package update

import (
	"context"
	"crypto/ed25519"
	"crypto/rand"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"
)

// SetPublicKey configures the trusted release-signing key for tests. Pass an
// empty string to clear it.
func SetPublicKey(hexKey string) error {
	pubKeyMu.Lock()
	defer pubKeyMu.Unlock()
	if strings.TrimSpace(hexKey) == "" {
		updatePubKey = nil
		return nil
	}
	raw, err := hex.DecodeString(strings.TrimSpace(hexKey))
	if err != nil {
		return fmt.Errorf("update: bad public key hex: %w", err)
	}
	if len(raw) != ed25519.PublicKeySize {
		return fmt.Errorf("update: public key is %d bytes, want %d", len(raw), ed25519.PublicKeySize)
	}
	updatePubKey = ed25519.PublicKey(raw)
	return nil
}

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

// TestVerifySignature_NoKeyConfigured — the policy depends on the source.
// SourceGitHub tolerates a missing key (its trust anchor is GitHub
// Releases' TLS + asset integrity). SourceEndpoint REFUSES a missing key:
// a self-supplied checksum is not a trust anchor (UPD-001).
func TestVerifySignature_NoKeyConfigured(t *testing.T) {
	_ = SetPublicKey("")
	DefaultPublicKeyHex = ""
	svc := New(Config{})

	t.Run("github source accepts no-key unsigned", func(t *testing.T) {
		info := &UpdateInfo{Version: "1.2.3", SHA256: fakeSum} // no Signature
		if err := svc.verifySignature(info, SourceGitHub); err != nil {
			t.Errorf("verifySignature(GitHub, no key): got %v, want nil", err)
		}
	})

	t.Run("endpoint source refuses no-key unsigned", func(t *testing.T) {
		info := &UpdateInfo{Version: "1.2.3", SHA256: fakeSum} // no Signature
		if err := svc.verifySignature(info, SourceEndpoint); !errors.Is(err, ErrSignatureKeyNotConfigured) {
			t.Errorf("verifySignature(Endpoint, no key): got %v, want ErrSignatureKeyNotConfigured", err)
		}
	})
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
	if err := svc.verifySignature(info, SourceEndpoint); err != nil {
		t.Errorf("verifySignature(valid): got %v, want nil", err)
	}
}

// TestVerifySignature_Missing — a configured key rejects an unsigned release.
func TestVerifySignature_Missing(t *testing.T) {
	withKey(t)
	svc := New(Config{})
	info := &UpdateInfo{Version: "1.2.3", SHA256: fakeSum}
	if err := svc.verifySignature(info, SourceEndpoint); !errors.Is(err, ErrSignatureMissing) {
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
	err := svc.verifySignature(info, SourceEndpoint)
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
	if err := svc.verifySignature(info, SourceEndpoint); err == nil {
		t.Error("verifySignature(wrong key): got nil, want *ErrSignatureInvalid")
	}
}

// TestVerifySignature_BadHex — a non-hex signature is rejected cleanly.
func TestVerifySignature_BadHex(t *testing.T) {
	withKey(t)
	svc := New(Config{})
	info := &UpdateInfo{Version: "1.2.3", SHA256: fakeSum, Signature: "not-hex!!"}
	if err := svc.verifySignature(info, SourceEndpoint); err == nil {
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

// TestApply_EndpointSourceRefusedWithoutKey is the live regression guard
// for VULN self-update-integrity (UPD-001): an endpoint-sourced update
// MUST be refused when no release-signing public key is configured. The
// endpoint supplies its own SHA256, so without a signature there's no
// trust anchor — a compromised endpoint would serve {malicious binary,
// matching sha256} and pass SHA validation.
//
// The companion test TestApply_GitHubSourceAcceptsNoKey covers the
// SourceGitHub path (different policy: GitHub Releases' TLS + asset
// integrity is the trust anchor).
func TestApply_EndpointSourceRefusedWithoutKey(t *testing.T) {
	// Ensure no key is configured for the duration of this test.
	_ = SetPublicKey("")
	prev := DefaultPublicKeyHex
	DefaultPublicKeyHex = ""
	t.Cleanup(func() {
		_ = SetPublicKey("")
		DefaultPublicKeyHex = prev
	})

	bin := []byte("the malicious binary the endpoint would like us to install")
	sumArr := sha256.Sum256(bin)
	sum := hex.EncodeToString(sumArr[:])

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_, _ = w.Write(bin)
	}))
	defer srv.Close()

	svc := New(Config{Source: SourceEndpoint, BaseDir: t.TempDir()})
	info := &UpdateInfo{
		Version: "9.9.9",
		SHA256:  sum, // self-supplied checksum matches the malicious binary
		URL:     srv.URL,
		// Signature deliberately omitted — this is the UPD-001 vector.
	}
	err := svc.Apply(context.Background(), info, noDrain)
	if err == nil {
		t.Fatal("Apply(Endpoint, no key): got nil, want signature-required error")
	}
	if !errors.Is(err, ErrSignatureKeyNotConfigured) {
		t.Errorf("Apply(Endpoint, no key): got %v, want ErrSignatureKeyNotConfigured", err)
	}
}

// TestApply_GitHubSourceAcceptsNoKey — SourceGitHub updates continue to
// apply when no signing key is embedded, because their trust anchor is
// GitHub Releases' TLS + asset integrity, not a self-supplied checksum.
// GitHub Releases don't ship signatures today, so refusing them would
// break every existing GitHub-sourced deployment (UPD-001 must not
// regress a working configuration).
func TestApply_GitHubSourceAcceptsNoKey(t *testing.T) {
	_ = SetPublicKey("")
	prev := DefaultPublicKeyHex
	DefaultPublicKeyHex = ""
	t.Cleanup(func() {
		_ = SetPublicKey("")
		DefaultPublicKeyHex = prev
	})

	bin := []byte("legitimate agezt binary served by GitHub Releases")
	sumArr := sha256.Sum256(bin)
	sum := hex.EncodeToString(sumArr[:])

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_, _ = w.Write(bin)
	}))
	defer srv.Close()

	svc := New(Config{Source: SourceGitHub, BaseDir: t.TempDir()})
	info := &UpdateInfo{Version: "9.9.9", SHA256: sum, URL: srv.URL} // no Signature
	if err := svc.Apply(context.Background(), info, noDrain); err != nil {
		t.Fatalf("Apply(GitHub, no key): %v", err)
	}
}
