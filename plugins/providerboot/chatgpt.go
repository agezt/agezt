// SPDX-License-Identifier: MIT

package providerboot

// ChatGPT subscription provider wiring ("Sign in with ChatGPT"). The token store
// + login flow live in kernel/chatgptauth + the control plane; this is the boot/
// reload glue that turns a signed-in token set into a registered governor
// provider speaking the Responses backend (plugins/providers/openairesponses).
//
// The provider is registered ONLY when tokens are present, with AuthMode
// Subscription (which the governor prefers). It is never auto-selected as the
// primary unless the operator sets AGEZT_PROVIDER=chatgpt — honoring the
// no-default-provider rule.
//
// The served model set is DISCOVERED, not hardcoded: OpenAI ships Codex models
// on its own cadence, so a constant here goes stale and leaves operators routing
// to ids the backend no longer knows. See resolveChatGPTModels for the three
// sources and their precedence.

import (
	"context"
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"time"

	"github.com/agezt/agezt/kernel/agent"
	"github.com/agezt/agezt/kernel/catalog"
	"github.com/agezt/agezt/kernel/chatgptauth"
	"github.com/agezt/agezt/kernel/governor"
	"github.com/agezt/agezt/plugins/providers/openairesponses"
)

// chatgptFallbackModels is the last-resort model set, used only when both
// discovery and the Codex CLI cache are unavailable (offline first boot, not
// signed in yet). It is a SNAPSHOT, not the source of truth — treat a stale
// entry here as cosmetic, since discovery replaces it as soon as the operator
// signs in.
var chatgptFallbackModels = []string{
	"gpt-5.6-sol",
	"gpt-5.6-terra",
	"gpt-5.6-luna",
	"gpt-5.5",
	"gpt-5.4",
	"gpt-5.4-mini",
	"gpt-5.3-codex-spark",
}

// chatgptFallbackDefault is the model used when the operator sets no override
// and discovery has nothing to say.
const chatgptFallbackDefault = "gpt-5.6-sol"

// Model-set source labels, in descending authority.
const (
	chatgptSourceBackend = "backend"
	chatgptSourceCache   = "codex-cli-cache"
	chatgptSourceBuiltin = "builtin"
)

// chatgptCacheTTL is how long a discovered model set is reused. Reload runs on
// every config change; without this each one would pay a network round trip.
const chatgptCacheTTL = 6 * time.Hour

// chatgptModelSet is the resolved model surface for the ChatGPT provider.
type chatgptModelSet struct {
	// IDs are the operator-facing model ids, most-preferred first.
	IDs []string
	// Default is the model used when nothing is pinned (IDs[0], or the builtin).
	Default string
	// Info carries the per-model metadata used to build catalog entries.
	Info map[string]openairesponses.ModelInfo
	// Instructions maps model id → its backend-served system prompt.
	Instructions map[string]string
	// Source records which of the three sources answered.
	Source string
}

// Authoritative reports whether the set came from the backend (directly or via
// the CLI's cache of the same reply) rather than the builtin snapshot. Only an
// authoritative set may overwrite an already-persisted catalog entry.
func (s chatgptModelSet) Authoritative() bool {
	return s.Source == chatgptSourceBackend || s.Source == chatgptSourceCache
}

// chatgptModelCache memoizes the last authoritative model set. Builtin results
// are never cached, so the first reload after sign-in still discovers.
var chatgptModelCache struct {
	mu  sync.Mutex
	set chatgptModelSet
	at  time.Time
}

// resolveChatGPTModels returns the model set, trying in order:
//
//  1. the backend's /models endpoint (authoritative; needs tokens),
//  2. the Codex CLI's models_cache.json — the same reply, cached by the CLI,
//     which covers an offline daemon on a host where `codex` has run,
//  3. the builtin snapshot.
//
// It never fails: a discovery error degrades to the next source, per the
// boot-resilient-config rule (a recoverable mismatch must warn and degrade, not
// block boot).
func resolveChatGPTModels(mgr *chatgptauth.Manager) chatgptModelSet {
	chatgptModelCache.mu.Lock()
	if cached := chatgptModelCache.set; cached.Authoritative() && time.Since(chatgptModelCache.at) < chatgptCacheTTL {
		chatgptModelCache.mu.Unlock()
		return cached
	}
	chatgptModelCache.mu.Unlock()

	set, ok := discoverChatGPTModels(mgr)
	if !ok {
		set, ok = chatgptModelsFromCLICache(chatgptCLICachePath())
	}
	if !ok {
		return builtinChatGPTModelSet()
	}
	chatgptModelCache.mu.Lock()
	chatgptModelCache.set, chatgptModelCache.at = set, time.Now()
	chatgptModelCache.mu.Unlock()
	return set
}

