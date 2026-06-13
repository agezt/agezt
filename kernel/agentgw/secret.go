// SPDX-License-Identifier: MIT

package agentgw

import (
	"crypto/rand"
	"encoding/hex"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"time"
)

// TokenSecretEnv, when set, overrides the persisted per-install gateway token
// secret. Useful when the daemon and the `agt` CLI run on different hosts and
// the operator distributes one secret out-of-band.
const TokenSecretEnv = "AGEZT_AGENTGW_TOKEN_SECRET"

// tokenSecretFile is the basename of the persisted per-install secret under
// the AGEZT base directory (0600, beside the encrypted vault).
const tokenSecretFile = "agentgw.secret"

// secretBytes is the length of a freshly generated token secret.
const secretBytes = 32

// ResolveTokenSecret returns the HMAC signing secret for agent-gateway tokens,
// shared between the daemon (which validates) and the `agt` CLI (which mints).
//
// Resolution order:
//  1. $AGEZT_AGENTGW_TOKEN_SECRET, if set (trimmed of surrounding whitespace so
//     a value piped from a shell heredoc still matches).
//  2. <baseDir>/agentgw.secret, if present (hex-encoded 32 bytes, 0600).
//  3. Otherwise a fresh 32-byte CSPRNG secret, persisted to that file (0600,
//     O_EXCL to survive a daemon/CLI first-run race) so every process derives
//     the same key.
//
// This replaces the former hardcoded "change-me-in-production" constant: the
// signing key is now per-install and never present in source.
func ResolveTokenSecret(baseDir string) ([]byte, error) {
	if env := strings.TrimSpace(os.Getenv(TokenSecretEnv)); env != "" {
		return []byte(env), nil
	}
	if baseDir == "" {
		// Nowhere to persist: use a process-lifetime random secret. Tokens
		// minted by this process stay valid only while it lives — the safe
		// default, and never a fixed key.
		return randomSecret()
	}
	if err := os.MkdirAll(baseDir, 0o700); err != nil {
		return nil, fmt.Errorf("agentgw: create base dir: %w", err)
	}
	path := filepath.Join(baseDir, tokenSecretFile)

	// Fast path: a decodable secret already exists.
	if b, err := os.ReadFile(path); err == nil {
		if s := decodeSecret(b); len(s) > 0 {
			return s, nil
		}
	}

	// First run: claim the slot exclusively and persist a fresh secret. A
	// concurrent first-runner that loses the O_EXCL race reads the winner's key
	// back via readPersistedSecretRetrying (which bridges the create→write
	// window), so the daemon and the CLI converge on ONE key.
	secret, err := randomSecret()
	if err != nil {
		return nil, err
	}
	f, err := os.OpenFile(path, os.O_CREATE|os.O_EXCL|os.O_WRONLY, 0o600)
	if err != nil {
		if os.IsExist(err) {
			// Lost the first-run claim; another runner owns the file.
			return readPersistedSecretRetrying(path)
		}
		return nil, fmt.Errorf("agentgw: persist token secret: %w", err)
	}
	if _, err := f.WriteString(hex.EncodeToString(secret)); err != nil {
		f.Close()
		return nil, fmt.Errorf("agentgw: write token secret: %w", err)
	}
	if err := f.Close(); err != nil {
		return nil, fmt.Errorf("agentgw: close token secret: %w", err)
	}
	return secret, nil
}

// readPersistedSecretRetrying reads a decodable secret from path, retrying with
// a short backoff. It exists for the first-run race: the O_EXCL winner creates
// the file and then writes its contents in two separate steps, so a loser that
// lost the claim can observe the file existing but still empty. The retry
// bridges that window (worst case ~250 ms) so losers converge on the winner's
// key instead of erroring. This must work across processes (daemon + CLI), so
// there is no in-memory synchronization to lean on.
func readPersistedSecretRetrying(path string) ([]byte, error) {
	wait := time.Millisecond
	for range 8 {
		if b, err := os.ReadFile(path); err == nil {
			if s := decodeSecret(b); len(s) > 0 {
				return s, nil
			}
		}
		time.Sleep(wait)
		wait *= 2
	}
	// One final attempt after the backoff window.
	if b, err := os.ReadFile(path); err != nil {
		return nil, fmt.Errorf("agentgw: read token secret: %w", err)
	} else if s := decodeSecret(b); len(s) > 0 {
		return s, nil
	}
	return nil, fmt.Errorf("agentgw: token secret %s claimed but empty after retries", path)
}

// decodeSecret interprets a persisted secret file's bytes: hex-encoded when it
// decodes to a full-length key, otherwise the trimmed raw bytes (so an
// operator-edited passphrase file still works).
func decodeSecret(b []byte) []byte {
	s := strings.TrimSpace(string(b))
	if s == "" {
		return nil
	}
	if raw, err := hex.DecodeString(s); err == nil && len(raw) >= secretBytes {
		return raw
	}
	return []byte(s)
}

// randomSecret returns a fresh 32-byte CSPRNG secret.
func randomSecret() ([]byte, error) {
	b := make([]byte, secretBytes)
	if _, err := rand.Read(b); err != nil {
		return nil, fmt.Errorf("agentgw: generate token secret: %w", err)
	}
	return b, nil
}
