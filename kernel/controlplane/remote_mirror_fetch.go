package controlplane

// Provenance: SPDX-License-Identifier: MIT kernel/controlplane remote-mirror HTTP
//             fetch helpers (fetchRemoteEvents, fetchRemoteArtifacts). Extracted
//             from remote_mirror.go during Day 211 god-file refactor (#86). Public
//             API unchanged.
//
// The fetches are the host's data access: they read the peer's HTTP endpoints
// and decode what they send. The rows that travel, the limits they travel
// under and the payload mode belong to the application layer
// (`kernel/app/remote/mirror.go`).

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strings"
	"time"

	appnodes "github.com/agezt/agezt/kernel/app/nodes"
	appremote "github.com/agezt/agezt/kernel/app/remote"
	"github.com/agezt/agezt/kernel/platform/netout"
)

const remoteEventMirrorResponseLimit = 4 << 20
const remoteArtifactMirrorResponseLimit = 1 << 20

func fetchRemoteEvents(ctx context.Context, p appnodes.Peer, corr string) ([]appremote.Event, error) {
	ctx, cancel := context.WithTimeout(ctx, 5*time.Second)
	defer cancel()
	endpoint := strings.TrimRight(p.URL, "/") + "/api/v1/runs/" + url.PathEscape(corr)
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, endpoint, nil)
	if err != nil {
		return nil, err
	}
	if p.Token != "" {
		req.Header.Set("Authorization", "Bearer "+p.Token)
	}
	resp, err := netout.OperatorClient(0).Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()
	if resp.StatusCode == http.StatusUnauthorized {
		return nil, fmt.Errorf("401 (token rejected)")
	}
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return nil, fmt.Errorf("status %d", resp.StatusCode)
	}
	var body struct {
		Events []appremote.Event `json:"events"`
	}
	if err := json.NewDecoder(io.LimitReader(resp.Body, remoteEventMirrorResponseLimit)).Decode(&body); err != nil {
		return nil, err
	}
	return body.Events, nil
}

func fetchRemoteArtifacts(ctx context.Context, p appnodes.Peer, corr string) ([]appremote.Artifact, bool, error) {
	ctx, cancel := context.WithTimeout(ctx, 5*time.Second)
	defer cancel()
	endpoint := strings.TrimRight(p.URL, "/") + "/api/v1/artifacts"
	q := url.Values{}
	q.Set("corr", corr)
	q.Set("limit", fmt.Sprintf("%d", appremote.MaxMirrorArtifacts))
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, endpoint+"?"+q.Encode(), nil)
	if err != nil {
		return nil, false, err
	}
	if p.Token != "" {
		req.Header.Set("Authorization", "Bearer "+p.Token)
	}
	resp, err := netout.OperatorClient(0).Do(req)
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
		Entries   []appremote.Artifact `json:"entries"`
		Truncated bool                 `json:"truncated"`
	}
	if err := json.NewDecoder(io.LimitReader(resp.Body, remoteArtifactMirrorResponseLimit)).Decode(&body); err != nil {
		return nil, false, err
	}
	return body.Entries, body.Truncated, nil
}
