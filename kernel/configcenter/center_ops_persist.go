// SPDX-License-Identifier: MIT

package configcenter

// Provenance: configcenter: on-disk persistence (entryFile + persistEntry +
//             loadStoreFromDisk) and the vault for secret-rated values
//             (UseVault). Split from center_ops.go during Day 211 god-file
//             refactor (#45).

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"log/slog"
	"os"
	"path/filepath"
	"strings"

	"github.com/agezt/agezt/kernel/platform/filestore"
)

// SecretStore holds secret-rated values instead of the entry files. The daemon
// passes its encrypted, machine-bound vault (*creds.Store satisfies this).
type SecretStore interface {
	Get(name string) string
	Set(name, value string) error
	Remove(name string) bool
	Save() error
}

// VaultPrefix namespaces config-center values in the vault, so they read as
// "configcenter:<key>" next to the provider credentials.
const VaultPrefix = "configcenter:"

func vaultName(key string) string { return VaultPrefix + key }

func (c *Center) entryFile(key string) string {
	// Create a safe filename from the key
	safeName := fmt.Sprintf("%x", sha256.Sum256([]byte(key)))[:16]
	return filepath.Join(c.config.Dir, fmt.Sprintf("entry_%s.json", safeName))
}

// UseVault attaches the vault and moves every secret-rated value into it.
// Entry files used to hold secret values in plaintext; from here on a
// secret-rated entry's file carries neither the value nor its hash, and the
// value is loaded from the vault. It returns how many plaintext secrets it
// migrated. A failure leaves the affected entry exactly as it was.
func (c *Center) UseVault(v SecretStore) (migrated int, err error) {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.vault = v
	for _, e := range c.store.List() {
		switch {
		case e.VaultBacked:
			e.Value = v.Get(e.VaultPath)
			e.ValueHash = hashValue(e.Value)
		case e.Rating == RatingSecret && e.Value != "":
			if perr := c.persistEntry(e); perr != nil {
				return migrated, fmt.Errorf("config center: move %q into the vault: %w", e.Key, perr)
			}
			migrated++
		}
	}
	return migrated, nil
}

// persistEntry writes an entry to disk: atomically, 0600, in a 0700 directory
// (internal/atomicfile via filestore). With a vault attached, a secret-rated
// value goes to the vault and the file keeps neither it nor its hash — a hash of
// a low-entropy secret is as good as the secret to an offline guesser. An entry
// that stops being secret takes its value back into its file, then leaves the
// vault. Caller holds c.mu for writing.
func (c *Center) persistEntry(entry *ConfigEntry) error {
	if c.config.Dir == "" {
		return nil
	}
	if err := filestore.EnsureDir(c.config.Dir); err != nil {
		return err
	}
	onDisk := *entry
	switch {
	case c.vault != nil && entry.Rating == RatingSecret:
		name := vaultName(entry.Key)
		if err := c.vault.Set(name, entry.Value); err != nil {
			return err
		}
		if err := c.vault.Save(); err != nil {
			return err
		}
		entry.VaultBacked, entry.VaultPath = true, name
		onDisk.VaultBacked, onDisk.VaultPath = true, name
		onDisk.Value, onDisk.ValueHash = "", ""
	case c.vault != nil && entry.VaultBacked:
		name := entry.VaultPath
		entry.VaultBacked, entry.VaultPath = false, ""
		onDisk.VaultBacked, onDisk.VaultPath = false, ""
		if err := filestore.Save(c.entryFile(entry.Key), &onDisk); err != nil {
			return err
		}
		c.vault.Remove(name)
		return c.vault.Save()
	}
	return filestore.Save(c.entryFile(entry.Key), &onDisk)
}

// removeEntry deletes an entry's file and, if it was vault-backed, its vault
// value. Caller holds c.mu for writing.
func (c *Center) removeEntry(entry *ConfigEntry) {
	_ = os.Remove(c.entryFile(entry.Key))
	if c.vault != nil && entry != nil && entry.VaultBacked {
		c.vault.Remove(entry.VaultPath)
		_ = c.vault.Save()
	}
}

func hashValue(v string) string {
	sum := sha256.Sum256([]byte(v))
	return hex.EncodeToString(sum[:])
}

// loadStoreFromDisk loads all entries from disk into the store.
func loadStoreFromDisk(store *Store, dir string) error {
	if dir == "" {
		return nil
	}

	entries, err := os.ReadDir(dir)
	if err != nil {
		if os.IsNotExist(err) {
			return nil
		}
		return err
	}

	for _, entry := range entries {
		// "entry_" is six bytes; this compared the first SEVEN ("entry_8…") with
		// it, so no file ever matched and every entry was lost at each restart.
		if entry.IsDir() || !strings.HasPrefix(entry.Name(), "entry_") || !strings.HasSuffix(entry.Name(), ".json") {
			continue
		}

		filepath := filepath.Join(dir, entry.Name())
		data, err := os.ReadFile(filepath)
		if err != nil {
			slog.Warn("config center: unreadable entry file skipped", "file", filepath, "error", err)
			continue
		}

		var e ConfigEntry
		if err := json.Unmarshal(data, &e); err != nil {
			slog.Warn("config center: corrupt entry file skipped", "file", filepath, "error", err)
			continue
		}

		store.Set(&e)
	}

	return nil
}
