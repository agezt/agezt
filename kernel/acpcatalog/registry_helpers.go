// SPDX-License-Identifier: MIT

// Registry helpers: platformID + commandMatchesLaunch + renderCommand + displayArg + firstNonEmpty.
// Code extracted from registry.go during the Day-70 god-file split. Public API unchanged.
package acpcatalog


import (
	"runtime"
	"strings"
)



func platformID() string {
	arch := runtime.GOARCH
	switch arch {
	case "amd64":
		arch = "x86_64"
	case "arm64":
		arch = "aarch64"
	}
	return runtime.GOOS + "-" + arch
}

func commandMatchesLaunch(active string, launch registryLaunch) bool {
	active = strings.TrimSpace(active)
	if active == "" || launch.Program == "" {
		return false
	}
	fields := strings.Fields(active)
	if len(fields) == 0 {
		return false
	}
	first := strings.ToLower(fields[0])
	if i := strings.LastIndexAny(first, "/\\"); i >= 0 {
		first = first[i+1:]
	}
	first = strings.TrimSuffix(first, ".exe")
	want := strings.ToLower(launch.Program)
	if i := strings.LastIndexAny(want, "/\\"); i >= 0 {
		want = want[i+1:]
	}
	want = strings.TrimSuffix(want, ".exe")
	if first != want {
		return false
	}
	// A runner executable alone is not an identity: every npx entry starts with
	// npx and every uvx entry starts with uvx. Require the pinned package too so
	// one configured npx default does not mark the entire registry active.
	if launch.Runner == "npx" || launch.Runner == "uvx" {
		packageIndex := 0
		if launch.Runner == "npx" {
			packageIndex = 1 // args[0] is --yes
		}
		if packageIndex >= len(launch.Args) {
			return false
		}
		wantPackage := launch.Args[packageIndex]
		for _, field := range fields[1:] {
			if field == wantPackage {
				return true
			}
		}
		return false
	}
	return true
}

func renderCommand(program string, args []string) string {
	parts := make([]string, 0, len(args)+1)
	parts = append(parts, displayArg(program))
	for _, arg := range args {
		parts = append(parts, displayArg(arg))
	}
	return strings.TrimSpace(strings.Join(parts, " "))
}

func displayArg(s string) string {
	if s == "" {
		return `""`
	}
	if strings.ContainsAny(s, " \t\r\n\"") {
		return `"` + strings.ReplaceAll(s, `"`, `\"`) + `"`
	}
	return s
}

func firstNonEmpty(values ...string) string {
	for _, v := range values {
		if strings.TrimSpace(v) != "" {
			return v
		}
	}
	return ""
}
