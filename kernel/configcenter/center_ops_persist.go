// SPDX-License-Identifier: MIT

package configcenter

// Provenance: configcenter: on-disk persistence helpers (entryFile + persistEntry +
//             loadStoreFromDisk). Split from center_ops.go during Day 211 god-file
//             refactor (#45). Public API unchanged.

import (
	"crypto/sha256"
	"encoding/json"
	"fmt"
	"log/slog"
	"os"
	"path/filepath"
	"strings"
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
