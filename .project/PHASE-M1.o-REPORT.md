# Phase Report — Milestone 1.o (Credentials vault; `agt provider creds`)

> Status: **shipped** · Date: 2026-05-29
> Per [SPEC-15 §3 (Operator UX for catalog-driven providers)](SPEC-15-PROVIDER-ECOSYSTEM.md),
> [TASKS P1-CONDUIT-13](TASKS.md). First non-catalog-pivot phase;
> continues [PHASE-M1.n-REPORT.md](PHASE-M1.n-REPORT.md).

## Scope

The catalog pivot (M1.f–M1.n) made every *provider* a catalog refresh
away. M1.o closes the matching operator-UX gap: a **credentials vault**
so the same operator who maintains 10+ provider keys doesn't have to
stuff them all into a shell rc file.

Three small pieces:

1. **`kernel/creds` package** — file-backed vault at
   `<BaseDir>/creds.json` (0600 perms, atomic writes, masked-display
   helper).
2. **Daemon integration** — `creds.ChainLookup(vault.Lookup, os.Getenv)`
   threaded through `buildGovernor` → `selectPrimary` →
   `buildFromCatalog` → `compat.Build`. Vault wins, env falls through;
   `HasCredentials` also uses the chain so auto-pick is vault-aware.
3. **`agt provider creds` CLI** — `set` (with `NAME=VALUE`,
   two-arg, and interactive-prompt forms), `list` (grouped by
   catalog provider when a catalog is synced), `rm`.

**No new external dependencies.** Plain JSON file; encryption defers
to M1.o.x (OS keychain integration). Hot reload defers to a future
SIGHUP cycle — today the daemon snapshots the vault on `Open` and a
restart picks up changes.

| Concern | M1.o status |
|---|---|
| Operators can manage credentials without env-var sprawl | ✅ `agt provider creds set/list/rm` |
| Vault precedes env in lookup chain (vault wins) | ✅ `creds.ChainLookup` |
| Env fall-through preserved (so `export FOO=...` overrides vault for a session) | ❌ inverted on purpose — vault wins |
| Vault-driven auto-pick (no env vars needed) | ✅ verified in demo banner |
| Masked display (4-char prefix + dots + 4-char suffix) | ✅ `creds.MaskValue` |
| Catalog-aware grouping (`agt provider creds list` shows by provider) | ✅ |
| Atomic writes (no half-written vault file on crash) | ✅ write-tmp-then-rename + chmod 0600 reapply |
| Daemon banner surfaces vault state | ✅ new `credentials` line |
| At-rest encryption / OS keychain | ⏳ M1.o.x |
| Hot reload | ⏳ separate phase |

**On the chain order (vault wins):** I considered env→vault (so
`export` overrides), but chose vault→env because the more common
operator pain is "I set this in the vault and a stray env var
shadowed it without warning." Vault-as-source-of-truth + env-as-
fallback gives clearer semantics. A future `agt provider creds peek
<NAME>` could explicitly show which source resolved.

## Changes

### 1. `kernel/creds` package

**`Store`** — concurrent-safe, mutex-guarded map persisted to JSON:

```go
func NewStore(baseDir string) *Store
func (s *Store) Load() error           // missing file = empty vault (no error)
func (s *Store) Save() error           // atomic; chmod 0600 reapplied
func (s *Store) Set(name, value) error // empty value removes (shell-style unset)
func (s *Store) Get(name) string
func (s *Store) Has(name) bool
func (s *Store) Remove(name) bool      // true if existed
func (s *Store) Names() []string       // sorted
func (s *Store) Lookup(name) string    // = Get; CredLookup-compatible
```

**`ChainLookup(sources ...func(string) string) func(string) string`** —
composes multiple lookups; first non-empty wins; nil sources skipped.
This is the *only* exported coupling between `kernel/creds` and the
rest of the codebase; the daemon imports it once.

**`MaskValue(v) string`** — `sk-a••••••7890`-style redaction for
display. ≤8 chars are fully masked.

13 tests in `kernel/creds/creds_test.go`:
- missing/empty/malformed file handling
- set-save-load roundtrip
- 0600 permissions (skipped on Windows)
- empty-value-removes (shell `unset` semantics)
- name validation (empty/whitespace rejected)
- atomic write (no `.tmp` leftover on success)
- chain lookup precedence + nil-source skipping
- mask-value table (5 cases)

### 2. Daemon integration

`cmd/agezt/main.go` `runDaemon`:

