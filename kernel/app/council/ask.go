// SPDX-License-Identifier: MIT

package council

import (
	"context"
	"encoding/json"
	"errors"
	"reflect"

	"github.com/agezt/agezt/kernel/app"
	"github.com/agezt/agezt/kernel/contract/opapi"
	"github.com/agezt/agezt/kernel/contract/types"
	"github.com/agezt/agezt/kernel/event"
	"github.com/agezt/agezt/kernel/platform/schema"
	"github.com/agezt/agezt/kernel/runtime"
)

// Engines convenes the panel and runs the Conductor under a correlation, and
// mints one when the caller supplies none.
type Engines struct {
	Convene        func(ctx context.Context, corr, question string, rounds int) (runtime.CouncilResult, error)
	RunConductor   func(ctx context.Context, corr string, cfg runtime.ConductorConfig) (runtime.ConductorResult, error)
	NewCorrelation func() string
}

// correlation keeps a plain client-supplied id, so the Web UI can subscribe to
// the run's live events before the blocking call returns, or mints one.
func (e Engines) correlation(raw json.RawMessage) string {
	if corr := app.SanitizeCorrelation(app.LenientText(raw)); corr != "" {
		return corr
	}
	return e.NewCorrelation()
}

type AskRequest struct {
	Question json.RawMessage `json:"question,omitempty"`
	Rounds   json.RawMessage `json:"rounds,omitempty"`
	Corr     json.RawMessage `json:"corr,omitempty"`
}

// OpinionRow is one member's position in one round. It is the contract type
// itself, so the panel's answer carries the runtime's opinions unchanged.
type OpinionRow = types.Opinion

type AskOutput struct {
	CorrelationID string       `json:"correlation_id"`
	Question      string       `json:"question"`
	Consensus     string       `json:"consensus"`
	Dissent       string       `json:"dissent"`
	Rounds        int          `json:"rounds"`
	Members       []Member     `json:"members"`
	Opinions      []OpinionRow `json:"opinions"`
	AsOf          string       `json:"as_of"`
	Brief         string       `json:"brief"`
}

// Ask convenes the default panel on a question and returns the deliberation
// and consensus.
func (e Engines) Ask(ctx context.Context, in AskRequest) (AskOutput, error) {
	question := app.LenientText(in.Question)
	if question == "" {
		return AskOutput{}, errors.New("args.question required")
	}
	rounds := app.LenientCount(in.Rounds)
	corr := e.correlation(in.Corr)
	res, err := e.Convene(ctx, corr, question, rounds)
	if err != nil {
		return AskOutput{}, err
	}
	// The panel's opinions are the contract type itself — the copy keeps the
	// answer's opinion list an empty array (never null) without the app
	// knowing the row's fields.
	opinions := make([]OpinionRow, len(res.Opinions))
	copy(opinions, res.Opinions)
	// The panel's members are the contract type itself — the copy keeps the
	// answer's member list an empty array (never null) without the app
	// knowing the row's fields.
	members := make([]Member, len(res.Members))
	copy(members, res.Members)
	return AskOutput{CorrelationID: corr, Question: res.Question, Consensus: res.Consensus, Dissent: res.Dissent, Rounds: res.Rounds, Members: members, Opinions: opinions, AsOf: res.AsOf, Brief: res.Brief}, nil
}

type ConductRequest struct {
	Task      json.RawMessage `json:"task,omitempty"`
	Corr      json.RawMessage `json:"corr,omitempty"`
	Thinker   json.RawMessage `json:"thinker,omitempty"`
	Worker    json.RawMessage `json:"worker,omitempty"`
	Verifier  json.RawMessage `json:"verifier,omitempty"`
	MaxRounds json.RawMessage `json:"max_rounds,omitempty"`
	Plan      json.RawMessage `json:"plan,omitempty"`
}

type ExecRow struct {
	Ran      bool   `json:"ran"`
	OK       bool   `json:"ok"`
	Language string `json:"language"`
	Output   string `json:"output"`
}

type StepRow struct {
	Round   int      `json:"round"`
	Role    string   `json:"role"`
	Model   string   `json:"model"`
	Text    string   `json:"text,omitempty"`
	Verdict string   `json:"verdict,omitempty"`
	Reason  string   `json:"reason,omitempty"`
	Error   string   `json:"error,omitempty"`
	Exec    *ExecRow `json:"exec,omitempty"`
}

