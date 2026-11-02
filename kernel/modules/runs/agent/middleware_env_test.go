// SPDX-License-Identifier: MIT

package agent

import (
	"testing"

	"github.com/agezt/agezt/internal/brand"
)

// envGet returns a config reader backed by a plain map — an isolated
// environment (unset names read "", exactly like os.Getenv on an unset var).
func envGet(m map[string]string) func(string) string {
	return func(k string) string { return m[k] }
}

// TestMiddlewareFromEnv_EmptyByDefault pins the opt-in rule: with nothing set
// the stack is EMPTY, so every provider is registered unwrapped and behaviour
// is unchanged. A stack that was never empty would change every provider call.
func TestMiddlewareFromEnv_EmptyByDefault(t *testing.T) {
	if mws := MiddlewareFromEnv(envGet(map[string]string{})); len(mws) != 0 {
		t.Fatalf("stack = %d middlewares, want 0 (the stack is opt-in)", len(mws))
	}
}

// TestMiddlewareFromEnv_Defaults pins the sampling-defaults knob: the three
// env vars fill a request that left them unset, and only those.
func TestMiddlewareFromEnv_Defaults(t *testing.T) {
	mws := MiddlewareFromEnv(envGet(map[string]string{
		brand.EnvPrefix + "GEN_TEMPERATURE":      "0.4",
		brand.EnvPrefix + "GEN_TOP_P":            "0.9",
		brand.EnvPrefix + "GEN_REASONING_EFFORT": "high",
	}))
	if len(mws) != 1 {
		t.Fatalf("stack = %d middlewares, want 1 (the defaults)", len(mws))
	}
	if mws[0].Name != "default-params" {
		t.Errorf("Name = %q, want default-params", mws[0].Name)
	}
	req := &CompletionRequest{}
	for _, mw := range mws {
		if mw.TransformRequest != nil {
			mw.TransformRequest(req)
		}
	}
	if req.Params.Temperature == nil || *req.Params.Temperature != 0.4 {
		t.Errorf("Temperature = %v, want 0.4", req.Params.Temperature)
	}
	if req.Params.TopP == nil || *req.Params.TopP != 0.9 {
		t.Errorf("TopP = %v, want 0.9", req.Params.TopP)
	}
	if req.Params.ReasoningEffort != "high" {
		t.Errorf("ReasoningEffort = %q, want high", req.Params.ReasoningEffort)
	}
}

// TestMiddlewareFromEnv_DefaultsSkipMalformed pins that a malformed number
// leaves the default unset instead of failing the boot.
func TestMiddlewareFromEnv_DefaultsSkipMalformed(t *testing.T) {
	mws := MiddlewareFromEnv(envGet(map[string]string{
		brand.EnvPrefix + "GEN_TEMPERATURE": "not-a-number",
	}))
	if len(mws) != 0 {
		t.Fatalf("stack = %d middlewares, want 0 (a malformed value sets no default)", len(mws))
	}
}

// TestMiddlewareFromEnv_ExtractReasoning pins the reasoning-extraction knob and
// the "on" spellings it accepts.
func TestMiddlewareFromEnv_ExtractReasoning(t *testing.T) {
	for _, on := range []string{"1", "on", "ON", "true", "yes"} {
		mws := MiddlewareFromEnv(envGet(map[string]string{brand.EnvPrefix + "EXTRACT_REASONING": on}))
		if len(mws) != 1 || mws[0].Name != "extract-reasoning" {
			t.Fatalf("%q: stack = %v, want the extract-reasoning middleware", on, mws)
		}
	}
	// An explicit off must not build the stack.
	mws := MiddlewareFromEnv(envGet(map[string]string{brand.EnvPrefix + "EXTRACT_REASONING": "off"}))
	if len(mws) != 0 {
		t.Fatalf("stack = %d middlewares, want 0 when the knob is off", len(mws))
	}
}

// TestMiddlewareFromEnv_SimulateStreaming pins the streaming-simulation knob.
func TestMiddlewareFromEnv_SimulateStreaming(t *testing.T) {
	mws := MiddlewareFromEnv(envGet(map[string]string{brand.EnvPrefix + "SIMULATE_STREAMING": "on"}))
	if len(mws) != 1 || !mws[0].SynthesizeStream {
		t.Fatalf("stack = %v, want the simulate-streaming middleware", mws)
	}
}

// TestMiddlewareFromEnv_NilGetUsesTheProcessEnv pins the nil default: with no
// reader the process environment is the source (the daemon's Config Center
// injects operator edits into the process env before boot).
func TestMiddlewareFromEnv_NilGetUsesTheProcessEnv(t *testing.T) {
	t.Setenv(brand.EnvPrefix+"GEN_TEMPERATURE", "0.25")
	mws := MiddlewareFromEnv(nil)
	if len(mws) != 1 {
		t.Fatalf("stack = %d middlewares, want 1 (the process env knob is set)", len(mws))
	}
	req := &CompletionRequest{}
	for _, mw := range mws {
		if mw.TransformRequest != nil {
			mw.TransformRequest(req)
		}
	}
	if req.Params.Temperature == nil || *req.Params.Temperature != 0.25 {
		t.Errorf("Temperature = %v, want 0.25 from the process env", req.Params.Temperature)
	}
}
