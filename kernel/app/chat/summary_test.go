// SPDX-License-Identifier: MIT

package chat

import (
	"context"
	"encoding/json"
	"errors"
	"reflect"
	"strings"
	"testing"

	"github.com/agezt/agezt/kernel/contract/llm"
	"github.com/agezt/agezt/kernel/contract/opapi"
	"github.com/agezt/agezt/kernel/event"
	"github.com/agezt/agezt/kernel/platform/schema"
)

type fakeSummaryProvider struct {
	reqs  []llm.CompletionRequest
	reply string
	err   error
}

func (f *fakeSummaryProvider) Name() string { return "fake" }

func (f *fakeSummaryProvider) Complete(_ context.Context, req llm.CompletionRequest) (*llm.CompletionResponse, error) {
	f.reqs = append(f.reqs, req)
	if f.err != nil {
		return nil, f.err
	}
	return &llm.CompletionResponse{Message: llm.Message{Role: llm.RoleAssistant, Content: f.reply}}, nil
}

func summarizer(p llm.Provider) *Summarizer {
	return NewSummarizer(SummaryPorts{Provider: func() llm.Provider { return p }, Model: func() string { return "default-model" }})
}

func TestSummarize(t *testing.T) {
	prov := &fakeSummaryProvider{reply: "  the briefing \n"}
	s := summarizer(prov)
	turns := json.RawMessage(`[{"role":"system","text":"Previous: Oslo"},5,{"role":"user","text":" "},{"role":" ","text":"x"},{"role":"user","text":"Make it Bergen"}]`)
	out, err := s.Summarize(context.Background(), SummarizeRequest{Turns: turns})
	if err != nil || out != (SummarizeOutput{Summary: "the briefing", Turns: 2}) {
		t.Fatal(out, err)
	}
	req := prov.reqs[0]
	content := req.Messages[0].Content
	if req.Model != "default-model" || req.TaskType != "summarize" || req.MaxTokens != 2048 || len(req.Messages) != 1 || req.Messages[0].Role != llm.RoleUser || !strings.HasPrefix(content, "Condense this conversation") || strings.Index(content, "Previous: Oslo") > strings.Index(content, "Make it Bergen") {
		t.Fatalf("%+v", req)
	}
	if _, err := s.Summarize(context.Background(), SummarizeRequest{Turns: turns, Model: json.RawMessage(`"custom"`)}); err != nil || prov.reqs[1].Model != "custom" {
		t.Fatal(err, prov.reqs[1].Model)
	}
	if _, err := s.Summarize(context.Background(), SummarizeRequest{Turns: turns, Model: json.RawMessage(`""`)}); err != nil || prov.reqs[2].Model != "default-model" {
		t.Fatal(err)
	}
	long := strings.Repeat("x", 30000)
	if _, err := s.Summarize(context.Background(), SummarizeRequest{Turns: json.RawMessage(`[{"role":"user","text":"` + long + `END"}]`)}); err != nil {
		t.Fatal(err)
	}
	if got := prov.reqs[3].Messages[0].Content; !strings.HasSuffix(got, "END") || len(got) != len("Condense this conversation into a compact briefing for the assistant's working memory. Preserve facts, names, numbers, decisions, preferences, and open questions; drop pleasantries. Output only the briefing.\n\n")+24<<10 {
		t.Fatal(len(got))
	}
	calls := len(prov.reqs)
	for _, c := range []struct {
		in   SummarizeRequest
		want string
	}{
		{SummarizeRequest{}, "args.turns required"},
		{SummarizeRequest{Turns: json.RawMessage(`[]`)}, "args.turns required"},
		{SummarizeRequest{Turns: json.RawMessage(`"text"`)}, "args.turns required"},
		{SummarizeRequest{Turns: json.RawMessage(`[{"role":"user"}]`), Model: json.RawMessage(`5`)}, "args.turns required"},
		{SummarizeRequest{Turns: turns, Model: json.RawMessage(`5`)}, "args.model must be a string"},
		{SummarizeRequest{Turns: turns, Model: json.RawMessage(`null`)}, "args.model must be a string"},
	} {
		if _, err := s.Summarize(context.Background(), c.in); err == nil || err.Error() != c.want {
			t.Fatal(c.in, err)
		}
	}
	if len(prov.reqs) != calls {
		t.Fatal("refused requests reached the provider")
	}
	if _, err := summarizer(nil).Summarize(context.Background(), SummarizeRequest{Turns: turns, Model: json.RawMessage(`5`)}); err == nil || err.Error() != "daemon has no provider configured" {
		t.Fatal("the provider check precedes the model check", err)
	}
	if _, err := summarizer(&fakeSummaryProvider{reply: " \t\n"}).Summarize(context.Background(), SummarizeRequest{Turns: turns}); err == nil || err.Error() != "summarizer returned an empty summary" {
		t.Fatal(err)
	}
	boom := errors.New("provider down")
	if _, err := summarizer(&fakeSummaryProvider{err: boom}).Summarize(context.Background(), SummarizeRequest{Turns: turns}); !errors.Is(err, boom) {
		t.Fatal(err)
	}
}

func TestSummaryOperations(t *testing.T) {
	if _, err := SummaryOperations(nil); err == nil {
		t.Fatal("provider is required")
	}
	ops, err := SummaryOperations(func(context.Context) *Summarizer { return nil })
	if err != nil || len(ops) != 1 {
		t.Fatal(len(ops), err)
	}
	s := ops[0].Spec()
	out, err := schema.FromType(reflect.TypeFor[SummarizeOutput](), false)
	if err != nil {
		t.Fatal(err)
	}
	if s.Name != "chat_summarize" || s.ReadOnly || s.Authz != opapi.PrimaryOnly || s.Tenancy != opapi.Primary || s.Stream != opapi.StreamLive || !s.AllowUnknownInput || s.HTTP.Method != "POST" || s.HTTP.Path != "/api/chat/summarize" || s.Input != reflect.TypeFor[SummarizeRequest]() || s.Output != reflect.TypeFor[SummarizeOutput]() || s.Emission != reflect.TypeFor[event.Event]() || string(s.OutputSchema) != string(out) {
		t.Fatalf("spec: %+v", s)
	}
	if !strings.Contains(string(s.InputSchema), `"properties":{"turns":{},"model":{}}`) {
		t.Fatal(string(s.InputSchema))
	}
	if err := schema.ValidateJSON(s.InputSchema, json.RawMessage(`{"turns":5,"model":null,"tenant":"t"}`)); err != nil {
		t.Fatal(err)
	}
}
