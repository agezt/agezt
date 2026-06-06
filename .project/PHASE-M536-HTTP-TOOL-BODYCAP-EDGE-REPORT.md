# M536 — Mutation testing the http tool: pin the request-body cap inclusive edge

## Context
Second `plugins/` target: `plugins/tools/http` (the agent's outbound HTTP tool —
host-allowlist + netguard egress + scheme/method/size limits). `GOMAXPROCS=3`.

## Triage — the SSRF/security core is solid
Negative control confirmed the security-critical paths are pinned:
- `hostAllowed` exact match (`pat == host → !=` killed) and wildcard subdomain
  (`*.example.com` matches `foo.example.com`, the `→ ==` mutant killed). The bare-apex
  guard `host != suffix[1:]` is **redundant/equivalent** — `HasSuffix(host, ".example.com")`
  already excludes the shorter bare `example.com` by length, so removing the guard survives
  but changes nothing.
- scheme restriction, method validation, netguard egress (loopback/private/metadata), and
  the per-redirect-hop allowlist re-check (M251) are covered by the existing suite.

## The genuine gap (closed)
```
if len(in.Body) > MaxRequestBodyBytes { return errResult("body too large …") }
```

`TestBodyTooLarge` uses `MaxRequestBodyBytes+1` (strictly over), so the exact-limit edge
was unpinned: `> → >=` survived — a request body of *exactly* `MaxRequestBodyBytes`
(256 KiB) would be wrongly rejected as too large. Same inclusive-max class as the plugin
`readFrame` (M509) and control-plane `readBoundedLine` (M531) guards.

## Fix
Added `TestBodyExactlyAtMax`: a POST with a body of exactly `MaxRequestBodyBytes` to a
loopback echo server is accepted (no "too large", no error).

## Negative control (manual, CPU-capped)
`len(in.Body) > MaxRequestBodyBytes → >=`: FAIL (the exactly-at-cap body is rejected).
Restored byte-for-byte (`git diff --ignore-all-space` on http.go empty); passes again.

## Verification / gate
- Test passes; `go vet` + `staticcheck` clean; gofmt-clean on the staged LF blob.
- Full `go test ./...` exit 0 (`GOMAXPROCS=3`, `-p 2`); `go.mod`/`go.sum` unchanged.

## Plugins-tree mutation progress
plugins/tools/file (M535, containment solid + read-range edge), plugins/tools/http (M536,
SSRF allowlist solid + body-cap edge). The redirect-chain cap (`len(via) >= maxRedirects`)
exact edge is a low-stakes one-hop difference (netguard guards every hop regardless) and
the response-truncation edge is cosmetic — left unpinned honestly. Remaining plugin
targets: shell exec, mcpbridge protocol, channel adapters.