```go
credStore := creds.NewStore(baseDir)
if err := credStore.Load(); err != nil { return 1 }
credLookup := creds.ChainLookup(credStore.Lookup, os.Getenv)
credDesc := fmt.Sprintf("vault entries=%d at %s (env vars fall through)",
    len(credStore.Names()), credStore.Path)

gov, govDesc, model, err := buildGovernor(cat, credLookup)
```

Signature changes:

```go
buildGovernor(cat, lookup)
selectPrimary(cat, lookup)
buildFromCatalog(entry, modelOverride, lookup)
```

The `lookup` parameter replaces direct `os.Getenv` references in:
- `entry.HasCredentials(lookup)` in the auto-pick loop
- `compat.Build(entry, modelID, lookup)` in `buildFromCatalog`

`AGEZT_PROVIDER` / `AGEZT_MODEL` are still read from env directly
— they're config knobs, not credentials, and a one-off shell
override (`AGEZT_PROVIDER=mistral agezt`) shouldn't need a vault
roundtrip.

New banner line:

```
credentials      : vault entries=2 at <path> (env vars fall through)
```

### 3. `agt provider creds` CLI

New file `cmd/agt/provider.go`. Three subcommands:

**`set <NAME> [<value>]`** — three input forms:
- `agt provider creds set ANTHROPIC_API_KEY=sk-...` (NAME=VALUE)
- `agt provider creds set ANTHROPIC_API_KEY sk-...` (two args; value can have spaces)
- `agt provider creds set ANTHROPIC_API_KEY` (prompts on stdin, no shell-history leak)

**`list`** — grouped by catalog provider when a catalog is synced:

```
2 vault entries at C:\Users\ersin\AppData\Local\Temp\agezt-m1o-demo\creds.json

  anthropic
    ANTHROPIC_API_KEY                        = sk-a••••••7890
  openai
    OPENAI_API_KEY                           = sk-o••••••rter
```

Vault entries whose names aren't in any catalog provider's `env`
list show under `(other)`. Catalog grouping is best-effort — works
without a daemon, falls back to a flat list when the catalog is
empty.

**`rm <NAME> [<NAME>...]`** — removes one or more entries.
Aliases: `remove`, `del`, `delete`, `unset`.

Both `set` and `rm` print a reminder:

```
restart the daemon to pick up this change
(vault is snapshot on daemon Open)
```

The reminder matches the existing `agt catalog sync` UX — same
"snapshot at startup, restart to refresh" model the operator
already knows.

## Demo transcript

Fresh home, no daemon running yet.

### Step 1 — empty vault

```
$ AGEZT_HOME=/tmp/agezt-m1o-demo agt provider creds list
vault is empty (C:\Users\ersin\AppData\Local\Temp\agezt-m1o-demo\creds.json)
use `agt provider creds set <NAME> <value>` to add a credential
```

### Step 2 — set two credentials, both input forms

```
$ agt provider creds set ANTHROPIC_API_KEY=sk-ant-secret-1234567890
stored ANTHROPIC_API_KEY = sk-a••••••7890 in C:\Users\ersin\AppData\Local\Temp\agezt-m1o-demo\creds.json
restart the daemon to pick up this change (vault is snapshot on daemon Open)

$ agt provider creds set OPENAI_API_KEY sk-openai-shorter
stored OPENAI_API_KEY = sk-o••••••rter in C:\Users\ersin\AppData\Local\Temp\agezt-m1o-demo\creds.json
restart the daemon to pick up this change (vault is snapshot on daemon Open)
```

### Step 3 — list with catalog grouping

```
$ agt provider creds list
2 vault entries at C:\Users\ersin\AppData\Local\Temp\agezt-m1o-demo\creds.json

  anthropic
    ANTHROPIC_API_KEY                        = sk-a••••••7890
  openai
    OPENAI_API_KEY                           = sk-o••••••rter
```

### Step 4 — daemon picks up vault creds (zero env vars)

```
$ AGEZT_PROVIDER=anthropic AGEZT_MODEL=claude-opus-4-7 agezt
Agezt 0.0.0-m0 — daemon ready (protocol v1)
  governor         : primary=anthropic(catalog; family=anthropic,
                     model=claude-opus-4-7) → fallback=mock(offline),
                     daily_ceiling=$20.00
  credentials      : vault entries=2 at <path> (env vars fall through)
  ...
```

The `ANTHROPIC_API_KEY` env var was **not set** in this shell — the
daemon read it from the vault.

### Step 5 — auto-pick is vault-aware

With `AGEZT_PROVIDER` unset:

