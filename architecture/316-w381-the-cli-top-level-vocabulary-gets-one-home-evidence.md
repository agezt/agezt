# W3.81 — the CLI's top-level vocabulary gets one home

> **Slice:** W3.81 · **Date:** 2026-11-19 · **Base:** `3dc1ac5d` (main) + W3.79 + W3.80
> **Kind:** a vocabulary moves; no behaviour changes.
> **Follows:** W3.80, which gave the CLI's 311 sub-command names one home
> (`cmd/agt/commands_dict.go`). This slice gives the CLI's **71 top-level
> command names** one home (`cmd/agt/commands_top.go`).

---

## 1. What was wrong

The CLI's top-level vocabulary was written **three** times:

| Surface | How the names were written |
|---|---|
| `cmd/agt/cmd_register.go` | 71 `Register(&Command{Name, Aliases, Run})` calls, in five hand-written groups |
| `cmd/agt/help_data.go` | 71 `commandHelp{name, summary, detail}` entries, in nine help groups |
| `cmd/agt/main.go`'s root dispatch | a `switch args[0]` that answered "version" and "help" before the registry ever saw them |

Two tests kept the first two copies in step (`TestHelp_CoversEveryRegisteredCommand`,
`TestHelp_DocumentsOnlyRegisteredCommands`) — which is a **guard, not a single
source**: a name had to be written twice and kept in step by hand.

The measurement that started this slice found no drift between the two copies —
**71 registered names ↔ 71 help entries, one for one** — but it found three
things that the duplication had made possible:

1. **The root dispatch kept a third list.** `main.go`'s `run()` switched on
   `args[0]` to answer `-v`, `--version`, `version`, `-h`, `--help` and `help`
   before calling `ExecuteCommand`.
2. **The registered `version` command was a no-op.** `cmd_register.go` registered
   `version` with a body of `return 0` — the root dispatch answered it, so the
   registered body never ran. `agt version` printed the version because the
   *dispatcher* printed it, not the command.
3. **The router carried three fields nobody read.** `router.Command.Description`
   was written for 4 of 71 commands and read by nothing; `HelpLong` was never
   written and never read; `HelpHandler` was read by one test that only checked
   it for `nil`.

## 2. What moved

The 71 top-level commands now live in **one table**, `cmd/agt/commands_top.go`
(854 lines):

```
71 commands · 9 help groups · 1 runner each
```

One command, as it is written there:

```go
{
	name:    "standing",
	group:   "Plans & automation",
	summary: "durable wake rules for agents",
	detail: []string{
		"standing " + cliStanding.synopsis(),
	},
	run: cmdStanding,
},
```

- `name` — the canonical name (`agt run`).
- `aliases` — the other names the command answers to.
- `group` — the help overview's section.
- `summary` — the one-line overview.
- `detail` — the usage block `agt help <command>` prints. A line that names a
  domain's sub-commands is **derived** from that domain's own table
  (`cmd/agt/commands_dict.go`), not copied: `"standing " + cliStanding.synopsis()`.
- `run` — the runner.

The registry and the help overview are the same declaration read two ways:

```go
func init() {
	fillDomains()
	cliTopCommands = []topCommand{ /* … */ }
	for _, command := range cliTopCommands {
		Register(&Command{
			Name:    command.name,
			Aliases: command.aliases,
			Run:     command.run,
		})
	}
}
```

```go
func helpGroups() []helpGroup { /* derived from cliTopCommands */ }
```

**The order is explicit.** The table is filled in `init()`, not in a var
initializer: it names `cmdHelp` as one of its runners, `cmdHelp` reads
`helpGroups()`, and `helpGroups()` reads the table — a var initializer would be
an initialization cycle. `init()` calls `fillDomains()` first, because the usage
lines that name a domain's sub-commands are derived from the filled domain
tables; that order is a statement, not a matter of which file the compiler saw
first.

## 3. What is gone

- **`cmd/agt/cmd_register.go`** (162 lines) — 71 registrations.
- **`cmd/agt/help_data.go`** (357 lines) — 71 help entries and the
  `commandHelp`/`helpGroup` types (the types moved to `help.go`).
- **`main.go`'s root `switch args[0]`** — the third copy of the names.
- **`router.Command.Description`, `.HelpLong`, `.HelpHandler`** — three fields
  nobody read.

And what replaced them:

- **`cmd/agt/version.go`** — `cmdVersion` prints the client's identity:
  `agt 1.1.0 (protocol v1)`. The same bytes the root dispatch printed.
- **`cmd/agt/commands_top.go`** — the table.

The root dispatch is now two lines:

