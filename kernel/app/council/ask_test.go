// SPDX-License-Identifier: MIT

package council

import (
	"context"
	"encoding/json"
	"errors"
	"reflect"
	"strconv"
	"strings"
	"testing"

	"github.com/agezt/agezt/kernel/app"
	"github.com/agezt/agezt/kernel/contract/opapi"
	"github.com/agezt/agezt/kernel/modules/reasoning/api"
	"github.com/agezt/agezt/kernel/platform/schema"
)

// askEngines is a scripted pair of engines: the panel and the Conductor.
type askEngines struct {
	convened      []string
	conducted     []api.ConductorConfig
	conductedCorr []string
	council       api.CouncilResult
	conductor     api.ConductorResult
	err           error
}

func (e *askEngines) engines() Engines {
	return Engines{
		Convene: func(_ context.Context, corr, question string, rounds int) (api.CouncilResult, error) {
			e.convened = append(e.convened, corr+"|"+question+"|"+strconv.Itoa(rounds))
			return e.council, e.err
		},
		RunConductor: func(_ context.Context, corr string, cfg api.ConductorConfig) (api.ConductorResult, error) {
			e.conducted = append(e.conducted, cfg)
			e.conductedCorr = append(e.conductedCorr, corr)
			return e.conductor, e.err
		},
		NewCorrelation: func() string { return "minted-correlation" },
	}
}

// TestSanitizeCorrelation: the moved helper keeps the legacy contract — a short
// plain token survives, anything else is refused so the server mints its own.
func TestSanitizeCorrelation(t *testing.T) {
	for raw, want := range map[string]string{
		"":                      "",
		"   ":                   "",
		"corr-1":                "corr-1",
		"  corr-1  ":            "corr-1",
		"a_b-C9":                "a_b-C9",
		"has space":             "",
		"has.dot":               "",
		"has/slash":             "",
		"emoji🙂":                "",
		strings.Repeat("a", 81): "",
		strings.Repeat("a", 80): strings.Repeat("a", 80),
	} {
		if got := app.SanitizeCorrelation(raw); got != want {
			t.Fatalf("SanitizeCorrelation(%q) = %q, want %q", raw, got, want)
		}
	}
}

// TestAskRejectsAMissingQuestion: the empty question fails before the panel is
// convened, with the legacy message.
func TestAskRejectsAMissingQuestion(t *testing.T) {
	e := &askEngines{}
	for _, raw := range []string{``, `null`, `""`, `"   "`, `7`, `{}`} {
		out, err := e.engines().Ask(context.Background(), AskRequest{Question: json.RawMessage(raw)})
		if err == nil || err.Error() != "args.question required" {
			t.Fatalf("%s: %v, want args.question required", raw, err)
		}
		if !reflect.DeepEqual(out, AskOutput{}) {
			t.Fatalf("%s: a refused question answers nothing: %+v", raw, out)
		}
	}
	if len(e.convened) != 0 {
		t.Fatal("a refused question never convenes the panel", e.convened)
	}
}

// TestAskMapsTheDeliberation: the panel's result becomes the typed answer, and
// the correlation is either the client's plain id or a minted one.
func TestAskMapsTheDeliberation(t *testing.T) {
	e := &askEngines{council: api.CouncilResult{
		Question:  "why?",
		Consensus: "because",
		Dissent:   "no",
		Rounds:    2,
		Members:   []api.CouncilMember{{Seat: "Chair", Model: "m1"}},
		Opinions: []api.Opinion{
			{Seat: "Chair", Model: "m1", Round: 1, Text: "yes"},
			{Seat: "Scribe", Model: "m2", Round: 1, Text: "", Error: "provider failed"},
		},
		AsOf:  "2026-10-19",
		Brief: "grounded",
	}}
	out, err := e.engines().Ask(context.Background(), AskRequest{Question: json.RawMessage(`"why?"`), Corr: json.RawMessage(`"client-corr"`)})
	if err != nil {
		t.Fatal(err)
	}
	if out.CorrelationID != "client-corr" || out.Question != "why?" || out.Consensus != "because" || out.Dissent != "no" || out.Rounds != 2 || out.AsOf != "2026-10-19" || out.Brief != "grounded" {
		t.Fatalf("output lost a field: %+v", out)
	}
	if len(out.Members) != 1 || out.Members[0] != (Member{Seat: "Chair", Model: "m1"}) {
		t.Fatalf("members: %+v", out.Members)
	}
	if len(out.Opinions) != 2 || out.Opinions[0] != (OpinionRow{Seat: "Chair", Model: "m1", Round: 1, Text: "yes"}) || out.Opinions[1] != (OpinionRow{Seat: "Scribe", Model: "m2", Round: 1, Error: "provider failed"}) {
		t.Fatalf("opinions: %+v", out.Opinions)
	}
	if len(e.convened) != 1 || e.convened[0] != "client-corr|why?|0" {
		t.Fatalf("the panel was not convened with the client's correlation: %v", e.convened)
	}
}