// discoverChatGPTModels asks the backend directly. ok is false when not signed
// in or the call fails.
func discoverChatGPTModels(mgr *chatgptauth.Manager) (chatgptModelSet, bool) {
	if mgr == nil || !mgr.HasTokens() {
		return chatgptModelSet{}, false
	}
	models, err := openairesponses.ListModels(context.Background(), openairesponses.DefaultBaseURL, chatgptTokenFn(mgr))
	if err != nil {
		return chatgptModelSet{}, false
	}
	return chatgptModelSetFrom(models, chatgptSourceBackend)
}

// chatgptCLICachePath is models_cache.json alongside the Codex CLI's auth.json.
func chatgptCLICachePath() string {
	auth := chatgptauth.DefaultCodexAuthPath()
	if auth == "" {
		return ""
	}
	return filepath.Join(filepath.Dir(auth), "models_cache.json")
}

// chatgptModelsFromCLICache reads a Codex CLI models cache file.
func chatgptModelsFromCLICache(path string) (chatgptModelSet, bool) {
	if strings.TrimSpace(path) == "" {
		return chatgptModelSet{}, false
	}
	raw, err := os.ReadFile(path) // #nosec G304 -- a fixed path under the operator's own Codex home
	if err != nil {
		return chatgptModelSet{}, false
	}
	var payload struct {
		Models []openairesponses.ModelInfo `json:"models"`
	}
	if json.Unmarshal(raw, &payload) != nil {
		return chatgptModelSet{}, false
	}
	return chatgptModelSetFrom(openairesponses.ParseModels(payload.Models), chatgptSourceCache)
}

// chatgptModelSetFrom projects discovered models into a set, keeping only the
// entries meant for an operator-facing picker. ok is false when none qualify.
func chatgptModelSetFrom(models []openairesponses.ModelInfo, source string) (chatgptModelSet, bool) {
	set := chatgptModelSet{
		Info:         map[string]openairesponses.ModelInfo{},
		Instructions: map[string]string{},
		Source:       source,
	}
	for _, m := range models {
		// Hidden entries still carry usable instructions (an operator may pin one
		// by id), so record them — but keep them out of the picker.
		if s := strings.TrimSpace(m.BaseInstructions); s != "" {
			set.Instructions[m.Slug] = s
		}
		if !m.Listed() {
			continue
		}
		set.IDs = append(set.IDs, m.Slug)
		set.Info[m.Slug] = m
	}
	if len(set.IDs) == 0 {
		return chatgptModelSet{}, false
	}
	set.Default = set.IDs[0] // ParseModels ordered by the backend's own priority
	return set, true
}

// builtinChatGPTModelSet is the offline snapshot.
func builtinChatGPTModelSet() chatgptModelSet {
	set := chatgptModelSet{
		IDs:          append([]string(nil), chatgptFallbackModels...),
		Default:      chatgptFallbackDefault,
		Info:         map[string]openairesponses.ModelInfo{},
		Instructions: map[string]string{},
		Source:       chatgptSourceBuiltin,
	}
	for _, id := range set.IDs {
		set.Info[id] = openairesponses.ModelInfo{Slug: id, DisplayName: id}
	}
	return set
}

// chatgptCatalogEntry is the catalog metadata so ChatGPT shows in Models and its
// model ids are routable. Env is the vault token key, so HasCredentials reflects
// sign-in state; NPM is empty (FamilyUnknown) so the compat loop never tries to
// build it — the dedicated adapter does.
func chatgptCatalogEntry(set chatgptModelSet) *catalog.Provider {
	models := map[string]*catalog.Model{}
	for _, id := range set.IDs {
		info := set.Info[id]
		name := strings.TrimSpace(info.DisplayName)
		if name == "" {
			name = id
		}
		m := &catalog.Model{ID: id, Name: name, ToolCall: true, Reasoning: true}
		if ctxWindow := info.MaxContextWindow; ctxWindow > 0 {
			m.Limit.Context = ctxWindow
		} else if info.ContextWindow > 0 {
			m.Limit.Context = info.ContextWindow
		}
		if len(info.InputModalities) > 0 {
			m.Modalities.Input = append([]string(nil), info.InputModalities...)
			m.Attachment = m.SupportsVision()
		}
		models[id] = m
	}
	return &catalog.Provider{
		ID:     "chatgpt",
		Name:   "ChatGPT (Sign in with ChatGPT)",
		Env:    []string{chatgptauth.VaultKey},
		API:    openairesponses.DefaultBaseURL,
		Doc:    "https://chatgpt.com",
		Models: models,
	}
}

// SeedChatGPTCatalog keeps the chatgpt entry in custom.json in step with what
// the backend actually serves, so it survives reloads and appears in Models.
//
// It seeds when absent, and REFRESHES an existing entry whenever the model set
// is authoritative — the seed-once-and-never-update behaviour it replaces is
// what left signed-in installs pinned to a model list frozen at first boot. A
// builtin (offline) set never overwrites an entry already on disk.
func SeedChatGPTCatalog(store *catalog.Store, baseDir string) {
	writeChatGPTEntry(store, resolveChatGPTModels(chatgptauth.NewManager(baseDir)))
}

