// SPDX-License-Identifier: MIT

package controlplane

// Named reusable fallback chains (M963): view and edit the governor's registry
// of named model ladders, plus the default chain. A chain is an ordered list of
// real model ids the governor tries in turn. Anywhere a model slot is set to
// "@<name>" (agent model, per-task chain, chat) the governor expands it to that
// chain's models at resolution time, so editing a chain in ONE place propagates
// everywhere it is referenced. Edits apply LIVE (governor.SetFallbackChains) and
// persist to the config store as AGEZT_FALLBACK_CHAINS / AGEZT_DEFAULT_CHAIN so
// they survive restart (startup injection + buildGovernor re-parse).

import (
	"fmt"
	"net"
	"regexp"
	"sort"
	"strings"

	"github.com/agezt/agezt/internal/brand"
	"github.com/agezt/agezt/kernel/governor"
	"github.com/agezt/agezt/kernel/settings"
)

// chainNameRe constrains chain names to a slug the "@name" token and UI can
// round-trip unambiguously (lower-case, digits, dash). No "@", no separators.
var chainNameRe = regexp.MustCompile(`^[a-z0-9][a-z0-9-]*$`)

type fallbackChainsGetter interface {
	FallbackChainsView() (map[string][]string, string)
}
type fallbackChainsSetter interface {
	SetFallbackChains(map[string][]string, string)
}

// handleChainsGet returns the named fallback chains, the default chain name, and
// per-chain usage activity folded from the journal (how often each chain's
// models actually fired in a fallback).
func (s *Server) handleChainsGet(conn net.Conn, req Request) {
	chains := map[string][]string{}
	def := ""
	if gov, ok := s.k.Provider().(fallbackChainsGetter); ok {
		chains, def = gov.FallbackChainsView()
	}
	s.writeResp(conn, Response{ID: req.ID, Type: RespResult, Result: map[string]any{
		"chains":  stringSliceMapToAny(chains),
		"default": def,
	}})
}

// handleChainsSet replaces the whole named-chain registry and default chain.
// args.chains is an object {name: [model, …]}; args.default is an optional chain
// name. Names are validated (slug), each chain must be non-empty, and chains may
// only hold real model ids (no nested "@name" — chains don't reference chains, so
// there is no cycle to resolve). The default, if set, must name a known chain.
// Applies live and persists. Unknown model ids are reported (warn) not rejected.
func (s *Server) handleChainsSet(conn net.Conn, req Request) {
	raw, ok := req.Args["chains"]
	if !ok {
		s.writeResp(conn, Response{ID: req.ID, Type: RespError, Error: "args.chains required (object {name: [models]})"})
		return
	}
	chains, err := decodeChains(raw)
	if err != nil {
		s.writeResp(conn, Response{ID: req.ID, Type: RespError, Error: err.Error()})
		return
	}
	// Validate names and reject nested chain references.
	for name, models := range chains {
		if !chainNameRe.MatchString(name) {
			s.writeResp(conn, Response{ID: req.ID, Type: RespError, Error: fmt.Sprintf("invalid chain name %q (use lower-case letters, digits, dashes)", name)})
			return
		}
		for _, m := range models {
			if strings.HasPrefix(m, governor.ChainPrefix) {
				s.writeResp(conn, Response{ID: req.ID, Type: RespError, Error: fmt.Sprintf("chain %q model %q: chains may not reference other chains", name, m)})
				return
			}
		}
	}
	def := strings.TrimSpace(stringArg(req.Args, "default"))
	if def != "" {
		if _, ok := chains[def]; !ok {
			s.writeResp(conn, Response{ID: req.ID, Type: RespError, Error: fmt.Sprintf("default chain %q is not one of the defined chains", def)})
			return
		}
	}

	// Persist to the config store (survives restart via injection + buildGovernor).
	store := settings.NewStore(s.baseDir)
	if err := store.Load(); err != nil {
		s.writeResp(conn, Response{ID: req.ID, Type: RespError, Error: "load config: " + err.Error()})
		return
	}
	chainsEnv := brand.EnvPrefix + "FALLBACK_CHAINS"
	if spec := encodeChains(chains); spec != "" {
		store.Set(chainsEnv, spec)
	} else {
		store.Remove(chainsEnv)
	}
	defEnv := brand.EnvPrefix + "DEFAULT_CHAIN"
	if def != "" {
		store.Set(defEnv, def)
	} else {
		store.Remove(defEnv)
	}
	if err := store.Save(); err != nil {
		s.writeResp(conn, Response{ID: req.ID, Type: RespError, Error: "save config: " + err.Error()})
		return
	}

	// Apply live.
	if gov, ok := s.k.Provider().(fallbackChainsSetter); ok {
		gov.SetFallbackChains(chains, def)
	}

	// Warn on model ids the catalog doesn't know (don't block).
	var unknown []string
	cat := s.k.Catalog()
	seen := map[string]bool{}
	for _, models := range chains {
		for _, m := range models {
			if seen[m] {
				continue
			}
			seen[m] = true
			if _, mdl := cat.FindModel(m); mdl == nil {
				unknown = append(unknown, m)
			}
		}
	}
	sort.Strings(unknown)
	result := map[string]any{"saved": true, "applied": "live", "chain_count": len(chains), "default": def}
	if len(unknown) > 0 {
		result["unknown_models"] = unknown
	}
	s.writeResp(conn, Response{ID: req.ID, Type: RespResult, Result: result})
}
