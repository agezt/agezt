# Phase Report — Milestone M105 (`agt netguard test`)

> Status: **shipped** · Date: 2026-06-01 · SPEC-06 / M16 egress guard.

## Why

The egress guard (M16) refuses the http/browser tools' connections to
internal/metadata addresses, defeating SSRF and cloud-metadata theft. But it
acted silently and invisibly: an operator could not ask "if a tool tries to
reach evil.example.com, will the guard stop it?" — especially the dangerous case
where a *public* hostname resolves to `169.254.169.254` or a private address
(an SSRF / DNS-rebinding trap). `agt netguard test <host>` previews that verdict
before any tool dials.

## What shipped

- **`agt netguard test <host|ip> [--json]`** — resolves the target and reports,
  per resolved IP, whether the guard would allow the connection and why. Exit 0
  = all allowed, 3 = at least one blocked (scriptable), 2 = usage/resolution
  error. Daemon-free: it constructs the guard locally.
- **`guardFromEnv`** — mirrors how the daemon configures the http tool
  (`AGEZT_HTTP_ALLOW_LOOPBACK` / `AGEZT_HTTP_ALLOW_PRIVATE`; link-local/metadata
  is never allowed), so running it in the daemon's environment gives a faithful
  preview.
- **`classifyIPs`** — pure, sorted per-IP verdicts over `netguard.Guard.Allowed`.

## Design notes

- **Catches the rebinding/SSRF case by design.** Because it resolves the host
  and classifies the *resulting* IPs (exactly what the guard's dial-time
  `Control` hook sees), a public name pointing at metadata is flagged BLOCK.
- **Daemon-free, faithful via env.** The guard's IP classification is config-
  independent; only the relax flags come from env, which `agt` shares with the
  operator who launched `agezt`. Documented in `--help`.

## Tests

- `TestClassifyIPs_DefaultGuard` — public allowed; loopback / private / metadata
  blocked with reasons under the strict default.
- `TestGuardFromEnv_Relaxations` — `AGEZT_HTTP_ALLOW_PRIVATE` permits 10/8 but
  metadata stays blocked; `AGEZT_HTTP_ALLOW_LOOPBACK` permits 127.0.0.1.

Test count: **1360 → 1362**. `go vet` clean, `GOOS=linux` builds, `go.mod`
unchanged, gofmt-clean.

## Live proof

```
$ agt netguard test 169.254.169.254
  [BLOCK] 169.254.169.254 — link-local (cloud metadata / autoconf)   (exit 3)
$ agt netguard test 1.1.1.1
  [ALLOW] 1.1.1.1                                                     (exit 0)
$ AGEZT_HTTP_ALLOW_PRIVATE=1 agt netguard test 192.168.1.10
  [ALLOW] 192.168.1.10                                               (exit 0)
```
