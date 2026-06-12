// SPDX-License-Identifier: MIT

package controlplane

// Council of Elders control plane (M839): the Web UI consults the multi-model
// panel (kernel/runtime, M837). `council_members` shows which models will speak;
// `council_ask` convenes the panel on a question and returns the deliberation +
// consensus; `council_set` replaces the default membership. The agent reaches the
// same engine through the `council` tool.

import (
	"context"
	"fmt"
	"net"
	"sort"
	"strings"

	"github.com/agezt/agezt/internal/brand"
	"github.com/agezt/agezt/kernel/runtime"
	"github.com/agezt/agezt/kernel/settings"
)

// councilMember is the wire form for a single council seat received from the UI
// (and stored in the settings store as AGEZT_COUNCIL_MEMBERS).
type councilMember struct {
	Seat  string `json:"seat"`
	Model string `json:"model"`
}

func (s *Server) handleCouncilMembers(conn net.Conn, req Request) {
	members := s.k.CouncilDefaultMembers()
	out := make([]map[string]any, 0, len(members))
	for _, m := range members {
		out = append(out, map[string]any{"seat": m.Seat, "model": m.Model})
	}
	s.writeResp(conn, Response{ID: req.ID, Type: RespResult, Result: map[string]any{
		"members": out,
		"count":   len(out),
	}})
}

func (s *Server) handleCouncilAsk(ctx context.Context, conn net.Conn, req Request) {
	question := stringArg(req.Args, "question")
	if question == "" {
		s.writeResp(conn, Response{ID: req.ID, Type: RespError, Error: "args.question required"})
		return
	}
	rounds := dlInt(req.Args, "rounds")
	corr := s.k.NewCorrelation()
	res, err := s.k.Council(ctx, corr, question, nil, rounds)
	if err != nil {
		s.writeResp(conn, Response{ID: req.ID, Type: RespError, Error: err.Error()})
		return
	}
	opinions := make([]map[string]any, 0, len(res.Opinions))
	for _, op := range res.Opinions {
		row := map[string]any{"seat": op.Seat, "model": op.Model, "round": op.Round, "text": op.Text}
		if op.Error != "" {
			row["error"] = op.Error
		}
		opinions = append(opinions, row)
	}
	members := make([]map[string]any, 0, len(res.Members))
	for _, m := range res.Members {
		members = append(members, map[string]any{"seat": m.Seat, "model": m.Model})
	}
	s.writeResp(conn, Response{ID: req.ID, Type: RespResult, Result: map[string]any{
		"correlation_id": corr,
		"question":       res.Question,
		"consensus":      res.Consensus,
		"dissent":        res.Dissent,
		"rounds":         res.Rounds,
		"members":        members,
		"opinions":       opinions,
	}})
}

// handleCouncilSet replaces the default Council membership. args.members is an
// array of {seat, model}. Applies live and persists to the config store so it
// survives restart (stored as AGEZT_COUNCIL_MEMBERS, same format main.go reads).
func (s *Server) handleCouncilSet(conn net.Conn, req Request) {
	raw, ok := req.Args["members"]
	if !ok {
		s.writeResp(conn, Response{ID: req.ID, Type: RespError, Error: "args.members required (array of {seat, model})"})
		return
	}
	arr, ok := raw.([]any)
	if !ok {
		s.writeResp(conn, Response{ID: req.ID, Type: RespError, Error: "args.members must be an array"})
		return
	}

	var parsed []councilMember
	for i, e := range arr {
		obj, ok := e.(map[string]any)
		if !ok {
			s.writeResp(conn, Response{ID: req.ID, Type: RespError, Error: fmt.Sprintf("args.members[%d] must be an object {seat, model}", i)})
			return
		}
		seat, _ := obj["seat"].(string)
		model, _ := obj["model"].(string)
		if model == "" {
			s.writeResp(conn, Response{ID: req.ID, Type: RespError, Error: fmt.Sprintf("args.members[%d].model is required", i)})
			return
		}
		parsed = append(parsed, councilMember{Seat: strings.TrimSpace(seat), Model: strings.TrimSpace(model)})
	}

	// Persist to the config store (survives restart via injection + Open).
	store := settings.NewStore(s.baseDir)
	if err := store.Load(); err != nil {
		s.writeResp(conn, Response{ID: req.ID, Type: RespError, Error: "load config: " + err.Error()})
		return
	}
	envName := brand.EnvPrefix + "COUNCIL_MEMBERS"
	if len(parsed) == 0 {
		store.Remove(envName)
	} else {
		// Sort by seat name for stable output.
		sort.Slice(parsed, func(i, j int) bool { return parsed[i].Seat < parsed[j].Seat })
		models := make([]string, len(parsed))
		for i, m := range parsed {
			models[i] = m.Model
		}
		store.Set(envName, strings.Join(models, ","))
	}
	if err := store.Save(); err != nil {
		s.writeResp(conn, Response{ID: req.ID, Type: RespError, Error: "save config: " + err.Error()})
		return
	}

	// Build the closure and apply live.
	fn := func() []runtime.CouncilMember {
		out := make([]runtime.CouncilMember, len(parsed))
		for i, m := range parsed {
			seat := m.Seat
			if seat == "" {
				seat = fmt.Sprintf("Elder %d", i+1)
			}
			out[i] = runtime.CouncilMember{Seat: seat, Model: m.Model}
		}
		return out
	}
	s.k.SetCouncilMembers(fn)

	// Warn on model ids the catalog doesn't know (don't block).
	var unknown []string
	cat := s.k.Catalog()
	seen := map[string]bool{}
	for _, m := range parsed {
		if seen[m.Model] {
			continue
		}
		seen[m.Model] = true
		if _, mdl := cat.FindModel(m.Model); mdl == nil {
			unknown = append(unknown, m.Model)
		}
	}
	sort.Strings(unknown)
	result := map[string]any{"saved": true, "applied": "live", "member_count": len(parsed)}
	if len(unknown) > 0 {
		result["unknown_models"] = unknown
	}
	s.writeResp(conn, Response{ID: req.ID, Type: RespResult, Result: result})
}
