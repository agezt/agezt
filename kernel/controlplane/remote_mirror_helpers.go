// SPDX-License-Identifier: MIT
//
// kernel/controlplane remote-mirror typed helpers (remoteEventMirrorMode, lookupNodePeer,
// redactedRemotePayload, remoteMirrorPayloadMode).
// Extracted from remote_mirror.go during Day 211 god-file refactor (#86).
// Public API unchanged.
package controlplane

import (
	"encoding/json"
	"os"
	"strings"

	"github.com/agezt/agezt/internal/brand"
	"github.com/agezt/agezt/kernel/redact"
)

func remoteEventMirrorMode() string {
	switch strings.ToLower(strings.TrimSpace(os.Getenv(brand.EnvPrefix + "REMOTE_EVENT_MIRROR"))) {
	case "1", "true", "yes", "on", "metadata":
		return "metadata"
	case "redacted", "payload", "payload-redacted":
		return "redacted"
	default:
		return ""
	}
}
func (s *Server) lookupNodePeer(name string) (nodePeer, bool, error) {
	peers, err := parseNodePeers(s.nodePeerSpec())
	if err != nil {
		return nodePeer{}, false, err
	}
	for _, p := range peers {
		if p.Name == name {
			return p, true, nil
		}
	}
	return nodePeer{}, false, nil
}
func redactedRemotePayload(raw json.RawMessage) (any, bool) {
	if len(raw) == 0 || string(raw) == "null" {
		return nil, false
	}
	red := redact.New().RedactBytes(raw)
	var payload any
	if err := json.Unmarshal(red, &payload); err != nil {
		return nil, false
	}
	return payload, true
}
func remoteMirrorPayloadMode(mode string) string {
	if mode == "redacted" {
		return "redacted"
	}
	return "none"
}
