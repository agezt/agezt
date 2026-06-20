// SPDX-License-Identifier: MIT

package cadence

import (
	"encoding/json"
	"testing"
	"time"
)

func TestEntryValidate_ValidIntent(t *testing.T) {
	e := Entry{ID: "s1", Target: TargetIntent, Intent: "check mail"}
	if err := e.Validate(); err != nil {
		t.Fatalf("valid intent entry: %v", err)
	}
}

func TestEntryValidate_ValidSystemTask(t *testing.T) {
	e := Entry{ID: "s2", Target: TargetSystemTask, SystemTask: SystemTaskCatalogSync}
	if err := e.Validate(); err != nil {
		t.Fatalf("valid system_task entry: %v", err)
	}
}

func TestEntryValidate_ValidWorkflow(t *testing.T) {
	e := Entry{ID: "s3", Target: TargetWorkflow, Workflow: "daily-flow"}
	if err := e.Validate(); err != nil {
		t.Fatalf("valid workflow entry: %v", err)
	}
}

func TestEntryValidate_ValidTool(t *testing.T) {
	e := Entry{ID: "s4", Target: TargetTool, Tool: "http"}
	if err := e.Validate(); err != nil {
		t.Fatalf("valid tool entry: %v", err)
	}
}

func TestEntryValidate_SystemTaskWithPayload(t *testing.T) {
	e := Entry{ID: "s5", Target: TargetSystemTask, SystemTask: SystemTaskCatalogSync, Payload: json.RawMessage(`{"evil":"rm -rf /"}`)}
	if err := e.Validate(); err == nil {
		t.Fatal("system_task with payload should fail validation")
	}
}

func TestEntryValidate_SystemTaskWithAgent(t *testing.T) {
	e := Entry{ID: "s6", Target: TargetSystemTask, SystemTask: SystemTaskLogClean, Agent: "researcher"}
	if err := e.Validate(); err == nil {
		t.Fatal("system_task with agent should fail validation")
	}
}

func TestEntryValidate_SystemTaskUnknown(t *testing.T) {
	e := Entry{ID: "s7", Target: TargetSystemTask, SystemTask: "rm -rf /"}
	if err := e.Validate(); err == nil {
		t.Fatal("unknown system_task should fail validation")
	}
}

func TestEntryValidate_WorkflowWithSystemTask(t *testing.T) {
	e := Entry{ID: "s8", Target: TargetWorkflow, Workflow: "flow", SystemTask: SystemTaskCatalogSync}
	if err := e.Validate(); err == nil {
		t.Fatal("workflow + system_task should fail validation")
	}
}

func TestEntryValidate_ToolWithWorkflow(t *testing.T) {
	e := Entry{ID: "s9", Target: TargetTool, Tool: "http", Workflow: "flow"}
	if err := e.Validate(); err == nil {
		t.Fatal("tool + workflow should fail validation")
	}
}

func TestEntryValidate_IntentWithTypedField(t *testing.T) {
	e := Entry{ID: "s10", Target: TargetIntent, Tool: "http"}
	if err := e.Validate(); err == nil {
		t.Fatal("intent target with tool field should fail validation")
	}
}

func TestEntryValidate_UnknownTarget(t *testing.T) {
	e := Entry{ID: "s11", Target: "malicious"}
	if err := e.Validate(); err == nil {
		t.Fatal("unknown target should fail validation")
	}
}

func TestSetSystemTaskTarget_RejectsUnknownTask(t *testing.T) {
	s := mustStore(t)
	e, err := s.Add("test", 3600*time.Second, "", SourceOperator, time.Now())
	if err != nil {
		t.Fatalf("Add: %v", err)
	}
	ok, err := s.SetSystemTaskTarget(e.ID, "rm -rf /")
	if ok {
		t.Fatal("SetSystemTaskTarget should return false for unknown task")
	}
	if err == nil {
		t.Fatal("SetSystemTaskTarget should error for unknown task")
	}
	// Verify the entry was NOT modified.
	got, found := s.Get(e.ID)
	if !found {
		t.Fatal("entry disappeared")
	}
	if got.Target == TargetSystemTask {
		t.Fatal("entry target should not have been set to system_task")
	}
}

func TestSetSystemTaskTarget_AcceptsKnownTask(t *testing.T) {
	s := mustStore(t)
	e, err := s.Add("test", 3600*time.Second, "", SourceOperator, time.Now())
	if err != nil {
		t.Fatalf("Add: %v", err)
	}
	ok, err := s.SetSystemTaskTarget(e.ID, SystemTaskCatalogSync)
	if !ok || err != nil {
		t.Fatalf("SetSystemTaskTarget(catalog_sync) = %v %v, want true nil", ok, err)
	}
	got, _ := s.Get(e.ID)
	if got.Target != TargetSystemTask || got.SystemTask != SystemTaskCatalogSync {
		t.Fatalf("got target=%q system_task=%q, want system_task/catalog_sync", got.Target, got.SystemTask)
	}
}
