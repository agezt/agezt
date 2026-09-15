// SPDX-License-Identifier: MIT
//
// cmd/agt image-loading + run-modes + simple-dispatch + journal sub-command.
// Includes imageMediaType + loadImageDataURL + runJSONMode + runDryRunMode
// + parseUSDToMicrocents + toStringSlice + cmdSimple + cmdJournal.
// Split from main.go during Day 211 god-file refactor (#42).
// Public API unchanged.
package main

import (
	"context"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"time"

	"github.com/agezt/agezt/internal/brand"
	"github.com/agezt/agezt/kernel/controlplane"
	"github.com/agezt/agezt/kernel/event"
	dialpkg "github.com/agezt/agezt/cmd/agt/dial"
)


// imageMediaType maps a file extension to the IANA media type the vision
// providers accept (Anthropic/OpenAI both support exactly this set). An
// unknown extension returns ok=false so the caller can reject it with a clear
// message rather than send an undeliverable attachment.
func imageMediaType(path string) (string, bool) {
	switch strings.ToLower(filepath.Ext(path)) {
	case ".png":
		return "image/png", true
	case ".jpg", ".jpeg":
		return "image/jpeg", true
	case ".gif":
		return "image/gif", true
	case ".webp":
		return "image/webp", true
	}
	return "", false
}

// loadImageDataURL reads an image file and returns it as an RFC 2397 data:
// URL (data:<media-type>;base64,<bytes>). The CLI does the read because the
// file lives on the operator's machine, not the daemon's — so only this
// self-contained string crosses the control plane, and the daemon forwards it
// verbatim to a vision-capable provider (M241). Previously only the basename
// travelled, which no provider could resolve, so the image never reached the
// model.
func loadImageDataURL(path string) (string, error) {
	mt, ok := imageMediaType(path)
	if !ok {
		return "", fmt.Errorf("unsupported image type %q (use .png, .jpg, .gif, or .webp)", filepath.Ext(path))
	}
	b, err := os.ReadFile(path)
	if err != nil {
		return "", err
	}
	if len(b) == 0 {
		return "", fmt.Errorf("image is empty")
	}
	// The control plane caps a single request at 16 MiB (server.go
	// maxRequestBytes) and base64 inflates by ~4/3, so refuse early with a
	// clear message instead of letting the daemon reject an oversized frame.
	const maxRaw = 12 << 20
	if len(b) > maxRaw {
		return "", fmt.Errorf("image is %d bytes; the limit is %d (control-plane request cap)", len(b), maxRaw)
	}
	return "data:" + mt + ";base64," + base64.StdEncoding.EncodeToString(b), nil
}

// runJSONMode implements `agt run --json`. Output is one JSON
// object per line — every streamed event as `{"type":"event","event":{...}}`
// (matching the control-plane Response shape), then a final
// `{"type":"result","result":{...}}` line. This is JSON-Lines /
// ndjson — operators pipe into `jq -c` for filtering, or read
// line-by-line in any language.
//
// Ephemeral token events (KindLLMToken) are emitted alongside
// journaled events; consumers that only want stable data should
// filter `.event.seq > 0`. Including them keeps streaming
// consumers responsive (e.g. live UIs that mirror agt run).
func runJSONMode(ctx context.Context, c *controlplane.Client, runArgs map[string]any, stdout, stderr io.Writer) int {
	enc := json.NewEncoder(stdout)
	// Compact one-object-per-line is the convention for ndjson;
	// jq -c handles it natively. Using NewEncoder also flushes
	// after every Encode call, so each line streams as it arrives.
	result, err := c.Stream(ctx, controlplane.CmdRun, runArgs, func(ev *event.Event) {
		_ = enc.Encode(map[string]any{"type": "event", "event": ev})
	})
	if err != nil {
		// Final line: error envelope. Exit 1 so CI scripts
		// distinguish failure from success.
		_ = enc.Encode(map[string]any{"type": "error", "error": err.Error()})
		return 1
	}
	_ = enc.Encode(map[string]any{"type": "result", "result": result})
	return 0
}

