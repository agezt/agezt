// SPDX-License-Identifier: MIT

package main

// Peer fetch helpers for `agt peers run / artifacts / artifact-get`:
// fetchPeerRun + fetchPeerArtifacts + fetchPeerArtifactBytes + the
// peerArtifact* response types they return. The dispatcher + cli
// wrappers (peersArtifacts, peersArtifactGet) live in
// peers_fetch_cmd.go. Carved out of peers.go during the Day 167
// god-file split so peers.go can stay focused on the dispatcher
// and simple types. Public API unchanged.

import (
	"context"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strings"
	"time"

	"github.com/agezt/agezt/plugins/tools/peer"
)

type peerArtifactEntry struct {
	ID        string `json:"id"`
	Ref       string `json:"ref"`
	Name      string `json:"name,omitempty"`
	Mime      string `json:"mime,omitempty"`
	Kind      string `json:"kind,omitempty"`
	Source    string `json:"source,omitempty"`
	Sender    string `json:"sender,omitempty"`
	Corr      string `json:"corr,omitempty"`
	Size      int64  `json:"size"`
	CreatedMs int64  `json:"created_ms"`
	Caption   string `json:"caption,omitempty"`
}
type peerArtifactList struct {
	Peer          string              `json:"peer"`
	URL           string              `json:"url"`
	CorrelationID string              `json:"correlation_id"`
	Count         int                 `json:"count"`
	TotalCount    int                 `json:"total_count,omitempty"`
	Truncated     bool                `json:"truncated,omitempty"`
	Entries       []peerArtifactEntry `json:"entries"`
	Error         string              `json:"error,omitempty"`
}
type peerArtifactBytes struct {
	Peer  string            `json:"peer"`
	URL   string            `json:"url"`
	ID    string            `json:"id"`
	Entry peerArtifactEntry `json:"entry,omitempty"`
	Size  int               `json:"size,omitempty"`
	Error string            `json:"error,omitempty"`
}
func fetchPeerRun(p peer.Peer, corr string) peerRunArc {
	out := peerRunArc{Peer: p.Name, URL: p.URL, CorrelationID: corr}
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	endpoint := strings.TrimRight(p.URL, "/") + "/api/v1/runs/" + url.PathEscape(corr)
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, endpoint, nil)
	if err != nil {
		out.Error = err.Error()
		return out
	}
	if p.Token != "" {
		req.Header.Set("Authorization", "Bearer "+p.Token)
	}
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		out.Error = err.Error()
		return out
	}
	defer resp.Body.Close()
	if resp.StatusCode == http.StatusUnauthorized {
		out.Error = "401 (token rejected)"
		return out
	}
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		out.Error = fmt.Sprintf("status %d", resp.StatusCode)
		return out
	}
	var body struct {
		CorrelationID string         `json:"correlation_id"`
		Count         int            `json:"count"`
		Events        []peerRunEvent `json:"events"`
	}
	if err := json.NewDecoder(io.LimitReader(resp.Body, maxPeerRunResponseBytes)).Decode(&body); err != nil {
		out.Error = "bad run response: " + err.Error()
		return out
	}
	if body.CorrelationID != "" {
		out.CorrelationID = body.CorrelationID
	}
	out.Count = body.Count
	if out.Count == 0 {
		out.Count = len(body.Events)
	}
	out.Events = body.Events
	return out
}
func fetchPeerArtifacts(p peer.Peer, corr string) peerArtifactList {
	out := peerArtifactList{Peer: p.Name, URL: p.URL, CorrelationID: corr}
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	endpoint := strings.TrimRight(p.URL, "/") + "/api/v1/artifacts"
	q := url.Values{}
	q.Set("corr", corr)
	q.Set("limit", fmt.Sprintf("%d", maxPeerArtifactEntries))
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, endpoint+"?"+q.Encode(), nil)
	if err != nil {
		out.Error = err.Error()
		return out
	}
	if p.Token != "" {
		req.Header.Set("Authorization", "Bearer "+p.Token)
	}
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		out.Error = err.Error()
		return out
	}
	defer resp.Body.Close()
	if resp.StatusCode == http.StatusUnauthorized {
		out.Error = "401 (token rejected)"
		return out
	}
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		out.Error = fmt.Sprintf("status %d", resp.StatusCode)
		return out
	}
	var body struct {
		CorrelationID string              `json:"correlation_id"`
		Count         int                 `json:"count"`
		TotalCount    int                 `json:"total_count"`
		Truncated     bool                `json:"truncated"`
		Entries       []peerArtifactEntry `json:"entries"`
	}
	if err := json.NewDecoder(io.LimitReader(resp.Body, maxPeerArtifactResponseBytes)).Decode(&body); err != nil {
		out.Error = "bad artifacts response: " + err.Error()
		return out
	}
	out.Count = body.Count
	if out.Count == 0 {
		out.Count = len(body.Entries)
	}
	out.TotalCount = body.TotalCount
	if out.TotalCount == 0 {
		out.TotalCount = out.Count
	}
	out.Truncated = body.Truncated
	out.Entries = body.Entries
	return out
}
func fetchPeerArtifactBytes(p peer.Peer, id string) (peerArtifactBytes, []byte) {
	out := peerArtifactBytes{Peer: p.Name, URL: p.URL, ID: id}
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()
	endpoint := strings.TrimRight(p.URL, "/") + "/api/v1/artifacts/" + url.PathEscape(id) + "/bytes"
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, endpoint, nil)
	if err != nil {
		out.Error = err.Error()
		return out, nil
	}
	if p.Token != "" {
		req.Header.Set("Authorization", "Bearer "+p.Token)
	}
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		out.Error = err.Error()
		return out, nil
	}
	defer resp.Body.Close()
	if resp.StatusCode == http.StatusUnauthorized {
		out.Error = "401 (token rejected)"
		return out, nil
	}
	if resp.StatusCode == http.StatusForbidden {
		out.Error = "403 (artifact bytes disabled on peer)"
		return out, nil
	}
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		out.Error = fmt.Sprintf("status %d", resp.StatusCode)
		return out, nil
	}
	var body struct {
		Entry peerArtifactEntry `json:"entry"`
		Size  int               `json:"size"`
		Data  string            `json:"data"`
	}
	if err := json.NewDecoder(io.LimitReader(resp.Body, maxPeerArtifactBytesResponseBytes)).Decode(&body); err != nil {
		out.Error = "bad artifact bytes response: " + err.Error()
		return out, nil
	}
	data, err := base64.StdEncoding.DecodeString(body.Data)
	if err != nil {
		out.Error = "bad artifact bytes response: invalid base64: " + err.Error()
		return out, nil
	}
	out.Entry = body.Entry
	out.Size = len(data)
	if body.Size > 0 && body.Size != len(data) {
		out.Error = fmt.Sprintf("bad artifact bytes response: size %d does not match decoded %d", body.Size, len(data))
		return out, nil
	}
	return out, data
}