// TestAskReadsTheLenientArguments: rounds accept a number or a digit string,
// and an odd correlation id is replaced by a minted one.
func TestAskReadsTheLenientArguments(t *testing.T) {
	for raw, want := range map[string]string{
		`2`:    "2",
		`2.9`:  "2",
		`"3"`:  "3",
		`"x"`:  "0",
		`true`: "0",
	} {
		e := &askEngines{council: api.CouncilResult{Question: "q"}}
		if _, err := e.engines().Ask(context.Background(), AskRequest{Question: json.RawMessage(`"q"`), Rounds: json.RawMessage(raw)}); err != nil {
			t.Fatal(err)
		}
		if got := e.convened[0]; !strings.HasSuffix(got, "|"+want) {
			t.Fatalf("rounds %s: %s, want %s", raw, got, want)
		}
	}
	e := &askEngines{council: api.CouncilResult{Question: "q"}}
	out, err := e.engines().Ask(context.Background(), AskRequest{Question: json.RawMessage(`"q"`), Corr: json.RawMessage(`"not a plain id"`)})
	if err != nil {
		t.Fatal(err)
	}
	if out.CorrelationID != "minted-correlation" {
		t.Fatalf("an odd correlation id is minted: %q", out.CorrelationID)
	}
}

// TestAskReportsTheEngineFailure: a panel failure is the operation's failure.
func TestAskReportsTheEngineFailure(t *testing.T) {
	e := &askEngines{err: errors.New("council: no members available")}
	if _, err := e.engines().Ask(context.Background(), AskRequest{Question: json.RawMessage(`"q"`)}); err == nil || err.Error() != "council: no members available" {
		t.Fatal(err)
	}
}

// TestConductRejectsAMissingTask: the empty task fails before the loop starts.
func TestConductRejectsAMissingTask(t *testing.T) {
	e := &askEngines{}
	for _, raw := range []string{``, `null`, `""`, `"  "`, `7`} {
		if _, err := e.engines().Conduct(context.Background(), ConductRequest{Task: json.RawMessage(raw)}); err == nil || err.Error() != "args.task required" {
			t.Fatalf("%s: %v, want args.task required", raw, err)
		}
	}
	if len(e.conducted) != 0 {
		t.Fatal("a refused task never runs the loop", e.conducted)
	}
}

