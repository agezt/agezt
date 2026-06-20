// SPDX-License-Identifier: MIT

package main

// ChatGPT subscription provider wiring ("Sign in with ChatGPT"). The token store
// + login flow live in kernel/chatgptauth + the control plane; this is the boot/
// reload glue that turns a signed-in token set into a registered governor
// provider speaking the Responses backend (plugins/providers/openairesponses).
//
// The provider is registered ONLY when tokens are present, with AuthMode
// Subscription (which the governor prefers). It is never auto-selected as the
// primary unless the operator sets AGEZT_PROVIDER=chatgpt — honoring the
// no-default-provider rule.

import (
	"context"

	"github.com/agezt/agezt/kernel/agent"
	"github.com/agezt/agezt/kernel/catalog"
	"github.com/agezt/agezt/kernel/chatgptauth"
	"github.com/agezt/agezt/kernel/governor"
	"github.com/agezt/agezt/plugins/providers/openairesponses"
)

// chatgptModels are the models the ChatGPT subscription backend serves via Codex.
var chatgptModels = []string{"gpt-5-codex", "gpt-5", "gpt-5-mini"}

const chatgptDefaultModel = "gpt-5-codex"

// chatgptCatalogEntry is the catalog metadata so ChatGPT shows in Models and its
// model ids are routable. Env is the vault token key, so HasCredentials reflects
// sign-in state; NPM is empty (FamilyUnknown) so the compat loop never tries to
// build it — the dedicated adapter does.
func chatgptCatalogEntry() *catalog.Provider {
	models := map[string]*catalog.Model{}
	for _, id := range chatgptModels {
		models[id] = &catalog.Model{ID: id, Name: id, ToolCall: true, Reasoning: true}
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

// seedChatGPTCatalog adds the chatgpt entry to custom.json once, so it survives
// reloads and appears in Models. Never clobbers an operator's customisation.
func seedChatGPTCatalog(store *catalog.Store) {
	if store == nil {
		return
	}
	if cur, _ := store.Load(); cur != nil {
		if _, ok := cur.Providers["chatgpt"]; ok {
			return
		}
	}
	_, _ = store.UpsertCustomProvider(chatgptCatalogEntry())
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

// buildChatGPTPrimary builds the ChatGPT provider for use as the primary
// (AGEZT_PROVIDER=chatgpt). ok is false when not signed in.
func buildChatGPTPrimary(baseDir, modelOverride string) (prov agent.Provider, desc string, auth governor.AuthMode, ok bool) {
	mgr := chatgptauth.NewManager(baseDir)
	if !mgr.HasTokens() {
		return nil, "", "", false
	}
	model := modelOverride
	if model == "" {
		model = chatgptDefaultModel
	}
	p := openairesponses.New("chatgpt", model, chatgptTokenFn(mgr))
	desc = "chatgpt (Sign in with ChatGPT"
	if email, _ := mgr.Account(); email != "" {
		desc += " — " + email
	}
	desc += ")"
	return p, desc, governor.AuthSubscription, true
}

// registerChatGPTAlternate registers ChatGPT as a model-routable alternate when
// signed in (and not already the primary). replace uses Registry.Replace (reload
// path) vs Register (boot). Returns true when registered.
func registerChatGPTAlternate(reg *governor.Registry, baseDir, primaryName string, replace bool) bool {
	if primaryName == "chatgpt" {
		return false
	}
	mgr := chatgptauth.NewManager(baseDir)
	if !mgr.HasTokens() {
		return false
	}
	info := &governor.ProviderInfo{
		Name:     "chatgpt",
		Provider: openairesponses.New("chatgpt", chatgptDefaultModel, chatgptTokenFn(mgr)),
		AuthMode: governor.AuthSubscription,
		Models:   chatgptModels,
	}
	var err error
	if replace {
		err = reg.Replace(info)
	} else {
		err = reg.Register(info)
	}
	return err == nil
}
