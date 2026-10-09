// SPDX-License-Identifier: MIT

// Package routing owns the operator's model routing controls: the governor's
// per-task model fallback chains (M703) with the journal's record of them
// firing (M706), and the registry of named reusable chains referenced as
// "@name" (M963) with what references each one (M964). Edits apply live and
// persist to the config store so they survive a restart.
package routing

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"reflect"
	"regexp"
	"sort"
	"strings"

	"github.com/agezt/agezt/internal/brand"
	"github.com/agezt/agezt/internal/strutil"
	"github.com/agezt/agezt/kernel/app"
	"github.com/agezt/agezt/kernel/contract/opapi"
	"github.com/agezt/agezt/kernel/event"
	"github.com/agezt/agezt/kernel/platform/schema"
)

// ChainPrefix marks a model slot that names a chain, as the governor reads it.
const ChainPrefix = "@"

// TaskTypes are the routing targets the Routing view offers rows for: the
// agentic jobs the daemon tags plus the main chat loop and delegated
// sub-agents. Operators may also configure custom task types.
var TaskTypes = []string{
	"chat", "plan", "code", "verify", "summarize", "salience", "distill", "forge", "shadow-eval", "delegate",
}

// chainName constrains chain names to a slug the "@name" token and the console
// round-trip unambiguously.
var chainName = regexp.MustCompile(`^[a-z0-9][a-z0-9-]*$`)

// Agent is the part of a roster profile that can reference a chain.
type Agent struct {
	Slug      string
	Model     string
	Fallbacks []string
}

// Store is the config store edits persist to.
type Store interface {
	Load() error
	Set(name, value string)
	Remove(name string) bool
	Save() error
}

// Ports reads and changes the primary kernel. Each governor read reports false,
// and each governor write does nothing, when the provider does not carry that
// registry; Models returns a lookup over one catalog snapshot.
type Ports struct {
	TaskChains    func() (map[string][]string, bool)
	SetTaskChains func(map[string][]string)
	Named         func() (map[string][]string, string, bool)
	SetNamed      func(map[string][]string, string)
	Agents        func() []Agent
	Journal       func(visit func(*event.Event))
	Store         func() Store
	Models        func() func(model string) bool
}

type Service struct{ ports Ports }

func New(ports Ports) *Service { return &Service{ports: ports} }

// wireChains is a chain map as the wire shows it: an object, never null, whose
// lists are arrays, never null.
func wireChains(in map[string][]string) map[string][]string {
	out := make(map[string][]string, len(in))
	for k, v := range in {
		if v == nil {
			v = []string{}
		}
		out[k] = v
	}
	return out
}

// Activity is one task type's model-chain fallbacks: how many fired and the
// most recent hop.
type Activity struct {
	Fallbacks  int    `json:"fallbacks"`
	LastFailed string `json:"last_failed"`
	LastNext   string `json:"last_next"`
	LastReason string `json:"last_reason"`
	LastMS     int64  `json:"last_ms"`
}

type RoutingRequest struct{}

type RoutingOutput struct {
	TaskTypes []string            `json:"task_types"`
	Chains    map[string][]string `json:"chains"`
	Activity  map[string]Activity `json:"activity"`
}

// activity folds the journal's model-chain fallback events by task type, in
// sequence order so the last hop kept is the newest. Provider-scope fallbacks
// belong to the provider stats and are skipped, as is any payload that does not
// decode.
func (s *Service) activity() map[string]Activity {
	byTask := map[string]*Activity{}
	s.ports.Journal(func(e *event.Event) {
		if e.Kind != event.KindProviderFallback {
			return
		}
		var p struct {
			FailedModel string `json:"failed_model"`
			NextModel   string `json:"next_model"`
			Reason      string `json:"reason"`
			Scope       string `json:"scope"`
			TaskType    string `json:"task_type"`
		}
		if json.Unmarshal(e.Payload, &p) != nil || p.Scope != "model-chain" {
			return
		}
		task := p.TaskType
		if task == "" {
			task = "(unknown)"
		}
		a := byTask[task]
		if a == nil {
			a = &Activity{}
			byTask[task] = a
		}
		a.Fallbacks++
		a.LastFailed, a.LastNext, a.LastMS = p.FailedModel, p.NextModel, e.TSUnixMS
		if p.Reason != "" {
			a.LastReason = strutil.Ellipsis(p.Reason, 160, "…")
		}
	})
	out := make(map[string]Activity, len(byTask))
	for task, a := range byTask {
		out[task] = *a
	}
	return out
}