```go
if len(args) >= 2 && (args[1] == "-h" || args[1] == "--help") && helpHas(args[0]) {
	return cmdHelp(args[:1], stdout, stderr)
}
return ExecuteCommand(args[0], args[1:], stdout, stderr)
```

## 4. The two kinds of usage line

A top-level command's usage block mixes two kinds of line:

| Kind | Example | Where it comes from |
|---|---|---|
| written out | `"doctor [--json] [--strict] [--repair]"` | the table |
| derived | `"standing " + cliStanding.synopsis()` | the domain's own table |

Twelve lines across eleven commands are derived (`agent`, `market` ×2, `mcp`,
`okr`, `provider chatgpt`, `skill workshop`, `standing`, `taste`, `toolforge`,
`workboard`, `workflow`). W3.80 had already derived these in the help table;
the derivation moved with the table.

The derivation reproduces W3.80's shape exactly — the angle brackets the
hand-written copy carried are gone, because the derived line is
`"standing " + cliStanding.synopsis()`, not `"standing <…>"`. The names are
W3.80's, so the derived lists are the corrected ones (`agt standing` names
`edit` and `why`, which the stale copy had outgrown).

## 5. The guard

`cmd/agt/commands_top_ratchet_test.go` holds six tests:

1. **`TestTopLevelVocabularyHasOneHome`** — no file registers a command outside
   the table, and no file switches on `args[0]`.
2. **`TestTopLevelNamesAreUnique`** — no name is answered by two commands (the
   canonical names and the aliases together).
3. **`TestEveryTopLevelCommandIsComplete`** — every command has a runner, a
   summary, a usage block and a group the help overview lists.
4. **`TestTheRegistryIsTheTable`** — the registry holds the table's names and
   aliases, and nothing else.
5. **`TestHelpOverviewIsDerivedFromTheTable`** — the help overview lists every
   command of the table exactly once, with the table's summary and usage block.
6. **`TestTheRootDispatcherKeepsNoList`** / **`TestTheHelpOverviewGroupsAreOrdered`**
   — the root dispatch keeps no list; the group order is written down once.

**The ratchet was proved to fire.** Two mutations, each reverted:

| Mutation | Expected | Result |
|---|---|---|
| a second registration outside the table (`cmd/agt/zz_mutation.go`: `Register(&Command{Name: "bogus", …})`) | test 1 fails | failed: *"zz_mutation.go registers a command outside the top-level vocabulary table; the table (commands_top.go) is the only place a top-level name is written"* |
| the `doctor` command given the alias `status` | test 2 fails | failed: *"the name "status" is answered by two commands: "doctor" and "status""* |

The first mutation is a second copy of the vocabulary; the second is one name
claimed twice. Neither is visible in the table's own contents — only the
repository can see them.

## 6. The measurement that moved, and the one that did not

**Moved:**

| Measurement | Before | After |
|---|---:|---:|
| places the top-level names are written | 3 | **1** |
| top-level commands registered | 71 | 71 |
| help entries | 71 | 71 |
| files holding the top-level vocabulary | 2 (+ the root switch) | **1** |
| `router.Command` fields | 6 | **3** |
| root `switch args[0]` statements in `cmd/agt` | 1 | **0** |
| hand-written copies of a domain's sub-command list in the help table | 0 (W3.80) | 0 |

**Did not move:** the command names, the aliases, the exit codes, the help
output's bytes and the wire protocol.

## 7. What is out of scope

- **`cmd/agt/keys`** is a separate package with its own vocabulary
  (`list|add|activate|rm`); it was not touched.
- **The domain vocabulary** (46 domains, 311 sub-commands) is W3.80's slice;
  this slice only *reads* it, to derive the usage lines that name a domain's
  sub-commands.
- **The Web UI's route tables** still spell the operation names out by hand —
  the next slice of the vocabulary programme.

## 8. The gates

All thirteen gates are green at `3dc1ac5d` + W3.79 + W3.80 + this slice:

```
gofmt -l kernel tools cmd plugins contract   → clean
go build ./...                                → clean
go vet ./...                                  → clean
go test ./...                                 → ok
go run ./tools/archcheck                      → 339 packages, 1 allowlisted import violation, 0 call sites
go run ./tools/deadcodecheck                  → OK
go run ./tools/depscheck                      → OK, 24 dependencies
go run ./tools/sdkparity -check docs/SDK-PARITY.md → ok
go run ./tools/structure-md -check            → ok
go run ./tools/changelog-lint                 → OK
go run ./tools/docclaimscheck -base origin/main → OK
go run ./tools/docsindex                      → OK
staticcheck ./...                             → clean (GOTOOLCHAIN=go1.27.1)
```
