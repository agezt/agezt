// SPDX-License-Identifier: MIT

package fileworkspace

import (
	"errors"
	"os"
)

// ErrSymlink retains the console's final-component deletion refusal.
var ErrSymlink = errors.New("symlinks are not served")

// Mkdir creates a directory at an already resolved workspace path.
// Callers own path resolution, authorization and journal admission.
func Mkdir(target string, parents bool) error {
	if parents {
		return os.MkdirAll(target, defaultDirPerm)
	}
	return os.Mkdir(target, defaultDirPerm)
}

// Rename moves an entry between already resolved workspace paths.
func Rename(from, to string) error {
	return os.Rename(from, to)
}

// Delete retains the console's Lstat guard and explicit recursive opt-in.
// Missing entries and filesystem failures retain their original OS errors.
func Delete(target string, recursive bool) error {
	info, err := os.Lstat(target)
	if err != nil {
		return err
	}
	if info.Mode()&os.ModeSymlink != 0 {
		return ErrSymlink
	}
	if info.IsDir() && recursive {
		return os.RemoveAll(target)
	}
	return os.Remove(target)
}
