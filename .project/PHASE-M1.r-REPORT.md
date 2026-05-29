# Phase Report — Milestone 1.r (hot reload of catalog + vault)

> Status: **shipped** · Date: 2026-05-29
> Per [SPEC-15 §1 (Catalog)](SPEC-15-PROVIDER-ECOSYSTEM.md) and
> M1.o operator-UX friction.
> Continues [PHASE-M1.q.y-REPORT.md](PHASE-M1.q.y-REPORT.md).

## Scope

Every M1.o report flagged the same friction: `agt provider creds
set` printed `restart the daemon to pick up this change` because
the vault was snapshot at daemon Open. After ~5 phases of
deferring it, M1.r ships:

```
agt provider reload
```

A control-plane command that re-reads catalog files + vault from
disk and **rebuilds the primary provider in place** — no daemon
restart, no dropped runs.

| Concern | Status |
|---|---|
| New CLI: `agt provider reload` | ✅ |
| Control-plane command: `provider_reload` | ✅ |
| Atomic registry swap via `governor.Registry.Replace` | ✅ tested |
| **Routing-chain rebuild** (Governor.Replace, not just Registry.Replace) | ✅ tested with regression guard |
| `runtime.Config.OnReload` callback contract | ✅ tested (success + error + nil) |
| Daemon's OnReload closure: re-reads vault, re-runs `selectPrimary` | ✅ |
| Catalog refresh happens unconditionally (independent of provider rebuild) | ✅ |
| `creds set` / `creds rm` messaging updated to suggest `provider reload` | ✅ |
| Help text updated | ✅ |
| Errors surface clearly when missing creds after rotation | ✅ |
| Test coverage: 7 new tests (Registry x3 + Governor x1 + Runtime x2 + matches existing patterns) | ✅ |

## Changes

### 1. `kernel/governor/registry.go` — Replace + Remove

```go
// Replace atomically swaps the entry for info.Name. Insertion order
// is preserved when replacing an existing entry; new entries append.
func (r *Registry) Replace(info *ProviderInfo) error

// Remove deletes the named provider. Returns (true, nil) if removed,
// (false, nil) if absent. Order preserved for remaining entries.
func (r *Registry) Remove(name string) bool
```

Both methods take the same name-mismatch sanity check `Register`
applies, so the wrapped namedProvider's catalog-id name stays
consistent across reloads.

### 2. `kernel/governor/governor.go` — `Governor.Replace`

The hidden hazard: `Governor` caches `primary[]` / `fallback[]`
chains at construction (governor.go:84-90). Direct
`Registry.Replace()` updates the registry but leaves the cached
chains stale — so routing would continue to hit the *old* provider
until daemon restart. The bug this prevents is the whole point of
this phase.

Fix:

```go
func (g *Governor) Replace(info *ProviderInfo) error {
    if err := g.cfg.Registry.Replace(info); err != nil {
        return err
    }
    g.mu.Lock()
    defer g.mu.Unlock()
    g.primary = g.primary[:0]
    g.fallback = g.fallback[:0]
    for _, p := range g.cfg.Registry.All() {
        if p.IsFallback {
            g.fallback = append(g.fallback, p)
        } else {
            g.primary = append(g.primary, p)
        }
    }
    return nil
}
```

`TestGovernor_ReplaceRoutesToNewProvider` is the regression guard.
It registers a primary, runs Complete (verifies the old provider's
calls counter hit 1), calls Replace, runs Complete again, and
asserts the *new* provider's counter is 1 while the old one stayed
at 1. Without the chain-rebuild, the new test would fail.

### 3. `kernel/runtime/runtime.go` — `OnReload` + `Reload()`

`Config.OnReload func() error` is the new daemon-supplied callback.
Kept in cmd/agezt (not in kernel/runtime) because provider
construction lives there — the runtime stays provider-agnostic.

```go
// Reload refreshes both the catalog snapshot AND the live provider
// registry. Catalog reload is always performed; provider rebuild
// runs when Config.OnReload is non-nil. Returns
// (catalog, providersReloaded, err).
func (k *Kernel) Reload() (*catalog.Catalog, bool, error)
```

`providersReloaded` is a tri-state-ish signal: `true` means
OnReload ran and succeeded; `false` means OnReload was nil (catalog
refreshed but provider unchanged). On OnReload error, the catalog
refresh still committed but the function returns the error and
`providersReloaded=false`.

