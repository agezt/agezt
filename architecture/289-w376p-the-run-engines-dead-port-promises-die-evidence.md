# W3.76p — the run engine's dead port promises die

**Slice:** W3.76p (the twenty-fourth of the runtime's dissolution)
**Base:** `8caa91d2` → the W3.76o merge
**Date:** 2026-11-02

---

## 1. The family

The run engine's port (`runner.KernelAPI`) is the **promise the host makes to
the run engine**. Every entry in it is a door the host opens on the engine's
behalf — and a promise nobody keeps is a promise nobody made.

W3.76k retired the daemon's dead **verbs**; this slice retires the run
engine's dead **port entries**. They are the same smell seen from the other
side, and the W3.76k lesson applies directly: a scan that says "nobody calls
this" is a **candidate**, and the compiler is the judge.

## 2. The measurement

Every one of the port's entries was counted by the callers in the runs module
(`kernel/modules/runs/**`, the port's own declaration excluded). **Thirteen**
entries had **zero** callers:

| the port entry | what it is |
|---|---|
| `Approvals` | the platform's approval registry |
| `ArtifactIndex` | the artifacts module's host index |
| `CatalogStore` | the providers module's catalog store |
| `ConfigCenter` | the platform's configuration centre |
| `DataLake` | the artifacts module's lake |
| `Edict` | the platform's decision engine |
| `Reflect` | the platform's reflection engine |
| `RunAssured` | the engine's own entry point, promised by the host |
| `Schedules` | the triggers module's host store |
| `Standing` | the triggers module's store |
| `StandingList` | the triggers module's rows |
| `Voice` | the media configuration |
| `Warden` | the platform's sandbox warden |

**The daemon keeps all thirteen names.** They are not dead on the daemon —
the application layer, `cmd/agezt` and the plugins call them (thirty-seven
callers for `Schedules`, twenty-six for `Standing`, twenty-three for `Voice`).
What was dead was the **promise**: the run engine never opened them, so the
port never had to carry them.

That is the difference the slice turns on: **a port entry and a public verb
are two different things.** The port entry is a promise to *this* module; the
public verb is the host's face to everyone else. The thirteen verbs stay; the
thirteen promises go.

`RunAssured` deserves its own word: the port listed the engine's **own** entry
point as something the host would provide — a leftover of the "Day 22 entry
points" comment, from before the engine's body moved into the module. The
engine's `RunWith` and `RunWithRetry` stay in the port because the engine
still delegates the child's run back to the host (the documented "Day 23 gap").

## 3. The measurement

| measure | before | after |
|---|---:|---:|
| the run engine's port (`runner.KernelAPI`) | 60 | **47** |
| the daemon's public verbs | 203 | **203** |
| the daemon's production lines (non-blank) | 6,162 | **6,162** |
| the runner package's production lines (non-blank) | 931 | **908** |

Nothing moved and nothing changed: the port simply stopped promising what
nobody asked for. The daemon's surface is untouched — the thirteen verbs are
still there for their real callers.

## 4. The ratchet — and the probe

The run engine's port-shape ratchet now pins the thirteen names' **absence**
(next to the two run-context names and the four roster names it already
pinned). A port entry that comes back fails the ratchet.

## 5. Verification

- **the compiler**: `go build ./...` and `go vet ./...` clean;
- **the port-shape ratchet** and its probe;
- **the full test suite** and **the thirteen gates**.

**No mutation run.** The slice deletes declarations that nothing called; there
is no rule whose reversal a mutation could expose. The evidence is the
compiler, the ratchet and its probe, and the full suite.

## 6. What is left

The run engine's port carries **47** entries. The W3 exit criterion — *no type
has more than 60 methods* — is now met by the port, and the daemon's own
`*Kernel` (203 public verbs) is the type that still breaks it.

Evidence: `architecture/289-w376p-the-run-engines-dead-port-promises-die-evidence.md`
