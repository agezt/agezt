# Phase Report — Milestone M17 (Operator-extensible hard-deny rules)

> Status: **shipped** · Date: 2026-05-31
> DECISIONS F4 / SPEC-06. The Edict policy engine's hard-deny layer was a fixed
> built-in list (fork-bomb, `rm -rf /`, `mkfs`, …). Operators can now append
> their own site-specific deny rules via `AGEZT_EDICT_DENY`, without forking.

## Why

Edict's hard-deny rules are the non-overridable floor of the policy engine: they
fire regardless of trust level, before any allow decision. The built-in set
covers universally-catastrophic shell patterns, but every deployment has its own
"never do this" list the kernel can't know — `git push` on a read-only mirror,
touching `/etc/shadow`, calling an internal admin URL, `kubectl delete`. Until
now the only way to add one was to fork `DefaultHardDeny()`. The `Options.HardDeny`
seam already existed; M17 makes it reachable from operator config.

## What shipped

- **`edict.ParseDenyRules(spec)`** parses a `;`-separated spec into
  `HardDenyRule`s appended to the built-ins. Each entry is either:
  - `substring` — denied for **every** capability; or
  - `<capability>:substring` — denied only for that capability, when the text
    before the first `:` is a known capability (`shell:rm -rf /etc`,
    `http.post:169.254`, `file.delete:/etc`).

  The capability check is what makes this unambiguous: `https://evil.example` has
  the prefix `https`, which is **not** a capability, so the whole string is taken
  verbatim as an all-capability substring — URLs and colons in patterns just work.
- **Empty-substring rejection.** A rule whose substring is blank (e.g. a bare
  `shell:`) would match the empty string and deny *every* action — that's a
  config footgun, so it's a hard parse error, surfaced at startup. A blank/all-
  whitespace spec is a no-op (not an error).
- **`edict.AllCapabilities()`** — the sorted capability list, used by the parser
  and available for operator-facing listings.
- **Daemon wiring.** `AGEZT_EDICT_DENY` is parsed at startup and appended to
  `DefaultHardDeny()`; a malformed rule fails the boot with a clear message. The
  policy banner reports the count (`…; +2 operator deny rule(s)`). Rules are named
  `operator[N]` so a denial's journaled reason points at which one fired.

## Proven

- **Unit:** the parse table (all-caps vs cap-scoped, the `https://` non-capability
  prefix, the skipped blank entry); empty-substring rejection; and rules firing
  through `Decide` — an all-caps `git push` rule hard-denies even at trust L4, a
  `shell:`-scoped rule fires for shell but **not** for `http.post`, the built-in
  `rm -rf /` still fires alongside the custom rules, and an ordinary command is
  still allowed.
- **Live (daemon + `agt edict test`):** booting with
  `AGEZT_EDICT_DENY="git push;shell:/etc/shadow"` the banner shows
  `+2 operator deny rule(s)`; `edict test shell "git push origin main"` →
  `deny (L0), operator[1]`; `"cat /etc/shadow"` → `deny, operator[2]`;
  `"echo hello"` → `allow`.

3 new tests; suite **1149** green, `go vet` clean, `GOOS=linux` builds,
`go.mod` unchanged.

## Deferred — named

- **Per-tenant deny rules** (today the operator set is global; a tenant could carry
  its own additional rules).
- **Runtime management** (`agt edict deny add/list/rm` over the control plane) so
  rules can change without a restart — today it is startup config.
- **Glob/regex rules** beyond substring, with the same compile-error care as the
  redaction custom-pattern follow-up (M15).
