// SPDX-License-Identifier: MIT

package artifacts

import (
	"context"
	"encoding/base64"
	"errors"
	blobs "github.com/agezt/agezt/kernel/artifact"
	"time"
)

type BlobStore interface{ Get(string) ([]byte, error) }
type Index interface {
	List(blobs.Filter) []blobs.Entry
	StaleEntries(int64) []blobs.Entry
	Collect(int64) (int, int64)
	Delete(string) error
}
type Service struct {
	store BlobStore
	index Index
	now   func() time.Time
}

func New(store BlobStore, index Index, now func() time.Time) *Service {
	if now == nil {
		now = time.Now
	}
	return &Service{store: store, index: index, now: now}
}

type GetInput struct {
	Ref string `json:"ref"`
}
type GetOutput struct {
	Ref  string `json:"ref"`
	Size int    `json:"size"`
	Data string `json:"data"`
}
type ListInput struct {
	Kind          string `json:"kind,omitempty"`
	Source        string `json:"source,omitempty"`
	CorrelationID string `json:"corr,omitempty"`
}
type Entry struct {
	ID            string `json:"id"`
	Ref           string `json:"ref"`
	Name          string `json:"name"`
	Mime          string `json:"mime"`
	Kind          string `json:"kind"`
	Source        string `json:"source"`
	Sender        string `json:"sender"`
	CorrelationID string `json:"corr"`
	Size          int64  `json:"size"`
	CreatedMS     int64  `json:"created_ms"`
	Caption       string `json:"caption"`
}
type ListOutput struct {
	Count   int     `json:"count"`
	Entries []Entry `json:"entries"`
}
type DeleteInput struct {
	ID string `json:"id"`
}
type DeleteOutput struct {
	Deleted bool   `json:"deleted"`
	ID      string `json:"id"`
}
type CollectInput struct {
	OlderThanDays int  `json:"older_than_days"`
	DryRun        bool `json:"dry_run"`
}
type Candidate struct {
	ID        string `json:"id"`
	Name      string `json:"name"`
	Kind      string `json:"kind"`
	Source    string `json:"source"`
	Size      int64  `json:"size"`
	CreatedMS int64  `json:"created_ms"`
}
type CollectOutput struct {
	DryRun        bool         `json:"dry_run"`
	OlderThanDays int          `json:"older_than_days"`
	CutoffMS      int64        `json:"cutoff_ms"`
	Count         int          `json:"count"`
	Bytes         int64        `json:"bytes"`
	Candidates    *[]Candidate `json:"candidates,omitempty"`
}

func (s *Service) Get(_ context.Context, in GetInput) (GetOutput, error) {
	if s.store == nil {
		return GetOutput{}, errors.New("artifact store unavailable")
	}
	data, err := s.store.Get(in.Ref)
	if err != nil {
		switch {
		case errors.Is(err, blobs.ErrBadRef):
			return GetOutput{}, errors.New("malformed ref (want a 64-hex content address)")
		case errors.Is(err, blobs.ErrNotFound):
			return GetOutput{}, errors.New("artifact not found: " + in.Ref)
		case errors.Is(err, blobs.ErrCorrupt):
			return GetOutput{}, errors.New("artifact CORRUPT (bytes do not match ref): " + in.Ref)
		default:
			return GetOutput{}, err
		}
	}
	return GetOutput{Ref: in.Ref, Size: len(data), Data: base64.StdEncoding.EncodeToString(data)}, nil
}
func (s *Service) List(_ context.Context, in ListInput) (ListOutput, error) {
	if s.index == nil {
		return ListOutput{}, errors.New("artifact index unavailable")
	}
	rows := s.index.List(blobs.Filter{Kind: in.Kind, Source: in.Source, Corr: in.CorrelationID})
	out := make([]Entry, 0, len(rows))
	for _, e := range rows {
		out = append(out, Entry{ID: e.ID, Ref: e.Ref, Name: e.Name, Mime: e.Mime, Kind: e.Kind, Source: e.Source, Sender: e.Sender, CorrelationID: e.Corr, Size: e.Size, CreatedMS: e.CreatedMs, Caption: e.Caption})
	}
	return ListOutput{Count: len(out), Entries: out}, nil
}
func (s *Service) Delete(_ context.Context, in DeleteInput) (DeleteOutput, error) {
	if s.index == nil {
		return DeleteOutput{}, errors.New("artifact index unavailable")
	}
	if err := s.index.Delete(in.ID); err != nil {
		if errors.Is(err, blobs.ErrNotFound) {
			return DeleteOutput{}, errors.New("artifact not found: " + in.ID)
		}
		return DeleteOutput{}, err
	}
	return DeleteOutput{Deleted: true, ID: in.ID}, nil
}
func (s *Service) Collect(_ context.Context, in CollectInput) (CollectOutput, error) {
	if s.index == nil {
		return CollectOutput{}, errors.New("artifact index unavailable")
	}
	days := in.OlderThanDays
	if days <= 0 {
		days = 30
	}
	cutoff := s.now().Add(-time.Duration(days) * 24 * time.Hour).UnixMilli()
	out := CollectOutput{DryRun: in.DryRun, OlderThanDays: days, CutoffMS: cutoff}
	if in.DryRun {
		rows := s.index.StaleEntries(cutoff)
		candidates := make([]Candidate, 0, len(rows))
		for _, e := range rows {
			out.Bytes += e.Size
			candidates = append(candidates, Candidate{ID: e.ID, Name: e.Name, Kind: e.Kind, Source: e.Source, Size: e.Size, CreatedMS: e.CreatedMs})
		}
		out.Count = len(candidates)
		out.Candidates = &candidates
		return out, nil
	}
	out.Count, out.Bytes = s.index.Collect(cutoff)
	return out, nil
}
