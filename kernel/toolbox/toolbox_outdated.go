// SPDX-License-Identifier: MIT
//
// Outdated reporter: the per-manager "what can upgrade" query runner that
// cross-references the catalog and returns the set of catalog tool names
// that appear upgradable. Best-effort and bounded.
// Extracted from toolbox.go during the Day-203 god-file split.
// Public API unchanged.
package toolbox

import (
	"context"
	"os/exec"
	"runtime"
	"strings"
	"time"
)

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
