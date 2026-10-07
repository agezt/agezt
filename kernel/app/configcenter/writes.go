// SPDX-License-Identifier: MIT
package configcenter

import (
	"context"
	"fmt"
	"strings"

	core "github.com/agezt/agezt/kernel/configcenter"
)

type Writer interface {
	Set(*core.ConfigEntry) error
	GetEntry(string) (*core.ConfigEntry, error)
	Delete(string) error
	GetAutoRating(string, string) core.Rating
}

type Writes struct{ writer Writer }

func NewWrites(writer Writer) *Writes { return &Writes{writer: writer} }

type SetInput struct {
	Key, Value, Rating, Description string
	AllowedAgents, ExcludedAgents   []string
}
type DeleteInput struct{ Key string }
type SetRatingInput struct{ Key, Rating string }
type SetAccessInput struct {
	Key                           string
	AllowedAgents, ExcludedAgents []string
}

func (s *Writes) Set(_ context.Context, in SetInput) (SetOutput, error) {
	rating := core.RatingInternal
	if in.Rating != "" {
		switch strings.ToLower(in.Rating) {
		case "public":
			rating = core.RatingPublic
		case "internal":
			rating = core.RatingInternal
		case "restricted":
			rating = core.RatingRestricted
		case "secret":
			rating = core.RatingSecret
		default:
			return SetOutput{}, fmt.Errorf("invalid rating: %s", in.Rating)
		}
	}
	entry := core.NewConfigEntry(in.Key, in.Value)
	entry.Rating = rating
	if in.Description != "" {
		entry.Description = in.Description
	}
	entry.AllowedAgents = in.AllowedAgents
	entry.ExcludedAgents = in.ExcludedAgents
	if s.writer == nil {
		return SetOutput{}, fmt.Errorf("config center not available")
	}
	if err := s.writer.Set(entry); err != nil {
		return SetOutput{}, err
	}
	updated, err := s.writer.GetEntry(in.Key)
	if err != nil {
		return SetOutput{}, err
	}
	return SetOutput{Entry: entryRow(updated)}, nil
}

func (s *Writes) Delete(_ context.Context, in DeleteInput) (DeleteOutput, error) {
	if s.writer == nil {
		return DeleteOutput{}, fmt.Errorf("config center not available")
	}
	if err := s.writer.Delete(in.Key); err != nil {
		return DeleteOutput{}, err
	}
	return DeleteOutput{Deleted: true}, nil
}

func (s *Writes) SetRating(_ context.Context, in SetRatingInput) (SetRatingOutput, error) {
	rating, err := core.ParseRating(in.Rating)
	if err != nil {
		return SetRatingOutput{}, err
	}
	if s.writer == nil {
		return SetRatingOutput{}, fmt.Errorf("config center not available")
	}
	entry, err := s.writer.GetEntry(in.Key)
	if err != nil {
		return SetRatingOutput{}, fmt.Errorf("key not found: %s", in.Key)
	}
	autoRating := s.writer.GetAutoRating(in.Key, entry.Value)
	isOverride := rating != autoRating
	entry.Rating = rating
	if err := s.writer.Set(entry); err != nil {
		return SetRatingOutput{}, err
	}
	return SetRatingOutput{Override: isOverride}, nil
}

func (s *Writes) SetAccess(_ context.Context, in SetAccessInput) (SetAccessOutput, error) {
	if s.writer == nil {
		return SetAccessOutput{}, fmt.Errorf("config center not available")
	}
	entry, err := s.writer.GetEntry(in.Key)
	if err != nil {
		return SetAccessOutput{}, fmt.Errorf("key not found: %s", in.Key)
	}
	entry.AllowedAgents = in.AllowedAgents
	entry.ExcludedAgents = in.ExcludedAgents
	if err := s.writer.Set(entry); err != nil {
		return SetAccessOutput{}, err
	}
	updated, err := s.writer.GetEntry(in.Key)
	if err != nil {
		return SetAccessOutput{}, err
	}
	return SetAccessOutput{Entry: entryRow(updated)}, nil
}
