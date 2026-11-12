# W3.76aj — the run's tool fold moves into the journal

**Slice:** W3.76aj (the forty-fourth of the runtime's dissolution)
**Base:** `4e9421e3` (the W3.76ai merge)
**Date:** 2026-11-12

---

## 1. The family

A run's **transcript** is built by folding the event log over one correlation:
how many tools the run called, and which ones, in order.

```go
count, names := j.FoldTools(corr)
```

That fold is a **walk over the event log** — the same family as the provenance
walks the journal already owns (`Why`, `Causes`, `ParentOf`, `Verify`). The event
log is the host's own file (`kernel/journal`), and the journal answers walks
over it.

What stayed in the daemon was a **second spelling** of that walk:

```go
// kernel/runtime/runner_delegates.go
func (k *Kernel) FoldRunTools(corr string) (int, []string) {
	// a copy of the journal's fold, over the daemon's own journal handle
}
```

and the **run engine**, which lives *inside* the runs module, asked the
**host** for it through its `KernelAPI` port:

```go
toolCount, names := r.k.FoldRunTools(corr)
```

**One family, two spellings, and a hop the host never had to keep.** The
W3.76ad slice had already taken the four provenance walks the same way — the
engine re-exported them, the daemon copied them, and the journal's own methods
were the only real bodies. The fold was the last verb in that row.

## 2. What moved

The fold's **body** moved into the journal (`kernel/journal/fold.go`), next to
the provenance walks it belongs with:

```go
func (j *Journal) FoldTools(corr string) (int, []string)
```

The engine asks the log through the journal the port **already** carried
(`Journal() *journal.Journal`), so the fold costs no new port entry — the port
loses **one**:

```go
toolCount, names := r.k.Journal().FoldTools(corr)
```

The daemon keeps no verb, no copy of the body, and no name for the journal's
fold: its delegate file (`runner_delegates.go`) held nothing else and is
**gone**. The daemon's package doc says where the walks live — the note moved
there with the file.

The **coverage moved with the subject**: the daemon's own test
(`foldruntools_internal_test.go`, which pinned the cross-run isolation — a
`||`→`&&` regression would have folded another run's tool results into the
transcript) moved to the journal's own test file, next to the walk it pins,
together with two new cases (a tool call whose payload carries no name still
**counts**; an unused correlation folds to nothing).

## 3. The port

The run engine's host port lost **one** entry — `FoldRunTools` — and carries
**23** now (it carried **24**). What stayed is what the **host** owns.

## 4. The measurement

| what | before | after |
|---|---|---|
| the daemon's public verbs | 108 | **107** |
| the daemon's production lines | 5,246 | **5,213** |
| the daemon's production files | 53 | **52** |
| the app packages that import `kernel/runtime` | 40 | **40** |
| the run engine's host port (`KernelAPI`) | 24 entries | **23 entries** |
| the run engine's package | 1,040 | **1,046** |
| the journal package | 8 files / 1,070 lines | **9 files / 1,117 lines** |

**One verb off the daemon's surface and 33 lines with it**, and a file gone
with them. The journal package grew by one file and 47 lines — the fold and
its note. The run engine's package grew by six lines (the two calls and the
comments that say where they go).

## 5. The ratchets — and the probe

**The daemon's side** (`kernel/runtime/fold_ratchet_test.go`, new): the daemon
holds **no** verb for the fold, **no copy** of the fold's body (the shape
`CorrelationID != corr || e.Kind != event.KindToolResult` — a copy under
another name would still be a second spelling) and does not **call** the
journal's fold either (the check reads the daemon's *code*, with the comments
stripped: the daemon's own doc may name the fold, but no daemon code may call
it).

**The module's side** (`kernel/modules/runs/runner/runner_fold_ratchet_test.go`,
new): the port carries **no** fold entry, the engine **asks the journal** for
its fold, and the engine keeps no copy of the walk either.

The daemon's own surface ceiling followed the floor again: **108 → 107** (220
→ 201 → 194 → 179 → 173 → 166 → 164 → 156 → 152 → 148 → 143 → 135 → 129 →
125 → 121 → 118 → 116 → 114 → 111 → 108 → 107), and the retired name joined
the retired-verb list.

**The probe** put the world back before this slice — the daemon's verb with its
**real** body, the entry back into the engine's port and the engine asking the
host again: the retired-verb ratchet, the surface ceiling (108 > 107), the
daemon's two fold ratchets and the module's two fold ratchets all went red at
once — **six** ratchets in one shot, and nothing else broke (the journal's own
tests stayed green, because the fold they pin is the fold the probe put back).
The probe was reverted and the suites went green again.

## 6. The guard

The daemon's surface is pinned by its own retired-verb list and its ceiling,
and the port's shape by the run engine's own ratchets; no control-plane family
changed by name. One earlier ratchet was **re-aimed**: the W3.76ad ratchet
read the daemon's delegate file for the note that says where the provenance
walks live, and that file is gone — the note now lives in the daemon's package
doc (`doc.go`), and the ratchet reads it there. The invariant is unchanged: the
daemon still says where the walks live.

## 7. Verification

- the compiler (`go build ./...`, `go vet ./...`) and `gofmt`;
- the daemon's tests, the run engine's tests and the journal's tests;
- the **full test suite** and **the thirteen gates** (`TOTAL FAILURES: 0`).

## 8. What is left

The daemon holds **107** public verbs and **5,213** production lines; **forty**
application packages still name it; the run engine's host port carries **23**
entries.

What is left in the daemon is the run's **remaining machinery**: the plan
runner's registration (`RunPlan`) and the two doors the run engine still opens
for the host's own state (`ResolveRunModel`, `BuildLoopConfig` — the latter
already the daemon's door over the module's rule since W3.76g).

The **standing package** itself still lives at `kernel/standing`, and its
eventual home is the **triggers module** (`kernel/modules/triggers`) — the
architecture gate already places both `kernel/cadence` and `kernel/standing`
in that module, so the family's boundary is drawn; the module's directory does
not exist yet, and the move is a path move (four production importers), not a
lifecycle one.
