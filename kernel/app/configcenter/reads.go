// SPDX-License-Identifier: MIT
package configcenter

import (
	"context"
	"fmt"
	core "github.com/agezt/agezt/kernel/configcenter"
	"strings"
	"time"
)

type Reader interface {
	GetEntry(string) (*core.ConfigEntry, error)
	ListEntries() []*core.ConfigEntry
	ListByRating(core.Rating) []*core.ConfigEntry
	AccessLog(string, string, time.Duration) []*core.AccessLogEntry
	AuditLog(time.Duration) []*core.AuditEntry
	Stats() map[string]any
}
type Reads struct{ reader Reader }

func NewReads(reader Reader) *Reads { return &Reads{reader: reader} }

type GetInput struct{ Key string }
type ListInput struct{ Rating string }
type AccessLogInput struct{ Key, AgentID, Since string }
type AuditInput struct{ Since string }
type HealthInput struct{}

func (s *Reads) Get(_ context.Context, in GetInput) (GetOutput, error) {
	if s.reader == nil {
		return GetOutput{}, fmt.Errorf("config center not available")
	}
	entry, err := s.reader.GetEntry(in.Key)
	if err != nil {
		return GetOutput{}, fmt.Errorf("key not found: %s", in.Key)
	}
	return GetOutput{Entry: entryRow(entry)}, nil
}
func (s *Reads) List(_ context.Context, in ListInput) (ListOutput, error) {
	var rating core.Rating
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
			return ListOutput{}, fmt.Errorf("invalid rating: %s", in.Rating)
		}
	}
	if s.reader == nil {
		return ListOutput{}, fmt.Errorf("config center not available")
	}
	var entries []*core.ConfigEntry
	if rating != "" {
		entries = s.reader.ListByRating(rating)
	} else {
		entries = s.reader.ListEntries()
	}
	out := make([]EntryRow, len(entries))
	for i, entry := range entries {
		out[i] = entryRow(entry)
	}
	return ListOutput{Entries: out, Count: len(out)}, nil
}
func (s *Reads) AccessLog(_ context.Context, in AccessLogInput) (AccessLogOutput, error) {
	var since time.Duration
	if in.Since != "" {
		var err error
		since, err = time.ParseDuration(in.Since)
		if err != nil {
			return AccessLogOutput{}, fmt.Errorf("invalid duration: %s", in.Since)
		}
	}
	if s.reader == nil {
		return AccessLogOutput{}, fmt.Errorf("config center not available")
	}
	logs := s.reader.AccessLog(in.Key, in.AgentID, since)
	out := make([]AccessLogRow, len(logs))
	for i, log := range logs {
		out[i] = AccessLogRow{Timestamp: log.Timestamp, Key: log.Key, AgentID: log.AgentID, RunID: log.RunID, Rating: string(log.Rating), Decision: string(log.Decision), Reason: log.Reason, ValueLog: log.ValueLog}
	}
	return AccessLogOutput{Logs: out, Count: len(out)}, nil
}
func (s *Reads) Audit(_ context.Context, in AuditInput) (AuditOutput, error) {
	var since time.Duration
	if in.Since != "" {
		var err error
		since, err = time.ParseDuration(in.Since)
		if err != nil {
			return AuditOutput{}, fmt.Errorf("invalid duration: %s", in.Since)
		}
	}
	if s.reader == nil {
		return AuditOutput{}, fmt.Errorf("config center not available")
	}
	entries := s.reader.AuditLog(since)
	out := make([]AuditRow, len(entries))
	for i, entry := range entries {
		out[i] = AuditRow{Timestamp: entry.Timestamp, Event: entry.Event, Key: entry.Key, AgentID: entry.AgentID, RunID: entry.RunID, Rating: string(entry.Rating), Reason: entry.Reason, Decision: string(entry.Decision), Policy: entry.Policy}
	}
	return AuditOutput{Entries: out, Count: len(out)}, nil
}
func (s *Reads) Health(_ context.Context, _ HealthInput) (HealthOutput, error) {
	if s.reader == nil {
		return HealthOutput{Status: "unavailable", Checks: map[string]string{"config_center": "not configured"}}, nil
	}
	stats := s.reader.Stats()
	return HealthOutput{Status: "healthy", Checks: map[string]string{"config_center": "ok", "store": "ok"}, Stats: &stats}, nil
}
