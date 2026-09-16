// SPDX-License-Identifier: MIT

// vault.go owns the `agt vault` dispatcher (cmdVault) and
// its inline help / runtime-KDF diagnostics printers
// (printVaultHelp, printVaultKDF). Every vault mutation
// subcommand (status / encrypt / decrypt / rotate /
// migrate) lives in vault_subcommands.go.
package main

import (
	"fmt"
	"io"

	"github.com/agezt/agezt/internal/brand"
	"github.com/agezt/agezt/kernel/creds"
)


// cmdVault dispatches `agt vault <subcommand>`. M1.w added three;
// M1.ee adds rotate:
//
//	agt vault status                   — show encrypted vs plaintext + path
//	agt vault encrypt                  — re-save plaintext vault as encrypted
//	                                    (requires AGEZT_VAULT_PASSPHRASE)
//	agt vault decrypt                  — re-save encrypted vault as plaintext
//	                                    (requires AGEZT_VAULT_PASSPHRASE)
//	agt vault rotate                   — re-encrypt under a NEW passphrase
//	                                    (needs both AGEZT_VAULT_PASSPHRASE
//	                                    and AGEZT_VAULT_PASSPHRASE_NEW)
//
// All four call the daemon-independent kernel/creds package
// directly — the vault file is operator-local, so going through
// the control plane would add round-trips without buying anything.
// (The daemon picks up changes via `agt provider reload`, same as
// any other vault edit.)
func cmdVault(args []string, stdout, stderr io.Writer) int {
	if len(args) == 0 {
		fmt.Fprintf(stderr, "%s vault: subcommand required (status|encrypt|decrypt|rotate|migrate)\n", brand.CLI)
		return 2
	}
	switch args[0] {
	case "status":
		return cmdVaultStatus(stdout, stderr)
	case "encrypt":
		return cmdVaultEncrypt(stdout, stderr)
	case "decrypt":
		return cmdVaultDecrypt(stdout, stderr)
	case "rotate":
		return cmdVaultRotate(stdout, stderr)
	case "migrate":
		return cmdVaultMigrate(stdout, stderr)
	case "-h", "--help", "help":
		printVaultHelp(stdout)
		return 0
	default:
		fmt.Fprintf(stderr, "%s vault: unknown subcommand %q (status|encrypt|decrypt|rotate|migrate)\n", brand.CLI, args[0])
		return 2
	}
}


func printVaultHelp(w io.Writer) {
	fmt.Fprintf(w, "usage: %s vault <subcommand>\n", brand.CLI)
	fmt.Fprintf(w, "\n")
	fmt.Fprintf(w, "  status     show whether the vault is encrypted (and where it lives)\n")
	fmt.Fprintf(w, "  encrypt    re-save plaintext vault as encrypted (needs AGEZT_VAULT_PASSPHRASE)\n")
	fmt.Fprintf(w, "  decrypt    re-save encrypted vault as plaintext (needs AGEZT_VAULT_PASSPHRASE)\n")
	fmt.Fprintf(w, "  rotate     re-encrypt under a new passphrase\n")
	fmt.Fprintf(w, "             (needs both AGEZT_VAULT_PASSPHRASE *and* AGEZT_VAULT_PASSPHRASE_NEW)\n")
	fmt.Fprintf(w, "  migrate    upgrade an old encrypted vault to the current key-derivation policy\n")
	fmt.Fprintf(w, "             (re-encrypts a legacy/low-iteration vault; needs AGEZT_VAULT_PASSPHRASE)\n")
	fmt.Fprintf(w, "\n")
	fmt.Fprintf(w, "Encryption is automatic on every Save when AGEZT_VAULT_PASSPHRASE is set;\n")
	fmt.Fprintf(w, "these subcommands force a re-save to migrate the existing file format.\n")
}

// printVaultKDF surfaces a vault's key-derivation policy and whether it is up
// to date, read from the envelope without needing the passphrase. It is a
// no-op for a plaintext or unreadable vault. A stale vault gets a pointer at
// `agt vault migrate` so the operator knows an upgrade is available before
// running it.
func printVaultKDF(stdout io.Writer, path string) {
	st, err := creds.InspectVault(path)
	if err != nil || !st.Encrypted {
		return
	}
	fmt.Fprintf(stdout, "key deriv:   %s (%d iterations)\n", st.KDF, st.Iterations)
	if st.UpToDate {
		fmt.Fprintf(stdout, "migration:   up to date\n")
	} else {
		fmt.Fprintf(stdout, "migration:   recommended — run `%s vault migrate` to upgrade to %s/%d iterations\n",
			brand.CLI, creds.KDFPBKDF2, creds.KDFIterations)
	}
}
