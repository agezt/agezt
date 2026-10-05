// SPDX-License-Identifier: MIT

package skill

import (
	"context"
	"fmt"
	"github.com/agezt/agezt/kernel/contract/opapi"
	curated "github.com/agezt/agezt/kernel/skill"
)

type CurationStore interface {
	Reassign(string, string, string) (curated.Skill, bool, error)
	Create(string, curated.CreateSpec) (curated.Skill, bool, error)
}
type Curation struct {
	forge       CurationStore
	agentExists func(string) bool
}

func NewCuration(forge CurationStore, agentExists func(string) bool) *Curation {
	return &Curation{forge: forge, agentExists: agentExists}
}

type ReassignInput struct {
	ID    string `json:"id"`
	Agent string `json:"agent,omitempty"`
}
type ShareOutput struct {
	Shared bool    `json:"shared"`
	ID     string  `json:"id"`
	Name   *string `json:"name,omitempty"`
}
type ReassignOutput struct {
	Reassigned bool    `json:"reassigned"`
	ID         string  `json:"id"`
	ToAgent    string  `json:"to_agent"`
	Name       *string `json:"name,omitempty"`
}
type ImportInput struct {
	Name          string            `json:"name"`
	Description   string            `json:"description,omitempty"`
	Triggers      []string          `json:"triggers,omitempty"`
	Body          string            `json:"body"`
	ToolsRequired []string          `json:"tools_required,omitempty"`
	Resources     map[string][]byte `json:"resources,omitempty"`
	Agent         string            `json:"agent,omitempty"`
}
type ImportOutput struct {
	ID        string         `json:"id"`
	Name      string         `json:"name"`
	Status    curated.Status `json:"status"`
	Created   bool           `json:"created"`
	Resources []string       `json:"resources"`
}

func (s *Curation) Share(ctx context.Context, in GetInput) (ShareOutput, error) {
	sk, found, err := s.forge.Reassign(opapi.CorrelationFromContext(ctx), in.ID, "")
	if err != nil {
		return ShareOutput{}, err
	}
	out := ShareOutput{Shared: found, ID: in.ID}
	if found {
		out.Name = &sk.Name
	}
	return out, nil
}
func (s *Curation) Reassign(ctx context.Context, in ReassignInput) (ReassignOutput, error) {
	if in.Agent != "" && (s.agentExists == nil || !s.agentExists(in.Agent)) {
		return ReassignOutput{}, fmt.Errorf("no such agent: %s", in.Agent)
	}
	sk, found, err := s.forge.Reassign(opapi.CorrelationFromContext(ctx), in.ID, in.Agent)
	if err != nil {
		return ReassignOutput{}, err
	}
	out := ReassignOutput{Reassigned: found, ID: in.ID, ToAgent: in.Agent}
	if found {
		out.Name = &sk.Name
	}
	return out, nil
}
func (s *Curation) Import(ctx context.Context, in ImportInput) (ImportOutput, error) {
	sk, created, err := s.forge.Create(opapi.CorrelationFromContext(ctx), curated.CreateSpec{Name: in.Name, Description: in.Description, Triggers: in.Triggers, Body: in.Body, ToolsRequired: in.ToolsRequired, Resources: in.Resources, Agent: in.Agent})
	if err != nil {
		return ImportOutput{}, err
	}
	return ImportOutput{ID: sk.ID, Name: sk.Name, Status: sk.Status, Created: created, Resources: sk.Resources}, nil
}
