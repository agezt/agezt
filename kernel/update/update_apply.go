// SPDX-License-Identifier: MIT

// Update apply path: Apply (drain + atomic swap) + ErrChecksumMismatch + ErrSignatureInvalid + resolvePublicKey + signedMessage.
// Code extracted from update.go during the Day-55 god-file split. Public API unchanged.
package update


import (
	"context"
	"crypto/ed25519"
	"encoding/hex"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"sync"
	"time"

	"github.com/agezt/agezt/internal/brand"
)


func (s *Service) Apply(ctx context.Context, info *UpdateInfo, drainFunc func(context.Context, time.Duration) DrainResult) error {
	// Guard against concurrent Apply calls.
	// The lockfile also serves as the PID file for the update sentinel.
	lockPath := filepath.Join(s.cfg.BaseDir, "update.lock")
	locked, err := s.acquireLock(lockPath)
	if err != nil {
		return fmt.Errorf("update: %w", err)
	}
	if !locked {
		return ErrUpdateInProgress
	}
	defer os.Remove(lockPath) // best-effort; next Apply will retry

	// 1. Download to staging path.
	binDir := filepath.Join(s.cfg.BaseDir, "bin")
	if err := os.MkdirAll(binDir, 0o755); err != nil {
		return fmt.Errorf("update: mkdir bin dir: %w", err)
	}

	// Detect binary name (agezt.exe on Windows).
	binaryName := brand.Binary
	if runtime.GOOS == "windows" && !strings.HasSuffix(binaryName, ".exe") {
		binaryName += ".exe"
	}
	stagingPath := filepath.Join(binDir, binaryName+".new")
	livePath := filepath.Join(binDir, binaryName)

	// Remove any stale staging file from a previous failed attempt.
	os.Remove(stagingPath)

	if err := s.downloadBinary(ctx, info.URL, stagingPath); err != nil {
		return fmt.Errorf("update: download failed: %w", err)
	}

	// 2. Validate SHA256.
	if err := s.validateSHA256(stagingPath, info.SHA256); err != nil {
		os.Remove(stagingPath)
		return fmt.Errorf("update: validation failed: %w", err)
	}

	// 2b. Verify the release signature (UPD-001 fix) — for the custom
	// endpoint source, signature verification is MANDATORY (a self-supplied
	// checksum is not a trust anchor). verifySignature refuses the update
	// when no key is configured; a configured key then refuses unsigned
	// or invalid signatures. GitHub-source updates rely on GitHub
	// Releases' own TLS + asset integrity, so the verifier still runs
	// for them but tolerates a missing key + signature.
	if err := s.verifySignature(info); err != nil {
		os.Remove(stagingPath)
		return fmt.Errorf("update: signature verification failed: %w", err)
	}

	// 3. Drain (if configured).
	var drainResult DrainResult
	if s.cfg.DrainTimeout > 0 {
		drainResult = drainFunc(ctx, s.cfg.DrainTimeout)
		if drainResult.Timeout {
			// Drain timed out — leave staging file in place for inspection.
			// Do NOT swap. Do NOT restart.
			return ErrDrainTimeout
		}
	}

	// 4. Atomic rename: staging → live.
	// os.Rename is atomic on the same filesystem on both POSIX and Windows.
	if err := os.Rename(stagingPath, livePath); err != nil {
		return fmt.Errorf("update: atomic rename failed (current binary untouched): %w", err)
	}

	// 5. Make the new binary executable (Linux/macOS; no-op on Windows).
	if runtime.GOOS != "windows" {
		if err := os.Chmod(livePath, 0o755); err != nil {
			// Binary is in place but not executable — fail-safe:
			// leave it and do not restart. Human must fix permissions.
			return fmt.Errorf("update: chmod new binary: %w (binary replaced but not executable — manual intervention required)", err)
		}
	}

	return nil
}

// DrainResult describes the outcome of the drain phase.
type DrainResult struct {
	Timeout    bool // true if drain timed out
	ActiveRuns int  // runs still in-flight when drain ended
}

// ErrUpdateInProgress is returned when Apply is called while an update
// is already in progress.
var ErrUpdateInProgress = errors.New("update: another update is already in progress")

// ErrDrainTimeout is returned when in-flight runs do not complete within
// the configured DrainTimeout.
var ErrDrainTimeout = errors.New("update: drain timed out — in-flight runs did not complete")

