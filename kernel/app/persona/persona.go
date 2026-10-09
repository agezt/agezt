// SPDX-License-Identifier: MIT

// Package persona owns the owner's chat defaults: the daemon's default
// identity (M710), the fallback system instructions for runs not bound to a
// roster agent, and the saved prompt library (M713) the Chat view launches
// from. Both are the owner editing their own daemon, so full text is returned.
package persona

import (
	"context"
	"encoding/json"
	"errors"
	"reflect"
	"strings"

	"github.com/agezt/agezt/internal/brand"
	"github.com/agezt/agezt/kernel/app"
	"github.com/agezt/agezt/kernel/contract/opapi"
	"github.com/agezt/agezt/kernel/platform/schema"
)

const (
	MaxPrompts     = 100
	MaxPromptTitle = 120
	MaxPromptText  = 8000
)

// Store is the config store the default identity persists to.
type Store interface {
	Load() error
	Set(name, value string)
	Remove(name string) bool
	Save() error
}

// Ports reads and changes the primary kernel's default identity, opens the
// config store, and reads and writes the prompt library file's bytes.
type Ports struct {
	System      func() string
	SetSystem   func(string)
	Store       func() Store
	ReadPrompts func() ([]byte, error)
	SavePrompts func([]byte) error
}

type Service struct{ ports Ports }

func New(ports Ports) *Service { return &Service{ports: ports} }

type GetRequest struct{}

type GetOutput struct {
	System string `json:"system"`
	Set    bool   `json:"set"`
}

func (s *Service) Get(_ context.Context, _ GetRequest) (GetOutput, error) {
	system := s.ports.System()
	return GetOutput{System: system, Set: system != ""}, nil
}

type SetRequest struct {
	System json.RawMessage `json:"system,omitempty"`
}

type SetOutput struct {
	Saved   bool   `json:"saved"`
	Applied string `json:"applied"`
	Set     bool   `json:"set"`
	Length  int    `json:"length"`
}

// Set replaces the default identity, persisting it as AGEZT_SYSTEM_PROMPT
// (removed when empty) and applying it live. The text is kept as given.
func (s *Service) Set(_ context.Context, in SetRequest) (SetOutput, error) {
	if len(in.System) == 0 {
		return SetOutput{}, errors.New("args.system required (string; empty to clear)")
	}
	var v any
	_ = json.Unmarshal(in.System, &v)
	system, ok := v.(string)
	if !ok {
		return SetOutput{}, errors.New("args.system must be a string")
	}
	store := s.ports.Store()
	if err := store.Load(); err != nil {
		return SetOutput{}, errors.New("load config: " + err.Error())
	}
	name := brand.EnvPrefix + "SYSTEM_PROMPT"
	if system != "" {
		store.Set(name, system)
	} else {
		store.Remove(name)
	}
	if err := store.Save(); err != nil {
		return SetOutput{}, errors.New("save config: " + err.Error())
	}
	s.ports.SetSystem(system)
	return SetOutput{Saved: true, Applied: "live", Set: system != "", Length: len(system)}, nil
}

type Prompt struct {
	Title string `json:"title"`
	Text  string `json:"text"`
}

type PromptsRequest struct{}

type PromptsOutput struct {
	Prompts []Prompt `json:"prompts"`
}

// Prompts returns the saved library; a missing or unreadable file is empty, so
// a corrupt file never breaks the Chat view.
func (s *Service) Prompts(_ context.Context, _ PromptsRequest) (PromptsOutput, error) {
	out := []Prompt{}
	if data, err := s.ports.ReadPrompts(); err == nil {
		var items []Prompt
		if json.Unmarshal(data, &items) == nil {
			out = append(out, items...)
		}
	}
	return PromptsOutput{Prompts: out}, nil
}

type PromptsSetRequest struct {
	Prompts json.RawMessage `json:"prompts,omitempty"`
}

type PromptsSetOutput struct {
	Saved bool `json:"saved"`
	Count int  `json:"count"`
}

