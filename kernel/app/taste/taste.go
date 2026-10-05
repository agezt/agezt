// SPDX-License-Identifier: MIT

// Package taste owns transport-independent exemplar curation use cases.
package taste

import (
	"context"
	"errors"
	"time"

	curated "github.com/agezt/agezt/kernel/taste"
)

type ListInput struct {
	Scope string `json:"scope,omitempty"`
	Tag   string `json:"tag,omitempty"`
	// Limit is already admitted by the native adapter (default 200).
	Limit int `json:"limit,omitempty"`
}
type CreateInput struct {
	Title string   `json:"title"`
	Body  string   `json:"body"`
	Scope string   `json:"scope,omitempty"`
	Tags  []string `json:"tags,omitempty"`
}
type DeleteInput struct {
	ID string `json:"id"`
}
type ListOutput struct {
	Exemplars []curated.Exemplar `json:"exemplars"`
	Count     int                `json:"count"`
}
type CreateOutput struct {
	Exemplar curated.Exemplar `json:"exemplar"`
}
type DeleteOutput struct {
	Deleted string `json:"deleted"`
}
type Service struct{ store *curated.Store }

func New(store *curated.Store) *Service { return &Service{store: store} }

func (s *Service) List(_ context.Context, in ListInput) (ListOutput, error) {
	exemplars := s.store.List(curated.Filter{Scope: in.Scope, Tag: in.Tag, Limit: in.Limit})
	// Keep the native present-empty array even if a store implementation returns nil.
	out := make([]curated.Exemplar, 0, len(exemplars))
	out = append(out, exemplars...)
	return ListOutput{Exemplars: out, Count: len(out)}, nil
}
func (s *Service) Create(_ context.Context, in CreateInput) (CreateOutput, error) {
	if in.Title == "" || in.Body == "" {
		return CreateOutput{}, errors.New("taste_create requires title and body")
	}
	exemplar, err := s.store.Create(curated.CreateSpec{Title: in.Title, Body: in.Body, Scope: in.Scope, Tags: in.Tags}, time.Now())
	if err != nil {
		return CreateOutput{}, err
	}
	return CreateOutput{Exemplar: exemplar}, nil
}
func (s *Service) Delete(_ context.Context, in DeleteInput) (DeleteOutput, error) {
	if in.ID == "" {
		return DeleteOutput{}, errors.New("taste_delete requires id")
	}
	if err := s.store.Delete(in.ID); err != nil {
		return DeleteOutput{}, err
	}
	return DeleteOutput{Deleted: in.ID}, nil
}
