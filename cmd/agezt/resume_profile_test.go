// SPDX-License-Identifier: MIT

package main

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/agezt/agezt/kernel/contract/llm"
	"github.com/agezt/agezt/kernel/contract/toolapi"
	"github.com/agezt/agezt/kernel/edict"
	"github.com/agezt/agezt/kernel/resume"
	"github.com/agezt/agezt/kernel/roster"
	kernelruntime "github.com/agezt/agezt/kernel/runtime"
	"github.com/agezt/agezt/plugins/providers/mock"
)

type resumeBlockingProvider struct{ started chan llm.CompletionRequest }

type resumeProviderFunc func(context.Context, llm.CompletionRequest) (*llm.CompletionResponse, error)

func (resumeProviderFunc) Name() string { return "resume-observer" }
func (f resumeProviderFunc) Complete(ctx context.Context, req llm.CompletionRequest) (*llm.CompletionResponse, error) {
	return f(ctx, req)
}

func (*resumeBlockingProvider) Name() string { return "resume-test" }
func (p *resumeBlockingProvider) Complete(ctx context.Context, req llm.CompletionRequest) (*llm.CompletionResponse, error) {
	p.started <- req
	<-ctx.Done()
	return nil, ctx.Err()
}

func TestBuildResumer_QuarantinesUnsafeProfileTickets(t *testing.T) {
	for _, reason := range []string{"disabled", "retired", "missing", "attempt-cap", "explicit-override"} {
		t.Run(reason, func(t *testing.T) {
			t.Setenv("AGEZT_RESUME_MAX_ATTEMPTS", "3")
			prov := mock.New(mock.FinalText("must not run"))
			k, err := kernelruntime.Open(kernelruntime.Config{BaseDir: t.TempDir(), Provider: prov, ResumeEnabled: true})
			if err != nil {
				t.Fatal(err)
			}
			t.Cleanup(func() { k.Close() })
			if _, err := k.AddProfile(roster.Profile{Slug: "ops", Soul: "ops soul", Model: "ops-model"}); err != nil {
				t.Fatal(err)
			}
			ticket := &resume.Ticket{Corr: "unsafe", AgentSlug: "ops", Intent: "task", Kind: resume.KindRun, Resumable: true, Status: resume.StatusSuspended}
			switch reason {
			case "disabled":
				_, err = k.Roster().SetEnabled("ops", false)
			case "retired":
				_, err = k.Roster().SetRetired("ops", true)
			case "missing":
				ticket.AgentSlug = "gone"
			case "attempt-cap":
				ticket.Attempts = 3
			case "explicit-override":
				ticket.Resumable = false
			}
			if err != nil {
				t.Fatal(err)
			}
			if err := k.ResumeStore().Put(ticket); err != nil {
				t.Fatal(err)
			}
			if result := buildResumer(context.Background(), k); result != "none resumable (1 quarantined)" {
				t.Fatalf("boot resume=%q", result)
			}
			if prov.CallCount() != 0 {
				t.Fatal("unsafe ticket reached provider")
			}
			if _, exists, err := k.ResumeStore().Get(ticket.Corr); exists || err != nil {
				t.Fatalf("ticket still active: exists=%v err=%v", exists, err)
			}
			files, err := os.ReadDir(filepath.Join(k.ResumeStore().Dir(), "quarantine"))
			if err != nil || len(files) != 1 {
				t.Fatalf("quarantine evidence: files=%v err=%v", files, err)
			}
		})
	}
}

