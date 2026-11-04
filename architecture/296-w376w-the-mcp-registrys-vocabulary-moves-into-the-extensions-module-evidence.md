# W3.76w — the MCP registry's vocabulary moves into the extensions module

**Slice:** W3.76w (the thirty-first of the runtime's dissolution)
**Base:** `b89e170c` (the W3.76v merge)
**Date:** 2026-11-04

---

## 1. The family

The **MCP registry** is the family's data *and* its meaning: the durable
**registrations** (the servers an operator added), the **live attachments**
(the child processes a run may call) and the **bridge** (the rule that folds an
attached server's tools into a run's tool map).

W3.71 already moved the registry's **lifecycle and the bridge** into the
extensions module — the data and the operations finally live together. What
stayed in the daemon was the **vocabulary**: seven forwarders over the module's
own operations plus two readers of the daemon's own.

## 2. The port the contract already had

The **contract** named the registry's agent-facing surface in
`mcpapi.Runner` — **seven verbs**, in the names the users say them:

- `AddMCPServer`, `AttachMCPServer`, `DetachMCPServer`, `RemoveMCPServer` —
  the lifecycle;
- `MCPServers`, `MCPServer` — the rows;
- `MCPAttached` — the attachment status.

That is the port the **agent-facing MCP tool** binds (the tool is a PLUGIN, so
it may reach the contract and nothing else).

The daemon held **nine** verbs over the same ground: the contract's seven plus
`SetMCPServerEnabled` and `AttachEnabledMCPServers`. And the **module** spoke
a *different* vocabulary — `Add`, `SetEnabled`, `Attach`, `Detach`, `Remove`,
`AttachEnabled`, `Attached` — with **two private** reads
(`registrations()`, `registration(ref)`) where the contract named two public
ones.

So the family had **three** spellings: the contract's seven, the daemon's
nine, and the module's seven-plus-two-private. The slice leaves **one**.

## 3. What moved

- **the module** now answers in the **contract's** names: `Add` →
  `AddMCPServer`, `SetEnabled` → `SetMCPServerEnabled`, `Attach` →
  `AttachMCPServer`, `Detach` → `DetachMCPServer`, `Remove` →
  `RemoveMCPServer`, `AttachEnabled` → `AttachEnabledMCPServers`, `Attached` →
  `MCPAttached` — and the two private reads became the contract's two public
  ones (`MCPServers`, `MCPServer`), nil-safe over the registry port the module
  already holds. The module's port (`extapi.MCPRegistryService`) now **embeds**
  the contract's `mcpapi.Runner` and adds the four verbs the contract does not
  name (`SetMCPServerEnabled`, `AttachEnabledMCPServers`, `MergeTools`,
  `Close`) — the W3.76t forge shape, verbatim: **the module's port is the
  contract's port plus what the host still needs**.
- **the daemon** lost **nine** verbs and holds four things:
  - `MCPStore` — the registry's **FILE** (the contract's store port; the
    composition root opens the file, W3.58);
  - `MCPReader` and `MCPRegistrationCount` — the **nil-safe read gates**. A
    read over the host's own store is the host's, exactly as the roster's and
    the workflow library's reads stayed;
  - `MCPRegistryService` — the module's **FACE**.
- **the application** (`kernel/app/tools`) binds the module's face: the
  `MCPLifecycle` and `Attachment` ports answer `k.MCPRegistryService()`. The
  application's own port already spoke the **contract's** names
  (`AddMCPServer`, `AttachMCPServer`, `DetachMCPServer`,
  `SetMCPServerEnabled`, `RemoveMCPServer`), so **no adapter was written** —
  the module's service satisfies it directly. The application became
  **nil-safe**: a daemon with no module holds no writer, and the lifecycle
  refuses with the **contract's row** (`mcpapi.ErrNoRegistry`) through a
  refusing writer instead of calling into nothing.
- **the plugin** binds the module's face too (`mcpPort(k)` answers
  `k.MCPRegistryService()`, nil-safe: a daemon with no module hands over a
  **nil** port and the tool answers "unavailable" itself).
