// SPDX-License-Identifier: MIT

// Package paths resolves the Agezt runtime base directory. Convention is
// $AGEZT_HOME if set, else <user-home>/.agezt (DECISIONS A1's ConfigDir).
package paths

import (
	"errors"
	"os"
	"path/filepath"

	"github.com/ersinkoc/agezt/internal/brand"
)

// BaseDir returns the resolved Agezt base directory.
//
//   - $AGEZT_HOME overrides everything.
//   - Otherwise <user-home>/.agezt.
//
// The directory is NOT created here; subsystems (journal, state,
// controlplane) create their own subdirs on first use.
func BaseDir() (string, error) {
	if env := os.Getenv(brand.EnvPrefix + "HOME"); env != "" {
		return env, nil
	}
	home, err := os.UserHomeDir()
	if err != nil {
		return "", errors.New("paths: cannot resolve user home directory; set " + brand.EnvPrefix + "HOME")
	}
	return filepath.Join(home, brand.ConfigDir), nil
}