type ConductOutput struct {
	CorrelationID string            `json:"correlation_id"`
	Task          string            `json:"task"`
	Answer        string            `json:"answer"`
	Passed        bool              `json:"passed"`
	Roles         map[string]string `json:"roles"`
	Rounds        int               `json:"rounds"`
	Plan          string            `json:"plan"`
	Steps         []StepRow         `json:"steps"`
}

// Conduct runs the full Thinker→Worker→Verifier loop and returns the answer
// and transcript. Roles left empty are filled from the default panel.
func (e Engines) Conduct(ctx context.Context, in ConductRequest) (ConductOutput, error) {
	task := app.LenientText(in.Task)
	if task == "" {
		return ConductOutput{}, errors.New("args.task required")
	}
	corr := e.correlation(in.Corr)
	res, err := e.RunConductor(ctx, corr, runtime.ConductorConfig{
		Task:      task,
		Thinker:   app.LenientText(in.Thinker),
		Worker:    app.LenientText(in.Worker),
		Verifier:  app.LenientText(in.Verifier),
		MaxRounds: app.LenientCount(in.MaxRounds),
		Plan:      app.LenientBool(in.Plan),
	})
	if err != nil {
		return ConductOutput{}, err
	}
	steps := make([]StepRow, 0, len(res.Steps))
	for _, st := range res.Steps {
		row := StepRow{Round: st.Round, Role: st.Role, Model: st.Model, Text: st.Text, Verdict: st.Verdict, Reason: st.Reason, Error: st.Error}
		if st.Exec != nil {
			row.Exec = &ExecRow{Ran: st.Exec.Ran, OK: st.Exec.OK, Language: st.Exec.Language, Output: st.Exec.Output}
		}
		steps = append(steps, row)
	}
	return ConductOutput{CorrelationID: corr, Task: res.Task, Answer: res.Answer, Passed: res.Passed, Roles: res.Roles, Rounds: res.Rounds, Plan: res.Plan, Steps: steps}, nil
}

// AskOperations declares the panel and the Conductor: audited, operator-only
// and live, so a disconnected client cancels the model calls. They send no
// event frames: the Web UI follows a run on the bus under the correlation it
// chose.
func AskOperations(provider func(context.Context) Engines) ([]app.Operation, error) {
	if provider == nil {
		return nil, errors.New("council engines provider required")
	}
	askOut, err := schema.FromType(reflect.TypeFor[AskOutput](), false)
	if err != nil {
		return nil, err
	}
	conductOut, err := schema.FromType(reflect.TypeFor[ConductOutput](), false)
	if err != nil {
		return nil, err
	}
	frames := json.RawMessage(event.WireSchema)
	ask, err := app.NewStreamingOperation(opapi.Spec{Name: "council_ask", Stream: opapi.StreamLive, OutputSchema: askOut, EmissionSchema: frames, Authz: opapi.PrimaryOnly, Tenancy: opapi.Primary, AllowUnknownInput: true, InputSchema: json.RawMessage(`{"type":"object","additionalProperties":true,"properties":{"question":{},"rounds":{},"corr":{}}}`), HTTP: opapi.HTTP{Method: "POST", Path: "/api/council/ask"}}, func(ctx context.Context, in AskRequest, _ func(event.Event) error) (AskOutput, error) {
		return provider(ctx).Ask(ctx, in)
	})
	if err != nil {
		return nil, err
	}
	conduct, err := app.NewStreamingOperation(opapi.Spec{Name: "conductor_ask", Stream: opapi.StreamLive, OutputSchema: conductOut, EmissionSchema: frames, Authz: opapi.PrimaryOnly, Tenancy: opapi.Primary, AllowUnknownInput: true, InputSchema: json.RawMessage(`{"type":"object","additionalProperties":true,"properties":{"task":{},"corr":{},"thinker":{},"worker":{},"verifier":{},"max_rounds":{},"plan":{}}}`)}, func(ctx context.Context, in ConductRequest, _ func(event.Event) error) (ConductOutput, error) {
		return provider(ctx).Conduct(ctx, in)
	})
	if err != nil {
		return nil, err
	}
	return []app.Operation{ask, conduct}, nil
}
