# W3.76i — the loop stops re-exporting the contract

**Slice:** W3.76i (the ninth of the run-engine programme, the seventeenth of the
runtime's dissolution)
**Base:** `2c3a52a0` (the W3.76h merge)
**Date:** 2026-11-02

---

## 1. The family

W3.76h moved the loop into the runs module. What it left behind was the last
piece of the W1.1 contract move: the loop's **alias block**.

`kernel/modules/runs/agent/agent.go` was **53 non-blank lines** and held
nothing but re-exports:

| kind | names | source |
|---|---|---|
| type aliases (18) | `Role`, `Message`, `ToolCall`, `CompletionRequest`, `StopReason`, `CompletionResponse`, `Usage`, `Params`, `Provider`, `StreamingProvider`, `Chunk` (llm); `Tool`, `ToolDef`, `ToolCapability`, `EffectClass`, `ToolEffect`, `Result`, `ObservationTrust` (toolapi) | `kernel/contract/llm`, `kernel/contract/toolapi` |
| constants (15) | `RoleSystem`, `RoleUser`, `RoleAssistant`, `RoleTool`, `StopEndTurn`, `StopToolUse`, `StopMaxTokens` (llm); `EffectUnknown`, `EffectReadOnly`, `EffectReversible`, `EffectCompensable`, `EffectIrreversible`, `ObservationTrustDefault`, `ObservationTrusted`, `ObservationUntrusted` (toolapi) | `kernel/contract/llm`, `kernel/contract/toolapi` |

Every one of those names was an **alias**, not a definition: `agent.Provider`
*was* `llm.Provider` — the same type, the same methods, the same identity.
The block existed so that a caller could write `agent.Provider` and never
import the contract.

## 2. Why the block had to go

The contract half of the family moved to the contract layer in **W1.1**. The
alias block was the **shim** that made the move invisible — and it is exactly
the kind of shim that outlives its purpose: with it in place, the tree had
**two spellings** for every provider and tool row (`llm.Provider` and
`agent.Provider`), and nothing said which one a new file should use. The
package's own documentation had to say it in prose: *"the aliases here keep
every `agent.X` reference compiling with identical types; new code should
import the contract packages directly."*

A rule that only the documentation can enforce is not a rule. The block is
gone, and the tree has **one** spelling of every contract row.

## 3. What changed, and what stayed

**Gone:** `kernel/modules/runs/agent/agent.go` — the whole file.

**Repointed, inside the module:** **617** bare-name uses across **47** of the
package's files (15 production, 32 test) became `llm.X` / `toolapi.X` — the
`Provider` in `LoopConfig.Provider`, the `Message` in the conversation, the
`ToolDef` in the tool set, the `Result` in the memo. The four production
files that used no alias at all (the platform forwards) were not touched.

**Repointed, outside the module:** **22** uses in **17** files:
`kernel/approval`, `kernel/controlplane`, `kernel/edict`, `kernel/governor`
(three files), `kernel/modules/extensions/plugin` (two),
`kernel/modules/runs/loopcfg` (its test), `plugins/external/mcpbridge`,
`plugins/providers/anthropic`, `plugins/providers/internal/provopts`,
`plugins/tools/boardtool`, `plugins/tools/file`,
`plugins/tools/overseertool`, `plugins/tools/skilltool` and
`tools/structure-md` (its test). Every one of them now names the contract
directly.

**Stayed — the loop's own API:** `agent.Run`, `agent.LoopConfig`,
`agent.ToolSelector`, `agent.ToolSelectionRequest`, `agent.ToolMemo`,
`agent.Steerer`, `agent.Directive`, `agent.Middleware`, `agent.Wrap`,
`agent.GenerateObject`, `agent.LexicalToolSelector`,
`agent.DeferredLexicalToolSelector`, the observation helpers and the
sentinels. These are the **loop's** names, not the contract's.

**Stayed — the platform's forwards:** `agent_context_policy.go` (the policy
rows → `policyapi`), `agent_context_offload.go` (the artifact port →
`platform/tooloutput`), `schema.go` (the validators → `platform/schema`) and
`toolctx.go` (the context helpers → `platform/policyctx`). These are a
**different** seam: the loop's compatibility with the **platform**, put in
place by W2.3h/j/k, and they are not the contract.

## 4. The measurement

Against the base `2c3a52a0`:

| | base | now | delta |
|---|---:|---:|---:|
| `kernel/modules/runs` | 63 files / 7,264 | **62 / 7,229** | −1 / **−35** |
| the loop module | 20 / 2,656 | **19 / 2,621** | −1 / **−35** |
| `kernel/runtime` | 54 / 6,281 | **54 / 6,281** | **0** |
| the daemon's public verbs | **238** | **238** | **0** |
| the contract's rows | **10,406** | **10,406** | **0** |
| the application layer's lines | **38,009** | **38,009** | **0** |
| the control-plane's lines | **7,570** | **7,570** | **0** |

