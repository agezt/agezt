// SPDX-License-Identifier: MIT
//
// cmd/agt `plugin new` scaffold file renderers (renderPluginMain, renderPluginGoMod,
// renderPluginReadme).
// Extracted from plugin_new.go during Day 211 god-file refactor (#101).
// Public API unchanged.
package main

import (
	"fmt"
	"go/format"
	"strings"

	"github.com/agezt/agezt/internal/brand"
)

func renderPluginMain(displayName, tool string) (string, error) {
	const tmpl = `// Command %s is an agezt tool plugin built with the official Go SDK
// (github.com/agezt/agezt/plugins/sdk). The SDK handles the stdio JSON
// protocol; everything below is just tool logic. Add more tools by
// passing more sdk.Tool values to sdk.Serve.
package main

import (
	"context"
	"encoding/json"

	"github.com/agezt/agezt/plugins/sdk"
)

func main() {
	sdk.Serve(sdk.Tool{
		Name:        "%s",
		Description: "Example tool — replace this with your own.",
		InputSchema: json.RawMessage(§{"type":"object","properties":{"text":{"type":"string"}}}§),
		Handle: func(ctx context.Context, input json.RawMessage) (sdk.Result, error) {
			var in struct {
				Text string §json:"text"§
			}
			if err := json.Unmarshal(input, &in); err != nil {
				return sdk.Errorf("invalid input: %%v", err), nil
			}
			// You can stream progress with sdk.Emit(ctx, "...") and call
			// host tools with sdk.CallHost(ctx, "tool", input).
			return sdk.Text("%s received: " + in.Text), nil
		},
	})
}
`
	src := fmt.Sprintf(tmpl, tool, tool, tool)
	src = strings.ReplaceAll(src, "§", "`")
	formatted, err := format.Source([]byte(src))
	if err != nil {
		return "", err
	}
	return string(formatted), nil
}
func renderPluginGoMod(module string) string {
	// The go directive matches the agezt module's floor; importing the
	// SDK pulls the agezt module, which requires a recent toolchain.
	// Module require versions are semver with a leading v; brand.Version
	// carries none, so add it here.
	return fmt.Sprintf(`module %s

go 1.25

require github.com/agezt/agezt v%s

// For local development against an agezt checkout, uncomment and point
// this at it (then `+"`go mod tidy`"+` resolves the SDK from disk):
// replace github.com/agezt/agezt => /path/to/agezt
`, module, brand.Version)
}
func renderPluginReadme(displayName, tool, module string) string {
	return fmt.Sprintf(`# %s

An [agezt](https://github.com/agezt/agezt) tool plugin, scaffolded with
`+"`agt plugin new`"+` and built on the Go SDK.

## Build

    go mod tidy
    go build -o %s .

## Run

Point a daemon at the built binary:

    AGEZT_PLUGINS="%s=./%s" agezt

The %s tool is then available to the agent. (Optionally pin the binary:
`+"`AGEZT_PLUGIN_PINS=\"%s=$(agt plugin hash ./%s)\"`"+`.)

## Develop

Edit `+"`main.go`"+`. Each tool is one `+"`sdk.Tool`"+` value passed to
`+"`sdk.Serve`"+`. Inside a handler you can:

- return `+"`sdk.Text(s)`"+` for success or `+"`sdk.Errorf(...)`"+` for a tool error;
- stream progress with `+"`sdk.Emit(ctx, msg)`"+`;
- call an allow-listed host tool with `+"`sdk.CallHost(ctx, name, input)`"+`.

Module: `+"`%s`"+`
`, displayName, tool, tool, tool, tool, tool, tool, module)
}
