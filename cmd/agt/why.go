// SPDX-License-Identifier: MIT

package main

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"time"

	"github.com/agezt/agezt/internal/brand"
	"github.com/agezt/agezt/kernel/controlplane"
)

// cmdWhy implements `agt why <event_id> [--json|--payload]`.
//
// Default output is a one-line summary per event (seq/kind/subject)
// — fast to skim when an operator just wants to know "did the
// task get to step N?".
//
// --json    dumps the full events array verbatim, suitable for
//
//	piping into jq or a teammate ("share the full chain
//	that led to this failure"). Includes every field
//	in *event.Event including payloads.
//
// --payload renders each event human-readable but inlines the
//
//	payload JSON pretty-printed; useful for eyeballing
//	the tool call/result bodies without leaving the
//	terminal. AND-composes with --json (--json wins).
func cmdWhy(args []string, stdout, stderr io.Writer) int {
	asJSON := false
	withPayload := false
	var eventID string

	for _, a := range args {
		switch a {
		case "--json":
			asJSON = true
		case "--payload":
			withPayload = true
		case "-h", "--help":
			fmt.Fprintf(stdout, "usage: %s why <event_id> [--json|--payload]\n", brand.CLI)
			fmt.Fprintf(stdout, "list every event sharing an event's correlation chain\n")
			fmt.Fprintf(stdout, "  --json     dump the full events array (jq-friendly)\n")
			fmt.Fprintf(stdout, "  --payload  render human-readable but include payload bodies\n")
			return 0
		default:
			if eventID == "" {
				eventID = a
				continue
			}
			fmt.Fprintf(stderr, "%s why: unexpected arg %q\n", brand.CLI, a)
			return 2
		}
	}
	if eventID == "" {
		fmt.Fprintf(stderr, "%s why: event_id required\n", brand.CLI)
		return 2
	}

	c := dial(stderr)
	if c == nil {
		return 1
	}
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()
	res, err := c.Call(ctx, controlplane.CmdWhy, map[string]any{"event_id": eventID})
	if err != nil {
		fmt.Fprintf(stderr, "%s why: %v\n", brand.CLI, err)
		return 1
	}
	events, _ := res["events"].([]any)
	parent, _ := res["parent_correlation"].(string)

	if asJSON {
		// Re-wrap so the JSON output is self-describing — a bare
		// array is less helpful when piped without context. Mirrors
		// the shape `agt status --json` returns.
		out := map[string]any{
			"event_id":           eventID,
			"count":              len(events),
			"correlation":        res["correlation"],
			"parent_correlation": parent,
			"events":             events,
		}
		enc := json.NewEncoder(stdout)
		enc.SetIndent("", "  ")
		_ = enc.Encode(out)
		return 0
	}

	fmt.Fprintf(stdout, "%d events in correlation:\n", len(events))
	for _, raw := range events {
		m, _ := raw.(map[string]any)
		fmt.Fprintf(stdout, "  seq=%-4v kind=%-22v subject=%v\n",
			m["seq"], m["kind"], m["subject"])
		if withPayload {
			renderPayload(stdout, m["payload"])
		}
	}
	// Parent backlink (M42): this chain is a sub-agent's — point up to its
	// lead. `agt runs show <parent>` renders the lead's task arc (which
	// includes the `delegated → …` line back to this child).
	if parent != "" {
		fmt.Fprintf(stdout, "\nspawned by %s  (try: %s runs show %s)\n", parent, brand.CLI, parent)
	}
	return 0
}

// renderPayload pretty-prints an event's payload below its
// summary line, indented under the event for visual nesting.
// Skips silently when there's no payload — many event kinds
// (task.received, halt) carry only the kind itself, and an
// empty "    (no payload)" line would just be noise.
func renderPayload(w io.Writer, payload any) {
	if payload == nil {
		return
	}
	// Payload comes through as either a base64 string (event.Event
	// marshals []byte that way) or a decoded structure depending on
	// how the response was constructed. Handle both, falling back
	// to %v if the shape is unexpected.
	switch p := payload.(type) {
	case string:
		if p == "" {
			return
		}
		// Try to interpret as JSON for pretty-printing — if it's not
		// JSON (e.g. genuine base64 binary), render the raw string.
		var pretty any
		if err := json.Unmarshal([]byte(p), &pretty); err == nil {
			renderJSONIndented(w, pretty)
			return
		}
		fmt.Fprintf(w, "      %s\n", p)
	case map[string]any, []any:
		renderJSONIndented(w, p)
	default:
		fmt.Fprintf(w, "      %v\n", p)
	}
}

func renderJSONIndented(w io.Writer, v any) {
	b, err := json.MarshalIndent(v, "      ", "  ")
	if err != nil {
		return
	}
	fmt.Fprintf(w, "      %s\n", b)
}
