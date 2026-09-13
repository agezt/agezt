// SPDX-License-Identifier: MIT
//
// ChatGPT provider-boot: chatgptTokenFn + newChatGPTProvider (the token
// function + factory) + buildChatGPTPrimary + registerChatGPTAlternate
// (the primary + alternate providers).
// Extracted from chatgpt.go during the Day-207 god-file split.
// Public API unchanged.
package providerboot

import (
	"context"

	"github.com/agezt/agezt/kernel/agent"
	"github.com/agezt/agezt/kernel/chatgptauth"
	"github.com/agezt/agezt/kernel/governor"
	"github.com/agezt/agezt/plugins/providers/openairesponses"
)

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
