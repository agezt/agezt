// SPDX-License-Identifier: MIT

package roster

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"reflect"
	"strings"

	"github.com/agezt/agezt/kernel/app"
	"github.com/agezt/agezt/kernel/contract/opapi"
	"github.com/agezt/agezt/kernel/platform/schema"
	core "github.com/agezt/agezt/kernel/roster"
)

// TaskUpdateRequest keeps every argument raw: args.task and the flat id/title/
// description/scope/status arguments are both live transports, and presence
// (not value) decides which fields an update touches.
type TaskUpdateRequest struct {
	Ref         json.RawMessage `json:"ref,omitempty"`
	Op          json.RawMessage `json:"op,omitempty"`
	Task        json.RawMessage `json:"task,omitempty"`
	ID          json.RawMessage `json:"id,omitempty"`
	Title       json.RawMessage `json:"title,omitempty"`
	Description json.RawMessage `json:"description,omitempty"`
	Scope       json.RawMessage `json:"scope,omitempty"`
	Status      json.RawMessage `json:"status,omitempty"`
}

func optionalString(raw json.RawMessage, key string) (string, bool, error) {
	v, present := rawValue(raw)
	if !present {
		return "", false, nil
	}
	s, ok := v.(string)
	if !ok {
		return "", true, fmt.Errorf("args.%s must be a string", key)
	}
	return s, true, nil
}

func taskFieldPresent(raw json.RawMessage, key string) bool {
	var obj map[string]json.RawMessage
	if json.Unmarshal(raw, &obj) != nil || obj == nil {
		return false
	}
	_, ok := obj[key]
	return ok
}

type TaskUpdateOutput struct {
	Updated bool           `json:"updated"`
	Profile ProfileOutput  `json:"profile"`
	Task    core.AgentTask `json:"task"`
}

// TaskUpdateService adds, updates or removes one entry of an agent's durable
// tasklist through the kernel's journaled profile update.
type TaskUpdateService struct {
	update func(string, func(*core.Profile)) (core.Profile, bool, error)
}

func NewTaskUpdate(update func(string, func(*core.Profile)) (core.Profile, bool, error)) *TaskUpdateService {
	return &TaskUpdateService{update: update}
}