// Routing returns the effective per-task model chains, the task types the view
// seeds rows from and each task's fallback activity.
func (s *Service) Routing(_ context.Context, _ RoutingRequest) (RoutingOutput, error) {
	chains, _ := s.ports.TaskChains()
	return RoutingOutput{TaskTypes: TaskTypes, Chains: wireChains(chains), Activity: s.activity()}, nil
}

// decodeChains reads the wire form {key: [models]}, trimming keys and models
// and dropping blank keys, non-string and blank models, and keys left empty.
func decodeChains(raw json.RawMessage) (map[string][]string, error) {
	var v any
	_ = json.Unmarshal(raw, &v)
	m, ok := v.(map[string]any)
	if !ok {
		return nil, fmt.Errorf("chains must be an object {task: [models]}")
	}
	out := map[string][]string{}
	for key, v := range m {
		key = strings.TrimSpace(key)
		if key == "" {
			continue
		}
		arr, ok := v.([]any)
		if !ok {
			return nil, fmt.Errorf("chains[%q] must be an array of model ids", key)
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
			out[key] = models
		}
	}
	return out, nil
}

// encodeChains renders chains as the env spec `key=m1,m2;key2=m3`, sorted.
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

// persist writes each env value, removing it when blank, and saves the store.
func (s *Service) persist(values [][2]string) error {
	store := s.ports.Store()
	if err := store.Load(); err != nil {
		return errors.New("load config: " + err.Error())
	}
	for _, kv := range values {
		if kv[1] != "" {
			store.Set(kv[0], kv[1])
		} else {
			store.Remove(kv[0])
		}
	}
	if err := store.Save(); err != nil {
		return errors.New("save config: " + err.Error())
	}
	return nil
}

// unknownModels lists, sorted, the models the catalog does not know. They are
// reported, never refused: the catalog can lag a freshly named model.
func (s *Service) unknownModels(chains map[string][]string) []string {
	known := s.ports.Models()
	var unknown []string
	seen := map[string]bool{}
	for _, models := range chains {
		for _, m := range models {
			if seen[m] {
				continue
			}
			seen[m] = true
			if !known(m) {
				unknown = append(unknown, m)
			}
		}
	}
	sort.Strings(unknown)
	return unknown
}

type RoutingSetRequest struct {
	Chains json.RawMessage `json:"chains,omitempty"`
}

type RoutingSetOutput struct {
	Saved         bool     `json:"saved"`
	Applied       string   `json:"applied"`
	TaskCount     int      `json:"task_count"`
	UnknownModels []string `json:"unknown_models,omitempty"`
}

// SetRouting replaces the per-task model chains, persists them as
// AGEZT_TASK_MODEL_CHAINS and applies them live.
func (s *Service) SetRouting(_ context.Context, in RoutingSetRequest) (RoutingSetOutput, error) {
	if len(in.Chains) == 0 {
		return RoutingSetOutput{}, errors.New("args.chains required (object {task: [models]})")
	}
	chains, err := decodeChains(in.Chains)
	if err != nil {
		return RoutingSetOutput{}, err
	}
	if err := s.persist([][2]string{{brand.EnvPrefix + "TASK_MODEL_CHAINS", encodeChains(chains)}}); err != nil {
		return RoutingSetOutput{}, err
	}
	s.ports.SetTaskChains(chains)
	return RoutingSetOutput{Saved: true, Applied: "live", TaskCount: len(chains), UnknownModels: s.unknownModels(chains)}, nil
}

type ChainsRequest struct{}

// Usage is what references one known chain. The usage map also carries, under
// "__dangling__", the sorted names referenced but not defined.
type Usage struct {
	Agents  []string `json:"agents,omitempty"`
	Tasks   []string `json:"tasks,omitempty"`
	Default bool     `json:"default,omitempty"`
}