// SyncChatGPTCatalog refreshes the catalog entry after a sign-in and reports the
// served model surface (ids most-preferred first + the default to pin). The
// control plane calls this through a Deps hook — the kernel never imports the
// provider layer.
//
// It is called on every sign-in status poll, so it must stay cheap: discovery is
// memoized and the catalog write is skipped when the entry already matches.
func SyncChatGPTCatalog(store *catalog.Store, baseDir string) (models []string, defaultModel string) {
	// Signing in unlocks the backend source, so drop a memo that came from the
	// weaker CLI-cache one. A backend result is already the best available and
	// stays memoized — otherwise polling would mean one request per poll.
	chatgptModelCache.mu.Lock()
	if chatgptModelCache.set.Source != chatgptSourceBackend {
		chatgptModelCache.set, chatgptModelCache.at = chatgptModelSet{}, time.Time{}
	}
	chatgptModelCache.mu.Unlock()

	set := resolveChatGPTModels(chatgptauth.NewManager(baseDir))
	writeChatGPTEntry(store, set)
	return set.IDs, set.Default
}

// writeChatGPTEntry persists the entry when it would actually change anything: a
// builtin (offline) set never overwrites what is already on disk, and a set that
// matches the stored model ids is a no-op.
func writeChatGPTEntry(store *catalog.Store, set chatgptModelSet) {
	if store == nil {
		return
	}
	cur, _ := store.Load()
	var existing *catalog.Provider
	if cur != nil {
		existing = cur.Providers["chatgpt"]
	}
	if existing != nil {
		if !set.Authoritative() {
			return // nothing better to say than what's already stored
		}
		if sameModelIDs(existing.Models, set.IDs) {
			return
		}
	}
	_, _ = store.UpsertCustomProvider(chatgptCatalogEntry(set))
}

// sameModelIDs reports whether a stored model map holds exactly ids.
func sameModelIDs(stored map[string]*catalog.Model, ids []string) bool {
	if len(stored) != len(ids) {
		return false
	}
	for _, id := range ids {
		if stored[id] == nil {
			return false
		}
	}
	return true
}

// chatgptTokenFn adapts a token Manager to the adapter's TokenFunc.
func chatgptTokenFn(mgr *chatgptauth.Manager) openairesponses.TokenFunc {
	return func(ctx context.Context, force bool) (string, string, error) {
		if force {
			return mgr.ForceRefresh(ctx)
		}
		return mgr.Token(ctx)
	}
}

// newChatGPTProvider builds the Responses adapter with the discovered per-model
// system prompts attached.
func newChatGPTProvider(mgr *chatgptauth.Manager, model string, set chatgptModelSet) *openairesponses.Provider {
	p := openairesponses.New("chatgpt", model, chatgptTokenFn(mgr))
	p.Instructions = set.Instructions
	return p
}

// buildChatGPTPrimary builds the ChatGPT provider for use as the primary
// (AGEZT_PROVIDER=chatgpt). ok is false when not signed in.
func buildChatGPTPrimary(baseDir, modelOverride string) (prov agent.Provider, desc string, auth governor.AuthMode, ok bool) {
	mgr := chatgptauth.NewManager(baseDir)
	if !mgr.HasTokens() {
		return nil, "", "", false
	}
	set := resolveChatGPTModels(mgr)
	model := modelOverride
	if model == "" {
		model = set.Default
	}
	p := newChatGPTProvider(mgr, model, set)
	desc = "chatgpt (Sign in with ChatGPT"
	if email, _ := mgr.Account(); email != "" {
		desc += " — " + email
	}
	desc += ")"
	return p, desc, governor.AuthSubscription, true
}

// registerChatGPTAlternate registers ChatGPT as a model-routable alternate when
// signed in (and not already the primary). replace uses Registry.Replace (reload
// path) vs Register (boot). The provider is wrapped in the shared M997
// middleware stack, same as every other registered provider. Returns true when
// registered.
func registerChatGPTAlternate(reg *governor.Registry, baseDir, primaryName string, replace bool, mw []agent.Middleware) bool {
	if primaryName == "chatgpt" {
		return false
	}
	mgr := chatgptauth.NewManager(baseDir)
	if !mgr.HasTokens() {
		return false
	}
	set := resolveChatGPTModels(mgr)
	info := &governor.ProviderInfo{
		Name:     "chatgpt",
		Provider: agent.Wrap(newChatGPTProvider(mgr, set.Default, set), mw...),
		AuthMode: governor.AuthSubscription,
		Models:   set.IDs,
	}
	var err error
	if replace {
		err = reg.Replace(info)
	} else {
		err = reg.Register(info)
	}
	return err == nil
}
