// SPDX-License-Identifier: MIT

// Package roster owns operator agent-management presentation and use cases.
package roster

import (
	"context"
	"encoding/json"
	"hash/fnv"
	"strconv"
	"strings"
	"sync"
	"time"

	core "github.com/agezt/agezt/kernel/roster"
)

// ProfileView preserves the legacy JSON round trip and kind/managed augmentation.
func ProfileView(p core.Profile) map[string]any {
	raw, _ := json.Marshal(p)
	var out map[string]any
	_ = json.Unmarshal(raw, &out)
	out["kind"] = p.Kind()
	out["managed"] = !p.AllowsDirectCall()
	return out
}

type ListInput struct {
	Limit       int
	Cursor      string
	DecodeError error
}
type ListService struct {
	profiles       func() []core.Profile
	statuses       func([]core.Profile) map[string]map[string]any
	now            func() time.Time
	mu             sync.RWMutex
	valid          bool
	key            uint64
	rows           []ProfileOutput
	total, enabled int
	at             time.Time
}

func NewList(profiles func() []core.Profile, statuses func([]core.Profile) map[string]map[string]any, now func() time.Time) *ListService {
	if now == nil {
		now = time.Now
	}
	return &ListService{profiles: profiles, statuses: statuses, now: now}
}

const ListCacheTTL = 1500 * time.Millisecond

func contentHash(profiles []core.Profile) uint64 {
	if len(profiles) == 0 {
		return 0
	}
	h := fnv.New64a()
	var buf []byte
	for _, p := range profiles {
		buf = buf[:0]
		buf = append(buf, p.Slug...)
		buf = append(buf, 0)
		buf = strconv.AppendInt(buf, p.UpdatedMS, 16)
		buf = append(buf, 0)
		if p.Enabled {
			buf = append(buf, 1)
		}
		if p.Retired {
			buf = append(buf, 1)
		}
		if p.System {
			buf = append(buf, 1)
		}
		buf = append(buf, 0)
		_, _ = h.Write(buf)
	}
	return h.Sum64()
}
func (s *ListService) cached(key uint64, profiles []core.Profile) ([]ProfileOutput, int, int, bool) {
	s.mu.RLock()
	defer s.mu.RUnlock()
	if !s.valid || s.key != key || s.now().Sub(s.at) > ListCacheTTL || contentHash(profiles) != s.key {
		return nil, 0, 0, false
	}
	return s.rows, s.total, s.enabled, true
}
func (s *ListService) store(key uint64, rows []ProfileOutput, total, enabled int) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.valid = true
	s.key = key
	s.rows = rows
	s.total = total
	s.enabled = enabled
	s.at = s.now()
}
func (s *ListService) Invalidate() { s.mu.Lock(); s.valid = false; s.rows = nil; s.mu.Unlock() }
func (s *ListService) List(_ context.Context, in ListInput) (ListOutput, error) {
	profiles := s.profiles()
	key := contentHash(profiles)
	rows, total, enabled, hit := s.cached(key, profiles)
	if !hit {
		statuses := s.statuses(profiles)
		rows = make([]ProfileOutput, 0, len(profiles))
		enabled = 0
		for _, p := range profiles {
			view := ProfileOutput{Profile: p, Kind: p.Kind(), Managed: !p.AllowsDirectCall()}
			if status, ok := statuses[p.Slug]; ok {
				view.StatusPresent = true
				var err error
				view.Status, err = statusOutput(status)
				if err != nil {
					return ListOutput{}, err
				}
			}
			rows = append(rows, view)
			if p.Enabled {
				enabled++
			}
		}
		total = len(rows)
		s.store(key, rows, total, enabled)
	}
	// Preserve outer-cache ownership and legacy nil empty-page representation.
	rows = append([]ProfileOutput(nil), rows...)
	if in.DecodeError != nil {
		return ListOutput{}, in.DecodeError
	}
	limit := in.Limit
	if limit > 1000 {
		limit = 1000
	}
	var cursorMS int64
	var cursorSlug string
	cursorOK := false
	if in.Cursor != "" {
		ms, slug, _ := strings.Cut(in.Cursor, ":")
		if parsed, err := strconv.ParseInt(ms, 10, 64); err == nil {
			cursorMS, cursorSlug, cursorOK = parsed, slug, true
		}
	}
	for i, j := 0, len(rows)-1; i < j; i, j = i+1, j-1 {
		rows[i], rows[j] = rows[j], rows[i]
	}
	if cursorOK {
		filtered := rows[:0]
		for _, view := range rows {
			ms := int64(float64(view.CreatedMS))
			slug := view.Slug
			if ms > cursorMS || ms == cursorMS && slug >= cursorSlug {
				continue
			}
			filtered = append(filtered, view)
		}
		rows = filtered
	}
	var next string
	if limit > 0 && len(rows) > limit {
		rows = rows[:limit]
		last := rows[limit-1]
		next = strconv.FormatInt(int64(float64(last.CreatedMS)), 10) + ":" + last.Slug
	}
	return ListOutput{Profiles: rows, Count: len(rows), Total: total, EnabledCount: enabled, NextCursor: next}, nil
}
