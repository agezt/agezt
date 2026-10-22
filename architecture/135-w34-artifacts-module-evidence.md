# W3.4 the artifacts module

The artifact store and the data lake were a module without a home — the layer
map named `kernel/artifact` and `kernel/datalake` as layer 3 in the module
`artifacts`, but both packages sat at the top of `kernel/`, outside the
`kernel/modules/` tree W3 establishes as the target home of the bounded
contexts.

The roadmap's W3 step 4 is `modules/artifacts` (artifact + datalake; GC
respects journal `raw_ref` via eventlog index). This slice does both, and the
second half turned out to be a **data-loss defect** the codemap had already
written down.

## What moved

| the package | before | after |
|---|---|---|
| the blob store + index | `kernel/artifact` | `kernel/modules/artifacts/artifact` |
| the Personal Data Lake | `kernel/datalake` | `kernel/modules/artifacts/datalake` |

The **package names do not change** (`artifact`, `datalake`), so the move is an
import-path change and nothing else: 44 Go files were repointed, and no call
site, type name or behaviour moved with them.

The placement is now *derived* rather than declared: the layer map's
`kernel/modules/*/...` rule (layer 3, module `@1`) places both packages in the
module `artifacts`, exactly where the two hand-written rules used to place
them. Those rules are gone.

The allowlist moved with the edges: thirteen entries changed their path (the
same thirteen violations, relocated). The ratchet therefore stays at **128
import violations and 13 call sites**, and the package count stays at 273.

## The collector deleted bytes the journal still named

A tool output too large to inline is offloaded into the content-addressed blob
store, and the journaled `tool.result` carries the blob's `raw_ref` instead of
the bytes (SPEC-04 §3.6). The indexer then records the blob in the artifact
index, so the operator can browse it.

The collector (M845, "reap stale artifacts, with confirm") removed the entry
**and** the blob when no other index entry referenced the same content. It
never looked at the journal.

So the sequence was: a run produces a 50 MB output → the journal names the
blob → 31 days later the operator collects the stale artifacts → the blob is
deleted → the journal's `raw_ref` points at nothing → `agt artifact_get <ref>`
fails and the run's output is gone, **while the journal still claims the bytes
are there**. The codemap already said so, in the artifact package's own
"Gotchas": *"the journal can hold dangling refs after a collect"*.

The proof, with the guard unwired:

```
collected 1 entries, want 0 (the journal still names the blob)
the entry was forgotten even though the journal still names its bytes
the journal's raw_ref points at nothing: artifact: not found
```

## The fix: the collector asks the journal

The module cannot read the journal itself — a module sees another module's
api package and nothing else (P5) — so it **asks**. The artifact index takes a
port:

```go
// kernel/modules/artifacts/artifact/index.go
type RefGuard interface {
    ReferencedRefs(candidates []string) map[string]bool
}

func (i *Index) SetRefGuard(g RefGuard)
```

The guard is asked **once per operation, with the whole candidate batch**, so
the host can answer from a single pass over its event log instead of one pass
per candidate. The host fills it in:

```go
// kernel/runtime/artifact_refs.go
type journalRefGuard struct{ j *journal.Journal }
```

It scans the journal for `tool.result` events and reports the refs they name.
The scan stops as soon as every candidate is accounted for.

What the module does with the answer:

- **`StaleEntries`** — the collector's candidates — are the age-based stale
  entries **minus** the ones the journal still names. A journal-named artifact
  is not a candidate at all, so the dry-run and the reap agree: neither
  promises bytes it will not free.
- **`Collect`** collects what is left, through the internal path that does not
  re-ask the guard per entry.
- **`Delete`** — the explicit delete — is **refused** with the named error
  `ErrStillReferenced` when the journal still names the blob. The application
  layer maps it to a sentence the operator can act on: *"artifact is still
  referenced by the journal: art-…"*.

The protection is narrow: an artifact the journal never named (an inbound
image, a file the operator saved) is collected exactly as before, and the
existing tests for that path are untouched.

## What this costs, stated plainly