type ChainsOutput struct {
	Chains  map[string][]string `json:"chains"`
	Default string              `json:"default"`
	Usage   map[string]any      `json:"usage"`
}

// dedupeSorted returns the unique values, sorted.
func dedupeSorted(in []string) []string {
	seen := map[string]struct{}{}
	out := make([]string, 0, len(in))
	for _, v := range in {
		if _, ok := seen[v]; ok {
			continue
		}
		seen[v] = struct{}{}
		out = append(out, v)
	}
	sort.Strings(out)
	return out
}

// usage scans agent models and fallbacks and the per-task chains for "@name"
// references. A defined chain gets its referencing agents and task types and
// whether it is the default, which is listed even when nothing references it;
// names referenced but not defined are listed under "__dangling__".
func (s *Service) usage(chains map[string][]string, def string) map[string]any {
	type use struct{ agents, tasks []string }
	byChain := map[string]*use{}
	dangling := map[string]*use{}
	get := func(name string) *use {
		m := byChain
		if _, ok := chains[name]; !ok {
			m = dangling
		}
		u := m[name]
		if u == nil {
			u = &use{}
			m[name] = u
		}
		return u
	}
	for _, a := range s.ports.Agents() {
		for _, ref := range append([]string{a.Model}, a.Fallbacks...) {
			if name, ok := strings.CutPrefix(ref, ChainPrefix); ok && name != "" {
				u := get(name)
				u.agents = append(u.agents, a.Slug)
			}
		}
	}
	if tasks, ok := s.ports.TaskChains(); ok {
		for task, models := range tasks {
			for _, m := range models {
				if name, ok := strings.CutPrefix(m, ChainPrefix); ok && name != "" {
					u := get(name)
					u.tasks = append(u.tasks, task)
				}
			}
		}
	}
	out := make(map[string]any, len(byChain)+len(dangling)+1)
	for name, u := range byChain {
		entry := Usage{Default: name == def}
		if len(u.agents) > 0 {
			entry.Agents = dedupeSorted(u.agents)
		}
		if len(u.tasks) > 0 {
			entry.Tasks = dedupeSorted(u.tasks)
		}
		out[name] = entry
	}
	if def != "" {
		if _, ok := out[def]; !ok {
			if _, known := chains[def]; known {
				out[def] = Usage{Default: true}
			}
		}
	}
	if len(dangling) > 0 {
		names := make([]string, 0, len(dangling))
		for name := range dangling {
			names = append(names, name)
		}
		sort.Strings(names)
		out["__dangling__"] = names
	}
	return out
}

// Chains returns the named chains, the default chain name and what references
// each chain.
func (s *Service) Chains(_ context.Context, _ ChainsRequest) (ChainsOutput, error) {
	chains, def, ok := s.ports.Named()
	if !ok {
		chains, def = map[string][]string{}, ""
	}
	return ChainsOutput{Chains: wireChains(chains), Default: def, Usage: s.usage(chains, def)}, nil
}

type ChainsSetRequest struct {
	Chains  json.RawMessage `json:"chains,omitempty"`
	Default json.RawMessage `json:"default,omitempty"`
}

type ChainsSetOutput struct {
	Saved         bool     `json:"saved"`
	Applied       string   `json:"applied"`
	ChainCount    int      `json:"chain_count"`
	Default       string   `json:"default"`
	UnknownModels []string `json:"unknown_models,omitempty"`
}

// lenientString is a trimmed string argument; any other value reads as "".
func lenientString(raw json.RawMessage) string {
	var v any
	_ = json.Unmarshal(raw, &v)
	str, _ := v.(string)
	return strings.TrimSpace(str)
}

