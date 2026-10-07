// SPDX-License-Identifier: MIT
package controlplane

import (
	"io/fs"
	"os"
	"path/filepath"
)

type storageFilesystem struct{}

func (storageFilesystem) ReadDir(base string) ([]fs.DirEntry, error) { return os.ReadDir(base) }
func (storageFilesystem) Usage(dir string) (int64, int64)            { return dirUsage(dir) }

// dirUsage sums sizes and counts of regular files under dir. Best-effort like
// dirSize: a missing dir or unreadable entry contributes 0 — an inventory must
// never be the reason a diagnostic errors.
func dirUsage(dir string) (bytes, files int64) {
	_ = filepath.WalkDir(dir, func(_ string, d fs.DirEntry, err error) error {
		if err != nil || d.IsDir() {
			return nil
		}
		if info, err := d.Info(); err == nil {
			bytes += info.Size()
			files++
		}
		return nil
	})
	return bytes, files
}
