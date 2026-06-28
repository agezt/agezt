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
package toolbox

import (
	"bufio"
	"context"
	"os/exec"
	"runtime"
	"sort"
	"strings"
	"sync"
	"time"
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

// ToolStatus is the per-tool detection result for the wire.
type ToolStatus struct {
	Name        string `json:"name"`
	Category    string `json:"category"`
	Description string `json:"description"`
	Installed   bool   `json:"installed"`
	Version     string `json:"version,omitempty"`
	Path        string `json:"path,omitempty"`
	Installable bool   `json:"installable"`
	Manager     string `json:"manager,omitempty"` // the manager that would install it
	Command     string `json:"command,omitempty"` // the install command, shown in the UI
}

// Inventory is the full detection snapshot.
type Inventory struct {
	OS             string       `json:"os"`
	Managers       []string     `json:"managers"`
	Tools          []ToolStatus `json:"tools"`
	InstalledCount int          `json:"installed_count"`
	MissingCount   int          `json:"missing_count"`
}

// versionTimeout bounds a single `--version` probe so a hanging or interactive
// binary (the inventory found ollama/convert misbehaving) can't stall detection.
const versionTimeout = 3 * time.Second

// probeVersion runs `bin <versionArgs>` and returns a one-line version string,
// best-effort. Never errors out the caller — a bad probe just yields "".
func probeVersion(ctx context.Context, bin string, args []string) string {
	cctx, cancel := context.WithTimeout(ctx, versionTimeout)
	defer cancel()
	cmd := exec.CommandContext(cctx, bin, args...)
	out, err := cmd.CombinedOutput()
	if err != nil && len(out) == 0 {
		return ""
	}
	return firstLine(string(out))
}

func firstLine(s string) string {
	sc := bufio.NewScanner(strings.NewReader(s))
	sc.Buffer(make([]byte, 0, 4096), 1<<16)
	for sc.Scan() {
		line := strings.TrimSpace(sc.Text())
		if line != "" {
			return clip(line, 120)
		}
	}
	return clip(strings.TrimSpace(s), 120)
}

func clip(s string, n int) string {
	if len(s) <= n {
		return s
	}
	return s[:n] + "…"
}

// Detect probes every catalog tool concurrently: LookPath for presence + a
// bounded version probe for the installed ones. Read-only.
func Detect(ctx context.Context) Inventory {
	goos := runtime.GOOS
	managers := DetectManagers(goos)
	inv := Inventory{OS: goos, Managers: ManagerList(managers)}

	statuses := make([]ToolStatus, len(Catalog))
	var wg sync.WaitGroup
	sem := make(chan struct{}, 12) // cap concurrent version probes
	for i, t := range Catalog {
		st := ToolStatus{Name: t.Name, Category: t.Category, Description: t.Description}
		if r, ok := ResolveInstall(t, goos, managers); ok {
			st.Installable = true
			st.Manager = r.Manager
			st.Command = strings.Join(r.Install, " ")
		}
		bin := t.bin(goos)
		if path, err := exec.LookPath(bin); err == nil {
			st.Installed = true
			st.Path = path
			wg.Add(1)
			go func(idx int, b string, va []string, base ToolStatus) {
				defer wg.Done()
				sem <- struct{}{}
				defer func() { <-sem }()
				base.Version = probeVersion(ctx, b, va)
				statuses[idx] = base
			}(i, bin, t.versionArgs(), st)
			continue
		}
		statuses[i] = st
	}
	wg.Wait()

	for _, st := range statuses {
		if st.Installed {
			inv.InstalledCount++
		} else {
			inv.MissingCount++
		}
		inv.Tools = append(inv.Tools, st)
	}
	return inv
}

// InstallResult is one tool's install outcome (streamed per-tool by the caller).
type InstallResult struct {
	Tool       string `json:"tool"`
	OK         bool   `json:"ok"`
	Skipped    bool   `json:"skipped,omitempty"` // not installable on this host
	Manager    string `json:"manager,omitempty"`
	Command    string `json:"command,omitempty"`
	Version    string `json:"version,omitempty"` // version after a successful install
	OutputTail string `json:"output_tail,omitempty"`
	Error      string `json:"error,omitempty"`
}

// installTimeout bounds one package install — downloads + compile can be slow,
// so this is generous.
const installTimeout = 20 * time.Minute

// Install runs the resolved package-manager command for one named tool at the
// host level and returns the outcome (re-probing the version on success).
// Unknown names / un-installable tools return Skipped, never an error, so a
// batch keeps going. progress, if non-nil, is unused here (the caller streams).
func Install(ctx context.Context, name string) InstallResult {
	goos := runtime.GOOS
	t, ok := byName(name)
	if !ok {
		return InstallResult{Tool: name, Skipped: true, Error: "unknown tool"}
	}
	managers := DetectManagers(goos)
	r, ok := ResolveInstall(t, goos, managers)
	res := InstallResult{Tool: name, Manager: r.Manager, Command: strings.Join(r.Install, " ")}
	if !ok || len(r.Install) == 0 {
		res.Skipped = true
		res.Error = "no install recipe for this host"
		return res
	}
	cctx, cancel := context.WithTimeout(ctx, installTimeout)
	defer cancel()
	cmd := exec.CommandContext(cctx, r.Install[0], r.Install[1:]...)
	out, err := cmd.CombinedOutput()
	res.OutputTail = tail(string(out), 1200)
	if err != nil {
		res.Error = err.Error()
		return res
	}
	res.OK = true
	res.Version = probeVersion(ctx, t.bin(goos), t.versionArgs())
	return res
}

func tail(s string, n int) string {
	s = strings.TrimSpace(s)
	if len(s) <= n {
		return s
	}
	return "…" + s[len(s)-n:]
}

func byName(name string) (Tool, bool) {
	for _, t := range Catalog {
		if t.Name == name {
			return t, true
		}
	}
	return Tool{}, false
}

// Outdated runs each present manager's "what can upgrade" query and returns the
// set of catalog tool names that appear upgradable. Best-effort and bounded —
// a manager that errors or has no upgrade-list command simply contributes
// nothing. Cross-references package ids loosely (substring), so it errs toward
// flagging rather than missing.
func Outdated(ctx context.Context) map[string]bool {
	goos := runtime.GOOS
	managers := DetectManagers(goos)
	flagged := map[string]bool{}

	queries := map[string][]string{}
	switch goos {
	case "windows":
		if managers["winget"] {
			queries["winget"] = []string{"winget", "upgrade"}
		}
		if managers["choco"] {
			queries["choco"] = []string{"choco", "outdated", "-r"}
		}
		if managers["scoop"] {
			queries["scoop"] = []string{"scoop", "status"}
		}
	case "darwin":
		if managers["brew"] {
			queries["brew"] = []string{"brew", "outdated"}
		}
	default:
		if managers["apt"] {
			queries["apt"] = []string{"apt", "list", "--upgradable"}
		}
		if managers["dnf"] {
			queries["dnf"] = []string{"dnf", "check-update"}
		}
	}

	var blob strings.Builder
	for _, argv := range queries {
		cctx, cancel := context.WithTimeout(ctx, 25*time.Second)
		cmd := exec.CommandContext(cctx, argv[0], argv[1:]...)
		out, _ := cmd.CombinedOutput() // exit code is unreliable across managers
		blob.Write(out)
		blob.WriteByte('\n')
		cancel()
	}
	hay := strings.ToLower(blob.String())
	if hay == "" {
		return flagged
	}
	for _, t := range Catalog {
		// Match the tool's package id (manager-specific) or its name in the
		// upgrade list. The recipe id is the most precise signal.
		ids := []string{strings.ToLower(t.Name)}
		for _, rs := range t.Recipes[goos] {
			if len(rs.Install) > 0 {
				ids = append(ids, strings.ToLower(rs.Install[len(rs.Install)-1]))
			}
		}
		for _, id := range ids {
			if id != "" && len(id) >= 2 && strings.Contains(hay, id) {
				flagged[t.Name] = true
				break
			}
		}
	}
	return flagged
}