// SetChains replaces the named chains and the default chain, persists them as
// AGEZT_FALLBACK_CHAINS and AGEZT_DEFAULT_CHAIN and applies them live. Names
// must be slugs, chains may hold only real model ids (chains never reference
// chains, so there is no cycle to resolve) and the default must name a chain.
func (s *Service) SetChains(_ context.Context, in ChainsSetRequest) (ChainsSetOutput, error) {
	if len(in.Chains) == 0 {
		return ChainsSetOutput{}, errors.New("args.chains required (object {name: [models]})")
	}
	chains, err := decodeChains(in.Chains)
	if err != nil {
		return ChainsSetOutput{}, err
	}
	for name, models := range chains {
		if !chainName.MatchString(name) {
			return ChainsSetOutput{}, fmt.Errorf("invalid chain name %q (use lower-case letters, digits, dashes)", name)
		}
		for _, m := range models {
			if strings.HasPrefix(m, ChainPrefix) {
				return ChainsSetOutput{}, fmt.Errorf("chain %q model %q: chains may not reference other chains", name, m)
			}
		}
	}
	def := lenientString(in.Default)
	if def != "" {
		if _, ok := chains[def]; !ok {
			return ChainsSetOutput{}, fmt.Errorf("default chain %q is not one of the defined chains", def)
		}
	}
	if err := s.persist([][2]string{{brand.EnvPrefix + "FALLBACK_CHAINS", encodeChains(chains)}, {brand.EnvPrefix + "DEFAULT_CHAIN", def}}); err != nil {
		return ChainsSetOutput{}, err
	}
	s.ports.SetNamed(chains, def)
	return ChainsSetOutput{Saved: true, Applied: "live", ChainCount: len(chains), Default: def, UnknownModels: s.unknownModels(chains)}, nil
}

// Operations declares the two read-only views and the two audited edits on
// their Web UI routes; all four are operator-only.
func Operations(provider func(context.Context) *Service) ([]app.Operation, error) {
	if provider == nil {
		return nil, errors.New("routing provider required")
	}
	outputs := map[string]json.RawMessage{}
	for name, typ := range map[string]reflect.Type{
		"routing_get": reflect.TypeFor[RoutingOutput](), "routing_set": reflect.TypeFor[RoutingSetOutput](),
		"chains_get": reflect.TypeFor[ChainsOutput](), "chains_set": reflect.TypeFor[ChainsSetOutput](),
	} {
		out, err := schema.FromType(typ, false)
		if err != nil {
			return nil, err
		}
		outputs[name] = out
	}
	spec := func(name, method, path string, read bool, input string) opapi.Spec {
		s := opapi.Spec{Name: name, ReadOnly: read, OutputSchema: outputs[name], Authz: opapi.PrimaryOnly, Tenancy: opapi.Primary, AllowUnknownInput: true, HTTP: opapi.HTTP{Method: method, Path: path}}
		if input != "" {
			s.InputSchema = json.RawMessage(input)
		}
		return s
	}
	routingGet, err := app.NewOperation(spec("routing_get", "GET", "/api/routing", true, ""), func(ctx context.Context, in RoutingRequest) (RoutingOutput, error) {
		return provider(ctx).Routing(ctx, in)
	})
	if err != nil {
		return nil, err
	}
	routingSet, err := app.NewOperation(spec("routing_set", "POST", "/api/routing/set", false, `{"type":"object","additionalProperties":true,"properties":{"chains":{}}}`), func(ctx context.Context, in RoutingSetRequest) (RoutingSetOutput, error) {
		return provider(ctx).SetRouting(ctx, in)
	})
	if err != nil {
		return nil, err
	}
	chainsGet, err := app.NewOperation(spec("chains_get", "GET", "/api/chains", true, ""), func(ctx context.Context, in ChainsRequest) (ChainsOutput, error) {
		return provider(ctx).Chains(ctx, in)
	})
	if err != nil {
		return nil, err
	}
	chainsSet, err := app.NewOperation(spec("chains_set", "POST", "/api/chains/set", false, `{"type":"object","additionalProperties":true,"properties":{"chains":{},"default":{}}}`), func(ctx context.Context, in ChainsSetRequest) (ChainsSetOutput, error) {
		return provider(ctx).SetChains(ctx, in)
	})
	if err != nil {
		return nil, err
	}
	return []app.Operation{routingGet, routingSet, chainsGet, chainsSet}, nil
}