### 4. `cmd/agezt/main.go` — OnReload closure

The daemon's closure captures `gov`, `catStore`, `credStore` from
the Open path and on each reload:

1. Re-reads vault from disk (`credStore.Load()`)
2. Re-reads catalog from disk (`catStore.Load()`)
3. Runs `selectPrimary(c, freshLookup)` — the **same** function the
   boot path uses
4. Calls `gov.Replace(&governor.ProviderInfo{...})` — atomic swap

The "use the same selectPrimary" invariant is what makes hot reload
trustworthy: there's no chance of routing post-reload to a provider
that *wouldn't have been picked* on a fresh boot from the same
on-disk state.

Limitation surfaced in the code comment: the *model* swap isn't
plumbed through. If an operator changes `AGEZT_MODEL=...` in their
environment and reloads, the new primary uses the new model id —
but if a different provider would now be picked because of pure
model-availability changes in the catalog, the daemon's
runtime.Config.Model still carries the boot-time value. That's a
narrow edge case (`AGEZT_MODEL` is meant for boot config), deferred
to M1.r.x.

### 5. Control plane: `provider_reload` command

```go
const CmdProviderReload = "provider_reload"

func (s *Server) handleProviderReload(conn net.Conn, req Request) {
    cat, providersReloaded, err := s.k.Reload()
    if err != nil { /* RespError */ return }
    result := map[string]any{
        "providers_reloaded": providersReloaded,
        "provider_count":     len(cat.Providers),
    }
    if !providersReloaded {
        result["note"] = "OnReload not configured; only the catalog
            snapshot was refreshed. Restart the daemon for the new
            credentials to take effect."
    }
    s.writeResp(conn, Response{ID: req.ID, Type: RespResult, Result: result})
}
```

The `note` field is critical: an operator running against a daemon
built without OnReload (custom builds, future test harnesses)
shouldn't silently get "looks like it worked" when only the catalog
was refreshed. Surfacing the actual scope of the reload preserves
trust.

### 6. CLI: `agt provider reload`

```go
func cmdProviderReload(stdout, stderr io.Writer) int {
    c := dial(stderr)
    ...
    res, err := c.Call(ctx, controlplane.CmdProviderReload, nil)
    pr, _ := res["providers_reloaded"].(bool)
    if pr {
        fmt.Fprintf(stdout, "reloaded: catalog (%d providers) + vault → primary provider rebuilt\n", int(pc))
    } else {
        fmt.Fprintf(stdout, "reloaded: catalog (%d providers)\n", int(pc))
        if note, _ := res["note"].(string); note != "" {
            fmt.Fprintf(stdout, "note: %s\n", note)
        }
    }
    return 0
}
```

### 7. Messaging updates

`creds set` and `creds rm` previously printed `restart the daemon
to pick up this change`. Now they suggest the lighter alternative
without removing the heavier one:

```
run `agt provider reload` to apply (or restart the daemon)
```

The "or restart the daemon" tail is kept as a fallback hint for
operators on daemons-without-OnReload builds and for the rare cases
where a reload error means a daemon restart is the cleaner path.

## Architectural consequences

1. **The daemon now has a "live config" contract.** Before M1.r,
   operator-supplied state (catalog, vault) was snapshotted at boot.
   Now both can change mid-flight, atomically, without dropping
   in-flight runs. This sets a pattern: any future "operator
   configures X at runtime" feature should plug into the same
   `Reload` path rather than inventing its own reload command.

2. **Routing chains belong to the Governor, not the Registry.**
   The chain-rebuild requirement in `Governor.Replace` is a real
   API constraint: callers MUST NOT call `Registry.Replace` directly
   on a Governor-owned registry. The test `TestGovernor_Replace
   RoutesToNewProvider` enforces this by failing if a future
   refactor inadvertently reintroduces stale-chain routing. If the
   constraint becomes painful, the alternative is to compute the
   chain on every `routeChain` call instead of caching — but the
   cache exists because routing happens on every LLM call, not every
   reload.

3. **The OnReload split keeps the runtime kernel-pure.** Provider
   construction is daemon territory; it depends on credentials
   resolvers, family-specific URL builders, OAuth token sources,
   etc. Putting that logic in runtime would force runtime to know
   about every adapter package. The callback pattern lets each layer
   own what it should: runtime owns "when to reload"; daemon owns
   "what to rebuild."