The journal is append-only with **full retention** — segments rotate but are
never deleted — so it names every ref it ever recorded, forever. Two
consequences follow, and both are the roadmap's ask rather than accidents:

1. **A journal-named artifact is not collectable.** The tool outputs the
   daemon offloaded stay in the store (and in the file manager's list) until
   the journal forgets them — which, today, it never does. The collector still
   reaps everything else. Freeing those bytes needs a journal retention that
   forgets old `raw_ref`s; that is future work, and it is named in this
   document rather than pretended away.
2. **The guard costs one journal pass per operation.** The journal has no
   index — every read is a full scan — so a collect (or an explicit delete)
   reads the event log once. That is seconds on a large journal, and the
   operations are rare and operator-triggered. The boot path is unaffected:
   the guard is only asked when something is about to be collected.

The alternative — collecting the bytes and leaving the `raw_ref` dangling — is
what the code did, and it loses run output quietly.

## What did not change

The move is a path change and the fix is a protection:

- the same on-disk layout (`<home>/artifacts/<aa>/<ref>`, `<home>/artifacts/index/*.json`,
  the data lake's collections under `<home>/datalake/`), the same records, the
  same blobs, the same dedupe;
- the same package names and the same exported API — three things were **added**
  (`RefGuard`, `SetRefGuard`, `ErrStillReferenced`), nothing was changed or
  removed;
- the same wire: `artifact_collect`'s dry-run and reap answer exactly as
  before (same fields, same meanings), and `artifact_delete` still answers
  `{deleted, id}` — it just refuses a delete that would break the journal's
  reference, with a reason in words;
- the same events: the artifact packages emit none.

## Runnable evidence

Permanent tests:

- `kernel/modules/artifacts/artifact/index_refguard_test.go` — the proof
  (`TestCollect_KeepsTheBlobTheJournalStillNames`: the blob the journal names
  survives the collector), the refusal
  (`TestDelete_RefusesABlobTheJournalStillNames`), the narrowness
  (`TestCollect_StillReapsTheBlobsTheJournalDoesNotName`), and the batching
  (`TestCollect_AsksTheGuardOncePerOperation`: the guard is asked once, with
  every candidate);
- `kernel/runtime/artifact_refs_test.go` — the end-to-end proof against a real
  kernel and a real journal: the journaled `raw_ref` still fetches its bytes
  after a collect (`TestArtifactCollect_KeepsTheBlobTheJournalNames`), a
  journal-named blob that the journal does not name is still reaped
  (`TestArtifactCollect_StillReapsWhatTheJournalDoesNotName`), and a batch of
  three journal-named blobs is protected as a batch
  (`TestArtifactCollect_ProtectsEveryJournalNamedBlob`);
- `kernel/app/artifacts/artifact_referenced_test.go` — the operator-facing
  wording of the refusal, and that the not-found wording survives next to it.

The mutations fail tests: **11 mutations, 11 caught** — the guard is never
consulted, the protected candidates stay candidates, the explicit delete
ignores the guard, the guard is not stored, the guard is asked per entry, the
host reads another event kind, the host reads another field, the host stops at
the first hit, the host ignores the candidates, the guard is not installed, and
the application does not name the journal reference.

The gates pass: the build, `gofmt`, `go vet`, `tools/deadcodecheck`,
`tools/archcheck` (273 packages, 128/13 — the ratchet is unchanged, the edges
moved with their packages), `tools/structure-md`, `tools/changelog-lint`,
`tools/docclaimscheck` and staticcheck. The full `go test ./...` passes.

## Not done here

**The module has no `api` package yet.** The artifact index's types are
imported by the runtime, the application layer, the REST API and four plugins
(the `db`, `fetch`, `codeexec` and `artifacts` tools) — those are the
thirteen relocated allowlist entries. The reasoning module's shape (an `api`
package that owns the vocabulary, the host keeping its public names as
aliases) would shrink that surface; it is a separate slice.

**The journal still has no index.** The guard reads the event log once per
operation. An index over the event log (kind and ref, per segment) would make
that O(candidates) instead of O(journal) — and would also give the future
journal retention a cheap way to find the refs it may forget.
