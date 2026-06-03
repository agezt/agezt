// SPDX-License-Identifier: MIT

package main

import (
	"fmt"
	"go/format"
	"io"
	"os"
	"path/filepath"
	"strings"

	"github.com/agezt/agezt/internal/brand"
)

// cmdPluginNew implements `agt plugin new <name> [--dir <path>] [--module <path>]`.
//
// It scaffolds a complete, buildable agezt tool plugin built on the Go
// SDK (github.com/agezt/agezt/plugins/sdk) — the realisation of the
// ROADMAP's `create-agezt-plugin` item. The generated project is just
// the author's tool logic; the SDK handles the wire protocol. The
// scaffold is the on-ramp that makes M221's SDK usable without copying
// the example by hand.
//
// Generated layout (in <dir>, default ./<name>):
//
//	main.go     — an SDK plugin with one example tool, gofmt-clean
//	go.mod      — module + require on agezt, with a local-dev replace hint
//	README.md   — how to build the binary and wire it via AGEZT_PLUGINS
//	.gitignore  — ignores the built binary
//
// The scaffolder refuses to write into a non-empty directory so it can
// never clobber existing work.
func cmdPluginNew(args []string, stdout, stderr io.Writer) int {
	var name, dir, module string
	for i := 0; i < len(args); i++ {
		a := args[i]
		switch {
		case a == "-h" || a == "--help":
			pluginNewUsage(stdout)
			return 0
		case a == "--dir":
			if i+1 >= len(args) {
				fmt.Fprintf(stderr, "%s plugin new: --dir requires a value\n", brand.CLI)
				return 2
			}
			i++
			dir = args[i]
		case a == "--module":
			if i+1 >= len(args) {
				fmt.Fprintf(stderr, "%s plugin new: --module requires a value\n", brand.CLI)
				return 2
			}
			i++
			module = args[i]
		case strings.HasPrefix(a, "-"):
			fmt.Fprintf(stderr, "%s plugin new: unknown flag %q\n", brand.CLI, a)
			return 2
		default:
			if name != "" {
				fmt.Fprintf(stderr, "%s plugin new: unexpected extra argument %q\n", brand.CLI, a)
				return 2
			}
			name = a
		}
	}

	if name == "" {
		fmt.Fprintf(stderr, "%s plugin new: a plugin name is required\n", brand.CLI)
		pluginNewUsage(stderr)
		return 2
	}

	// The tool identifier the plugin advertises is derived from the
	// name, sanitised to the conservative set agezt tool names use.
	tool := sanitizeToolName(name)
	if tool == "" {
		fmt.Fprintf(stderr, "%s plugin new: name %q has no usable letters/digits for a tool name\n", brand.CLI, name)
		return 2
	}
	if dir == "" {
		dir = name
	}
	if module == "" {
		module = "agezt-plugin-" + tool
	}

	// Refuse to write into a non-empty directory.
	if entries, err := os.ReadDir(dir); err == nil && len(entries) > 0 {
		fmt.Fprintf(stderr, "%s plugin new: directory %q is not empty — refusing to overwrite\n", brand.CLI, dir)
		return 1
	}
	if err := os.MkdirAll(dir, 0o755); err != nil {
		fmt.Fprintf(stderr, "%s plugin new: create %q: %v\n", brand.CLI, dir, err)
		return 1
	}

	mainSrc, err := renderPluginMain(name, tool)
	if err != nil {
		// format.Source failing means the template is broken — a
		// programming error, not user input. Surface it loudly.
		fmt.Fprintf(stderr, "%s plugin new: internal: render main.go: %v\n", brand.CLI, err)
		return 1
	}

	files := map[string]string{
		"main.go":    mainSrc,
		"go.mod":     renderPluginGoMod(module),
		"README.md":  renderPluginReadme(name, tool, module),
		".gitignore": "/" + tool + "\n/" + tool + ".exe\n",
	}
	for fname, content := range files {
		if err := os.WriteFile(filepath.Join(dir, fname), []byte(content), 0o644); err != nil {
			fmt.Fprintf(stderr, "%s plugin new: write %s: %v\n", brand.CLI, fname, err)
			return 1
		}
	}

	fmt.Fprintf(stdout, "Scaffolded plugin %q in %s\n\n", name, dir)
	fmt.Fprintf(stdout, "Next steps:\n")
	fmt.Fprintf(stdout, "  cd %s\n", dir)
	fmt.Fprintf(stdout, "  go mod tidy            # resolve the agezt SDK dependency\n")
	fmt.Fprintf(stdout, "  go build -o %s .\n", tool)
	fmt.Fprintf(stdout, "  %s=%q %s\n", "AGEZT_PLUGINS", tool+"=./"+tool, brand.Binary)
	fmt.Fprintf(stdout, "\nThe %q tool is then available to the agent. Edit main.go to add your own.\n", tool)
	return 0
}

func pluginNewUsage(w io.Writer) {
	fmt.Fprintf(w, "usage: %s plugin new <name> [--dir <path>] [--module <modulepath>]\n", brand.CLI)
	fmt.Fprintf(w, "\n")
	fmt.Fprintf(w, "Scaffold a new Go tool plugin built on the agezt SDK.\n")
	fmt.Fprintf(w, "  --dir <path>          target directory (default: ./<name>)\n")
	fmt.Fprintf(w, "  --module <modulepath> go module path (default: agezt-plugin-<name>)\n")
}

// sanitizeToolName reduces an arbitrary name to the characters agezt
// tool names use: lowercased ASCII letters, digits, underscore and
// dash. Runs of other characters collapse to a single dash; leading and
// trailing dashes are trimmed.
func sanitizeToolName(name string) string {
	var b strings.Builder
	lastDash := false
	for _, r := range strings.ToLower(name) {
		switch {
		case r >= 'a' && r <= 'z', r >= '0' && r <= '9', r == '_':
			b.WriteRune(r)
			lastDash = false
		case r == '-' || r == ' ' || r == '.' || r == '/':
			if !lastDash && b.Len() > 0 {
				b.WriteByte('-')
				lastDash = true
			}
		default:
			// drop other runes entirely
		}
	}
	return strings.Trim(b.String(), "-")
}

// renderPluginMain builds the example main.go and runs it through
// go/format so the scaffold is always gofmt-clean and — because
// format.Source parses it — guaranteed to be valid Go.
//
// The template uses § as a stand-in for the backtick character, which a
// Go raw-string literal cannot contain; it is substituted after the
// fmt verbs are filled.
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
