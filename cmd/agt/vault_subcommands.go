// SPDX-License-Identifier: MIT

// vault_subcommands.go owns the five `agt vault` mutation
// subcommands: cmdVaultMigrate (PBKDF2 upgrade), cmdVaultStatus
// (encrypted vs plaintext + KDF metadata), cmdVaultEncrypt
// (plaintext → encrypted), cmdVaultRotate (re-encrypt under a
// new passphrase), and cmdVaultDecrypt (encrypted → plaintext).
// The dispatcher + inline help / KDF diagnostics in
// vault.go. All call kernel/creds directly — operator-local,
// no daemon round-trip needed.
package main

import (
	"fmt"
	"io"
	"os"
	"strings"

	"github.com/agezt/agezt/internal/brand"
	"github.com/agezt/agezt/internal/paths"
	"github.com/agezt/agezt/kernel/creds"
)


// cmdVaultMigrate upgrades an encrypted vault to the current key-derivation
// policy (PBKDF2 at the current iteration count). It inspects the envelope
// without the passphrase first, so the plaintext / absent / already-current
// cases report clearly without demanding the secret; only an actual re-encrypt
// needs AGEZT_VAULT_PASSPHRASE (M264).
func cmdVaultMigrate(stdout, stderr io.Writer) int {
	base, err := paths.BaseDir()
	if err != nil {
		fmt.Fprintf(stderr, "%s vault migrate: %v\n", brand.CLI, err)
		return 1
	}
	store := creds.NewStore(base)

	st, err := creds.InspectVault(store.Path)
	if err != nil {
		fmt.Fprintf(stderr, "%s vault migrate: %v\n", brand.CLI, err)
		return 1
	}
	if !st.Encrypted {
		fmt.Fprintf(stdout, "vault is not encrypted — nothing to migrate (`%s vault encrypt` to encrypt it)\n", brand.CLI)
		return 0
	}
	if st.UpToDate {
		fmt.Fprintf(stdout, "vault already at the current key-derivation policy (%s, %d iterations)\n", st.KDF, st.Iterations)
		return 0
	}
	if strings.TrimSpace(os.Getenv(creds.PassphraseEnvVar)) == "" {
		fmt.Fprintf(stderr, "%s vault migrate: %s must be set to re-encrypt (current KDF: %s)\n", brand.CLI, creds.PassphraseEnvVar, st.KDF)
		return 2
	}
	migrated, before, err := store.MigrateEncryption()
	if err != nil {
		fmt.Fprintf(stderr, "%s vault migrate: %v\n", brand.CLI, err)
		return 1
	}
	if !migrated {
		fmt.Fprintf(stdout, "vault already up to date\n")
		return 0
	}
	fmt.Fprintf(stdout, "vault migrated: %s\n", store.Path)
	fmt.Fprintf(stdout, "  key derivation: %s → %s\n", before.KDF, creds.KDFPBKDF2)
	fmt.Fprintf(stdout, "  iterations    : %d → %d\n", before.Iterations, creds.KDFIterations)
	fmt.Fprintf(stdout, "run `%s provider reload` to reload the daemon with the upgraded file\n", brand.CLI)
	return 0
}


