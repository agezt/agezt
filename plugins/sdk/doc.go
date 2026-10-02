// SPDX-License-Identifier: MIT

// Package sdk is the official Go authoring kit for agezt tool plugins.
//
// Writing a plugin by hand means implementing the line-delimited JSON
// protocol yourself: the stdin read loop, the request/response frame
// demux, write serialisation across goroutines, progress streaming, and
// host-callback routing. The reference plugin (kernel/plugin/testdata/
// echoplugin) is ~260 lines of exactly that boilerplate. This package
// collapses it so a plugin author writes only their tool logic:
//
//	package main
//
//	import (
//		"context"
//		"encoding/json"
//
//		"github.com/agezt/agezt/plugins/sdk"
//	)
//
//	func main() {
//		sdk.Serve(sdk.Tool{
//			Name:        "greet",
//			Description: "Returns a greeting.",
//			InputSchema: json.RawMessage(`{"type":"object","properties":{"name":{"type":"string"}}}`),
//			Handle: func(ctx context.Context, input json.RawMessage) (sdk.Result, error) {
//				var in struct{ Name string `json:"name"` }
//				json.Unmarshal(input, &in)
//				return sdk.Text("hello, " + in.Name), nil
//			},
//		})
//	}
//
// That binary is a complete, spec-conformant agezt plugin. Point a
// daemon at it with AGEZT_PLUGINS and the "greet" tool is live.
//
// # Design constraints (mirrors the host)
//
// This package imports ONLY the Go standard library. It deliberately
// does NOT import kernel/plugin or kernel/agent: a plugin must not have
// to compile against the kernel (DECISIONS B0). The wire types here are
// independent copies of the small plugin-side contract documented in
// kernel/plugin/protocol.go — the same on-disk JSON shape an author in
// Python or Rust would target by hand.
//
// # What Serve handles for you
//
//   - initialize: replies with the tool definitions you registered.
//   - tool/invoke: routes to the matching Handle, on its own goroutine
//     so the read loop stays responsive to concurrent invokes and to
//     host-callback replies.
//   - shutdown: returns cleanly.
//   - Panics inside a handler are recovered and surfaced as a tool error
//     (IsError) rather than crashing the plugin — one bad tool call does
//     not take the process down.
//   - Progress streaming via Emit and host callbacks via CallHost, both
//     keyed to the in-flight request so you never touch a frame id.
package sdk
