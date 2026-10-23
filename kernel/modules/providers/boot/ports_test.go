// SPDX-License-Identifier: MIT

package boot

// The engine's test seam. The module never imports the wire adapters, so its
// own tests stand them in: fakePorts implements api.Ports (an interface, so the
// compiler refuses to let a port go missing) and records what the engine asked
// of it — the built providers, the wrapped ones, the ChatGPT instructions it
// handed over.
//
// The real adapters are exercised at the composition root, where they are
// bound (cmd/agezt).

import (
	"context"
	"fmt"
	"sort"
	"strings"

	"github.com/agezt/agezt/internal/brand"
	"github.com/agezt/agezt/kernel/contract/llm"
	"github.com/agezt/agezt/kernel/modules/providers/api"
	"github.com/agezt/agezt/kernel/modules/providers/catalog"
)

// fakeProvider is a wire stand-in: it reports a name and answers with a fixed
// text, so a test can assert WHICH provider the engine registered.
type fakeProvider struct {
	name string
	text string
}

func (p *fakeProvider) Name() string { return p.name }

func (p *fakeProvider) Complete(ctx context.Context, _ llm.CompletionRequest) (*llm.CompletionResponse, error) {
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	return &llm.CompletionResponse{Message: llm.Message{Role: llm.RoleAssistant, Content: p.text}}, nil
}

// wrappedProvider is what the fake middleware stack produces. It is a
// DIFFERENT type from fakeProvider on purpose: the engine's tests assert that
// a provider went through the stack by type, exactly as they used to assert
// it was not the raw adapter.
type wrappedProvider struct {
	llm.Provider
}

// fakePorts is the test double for the wire adapters.
type fakePorts struct {
	// supported is the set of catalog families the fake adapter can serve.
	// A nil map serves every family.
	supported map[catalog.Family]bool
	// buildErr fails the build of the named provider id.
	buildErr map[string]error
	// built records the provider ids the engine asked to build, in order.
	built []string
	// wrapped counts the providers that went through the middleware stack.
	wrapped int
	// models is the ChatGPT surface the fake backend serves.
	models []api.ModelInfo
	// chatGPTInstructions records the instructions the engine handed to the
	// ChatGPT adapter.
	chatGPTInstructions map[string]string
	// chatGPTModel records the model the engine asked the ChatGPT adapter for.
	chatGPTModel string
}

// ports returns the fake ports with the given supported-family set.
func ports(supported ...catalog.Family) *fakePorts {
	if supported == nil {
		return &fakePorts{}
	}
	set := map[catalog.Family]bool{}
	for _, f := range supported {
		set[f] = true
	}
	return &fakePorts{supported: set}
}

func (f *fakePorts) Build(entry *catalog.Provider, modelID string, _ func(string) string) (llm.Provider, string, error) {
	if entry == nil {
		return nil, "", fmt.Errorf("fake: nil entry")
	}
	// The real adapter refuses an empty model id — the fake must too, or it
	// would hide the "a catalog entry with no models" failure.
	if modelID == "" {
		return nil, "", fmt.Errorf("fake: model id required")
	}
	// ...and it refuses a model the entry does not serve, exactly like the
	// compat adapter (ErrModelUnknown). Without this the fake would accept the
	// unrepaired cross-provider model and hide the placeholder repair.
	if _, ok := entry.Models[modelID]; !ok {
		return nil, "", fmt.Errorf("fake: model %q not in provider %q", modelID, entry.ID)
	}
	f.built = append(f.built, entry.ID)
	if err := f.buildErr[entry.ID]; err != nil {
		return nil, "", err
	}
	return &fakeProvider{name: entry.ID, text: "fake:" + entry.ID}, modelID, nil
}

func (f *fakePorts) SupportedFamily(family catalog.Family) bool {
	if f.supported == nil {
		return true
	}
	return f.supported[family]
}

func (f *fakePorts) FirstModelID(entry *catalog.Provider) string {
	if entry == nil || len(entry.Models) == 0 {
		return ""
	}
	ids := make([]string, 0, len(entry.Models))
	for id := range entry.Models {
		ids = append(ids, id)
	}
	sort.Strings(ids)
	return ids[0]
}

func (f *fakePorts) DemoEcho() llm.Provider {
	return &fakeProvider{name: "mock", text: "echo"}
}

// Wrap mirrors the real middleware stack's shape: an empty stack returns the
// provider unchanged (that is what agent.Wrap does), a non-empty one hands
// back a DIFFERENT type. The engine's tests tell "wrapped" from "raw" by
// type, exactly as they told it with the real adapter.
func (f *fakePorts) Wrap(get func(string) string) func(llm.Provider) llm.Provider {
	stack := 0
	if get != nil {
		for _, knob := range []string{"GEN_TEMPERATURE", "GEN_TOP_P", "GEN_REASONING_EFFORT", "EXTRACT_REASONING", "SIMULATE_STREAMING"} {
			if strings.TrimSpace(get(brand.EnvPrefix+knob)) != "" {
				stack++
			}
		}
	}
	return func(p llm.Provider) llm.Provider {
		if stack == 0 {
			return p
		}
		f.wrapped++
		return &wrappedProvider{Provider: p}
	}
}

func (f *fakePorts) ChatGPTDiscover(_ api.TokenSource) ([]api.ModelInfo, bool) {
	return f.models, len(f.models) > 0
}

func (f *fakePorts) ChatGPTProvider(_ api.TokenSource, model string, instructions map[string]string) llm.Provider {
	f.chatGPTModel = model
	f.chatGPTInstructions = map[string]string{}
	for k, v := range instructions {
		f.chatGPTInstructions[k] = v
	}
	return &fakeProvider{name: "chatgpt", text: "chatgpt:" + model}
}

func (f *fakePorts) ChatGPTBaseURL() string { return "https://chatgpt.test/v1" }
