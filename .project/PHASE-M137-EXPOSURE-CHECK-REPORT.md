# M137 — Network-exposure check (`agt doctor` + `agt status`)

## Why
A security-operability gap. The daemon's network-exposed HTTP servers — the web UI,
the native REST API, and the OpenAI-compatible API — drive the **full agent loop**
(shell / file / http tools), gated only by a bearer token. Binding any of them to a
non-loopback address (`0.0.0.0`, a public IP) therefore puts a code-executing agent
on the network. The daemon already warns **once at boot** per server
(`[WARNING: not loopback — reachable beyond localhost]`), but that scrolls past;
an operator running `agt doctor` later — the go-to security/health diagnostic — got
no signal. This makes the exposure a persistent, first-class check (the M98–M121
doctor pattern).

## What
- The daemon records its enabled HTTP servers via `SetHTTPBindings([]HTTPBinding{
  Name, Addr, Loopback})`, built from the configured `AGEZT_WEB_ADDR` /
  `AGEZT_REST_ADDR` / `AGEZT_API_ADDR` classified with the existing `isLoopback`
  helper.
- `agt status` (and `--json`) now reports `http_servers: [{name, addr, loopback}]`.
- `agt doctor` gains `checkExposure(status)` — reads the already-fetched status
  snapshot (no extra round-trip) and **WARNs** when any server is non-loopback,
  naming it and pointing at the fix (bind to 127.0.0.1 + TLS reverse proxy, or a
  firewall). All-loopback / no-HTTP is an OK. WARN (not FAIL): exposure can be a
  deliberate, firewalled choice — surfaced, not blocked.

## Files
- `kernel/controlplane/server.go` — `HTTPBinding` type, `httpBindings` field,
  `SetHTTPBindings`.
- `kernel/controlplane/status.go` — `http_servers` in the status response.
- `cmd/agezt/main.go` — collect bindings from the configured addrs + `isLoopback`;
  `srv.SetHTTPBindings(...)`.
- `cmd/agt/doctor.go` — `checkExposure`; wired into `runDoctorChecks`.
- `cmd/agt/doctor_test.go` — `TestCheckExposure` (non-loopback → WARN naming only
  the exposed one; all-loopback → OK; none → OK).

## Live proof (offline mock; REST on 0.0.0.0, web on 127.0.0.1)
```
$ agt doctor
  [WARN] network exposure : 1 HTTP server(s) reachable beyond localhost: rest api (0.0.0.0:8819)
           ↳ the agent (shell/file/http tools) is exposed to the network, gated only by
             a token — bind to 127.0.0.1 and front it with a TLS reverse proxy, or restrict
             with a firewall
  (doctor exit 0 — WARN, the operator's choice)

$ agt status --json | jq .http_servers
  [ {"name":"web ui","addr":"127.0.0.1:8820","loopback":true},
    {"name":"rest api","addr":"0.0.0.0:8819","loopback":false} ]
```

## Verification
- 55 packages `ok`, **FAIL 0**; **1433 tests**.
- `go vet` clean, `GOOS=linux go build ./...` ok, `go.mod` / `go.sum` unchanged.
- gofmt: all touched files clean under LF.
