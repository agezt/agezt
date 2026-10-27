// SPDX-License-Identifier: MIT

package llm

import (
	"context"
	"encoding/json"
)

// Generator produces one schema-valid JSON value from a provider and
// unmarshals it into out. It is the structured-output path — the one way a
// model answer becomes a typed value — and it is INJECTED by the host.
//
// It lives here, in the provider's own contract, because it is not the
// memory module's path or the pulse module's path: it is every module's
// path. A module that needs it may not name the module that owns it, so the
// host injects the function and the module names this type.
//
// A nil generator means "no structured output available": the caller then
// reports that, rather than pretending it produced a value.
type Generator func(ctx context.Context, p Provider, req CompletionRequest, schema json.RawMessage, out any) (Usage, error)