// ErrChecksumMismatch is returned when the downloaded binary's SHA256
// does not match the manifest.
type ErrChecksumMismatch struct {
	Have string // hex of what was downloaded
	Want string // hex from the manifest
}

func (e *ErrChecksumMismatch) Error() string {
	return fmt.Sprintf("update: SHA256 mismatch (have=%s, want=%s)", e.Have[:8], e.Want[:8])
}

// ErrSignatureMissing is returned when a public key is configured but the
// manifest carries no signature.
var ErrSignatureMissing = errors.New("update: release is not signed but a public key is configured")

// ErrSignatureKeyNotConfigured is returned when Apply is invoked with a
// SourceEndpoint update but no release-signing public key has been embedded
// at build time or set at runtime. Endpoint-sourced updates MUST be
// signature-verified — a checksum supplied by the same endpoint that
// supplied the binary is not a trust anchor (UPD-001). GitHub-sourced
// updates are exempt because their trust anchor is GitHub Releases' TLS
// + asset integrity, not the supplied SHA256 alone.
var ErrSignatureKeyNotConfigured = errors.New("update: no release-signing public key configured; only a manifest fetched from a GitHub release may apply without one — an endpoint-sourced or caller-supplied manifest (e.g. a version/sha256/url posted to /api/v1/update/apply) requires a signature, because nothing else attests its download URL. Embed DefaultPublicKeyHex at build time and sign releases (`go build -ldflags '-X github.com/agezt/agezt/kernel/update.DefaultPublicKeyHex=<hex>'`)")

// ErrSignatureInvalid is returned when the manifest's signature does not verify
// under the configured public key (wrong key, tampered version/hash, or a
// malformed signature).
type ErrSignatureInvalid struct{ Reason string }

func (e *ErrSignatureInvalid) Error() string {
	return "update: release signature is invalid: " + e.Reason
}

// DefaultPublicKeyHex is the Ed25519 public key (lowercase hex, 64 chars) the
// daemon trusts for release signatures. Inject it at build time, e.g.:
//
//	go build -ldflags '-X github.com/agezt/agezt/kernel/update.DefaultPublicKeyHex=<hex>'
//
// Embedded empty (the default) MEANS endpoint-sourced updates will be
// refused with ErrSignatureKeyNotConfigured (a checksum supplied by the
// same endpoint that supplied the binary is not a trust anchor). Set the
// key — at build time via -ldflags or at runtime via SetPublicKey — before
// deploying a daemon that uses SourceEndpoint. GitHub-sourced updates are
// exempt because GitHub Releases' TLS + asset integrity is the trust
// anchor (UPD-001 fix).
var DefaultPublicKeyHex = ""

// Trusted release-signing key. Protected by pubKeyMu.
var (
	pubKeyMu     sync.RWMutex
	updatePubKey ed25519.PublicKey
)

// resolvePublicKey returns the trusted key: the runtime-configured key if set,
// otherwise the build-time DefaultPublicKeyHex, otherwise nil (SHA-only mode).
func resolvePublicKey() ed25519.PublicKey {
	pubKeyMu.RLock()
	k := updatePubKey
	pubKeyMu.RUnlock()
	if len(k) == ed25519.PublicKeySize {
		return k
	}
	if h := strings.TrimSpace(DefaultPublicKeyHex); h != "" {
		if raw, err := hex.DecodeString(h); err == nil && len(raw) == ed25519.PublicKeySize {
			return ed25519.PublicKey(raw)
		}
	}
	return nil
}

// signedMessage is the canonical bytes a release signature covers: the version
// and the binary SHA256, newline-separated. Both are integrity-critical (the
// URL is transport, gated separately by requireHTTPS; notes are cosmetic).
func signedMessage(version, sumHex string) []byte {
	return []byte(version + "\n" + sumHex)
}

// verifySignature enforces the release signature when a public key is
// configured. The source parameter determines the trust-anchor policy
// (UPD-001 fix):
//
//   - SourceEndpoint: a self-supplied checksum is not a trust anchor, so
//     signature verification is MANDATORY. Refuses the update with
//     ErrSignatureKeyNotConfigured when no key is configured; refuses
//     with ErrSignatureMissing when the manifest is unsigned; refuses
//     with *ErrSignatureInvalid when verification fails.
//   - SourceGitHub: the trust anchor is GitHub Releases' TLS + asset
//     integrity, so signature verification is best-effort. With no key
//     configured it returns nil (signed manifests are still verified
//     when a key is embedded; unsigned GitHub releases continue to
//     apply).