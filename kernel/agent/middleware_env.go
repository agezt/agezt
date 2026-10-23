// SPDX-License-Identifier: MIT

package agent

import (
	"os"
	"strconv"
	"strings"

	"github.com/agezt/agezt/internal/brand"
	"github.com/agezt/agezt/kernel/contract/llm"
)

// MiddlewareFromEnv builds the opt-in provider middleware stack from the
// environment (M997). It is empty by default, so every provider is registered
// unwrapped and behaviour is unchanged. Operators opt in to:
//   - DefaultParams: AGEZT_GEN_TEMPERATURE / AGEZT_GEN_TOP_P / AGEZT_GEN_REASONING_EFFORT
//     supply per-call sampling defaults filled in only where a request left them unset.
//   - ExtractReasoning: AGEZT_EXTRACT_REASONING=on pulls inline <think>…</think> out of
//     the answer into ReasoningContent (for inline-reasoning models on OpenAI-compatible /
//     Ollama gateways that don't use a dedicated reasoning field).
//   - SimulateStreaming: AGEZT_SIMULATE_STREAMING=on lets non-streaming providers present
//     a single-chunk stream for a uniform UI.
//
// The stack belongs here, next to the middlewares it builds: it is agent-side
// policy about how a provider call is made, not provider selection. The
// providers module asks for it through its ports, so a module never reaches
// into this one.
//
// get is nil-defaulted to os.Getenv.
func MiddlewareFromEnv(get func(string) string) []Middleware {
	if get == nil {
		get = os.Getenv
	}
	envOn := func(suffix string) bool {
		v := strings.ToLower(strings.TrimSpace(get(brand.EnvPrefix + suffix)))
		return v == "1" || v == "on" || v == "true" || v == "yes"
	}
	var mws []Middleware

	var defaults llm.Params
	if s := strings.TrimSpace(get(brand.EnvPrefix + "GEN_TEMPERATURE")); s != "" {
		if f, err := strconv.ParseFloat(s, 64); err == nil {
			defaults.Temperature = &f
		}
	}
	if s := strings.TrimSpace(get(brand.EnvPrefix + "GEN_TOP_P")); s != "" {
		if f, err := strconv.ParseFloat(s, 64); err == nil {
			defaults.TopP = &f
		}
	}
	defaults.ReasoningEffort = strings.TrimSpace(get(brand.EnvPrefix + "GEN_REASONING_EFFORT"))
	if !defaults.IsZero() {
		mws = append(mws, DefaultParamsMiddleware(defaults))
	}
	if envOn("EXTRACT_REASONING") {
		mws = append(mws, ExtractReasoningMiddleware("<think>", "</think>"))
	}
	if envOn("SIMULATE_STREAMING") {
		mws = append(mws, SimulateStreamingMiddleware())
	}
	return mws
}
