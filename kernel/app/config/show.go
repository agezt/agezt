// SPDX-License-Identifier: MIT
package config

import (
	"context"
	"path/filepath"
)

// RoutingReader exposes effective provider routing tables, without provider calls.
type RoutingReader interface {
	TaskRoutesView() map[string][]string
	TaskRouteRequiresView() map[string][]string
	TaskModelOverridesView() map[string]string
}

// Reader projects selected daemon configuration without exposing secret values or
// system prompt contents. Process lifecycle and configuration writes stay outside.
type Reader interface {
	BaseDir() string
	Model() string
	SystemPromptSet() bool
	ToolCount() int
	PluginCount() int
	AskPolicy() string
	Routing() (RoutingReader, bool)
}
type Service struct {
	reader     Reader
	envNames   []string
	envPresent func(string) bool
}

func New(reader Reader, envNames []string, envPresent func(string) bool) *Service {
	return &Service{reader: reader, envNames: append([]string(nil), envNames...), envPresent: envPresent}
}

type ShowInput struct{}
type Paths struct {
	Base    string `json:"base"`
	Journal string `json:"journal"`
	State   string `json:"state"`
	Runtime string `json:"runtime"`
	Catalog string `json:"catalog"`
	Vault   string `json:"vault"`
}
type Routing struct {
	Routes         map[string][]string `json:"routes,omitempty"`
	Requires       map[string][]string `json:"requires,omitempty"`
	ModelOverrides map[string]string   `json:"model_overrides,omitempty"`
}
type ShowOutput struct {
	Paths           Paths           `json:"paths"`
	Model           string          `json:"model"`
	SystemPromptSet bool            `json:"system_prompt_set"`
	ToolCount       int             `json:"tool_count"`
	PluginCount     int             `json:"plugin_count"`
	AskPolicy       string          `json:"ask_policy"`
	Env             map[string]bool `json:"env"`
	Routing         *Routing        `json:"routing,omitempty"`
}

func (s *Service) Show(_ context.Context, _ ShowInput) (ShowOutput, error) {
	base := s.reader.BaseDir()
	paths := Paths{Base: base, Journal: filepath.Join(base, "journal"), State: filepath.Join(base, "state"), Runtime: filepath.Join(base, "runtime"), Catalog: filepath.Join(base, "catalog"), Vault: filepath.Join(base, "vault.json")}

	env := map[string]bool{}
	for _, name := range s.envNames {
		if s.envPresent(name) {
			env[name] = true
		}
	}

	result := ShowOutput{Paths: paths, Model: s.reader.Model(), SystemPromptSet: s.reader.SystemPromptSet(), ToolCount: s.reader.ToolCount(), PluginCount: s.reader.PluginCount(), AskPolicy: s.reader.AskPolicy(), Env: env}

	// Effective routing tables (M108): surface what AGEZT_TASK_ROUTES /
	// _ROUTE_REQUIRES / _MODEL_OVERRIDES actually parsed to, so an operator can
	// confirm a rule loaded rather than reading the boot log. Only present when
	// the provider is the governor (the usual case) and a table is non-empty.
	if gov, ok := s.reader.Routing(); ok {
		routing := Routing{}
		if r := gov.TaskRoutesView(); len(r) > 0 {
			routing.Routes = copyStringSliceMap(r)
		}
		if r := gov.TaskRouteRequiresView(); len(r) > 0 {
			routing.Requires = copyStringSliceMap(r)
		}
		if o := gov.TaskModelOverridesView(); len(o) > 0 {
			m := make(map[string]string, len(o))
			for k, v := range o {
				m[k] = v
			}
			routing.ModelOverrides = m
		}
		if len(routing.Routes)+len(routing.Requires)+len(routing.ModelOverrides) > 0 {
			result.Routing = &routing
		}
	}

	return result, nil
}

func copyStringSliceMap(in map[string][]string) map[string][]string {
	out := make(map[string][]string, len(in))
	for key, values := range in {
		row := make([]string, len(values))
		copy(row, values)
		out[key] = row
	}
	return out
}
