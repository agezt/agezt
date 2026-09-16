// SPDX-License-Identifier: MIT
//
// kernel/controlplane remote-mirror HTTP fetch helpers (fetchRemoteEvents, fetchRemoteArtifacts).
// Extracted from remote_mirror.go during Day 211 god-file refactor (#86).
// Public API unchanged.
package controlplane

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strings"
	"time"

	"github.com/agezt/agezt/kernel/event"
)

const remoteEventMirrorResponseLimit = 4 << 20
const remoteEventMirrorMaxEvents = 200
const remoteArtifactMirrorResponseLimit = 1 << 20
const remoteArtifactMirrorMaxEntries = 200

func fetchRemoteEvents(ctx context.Context, p nodePeer, corr, mode string) ([]map[string]any, bool, error) {
	ctx, cancel := context.WithTimeout(ctx, 5*time.Second)
	defer cancel()
	endpoint := strings.TrimRight(p.URL, "/") + "/api/v1/runs/" + url.PathEscape(corr)
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, endpoint, nil)
	if err != nil {
		return nil, false, err
	}
	if p.Token != "" {
		req.Header.Set("Authorization", "Bearer "+p.Token)
	}
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		return nil, false, err
	}
	defer resp.Body.Close()
	if resp.StatusCode == http.StatusUnauthorized {
		return nil, false, fmt.Errorf("401 (token rejected)")
	}
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return nil, false, fmt.Errorf("status %d", resp.StatusCode)
	}
	var body struct {
		Events []struct {
			ID            string          `json:"id"`
			Seq           int64           `json:"seq"`
			TSUnixMS      int64           `json:"ts_unix_ms"`
			Subject       string          `json:"subject"`
			Actor         string          `json:"actor"`
			Kind          event.Kind      `json:"kind"`
			CorrelationID string          `json:"correlation_id"`
			Hash          string          `json:"hash"`
			Payload       json.RawMessage `json:"payload"`
		} `json:"events"`
	}
	if err := json.NewDecoder(io.LimitReader(resp.Body, remoteEventMirrorResponseLimit)).Decode(&body); err != nil {
		return nil, false, err
	}
	truncated := len(body.Events) > remoteEventMirrorMaxEvents
	limit := len(body.Events)
	if limit > remoteEventMirrorMaxEvents {
		limit = remoteEventMirrorMaxEvents
	}
	out := make([]map[string]any, 0, limit)
	for _, ev := range body.Events[:limit] {
		row := map[string]any{
			"id":             ev.ID,
			"seq":            ev.Seq,
			"ts_unix_ms":     ev.TSUnixMS,
			"subject":        ev.Subject,
			"actor":          ev.Actor,
			"kind":           ev.Kind,
			"correlation_id": ev.CorrelationID,
			"hash":           ev.Hash,
		}
		if mode == "redacted" {
			if payload, ok := redactedRemotePayload(ev.Payload); ok {
				row["payload_redacted"] = payload
			}
		}
		out = append(out, row)
	}
	return out, truncated, nil
}
func fetchRemoteArtifacts(ctx context.Context, p nodePeer, corr string) ([]map[string]any, bool, error) {
	ctx, cancel := context.WithTimeout(ctx, 5*time.Second)
	defer cancel()
	endpoint := strings.TrimRight(p.URL, "/") + "/api/v1/artifacts"
	q := url.Values{}
	q.Set("corr", corr)
	q.Set("limit", fmt.Sprintf("%d", remoteArtifactMirrorMaxEntries))
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, endpoint+"?"+q.Encode(), nil)
	if err != nil {
		return nil, false, err
	}
	if p.Token != "" {
		req.Header.Set("Authorization", "Bearer "+p.Token)
	}
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		return nil, false, err
	}
	defer resp.Body.Close()
	if resp.StatusCode == http.StatusUnauthorized {
		return nil, false, fmt.Errorf("401 (token rejected)")
	}
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return nil, false, fmt.Errorf("status %d", resp.StatusCode)
	}
	var body struct {
		Entries []struct {
			ID        string `json:"id"`
			Ref       string `json:"ref"`
			Name      string `json:"name"`
			Mime      string `json:"mime"`
			Kind      string `json:"kind"`
			Source    string `json:"source"`
			Sender    string `json:"sender"`
			Corr      string `json:"corr"`
			Size      int64  `json:"size"`
			CreatedMs int64  `json:"created_ms"`
			Caption   string `json:"caption"`
		} `json:"entries"`
		Truncated bool `json:"truncated"`
	}
	if err := json.NewDecoder(io.LimitReader(resp.Body, remoteArtifactMirrorResponseLimit)).Decode(&body); err != nil {
		return nil, false, err
	}
	truncated := body.Truncated || len(body.Entries) > remoteArtifactMirrorMaxEntries
	limit := len(body.Entries)
	if limit > remoteArtifactMirrorMaxEntries {
		limit = remoteArtifactMirrorMaxEntries
	}
	out := make([]map[string]any, 0, limit)
	for _, a := range body.Entries[:limit] {
		row := map[string]any{
			"id":         a.ID,
			"ref":        a.Ref,
			"name":       a.Name,
			"mime":       a.Mime,
			"kind":       a.Kind,
			"source":     a.Source,
			"sender":     a.Sender,
			"corr":       a.Corr,
			"size":       a.Size,
			"created_ms": a.CreatedMs,
		}
		if a.Caption != "" {
			row["caption"] = a.Caption
		}
		out = append(out, row)
	}
	return out, truncated, nil
}
