// SPDX-License-Identifier: MIT
//
// Read-only detection path: ToolStatus + Inventory + versionTimeout +
// probeVersion + firstLine + clip + Detect.
// Extracted from toolbox.go during the Day-203 god-file split.
// Public API unchanged.
package toolbox

import (
	"bufio"
	"context"
	"os/exec"
	"runtime"
	"strings"
	"sync"
	"time"
)

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
