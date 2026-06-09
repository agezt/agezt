// SPDX-License-Identifier: MIT

package controlplane

// Per-task model routing (M703): view and edit the governor's per-task-type
// model fallback CHAINS at runtime. A chain is an ordered list of model ids the
// governor tries in turn for a task type (each routing to its serving provider),
// falling back model→model. Edits apply LIVE (governor.SetTaskModelChains) and
// persist to the config store as AGEZT_TASK_MODEL_CHAINS so they survive restart
// (startup injection + buildGovernor re-parse).

import (
	"fmt"
	"net"
	"sort"
	"strings"

	"github.com/agezt/agezt/internal/brand"
	"github.com/agezt/agezt/kernel/settings"
)

// knownTaskTypes are the routing targets the Routing UI offers rows for — the
// agentic jobs the daemon tags (M703) plus the main chat loop and delegated
// sub-agents. Operators may also configure custom task types.
var knownTaskTypes = []string{
	"chat", "plan", "code", "verify", "summarize", "salience", "distill", "forge", "shadow-eval", "delegate",
}

type chainsGetter interface {
	TaskModelChainsView() map[string][]string
}
type chainsSetter interface {
	SetTaskModelChains(map[string][]string)
}

// handleRoutingGet returns the effective per-task model chains plus the known
// task-type list the UI seeds rows from.
func (s *Server) handleRoutingGet(conn net.Conn, req Request) {
	chains := map[string][]string{}
	if gov, ok := s.k.Provider().(chainsGetter); ok {
		chains = gov.TaskModelChainsView()
	}
	s.writeResp(conn, Response{ID: req.ID, Type: RespResult, Result: map[string]any{
		"task_types": knownTaskTypes,
		"chains":     stringSliceMapToAny(chains),
	}})
}

// handleRoutingSet replaces the per-task model chains. args.chains is an object
// {task: [model, …]}. Applies live and persists to the config store. Model ids
// unknown to the catalog are reported (warn) but not rejected — the catalog can
// lag a freshly-named model.
func (s *Server) handleRoutingSet(conn net.Conn, req Request) {
	raw, ok := req.Args["chains"]
	if !ok {
		s.writeResp(conn, Response{ID: req.ID, Type: RespError, Error: "args.chains required (object {task: [models]})"})
		return
	}
	chains, err := decodeChains(raw)
	if err != nil {
		s.writeResp(conn, Response{ID: req.ID, Type: RespError, Error: err.Error()})
		return
	}

	// Persist to the config store (survives restart via injection + buildGovernor).
	store := settings.NewStore(s.baseDir)
	if err := store.Load(); err != nil {
		s.writeResp(conn, Response{ID: req.ID, Type: RespError, Error: "load config: " + err.Error()})
		return
	}
	envName := brand.EnvPrefix + "TASK_MODEL_CHAINS"
	if spec := encodeChains(chains); spec != "" {
		store.Set(envName, spec)
	} else {
		store.Remove(envName)
	}
	if err := store.Save(); err != nil {
		s.writeResp(conn, Response{ID: req.ID, Type: RespError, Error: "save config: " + err.Error()})
		return
	}

	// Apply live.
	if gov, ok := s.k.Provider().(chainsSetter); ok {
		gov.SetTaskModelChains(chains)
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
	result := map[string]any{"saved": true, "applied": "live", "task_count": len(chains)}
	if len(unknown) > 0 {
		result["unknown_models"] = unknown
	}
	s.writeResp(conn, Response{ID: req.ID, Type: RespResult, Result: result})
}

// decodeChains converts the wire form {task: [models]} into a typed map, dropping
// empty task keys and empty/blank model entries.
func decodeChains(raw any) (map[string][]string, error) {
	m, ok := raw.(map[string]any)
	if !ok {
		return nil, fmt.Errorf("chains must be an object {task: [models]}")
	}
	out := map[string][]string{}
	for task, v := range m {
		task = strings.TrimSpace(task)
		if task == "" {
			continue
		}
		arr, ok := v.([]any)
		if !ok {
			return nil, fmt.Errorf("chains[%q] must be an array of model ids", task)
		}
		models := make([]string, 0, len(arr))
		for _, e := range arr {
			if str, ok := e.(string); ok {
				if str = strings.TrimSpace(str); str != "" {
					models = append(models, str)
				}
			}
		}
		if len(models) > 0 {
			out[task] = models
		}
	}
	return out, nil
}

// encodeChains serialises chains to the AGEZT_TASK_MODEL_CHAINS env spec
// (`task=m1,m2;task2=m3`), sorted for stable output.
func encodeChains(chains map[string][]string) string {
	keys := make([]string, 0, len(chains))
	for k := range chains {
		keys = append(keys, k)
	}
	sort.Strings(keys)
	parts := make([]string, 0, len(keys))
	for _, k := range keys {
		if len(chains[k]) == 0 {
			continue
		}
		parts = append(parts, k+"="+strings.Join(chains[k], ","))
	}
	return strings.Join(parts, ";")
}
