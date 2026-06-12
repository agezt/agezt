// SPDX-License-Identifier: MIT

package creds

// Machine-bound at-rest encryption (M934). The M1.w vault encryption existed
// but was OPT-IN via AGEZT_VAULT_PASSPHRASE — almost nobody sets it, so in
// practice every API key sat in creds.json in the clear ("kabak gibi").
//
// The vault now encrypts BY DEFAULT with a passphrase derived from stable
// machine + user identity (Windows MachineGuid, /etc/machine-id, macOS
// IOPlatformUUID — plus the OS user), so:
//
//   - creds.json on disk is always an AES-256-GCM envelope, never plaintext;
//   - a copy that leaves the machine (cloud-synced home, backup, accidental
//     commit, stolen disk image read on another box) does not decrypt;
//   - the operator manages NO passphrase — the same machine derives the same
//     key forever (the identity sources never change in normal operation).
//
// Honest threat model: a process running as the same user on the same machine
// can derive the key too — this protects the FILE leaving the machine, not
// against local same-user malware (nothing passphrase-less can). Operators who
// want a real secret keep AGEZT_VAULT_PASSPHRASE, which always WINS over the
// machine key; AGEZT_VAULT_AUTOENCRYPT=off restores plaintext-at-rest.
//
// Precedence (the default passphrase chain, used by Load and Save):
//
//	1. AGEZT_VAULT_PASSPHRASE   — operator-managed, unchanged semantics.
//	2. machine-derived key      — unless AGEZT_VAULT_AUTOENCRYPT=off.
//	3. "" (plaintext)           — opt-out, or no identity source available
//	                              (e.g. a container without /etc/machine-id).

import (
	"crypto/sha256"
	"encoding/hex"
	"os"
	"os/user"
	"strings"
	"sync"
)

// AutoEncryptEnvVar opts OUT of machine-bound auto-encryption: set to "off" to
// keep/return the vault to plaintext-at-rest (the pre-M934 default).
const AutoEncryptEnvVar = "AGEZT_VAULT_AUTOENCRYPT"

// machinePassphraseOnce caches the derived passphrase: the identity sources are
// stable for the process lifetime and the registry/exec lookups aren't free.
var machinePassphraseOnce = sync.OnceValue(computeMachinePassphrase)

// MachinePassphrase returns the machine+user-bound vault passphrase, or "" when
// no stable machine identity source is available on this host (the chain then
// falls through to plaintext rather than inventing an unstable key that would
// brick the vault on the next hostname change).
func MachinePassphrase() string { return machinePassphraseOnce() }

func computeMachinePassphrase() string {
	id := machineID()
	if id == "" {
		return ""
	}
	// Bind to the OS user too: per-user vaults on a shared machine derive
	// different keys. user.Current can fail in odd environments (static
	// binaries without cgo on some systems) — fall back to the env so the
	// derivation stays deterministic rather than erroring.
	who := ""
	if u, err := user.Current(); err == nil {
		who = u.Uid + "|" + u.Username
	} else {
		who = os.Getenv("USERNAME") + os.Getenv("USER")
	}
	sum := sha256.Sum256([]byte("agezt-vault-machine-v1|" + id + "|" + who))
	return "machine-v1:" + hex.EncodeToString(sum[:])
}

// defaultPassphraseChain is the Store's default passphrase source (M934): the
// explicit operator passphrase wins; otherwise the machine-bound key, unless
// auto-encryption is opted out.
func defaultPassphraseChain() string {
	if p := os.Getenv(PassphraseEnvVar); p != "" {
		return p
	}
	if strings.EqualFold(strings.TrimSpace(os.Getenv(AutoEncryptEnvVar)), "off") {
		return ""
	}
	return MachinePassphrase()
}

// EncryptInPlace upgrades a loaded PLAINTEXT vault to encrypted-at-rest using
// the current passphrase chain — the boot-time migration (M934). No-ops (false,
// nil) when the vault is already encrypted, is empty (nothing to protect; a
// first Save encrypts anyway), or no passphrase is available (opt-out / no
// machine identity). Call after Load.
func (s *Store) EncryptInPlace() (bool, error) {
	s.mu.RLock()
	already := s.wasEncrypted
	empty := len(s.data) == 0
	pass := s.passphraseFn()
	s.mu.RUnlock()
	if already || empty || pass == "" {
		return false, nil
	}
	if err := s.Save(); err != nil {
		return false, err
	}
	s.mu.Lock()
	s.wasEncrypted = true
	s.mu.Unlock()
	return true, nil
}
