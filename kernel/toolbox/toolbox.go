// SPDX-License-Identifier: MIT

// Package toolbox is the host CLI-tool inventory + installer (M956). It answers
// "what command-line tools are on this machine, which are missing, and which are
// out of date" and resolves a per-OS package-manager command to install a
// missing one — so the operator can provision the agent's host from the web UI
// instead of hand-running winget/brew/apt.
//
// Detection is read-only (exec.LookPath + a bounded `--version` probe).
// Installation runs the real package manager at the HOST level (no isolation —
// an installer must be able to change the system, which is the opposite of what
// warden's sandbox provides), so it is gated by the authed control plane and
// every install is journaled by the caller.
//
// The actual Detect / Install / Outdated paths live in toolbox_detect.go,
// toolbox_install.go and toolbox_outdated.go respectively.
// Extracted from toolbox.go during the Day-203 god-file split.
// Public API unchanged.
package toolbox

import (
	"os/exec"
	"sort"
)

// Recipe is one way to install a tool: a package manager plus the argv to run.
type Recipe struct {
	Manager string   // winget | choco | scoop | brew | apt | dnf | pacman | pip | npm | cargo | go
	Install []string // full argv, e.g. ["winget","install","-e","--id","jqlang.jq"]
	Upgrade []string // full argv for an in-place upgrade (optional)
}

// Tool is one catalog entry: a canonical command name plus per-OS install
// recipes. Recipes are ordered candidates; the first whose manager is present on
// the host wins (ResolveInstall).
type Tool struct {
	Name        string
	Category    string
	Description string
	VersionArgs []string            // how to print its version (default --version)
	BinByOS     map[string]string   // GOOS -> binary name when it differs from Name (e.g. apt fd-find -> fdfind)
	Recipes     map[string][]Recipe // GOOS -> ordered install candidates
}

// bin returns the binary name to look up for this tool on goos.
func (t Tool) bin(goos string) string {
	if t.BinByOS != nil {
		if b, ok := t.BinByOS[goos]; ok && b != "" {
			return b
		}
	}
	return t.Name
}

func (t Tool) versionArgs() []string {
	if len(t.VersionArgs) > 0 {
		return t.VersionArgs
	}
	return []string{"--version"}
}

// ResolveInstall picks the first recipe for goos whose manager is in available.
// Pure — the unit-tested core of per-OS routing. ok=false when nothing can
// install this tool on this host (no recipe for the OS, or no manager present).
func ResolveInstall(t Tool, goos string, available map[string]bool) (Recipe, bool) {
	for _, r := range t.Recipes[goos] {
		if available[r.Manager] {
			return r, true
		}
	}
	return Recipe{}, false
}

// managerProbes lists, per GOOS, the package-manager binaries to look for and
// the manager key they map to. Cross-platform language managers (pip/npm/...)
// are appended for every OS.
func managerProbes(goos string) map[string]string {
	probes := map[string]string{}
	switch goos {
	case "windows":
		probes["winget"] = "winget"
		probes["choco"] = "choco"
		probes["scoop"] = "scoop"
	case "darwin":
		probes["brew"] = "brew"
	default: // linux & friends
		probes["apt-get"] = "apt"
		probes["dnf"] = "dnf"
		probes["pacman"] = "pacman"
	}
	// Language/runtime managers, available anywhere they're installed.
	probes["pip"] = "pip"
	probes["pip3"] = "pip" // pip3 also satisfies the "pip" manager
	probes["npm"] = "npm"
	probes["cargo"] = "cargo"
	probes["go"] = "go"
	return probes
}

// DetectManagers reports which package managers are usable on this host.
func DetectManagers(goos string) map[string]bool {
	out := map[string]bool{}
	for probeBin, manager := range managerProbes(goos) {
		if _, err := exec.LookPath(probeBin); err == nil {
			out[manager] = true
		}
	}
	return out
}

// ManagerList returns the detected managers as a sorted slice (for the UI).
func ManagerList(m map[string]bool) []string {
	out := make([]string, 0, len(m))
	for k, v := range m {
		if v {
			out = append(out, k)
		}
	}
	sort.Strings(out)
	return out
}

func byName(name string) (Tool, bool) {
	for _, t := range Catalog {
		if t.Name == name {
			return t, true
		}
	}
	return Tool{}, false
}