The module lost **35** lines: the alias block's **53**, minus the **18** the
qualified names cost. Nothing else in the tree moved — the daemon, the
contract, the application layer and the control plane are all **byte-for-byte
the same size**.

**No type changed.** An alias and its target are the same type, so every
caller that said `agent.Provider` and now says `llm.Provider` passes the
**same** value to the **same** parameter. That is the whole claim of the
slice, and it is the reason the slice's evidence is a compile-time one
(§6).

## 5. The tests

The module's test functions are **139 → 139**: the rewrite changed no test's
*meaning*, and one test changed its *name and its claim*:

- **Before:** `TestTheLoopKeepsItsContractAliases` — a compile-time
  assertion that the aliases existed and had the contract's identity.
- **After:** `TestTheLoopNoLongerReExportsTheContract` — the inverse. It
  parses the package's own production files and **fails if any of them
  declares a type alias to `llm` or `toolapi`**. The loop's public surface
  must not re-export the contract again.

The ratchet was **verified by a probe**: adding one line
(`type AliasProbe = llm.Provider`) to the package makes the ratchet fail
with the file and the row named, and removing it makes the ratchet pass
again.

## 6. Why this slice has no mutation run

Every other slice in this programme carries a mutation run, because every
other slice changed a **rule**. This one changed a **name**.

A mutation cannot distinguish an alias from its target: `agent.Provider` and
`llm.Provider` are the *same type*, so no behavioural change can separate
them. The instrument that fits this slice is the **type-checker**:

- every one of the **617** internal and **22** external renamings is
  type-checked by `go build ./...` and `go vet ./...` — a rename that hit a
  declaration name, a struct field or a composite-literal key would not
  compile;
- the **ratchet probe** shows the new invariant is enforced, not merely
  satisfied;
- the **full test suite** shows no behaviour moved.

One finding came out of the probe, and it is worth recording: the ratchet
was first written with the alias check **inverted** — it skipped the
declarations it was supposed to catch (`ts.Assign.IsValid()` read as "is an
alias" when it meant "is NOT an alias"). The probe caught it in one run; a
ratchet that is never probed is a ratchet that never fires.

One more observation, from the gates: the first full-suite run failed on two
tests that have nothing to do with this slice —
`toolbox.TestProbeVersion` (a `go version` probe that timed out under the
parallel load of `go test ./...`) and `mcpbridge.TestSSETransport_CloseEndsReadLoop`
(a local SSE dial the netguard blocked while the machine was busy). Both pass
in isolation, and the second full-suite run was green. They are the machine's
flakes, not this slice's, and they are recorded here because a gate that
fails once must be explained rather than re-run until it passes.

## 7. How the rewrite was done

By hand would have been 639 edits (617 internal + 22 external). The rewrite
was done with a purpose-built tool that edits **by AST position**:

- it parses each file with `go/parser` and computes the byte range of every
  identifier it renames;
- it refuses to touch identifiers that are **declaration names**, **struct
  field names**, **composite-literal keys** or **selector fields** — the four
  places where a name is a *name* rather than a *reference*;
- it applies the edits to the original bytes, so **comments and string
  literals stay byte-for-byte identical** (a renamed word inside a string
  would have changed what a test asserts);
- `goimports` then adds and removes the import lines.

The tool was checked against the compiler at every step: `go build ./...`
and `go vet ./...` are clean, and the full test suite passes.

## 8. The documentation

The living documents were corrected in the same slice, because a codemap that
names a symbol the tree no longer has is a map of a different tree:

- `architecture/00-README.md`, `02`, `04`, `05`, `06`, `07`, `08`, `10` — the
  alias names became the contract names, and the loop's path became
  `kernel/modules/runs/agent` where it was still `kernel/agent`;
- `architecture/04-agent-runtime.md` — the file table's `agent.go` row now
  says the file is **gone**, and says which names the loop still exports;
- the **historical** records were left alone on purpose: the W3.76h evidence
  (281) and the roadmap's W3.76h row describe the tree as it was at that
  slice, when the aliases still existed. The W3.76i row and this document
  say what changed since.

## 9. What is left

W3.76 is now **complete**: the runs module holds every family the target
architecture gives it, and the loop's public surface names the contract
directly.

The next step is the **W3 exit**: `kernel/runtime` and `kernel/controlplane`
dissolved, the daemon reduced to a host. That exit is what finally makes
`modules/runs/internal/loop` reachable — today the daemon, the scheduler,
the runner and `cmd/agezt` all import the loop, and `internal/` means
*importable only from inside the tree*.

After that: the **W2** domains (seven PRs), **W4** (five), **W5** (four).

## 10. The numbers that did NOT move

- the daemon's **public verbs**: 238 → 238.
- the **contract**: +0 rows. The rows were already there; only the loop's
  re-export of them is gone.
- the **application layer**: +0 lines.
- the **control plane**: +0 lines.
- the **types**: nothing changed identity — an alias and its target are the
  same type, which is the entire claim.
