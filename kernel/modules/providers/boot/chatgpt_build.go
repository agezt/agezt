package boot

// Provenance: SPDX-License-Identifier: MIT ChatGPT provider-boot:
//             newChatGPTProvider (the factory) + buildChatGPTPrimary +
//             registerChatGPTAlternate (the primary + alternate providers).
//             Extracted from chatgpt.go during the Day-207 god-file split.
//             Public API unchanged.
//
// The token function and the Responses adapter itself live above the module:
// the module owns the token lifecycle and the catalog surface, the adapter
// owns the wire. They meet at api.Ports.

import (
	"github.com/agezt/agezt/kernel/contract/llm"
	"github.com/agezt/agezt/kernel/governor"
	"github.com/agezt/agezt/kernel/modules/providers/chatgptauth"
)

// newChatGPTProvider builds the Responses adapter with the discovered per-model
// system prompts attached.
func newChatGPTProvider(d Deps, mgr *chatgptauth.Manager, model string, set chatgptModelSet) llm.Provider {
	return d.Ports.ChatGPTProvider(mgr, model, set.Instructions)
}

// buildChatGPTPrimary builds the ChatGPT provider for use as the primary
// (AGEZT_PROVIDER=chatgpt). ok is false when not signed in.
func buildChatGPTPrimary(d Deps, modelOverride string) (prov llm.Provider, desc string, auth governor.AuthMode, ok bool) {
	mgr := chatgptauth.NewManager(d.BaseDir)
	if !mgr.HasTokens() {
		return nil, "", "", false
	}
	set := resolveChatGPTModels(d.Ports, mgr)
	model := modelOverride
	if model == "" {
		model = set.Default
	}
	p := newChatGPTProvider(d, mgr, model, set)
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
func registerChatGPTAlternate(reg *governor.Registry, d Deps, primaryName string, replace bool, wrap func(llm.Provider) llm.Provider) bool {
	if primaryName == "chatgpt" {
		return false
	}
	mgr := chatgptauth.NewManager(d.BaseDir)
	if !mgr.HasTokens() {
		return false
	}
	set := resolveChatGPTModels(d.Ports, mgr)
	info := &governor.ProviderInfo{
		Name:     "chatgpt",
		Provider: wrap(newChatGPTProvider(d, mgr, set.Default, set)),
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
