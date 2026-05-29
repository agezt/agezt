# Phase Report — Milestone 1.ff (Plugin signing / pin verification)

> Status: **shipped** · Date: 2026-05-29
> Closes the M1.y "plugin signing" deferral.
> Continues [PHASE-M1.ee-REPORT.md](PHASE-M1.ee-REPORT.md).

## Scope

M1.y shipped the out-of-process plugin host: `AGEZT_PLUGINS=...`
declares plugins, the daemon spawns each as a subprocess. Reading
the M1.y report again:

> Deferred: plugin sandboxing, **signing**, hot-reload, streaming, callbacks.

Of those, **signing** is the easiest win and the one with the
most concrete threat model: an operator pins the BLAKE3-256 hash
of each plugin binary, and the daemon refuses to spawn a binary
whose hash drifts.

```
AGEZT_PLUGINS="search=/opt/agezt/search,scrape=/opt/agezt/scrape"
AGEZT_PLUGIN_PINS="search=ec64a4b2...,scrape=8fb12d09..."
agezt
# Either both binaries match → daemon spawns both as usual.
# Either binary drifted (apt upgrade replaced it; attacker swapped
# it; bit-rot on a flaky disk) → daemon refuses with a clear
# "plugin pin mismatch" error before exec'ing the child.
```

Operators record the hash once via `agt plugin hash <path>` and
substitute it back into the env var. No PKI required.

| Concern | Status |
|---|---|
| `plugin.Config.PinnedHash` — opt-in, empty default = no change | ✅ |
| `Spawn` verifies hash BEFORE starting the child (no exec on drift) | ✅ tested |
| BLAKE3-256 streaming hash (already a project dep) | ✅ |
| `plugin.VerifyPin(path, pin)` — exported for CLI + tests | ✅ tested |
| `plugin.HashFile(path)` — exported for the `agt plugin hash` helper | ✅ tested |
| Distinct `ErrPinMismatch` sentinel for downstream `errors.Is` matching | ✅ tested |
| Format validation (64 lowercase hex chars) before file I/O | ✅ tested |
| Error message names both expected + actual hashes (operator-diffable) | ✅ tested |
| `AGEZT_PLUGIN_PINS` env-spec parser (`prefix=hash,prefix=hash`) | ✅ tested |
| Malformed pin → hard daemon startup error (no silent skip) | ✅ tested |
| Unknown prefix in pin spec → startup warning (operator-typo case) | ✅ |
| Wired into daemon plugin loop with per-prefix hash lookup | ✅ |
| `agt plugin hash <path>` CLI helper to record the pin | ✅ |

## Why BLAKE3 hash pinning, not GPG signing

The 30-second version:

| Story | Pinning (M1.ff) | GPG signing |
|---|---|---|
| Operator records expected hash once via `agt plugin hash` | yes | n/a |
| Operator manages signing keys + trust roots | no | yes |
| Daemon rejects modified binary | yes | yes |
| Daemon rejects binary signed by attacker's key | n/a | yes |
| Plugin author has to do anything | no | sign every release |
| Threat model "did the binary change since I checked it" | answered | answered |
| Threat model "is this a binary I've never seen, signed by someone I trust" | not answered | answered |

The threat operators actually have is the first one: "I installed
plugin X yesterday and want a daemon-enforced check that it's the
same binary today." Pinning answers that without invoking
key-distribution infrastructure (whose key, how operators get it,
what does trust-on-first-use look like?). For a single-operator
deployment — which is every agezt deployment today — pinning is
strictly simpler and answers the actual question.

A future signing layer can sit on top of pinning (sign the pin
manifest itself) without removing this floor. Don't make the
operator pay PKI costs they didn't ask for.

## Why BLAKE3 (not SHA-256)

- **Already a project dep.** agezt uses BLAKE3 for the journal's
  hash chain (M0). Adding crypto/sha256 would still be free (it's
  stdlib), but BLAKE3 is already linked and benchmarked; one
  fewer code path to keep in sync.
- **Faster.** A 50 MB plugin binary hashes in <50 ms on commodity
  hardware. SHA-256 is ~3× slower; the difference is invisible on
  small plugins but adds up across a daemon with several pinned
  plugins at startup.
- **Streaming-friendly.** We `io.Copy` from the file into the
  hasher — no need to load the whole binary into memory. Stdlib's
  sha256 supports the same pattern, so this is a tie on
  ergonomics; BLAKE3 wins on the throughput point above.

## Files

### `kernel/plugin/pin.go` — new (~95 LoC)

- `VerifyPin(path, pin string) error` — happy path: hash the file,
  compare to pin (lowercase hex, 64 chars), return nil on match,
  wrapped `ErrPinMismatch` on drift.
