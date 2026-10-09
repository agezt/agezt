// SPDX-License-Identifier: MIT

// Package council owns the Council of Elders' default membership (M839): which
// models speak when the multi-model panel is convened without an explicit
// panel. Convening the panel itself stays a streaming command.
package council

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"reflect"
	"sort"
	"strings"

	"github.com/agezt/agezt/internal/brand"
	"github.com/agezt/agezt/kernel/app"
	"github.com/agezt/agezt/kernel/contract/opapi"
	"github.com/agezt/agezt/kernel/platform/schema"
)

// Member is one seat on the panel and the model that speaks for it.
type Member struct {
	Seat  string `json:"seat"`
	Model string `json:"model"`
}

// Store is the config store the membership persists to.
type Store interface {
	Load() error
	Set(name, value string)
	Remove(name string) bool
	Save() error
}

// Ports reads the kernel's default panel, replaces it live, opens the config
// store and looks models up in one catalog snapshot.
type Ports struct {
	Members    func() []Member
	SetMembers func([]Member)
	Store      func() Store
	Models     func() func(model string) bool
}

type Service struct{ ports Ports }

func New(ports Ports) *Service { return &Service{ports: ports} }

type MembersRequest struct{}

type MembersOutput struct {
	Members []Member `json:"members"`
	Count   int      `json:"count"`
}

func (s *Service) Members(_ context.Context, _ MembersRequest) (MembersOutput, error) {
	members := append([]Member{}, s.ports.Members()...)
	return MembersOutput{Members: members, Count: len(members)}, nil
}

type SetRequest struct {
	Members json.RawMessage `json:"members,omitempty"`
}

type SetOutput struct {
	Saved         bool     `json:"saved"`
	Applied       string   `json:"applied"`
	MemberCount   int      `json:"member_count"`
	UnknownModels []string `json:"unknown_models,omitempty"`
}

// Set replaces the default membership. Each entry needs a model; seats and
// models are trimmed. The models are persisted, in seat order, as
// AGEZT_COUNCIL_MEMBERS (removed when empty) and the panel applies live with a
// blank seat named after its position. Models the catalog does not know are
// reported, never refused.
func (s *Service) Set(_ context.Context, in SetRequest) (SetOutput, error) {
	if len(in.Members) == 0 {
		return SetOutput{}, errors.New("args.members required (array of {seat, model})")
	}
	var v any
	_ = json.Unmarshal(in.Members, &v)
	arr, ok := v.([]any)
	if !ok {
		return SetOutput{}, errors.New("args.members must be an array")
	}
	var parsed []Member
	for i, e := range arr {
		obj, ok := e.(map[string]any)
		if !ok {
			return SetOutput{}, fmt.Errorf("args.members[%d] must be an object {seat, model}", i)
		}
		seat, _ := obj["seat"].(string)
		model, _ := obj["model"].(string)
		if model == "" {
			return SetOutput{}, fmt.Errorf("args.members[%d].model is required", i)
		}
		parsed = append(parsed, Member{Seat: strings.TrimSpace(seat), Model: strings.TrimSpace(model)})
	}
	store := s.ports.Store()
	if err := store.Load(); err != nil {
		return SetOutput{}, errors.New("load config: " + err.Error())
	}
	name := brand.EnvPrefix + "COUNCIL_MEMBERS"
	if len(parsed) == 0 {
		store.Remove(name)
	} else {
		sort.Slice(parsed, func(i, j int) bool { return parsed[i].Seat < parsed[j].Seat })
		models := make([]string, len(parsed))
		for i, m := range parsed {
			models[i] = m.Model
		}
		store.Set(name, strings.Join(models, ","))
	}
	if err := store.Save(); err != nil {
		return SetOutput{}, errors.New("save config: " + err.Error())
	}
	live := make([]Member, len(parsed))
	for i, m := range parsed {
		if m.Seat == "" {
			m.Seat = fmt.Sprintf("Elder %d", i+1)
		}
		live[i] = m
	}
	s.ports.SetMembers(live)
	known := s.ports.Models()
	var unknown []string
	seen := map[string]bool{}
	for _, m := range parsed {
		if seen[m.Model] {
			continue
		}
		seen[m.Model] = true
		if !known(m.Model) {
			unknown = append(unknown, m.Model)
		}
	}
	sort.Strings(unknown)
	return SetOutput{Saved: true, Applied: "live", MemberCount: len(parsed), UnknownModels: unknown}, nil
}

// Operations declares the read-only membership view and the audited
// replacement, both operator-only, on their Web UI routes.
func Operations(provider func(context.Context) *Service) ([]app.Operation, error) {
	if provider == nil {
		return nil, errors.New("council provider required")
	}
	membersOut, err := schema.FromType(reflect.TypeFor[MembersOutput](), false)
	if err != nil {
		return nil, err
	}
	setOut, err := schema.FromType(reflect.TypeFor[SetOutput](), false)
	if err != nil {
		return nil, err
	}
	members, err := app.NewOperation(opapi.Spec{Name: "council_members", ReadOnly: true, OutputSchema: membersOut, Authz: opapi.PrimaryOnly, Tenancy: opapi.Primary, AllowUnknownInput: true, HTTP: opapi.HTTP{Method: "GET", Path: "/api/council/members"}}, func(ctx context.Context, in MembersRequest) (MembersOutput, error) {
		return provider(ctx).Members(ctx, in)
	})
	if err != nil {
		return nil, err
	}
	set, err := app.NewOperation(opapi.Spec{Name: "council_set", OutputSchema: setOut, Authz: opapi.PrimaryOnly, Tenancy: opapi.Primary, AllowUnknownInput: true, InputSchema: json.RawMessage(`{"type":"object","additionalProperties":true,"properties":{"members":{}}}`), HTTP: opapi.HTTP{Method: "POST", Path: "/api/council/set"}}, func(ctx context.Context, in SetRequest) (SetOutput, error) {
		return provider(ctx).Set(ctx, in)
	})
	if err != nil {
		return nil, err
	}
	return []app.Operation{members, set}, nil
}
