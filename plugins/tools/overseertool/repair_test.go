// SPDX-License-Identifier: MIT

package overseertool

import (
	"strings"
	"testing"

	"github.com/agezt/agezt/internal/brand"
	"github.com/agezt/agezt/kernel/governor"
	"github.com/agezt/agezt/kernel/roster"
	kernelruntime "github.com/agezt/agezt/kernel/runtime"
	"github.com/agezt/agezt/kernel/settings"
)

func TestParseRepairProposal_RoutingAwareFields(t *testing.T) {
	prop := parseRepairProposal("done\n```json\n{\n  \"task_type\": \" code \",\n  \"task_model_chain\": [\" gpt-5 \", \"\", \" gpt-4.1 \"],\n  \"config_overrides\": {\" agezt_max_iter \": \" 6 \"}\n}\n```")
	if prop == nil {
		t.Fatal("proposal = nil")
	}
	if prop.TaskType != "code" {
		t.Fatalf("task_type = %q, want code", prop.TaskType)
	}
	if len(prop.TaskModelChain) != 2 || prop.TaskModelChain[0] != "gpt-5" || prop.TaskModelChain[1] != "gpt-4.1" {
		t.Fatalf("task_model_chain = %#v", prop.TaskModelChain)
	}
	if prop.ConfigOverrides["AGEZT_MAX_ITER"] != "6" {
		t.Fatalf("config overrides = %#v", prop.ConfigOverrides)
	}
}

func TestApplyRepairProposal_AppliesTaskType(t *testing.T) {
	dst := &roster.Profile{TaskType: "research"}
	applied := applyRepairProposal(dst, &repairProposal{TaskType: "code"})
	if dst.TaskType != "code" {
		t.Fatalf("dst task_type = %q, want code", dst.TaskType)
	}
	if len(applied) != 1 || applied[0] != "task_type" {
		t.Fatalf("applied = %#v", applied)
	}
}

func TestBuildRepairBrief_IncludesRoutingEvidenceAndCurrentChain(t *testing.T) {
	brief := buildRepairBrief(
		roster.Profile{Slug: "builder", TaskType: "code", Model: "gpt-5"},
		kernelruntime.ReaperReport{
			RoutingPressure: []kernelruntime.RoutingPressureAgent{{
				Slug:            "builder",
				Count:           4,
				Threshold:       3,
				WindowSec:       86400,
				TaskType:        "code",
				LastFailedModel: "gpt-5",
				LastNextModel:   "gpt-4.1",
				LastReason:      "provider overloaded",
			}},
		},
		"recent routing churn",
		[]string{"gpt-5", "gpt-4.1", "deepseek-chat"},
	)
	for _, want := range []string{
		"task_model_chain=gpt-5 -> gpt-4.1 -> deepseek-chat",
		"Model-chain fallback pressure: 4 fallback hop(s)",
		"last hop: gpt-5 -> gpt-4.1",
		"task_model_chain",
	} {
		if !strings.Contains(brief, want) {
			t.Fatalf("brief missing %q\n%s", want, brief)
		}
	}
}

func TestRepairProposalTaskType_FallsBackToExistingProfile(t *testing.T) {
	got := repairProposalTaskType(roster.Profile{TaskType: "research"}, &repairProposal{TaskModelChain: []string{"gpt-5"}})
	if got != "research" {
		t.Fatalf("task type = %q, want research", got)
	}
}

func TestPersistTaskModelChains_SavesGovernorSpec(t *testing.T) {
	baseDir := t.TempDir()
	chains := map[string][]string{
		"code": []string{"gpt-5", "gpt-4.1"},
		"chat": []string{"claude-opus-4-7", "gpt-5"},
	}
	if err := persistTaskModelChains(baseDir, chains); err != nil {
		t.Fatalf("persistTaskModelChains: %v", err)
	}
	store := settings.NewStore(baseDir)
	if err := store.Load(); err != nil {
		t.Fatalf("Load: %v", err)
	}
	raw, ok := store.Get(brand.EnvPrefix + "TASK_MODEL_CHAINS")
	if !ok {
		t.Fatal("AGEZT_TASK_MODEL_CHAINS missing from settings store")
	}
	parsed, err := governor.ParseTaskModelChainsEnv(raw)
	if err != nil {
		t.Fatalf("ParseTaskModelChainsEnv: %v", err)
	}
	if len(parsed["code"]) != 2 || parsed["code"][1] != "gpt-4.1" {
		t.Fatalf("parsed code chain = %#v", parsed["code"])
	}
	if len(parsed["chat"]) != 2 || parsed["chat"][0] != "claude-opus-4-7" {
		t.Fatalf("parsed chat chain = %#v", parsed["chat"])
	}
}
