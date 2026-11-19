# W3.79 — the operation vocabulary gets one home

> **Slice:** W3.79 · **Date:** 2026-11-18 · **Base:** `3dc1ac5d` (main)
> **Kind:** a vocabulary moves; no behaviour changes.
> **Supersedes:** the `Cmd*` constant block in `kernel/controlplane/protocol_commands*.go`.

---

## 1. What was wrong

The daemon answers to **325** command names. Until this slice, that vocabulary was
written **twice**:

1. `kernel/controlplane/protocol_commands*.go` declared 325 `Cmd*` string constants
   (`CmdApprovalsLog = "approvals_log"`, …) across five files — 1,515 lines with
   their documentation.
2. The application layer declared the same names again, as string literals inside
   the operation specs (`opapi.Spec{Name: "approvals_log"}`, …), spread over 65
   packages.

The two lists agreed — that was the finding, and it is worth stating plainly: the
**325 constants are exactly the 324 typed operations plus one native command**
(`whoami`). The vocabulary was never out of sync. But it was *declared* twice, so
nothing but a reader's memory kept it that way, and the target architecture's
"OpSpec is the single source of truth" (P2, `architecture/20-target-architecture.md`
§3.1) was **not** true of the name itself.

The cost of the second declaration was not the 325 constants — it was that every
name in the system had to be *spelled* by hand in the place that used it:

| Surface | How the name was reached |
|---|---|
| `kernel/app/**` (65 packages) | a string literal in the spec |
| `kernel/controlplane` (the dispatch registry) | `controlplane.CmdX` |
| `kernel/webui` (198 route entries) | `controlplane.CmdX` |
| `cmd/agt` (the CLI, 252 files) | `controlplane.CmdX` |
| `sdk` (the Go SDK) | `controlplane.CmdX` |

**1,162 production references** across **597 files**.

## 2. What moved

The vocabulary is now **one catalogue**, in the contract layer, where the
operation's *descriptor* already lived:

```
kernel/contract/opapi/names.go          301 lines    58 names
kernel/contract/opapi/names_data.go     457 lines    67 names
kernel/contract/opapi/names_domains.go  486 lines   132 names
kernel/contract/opapi/names_edict.go    236 lines    32 names
kernel/contract/opapi/names_misc.go      63 lines    36 names
                                     ─────────────
                                     1,543 lines   325 names
```

The five files map one-to-one onto the five they replace, so the move stays
reviewable: `protocol_commands.go` → `names.go`, `protocol_commands_data.go` →
`names_data.go`, and so on. The identifiers lost the `Cmd` prefix
(`CmdApprovalsLog` → `opapi.ApprovalsLog`); **the values did not change**, so the
wire protocol did not change.

The documentation travelled with the names. Every constant kept the comment that
described its arguments and its return shape; references to sibling commands
inside those comments were repointed to their new spelling.

The application layer now **binds** the catalogue: 320 operation specs name
themselves with the constant instead of a literal, and the four operation names
the `kernel/app/files` family kept on its own (`Mkdir`, `Rename`, `Delete`,
`RestoreOperation`) point at the catalogue too — their *error* codes
(`file_invalid_path`, `file_io`, …) are the family's own and stayed.

## 3. What is gone

- `kernel/controlplane/protocol_commands.go`
- `kernel/controlplane/protocol_commands_data.go`
- `kernel/controlplane/protocol_commands_domains.go`
- `kernel/controlplane/protocol_commands_edict.go`
- `kernel/controlplane/protocol_commands_misc.go`

— 1,515 lines, and with them the 325 `Cmd*` constants. Every reference now points
at `kernel/contract/opapi`.

## 4. The guard

A new ratchet, `kernel/controlplane/op_vocabulary_ratchet_test.go`, holds three
tests:

1. **`TestOperationVocabularyMatchesCatalogue`** — the registry and the catalogue
   are the same vocabulary. An operation registered under a name the catalogue
   does not carry fails; a catalogue name with no operation behind it fails. The
   one native command (`whoami`) is declared as an exception, with its reason.
2. **`TestApplicationLayerNamesComeFromTheCatalogue`** — the application layer
   does not repeat a name as a literal where the constant belongs. This is the
   test that keeps the catalogue *used*; the first one only keeps it *true*.
3. **`TestNativeCommandsAreDeclared`** — the native exception stays honest: a
   name declared native must not have a typed operation behind it, and vice versa.

**The ratchet was proved to fire.** Two mutations, each reverted:

