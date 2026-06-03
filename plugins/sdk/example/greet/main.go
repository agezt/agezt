// SPDX-License-Identifier: MIT

// Command greet is a complete, runnable agezt plugin written with the
// official Go SDK (github.com/agezt/agezt/plugins/sdk). It is the
// counterpart to kernel/plugin/testdata/echoplugin — same capabilities,
// but the protocol plumbing is handled by the SDK so only the tool
// logic remains. Copy this file as the starting point for a new plugin.
//
// Point a daemon at the built binary with:
//
//	AGEZT_PLUGINS=greet=/path/to/greet agezt
//
// and the three tools below become available to the agent.
package main

import (
	"context"
	"encoding/json"
	"strings"

	"github.com/agezt/agezt/plugins/sdk"
)

func main() {
	sdk.Serve(
		// A plain tool: parse input, return output.
		sdk.Tool{
			Name:        "greet",
			Description: "Returns a friendly greeting for the given name.",
			InputSchema: json.RawMessage(`{"type":"object","properties":{"name":{"type":"string"}},"required":["name"]}`),
			Handle: func(ctx context.Context, input json.RawMessage) (sdk.Result, error) {
				var in struct {
					Name string `json:"name"`
				}
				if err := json.Unmarshal(input, &in); err != nil {
					return sdk.Errorf("invalid input: %v", err), nil
				}
				if in.Name == "" {
					return sdk.Errorf("name is required"), nil
				}
				return sdk.Text("Hello, " + in.Name + "!"), nil
			},
		},

		// A streaming tool: report progress as work proceeds. The host
		// forwards each Emit to whoever is watching the run.
		sdk.Tool{
			Name:        "slow",
			Description: "Counts to three, emitting progress for each step.",
			InputSchema: json.RawMessage(`{"type":"object"}`),
			Handle: func(ctx context.Context, input json.RawMessage) (sdk.Result, error) {
				for _, step := range []string{"one", "two", "three"} {
					sdk.Emit(ctx, "counting: "+step)
				}
				return sdk.Text("done"), nil
			},
		},

		// A composing tool: call back into a host tool and weave its
		// result in. The host decides which of its tools this plugin
		// may reach (operator-configured allow-list).
		sdk.Tool{
			Name:        "shout",
			Description: "Uppercases the input by delegating to the host's 'upper' tool.",
			InputSchema: json.RawMessage(`{"type":"object","properties":{"text":{"type":"string"}}}`),
			Handle: func(ctx context.Context, input json.RawMessage) (sdk.Result, error) {
				out, err := sdk.CallHost(ctx, "upper", input)
				if err != nil {
					return sdk.Errorf("host upper failed: %v", err), nil
				}
				return sdk.Text(strings.TrimSpace(out)), nil
			},
		},
	)
}
