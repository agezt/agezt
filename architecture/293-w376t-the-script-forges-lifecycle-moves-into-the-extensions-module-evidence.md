# W3.76t — the script forge's lifecycle moves into the extensions module

**Slice:** W3.76t (the twenty-eighth of the runtime's dissolution)
**Base:** `764830d2` (the W3.76s merge)
**Date:** 2026-11-02

---

## 1. The family

The **script-tool forge** is the family that lets an agent build its own
durable tools: it drafts a named script (Python/Node/Deno), tests it in the
code-exec sandbox, and — once a test of the current code passes — the
**operator** promotes it, and from then on every run can call it as a real
`forge_<name>` tool.

The forge's **catalogue** has been the extensions module's since W3.72, and the
forge's **file** is the host's (W3.45 — the daemon opens it and hands over the
contract's store port). What was left in the daemon was **seven forwarders**
over the module's own operations:

`DraftScriptTool`, `UpdateScriptTool`, `TestScriptTool`, `PromoteScriptTool`,
`RequestToolPromotion`, `QuarantineScriptTool` and `RemoveScriptTool`.

Each one did the same three lines: take the module's service out of the
daemon, call the module, return the module's answer. A forwarder over a
module's own operation is a **second spelling** of it — and the daemon held a
second copy of the whole forge surface, seven doors wide, for the application
layer and the plugin to call through.

## 2. The port the forge already had

The **contract** (`kernel/contract/toolforgeapi.Runner`) is the surface the
`tool_forge` plugin speaks, and it names the forge's operations the way the
forge's users say them: *draft a script tool*, *test a script tool*, *request
tool promotion*. The daemon implemented that port.

So the port was never the daemon's to implement — it was the contract's, and
the **module** now answers in those names. The module's service
(`kernel/modules/extensions/toolforge`) renamed its methods to the contract's
words, and the module's own port (`extapi.ForgeRunner`) **embeds** the
contract's `Runner` and adds the three verbs the contract does not name —
promote, quarantine, remove — plus the bridge (`MergeTools`).

The application's own contract already spoke those names: `kernel/app/tools`'s
`ForgeWriter` port listed `DraftScriptTool`, `UpdateScriptTool`,
`TestScriptTool`, `PromoteScriptTool`, `QuarantineScriptTool` and
`RemoveScriptTool`. So the application binds the module's face **directly** —
no adapter was needed, and none was written.

## 3. What moved

- **the module** answers the contract's vocabulary: its service's methods are
  the contract's names, and its api port embeds the contract's `Runner`;
- **the daemon** keeps two things and no more: the forge's **FILE**
  (`ToolForge` — the contract's store port) and the module's **FACE**
  (`ForgeRunner`);
- **the application layer** binds the module's face
  (`PortsFromKernel` fills `ForgeWriter` with `k.ForgeRunner()`), and a daemon
  with no forge holds no writer at all — the application then refuses with the
  contract's row (`toolforgeapi.ErrNoStore`) instead of calling into nothing;
- **the plugin** binds the module's runner too (`forgeRunnerPort(k)` answers
  `k.ForgeRunner()`), so `tool_forge` talks to the module and never to the
  daemon's verbs.

The daemon's `ErrNoForge` alias stays: it is the contract's row
(`toolforgeapi.ErrNoStore`), so a caller can ask the daemon's question in the
daemon's name and compare with `errors.Is`.

## 4. The measurement

| what | before | after |
|---|---|---|
| the daemon's public verbs | 179 | **173** |
| the daemon's production lines | 5,979 | **5,899** |
| the daemon's production files | 54 | **54** |
| the app packages that import `kernel/runtime` | 40 | **40** |
| the forge module's production lines (`extensions/toolforge` + `extensions/api`) | 916 | **923** |

**Six verbs off the daemon's surface in one slice** — the seven forwarders
gone, the module's face (`ForgeRunner`) added. The module's lines went **up**
by seven: *moving a rule moves the CONDITIONS, not the lines* — the module's
lines are the moved lines **plus** the port.

The app package count did not move, and the reason is worth recording:
`kernel/app/tools` still names the daemon for the families it owns besides the
forge — the tool registry (`Registered`), the MCP registrations and
attachments, and the tool invocation (`RunTool`). The forge's binding was
narrowed, not the package's. The **shape** is still the W3.76r/s one: the
daemon's verb for a family's operation is gone, and the application asks the
module.

## 5. The ratchets — and the probes

Two ratchets, **both probes fired**:

- the daemon's new forge ratchet (`kernel/runtime/forge_ratchet_test.go`)
  pins that the seven forwarders do not come back, and that the daemon still
  holds `ToolForge` and `ForgeRunner`;
- the application's new binding ratchet
  (`kernel/app/tools/forge_binding_ratchet_test.go`) pins that the tools
  package does not bind the **daemon** as its forge writer again. It is
  narrower than the workboard's ratchet on purpose: this package still names
  the daemon for the other families it owns.

The daemon's own surface ceiling (`kernel/runtime/dead_surface_ratchet_test.go`)
followed the floor again: **179 → 173**.

Putting the seven forwarders back on `*Kernel` and binding the daemon as the
application's forge writer failed **all three** at once — the retired-verb
ratchet, the surface ceiling (179 > 173) and the application's binding
ratchet. The first probe attempt failed even earlier: with the verbs gone from
the daemon, `ForgeWriter: k` does not compile at all — the daemon can no
longer satisfy the application's port, and the type system says so before any
test runs.

## 6. The coverage moved with the subject

The daemon's forge coverage drove the daemon's verbs, so it went where the
verbs went:

- the module's own tests now exercise the module's service under the
  contract's names (`kernel/modules/extensions/toolforge/forge_test.go`);
- the daemon's tests pin what the daemon still does — it holds the file, it
  holds the module's face, and it holds **no** verb per operation
  (`TestTheDaemonKeepsNoForgeVerbsOfItsOwn`);
- the refusal of every operation without a store now runs against the
  **application** layer (`kernel/runtime/scripttool_test.go` asks the
  application's lifecycle, not the daemon's verbs), because the refusal
  belongs to the layer that asks.

## 7. Verification

- the compiler (`go build ./...`, `go vet ./...`);
- the module's own tests (the lifecycle, the bridge, the contract type);
- the application's tests (the ports, the lifecycle, the catalog);
- the daemon's tests (the file, the face, the refusals, the ratchets);
- the plugin's tests (the store port, the runner port);
- the **full test suite** and **the thirteen gates** (`TOTAL FAILURES: 0`).

## 8. What is left

The forge is done: its file is the host's, its rules and its lifecycle are
the module's, and the application binds the module.

The same shape is waiting for the families that still have forwarders in the
daemon: the **workflow engine** (12), the **MCP registry** (12), the
**standing library** (9) and the **roster's** remaining read doors (9). The
MCP registry is the mixed one: the registry's lifecycle is the module's, but
the **attach/detach** verbs are the host's (they hold live child processes).