| Mutation | Expected | Result |
|---|---|---|
| an operation renamed to `status_bogus` | test 1 fails | failed with both symptoms: *"operation "status_bogus" is registered but the catalogue does not carry it"* and *"catalogue name "status" (Status) has no operation behind it"* |
| the same operation named `"status"` (same value, literal) | test 2 fails | failed: *"../app/system/operations.go writes the operation name "status" as a literal; the catalogue's constant belongs here"* |

The first mutation is a divergence; the second is a re-divergence of the *same*
name in a second place. The first test cannot see the second one — the value is
identical, so nothing is broken — which is why the second test exists.

**The pre-existing ratchet was repointed, not deleted.**
`kernel/controlplane/dispatch_registry_test.go`'s
`TestRegistry_MatchesProtocolConstants` read the `Cmd*` constants off disk to
compare them with the dispatch registry. It now reads the catalogue. The
invariant is unchanged and now stronger: it says the *registry* and the
*catalogue* are the same set, and it covers the native command too.

## 5. The measurement that moved, and the one that did not

**Moved:**

| Measurement | Before | After |
|---|---:|---:|
| `Cmd*` constants | 325 | **0** |
| Names declared in the control plane | 325 (1,515 lines) | **0** |
| Names declared in the contract layer | 0 | **325** (1,543 lines) |
| References to a command name | 1,162 | 1,162 — but to **one** declaration |
| Files carrying the vocabulary | 5 | 5 (in `kernel/contract/opapi`) |

**Did not move, on purpose:**

- The **registry**: 324 typed operations before, 324 after. The same names, the
  same order, no duplicates.
- The **wire protocol**: every value is the same string it was.
- The **documentation**: the same words, in the same order, beside the same
  operation — only the file and the spelling of the identifier changed.

The catalogue is 28 lines longer than the block it replaces. That is the five
file headers; the names and their comments moved verbatim.

## 6. Why the contract layer

The catalogue cannot live in the application layer: `kernel/controlplane` would
have to import it for the dispatch registry, and the application layer is
*below* the control plane — the dependency would point the wrong way. It cannot
live in the control plane either: the application layer would have to import it
to bind the spec's name, and `archcheck` forbids that direction (L4 → L5 is an
upward edge).

`kernel/contract/opapi` is the layer both can see from: it already owns the
operation's *descriptor* (`Spec`), and it is pure — types and constants, no
logic, no imports outside the standard library. The name and the descriptor now
sit together, which is what "the operation's contract" means.

## 7. What this does not do

- **It does not generate the clients.** The Web UI's 198 route entries, the
  CLI's 252 files and the SDKs still name their operations by hand. They now
  name them by *constant*, which removes the drift risk, but a route table is
  still a hand-maintained table. The next slice can read the catalogue's
  `Spec.HTTP` hints and derive the routes.
- **It does not retire the native command.** `whoami` stays native, by design:
  it echoes the transport principal and must not route the tenant it names.
- **It does not touch the values.** Nothing about the daemon's behaviour, its
  protocol, or its answers changed.

## 8. The gates

All thirteen, green:

| Gate | Result |
|---|---|
| `gofmt -l kernel tools cmd plugins contract` | clean |
| `go build ./...` | green |
| `go vet ./...` | green |
| `go test ./...` | green |
| `tools/archcheck` | 339 packages placed; **1** allowlisted import violation; **0** call sites |
| `tools/deadcodecheck` | no unexpected dead code; 33 public SDK findings + 1 test seam allowlisted |
| `tools/depscheck` | 24 core dependencies, all justified |
| `tools/sdkparity -check docs/SDK-PARITY.md` | green |
| `tools/structure-md -check` | green |
| `tools/changelog-lint` | green |
| `tools/docclaimscheck -base origin/main` | green |
| `tools/docsindex` | green |
| `staticcheck` (pinned `GOTOOLCHAIN=go1.27.1`) | green |

## 9. The shape of the change

| | Files | Lines |
|---|---:|---:|
| Deleted (`protocol_commands*.go`) | 5 | 1,515 |
| Added (`kernel/contract/opapi/names*.go`) | 5 | 1,543 |
| Added (`op_vocabulary_ratchet_test.go`) | 1 | 170 |
| Rewritten (references repointed) | 589 | — |
| Documentation (this file, the changelog, `NEXT.md`) | 3 | — |

**603 files touched** by this slice (600 Go files + 3 documents). The working tree
also carries an unrelated work-in-progress in `workboard`, `selfrepair`,
`reasoning` and `security-report`, which this slice did not touch; the whole
working tree diff is 607 files, 3,479 insertions, 5,073 deletions.

## 10. What is left

The vocabulary has one home. The **clients** do not: the CLI, the Web UI's route
tables and the SDKs are still hand-written mirrors of the operation set. That is
the next slice, and the catalogue is now the place it can be read from.
