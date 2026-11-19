# W3.80 — the CLI command vocabulary gets one home

> **Slice:** W3.80 · **Date:** 2026-11-18 · **Base:** `3dc1ac5d` (main)
> **Kind:** a vocabulary moves; no behaviour changes.
> **Follows:** W3.79, which gave the daemon's 325 operation names one home
> (`kernel/contract/opapi`). This slice gives the CLI's 311 sub-command names
> one home (`cmd/agt/commands_dict.go`).

---

## 1. What was wrong

`agt` has **46 command domains** and **311 sub-commands**. Until this slice,
every domain's list of sub-command names was written **four or five times**:

| Surface | How the names were written |
|---|---|
| the dispatcher's `switch args[0]` | the definition |
| the usage line (`usage: agt agent <list\|add\|…>`) | a copy |
| the unknown-sub-command message | a copy (in 19 of 44 defaults) |
| the "subcommand required" guard | a copy (in 37 variants) |
| `cmd/agt/help_data.go`'s detail lines | a copy |

The copies had already drifted. The measurement that started this slice found
**nine stale lists**: `agt agent`'s own usage line was missing `authority`, the
help table still carried `preview` next to `dry-run`, `standing`'s list had
grown an `edit` the dispatcher no longer answered to, and so on. Nothing
checked the copies against the definition, so the drift was invisible until a
person typed a name the help did not offer.

The names could not simply be *derived* from the operation catalogue, and this
slice measured why before it designed anything:

- the `domain_sub` naming rule matched only **175 of 360** CLI names (48 %);
- the CLI has **alias** names (`ls`, `rm`, `create`, `gen`, `viz`) that no
  operation carries;
- **33 commands drive no operation at all** (they print help, or they work
  entirely on the client).

So the CLI's vocabulary is *not* the daemon's vocabulary. It is its own — and it
deserves its own single home, tied to the catalogue by the operations each
command drives.

## 2. What moved

The vocabulary is now **one table per domain**, in
`cmd/agt/commands_dict.go` (2,286 lines):

```
46 domains · 311 commands · 247 with an operation column · 28 hidden (help aliases)
```

One command, as it is written there:

```go
{
	names: []string{"add", "create"},
	ops:   []string{opapi.AgentAdd},
	run:   shifted(cmdAgentAdd),
},
```

- `names` — every name the command answers to. `names[0]` is the canonical one;
  the rest are aliases. The synopsis prints the canonical name only.
- `ops` — the catalogue operations the command can drive, named by the
  catalogue's **constants**, not by literals: a name the catalogue does not
  carry does not compile.
- `run` — how the command runs.

The dispatchers no longer keep a list. They look the table up:

```go
func cmdAgent(args []string, stdout, stderr io.Writer) int {
	if len(args) == 0 {
		return agentUsage(stderr)
	}
	if sub, ok := cliAgent.lookup(args[0]); ok {
		return sub.run(nil, args, stdout, stderr)
	}
	fmt.Fprintf(stderr, "%s agent: unknown subcommand %q\n", brand.CLI, args[0])
	return agentUsage(stderr)
}
```

**44 dispatchers** now dispatch through the table (46 domains; `cmdSeat` and
`cmdProviderCreds` are the two that were written by hand — §4).

Everything else that used to spell a list out now *derives* it:

```go
fmt.Fprintf(w, "usage: %s agent %s\n", brand.CLI, cliAgent.synopsis())
```

**35 messages** across the CLI derive their list from the table (33 rewritten
mechanically, 2 written by hand with `cmdProviderCreds`), among them the usage
lines of `agent`, `mcp`, `okr`, `rollback`, `skill workshop`, `standing`,
`taste`, `token`, `toolforge`, `workboard` and `workflow`, the
unknown-sub-command messages, the "subcommand required" guards and the
`help_data.go` detail lines.

The derived lists are *better* than the ones they replaced: `agt agent`'s usage
line now names `authority` (the definition had it, the copy did not), and
`rollback`'s names `preview` next to `dry-run`.

## 3. What is gone

- **44 `switch args[0]` statements** — the dispatcher's copy of the vocabulary.
- **35 hand-written command lists** in usage lines, unknown-sub-command
  messages, guards and the `help_data.go` detail lines.

No name was invented and no name was dropped: the table's 311 commands are the
dispatchers' 311 cases, one for one.

Two `switch args[0]` statements are left in `cmd/agt`, and both are outside the
CLI's domain vocabulary: `cmd/agt/main.go`'s root dispatch (the CLI's entry
point) and `cmd/agt/keys/keys.go` (a separate package).

## 4. The two hand-written domains

Two domains could not be moved mechanically, and were written by hand rather
than forced:

- **`cmdProviderCreds`** opened a credential store *before* it dispatched. A
  table entry cannot carry the resource the dispatcher opened, so the three
  commands grew thin wrappers (`credsList`, `credsSet`, `credsRm`) that open the
  store themselves. The names live in the table like every other domain's.
- **`cmdSeat`'s "list" case did not return** — it fell through to the listing
  code below the switch. The listing became its own function (`cmdSeatList`)
  and the table points at it.

Both are declared in the migration's manual table, so the move is reproducible
rather than a one-off edit.

## 5. The guard

`cmd/agt/commands_dict_ratchet_test.go` holds four tests:

1. **`TestDictionaryIsTheOnlyCommandVocabulary`** — no message spells a domain's
   command list out by hand any more. Every string literal in `cmd/agt` is
   scanned for a `<a|b|c>` or `(a, b, c)` group; a group whose names are all
   names of one domain is a second copy of that domain's vocabulary, and the
   test fails.
2. **`TestDictionaryOperationsMatchTheirHandlers`** — the operations a command
   claims are the operations its handler really reaches. A command that claims
   an operation it never calls is lying; an operation a handler reaches that no
   command of it claims is undeclared. The reachability follows the tables, so a
   nested domain (`agt config schema`) is measured through its own table.
3. **`TestEveryDomainDispatchesThroughTheDictionary`** — a dispatcher does not
   keep a list of its own: the functions that look a domain up must not switch
   on the sub-command name any more.
4. **`TestDictionaryNamesAreUnique`** — a domain cannot answer to the same name
   twice, and every command has a name and a runner.

**The ratchet was proved to fire.** Two mutations, each reverted:

| Mutation | Expected | Result |
|---|---|---|
| the `agent list` command's operation changed from `opapi.AgentList` to `opapi.AgentRemove` | test 2 fails | failed with both symptoms: *"cliAgent [list] claims the operation "AgentRemove", but [cmdAgentList] never reaches it"* and *"the operation "AgentList" is reachable from cmdAgentList but no command that runs it claims it"* |
| `agt agent`'s usage line written out by hand again (`usage: %s agent <list\|add\|…>`) | test 1 fails | failed: *"agent.go writes the command list "list\|add\|show\|impact\|tombstone\|graveyard\|set\|task\|wake\|repair\|repair-status\|pause\|resume\|retire\|revive\|remove" of cliAgent out by hand; the dictionary derives it"* |

The first mutation is a divergence between the table and the handler; the
second is a re-divergence of the *same* names in a second place. The first test
cannot see the second one — the names are identical, so nothing is broken —
which is why the second test exists.

## 6. The measurement that moved, and the one that did not

**Moved:**

| Measurement | Before | After |
|---|---:|---:|
| domains with a hand-written command list | 46 | **0** |
| `switch args[0]` statements in `cmd/agt` | 47 | **2** (the root dispatch and the separate `keys` package) |
| hand-written command lists in messages | 35 | **0** |
| places the vocabulary is written | 4–5 per domain | **1** |
| commands whose operation is pinned to the catalogue | 0 | **247** |

**Did not move:** the number of commands (**311**), the number of domains
(**46**), the names themselves, the wire protocol, and the exit codes — the
dispatcher's guard and its unknown-sub-command branch were carried over
verbatim, so `agt agent bogus` still exits the way it did before.

**The table is bigger than the switch it replaces** (2,286 lines against the
~1,900 the 44 switches occupied). *Moving a vocabulary moves the
declarations, not the lines* — the table carries the operation column, the
aliases and the runners, and the dispatchers lost only their `case` arms.

## 7. What is out of scope

- **`cmd/agt/keys`** is a separate package (`package keys`) with its own
  vocabulary (`list|add|activate|rm`). It was not touched.
- **`agt help`'s top-level overview** (`cmd_register.go`'s 66 commands) is the
  *top-level* vocabulary, not the domain one. It is a separate table and a
  separate slice.
- **`cmd/agt/main.go`'s root dispatch** (`run`) is the CLI's entry point, not a
  domain.

## 8. The gates

All thirteen gates are green at `3dc1ac5d` + this slice:

```
gofmt -l kernel tools cmd plugins contract   → clean
go build ./...                                → clean
go vet ./...                                  → clean
go test ./...                                 → ok
go run ./tools/archcheck                      → 339 packages, 1 allowlisted violation, 0 call sites
go run ./tools/deadcodecheck                  → OK
go run ./tools/depscheck                      → OK, 24 dependencies
go run ./tools/sdkparity -check docs/SDK-PARITY.md → ok
go run ./tools/structure-md -check            → ok
go run ./tools/changelog-lint                 → OK
go run ./tools/docclaimscheck -base origin/main → OK
go run ./tools/docsindex                      → OK
staticcheck ./...                             → clean (GOTOOLCHAIN=go1.27.1)
```