// Interrupt a real profiled run, reopen its stores and invoke the actual boot
// resumer. Its first provider call must observe the attempt already persisted.
func TestBuildResumer_ProfileRunSurvivesRestart(t *testing.T) {
	for _, kind := range []string{"soul", "model", "both"} {
		t.Run(kind, func(t *testing.T) {
			home := t.TempDir()
			prov := &resumeBlockingProvider{started: make(chan llm.CompletionRequest, 1)}
			cfg := kernelruntime.Config{BaseDir: home, Provider: prov, Model: "default-model", ResumeEnabled: true,
				Tools: map[string]toolapi.Tool{"probe": &scheduledProbeTool{}},
				Edict: edict.New(edict.Options{Levels: map[edict.Capability]edict.TrustLevel{edict.CapShell: edict.LevelAllow}, AskPolicy: edict.AskAllow})}
			k, err := kernelruntime.Open(cfg)
			if err != nil {
				t.Fatal(err)
			}
			t.Cleanup(func() { k.Close() })
			profile := roster.Profile{Slug: "resume-agent", Enabled: true, ToolDeny: []string{"probe"}, TrustCeiling: "L2"}
			if kind != "model" {
				profile.Soul = "durable agent soul"
			}
			if kind != "soul" {
				profile.Model = "agent-model"
			}
			profile, err = k.AddProfile(profile)
			if err != nil {
				t.Fatal(err)
			}
			ctx, cancel := context.WithCancel(context.Background())
			t.Cleanup(cancel)
			ctx = kernelruntime.WithAgentProfile(ctx, profile)
			ctx = kernelruntime.WithTrustCeiling(ctx, edict.LevelDeny)
			ctx = kernelruntime.WithMaxCost(ctx, 321)
			const corr = "profile-restart"
			done := make(chan error, 1)
			go func() { _, err := k.RunWith(ctx, corr, "continue the task"); done <- err }()
			select {
			case <-prov.started:
			case <-time.After(3 * time.Second):
				t.Fatal("run did not start")
			}
			if k.Suspend("test restart") != 1 {
				t.Fatal("run was not suspended")
			}
			cancel()
			select {
			case <-done:
			case <-time.After(3 * time.Second):
				t.Fatal("run did not stop")
			}
			if err := k.Close(); err != nil {
				t.Fatal(err)
			}

			type observation struct {
				req           llm.CompletionRequest
				ticket        *resume.Ticket
				err           error
				cost          int64
				ceilingDenied bool
			}
			observed := make(chan observation, 1)
			var restarted *kernelruntime.Kernel
			finisher := resumeProviderFunc(func(ctx context.Context, req llm.CompletionRequest) (*llm.CompletionResponse, error) {
				ticket, _, err := restarted.ResumeStore().Get(corr)
				verdict := restarted.CheckPolicy(ctx, llm.ToolCall{Name: "shell", Input: []byte(`{"command":"echo probe"}`)})
				observed <- observation{req: req, ticket: ticket, err: err, cost: restarted.MaxCostFromCtx(ctx), ceilingDenied: !verdict.Allow && strings.Contains(verdict.Reason, "ceiling")}
				response := mock.FinalText("resumed")
				return &response, nil
			})
			cfg.Provider = finisher
			restarted, err = kernelruntime.Open(cfg)
			if err != nil {
				t.Fatal(err)
			}
			t.Cleanup(func() { restarted.Close() })
			// The stored tighter run ceiling must survive even if the current profile
			// is looser after boot. Profile restrictions still apply independently.
			if _, err := restarted.Roster().Update(profile.Slug, func(p *roster.Profile) { p.TrustCeiling = "L4" }); err != nil {
				t.Fatal(err)
			}
			if result := buildResumer(context.Background(), restarted); result != "1 run(s) resumed" {
				t.Fatalf("boot resume = %q", result)
			}
			var obs observation
			select {
			case obs = <-observed:
			case <-time.After(3 * time.Second):
				t.Fatal("resumed run did not reach provider")
			}
			if obs.err != nil || obs.ticket == nil || obs.ticket.Attempts != 1 {
				t.Fatalf("attempt not durable before dispatch: %+v, %v", obs.ticket, obs.err)
			}
			if obs.ticket.TrustCeiling == nil || *obs.ticket.TrustCeiling != int(edict.LevelDeny) || obs.ticket.MaxCostMc != 321 || obs.cost != 321 || !obs.ceilingDenied {
				t.Fatalf("governance lost: %+v", obs.ticket)
			}
			wantModel := "default-model"
			if profile.Model != "" {
				wantModel = profile.Model
			}
			if obs.req.Model != wantModel || obs.req.CorrelationID != corr {
				t.Errorf("model=%q corr=%q", obs.req.Model, obs.req.CorrelationID)
			}
			for _, tool := range obs.req.Tools {
				if tool.Name == "probe" {
					t.Error("resumed agent regained denied tool")
				}
			}
			if profile.Soul != "" {
				soul := strings.Contains(obs.req.System, profile.Soul)
				for _, msg := range obs.req.Messages {
					soul = soul || strings.Contains(msg.Content, profile.Soul)
				}
				if !soul {
					t.Error("resumed agent lost soul")
				}
			}
			deadline := time.Now().Add(3 * time.Second)
			for {
				_, exists, err := restarted.ResumeStore().Get(corr)
				if err != nil {
					t.Fatal(err)
				}
				if !exists {
					break
				}
				if time.Now().After(deadline) {
					t.Fatal("completed resume ticket was not cleared")
				}
				time.Sleep(10 * time.Millisecond)
			}
		})
	}
}