func cmdVaultStatus(stdout, stderr io.Writer) int {
	base, err := paths.BaseDir()
	if err != nil {
		fmt.Fprintf(stderr, "%s vault status: %v\n", brand.CLI, err)
		return 1
	}
	store := creds.NewStore(base)

	// Load is the only reliable way to learn whether the file is
	// encrypted, but Load on an encrypted vault without passphrase
	// returns ErrPassphraseRequired — which tells us what we need.
	err = store.Load()
	pathInfo := fmt.Sprintf("path:        %s\n", store.Path)
	if _, statErr := os.Stat(store.Path); os.IsNotExist(statErr) {
		fmt.Fprintf(stdout, "%sstatus:      no vault file (first run will create it)\n", pathInfo)
		return 0
	}
	switch err {
	case nil:
		state := "plaintext"
		if store.IsEncrypted() {
			state = "encrypted (aes-256-gcm)"
		}
		fmt.Fprintf(stdout, "%sstatus:      %s\nentries:     %d\n",
			pathInfo, state, len(store.Names()))
		printVaultKDF(stdout, store.Path)
	case creds.ErrPassphraseRequired:
		fmt.Fprintf(stdout, "%sstatus:      encrypted (aes-256-gcm)\n",
			pathInfo)
		fmt.Fprintf(stdout, "entries:     unknown (set AGEZT_VAULT_PASSPHRASE and re-run to count)\n")
		printVaultKDF(stdout, store.Path)
		fmt.Fprintf(stdout, "hint:        export AGEZT_VAULT_PASSPHRASE=... before running %s commands\n", brand.Binary)
	case creds.ErrWrongPassphrase:
		fmt.Fprintf(stdout, "%sstatus:      encrypted (aes-256-gcm)\n",
			pathInfo)
		fmt.Fprintf(stderr, "%s vault status: AGEZT_VAULT_PASSPHRASE set but decryption failed (wrong passphrase or corrupted file)\n", brand.CLI)
		return 1
	default:
		fmt.Fprintf(stderr, "%s vault status: %v\n", brand.CLI, err)
		return 1
	}
	return 0
}

func cmdVaultEncrypt(stdout, stderr io.Writer) int {
	if strings.TrimSpace(os.Getenv(creds.PassphraseEnvVar)) == "" {
		fmt.Fprintf(stderr, "%s vault encrypt: %s must be set\n", brand.CLI, creds.PassphraseEnvVar)
		return 2
	}
	base, err := paths.BaseDir()
	if err != nil {
		fmt.Fprintf(stderr, "%s vault encrypt: %v\n", brand.CLI, err)
		return 1
	}
	store := creds.NewStore(base)
	if err := store.Load(); err != nil {
		fmt.Fprintf(stderr, "%s vault encrypt: load: %v\n", brand.CLI, err)
		return 1
	}
	if store.IsEncrypted() {
		fmt.Fprintf(stdout, "vault is already encrypted (path: %s)\n", store.Path)
		return 0
	}
	// Save re-writes through the encryption path because passphrase
	// is now set.
	if err := store.Save(); err != nil {
		fmt.Fprintf(stderr, "%s vault encrypt: save: %v\n", brand.CLI, err)
		return 1
	}
	fmt.Fprintf(stdout, "vault encrypted: %s\n", store.Path)
	fmt.Fprintf(stdout, "remember: every shell/daemon that reads the vault now needs %s set\n", creds.PassphraseEnvVar)
	fmt.Fprintf(stdout, "run `%s provider reload` to reload the daemon with the new file\n", brand.CLI)
	return 0
}

