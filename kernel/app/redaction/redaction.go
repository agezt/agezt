// SPDX-License-Identifier: MIT

// Package redaction lets an operator confirm the live secret redactor catches a
// candidate secret before it could reach the permanent journal. Only the
// redacted form is ever returned.
package redaction

import (
	"context"
	"encoding/json"
	"errors"
	"reflect"

	"github.com/agezt/agezt/kernel/app"
	"github.com/agezt/agezt/kernel/contract/opapi"
	"github.com/agezt/agezt/kernel/platform/schema"
)

// Redactor is the bus's live redactor: built-in patterns plus configured
// secret literals.
type Redactor interface {
	Redact(string) string
}

// Service checks candidates against the live redactor (nil when redaction is
// off) and names the built-in pattern categories a candidate matches.
type Service struct {
	redactor   func() Redactor
	categories func(string) []string
}

func New(redactor func() Redactor, categories func(string) []string) *Service {
	return &Service{redactor: redactor, categories: categories}
}

type TestRequest struct {
	Text json.RawMessage `json:"text,omitempty"`
}

type TestOutput struct {
	Enabled     bool     `json:"enabled"`
	WouldRedact bool     `json:"would_redact"`
	Redacted    string   `json:"redacted"`
	Categories  []string `json:"categories"`
	LiteralHit  bool     `json:"literal_hit"`
}

// Test reports whether the candidate would be scrubbed. A change the built-in
// patterns do not explain is a configured literal; which one is never said.
func (s *Service) Test(_ context.Context, in TestRequest) (TestOutput, error) {
	text := ""
	if len(in.Text) > 0 {
		var v any
		_ = json.Unmarshal(in.Text, &v)
		str, ok := v.(string)
		if !ok {
			return TestOutput{}, errors.New("args.text must be a string")
		}
		text = str
	}
	red := s.redactor()
	redacted := text
	if red != nil {
		redacted = red.Redact(text)
	}
	categories := append(make([]string, 0), s.categories(text)...)
	wouldRedact := redacted != text
	return TestOutput{
		Enabled:     red != nil,
		WouldRedact: wouldRedact,
		Redacted:    redacted,
		Categories:  categories,
		LiteralHit:  wouldRedact && len(categories) == 0,
	}, nil
}

// Operations declares the unaudited, operator-only redaction check. It is read
// only, so the candidate is never journaled; the Web UI posts it.
func Operations(provider func(context.Context) *Service) ([]app.Operation, error) {
	if provider == nil {
		return nil, errors.New("redaction provider required")
	}
	output, err := schema.FromType(reflect.TypeFor[TestOutput](), false)
	if err != nil {
		return nil, err
	}
	op, err := app.NewOperation(opapi.Spec{Name: "redact_test", ReadOnly: true, OutputSchema: output, Authz: opapi.PrimaryOnly, Tenancy: opapi.Primary, AllowUnknownInput: true, InputSchema: json.RawMessage(`{"type":"object","additionalProperties":true,"properties":{"text":{}}}`), HTTP: opapi.HTTP{Method: "POST", Path: "/api/redact/test"}}, func(ctx context.Context, in TestRequest) (TestOutput, error) {
		return provider(ctx).Test(ctx, in)
	})
	if err != nil {
		return nil, err
	}
	return []app.Operation{op}, nil
}
