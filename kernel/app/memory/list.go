// SPDX-License-Identifier: MIT

package memory

import (
	"context"
	"sort"
	"strconv"
	"strings"

	store "github.com/agezt/agezt/kernel/memory"
)

type ListInput struct {
	Limit  float64 `json:"limit,omitempty"`
	Cursor string  `json:"cursor,omitempty"`
}
type ListOutput struct {
	Records    []Record `json:"records"`
	Count      int      `json:"count"`
	Total      int      `json:"total"`
	NextCursor string   `json:"next_cursor,omitempty"`
}

// PreparedList retains the active read that precedes native argument admission.
// Each page works on a copy so the prepared read can be paged repeatedly.
type PreparedList struct{ records []store.Record }

func (s *Service) PrepareList(_ context.Context) (*PreparedList, error) {
	recs, err := s.manager.Active()
	if err != nil {
		return nil, err
	}
	return &PreparedList{records: recs}, nil
}

func (s *PreparedList) Page(_ context.Context, in ListInput) (ListOutput, error) {
	recs := append([]store.Record(nil), s.records...)
	limit := 100
	if in.Limit > 0 {
		limit = int(in.Limit)
	}
	if limit > 1000 {
		limit = 1000
	}
	sort.SliceStable(recs, func(i, j int) bool {
		if recs[i].CreatedMS != recs[j].CreatedMS {
			return recs[i].CreatedMS > recs[j].CreatedMS
		}
		return recs[i].ID > recs[j].ID
	})
	total := len(recs)
	var cursorMS int64
	var cursorID string
	cursorOK := false
	if in.Cursor != "" {
		msStr, id, _ := strings.Cut(in.Cursor, ":")
		if ms, err := strconv.ParseInt(msStr, 10, 64); err == nil {
			cursorMS, cursorID, cursorOK = ms, id, true
		}
	}
	if cursorOK {
		filtered := recs[:0]
		for _, r := range recs {
			if r.CreatedMS > cursorMS {
				continue
			}
			if r.CreatedMS == cursorMS && r.ID >= cursorID {
				continue
			}
			filtered = append(filtered, r)
		}
		recs = filtered
	}
	var nextCursor string
	if limit > 0 && len(recs) > limit {
		recs = recs[:limit]
		nextCursor = strconv.FormatInt(recs[limit-1].CreatedMS, 10) + ":" + recs[limit-1].ID
	}
	out := make([]Record, 0, len(recs))
	for _, r := range recs {
		out = append(out, recordView(r))
	}
	return ListOutput{Records: out, Count: len(out), Total: total, NextCursor: nextCursor}, nil
}
