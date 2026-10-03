// SPDX-License-Identifier: MIT

package toolapi

import (
	"context"
	"encoding/json"
)

// ToolLookup resolves registered or invocation-local tool implementations.
type ToolLookup interface {
	LookupTool(name string) (Tool, bool)
}

// ArtifactPutter stores the full audit output when an invocation offloads it.
type ArtifactPutter interface {
	Put(data []byte) (string, error)
}

// Invocation describes one call to the governed invocation pipeline. Lookup
// overrides the bound registry for invocation-local adapters. Artifact settings
// are supplied per call so profile overrides do not become constructor state.
type Invocation struct {
	CorrelationID, CallID, Name string
	Input                       json.RawMessage
	Lookup                      ToolLookup
	Artifacts                   ArtifactPutter
	ArtifactThreshold           int
}

// Invoker is the lower-layer port used by runtime callers. Application services
// are injected by the composition root; callers need not import their package.
type Invoker interface {
	Invoke(ctx context.Context, call Invocation) (Result, error)
}
