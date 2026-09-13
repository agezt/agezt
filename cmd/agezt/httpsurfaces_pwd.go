// SPDX-License-Identifier: MIT
//
// cmd/agezt WebUI password helpers: consolePasswordFile + consolePasswordBytes
// consts + ensureConsolePassword (the minter) +
// webPasswordDefaultDisabled (the env-check) +
// effectiveWebPassword (the resolution) +
// bannerColor + bannerColorEnabled (the rendering helpers).
// Extracted from httpsurfaces.go during the Day-208 god-file split.
// Public API unchanged.
package main

import (
	"crypto/rand"
	"encoding/hex"
	"fmt"
	"os"
	"strings"

	"github.com/agezt/agezt/internal/brand"
	kernelauth "github.com/agezt/agezt/kernel/auth"
)

func envDisabled(value string) bool {
	switch strings.ToLower(strings.TrimSpace(value)) {
	case "off", "disabled", "none", "no", "0", "false":
		return true
	default:
		return false
	}
}

func bannerColor(s, code string) string {
	if !bannerColorEnabled() {
		return s
	}
	return "\x1b[" + code + "m" + s + "\x1b[0m"
}

func bannerColorEnabled() bool {
	if os.Getenv("NO_COLOR") != "" {
		return false
	}
	if strings.HasSuffix(os.Args[0], ".test") {
		return false
	}
	return true
}

// consolePasswordFile holds the per-install built-in console password, beside
// the other 0600 credential files under the daemon's base directory.
const consolePasswordFile = "web-password"

// consolePasswordBytes is the entropy of a minted console password. 96 bits,
// rendered as 24 hex characters — shorter than a 256-bit listen token because a
// human pastes this one into a login form, and far beyond guessing for a
// credential that gates the whole control plane.
const consolePasswordBytes = 12

// ensureConsolePassword returns this install's built-in console password,
// minting and persisting one (0600) the first time it is asked. minted reports
// whether THIS call created it, which is what lets the caller print a fresh
// password in the boot banner exactly once.
//
// A read failure is returned rather than swallowed: minting a second password
// over a file we could not read would lock the operator out of the one already
// in force. An empty file is treated as absent (mint), which also self-heals a
// truncated write.
func ensureConsolePassword(baseDir string) (password string, minted bool, err error) {
	existing, err := kernelauth.ReadTokenFile(baseDir, consolePasswordFile)
	if err != nil {
		return "", false, err
	}
	if existing != "" {
		return existing, false, nil
	}
	raw := make([]byte, consolePasswordBytes)
	if _, err := rand.Read(raw); err != nil {
		return "", false, fmt.Errorf("mint console password: %w", err)
	}
	pw := hex.EncodeToString(raw)
	if _, err := kernelauth.WriteTokenFile(baseDir, consolePasswordFile, pw); err != nil {
		return "", false, err
	}
	return pw, true, nil
}

// webPasswordDefaultDisabled reports the operator's explicit opt-out of the
// BUILT-IN password (AGEZT_WEB_PASSWORD_DEFAULT). Consulted both where the
// password is minted and where it is resolved, so the daemon never persists a
// credential it has been told not to use.
func webPasswordDefaultDisabled() bool {
	return envDisabled(os.Getenv(brand.EnvPrefix + "WEB_PASSWORD_DEFAULT"))
}

// effectiveWebPassword resolves the console password for one bind. Precedence,
// highest first: an explicitly configured AGEZT_WEB_PASSWORD (re-read per gate
// decision, so Setup / Config Center apply live); the opt-out keyword on
// AGEZT_WEB_PASSWORD_DEFAULT, which turns the built-in off entirely; otherwise
// the per-install builtin, but only on a loopback bind — a console reachable
// beyond localhost gets no built-in password at all.
//
// builtin comes from ensureConsolePassword and is "" when the daemon could not
// mint one, which correctly degrades to "token only" rather than to a default.
func effectiveWebPassword(builtin, addr string) string {
	if v := strings.TrimSpace(os.Getenv(brand.EnvPrefix + "WEB_PASSWORD")); v != "" {
		return v
	}
	if webPasswordDefaultDisabled() {
		return ""
	}
	if isLoopback(addr) {
		return builtin
	}
	return ""
}
