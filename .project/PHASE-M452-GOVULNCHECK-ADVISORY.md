# M452 — govulncheck advisory: build with Go 1.26.4+

## Context
Ran `govulncheck ./...` (DB 2026-06-02, scanner v1.3.0) over the whole codebase
as a security-hardening verification step — the first vulnerability scan recorded
in this arc.

## Findings — 2 reachable standard-library vulnerabilities
Both are **Go standard library** issues, **fixed in go1.26.4**; the module pins
`go 1.26.3` (go.mod) and the build toolchain is go1.26.3.

### GO-2026-5039 — net/textproto: unescaped arbitrary input in errors
- Fixed in net/textproto@go1.26.4.
- Reachable traces:
  - `plugins/channels/email/email.go` → `smtp.SendMail` → `textproto.Reader.ReadResponse`
    (parses an SMTP server's response; the genuine path — an operator-configured,
    possibly hostile/misbehaving SMTP server).
  - `plugins/external/mcpbridge/main.go` → `fmt.Fprintln` → `textproto.Error.Error`.
  - `kernel/journal/journal.go` → `bufio.Scanner.Scan` → `textproto.Reader.ReadMIMEHeader`
    — a conservative static trace; the journal uses a custom split func, not MIME
    header reading, so this path is not exercised at runtime.
- Severity: LOW — an error-message escaping issue, not RCE.

### GO-2026-5037 — crypto/x509: inefficient candidate hostname parsing
- Fixed in crypto/x509@go1.26.4.
- Reachable traces:
  - `plugins/channels/slack/slack.go` → `http.Server.ListenAndServe` →
    `x509.Certificate.Verify`/`VerifyHostname` — a conservative trace through the
    TLS-capable `ListenAndServe`; the channel servers run **plaintext** (no TLS, no
    `ListenAndServeTLS`), so x509 verification is not invoked at runtime.
  - `plugins/external/mcpbridge/main.go` → `x509.HostnameError.Error` (only on a
    cert-verification error, which mcpbridge's stdio transport never produces).
- Severity: LOW — a parsing-inefficiency (DoS-adjacent) issue; the reachable paths
  are largely not exercised at runtime.

(One further vulnerability exists in an imported package but the code doesn't call
it — informational only.)

## Remediation (environment / release, not a code change)
Build and ship with **go1.26.4 or later**. Both vulnerabilities are stdlib-only
and are fully resolved by the patched toolchain; no source change is required.

## Why this is documented, not applied here
1. The fix is a toolchain upgrade, not code — there is no source-level defect.
2. The standing project constraint keeps **go.mod / go.sum unchanged**; bumping the
   `go 1.26.3` directive to `1.26.4` is a go.mod change.
3. With go1.26.3 installed locally, setting `go 1.26.4` would trigger a toolchain
   auto-download on the next build (network) and make the gate **unverifiable
   offline** — bumping it blind, without being able to re-run the build+suite on
   1.26.4, would violate the "don't apply unverifiable changes" discipline.

So this milestone records the advisory; the toolchain bump is a release-time action
(update CI/build images and, if the operator chooses, the go.mod `go` directive to
`1.26.4`) to be verified where go1.26.4 is installed.

## Action for the release/build process
- Pin the build toolchain to **go1.26.4+** (CI image, Dockerfile base, dev setup).
- Optionally bump go.mod `go 1.26.4` once a 1.26.4 toolchain is available to build
  and re-run the gate.
- Re-run `govulncheck ./...` on 1.26.4 to confirm a clean scan.

## CHANGELOG
A Security advisory entry records the recommended minimum Go toolchain.
