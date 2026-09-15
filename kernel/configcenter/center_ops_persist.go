// SPDX-License-Identifier: MIT

// configcenter: on-disk persistence helpers (entryFile + persistEntry +
// loadStoreFromDisk). Split from center_ops.go during Day 211 god-file
// refactor (#45). Public API unchanged.
package configcenter

import (
	"crypto/sha256"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
)

func (c *Center) entryFile(key string) string {
	// Create a safe filename from the key
	safeName := fmt.Sprintf("%x", sha256.Sum256([]byte(key)))[:16]
	return filepath.Join(c.config.Dir, fmt.Sprintf("entry_%s.json", safeName))
}

// persistEntry writes an entry to disk.
func (c *Center) persistEntry(entry *ConfigEntry) error {
	if c.config.Dir == "" {
		return nil
	}

	os.MkdirAll(c.config.Dir, 0755)

	filename := c.entryFile(entry.Key)

	// Marshal to JSON
	data, err := json.MarshalIndent(entry, "", "  ")
	if err != nil {
		return fmt.Errorf("marshal entry: %w", err)
	}

	return os.WriteFile(filename, data, 0644)
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
		if entry.IsDir() || len(entry.Name()) < 7 || entry.Name()[:7] != "entry_" {
			continue
		}

		filepath := filepath.Join(dir, entry.Name())
		data, err := os.ReadFile(filepath)
		if err != nil {
			continue
		}

		var e ConfigEntry
		if err := json.Unmarshal(data, &e); err != nil {
			continue
		}

		store.Set(&e)
	}

	return nil
}