```
$ agezt
  governor         : primary=anthropic(catalog; family=anthropic,
                     model=claude-3-5-haiku-20241022)
                     → fallback=mock(offline), daily_ceiling=$20.00
  credentials      : vault entries=1 at <path> (env vars fall through)
```

`HasCredentials` walked the catalog and matched Anthropic via the
vault — auto-pick now respects vaulted credentials as first-class
config.

### Step 6 — rm

```
$ agt provider creds rm OPENAI_API_KEY
removed OPENAI_API_KEY
restart the daemon to pick up this change

$ agt provider creds list
1 vault entry at <path>

  anthropic
    ANTHROPIC_API_KEY                        = sk-a••••••7890
```

## Architectural consequences

1. **Operator workflow is now end-to-end vault-driven.**
   - Sync providers: `agt catalog sync` → catalog updates
   - Configure credentials: `agt provider creds set FOO sk-...` → vault updates
   - Run: `AGEZT_PROVIDER=<id> agt run "..."` → daemon reads vault + catalog
   - **No shell rc file edits required for normal day-to-day use.**

2. **`CredLookup` is now a real abstraction.** Pre-M1.o it was just
   `os.Getenv` at every call site. M1.o promoted it to a chainable
   abstraction that future sources (OS keychain, HashiCorp Vault,
   `gcloud secrets`, 1Password CLI) plug into via the same
   `ChainLookup(...)` pattern. Adding a source is one function
   matching `func(string) string`.

3. **The daemon banner is now a real diagnostic.** Pre-M1.o it told
   operators which Governor primary they got. Post-M1.o it also
   tells them where their credentials came from. The `credentials`
   line is the third banner section operators check (after governor
   primary and tools).

4. **`agt provider` is a real subcommand tree.** Adding sibling
   verbs in future phases is mechanical:
   - `agt provider check <id>` — live roundtrip + latency + cost
   - `agt provider list` — installed providers with cred status
   - `agt provider refresh` — re-sync just one provider's models

5. **Vault wins on purpose.** Inverting the chain (env wins) would
   match `$EDITOR`-style precedence but creates a subtle failure mode
   where a stray `ANTHROPIC_API_KEY=` from a sourced script
   *silently* preempts the vault. Vault-wins makes the vault the
   single source of truth; env is the explicit-override knob. The
   tradeoff is "to override a vaulted value, `rm` and re-set
   transiently OR explicitly unvault for the session" — acceptable.

## Deferrals → M1.o.x and beyond

**M1.o.x — vault hardening:**
- At-rest encryption via OS keychain (macOS Keychain, Windows DPAPI,
  Linux libsecret / pass).
- SIGHUP-driven hot reload so `agt provider creds set ...` takes
  effect without restarting the daemon.
- `agt provider creds peek <NAME>` showing which source (vault /
  env / OS keychain when added) resolved a credential.
- Optional vault sync from external secret stores (HashiCorp Vault,
  AWS Secrets Manager, 1Password CLI).

**Unrelated deferrals from prior milestones** (unchanged):
- Streaming (SSE) uniformly across the 7 wire adapters.
- Subscription-first routing (DECISIONS C2).
- Browser tool, out-of-process plugin host.
- Pulse v1 (observers, salience, initiative).
- LLM-driven planner (intent → DAG).
- Bedrock SigV4 + non-Anthropic body shapes (M1.m.x).
- Vertex Anthropic + ADC + workload-identity (M1.n.x).

## Files touched

```
kernel/creds/creds.go                NEW (~180 LoC)
kernel/creds/creds_test.go           NEW (13 tests)
cmd/agezt/main.go                   (+ creds import + vault load
                                        + ChainLookup + credDesc banner line
                                        + buildGovernor/selectPrimary/
                                          buildFromCatalog gain `lookup` arg)
cmd/agt/main.go                      (+ "provider" dispatch case
                                        + help text additions)
cmd/agt/provider.go                  NEW (~210 LoC: cmdProvider +
                                        cmdProviderCreds + list/set/rm)
```

No schema changes. No new external deps. No daemon-command changes
in the control plane (agt mutates the vault directly).

## Verification

```
$ go build ./...           # clean
$ go test ./... -count=1   # 306 pass, 0 fail (up from 293 in M1.n)
```

Cumulative test growth this and prior pivot phases:

| Milestone | Tests |
|---|---:|
| M1.n (catalog pivot closed) | 293 |
| **M1.o (this phase)** | **306** |

New tests this phase: 13 in `kernel/creds`.
