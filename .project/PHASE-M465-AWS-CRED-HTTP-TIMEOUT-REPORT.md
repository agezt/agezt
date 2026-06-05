# M465 — Bound AWS credential-fetch HTTP calls (SSO / STS / web-identity)

## Context
The AWS credential chain can fetch credentials over HTTP from three endpoints:
SSO portal (`sso.go`), STS AssumeRole (`sts.go`), and web-identity / IRSA
(`web_identity.go`). The IMDS path (`aws.go`) already bounds itself
(`http.Client{Timeout: IMDSTimeout}` + a `context.WithTimeout`).

## The bug (MED — availability)
All three of the other paths built their client as:

```go
client := http.DefaultClient   // no timeout
```

and are reached from lookup wrappers that pass `context.Background()` (no
deadline). `http.DefaultClient` has no timeout either, so a stalled or black-holed
endpoint (a hung proxy, a TCP-accept-but-never-respond host, a partial network)
makes the credential lookup block **indefinitely**. Because these lookups run on
the credential chain at daemon startup, a hung endpoint can hang daemon boot with
no recovery. The IMDS path bounding itself while these don't is an inconsistency
that makes the omission a real defect rather than a deliberate choice.

## The fix
A shared, lowerable timeout and a bounded client at each site:

```go
// var (not const) so tests can lower it.
var credentialHTTPTimeout = 10 * time.Second
...
client := &http.Client{Timeout: credentialHTTPTimeout}   // sso.go, sts.go, web_identity.go
```

The existing `p.HTTP` injection is preserved: a caller-supplied `*http.Client` (or
custom `Do`-er) still wins; only the production default path changes from the
unbounded `http.DefaultClient` to a 10 s-bounded client. 10 s matches the existing
`credentialProcessTimeout`.

## Test + negative control
`kernel/creds/aws_http_timeout_internal_test.go` (white-box, to lower the unexported
timeout): `TestAssumeRole_HTTPTimeoutBounded` points `AssumeRole` at an httptest
server that never responds, lowers `credentialHTTPTimeout` to 100 ms, and asserts
the call returns an error within 3 s. STS is the behavioral witness; SSO and
web-identity share the identical one-line fix and the same timeout var.

**Negative control:** reverting `sts.go` to `client := http.DefaultClient` made the
call hang against the stalled server — the test reported `AssumeRole did not time
out against a hanging endpoint` and FAILED (3 s timeout). Restored; test passes.

## Other findings from this review (not fixed here)
The scoped review of creds/netguard/restapi found netguard (SSRF guard), creds at-
rest crypto (`encrypt.go`), and restapi all CLEAN. Two LOW creds items remain
documented: `Save()`/`Rotate()` use a non-unique `*.tmp` name under an RLock (only
matters under concurrent Saves; `agt` is single-shot today), and SSO query params
are concatenated unescaped (operator-controlled config; breaks role names with
`+`/`@`/`,`). Tracked as LOW.

## Verification / gate
- `kernel/creds` tests pass.
- gofmt-clean on staged LF blobs, `go vet` clean, `GOOS=linux` build clean, full
  `go test ./...` exit 0, `go.mod`/`go.sum` unchanged.