- `HashFile(path string) (string, error)` — streaming BLAKE3-256
  digest of a file. Exported so the `agt plugin hash` CLI helper
  can reuse the same code the host enforces with — single source
  of truth for "the hash of plugin X."
- `looksLikeBLAKE3Pin(s string) bool` — cheap pre-flight format
  check before burning I/O on hashing.
- `ErrPinMismatch` — sentinel for `errors.Is` matching downstream
  (the daemon publishes a tailored stderr message; a future
  audit-event publisher can recognise the case without
  string-matching).
- `makeChild(path, args)` — trivial wrapper around
  `osexec.Command`. Centralising it makes future sandbox additions
  (cgroup wrapping, seccomp via prctl, etc.) one-touch.

### `kernel/plugin/pinspec.go` — new (~75 LoC)

- `PinSpec map[string]string` — prefix → pin.
- `ParsePinSpec(spec string)` — decodes the
  `prefix1=hash1,prefix2=hash2` env-var format. Hard error on
  bad format (operators want fast feedback on a security
  setting); tolerant of empty / whitespace-only spec.
- `PinSpec.UnusedPins(usedPrefixes []string)` — diff helper
  the daemon calls after spawning every plugin to warn about
  stale entries (typo or removed plugin).

### `kernel/plugin/host.go` — three lines added

```go
type Config struct {
    ...
    PinnedHash string  // BLAKE3-256 hex; "" = no verification
}

func Spawn(...) (*Plugin, error) {
    ...
    if cfg.PinnedHash != "" {
        if err := VerifyPin(cfg.Path, cfg.PinnedHash); err != nil {
            return nil, err
        }
    }
    cmd := makeChild(cfg.Path, cfg.Args)  // was osexec.Command(...)
    ...
}
```

The pin check is **before** any child process gets started. This
matters: even an aborted exec of an attacker-supplied binary is
itself a compromise (the binary's process gets file-descriptor
access, can write side-channel artifacts, etc.). The order is the
security-critical bit; the rest is plumbing.

### `kernel/plugin/pin_test.go` — new (~190 LoC, 10 tests)

| Test | Locks in |
|---|---|
| `TestVerifyPin_HashMatch` | Self-consistency: hash a file, pin matches, no error |
| `TestVerifyPin_HashMismatch` | `ErrPinMismatch` returned; error message includes both hashes for diff |
| `TestVerifyPin_RejectsMalformedPin` | Empty / non-hex / wrong-length pins rejected pre-I/O |
| `TestVerifyPin_FileMissing` | Missing file returns wrapped read error (not misclassified as drift) |
| `TestParsePinSpec_Basic` | Happy-path of `prefix=hash,prefix=hash` parsing |
| `TestParsePinSpec_RejectsBadFormat` | Missing '=', empty prefix, bad hash all hard error |
| `TestParsePinSpec_Empty` | Whitespace-only spec → empty map, no error |
| `TestPinSpec_UnusedPins` | Diff helper reports stale prefixes |
| `TestSpawn_RejectsMismatchedPin` | Integration: wrong pin → `ErrPinMismatch`, no child spawned |
| `TestSpawn_AcceptsCorrectPin` | Matching pin → Spawn completes normally, tools register |

### `cmd/agezt/main.go` — daemon-side wiring (~20 LoC)

The plugin boot loop now:

1. Parses `AGEZT_PLUGIN_PINS` upfront. Bad format → hard startup
   error (returns from `buildTools`, daemon refuses to start).
2. Looks up `pins[prefix]` per plugin Config — empty string when
   no pin is configured for that prefix (no verification = current
   behaviour, opt-in security).
3. After every plugin attempted, calls `pins.UnusedPins(...)` to
   warn about stale entries.

The flow is purely additive — operators who don't set
`AGEZT_PLUGIN_PINS` see no behaviour change.

### `cmd/agt/plugin.go` — new CLI surface (~65 LoC)

`agt plugin hash <path>` prints the BLAKE3-256 digest of the file
at `<path>`. Plain digest on stdout (no path prefix, no banner) so
operators can `$(...)`-substitute it directly:

```bash
AGEZT_PLUGIN_PINS="search=$(agt plugin hash /opt/agezt/search)"
```

`agt plugin -h` documents the subcommand and the full env-var
flow.

## Operator workflow examples

**First-time setup of a pinned plugin:**

```bash
# 1. Install the plugin
sudo install -m 0755 ./search /opt/agezt/

# 2. Record its hash
HASH=$(agt plugin hash /opt/agezt/search)
echo "search pin: $HASH"

# 3. Wire env vars (e.g. into your systemd EnvironmentFile)
echo 'AGEZT_PLUGINS="search=/opt/agezt/search"' >> /etc/agezt.env
echo "AGEZT_PLUGIN_PINS=\"search=$HASH\"" >> /etc/agezt.env

# 4. Start the daemon — pin verified.
systemctl restart agezt
```

