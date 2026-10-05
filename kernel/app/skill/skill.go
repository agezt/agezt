// SPDX-License-Identifier: MIT

// Package skill owns transport-independent skill use cases.
package skill

import (
	"context"
	curated "github.com/agezt/agezt/kernel/skill"
)

// Reader is the actual read-only Forge surface, preserving store error causes.
type Reader interface {
	List() ([]curated.Skill, error)
	Get(string) (curated.Skill, bool, error)
}
type Service struct{ reader Reader }

func New(reader Reader) *Service { return &Service{reader: reader} }

type ListInput struct{}
type GetInput struct {
	ID string `json:"id"`
}

// Metrics retains present-zero fields in the native wire projection.
type Metrics struct {
	Uses        int   `json:"uses"`
	Successes   int   `json:"successes"`
	Failures    int   `json:"failures"`
	LastUsedMS  int64 `json:"last_used_ms"`
	ShadowEvals int   `json:"shadow_evals"`
	ShadowWins  int   `json:"shadow_wins"`
}
type Record struct {
	ID            string         `json:"id"`
	Name          string         `json:"name"`
	Description   string         `json:"description"`
	Status        curated.Status `json:"status"`
	Version       string         `json:"version"`
	Agent         string         `json:"agent"`
	CreatedMS     int64          `json:"created_ms"`
	LastSeenMS    int64          `json:"last_seen_ms"`
	Metrics       Metrics        `json:"metrics"`
	Triggers      []string       `json:"triggers,omitempty"`
	ToolsRequired []string       `json:"tools_required,omitempty"`
	Resources     []string       `json:"resources,omitempty"`
	Lineage       []string       `json:"lineage,omitempty"`
	Body          string         `json:"body,omitempty"`
	SourceEvent   string         `json:"source_event,omitempty"`
}

func project(sk curated.Skill) Record {
	return Record{ID: sk.ID, Name: sk.Name, Description: sk.Description, Status: sk.Status, Version: sk.Version, Agent: sk.Agent, CreatedMS: sk.CreatedMS, LastSeenMS: sk.LastSeenMS,
		Metrics:  Metrics{Uses: sk.Metrics.Uses, Successes: sk.Metrics.Successes, Failures: sk.Metrics.Failures, LastUsedMS: sk.Metrics.LastUsedMS, ShadowEvals: sk.Metrics.ShadowEvals, ShadowWins: sk.Metrics.ShadowWins},
		Triggers: sk.Triggers, ToolsRequired: sk.ToolsRequired, Resources: sk.Resources, Lineage: sk.Lineage, Body: sk.Body, SourceEvent: sk.SourceEvent}
}

type ListOutput struct {
	Skills      []Record `json:"skills"`
	Count       int      `json:"count"`
	ActiveCount int      `json:"active_count"`
}
type GetOutput struct {
	Found bool    `json:"found"`
	Skill *Record `json:"skill,omitempty"`
}

func (s *Service) List(_ context.Context, _ ListInput) (ListOutput, error) {
	skills, err := s.reader.List()
	if err != nil {
		return ListOutput{}, err
	}
	out := ListOutput{Skills: make([]Record, 0, len(skills)), Count: len(skills)}
	for _, sk := range skills {
		out.Skills = append(out.Skills, project(sk))
		if sk.Active() {
			out.ActiveCount++
		}
	}
	return out, nil
}
func (s *Service) Get(_ context.Context, in GetInput) (GetOutput, error) {
	sk, found, err := s.reader.Get(in.ID)
	if err != nil {
		return GetOutput{}, err
	}
	out := GetOutput{Found: found}
	if found {
		record := project(sk)
		out.Skill = &record
	}
	return out, nil
}
