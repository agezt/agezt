# M466 — Escape SSO GetRoleCredentials query parameters

## Context
`GetSSORoleCredentials` (kernel/creds/sso.go) fetches temporary credentials from
the AWS SSO portal with a `GET .../federation/credentials?account_id=…&role_name=…`
request. `account_id` and `role_name` come from the operator's `~/.aws/config`.

## The bug (LOW)
The query was built by raw string concatenation:

```go
url := ssoPortalEndpoint(...) +
    "/federation/credentials?account_id=" + p.AccountID +
    "&role_name=" + p.RoleName
```

IAM role names legitimately contain characters that are special in a URL query —
`+ = , . @ - _`. With raw concatenation, a `+` is decoded by the server as a space,
and `&`/`#`/`=` corrupt the query. So an operator whose role is e.g. `My+Admin@2`
would have the wrong `role_name` sent and the credential fetch would fail (or, in
principle, request a different role). Not attacker-controlled (it's local config),
so severity LOW — but a real correctness bug, and inconsistent with `sts.go`, which
already builds its form body with `url.Values`.

## The fix
Build the query with `url.Values` and proper encoding:

```go
reqURL := ssoPortalEndpoint(p.Region, p.Endpoint) + "/federation/credentials"
req, _ := http.NewRequestWithContext(ctx, "GET", reqURL, nil)
q := url.Values{}
q.Set("account_id", p.AccountID)
q.Set("role_name", p.RoleName)
req.URL.RawQuery = q.Encode()
```

## Test + negative control
`kernel/creds/sso_test.go`: `TestGetSSORoleCredentials_EscapesQueryParams` — uses a
role name `My+Admin@2,Role` and asserts the httptest portal receives it intact via
`r.URL.Query().Get("role_name")`.

**Negative control:** restoring raw concatenation
(`RawQuery = "account_id=" + ... + "&role_name=" + ...`) made the portal receive
`My Admin@2,Role` (the `+` decoded to a space) — the test FAILED with that exact
mismatch. Restored; test passes.

## Verification / gate
- `kernel/creds` tests pass.
- gofmt-clean on staged LF blobs, `go vet` clean, `GOOS=linux` build clean, full
  `go test ./...` exit 0, `go.mod`/`go.sum` unchanged.

## Remaining LOW (not fixed)
`Save()`/`Rotate()` use a non-unique `creds.json.tmp` under an RLock — only a
problem under concurrent `Save()` on one `Store`, which the current architecture
(single-shot `agt`, read-only daemon) never does. Left documented as a latent
footgun rather than churned.
