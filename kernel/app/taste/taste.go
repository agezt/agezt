// SPDX-License-Identifier: MIT

// Package taste owns transport-independent exemplar curation use cases.
package taste

import (
	"context"
	"errors"
	"time"

	"github.com/agezt/agezt/kernel/contract/tasteapi"
)

// errNoExemplarLibrary is the answer the taste app layer gives when it holds
// no library.
//
// The exemplar library is the DAEMON's (W3.52): the kernel holds the
// contract's port, and a host that was given none holds a nil port. The
// refusal is the answer — not a panic, and not a silent "nothing to do".
var errNoExemplarLibrary = errors.New("taste: the exemplar library is not available on this daemon")

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
	Exemplars []tasteapi.Exemplar `json:"exemplars"`
	Count     int                 `json:"count"`
}
type CreateOutput struct {
	Exemplar tasteapi.Exemplar `json:"exemplar"`
}
type DeleteOutput struct {
	Deleted string `json:"deleted"`
}

// Service curates the exemplars through the contract's store port.
//
// The port is the CONTRACT's, not a module's store (W3.52): the library's file
// is the daemon's business and this layer may not name the module that keeps
// it.
type Service struct{ store tasteapi.Store }

func New(store tasteapi.Store) *Service { return &Service{store: store} }

func (s *Service) List(_ context.Context, in ListInput) (ListOutput, error) {
	// A host with no library has no exemplars — the empty list is the honest
	// answer, not a panic (W3.52).
	if s.store == nil {
		return ListOutput{Exemplars: []tasteapi.Exemplar{}}, nil
	}
	exemplars := s.store.List(tasteapi.Filter{Scope: in.Scope, Tag: in.Tag, Limit: in.Limit})
	// Keep the native present-empty array even if a store implementation returns nil.
	out := make([]tasteapi.Exemplar, 0, len(exemplars))
	out = append(out, exemplars...)
	return ListOutput{Exemplars: out, Count: len(out)}, nil
}
func (s *Service) Create(_ context.Context, in CreateInput) (CreateOutput, error) {
	if in.Title == "" || in.Body == "" {
		return CreateOutput{}, errors.New("taste_create requires title and body")
	}
	if s.store == nil {
		return CreateOutput{}, errNoExemplarLibrary
	}
	exemplar, err := s.store.Create(tasteapi.CreateSpec{Title: in.Title, Body: in.Body, Scope: in.Scope, Tags: in.Tags}, time.Now().UnixMilli())
	if err != nil {
		return CreateOutput{}, err
	}
	return CreateOutput{Exemplar: exemplar}, nil
}
func (s *Service) Delete(_ context.Context, in DeleteInput) (DeleteOutput, error) {
	if in.ID == "" {
		return DeleteOutput{}, errors.New("taste_delete requires id")
	}
	if s.store == nil {
		return DeleteOutput{}, errNoExemplarLibrary
	}
	if err := s.store.Delete(in.ID); err != nil {
		return DeleteOutput{}, err
	}
	return DeleteOutput{Deleted: in.ID}, nil
}