// TestConductMapsTheTranscript: the loop's config and transcript become the
// typed answer, and the empty step fields stay empty.
func TestConductMapsTheTranscript(t *testing.T) {
	e := &askEngines{conductor: api.ConductorResult{
		Task:   "what is 2+2?",
		Answer: "4",
		Passed: true,
		Roles:  map[string]string{"thinker": "m1", "worker": "m2", "verifier": "m3"},
		Rounds: 1,
		Plan:   "a plan",
		Steps: []api.ConductorStep{
			{Round: 1, Role: "thinker", Model: "m1", Text: "think"},
			{Round: 1, Role: "worker", Model: "m2", Text: "work", Verdict: "pass", Reason: "fine"},
			{Round: 1, Role: "verifier", Model: "m3", Error: "model unavailable"},
			{Round: 1, Role: "verifier", Model: "m3", Exec: &api.ConductorExec{Ran: true, OK: false, Language: "go", Output: "boom"}},
		},
	}}
	out, err := e.engines().Conduct(context.Background(), ConductRequest{
		Task: json.RawMessage(`"what is 2+2?"`), Corr: json.RawMessage(`"client-corr"`),
		Thinker: json.RawMessage(`"m1"`), Worker: json.RawMessage(`"m2"`), Verifier: json.RawMessage(`"m3"`),
		MaxRounds: json.RawMessage(`"2"`), Plan: json.RawMessage(`"true"`),
	})
	if err != nil {
		t.Fatal(err)
	}
	if out.CorrelationID != "client-corr" || out.Task != "what is 2+2?" || out.Answer != "4" || !out.Passed || out.Rounds != 1 || out.Plan != "a plan" {
		t.Fatalf("output lost a field: %+v", out)
	}
	if out.Roles["verifier"] != "m3" {
		t.Fatalf("roles: %+v", out.Roles)
	}
	if len(out.Steps) != 4 {
		t.Fatalf("steps: %+v", out.Steps)
	}
	if out.Steps[2].Error != "model unavailable" || out.Steps[2].Text != "" {
		t.Fatalf("an empty step field stays empty: %+v", out.Steps[2])
	}
	if out.Steps[3].Exec == nil || !out.Steps[3].Exec.Ran || out.Steps[3].Exec.OK || out.Steps[3].Exec.Language != "go" || out.Steps[3].Exec.Output != "boom" {
		t.Fatalf("exec: %+v", out.Steps[3].Exec)
	}
	cfg := e.conducted[0]
	if cfg.Thinker != "m1" || cfg.Worker != "m2" || cfg.Verifier != "m3" || cfg.MaxRounds != 2 || !cfg.Plan {
		t.Fatalf("the loop config: %+v", cfg)
	}
}

// TestConductReadsTheLenientPlanFlag: the plan flag accepts a boolean or the
// strings "true" and "1".
func TestConductReadsTheLenientPlanFlag(t *testing.T) {
	for raw, want := range map[string]bool{
		`true`:   true,
		`"true"`: true,
		`"1"`:    true,
		`false`:  false,
		`"no"`:   false,
		`7`:      false,
	} {
		e := &askEngines{}
		if _, err := e.engines().Conduct(context.Background(), ConductRequest{Task: json.RawMessage(`"t"`), Plan: json.RawMessage(raw)}); err != nil {
			t.Fatal(err)
		}
		if e.conducted[0].Plan != want {
			t.Fatalf("plan %s: %v, want %v", raw, e.conducted[0].Plan, want)
		}
	}
}

// TestAskOperations: the panel and the Conductor are primary-only, live and
// audited operations on their own routes.
func TestAskOperations(t *testing.T) {
	if _, err := AskOperations(nil); err == nil {
		t.Fatal("provider is required")
	}
	ops, err := AskOperations(func(context.Context) Engines { return Engines{} })
	if err != nil || len(ops) != 2 {
		t.Fatal(len(ops), err)
	}
	for i, want := range []struct {
		name, method, path string
		output             reflect.Type
	}{
		// The panel has a Web UI route; the Conductor's nav cluster was
		// removed in the Day-28 topology audit, so it has none.
		{"council_ask", "POST", "/api/council/ask", reflect.TypeFor[AskOutput]()},
		{"conductor_ask", "", "", reflect.TypeFor[ConductOutput]()},
	} {
		s := ops[i].Spec()
		out, err := schema.FromType(want.output, false)
		if err != nil {
			t.Fatal(err)
		}
		if s.Name != want.name || s.ReadOnly || s.Authz != opapi.PrimaryOnly || s.Tenancy != opapi.Primary || s.Stream != opapi.StreamLive || !s.AllowUnknownInput || s.HTTP.Method != want.method || s.HTTP.Path != want.path || s.Output != want.output || string(s.OutputSchema) != string(out) {
			t.Fatalf("%s spec: %+v", want.name, s)
		}
		if len(s.EmissionSchema) == 0 {
			t.Fatalf("%s declares no event frames", want.name)
		}
		for _, in := range []string{`{}`, `{"question":"q","rounds":2,"corr":"c"}`} {
			if err := schema.ValidateJSON(s.InputSchema, json.RawMessage(in)); err != nil {
				t.Fatal(want.name, in, err)
			}
		}
	}
}
