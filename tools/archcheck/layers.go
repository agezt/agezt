// SPDX-License-Identifier: MIT

package main

import (
	"encoding/json"
	"fmt"
	"os"
	"strings"
)

// Layer numbers mirror architecture/20-target-architecture.md §2. Lower is
// more fundamental; a package may only depend downward (see rules.go).
const (
	LayerFoundation = 0 // internal/*: leaf helpers, stdlib only
	LayerContract   = 1 // pure types + interfaces
	LayerPlatform   = 2 // infrastructure with no domain knowledge
	LayerModule     = 3 // one bounded context per module
	LayerApp        = 4 // operation registry + pipelines
	LayerAdapter    = 5 // inbound transports
	LayerPlugin     = 6 // implementations of contracts
	LayerRoot       = 7 // binaries, SDKs, repo tooling: may import anything
)

var layerNames = map[int]string{
	LayerFoundation: "L0 foundation",
	LayerContract:   "L1 contract",
	LayerPlatform:   "L2 platform",
	LayerModule:     "L3 module",
	LayerApp:        "L4 app",
	LayerAdapter:    "L5 adapter",
	LayerPlugin:     "L6 plugin",
	LayerRoot:       "L7 root",
}

// Rule is one entry of layers.json: every package whose module-relative
// import path matches Pattern belongs to Layer (and, for L3, to Module).
//
// Pattern syntax: path segments separated by "/"; "*" matches exactly one
// segment; a trailing "/..." matches the path itself and everything below it.
// Module may be "@1" to take the segment the first "*" matched, which is how
// the future kernel/modules/<name>/... tree names its modules without one
// rule per module.
type Rule struct {
	Pattern string `json:"pattern"`
	Layer   int    `json:"layer"`
	Module  string `json:"module,omitempty"`
	Note    string `json:"note,omitempty"`
}

// Config is the parsed layers.json.
type Config struct {
	// ModulePath is the Go module prefix stripped from import paths.
	ModulePath string `json:"module_path"`
	Rules      []Rule `json:"rules"`
}

// Class is the classification of one package.
type Class struct {
	Layer  int
	Module string
	Rule   string // pattern that matched, for diagnostics
}

func loadConfig(path string) (*Config, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, err
	}
	var cfg Config
	if err := json.Unmarshal(data, &cfg); err != nil {
		return nil, fmt.Errorf("parse %s: %w", path, err)
	}
	if err := cfg.validate(); err != nil {
		return nil, fmt.Errorf("%s: %w", path, err)
	}
	return &cfg, nil
}

func (c *Config) validate() error {
	if c.ModulePath == "" {
		return fmt.Errorf("module_path is required")
	}
	if len(c.Rules) == 0 {
		return fmt.Errorf("no rules")
	}
	seen := map[string]bool{}
	for i, r := range c.Rules {
		if r.Pattern == "" {
			return fmt.Errorf("rule %d: empty pattern", i)
		}
		if seen[r.Pattern] {
			return fmt.Errorf("rule %d: duplicate pattern %q", i, r.Pattern)
		}
		seen[r.Pattern] = true
		if _, ok := layerNames[r.Layer]; !ok {
			return fmt.Errorf("rule %q: unknown layer %d", r.Pattern, r.Layer)
		}
		if r.Layer == LayerModule && r.Module == "" {
			return fmt.Errorf("rule %q: L3 rules must name a module", r.Pattern)
		}
		if r.Layer != LayerModule && r.Module != "" {
			return fmt.Errorf("rule %q: only L3 rules may name a module", r.Pattern)
		}
		if r.Module == "@1" && !strings.Contains(r.Pattern, "*") {
			return fmt.Errorf("rule %q: module @1 needs a * in the pattern", r.Pattern)
		}
	}
	return nil
}

// relPath strips the module prefix; packages outside the module return ok=false.
func (c *Config) relPath(importPath string) (string, bool) {
	if importPath == c.ModulePath {
		return "", true
	}
	if rest, ok := strings.CutPrefix(importPath, c.ModulePath+"/"); ok {
		return rest, true
	}
	return "", false
}

// classify returns the class of a module-relative path. The FIRST matching
// rule wins, so specific patterns must precede broader ones in layers.json.
// An unmatched package is an error on purpose (principle P6, fail closed):
// a new package must be placed in the architecture before it can merge.
func (c *Config) classify(rel string) (Class, bool) {
	for _, r := range c.Rules {
		capture, ok := match(r.Pattern, rel)
		if !ok {
			continue
		}
		mod := r.Module
		if mod == "@1" {
			mod = capture
		}
		return Class{Layer: r.Layer, Module: mod, Rule: r.Pattern}, true
	}
	return Class{}, false
}

// match reports whether rel matches pattern and returns the segment matched
// by the first "*" (if any).
func match(pattern, rel string) (string, bool) {
	recursive := false
	if p, ok := strings.CutSuffix(pattern, "/..."); ok {
		pattern, recursive = p, true
	}
	ps := strings.Split(pattern, "/")
	rs := strings.Split(rel, "/")
	if len(rs) < len(ps) || (!recursive && len(rs) != len(ps)) {
		return "", false
	}
	capture, captured := "", false
	for i, seg := range ps {
		if seg == "*" {
			if !captured {
				capture, captured = rs[i], true
			}
			continue
		}
		if seg != rs[i] {
			return "", false
		}
	}
	return capture, true
}

// isAPIPackage reports whether rel is a module's public api package — the one
// place another module may import (principle P5).
func isAPIPackage(rel string) bool {
	return strings.HasSuffix(rel, "/api") || strings.Contains(rel, "/api/")
}
