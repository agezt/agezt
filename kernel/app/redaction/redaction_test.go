// SPDX-License-Identifier: MIT

package redaction

import (
	"context"
	"encoding/json"
	"reflect"
	"strings"
	"testing"

	"github.com/agezt/agezt/kernel/contract/opapi"
	"github.com/agezt/agezt/kernel/platform/schema"
)

type replacer struct{ old, new string }

func (r replacer) Redact(s string) string { return strings.ReplaceAll(s, r.old, r.new) }

func request(t *testing.T, raw string) TestRequest {
	t.Helper()
	var in TestRequest
	if err := json.Unmarshal([]byte(raw), &in); err != nil {
		t.Fatal(err)
	}
	return in
}

func TestRedactionTest(t *testing.T) {
	ctx := context.Background()
	var seen []string
	categories := func(s string) []string {
		seen = append(seen, s)
		if strings.Contains(s, "sk-") {
			return []string{"openai_key"}
		}
		return nil
	}
	live := func() Redactor { return replacer{"sk-1", "[REDACTED]"} }
	for _, raw := range []string{`{"text":3}`, `{"text":null}`, `{"text":["x"]}`} {
		seen = nil
		if _, err := New(live, categories).Test(ctx, request(t, raw)); err == nil || err.Error() != "args.text must be a string" || seen != nil {
			t.Fatal(raw, err)
		}
	}
	out, err := New(live, categories).Test(ctx, request(t, `{"text":" key sk-1 "}`))
	if err != nil || !reflect.DeepEqual(out, TestOutput{Enabled: true, WouldRedact: true, Redacted: " key [REDACTED] ", Categories: []string{"openai_key"}, LiteralHit: false}) || seen[0] != " key sk-1 " {
		t.Fatalf("a pattern match is scrubbed and named; the candidate passes untrimmed: %+v %v", out, err)
	}
	literal := func() Redactor { return replacer{"hunter2", "[REDACTED]"} }
	out, _ = New(literal, categories).Test(ctx, request(t, `{"text":"pw hunter2"}`))
	if !out.WouldRedact || !out.LiteralHit || out.Redacted != "pw [REDACTED]" || out.Categories == nil || len(out.Categories) != 0 {
		t.Fatalf("a change no pattern explains is a configured literal: %+v", out)
	}
	out, _ = New(live, categories).Test(ctx, request(t, `{"text":"plain"}`))
	if out.WouldRedact || out.LiteralHit || out.Redacted != "plain" || !out.Enabled {
		t.Fatal(out)
	}
	off := func() Redactor { return nil }
	out, _ = New(off, categories).Test(ctx, request(t, `{"text":"sk-1"}`))
	if out.Enabled || out.WouldRedact || out.LiteralHit || out.Redacted != "sk-1" || !reflect.DeepEqual(out.Categories, []string{"openai_key"}) {
		t.Fatalf("with redaction off nothing is scrubbed, though the patterns still name a match: %+v", out)
	}
	out, err = New(live, categories).Test(ctx, request(t, `{}`))
	if raw, _ := json.Marshal(out); err != nil || string(raw) != `{"enabled":true,"would_redact":false,"redacted":"","categories":[],"literal_hit":false}` {
		t.Fatal("an absent candidate is empty", string(raw), err)
	}
}

func TestRedactionOperations(t *testing.T) {
	if _, err := Operations(nil); err == nil {
		t.Fatal("nil provider")
	}
	svc := New(func() Redactor { return nil }, func(string) []string { return nil })
	ops, err := Operations(func(context.Context) *Service { return svc })
	if err != nil || len(ops) != 1 {
		t.Fatal(ops, err)
	}
	spec := ops[0].Spec()
	if spec.Name != "redact_test" || !spec.ReadOnly || spec.Authz != opapi.PrimaryOnly || spec.Tenancy != opapi.Primary || !spec.AllowUnknownInput || spec.HTTP != (opapi.HTTP{Method: "POST", Path: "/api/redact/test"}) {
		t.Fatal(spec)
	}
	out, _ := svc.Test(context.Background(), TestRequest{})
	raw, _ := json.Marshal(out)
	if err := schema.ValidateJSON(spec.OutputSchema, raw); err != nil {
		t.Fatal(err, string(raw))
	}
}
