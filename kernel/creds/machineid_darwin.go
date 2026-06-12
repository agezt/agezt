// SPDX-License-Identifier: MIT

//go:build darwin

package creds

import (
	"os/exec"
	"regexp"
)

var ioPlatformUUIDRe = regexp.MustCompile(`"IOPlatformUUID"\s*=\s*"([0-9A-Fa-f-]+)"`)

// machineID returns the Mac's IOPlatformUUID (stable hardware identity), read
// via ioreg — the standard passphrase-less source on macOS. "" on any failure.
func machineID() string {
	out, err := exec.Command("/usr/sbin/ioreg", "-rd1", "-c", "IOPlatformExpertDevice").Output()
	if err != nil {
		return ""
	}
	if m := ioPlatformUUIDRe.FindSubmatch(out); m != nil {
		return string(m[1])
	}
	return ""
}
