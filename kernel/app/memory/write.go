// SPDX-License-Identifier: MIT

package memory

import (
	"context"
	"errors"

	store "github.com/agezt/agezt/kernel/memory"
)

type RememberInput struct {
	Content    string            `json:"content"`
	Subject    string            `json:"subject,omitempty"`
	Type       string            `json:"type,omitempty"`
	Confidence float64           `json:"confidence,omitempty"`
	Evidence   string            `json:"evidence,omitempty"`
	HalfLifeMS float64           `json:"half_life_ms,omitempty"`
	Tags       map[string]string `json:"tags,omitempty"`
}
type SupersedeInput struct {
	RememberInput
	OldID string `json:"old_id"`
}
type RememberOutput struct {
	ID       string `json:"id"`
	Created  bool   `json:"created"`
	Type     string `json:"type"`
	Subject  string `json:"subject"`
	Evidence string `json:"evidence"`
}
type SupersedeOutput struct {
	NewID      string `json:"new_id"`
	OldID      string `json:"old_id"`
	Superseded bool   `json:"superseded"`
	Type       string `json:"type"`
	Subject    string `json:"subject"`
}
type ForgetOutput struct {
	Forgotten bool `json:"forgotten"`
}
type PromoteOutput struct {
	Promoted bool    `json:"promoted"`
	ID       string  `json:"id"`
	Subject  *string `json:"subject,omitempty"`
}
type BulkForgetInput struct {
	IDs []string `json:"ids"`
}
type BulkForgetOutput struct {
	Forgotten int `json:"forgotten"`
	NotFound  int `json:"not_found"`
}

func rememberSpec(in RememberInput) store.RememberSpec {
	tags := map[string]string{"source": "operator"}
	for key, value := range in.Tags {
		tags[key] = value
	}
	var halfLife int64
	if in.HalfLifeMS > 0 {
		halfLife = int64(in.HalfLifeMS)
	}
	return store.RememberSpec{Type: store.Type(in.Type), Subject: in.Subject, Content: in.Content,
		Tags: tags, Confidence: in.Confidence, Evidence: store.Evidence(in.Evidence), HalfLifeMS: halfLife,
		Actor: "operator", Force: true}
}

func (s *Service) Remember(_ context.Context, in RememberInput) (RememberOutput, error) {
	rec, created, err := s.manager.Remember("", rememberSpec(in))
	if err != nil {
		return RememberOutput{}, err
	}
	return RememberOutput{ID: rec.ID, Created: created, Type: string(rec.Type), Subject: rec.Subject, Evidence: string(rec.Evidence)}, nil
}

func (s *Service) Supersede(_ context.Context, in SupersedeInput) (SupersedeOutput, error) {
	rec, err := s.manager.Supersede("", in.OldID, rememberSpec(in.RememberInput))
	if err != nil {
		return SupersedeOutput{}, err
	}
	return SupersedeOutput{NewID: rec.ID, OldID: in.OldID, Superseded: rec.ID != in.OldID, Type: string(rec.Type), Subject: rec.Subject}, nil
}

func (s *Service) Forget(_ context.Context, in GetInput) (ForgetOutput, error) {
	ok, err := s.manager.Forget("", in.ID)
	if err != nil {
		return ForgetOutput{}, err
	}
	return ForgetOutput{Forgotten: ok}, nil
}

func (s *Service) Promote(_ context.Context, in GetInput) (PromoteOutput, error) {
	rec, found, err := s.manager.Promote("", in.ID)
	if err != nil {
		return PromoteOutput{}, err
	}
	out := PromoteOutput{Promoted: found, ID: in.ID}
	if found {
		out.Subject = &rec.Subject
	}
	return out, nil
}

func (s *Service) BulkForget(_ context.Context, in BulkForgetInput) (BulkForgetOutput, error) {
	if len(in.IDs) > 500 {
		return BulkForgetOutput{}, errors.New("args.ids exceeds 500 — use smaller batches")
	}
	var out BulkForgetOutput
	for _, id := range in.IDs {
		ok, err := s.manager.Forget("", id)
		if err != nil {
			return BulkForgetOutput{}, err
		}
		if ok {
			out.Forgotten++
		} else {
			out.NotFound++
		}
	}
	return out, nil
}
