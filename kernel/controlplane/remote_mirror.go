package controlplane

// Provenance: SPDX-License-Identifier: MIT kernel/controlplane remote-mirror
//             execution-profile event handler (mirrorRemoteExecutionProfileEvents).
//             Extracted from remote_mirror.go during Day 211 god-file refactor
//             (#86). Public API unchanged.
//
// The mirror's DECISIONS live in the application layer
// (`kernel/app/remote/mirror.go`): whether the mirror runs, what a missing
// peer or a failing peer looks like, which fields of a peer event travel and
// whether a payload is redacted before it travels. What stays here is the
// host's data access: the peer lookup, the two peer HTTP fetches, the
// redactor and the journal publish.

import (
	"context"

	appremote "github.com/agezt/agezt/kernel/app/remote"
	appruns "github.com/agezt/agezt/kernel/app/runs"
	"github.com/agezt/agezt/kernel/event"
	"github.com/agezt/agezt/kernel/runtime"
)

func (s *Server) mirrorRemoteExecutionProfileEvents(ctx context.Context, k *runtime.Kernel, corr string, meta map[string]string) {
	appremote.Mirror(ctx, appremote.MirrorPorts{
		LookupPeer:     s.lookupNodePeer,
		FetchEvents:    fetchRemoteEvents,
		FetchArtifacts: fetchRemoteArtifacts,
		Redact:         redactedRemotePayload,
		Publish: func(ctx context.Context, corr string, kind event.Kind, suffix string, payload map[string]any) error {
			return appruns.PublishRemoteRunEvent(k, corr, kind, suffix, payload)
		},
	}, appremote.EnvMode(), corr, meta)
}
