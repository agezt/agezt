// SPDX-License-Identifier: MIT

package governor_test

import (
	"context"
	"testing"

	"github.com/ersinkoc/agezt/kernel/agent"
	"github.com/ersinkoc/agezt/kernel/governor"
)

// modelRecordingProvider remembers the Model field the Governor
// passed to Complete, so tests can verify the override happened
// before the wire call.
type modelRecordingProvider struct {
	fakeProvider
	gotModel string
}

func (p *modelRecordingProvider) Complete(ctx context.Context, req agent.CompletionRequest) (*agent.CompletionResponse, error) {
	p.gotModel = req.Model
	return p.fakeProvider.Complete(ctx, req)
}

// TestGovernor_TaskModelOverride_Applies: when override matches,
// the provider sees the new model id.
func TestGovernor_TaskModelOverride_Applies(t *testing.T) {
	r := governor.NewRegistry()
	prov := &modelRecordingProvider{fakeProvider: fakeProvider{name: "p", resp: okResp("ignored", 1, 1)}}
	mustRegister(t, r,
		&governor.ProviderInfo{Name: "p", Provider: prov, AuthMode: governor.AuthAPIKey},
	)
	g, err := governor.New(governor.Config{
		Registry:           r,
		TaskModelOverrides: governor.TaskModelOverrides{"salience": "haiku-cheap"},
	})
	if err != nil {
		t.Fatalf("New: %v", err)
	}
	_, err = g.Complete(context.Background(), agent.CompletionRequest{
		Model:    "expensive-default",
		TaskType: "salience",
		Messages: []agent.Message{{Role: agent.RoleUser, Content: "x"}},
	})
	if err != nil {
		t.Fatalf("Complete: %v", err)
	}
	if prov.gotModel != "haiku-cheap" {
		t.Errorf("provider saw model %q, want %q (override should have replaced)", prov.gotModel, "haiku-cheap")
	}
}

// TestGovernor_TaskModelOverride_NoMatchingType: untouched when
// TaskType doesn't match.
func TestGovernor_TaskModelOverride_NoMatchingType(t *testing.T) {
	r := governor.NewRegistry()
	prov := &modelRecordingProvider{fakeProvider: fakeProvider{name: "p", resp: okResp("x", 1, 1)}}
	mustRegister(t, r,
		&governor.ProviderInfo{Name: "p", Provider: prov, AuthMode: governor.AuthAPIKey},
	)
	g, err := governor.New(governor.Config{
		Registry:           r,
		TaskModelOverrides: governor.TaskModelOverrides{"salience": "haiku-cheap"},
	})
	if err != nil {
		t.Fatalf("New: %v", err)
	}
	_, err = g.Complete(context.Background(), agent.CompletionRequest{
		Model:    "expensive-default",
		TaskType: "code",
		Messages: []agent.Message{{Role: agent.RoleUser, Content: "x"}},
	})
	if err != nil {
		t.Fatalf("Complete: %v", err)
	}
	if prov.gotModel != "expensive-default" {
		t.Errorf("got model %q, want %q (no override for 'code' task)", prov.gotModel, "expensive-default")
	}
}

// TestGovernor_TaskModelOverride_EmptyTaskType: untouched when
// TaskType is empty (the override is opt-in).
func TestGovernor_TaskModelOverride_EmptyTaskType(t *testing.T) {
	r := governor.NewRegistry()
	prov := &modelRecordingProvider{fakeProvider: fakeProvider{name: "p", resp: okResp("x", 1, 1)}}
	mustRegister(t, r,
		&governor.ProviderInfo{Name: "p", Provider: prov, AuthMode: governor.AuthAPIKey},
	)
	g, err := governor.New(governor.Config{
		Registry:           r,
		TaskModelOverrides: governor.TaskModelOverrides{"salience": "haiku-cheap"},
	})
	if err != nil {
		t.Fatalf("New: %v", err)
	}
	_, err = g.Complete(context.Background(), agent.CompletionRequest{
		Model: "user-pick",
		// No TaskType.
		Messages: []agent.Message{{Role: agent.RoleUser, Content: "x"}},
	})
	if err != nil {
		t.Fatalf("Complete: %v", err)
	}
	if prov.gotModel != "user-pick" {
		t.Errorf("got model %q, want %q (empty TaskType should bypass override)", prov.gotModel, "user-pick")
	}
}

// TestParseTaskModelOverridesEnv_Basic exercises the parser.
func TestParseTaskModelOverridesEnv_Basic(t *testing.T) {
	parsed, err := governor.ParseTaskModelOverridesEnv("plan=opus;salience=haiku;code=sonnet")
	if err != nil {
		t.Fatalf("parse: %v", err)
	}
	if parsed["plan"] != "opus" {
		t.Errorf("plan = %q, want opus", parsed["plan"])
	}
	if parsed["salience"] != "haiku" {
		t.Errorf("salience = %q, want haiku", parsed["salience"])
	}
	if parsed["code"] != "sonnet" {
		t.Errorf("code = %q, want sonnet", parsed["code"])
	}
}

// TestParseTaskModelOverridesEnv_BadFormat: missing '=' or empty key errors.
func TestParseTaskModelOverridesEnv_BadFormat(t *testing.T) {
	for _, c := range []string{"plan", "=opus", "   =opus"} {
		_, err := governor.ParseTaskModelOverridesEnv(c)
		if err == nil {
			t.Errorf("ParseTaskModelOverridesEnv(%q): expected error", c)
		}
	}
}

// TestParseTaskModelOverridesEnv_EmptyValueDeletes: `plan=` removes prior.
func TestParseTaskModelOverridesEnv_EmptyValueDeletes(t *testing.T) {
	parsed, err := governor.ParseTaskModelOverridesEnv("plan=opus;plan=")
	if err != nil {
		t.Fatalf("parse: %v", err)
	}
	if _, ok := parsed["plan"]; ok {
		t.Errorf("plan still present after empty-value override: %v", parsed)
	}
}
