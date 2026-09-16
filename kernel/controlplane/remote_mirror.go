// SPDX-License-Identifier: MIT
//
// kernel/controlplane remote-mirror execution-profile event handler (mirrorRemoteExecutionProfileEvents).
// Extracted from remote_mirror.go during Day 211 god-file refactor (#86).
// Public API unchanged.
package controlplane

import (
	"context"
	"strings"

	"github.com/agezt/agezt/kernel/event"
	"github.com/agezt/agezt/kernel/runtime"
)

func (s *Server) mirrorRemoteExecutionProfileEvents(ctx context.Context, k *runtime.Kernel, corr string, meta map[string]string) {
	mode := remoteEventMirrorMode()
	if mode == "" {
		return
	}
	peerName := strings.TrimSpace(meta["remote_peer"])
	remoteCorr := strings.TrimSpace(meta["remote_correlation"])
	if peerName == "" || remoteCorr == "" {
		_ = publishRemoteExecutionProfileRunEvent(k, corr, event.KindInfo, "remote", map[string]any{
			"profile": "remote-agezt",
			"phase":   "peer_events_unavailable",
			"mode":    mode,
			"error":   "remote peer/correlation metadata missing",
		})
		return
	}
	peer, ok, err := s.lookupNodePeer(peerName)
	if err != nil || !ok {
		msg := "peer not found"
		if err != nil {
			msg = err.Error()
		}
		_ = publishRemoteExecutionProfileRunEvent(k, corr, event.KindInfo, "remote", map[string]any{
			"profile":            "remote-agezt",
			"phase":              "peer_events_unavailable",
			"mode":               mode,
			"remote_peer":        peerName,
			"remote_correlation": remoteCorr,
			"error":              msg,
		})
		return
	}
	events, truncated, err := fetchRemoteEvents(ctx, peer, remoteCorr, mode)
	if err != nil {
		_ = publishRemoteExecutionProfileRunEvent(k, corr, event.KindInfo, "remote", map[string]any{
			"profile":            "remote-agezt",
			"phase":              "peer_events_unavailable",
			"mode":               mode,
			"remote_peer":        peerName,
			"remote_correlation": remoteCorr,
			"error":              err.Error(),
		})
		return
	}
	payload := map[string]any{
		"profile":            "remote-agezt",
		"phase":              "peer_events_mirrored",
		"mode":               mode,
		"payload_mode":       remoteMirrorPayloadMode(mode),
		"remote_peer":        peerName,
		"remote_correlation": remoteCorr,
		"count":              len(events),
		"truncated":          truncated,
		"events":             events,
	}
	if artifacts, artifactTruncated, artifactErr := fetchRemoteArtifacts(ctx, peer, remoteCorr); artifactErr == nil {
		payload["artifact_count"] = len(artifacts)
		payload["artifacts_truncated"] = artifactTruncated
		payload["artifacts"] = artifacts
	} else {
		payload["artifacts_unavailable"] = artifactErr.Error()
	}
	_ = publishRemoteExecutionProfileRunEvent(k, corr, event.KindInfo, "remote", payload)
}