**Plugin upgrade (intentional):**

```bash
# 1. Install the new version
sudo install -m 0755 ./search-v2 /opt/agezt/search

# 2. Start daemon → fails with "plugin pin mismatch" (this is the
#    safety net working — confirms the binary changed and forces
#    you to re-acknowledge).
systemctl restart agezt   # fails

# 3. Re-record the hash, update env, restart
sed -i "s|AGEZT_PLUGIN_PINS=.*|AGEZT_PLUGIN_PINS=\"search=$(agt plugin hash /opt/agezt/search)\"|" /etc/agezt.env
systemctl restart agezt   # succeeds
```

**Suspected compromise:**

```bash
# Daemon refuses to start; stderr says:
#   WARNING: plugin "search" (/opt/agezt/search) failed to start:
#   plugin: binary hash does not match pinned value: "/opt/agezt/search"
#     expected: ec64a4b2…
#     got:      9d3e7c91…
#
# Operator now investigates: who wrote that file, when, with what
# version? Crucially, the modified binary NEVER RAN — the check is
# before exec.
```

## Test summary

```
go test ./kernel/plugin/ -v -count=1 -run 'TestVerifyPin|TestParsePin|TestPinSpec|TestSpawn_.*Pin'
(10 tests — all PASS)

go test ./... -count=1
(35 packages — all PASS, no regressions)
```

+10 from M1.ff.

## What's intentionally NOT in M1.ff

- **GPG / Sigstore signature verification.** See "Why BLAKE3 hash
  pinning, not GPG signing" above. The pin floor doesn't preclude
  a future signing layer; it precedes it.
- **TOFU (trust-on-first-use) auto-pinning.** Tempting — daemon
  notes the hash on first launch and pins it automatically. But
  it changes the security story from "operator explicitly
  verified" to "daemon trusted whatever was there first," which
  silently weakens the model on a compromised install. Hard
  no on TOFU for security-sensitive code paths.
- **Per-plugin pin files** (`~/.agezt/plugin_pins.json`).
  Considered, rejected. One config surface (env var) is easier
  to audit than two; operators who want a file can source it from
  shell rc / systemd EnvironmentFile.
- **Pin rotation reminders.** Hashes don't rotate (a binary's
  hash is a property of the bits, not a key). The only "rotation"
  is "the binary changed" — which the pin already catches.
- **Sandboxing / seccomp / cgroups.** Separate concern, still on
  the M1.y deferral list. Pinning verifies the binary is what
  the operator expects; sandboxing limits what even an expected
  binary can do. Both are useful; M1.ff only does the first.
- **Per-plugin allow/deny tool list.** A spawned plugin can today
  register any tool name (under its prefix); a future "operator
  declares the expected tool set, daemon refuses extras" would
  complement the binary pin. Not in scope here.

## Files touched

- [kernel/plugin/pin.go](../kernel/plugin/pin.go) — new, VerifyPin / HashFile / ErrPinMismatch.
- [kernel/plugin/pinspec.go](../kernel/plugin/pinspec.go) — new, PinSpec + parser + UnusedPins diff.
- [kernel/plugin/pin_test.go](../kernel/plugin/pin_test.go) — new, 10 tests.
- [kernel/plugin/host.go](../kernel/plugin/host.go) — added `Config.PinnedHash`, pre-spawn verification gate.
- [cmd/agezt/main.go](../cmd/agezt/main.go) — env parsing + per-plugin pin lookup + unused-pin warning.
- [cmd/agt/main.go](../cmd/agt/main.go) — dispatcher entry for `agt plugin`.
- [cmd/agt/plugin.go](../cmd/agt/plugin.go) — new, `agt plugin hash` subcommand.

Zero changes to bus, journal, governor, agent loop, providers, or
any other package. Plugin signing sits cleanly on top of M1.y.

## Deferrals after M1.ff

Unchanged from M1.ee, minus the signing item just shipped:

- Pulse v3+ (TUI dashboard, until/last flags, replay rate limit,
  subject indexing).
- Planner v2 (re-planning, sub-planners, planner-side tools).
- Plugin sandboxing, hot-reload, streaming, callbacks (signing
  done; rest remain).
- Browser: JS rendering, screenshots, search, cookies.
- Non-Anthropic body shapes on Bedrock.
- Vault: OS-keychain integration, argon2.
- MCP bridge v2 (resources/sampling/progress/cancellation/SSE/image).
- Routing extensions (TaskRouteRequire, per-task-type budgets,
  per-task-type model overrides) — wait for demand.
- AWS extensions (SSO, assume-role, web identity, process creds,
  IMDSv1) — wait for demand.
- Plugin signing extensions: GPG, TOFU, sandboxing, per-plugin
  tool allowlist — wait for demand or threat-model evolution.