// SetPrompts replaces the library. Entries need a title and a text after
// trimming; non-object entries are skipped; each field is cut to its byte cap
// and at most MaxPrompts are kept.
func (s *Service) SetPrompts(_ context.Context, in PromptsSetRequest) (PromptsSetOutput, error) {
	if len(in.Prompts) == 0 {
		return PromptsSetOutput{}, errors.New("args.prompts required (array of {title, text})")
	}
	var v any
	_ = json.Unmarshal(in.Prompts, &v)
	arr, ok := v.([]any)
	if !ok {
		return PromptsSetOutput{}, errors.New("args.prompts must be an array")
	}
	items := make([]Prompt, 0, len(arr))
	for _, e := range arr {
		m, ok := e.(map[string]any)
		if !ok {
			continue
		}
		title, _ := m["title"].(string)
		text, _ := m["text"].(string)
		title = strings.TrimSpace(title)
		text = strings.TrimSpace(text)
		if title == "" || text == "" {
			continue
		}
		if len(title) > MaxPromptTitle {
			title = title[:MaxPromptTitle]
		}
		if len(text) > MaxPromptText {
			text = text[:MaxPromptText]
		}
		items = append(items, Prompt{Title: title, Text: text})
		if len(items) >= MaxPrompts {
			break
		}
	}
	data, err := json.MarshalIndent(items, "", "  ")
	if err != nil {
		return PromptsSetOutput{}, errors.New("encode prompts: " + err.Error())
	}
	if err := s.ports.SavePrompts(data); err != nil {
		return PromptsSetOutput{}, errors.New("save prompts: " + err.Error())
	}
	return PromptsSetOutput{Saved: true, Count: len(items)}, nil
}

// Operations declares the two read-only views and the two audited edits, all
// operator-only, on their Web UI routes.
func Operations(provider func(context.Context) *Service) ([]app.Operation, error) {
	if provider == nil {
		return nil, errors.New("persona provider required")
	}
	defs := []struct {
		name, method, path string
		read               bool
		output             reflect.Type
		input              string
		build              func(opapi.Spec) (app.Operation, error)
	}{
		{"persona_get", "GET", "/api/persona", true, reflect.TypeFor[GetOutput](), "", func(s opapi.Spec) (app.Operation, error) {
			return app.NewOperation(s, func(ctx context.Context, in GetRequest) (GetOutput, error) { return provider(ctx).Get(ctx, in) })
		}},
		{"persona_set", "POST", "/api/persona/set", false, reflect.TypeFor[SetOutput](), "system", func(s opapi.Spec) (app.Operation, error) {
			return app.NewOperation(s, func(ctx context.Context, in SetRequest) (SetOutput, error) { return provider(ctx).Set(ctx, in) })
		}},
		{"prompts_get", "GET", "/api/prompts", true, reflect.TypeFor[PromptsOutput](), "", func(s opapi.Spec) (app.Operation, error) {
			return app.NewOperation(s, func(ctx context.Context, in PromptsRequest) (PromptsOutput, error) {
				return provider(ctx).Prompts(ctx, in)
			})
		}},
		{"prompts_set", "POST", "/api/prompts/set", false, reflect.TypeFor[PromptsSetOutput](), "prompts", func(s opapi.Spec) (app.Operation, error) {
			return app.NewOperation(s, func(ctx context.Context, in PromptsSetRequest) (PromptsSetOutput, error) {
				return provider(ctx).SetPrompts(ctx, in)
			})
		}},
	}
	ops := make([]app.Operation, 0, len(defs))
	for _, d := range defs {
		out, err := schema.FromType(d.output, false)
		if err != nil {
			return nil, err
		}
		spec := opapi.Spec{Name: d.name, ReadOnly: d.read, OutputSchema: out, Authz: opapi.PrimaryOnly, Tenancy: opapi.Primary, AllowUnknownInput: true, HTTP: opapi.HTTP{Method: d.method, Path: d.path}}
		if d.input != "" {
			spec.InputSchema = json.RawMessage(`{"type":"object","additionalProperties":true,"properties":{"` + d.input + `":{}}}`)
		}
		op, err := d.build(spec)
		if err != nil {
			return nil, err
		}
		ops = append(ops, op)
	}
	return ops, nil
}