func (s *TaskUpdateService) TaskUpdate(_ context.Context, in TaskUpdateRequest) (TaskUpdateOutput, error) {
	ref, err := RefPageRequest{Ref: in.Ref}.ref()
	if err != nil {
		return TaskUpdateOutput{}, err
	}
	op, _, err := optionalString(in.Op, "op")
	if err != nil {
		return TaskUpdateOutput{}, err
	}
	op = strings.ToLower(strings.TrimSpace(op))
	if op == "" {
		op = "update"
	}
	if op != "add" && op != "update" && op != "remove" && op != "delete" {
		return TaskUpdateOutput{}, errors.New("args.op must be add, update, or remove")
	}
	var task core.AgentTask
	if len(in.Task) > 0 {
		if err := json.Unmarshal(in.Task, &task); err != nil {
			return TaskUpdateOutput{}, errors.New("args.task: " + err.Error())
		}
	}
	// Flat-arg overrides layered over args.task.
	flat := map[string]bool{}
	for _, f := range []struct {
		key string
		raw json.RawMessage
		dst *string
	}{
		{"id", in.ID, &task.ID}, {"title", in.Title, &task.Title}, {"description", in.Description, &task.Description},
		{"scope", in.Scope, &task.Scope}, {"status", in.Status, &task.Status},
	} {
		v, present, err := optionalString(f.raw, f.key)
		if err != nil {
			return TaskUpdateOutput{}, err
		}
		if present {
			*f.dst = v
			flat[f.key] = true
		}
	}
	titleProvided := flat["title"] || taskFieldPresent(in.Task, "title")
	scopeProvided := flat["scope"] || taskFieldPresent(in.Task, "scope")
	statusProvided := flat["status"] || taskFieldPresent(in.Task, "status")
	if op == "add" || (op == "update" && titleProvided) {
		if strings.TrimSpace(task.Title) == "" {
			return TaskUpdateOutput{}, errors.New("args.title required")
		}
	}
	if scopeProvided {
		switch strings.TrimSpace(task.Scope) {
		case "", "cycle", "total":
		default:
			return TaskUpdateOutput{}, errors.New("args.scope must be cycle or total")
		}
	}
	if statusProvided {
		switch strings.TrimSpace(task.Status) {
		case "", "todo", "doing", "done", "blocked", "retired":
		default:
			return TaskUpdateOutput{}, errors.New("args.status must be todo, doing, done, blocked, or retired")
		}
	}
	var result core.AgentTask
	found := false
	p, exists, err := s.update(ref, func(dst *core.Profile) {
		switch op {
		case "add":
			result = task
			dst.TaskList = append(dst.TaskList, result)
			found = true
		case "update":
			id := strings.TrimSpace(task.ID)
			if id == "" {
				return
			}
			for i := range dst.TaskList {
				if dst.TaskList[i].ID != id {
					continue
				}
				if flat["title"] || task.Title != "" {
					dst.TaskList[i].Title = task.Title
				}
				if flat["description"] || task.Description != "" {
					dst.TaskList[i].Description = task.Description
				}
				if flat["scope"] || task.Scope != "" {
					dst.TaskList[i].Scope = task.Scope
				}
				if flat["status"] || task.Status != "" {
					dst.TaskList[i].Status = task.Status
				}
				result = dst.TaskList[i]
				found = true
				return
			}
		case "remove", "delete":
			id := strings.TrimSpace(task.ID)
			if id == "" {
				return
			}
			for i := range dst.TaskList {
				if dst.TaskList[i].ID != id {
					continue
				}
				result = dst.TaskList[i]
				dst.TaskList = append(append([]core.AgentTask{}, dst.TaskList[:i]...), dst.TaskList[i+1:]...)
				found = true
				return
			}
		}
	})
	if err != nil {
		return TaskUpdateOutput{}, err
	}
	if !exists {
		return TaskUpdateOutput{}, errors.New("unknown agent: " + ref)
	}
	if !found {
		if strings.TrimSpace(task.ID) == "" && op != "add" {
			return TaskUpdateOutput{}, errors.New("args.id required")
		}
		if op == "add" {
			return TaskUpdateOutput{}, errors.New("args.title required")
		}
		return TaskUpdateOutput{}, errors.New("unknown agent task: " + task.ID)
	}
	if op == "add" {
		// Report the stored task, with the id and timestamps the store assigned.
		for _, t := range p.TaskList {
			if t.Title == strings.TrimSpace(task.Title) && (strings.TrimSpace(task.ID) == "" || t.ID == strings.TrimSpace(task.ID)) {
				result = t
			}
		}
	}
	return TaskUpdateOutput{Updated: true, Profile: ProfileOutput{Profile: p, Kind: p.Kind(), Managed: !p.AllowsDirectCall()}, Task: result}, nil
}

type taskUpdateSchema struct {
	Updated bool           `json:"updated"`
	Profile profileSchema  `json:"profile"`
	Task    core.AgentTask `json:"task"`
}

func TaskUpdateOperations(provider func(context.Context) *TaskUpdateService) ([]app.Operation, error) {
	if provider == nil {
		return nil, errors.New("roster task update provider required")
	}
	output, err := schema.FromType(reflect.TypeFor[taskUpdateSchema](), false)
	if err != nil {
		return nil, err
	}
	op, err := app.NewOperation(opapi.Spec{Name: "agent_task_update", OutputSchema: output, Authz: opapi.PrimaryOnly, Tenancy: opapi.Primary, AllowUnknownInput: true, InputSchema: json.RawMessage(`{"type":"object","additionalProperties":true,"properties":{"ref":{},"op":{},"task":{},"id":{},"title":{},"description":{},"scope":{},"status":{}}}`), HTTP: opapi.HTTP{Method: "POST", Path: "/api/agents/task"}}, func(ctx context.Context, in TaskUpdateRequest) (TaskUpdateOutput, error) {
		return provider(ctx).TaskUpdate(ctx, in)
	})
	if err != nil {
		return nil, err
	}
	return []app.Operation{op}, nil
}
