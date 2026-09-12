// SPDX-License-Identifier: MIT

// Registry launch: ResolveLaunch + launchForRegistryAgent.
// Code extracted from registry.go during the Day-70 god-file split. Public API unchanged.
package acpcatalog


import (
	"context"
	"errors"
	"fmt"
	"os/exec"
	"strings"
)


func countInventory(inv *Inventory) {
	inv.RegisteredCount, inv.CompatibleCount, inv.RunnableCount, inv.InstalledCount = 0, 0, 0, 0
	for _, st := range inv.Agents {
		if st.Registered {
			inv.RegisteredCount++
		}
		if st.Compatible {
			inv.CompatibleCount++
		}
		if st.Runnable {
			inv.RunnableCount++
		}
		if st.Installed {
			inv.InstalledCount++
		}
	}
	inv.MissingCount = len(inv.Agents) - inv.InstalledCount
}

type registryLaunch struct {
	Program       string
	Args          []string
	Env           map[string]string
	Runner        string
	Display       string
	Archive       string
	InstalledPath string
}

// LaunchSpec is a resolved ACP subprocess recipe. Registry entries are always
// represented as a program plus argument vector (Shell=false), so no registry
// field is interpolated into a command shell. Shell=true is reserved for the
// operator-controlled AGEZT_ACP_AGENT_CMD fallback.
type LaunchSpec struct {
	Program string
	Args    []string
	Env     map[string]string
	Display string
	Shell   bool
}

// ResolveLaunch resolves an untrusted per-call selector strictly as either a
// built-in slug or an exact official registry id. It never treats selector text
// as a command. Package distributions run through pinned npx/uvx recipes; binary
// distributions must already be on PATH. Only the empty-selector fallback may
// contain an operator-authored shell command.
func ResolveLaunch(ctx context.Context, ref, fallback string) (LaunchSpec, error) {
	ref = strings.TrimSpace(strings.ToLower(ref))
	if ref == "" {
		fallback = strings.TrimSpace(fallback)
		if fallback == "" {
			return LaunchSpec{}, errors.New("no default ACP agent is configured; select a registry agent")
		}
		return LaunchSpec{Program: fallback, Display: fallback, Shell: true}, nil
	}
	if !registryIDPattern.MatchString(ref) {
		return LaunchSpec{}, fmt.Errorf("invalid ACP registry agent id %q", ref)
	}

	// Prefer an already-installed direct binary for the common offline catalog.
	if a, ok := Find(ref); ok && Installed(a) {
		fields := strings.Fields(a.Command)
		if len(fields) == 0 {
			return LaunchSpec{}, fmt.Errorf("ACP agent %q has an empty launch command", ref)
		}
		program := fields[0]
		if path, err := exec.LookPath(a.Bin); err == nil {
			program = path
		}
		return LaunchSpec{Program: program, Args: fields[1:], Display: a.Command}, nil
	}

	reg, _, _, fetchErr := DefaultRegistry.Fetch(ctx, false)
	for _, a := range reg.Agents {
		if a.ID != ref {
			continue
		}
		launch, compatible, runnable := launchForRegistryAgent(a)
		if !compatible {
			return LaunchSpec{}, fmt.Errorf("ACP agent %q has no distribution for %s", ref, platformID())
		}
		if !runnable {
			if launch.Runner == "binary" && launch.Archive != "" {
				return LaunchSpec{}, fmt.Errorf("ACP agent %q is not installed; binary: %s", ref, launch.Archive)
			}
			return LaunchSpec{}, fmt.Errorf("ACP agent %q needs %s on PATH", ref, launch.Runner)
		}
		return LaunchSpec{
			Program: launch.Program, Args: launch.Args, Env: launch.Env,
			Display: launch.Display,
		}, nil
	}
	if fetchErr != nil {
		return LaunchSpec{}, fmt.Errorf("resolve ACP agent %q: registry unavailable: %w", ref, fetchErr)
	}
	return LaunchSpec{}, fmt.Errorf("ACP agent %q is not registered", ref)
}

func launchForRegistryAgent(a RegistryAgent) (registryLaunch, bool, bool) {
	platform := platformID()
	if target, ok := a.Distribution.Binary[platform]; ok {
		program := strings.TrimPrefix(strings.TrimSpace(target.Cmd), "./")
		if program != "" {
			candidate := program
			if i := strings.LastIndexAny(candidate, "/\\"); i >= 0 {
				candidate = candidate[i+1:]
			}
			if path, err := exec.LookPath(candidate); err == nil {
				l := registryLaunch{Program: path, Args: target.Args, Env: target.Env, Runner: "binary", Archive: target.Archive, InstalledPath: path}
				l.Display = renderCommand(candidate, target.Args)
				return l, true, true
			}
		}
	}
	if d := a.Distribution.NPX; d != nil {
		l := registryLaunch{Program: "npx", Args: append([]string{"--yes", d.Package}, d.Args...), Env: d.Env, Runner: "npx"}
		l.Display = renderCommand(l.Program, l.Args)
		_, err := exec.LookPath(l.Program)
		return l, true, err == nil
	}
	if d := a.Distribution.UVX; d != nil {
		l := registryLaunch{Program: "uvx", Args: append([]string{d.Package}, d.Args...), Env: d.Env, Runner: "uvx"}
		l.Display = renderCommand(l.Program, l.Args)
		_, err := exec.LookPath(l.Program)
		return l, true, err == nil
	}
	if target, ok := a.Distribution.Binary[platform]; ok {
		program := strings.TrimPrefix(strings.TrimSpace(target.Cmd), "./")
		l := registryLaunch{Program: program, Args: target.Args, Env: target.Env, Runner: "binary", Archive: target.Archive}
		l.Display = renderCommand(program, target.Args)
		return l, true, false
	}
	return registryLaunch{}, false, false
}