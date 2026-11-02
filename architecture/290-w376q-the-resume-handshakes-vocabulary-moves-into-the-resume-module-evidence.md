# W3.76q — the resume handshake's vocabulary moves into the resume module

**Slice:** W3.76q (the twenty-fifth of the runtime's dissolution)
**Base:** `ba7593cc` (the W3.76p merge)
**Date:** 2026-11-02

---

## 1. The family

The **resume handshake** is two facts a run carries in its context: the
**owned marker** (an outer frame — a governed wrapper or the resumer — already
holds this run's ticket) and the **seed** (the conversation a suspended run
must continue from).

Both of them lived in the daemon (`kernel/runtime/resume.go`): the context
keys, the package-level setters the resumer called, and the readers the run
engine asked the host for.

Both of them are the **resume module's** vocabulary. The module owns the
ticket, its kinds (`run`, `assured`, `retry`), its dispositions and its
snapshot; the owned marker says *which ticket kind an outer frame holds*, and
the seed carries *the conversation that ticket saved*. A word that describes a
ticket is the ticket's owner's word.

The module's own package doc admitted the misplacement in one sentence:

> The runtime assembles a Ticket from its context values; the resumer (in the
> daemon, **where the context builders live**) rebuilds a run context from the
> flattened fields.

The builders live there now — in `kernel/modules/runs/resume`, next to the
ticket they describe.

## 2. What moved

The resume module gained the handshake (`handshake.go`, 66 lines): the two
context keys and the four functions that speak them — `WithSeed`,
`SeedFromCtx`, `WithOwned`, `OwnedKindFromCtx`.

The daemon lost:

- the two context keys and the two private readers;
- the package-level `WithResumeSeed` / `WithResumeOwned` setters (the resumer
  in `cmd/agezt` calls the module now);
- the two public readers `ResumeSeedFromCtx` / `ResumeOwnedKindFromCtx` — the
  run engine asks the module now.

The run engine reads the handshake from the module it lives next to:

```go
if kind, owned := resume.OwnedKindFromCtx(runCtx); ownedHere || (owned && kind == resume.KindRun) {
	resumeCheckpoint = r.k.ResumeCheckpointFn(corr)
	if msgs, it, ok := resume.SeedFromCtx(runCtx); ok {
		resumePriorMessages, resumeStartIter = msgs, it
	}
}
```

**What stayed in the host is the host's own state**: the checkpoint callback
(the host owns the ticket store), the resolved model and the run's effective
configuration. The handshake's *words* moved; the *state* did not.

## 3. The measurement

| measure | before | after |
|---|---:|---:|
| the daemon's public verbs | 203 | **201** |
| the daemon's production lines (non-blank) | 6,162 | **6,121** |
| the run engine's port (`runner.KernelAPI`) | 47 | **45** |
| the resume module's production lines (non-blank) | 569 | **635** |
| the runner package's production lines (non-blank) | 908 | **910** |

**This is the first slice in a while where the daemon shrank.** The previous
ones moved bodies *into* the host's wiring; this one moved a vocabulary *out* of
it, and the host's doors went with the words: two public verbs and two port
entries.

## 4. The ratchets — and the probes

Two ratchets changed, both honestly:

- the **run engine's port-shape ratchet** now pins the two handshake names'
  absence next to the run-context names and the roster names;
- the **daemon's own ratchet** flipped: `TestTheDaemonKeepsItsOwnResumeKeys`
  used to *require* the daemon's `ctxKeyResumeOwned` / `ctxKeyResumeSeed`
  ("the daemon's boot protocol, not the run's vocabulary") — the decision that
  W3.76l and W3.76m made. It is now
  `TestTheDaemonKeepsNoResumeKeysOfItsOwn`, and it pins their **absence**,
  over the daemon's **whole package** (the keys were declared in one file, and
  a file-scoped check would have watched only that one).

**Both probes fired**:

```
--- FAIL: TestTheHostStillImplementsThePort (0.00s)
    home_internal_test.go:138: the KernelAPI port has a ResumeSeedFromCtx method again — the resume handshake's vocabulary is the resume module's
--- FAIL: TestTheDaemonKeepsNoResumeKeysOfItsOwn (0.01s)
    runctx_ratchet_test.go:105: the daemon declared ctxKeyResumeOwned; the resume handshake's vocabulary is the resume module's
```

## 5. Verification

- **the compiler**: `go build ./...` and `go vet ./...` clean;
- **the module's own test** (`kernel/modules/runs/resume/handshake_test.go`):
  the owned marker, the seed, a bare context carrying neither, and the empty
  conversation that must not resume;
- **the daemon's test** (`kernel/runtime/resume_lifecycle_test.go`): the
  resumer's exact dispatch contract still works — the seeded conversation
  reaches the run and the ticket is cleared on clean completion;
- **the ratchets and their probes** (§4);
- **the full test suite** and **the thirteen gates**.

**No mutation run.** The slice moves a vocabulary and deletes the doors that
spoke it; there is no rule whose reversal a mutation could expose. The
evidence is the compiler, the permanent tests, the ratchets and their probes,
and the full suite.

## 6. What is left

The daemon holds **201** public verbs and **6,121** production lines; the run
engine's port carries **45** entries. The W3 exit criterion — *no type has more
than 60 methods* — is met by the port with room to spare, and the daemon's own
`*Kernel` (201 verbs) is still the type that breaks it.

The next doors in the same family: the runner's port still promises
`RunWith` and `RunWithRetry` (the engine delegates the child's run back to the
host — the documented "Day 23 gap"), and the daemon's own ticket doors
(`ClaimResumeTicket`, `FinalizeResumeTicket`, `ResumeCheckpointFn`) still
carry the ticket protocol's *mechanics* while the module keeps its *rules*.

Evidence: `architecture/290-w376q-the-resume-handshakes-vocabulary-moves-into-the-resume-module-evidence.md`
