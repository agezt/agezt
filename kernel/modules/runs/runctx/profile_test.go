// SPDX-License-Identifier: MIT

package runctx

import (
	"strings"
	"testing"

	"github.com/agezt/agezt/kernel/contract/agentapi"
)

// The profile's system prompt is the run's identity: the soul says who the
// agent IS, the instructions say how it works, the task list says what it is
// doing. The renderer moved to the runs module in W3.76c with the vocabulary
// that reads it.

func TestAgentProfileSystem_RendersTheSoul(t *testing.T) {
	got := AgentProfileSystem(agentapi.Profile{Slug: "ops", Soul: "ops soul"})
	if got != "ops soul" {
		t.Fatalf("system prompt = %q, want the soul", got)
	}
}

func TestAgentProfileSystem_EmptyProfileHasNoPrompt(t *testing.T) {
	if got := AgentProfileSystem(agentapi.Profile{Slug: "ops"}); got != "" {
		t.Fatalf("system prompt = %q, want empty", got)
	}
}

// TestAgentProfileSystem_RendersTheInstructions pins the standing-instructions
// section: one bullet per instruction, trimmed, and the section header only
// when there is something to say.
func TestAgentProfileSystem_RendersTheInstructions(t *testing.T) {
	got := AgentProfileSystem(agentapi.Profile{
		Slug:         "ops",
		Soul:         "ops soul",
		Instructions: []string{"ask before deleting", "  ", "never leak secrets"},
	})
	if !strings.Contains(got, "Standing instructions:") {
		t.Fatalf("system prompt = %q, want the instructions section", got)
	}
	if !strings.Contains(got, "- ask before deleting") || !strings.Contains(got, "- never leak secrets") {
		t.Fatalf("system prompt = %q, want both instructions", got)
	}
	if strings.Contains(got, "-  \n") {
		t.Fatalf("system prompt = %q, want the blank instruction dropped", got)
	}
}

// TestAgentProfileSystem_SplitsTheTaskListByScope pins the task section: the
// cycle tasks and the total tasks are separate lists, a DONE or RETIRED task
// is not a task any more, and the section only appears when a live task
// remains.
func TestAgentProfileSystem_SplitsTheTaskListByScope(t *testing.T) {
	got := AgentProfileSystem(agentapi.Profile{
		Slug: "ops",
		TaskList: []agentapi.AgentTask{
			{Title: "cycle task", Scope: "cycle"},
			{Title: "total task"},
			{Title: "finished task", Status: "done"},
			{Title: "retired task", Status: "retired"},
		},
	})
	if !strings.Contains(got, "Cycle tasks:") || !strings.Contains(got, "- [todo] cycle task") {
		t.Fatalf("system prompt = %q, want the cycle task", got)
	}
	if !strings.Contains(got, "Total tasks:") || !strings.Contains(got, "- [todo] total task") {
		t.Fatalf("system prompt = %q, want the total task", got)
	}
	if strings.Contains(got, "finished task") || strings.Contains(got, "retired task") {
		t.Fatalf("system prompt = %q, want the finished tasks dropped", got)
	}
}

// TestAgentProfileSystem_DropsAnAllDoneTaskList pins the consequence of the
// rule above: a task list with nothing live renders no section at all.
func TestAgentProfileSystem_DropsAnAllDoneTaskList(t *testing.T) {
	got := AgentProfileSystem(agentapi.Profile{
		Slug:     "ops",
		TaskList: []agentapi.AgentTask{{Title: "finished task", Status: "done"}},
	})
	if strings.Contains(got, "tasks:") {
		t.Fatalf("system prompt = %q, want no task section", got)
	}
}

// TestAgentProfileSystem_CapsTheRenderedTaskList pins the cap: a long task
// list is truncated with an ellipsis rather than flooding the system prompt.
func TestAgentProfileSystem_CapsTheRenderedTaskList(t *testing.T) {
	var tasks []agentapi.AgentTask
	for i := 0; i < 30; i++ {
		tasks = append(tasks, agentapi.AgentTask{Title: "task " + strings.Repeat("x", 3)})
	}
	got := AgentProfileSystem(agentapi.Profile{Slug: "ops", TaskList: tasks})
	if !strings.Contains(got, "- ...") {
		t.Fatalf("system prompt = %q, want the truncation marker", got)
	}
	if strings.Count(got, "- [todo]") != 20 {
		t.Fatalf("system prompt rendered %d tasks, want 20", strings.Count(got, "- [todo]"))
	}
}

// TestAgentProfileSystem_RendersTheTaskDescription pins the description half
// of a task line: it rides after the title, separated by a dash.
func TestAgentProfileSystem_RendersTheTaskDescription(t *testing.T) {
	got := AgentProfileSystem(agentapi.Profile{
		Slug: "ops",
		TaskList: []agentapi.AgentTask{
			{Title: "ship it", Description: "the whole thing", Status: "doing"},
		},
	})
	if !strings.Contains(got, "- [doing] ship it - the whole thing") {
		t.Fatalf("system prompt = %q, want the described task", got)
	}
}
