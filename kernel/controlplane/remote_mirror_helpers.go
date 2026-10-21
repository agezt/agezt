package controlplane

// Provenance: SPDX-License-Identifier: MIT kernel/controlplane remote-mirror typed
//             helpers (lookupNodePeer, redactedRemotePayload). Extracted from
//             remote_mirror.go during Day 211 god-file refactor (#86). Public API
//             unchanged.
//
// The mirror's mode policy and its payload mode moved to the application layer
// (`kernel/app/remote/mirror.go`). What stays here is the host's data access:
// the peer configuration and the redactor.

import (
	"encoding/json"

	appnodes "github.com/agezt/agezt/kernel/app/nodes"
	"github.com/agezt/agezt/kernel/redact"
)

func (s *Server) lookupNodePeer(name string) (appnodes.Peer, bool, error) {
	peers, err := appnodes.ParsePeers(s.nodePeerSpec())
	if err != nil {
		return appnodes.Peer{}, false, err
	}
	for _, p := range peers {
		if p.Name == name {
			return p, true, nil
		}
	}
	return appnodes.Peer{}, false, nil
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
