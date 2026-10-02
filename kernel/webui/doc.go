// SPDX-License-Identifier: MIT

// Package webui serves the Agezt Web UI (SPEC-07, decision A4): a React 19 +
// Vite single-page app, built to static assets and go:embed-ded into the daemon
// (see embed.go) — one binary, no Node at runtime, and no Go dependency added.
// The Go side here is the thin server: it serves the embedded bundle and proxies
// the same control-plane commands + bus stream the SPA renders. Faithful to §0
// ("one event truth, many views; the UI never holds authoritative state, it
// subscribes and renders") and §5.2 ("Live Monitor driven entirely by events").
//
// It holds no state. Three data paths, all reusing what already exists:
//   - the SPA is the embedded Vite bundle (index.html + hashed /assets/*);
//   - the live event feed subscribes to the kernel bus (the same ">" stream the
//     daemon tees to stdout) over SSE at /events;
//   - every read panel proxies a control-plane command through the same Client
//     `agt` uses, so the CLI and the Web UI are guaranteed-consistent views and
//     no query logic is duplicated.
//
// Security (SPEC-06): the server is bound by the operator (loopback by
// default) and token-authed on every request. Reads are GET; the few
// mutating actions (halt, resume, approve/deny) are POST-only and pass the
// same token — a cross-site page can't forge them because it can't read the
// token, and the surface is loopback. The write set is a fixed allowlist
// (writeRoutes); there is no generic passthrough.
package webui