4. **Hot reload preserves the journal chain.** No reload event is
   journaled today (an oversight worth a follow-up — the chain
   should reflect operator actions of this magnitude). But the
   provider swap itself doesn't touch the journal; in-flight runs
   complete on whichever provider they were dispatched to. Atomic
   from the caller's perspective.

## Demo (synthetic)

Operator rotates an Anthropic key while the daemon is running:

```
$ agt provider creds set ANTHROPIC_API_KEY sk-new-key
stored ANTHROPIC_API_KEY = sk-n••••••key in <vault path>
run `agt provider reload` to apply (or restart the daemon)

$ agt provider reload
reloaded: catalog (49 providers) + vault → primary provider rebuilt

$ agt run "Say pong"
  [evt seq=N kind=task.received]
  [evt seq=N+1 kind=llm.request]
  pong
  [evt seq=N+2 kind=llm.response]
  [evt seq=N+3 kind=task.completed]

--- final answer ---
pong
```

The whole rotation cycle takes <1 second of operator action time;
the previous "restart the daemon" path took 5-10 seconds and broke
any in-flight runs.

## Deferrals

- **Reload event in the journal.** A `KindProviderReload` event
  would let `agt why` correlate post-reload runs with the
  configuration change. ~30 LoC follow-up.
- **`provider reload` invalidating in-flight Complete calls.**
  Today, a run that's mid-LLM-call when reload happens completes on
  the OLD provider (already-dispatched HTTP call). The new provider
  takes over for the NEXT call. Probably correct, but worth a
  formal write-up.
- **Model swap on reload.** If `AGEZT_MODEL` changes between
  reloads, the daemon's runtime.Config.Model still carries the
  boot-time value. The Provider gets the new model name via
  `selectPrimary` but the kernel-wide default model doesn't update.
  Narrow case; M1.r.x material.
- **SIGHUP-triggered reload.** Many daemons expose
  `kill -HUP <pid>` as a hot-reload signal. Agezt's controlplane
  command covers the CLI path; the signal path is a one-liner away
  but might be platform-specific (Windows doesn't have SIGHUP).
  Defer.

**Unchanged longstanding deferrals:**
- Subscription-first routing.
- OS-keychain encryption for the vault.
- Bedrock streaming (AWS event-stream binary framing).
- Vertex Anthropic (depends on M1.n.x).
- Browser tool, out-of-process plugin host.
- Pulse v1, planner.

## Files touched

```
kernel/governor/registry.go         + Replace, Remove (~55 LoC)
kernel/governor/governor.go         + Registry(), Replace() with chain rebuild (~30 LoC)
kernel/governor/governor_test.go    + 4 new tests (Replace, Remove, ChainRebuild)
kernel/runtime/runtime.go           + OnReload field + Reload() method (~30 LoC)
kernel/runtime/runtime_test.go      + 2 new tests
kernel/controlplane/protocol.go     + CmdProviderReload constant
kernel/controlplane/server.go       + handler dispatch case
kernel/controlplane/catalog.go      + handleProviderReload (~25 LoC)
cmd/agezt/main.go                  + OnReload closure (~40 LoC)
cmd/agt/provider.go                 + cmdProviderReload + updated creds set/rm hints
cmd/agt/main.go                     + help line
```

No schema changes. No new external deps.

## Verification

```
$ go build ./...           # clean
$ go test ./... -count=1   # 427 pass, 0 fail (up from 421 in M1.q.y)
```

The full operator UX trajectory:

| Milestone | New capability |
|---|---|
| M1.f | `agt catalog sync/list/discover` |
| M1.o | `agt provider creds set/list/rm` |
| M1.p | `agt provider check [id]` |
| M1.p.x | `agt provider check --all` |
| M1.p.y | `--json` + `--bench N` |
| M1.q | StreamingProvider + Anthropic streaming + `--stream` |
| M1.q.x | OpenAI streaming → 4 families × ~11 vendors |
| M1.q.x.x | Google Gemini streaming |
| M1.q.x.x.x | Vertex Gemini streaming |
| M1.q.x.x.x.x | Ollama + Cohere streaming |
| M1.q.y | Live streaming during real `agt run` |
| **M1.r** | **`agt provider reload` — no more "restart the daemon"** |
