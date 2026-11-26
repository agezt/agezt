# W5.20 — the request-body ratchet: every body is behind a door, and the list may only shrink

## 1. What was wrong

The programme's sentence had become true, slice by slice: **every surface's
request body is named in a contract, and every surface answers a mistyped body
in the registry's words.**

- the `/api/v1` surface (W5.11–W5.13),
- the console's own endpoints (W5.12),
- the OpenAI-compatible surface (W5.15),
- the agent gateway (W5.14),
- the fifteen channel webhook members (W5.16–W5.18),
- the ACP server (W5.19).

But that sentence was a **document**. A document does not fail a build. Two
things were therefore still possible:

1. **A new surface could read a body with no contract, no schema and no
   door** — and nothing would say so.
2. **A surface that HAS a door could grow a second, unguarded body** — a
   handler added next to the measured one, reading the body into whatever
   struct it happened to use. The surface would still "have a door"; the
   second body would simply be behind it.

The inventory that the programme produced — a table of surfaces and their
doors — was true when it was written and decays silently.

## 2. What was built

### 2.1 A gate that asks one question

`tools/bodydoor` walks the tree and asks, per **function**:

> does this function read an inbound request body, and is a door in reach?

A **door** is a call to the shared core (`kernel/httpserver`):

```go
httpserver.DecodeJSONBody(...)  // the REST surface, the console, the
                                 // OpenAI-compatible surface, the gateway
httpserver.CheckJSONBody(...)   // the channel webhook members, the ACP server
```

A function that calls a door directly is behind it; so is a function that
calls a same-package function that does — the closure is computed per package,
to a fixed point, because that is how the console and the gateway are built
(their handlers call a local helper, and the helper calls the core).

A **read** is a use of `<request>.Body`, where `<request>` is a parameter of
type `*http.Request` of the function being analysed. The **type** is what
decides: an outbound client reads `resp.Body`, and that is none of this
tool's business — a false "request body" on every HTTP client in the tree
would be a gate nobody keeps.

### 2.2 The declarations, and why they are a ratchet

A body read with no door in reach must be **declared**, in
`tools/bodydoor/exceptions.txt`, with the reason it is not measured:

```
kernel/httpserver/body.go :: the shared door core: it IS the door …
kernel/httpserver/limits.go :: the body-cap wrapper: it re-wraps the body …
kernel/webui/transcribe.go :: an audio upload (multipart), not a JSON body …
kernel/webui/webui_proxy.go :: the console's forwarding proxy …
kernel/webui/webui_security.go :: the workflow hook's body is opaque on purpose …
plugins/channels/sms/sms.go :: a form, not a JSON body …
plugins/channels/wecom/wecom_message.go :: an XML callback, not a JSON body …
```

The list is a **RATCHET**, in the same shape as `archcheck`'s:

- a body read with no door and no declaration **FAILS** the check (no new
  debt);
- a declaration whose file **no longer reads a body** ALSO fails, so a fixed
  file must be deleted from the list in the same change — and can never
  silently come back.

`go run ./tools/bodydoor -update` drops the stale ones.

### 2.3 It is wired where it is run

The gate lives beside the other repository gates: `make body-door-check`, and
a step in CI's `deps-check` job (`go run ./tools/bodydoor`). A gate that only
exists in a Makefile target nothing runs automatically is the failure class
this repository has already paid for twice.

## 3. What was measured

The tree, measured by the gate itself (the same program runs on `main`
(`d5f326c0`) and on this branch):

| | before | after |
|---|---|---|
| functions that read an inbound request body | 22 | 22 |
| of those, behind a door | 15 | 15 |
| of those, declared as exceptions | **prose in a document** | **7 declarations, in a checked list** |
| request-body reads the tree makes with no door | **not enforced** | **9 read expressions in 7 files, each declared** |
| a new unguarded body read | **silent** | **fails the build** |

The last row is the one that matters, and it is measured, not asserted: a
probe handler that reads `r.Body` with no door was injected into
`kernel/restapi`, and the gate named it —

```
bodydoor: a request body is read with no door in reach:
  kernel/restapi/zz_probe_tmp.go:14 (probeUnguardedRead): reads the request body
```

— and failed. The probe was deleted, and the gate went back to green.

The **before** side of the table is not a different tree: it is the *same*
tree with the same 22 reads, and no gate to enforce anything. Run against
`main` without the declarations, the tool reports the same nine read
expressions in seven files — the exceptions were true, but they were prose,
and nothing checked them.

## 4. The guards

**Six** guards hold the tool to its own question (in `tools/bodydoor`):

- a read behind a door is not a violation;
- a read behind a **local** door (one call away) is not a violation;
- a read with no door **is** a violation;
- a **response** body is not a request body;
- a local function that happens to share a door's **name** is not a door;
- a declared file is exempt, and a declaration that has gone **stale** fails
  (the ratchet), and `-update` drops it.

**Eight reverted mutations** prove them — one of which breaks the closure
(the door one call away), one of which makes a response body count as a
request body, one of which makes a same-named local function pass for a door,
and one of which breaks the ratchet in each direction.

## 5. What is untouched

- **No production code changed.** The gate reads the tree; it does not move
  it. The seven exceptions are the tree's own truth, stated.
- **No SDK moved** (`sdkgen -check` is in sync) and **no contract moved**.
- The **tool itself** is a new package under `tools/` (layer 7), so no layer
  rule changed (`archcheck` still reports 0 violations).

## 6. What remains open

The programme's sentence now has a **gate** behind it, and the two smaller
ends it named are still open — both deliberately, both a decision rather than
an omission:

- **`chatwebhook`'s Mattermost dialect** is a **form**, so the door does not
  measure it. Measuring it needs a form-shaped sibling of `CheckJSONBody`.
- The **SDK catalogs** have not moved through W5.11–W5.20: the SDKs speak
  `/api/v1`, and neither the channel webhook family nor the ACP surface is
  part of the SDK contract.

One honest limit of this gate is worth naming: it matches a door by **name**
within a package, not by type. A package with two methods of the same name —
one of them a door — would let one stand for the other. The tree has no such
pair, and the door functions have distinctive names; but a future package
could, and the fix would be a type-aware resolution rather than a longer
name list.
