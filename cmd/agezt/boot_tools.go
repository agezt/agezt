// SPDX-License-Identifier: MIT

package main

import (
	"fmt"
	"io"
	"os"
	"strings"

	"github.com/agezt/agezt/internal/brand"
	"github.com/agezt/agezt/kernel/agent"
	kernelruntime "github.com/agezt/agezt/kernel/runtime"
	"github.com/agezt/agezt/kernel/toolreg"
	"github.com/agezt/agezt/kernel/warden"
	"github.com/agezt/agezt/plugins/builtintools"
)

// buildTools builds the first-party tool map for the kernel — entirely from
// the kernel/toolreg registry (Phase 2.2; the specs live in
// plugins/builtintools, the tool mirror of builtinchannels):
//
//	tools map + registry Set + plugin manifest + per-tool policy cap map +
//	human banner description.
//
// The *toolreg.Set carries the built specs alongside the map so main.go can
// drive the remaining phases against the SAME instances the map holds:
// Set.ApplyPreOpen before runtime.Open, Set.Configure after it, and
// Set.ConfigureLate once the live channels + shared board store exist.
// notifyTargets (channel kind → operator allowlist ids, derived from the
// channel manifests BEFORE this call) gates the notify/send_media specs.
// AGEZT_ALLOW_ALL is the master permissive switch (M611): it implies the full
// open posture for the network tools too — any host, including loopback and
// the private network.
func buildTools(baseDir string, stderr io.Writer, ward warden.Engine, notifyTargets map[string][]string) (map[string]agent.Tool, *toolreg.Set, []kernelruntime.PluginInfo, map[string]string, string, error) {
	// Idempotent (Register replaces by name), so tests calling buildTools
	// repeatedly and the daemon calling it at boot are both fine.
	builtintools.RegisterAll()
	set, err := toolreg.BuildAll(toolreg.BuildDeps{
		BaseDir: baseDir,
		// Workspace root — both the file tool (scoped to it) and the shell
		// tool (runs in it, M609) use this so `dir`/`ls` (shell) and `file
		// read x` (file) agree on what "here" is.
		WorkspaceRoot: workspaceRoot(baseDir),
		Warden:        ward,
		Stderr:        stderr,
		Get:           os.Getenv,
		AllowAll:      os.Getenv(brand.EnvPrefix+"ALLOW_ALL") == "1",
		NotifyTargets: notifyTargets,
	})
	if err != nil {
		return nil, nil, nil, nil, "", err
	}
	return set.Tools(), set, set.PluginManifest(), set.ToolCapabilities(), strings.Join(set.Descs(), ", "), nil
}

// councilSeatName labels the i-th council seat (M837): the first three get
// distinct elder names, the rest a numbered fallback.
func councilSeatName(i int) string {
	names := []string{"Elder Alpha", "Elder Beta", "Elder Gamma"}
	if i < len(names) {
		return names[i]
	}
	return fmt.Sprintf("Elder %d", i+1)
}
