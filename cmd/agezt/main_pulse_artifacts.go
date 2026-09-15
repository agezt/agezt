// SPDX-License-Identifier: MIT

// Artifact-indexer wiring + board slug + injectConfig + workspaceRoot helpers.
// Extracted from main_pulse.go during Day 211 god-file refactor (#57).
// Public API unchanged.
package main

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/agezt/agezt/internal/brand"
	"github.com/agezt/agezt/kernel/artifact"
	"github.com/agezt/agezt/kernel/creds"
	"github.com/agezt/agezt/kernel/event"
	kernelruntime "github.com/agezt/agezt/kernel/runtime"
	"github.com/agezt/agezt/kernel/settings"
)

func wireArtifactIndexer(ctx context.Context, k *kernelruntime.Kernel) {
	idx := k.ArtifactIndex()
	if idx == nil {
		return
	}
	sub, err := k.Bus().Subscribe(">", 256)
	if err != nil {
		return
	}
	go func() {
		defer sub.Cancel()
		for {
			select {
			case <-ctx.Done():
				return
			case ev, ok := <-sub.C:
				if !ok {
					return
				}
				if ev.Kind != event.KindToolResult {
					continue
				}
				var p struct {
					RawRef      string `json:"raw_ref"`
					Tool        string `json:"tool"`
					OutputBytes int64  `json:"output_bytes"`
				}
				if json.Unmarshal(ev.Payload, &p) != nil || p.RawRef == "" {
					continue
				}
				name := p.Tool
				if name == "" {
					name = "tool"
				}
				_, _ = idx.IndexRef(p.RawRef, artifact.Entry{
					Kind:   "tool-output",
					Source: "run",
					Name:   fmt.Sprintf("%s-output.txt", name),
					Mime:   "text/plain",
					Corr:   ev.CorrelationID,
					Size:   p.OutputBytes,
				}, time.Now().UnixMilli())
			}
		}
	}()
}
func boardSubjectSlug(topic string) string {
	var b strings.Builder
	lastDash := false
	for _, r := range strings.ToLower(strings.TrimSpace(topic)) {
		if (r >= 'a' && r <= 'z') || (r >= '0' && r <= '9') || r == '_' || r == '-' {
			b.WriteRune(r)
			lastDash = false
		} else if !lastDash {
			b.WriteByte('-')
			lastDash = true
		}
	}
	s := strings.Trim(b.String(), "-")
	if s == "" {
		return "untopiced"
	}
	return s
}
func injectConfig(baseDir string, vault *creds.Store, stdout io.Writer) map[string]bool {
	pinned := map[string]bool{}
	// Pin across the FULL merged surface (built-in + registered) so a skill's
	// registered field is also marked read-only when the operator pins it in .env.
	for _, sec := range settings.NewRegistry(baseDir).Sections() {
		for _, f := range sec.Fields {
			if os.Getenv(f.Env) != "" {
				pinned[f.Env] = true
			}
		}
	}
	if strings.EqualFold(strings.TrimSpace(os.Getenv(brand.EnvPrefix+"CONFIG")), "off") {
		return pinned
	}
	store := settings.NewStore(baseDir)
	if err := store.Load(); err != nil {
		fmt.Fprintf(stdout, "  config store     : load failed (%v) — environment only\n", err)
		return pinned
	}
	injected := 0
	for name, val := range store.All() {
		if val != "" && os.Getenv(name) == "" {
			_ = os.Setenv(name, val)
			injected++
		}
	}
	// Channel/config SECRETS live in the vault under their AGEZT_* name; inject
	// those too. Provider API keys are NON-AGEZT_ and resolved via the cred chain,
	// so they need no env injection.
	for _, name := range vault.Names() {
		if strings.HasPrefix(name, brand.EnvPrefix) && os.Getenv(name) == "" {
			if v := vault.Get(name); v != "" {
				_ = os.Setenv(name, v)
				injected++
			}
		}
	}
	if injected > 0 {
		fmt.Fprintf(stdout, "  config store     : %d setting(s) applied from %s\n", injected, store.Path)
	}
	return pinned
}
func workspaceRoot(baseDir string) string {
	if ws := os.Getenv(brand.EnvPrefix + "WORKSPACE"); ws != "" {
		return ws
	}
	return filepath.Join(baseDir, "workspace")
}
