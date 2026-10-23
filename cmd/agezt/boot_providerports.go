// SPDX-License-Identifier: MIT

package main

// Provider bootstrap glue: the selection/registration/reload logic itself lives
// in the providers module (kernel/modules/providers/boot) — this file only
// assembles the daemon's dependency bundle for it and binds the module's
// ports to the wire adapters that live above it. awschain.go (the credential
// chain builders) stays in cmd/agezt because the AWS/IMDS stages are host
// concerns, not provider bootstrap.
//
// The binding lives here, in the composition root, because a module may not
// import a plugin: the compat family, the offline mock and the ChatGPT
// Responses backend are adapters, and the root is where an adapter is chosen.

import (
	"context"
	"strings"

	"github.com/agezt/agezt/kernel/agent"
	"github.com/agezt/agezt/kernel/contract/llm"
	"github.com/agezt/agezt/kernel/modules/providers/api"
	"github.com/agezt/agezt/kernel/modules/providers/catalog"
	"github.com/agezt/agezt/plugins/providers/compat"
	"github.com/agezt/agezt/plugins/providers/mock"
	"github.com/agezt/agezt/plugins/providers/openairesponses"
)

// providerAdapters binds the providers module's ports to the wire adapters.
//
// The module owns the DECISIONS (which provider runs, which alternates are
// eligible, what the catalog says about a model); the adapters own the WIRE
// (how a completion is actually sent). Keeping the binding here — rather than
// inside the module — is what keeps the module free of upward edges.
type providerAdapters struct{}

func providerPorts() api.Ports { return providerAdapters{} }

// Build constructs the wire provider for a catalog entry — the compat family
// speaks every OpenAI-compatible dialect the catalog knows.
func (providerAdapters) Build(entry *catalog.Provider, modelID string, lookup func(string) string) (llm.Provider, string, error) {
	return compat.Build(entry, modelID, compat.CredLookup(lookup))
}

// SupportedFamily reports whether the compat family can serve a catalog family.
func (providerAdapters) SupportedFamily(family catalog.Family) bool {
	return compat.IsSupportedFamily(family)
}

// FirstModelID picks the model a wire is constructed with when the operator
// pinned none.
func (providerAdapters) FirstModelID(entry *catalog.Provider) string {
	return compat.FirstModelID(entry)
}

// DemoEcho builds the explicit offline demo provider (AGEZT_DEMO_ECHO=1). It is
// the daemon's only mock path and it never answers by itself.
func (providerAdapters) DemoEcho() llm.Provider {
	p := mock.New()
	p.Responder = func(req llm.CompletionRequest) llm.CompletionResponse {
		text := ""
		for i := len(req.Messages) - 1; i >= 0; i-- {
			if req.Messages[i].Role == llm.RoleUser {
				text = strings.TrimSpace(req.Messages[i].Content)
				if text != "" {
					break
				}
			}
		}
		if text == "" {
			text = "ok"
		}
		return mock.FinalText("[echo] " + text)
	}
	return p
}

// Wrap returns the wrapper that applies the operator's middleware stack. The
// middleware itself lives in the runs module; the boot engine asks for it once
// per boot cycle so the primary and every alternate share one stack.
func (providerAdapters) Wrap(get func(string) string) func(llm.Provider) llm.Provider {
	mw := agent.MiddlewareFromEnv(get)
	return func(p llm.Provider) llm.Provider { return agent.Wrap(p, mw...) }
}

// ChatGPTDiscover lists the ChatGPT model surface from the backend and maps it
// into the module's own model row.
func (providerAdapters) ChatGPTDiscover(src api.TokenSource) ([]api.ModelInfo, bool) {
	models, err := openairesponses.ListModels(context.Background(), openairesponses.DefaultBaseURL, chatGPTTokenFn(src))
	if err != nil {
		return nil, false
	}
	return chatGPTModelRows(models), true
}

// chatGPTModelRows maps the backend's reply into the module's own model row.
// The mapping is the crossing between two vocabularies, so it is a named
// function and not a loop buried in the discovery: the fields the module
// routes on (slug, display name, priority, instructions, modalities) must
// survive it, or the catalog entry describes a model that does not exist.
func chatGPTModelRows(models []openairesponses.ModelInfo) []api.ModelInfo {
	out := make([]api.ModelInfo, 0, len(models))
	for _, m := range models {
		out = append(out, api.ModelInfo{
			Slug:             m.Slug,
			DisplayName:      m.DisplayName,
			Description:      m.Description,
			Visibility:       m.Visibility,
			Priority:         m.Priority,
			ContextWindow:    m.ContextWindow,
			MaxContextWindow: m.MaxContextWindow,
			DefaultReasoning: m.DefaultReasoning,
			InputModalities:  m.InputModalities,
			SupportedInAPI:   m.SupportedInAPI,
			ParallelTools:    m.ParallelTools,
			BaseInstructions: m.BaseInstructions,
		})
	}
	return out
}

// ChatGPTProvider builds the Responses adapter with the per-model system
// prompts the module discovered attached.
func (providerAdapters) ChatGPTProvider(src api.TokenSource, model string, instructions map[string]string) llm.Provider {
	p := openairesponses.New("chatgpt", model, chatGPTTokenFn(src))
	p.Instructions = instructions
	return p
}

// ChatGPTBaseURL is the backend base URL recorded in the catalog entry.
func (providerAdapters) ChatGPTBaseURL() string { return openairesponses.DefaultBaseURL }

// chatGPTTokenFn adapts the module's token source to the Responses adapter's
// token function: a forced refresh is the adapter's 401-retry path.
func chatGPTTokenFn(src api.TokenSource) openairesponses.TokenFunc {
	return func(ctx context.Context, force bool) (string, string, error) {
		if force {
			return src.ForceRefresh(ctx)
		}
		return src.Token(ctx)
	}
}