// runDryRunMode implements `agt run --dry-run`: a single non-streaming Call
// returns the resolved plan (what the run WOULD do) and nothing executes. With
// --json the raw plan object is emitted; otherwise a compact human summary.
func runDryRunMode(ctx context.Context, c *controlplane.Client, runArgs map[string]any, asJSON bool, stdout, stderr io.Writer) int {
	plan, err := c.Call(ctx, controlplane.CmdRun, runArgs)
	if err != nil {
		if asJSON {
			_ = json.NewEncoder(stdout).Encode(map[string]any{"type": "error", "error": err.Error()})
		} else {
			fmt.Fprintf(stderr, "%s run --dry-run: %v\n", brand.CLI, err)
		}
		return 1
	}
	if asJSON {
		enc, _ := json.Marshal(plan)
		fmt.Fprintf(stdout, "%s\n", enc)
		return 0
	}

	str := func(k string) string { s, _ := plan[k].(string); return s }
	fmt.Fprintf(stdout, "dry-run — this run would execute as:\n")
	fmt.Fprintf(stdout, "  intent        : %s\n", str("intent"))
	fmt.Fprintf(stdout, "  tenant        : %s\n", str("tenant"))
	model := str("model")
	if known, _ := plan["model_known"].(bool); known {
		caps := []string{}
		if v, _ := plan["supports_vision"].(bool); v {
			caps = append(caps, "vision")
		}
		if v, _ := plan["supports_tools"].(bool); v {
			caps = append(caps, "tool_call")
		}
		capStr := "no advertised caps"
		if len(caps) > 0 {
			capStr = strings.Join(caps, "+")
		}
		fmt.Fprintf(stdout, "  model         : %s (%s) [catalog: %s]\n", model, str("model_source"), capStr)
	} else {
		fmt.Fprintf(stdout, "  model         : %s (%s) [not in catalog]\n", model, str("model_source"))
	}
	fmt.Fprintf(stdout, "  system prompt : %s\n", str("system_source"))
	fmt.Fprintf(stdout, "  timeout       : %s\n", str("timeout"))
	fmt.Fprintf(stdout, "  cost cap      : %s\n", str("cost_cap"))
	fmt.Fprintf(stdout, "  execution     : %s (%s), warden=%s\n", str("execution_profile"), str("execution_profile_source"), str("warden_profile"))
	if peer := str("remote_peer"); peer != "" {
		fmt.Fprintf(stdout, "  remote peer   : %s\n", peer)
	}

	tools := toStringSlice(plan["tools"])
	switch str("tools_mode") {
	case "all":
		fmt.Fprintf(stdout, "  tools         : all (%d): %s\n", len(tools), strings.Join(tools, ", "))
	case "restricted":
		fmt.Fprintf(stdout, "  tools         : restricted (%d): %s\n", len(tools), strings.Join(tools, ", "))
	default: // "none (--no-tools)"
		fmt.Fprintf(stdout, "  tools         : none (--no-tools)\n")
	}
	if dropped := toStringSlice(plan["tools_dropped"]); len(dropped) > 0 {
		fmt.Fprintf(stdout, "  tools dropped : %s (requested but not registered)\n", strings.Join(dropped, ", "))
	}
	if warns := toStringSlice(plan["warnings"]); len(warns) > 0 {
		fmt.Fprintf(stdout, "\nwarnings:\n")
		for _, w := range warns {
			fmt.Fprintf(stdout, "  ! %s\n", w)
		}
	}
	fmt.Fprintf(stdout, "\n(no run started, no tokens spent — drop --dry-run to execute)\n")
	return 0
}

// parseUSDToMicrocents converts a dollar amount ("0.50", "$0.50", "1") to
// USD-microcents, the kernel's internal spend unit ($1 = 1e9 microcents, matching
// governor.DefaultDailyCeilingMicrocents). Returns an error on a non-numeric or
// non-positive amount, so `--max-cost 0` / garbage is a usage error rather than a
// silently-uncapped run.
func parseUSDToMicrocents(s string) (int64, error) {
	s = strings.TrimSpace(strings.TrimPrefix(strings.TrimSpace(s), "$"))
	usd, err := strconv.ParseFloat(s, 64)
	if err != nil || usd <= 0 {
		return 0, fmt.Errorf("invalid amount %q", s)
	}
	return int64(usd * 1_000_000_000), nil
}

// toStringSlice coerces a decoded JSON array (interface slice) to []string,
// skipping non-string elements. Nil-safe.
func toStringSlice(v any) []string {
	if xs, ok := v.([]string); ok {
		return append([]string(nil), xs...)
	}
	list, ok := v.([]any)
	if !ok {
		return nil
	}
	out := make([]string, 0, len(list))
	for _, e := range list {
		if s, ok := e.(string); ok {
			out = append(out, s)
		}
	}
	return out
}

func cmdSimple(cmd string, args map[string]any, stdout, stderr io.Writer) int {
	c := dialpkg.New(stderr)
	if c == nil {
		return 1
	}
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()
	res, err := c.Call(ctx, cmd, args)
	if err != nil {
		fmt.Fprintf(stderr, "%s %s: %v\n", brand.CLI, cmd, err)
		return 1
	}
	enc, _ := json.MarshalIndent(res, "", "  ")
	fmt.Fprintf(stdout, "%s\n", enc)
	return 0
}

func cmdJournal(args []string, stdout, stderr io.Writer) int {
	if len(args) == 0 {
		fmt.Fprintf(stderr, "%s journal: subcommand required (verify|tail)\n", brand.CLI)
		return 2
	}
	switch args[0] {
	case "verify":
		return cmdJournalVerify(args[1:], stdout, stderr)
	case "tail":
		return cmdJournalTail(args[1:], stdout, stderr)
	case "grep":
		return cmdJournalGrep(args[1:], stdout, stderr)
	case "head":
		return cmdJournalHead(args[1:], stdout, stderr)
	case "export":
		return cmdJournalExport(args[1:], stdout, stderr)
	case "import":
		return cmdJournalImport(args[1:], stdout, stderr)
	case "stats":
		return cmdJournalStats(args[1:], stdout, stderr)
	default:
		fmt.Fprintf(stderr, "%s journal: unknown subcommand %q (verify|tail|grep|head|export|import|stats)\n", brand.CLI, args[0])
		return 2
	}
}