// cmdVaultRotate re-encrypts the vault under a new passphrase
// (M1.ee). Both AGEZT_VAULT_PASSPHRASE (current) and
// AGEZT_VAULT_PASSPHRASE_NEW (target) must be set so the operator
// can hold both at once — without that separation a typo in the
// new passphrase would lose access to every vaulted credential
// permanently.
//
// Two-env-var protocol:
//
//	export AGEZT_VAULT_PASSPHRASE=oldpass
//	export AGEZT_VAULT_PASSPHRASE_NEW=newpass
//	agt vault rotate
//	# Then update your shell rc / systemd / 1Password CLI:
//	export AGEZT_VAULT_PASSPHRASE=newpass
//	unset AGEZT_VAULT_PASSPHRASE_NEW
//	agt provider reload   # daemon picks up the new vault state
//
// We refuse to operate if either var is missing — a one-sided
// "rotate to the same passphrase" is harmless but pointless;
// a one-sided "rotate from no current passphrase" is a category
// error (`agt vault encrypt` is the right command for a plaintext
// → encrypted transition).
func cmdVaultRotate(stdout, stderr io.Writer) int {
	cur := strings.TrimSpace(os.Getenv(creds.PassphraseEnvVar))
	nxt := strings.TrimSpace(os.Getenv(creds.NewPassphraseEnvVar))
	if cur == "" {
		fmt.Fprintf(stderr, "%s vault rotate: %s must be set (the current passphrase to decrypt the existing vault)\n",
			brand.CLI, creds.PassphraseEnvVar)
		return 2
	}
	if nxt == "" {
		fmt.Fprintf(stderr, "%s vault rotate: %s must be set (the new passphrase to re-encrypt under)\n",
			brand.CLI, creds.NewPassphraseEnvVar)
		return 2
	}
	if cur == nxt {
		fmt.Fprintf(stderr, "%s vault rotate: current and new passphrases are identical — nothing to do\n", brand.CLI)
		return 2
	}
	base, err := paths.BaseDir()
	if err != nil {
		fmt.Fprintf(stderr, "%s vault rotate: %v\n", brand.CLI, err)
		return 1
	}
	store := creds.NewStore(base)
	if err := store.Load(); err != nil {
		fmt.Fprintf(stderr, "%s vault rotate: load: %v\n", brand.CLI, err)
		return 1
	}
	if !store.IsEncrypted() {
		// Rotate is only meaningful on already-encrypted vaults. A
		// plaintext vault has no current passphrase to rotate from;
		// use `agt vault encrypt` instead.
		fmt.Fprintf(stderr, "%s vault rotate: vault is plaintext — use `%s vault encrypt` to set an initial passphrase\n",
			brand.CLI, brand.CLI)
		return 2
	}
	if err := store.Rotate(nxt); err != nil {
		fmt.Fprintf(stderr, "%s vault rotate: %v\n", brand.CLI, err)
		return 1
	}
	entries := len(store.Names())
	fmt.Fprintf(stdout, "vault re-encrypted under new passphrase: %s\n", store.Path)
	fmt.Fprintf(stdout, "entries preserved: %d\n", entries)
	fmt.Fprintf(stdout, "\nnext steps:\n")
	fmt.Fprintf(stdout, "  1. update %s in your shell rc / systemd unit to the new value\n", creds.PassphraseEnvVar)
	fmt.Fprintf(stdout, "  2. unset %s once the rotation is committed\n", creds.NewPassphraseEnvVar)
	fmt.Fprintf(stdout, "  3. run `%s provider reload` (the daemon still holds the old key in memory)\n", brand.CLI)
	return 0
}

func cmdVaultDecrypt(stdout, stderr io.Writer) int {
	pass := strings.TrimSpace(os.Getenv(creds.PassphraseEnvVar))
	if pass == "" {
		fmt.Fprintf(stderr, "%s vault decrypt: %s must be set (to read the current vault before re-saving plaintext)\n", brand.CLI, creds.PassphraseEnvVar)
		return 2
	}
	base, err := paths.BaseDir()
	if err != nil {
		fmt.Fprintf(stderr, "%s vault decrypt: %v\n", brand.CLI, err)
		return 1
	}
	store := creds.NewStore(base)
	if err := store.Load(); err != nil {
		fmt.Fprintf(stderr, "%s vault decrypt: load: %v\n", brand.CLI, err)
		return 1
	}
	if !store.IsEncrypted() {
		fmt.Fprintf(stdout, "vault is already plaintext (path: %s)\n", store.Path)
		return 0
	}
	// Override the passphrase function to return empty so Save
	// writes plaintext even though the env var is set. Operator-
	// intent capture: they ran `vault decrypt` knowing what they
	// want; we don't second-guess.
	store.SetPassphraseFn(func() string { return "" })
	if err := store.Save(); err != nil {
		fmt.Fprintf(stderr, "%s vault decrypt: save: %v\n", brand.CLI, err)
		return 1
	}
	fmt.Fprintf(stdout, "vault decrypted to plaintext: %s\n", store.Path)
	fmt.Fprintf(stdout, "warning: every credential in this file is now readable by anyone with file-read access\n")
	fmt.Fprintf(stdout, "         unset %s to keep vaults plaintext on future saves\n", creds.PassphraseEnvVar)
	return 0
}