- **the composition root** asks the module's face for the **boot path**
  (`AttachEnabledMCPServers`) and gives the **marketplace** the module's face
  as its MCP installer (the marketplace's port already named the contract's
  `AddMCPServer` row — "the marketplace must not name the MCP module to
  register a server").
- **the daemon's alias** `runtime.ErrNoMCPRegistry` is **gone**: the daemon
  raises no refusal of its own any more, and a second spelling of a row it
  does not own is exactly what this program removes. The contract's
  `mcpapi.ErrNoRegistry` is the one row.

## 4. The measurement

| what | before | after |
|---|---|---|
| the daemon's public verbs | 164 | **156** |
| the daemon's production lines | 5,796 | **5,714** |
| the daemon's production files | 54 | **54** |
| the app packages that import `kernel/runtime` | 40 | **40** |
| the extensions module's MCP package | 1,323 | **1,326** |
| the extensions module's api package | 221 | **214** |
| the MCP contract | 289 | **289** |
| the tools application | 1,669 | **1,708** |

**Eight verbs off the daemon's surface in one slice** — nine forwarders and
readers gone, the module's face added.

The **contract did not move at all**: it already named the family in the words
the users say. The **module** moved by three lines (the renamed verbs and the
two reads that became public), and the **api** package moved by **seven lines
down** — the module's port is narrower now, because the contract's port says
the same thing in the contract's words.

The lines that went **up** are the ones that carry the conditions: the tools
application by thirty-nine (the refusing writer and the nil-safe guards) and
the daemon's file by the lines the forwarders used to hold.

## 5. The ratchets — and the probe

Two ratchets, **one probe firing all of them at once**:

- the daemon's new MCP ratchet (`kernel/runtime/mcp_ratchet_test.go`) pins that
  the nine verbs do not come back, and that the daemon still holds the
  registry's file, its read gates and the module's face;
- the tools application's new binding ratchet
  (`kernel/app/tools/mcp_binding_ratchet_test.go`) pins that the application
  does not take the MCP lifecycle (or the attachment status) from the
  **daemon** any more. The ratchet matches the *shape* `Attachment: k` — the
  handle itself — and not a call on it (`Attachment: k.MCPRegistryService()`),
  because the module's face is the module's.

The daemon's own surface ceiling (`kernel/runtime/dead_surface_ratchet_test.go`)
followed the floor again: **164 → 156** (220 → 201 → 194 → 179 → 173 → 166 →
164 → 156), and the nine retired names joined the retired-verb list.

**The probe** put the daemon's `MCPAttached` verb back and bound the daemon as
the application's attachment port: the retired-verb ratchet, the surface
ceiling (157 > 156), the daemon's MCP ratchet **and** the application's binding
ratchet all went red at once. The probe was reverted and the suites went green
again.

## 6. The coverage moved with the subject

The daemon's MCP coverage drove the daemon's verbs, so it went where the verbs
went:

- the **module** keeps its own tests — the registry's refusal without a store
  (`mcpapi.ErrNoRegistry`), the lifecycle's transitions, the bridge's rule —
  and they now run against the **contract's** names;
- the **daemon's** tests pin what the daemon still does: it holds the file,
  the read gates and the face, it holds **no** verb per operation, and a
  daemon with no module holds **no** face at all
  (`TestTheDaemonHoldsNoRegistryFaceWithoutTheModule`,
  `TestTheDaemonKeepsOnlyTheRegistryGates`);
- the **application's** tests pin the refusal: a daemon with no module yields a
  lifecycle that refuses with the contract's row
  (`TestTheToolsPortsReachTheKernelTools`);
- the **composition root's** tests pin the wiring: the port the MCP tool
  receives is the module's face
  (`TestToolKernelDepsSuppliesTheMCPAndStandingPorts`), and the plugin's
  wiring is the module's face too
  (`TestMCPSpecWiresTheLiveKernel`).

## 7. Verification

- the compiler (`go build ./...`, `go vet ./...`);
- the module's own tests, the daemon's tests, the application's tests, the
  control plane's tests, the plugins' tests, the composition root's tests;
- the **full test suite** and **the thirteen gates** (`TOTAL FAILURES: 0`).

## 8. What is left

The registry's vocabulary is the contract's, the daemon holds the module's
face, and the application and the plugin ask the module.

The same shape is waiting for the families that still have forwarders in the
daemon: the **standing library** (nine verbs, and **no module home** yet — it
needs one before its four writes can move) and the **roster's** remaining write
doors (five forwarders, and the config plugin's port has to be split).
