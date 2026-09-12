// SPDX-License-Identifier: MIT

// Control-plane roster task-update handler + hasArg + taskFieldPresent helpers.
// Code extracted from roster_crud.go during the Day-116 god-file split.
// Public API unchanged.
package controlplane


import (
	"net"
	"strings"

	"encoding/json"
	"github.com/agezt/agezt/kernel/roster"
)

func (s *Server) handleAgentTaskUpdate(conn net.Conn, req Request) {
	ref, err := requiredArgString(req.Args, "ref")
	if err != nil {
		s.fail(conn, req, err)
		return
	}
	op, _, err := argString(req.Args, "op")
	if err != nil {
		s.fail(conn, req, err)
		return
	}
	op = strings.ToLower(strings.TrimSpace(op))
	if op == "" {
		op = "update"
	}
	if op != "add" && op != "update" && op != "remove" && op != "delete" {
		s.failMsg(conn, req, "args.op must be add, update, or remove")
		return
	}
	var in roster.AgentTask
	if raw, ok := req.Args["task"]; ok {
		b, err := json.Marshal(raw)
		if err != nil {
			s.failMsg(conn, req, "args.task: "+err.Error())
			return
		}
		if err := json.Unmarshal(b, &in); err != nil {
			s.failMsg(conn, req, "args.task: "+err.Error())
			return
		}
	}
	// Flat-arg overrides layered over args.task (both transports are live).
	for _, f := range []struct {
		key string
		dst *string
	}{
		{"id", &in.ID}, {"title", &in.Title}, {"description", &in.Description},
		{"scope", &in.Scope}, {"status", &in.Status},
	} {
		if v, present, err := argString(req.Args, f.key); err != nil {
			s.fail(conn, req, err)
			return
		} else if present {
			*f.dst = v
		}
	}
	titleProvided := hasArg(req.Args, "title") || taskFieldPresent(req.Args["task"], "title")
	scopeProvided := hasArg(req.Args, "scope") || taskFieldPresent(req.Args["task"], "scope")
	statusProvided := hasArg(req.Args, "status") || taskFieldPresent(req.Args["task"], "status")
	if op == "add" || (op == "update" && titleProvided) {
		if strings.TrimSpace(in.Title) == "" {
			s.writeResp(conn, Response{ID: req.ID, Type: RespError, Error: "args.title required"})
			return
		}
	}
	if scopeProvided {
		switch strings.TrimSpace(in.Scope) {
		case "", "cycle", "total":
		default:
			s.writeResp(conn, Response{ID: req.ID, Type: RespError, Error: "args.scope must be cycle or total"})
			return
		}
	}
	if statusProvided {
		switch strings.TrimSpace(in.Status) {
		case "", "todo", "doing", "done", "blocked", "retired":
		default:
			s.writeResp(conn, Response{ID: req.ID, Type: RespError, Error: "args.status must be todo, doing, done, blocked, or retired"})
			return
		}
	}
	var task roster.AgentTask
	found := false
	p, exists, err := s.k.UpdateProfile(ref, func(dst *roster.Profile) {
		switch op {
		case "add":
			task = in
			dst.TaskList = append(dst.TaskList, task)
			found = true
		case "update":
			id := strings.TrimSpace(in.ID)
			if id == "" {
				return
			}
			for i := range dst.TaskList {
				if dst.TaskList[i].ID != id {
					continue
				}
				if _, ok := req.Args["title"]; ok || in.Title != "" {
					dst.TaskList[i].Title = in.Title
				}
				if _, ok := req.Args["description"]; ok || in.Description != "" {
					dst.TaskList[i].Description = in.Description
				}
				if _, ok := req.Args["scope"]; ok || in.Scope != "" {
					dst.TaskList[i].Scope = in.Scope
				}
				if _, ok := req.Args["status"]; ok || in.Status != "" {
					dst.TaskList[i].Status = in.Status
				}
				task = dst.TaskList[i]
				found = true
				return
			}
		case "remove", "delete":
			id := strings.TrimSpace(in.ID)
			if id == "" {
				return
			}
			for i := range dst.TaskList {
				if dst.TaskList[i].ID != id {
					continue
				}
				task = dst.TaskList[i]
				dst.TaskList = append(append([]roster.AgentTask{}, dst.TaskList[:i]...), dst.TaskList[i+1:]...)
				found = true
				return
			}
		}
	})
	if err != nil {
		s.fail(conn, req, err)
		return
	}
	if !exists {
		s.writeResp(conn, Response{ID: req.ID, Type: RespError, Error: "unknown agent: " + ref})
		return
	}
	if !found {
		if strings.TrimSpace(in.ID) == "" && op != "add" {
			s.writeResp(conn, Response{ID: req.ID, Type: RespError, Error: "args.id required"})
			return
		}
		if op == "add" {
			s.writeResp(conn, Response{ID: req.ID, Type: RespError, Error: "args.title required"})
			return
		}
		s.writeResp(conn, Response{ID: req.ID, Type: RespError, Error: "unknown agent task: " + in.ID})
		return
	}
	if op == "add" {
		for _, t := range p.TaskList {
			if t.Title == strings.TrimSpace(in.Title) && (strings.TrimSpace(in.ID) == "" || t.ID == strings.TrimSpace(in.ID)) {
				task = t
			}
		}
	}
	s.writeResp(conn, Response{ID: req.ID, Type: RespResult, Result: map[string]any{
		"updated": true,
		"profile": profileView(p),
		"task":    task,
	}})
}

func hasArg(args map[string]any, key string) bool {
	_, ok := args[key]
	return ok
}

func taskFieldPresent(raw any, key string) bool {
	obj, ok := raw.(map[string]any)
	if !ok {
		return false
	}
	_, ok = obj[key]
	return ok
}

// handleAgentImpact reports what depends on an agent — shown before retiring or
// removing so the operator sees the effects (M846).
