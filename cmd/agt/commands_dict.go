// SPDX-License-Identifier: MIT

package main

// Provenance: the cmd/agt command vocabulary. Every domain's sub-commands are
// declared once, in one table, and the dispatchers look the table up instead
// of writing the names out again.
//
// A name that can be written in two places is a name that can drift. The
// dispatcher's switch, the usage line, the unknown-sub-command message and
// the help table all used to carry the same list of names; they disagreed
// with each other often enough to matter. Now the table is the only place a
// sub-command name is written, and everything else is derived from it.

import (
	"bufio"
	"fmt"
	"io"
	"os"
	"strings"

	"github.com/agezt/agezt/cmd/agt/keys"
	"github.com/agezt/agezt/internal/brand"
	"github.com/agezt/agezt/kernel/contract/opapi"
)

// cliRunner runs one sub-command. args[0] is the sub-command name.
type cliRunner func(stdin io.Reader, args []string, stdout, stderr io.Writer) int

// cliCommand is one sub-command of one domain.
type cliCommand struct {
	// names are every name the command answers to. names[0] is the canonical
	// one; the rest are aliases.
	names []string
	// ops are the catalogue operations the command can drive. The empty list
	// means the command drives no operation (it prints help, or it works
	// entirely on the client).
	ops []string
	// run executes the command.
	run cliRunner
	// hidden keeps a command out of the synopsis: the help aliases answer to
	// every domain but are not part of its vocabulary.
	hidden bool
}

// cliDomain is one command domain: `agt <domain> <sub-command> …`.
type cliDomain struct {
	name     string
	commands []cliCommand
}

// lookup finds the command that answers to name.
func (d *cliDomain) lookup(name string) (cliCommand, bool) {
	for _, command := range d.commands {
		for _, candidate := range command.names {
			if candidate == name {
				return command, true
			}
		}
	}
	return cliCommand{}, false
}

// synopsis lists the domain's canonical names: "list|add|show".
func (d *cliDomain) synopsis() string {
	return strings.Join(d.canonical(), "|")
}

// listNames lists the domain's canonical names: "list, add, show".
func (d *cliDomain) listNames() string {
	return strings.Join(d.canonical(), ", ")
}

// canonical returns the canonical name of every command in the domain.
func (d *cliDomain) canonical() []string {
	names := make([]string, 0, len(d.commands))
	for _, command := range d.commands {
		if command.hidden || len(command.names) == 0 {
			continue
		}
		names = append(names, command.names[0])
	}
	return names
}

// shifted hands the handler the arguments after the sub-command name.
func shifted(handler func([]string, io.Writer, io.Writer) int) cliRunner {
	return func(stdin io.Reader, args []string, stdout, stderr io.Writer) int {
		return handler(args[1:], stdout, stderr)
	}
}

// direct hands the handler the whole argument list, sub-command name included.
func direct(handler func([]string, io.Writer, io.Writer) int) cliRunner {
	return func(stdin io.Reader, args []string, stdout, stderr io.Writer) int {
		return handler(args, stdout, stderr)
	}
}

// cliAgent is the command vocabulary of `agt agent`.
var cliAgent cliDomain

// cliArtifact is the command vocabulary of `agt artifact`.
var cliArtifact cliDomain

// cliCatalog is the command vocabulary of `agt catalog`.
var cliCatalog cliDomain

// cliCompare is the command vocabulary of `agt compare`.
var cliCompare cliDomain

// cliConfig is the command vocabulary of `agt config`.
var cliConfig cliDomain

// cliConfigCenter is the command vocabulary of `agt configcenter`.
var cliConfigCenter cliDomain

// cliConfigSchema is the command vocabulary of `agt config`.
var cliConfigSchema cliDomain

// cliEdict is the command vocabulary of `agt edict`.
var cliEdict cliDomain

// cliEdictDeny is the command vocabulary of `agt edict`.
var cliEdictDeny cliDomain

// cliExecProfile is the command vocabulary of `agt execprofile`.
var cliExecProfile cliDomain

// cliJournal is the command vocabulary of `agt journal`.
var cliJournal cliDomain

// cliMCP is the command vocabulary of `agt mcp`.
var cliMCP cliDomain

// cliMarket is the command vocabulary of `agt market`.
var cliMarket cliDomain

// cliMemory is the command vocabulary of `agt memory`.
var cliMemory cliDomain

// cliNetguard is the command vocabulary of `agt netguard`.
var cliNetguard cliDomain

// cliOKR is the command vocabulary of `agt okr`.
var cliOKR cliDomain

// cliPlan is the command vocabulary of `agt plan`.
var cliPlan cliDomain

// cliPlugin is the command vocabulary of `agt plugin`.
var cliPlugin cliDomain

// cliProvider is the command vocabulary of `agt provider`.
var cliProvider cliDomain

// cliProviderChatGPT is the command vocabulary of `agt providerchatgpt`.
var cliProviderChatGPT cliDomain

// cliProviderCreds is the command vocabulary of `agt provider creds`.
var cliProviderCreds cliDomain

// cliPulse is the command vocabulary of `agt pulse`.
var cliPulse cliDomain

// cliRateLimit is the command vocabulary of `agt ratelimit`.
var cliRateLimit cliDomain

// cliRedact is the command vocabulary of `agt redact`.
var cliRedact cliDomain

// cliReflect is the command vocabulary of `agt reflect`.
var cliReflect cliDomain

// cliRollback is the command vocabulary of `agt rollback`.
var cliRollback cliDomain

// cliRuns is the command vocabulary of `agt runs`.
var cliRuns cliDomain

// cliSchedule is the command vocabulary of `agt schedule`.
var cliSchedule cliDomain

// cliSeat is the command vocabulary of `agt seats`.
var cliSeat cliDomain

// cliSkill is the command vocabulary of `agt skill`.
var cliSkill cliDomain

// cliSkillWorkshop is the command vocabulary of `agt skillworkshop`.
var cliSkillWorkshop cliDomain

// cliStanding is the command vocabulary of `agt standing`.
var cliStanding cliDomain

// cliState is the command vocabulary of `agt state`.
var cliState cliDomain

// cliTaste is the command vocabulary of `agt taste`.
var cliTaste cliDomain

// cliTenant is the command vocabulary of `agt tenant`.
var cliTenant cliDomain

// cliToken is the command vocabulary of `agt token`.
var cliToken cliDomain

// cliTool is the command vocabulary of `agt tool`.
var cliTool cliDomain

// cliToolforge is the command vocabulary of `agt toolforge`.
var cliToolforge cliDomain

// cliVault is the command vocabulary of `agt vault`.
var cliVault cliDomain

// cliWarden is the command vocabulary of `agt warden`.
var cliWarden cliDomain

// cliWeb is the command vocabulary of `agt web`.
var cliWeb cliDomain

// cliWebPassword is the command vocabulary of `agt web`.
var cliWebPassword cliDomain

// cliWebhook is the command vocabulary of `agt webhook`.
var cliWebhook cliDomain

// cliWorkboard is the command vocabulary of `agt workboard`.
var cliWorkboard cliDomain

// cliWorkflow is the command vocabulary of `agt workflow`.
var cliWorkflow cliDomain

// cliWorld is the command vocabulary of `agt world`.
var cliWorld cliDomain

// fillDomains fills every domain table.
//
// The domains are filled in a function, not in a declaration, so that a
// usage function can read its own domain back without an initialization
// cycle. The top-level vocabulary table (cmd/agt/commands_top.go) calls this
// before it builds its own table: the usage lines that name a domain's
// sub-commands are derived from the filled tables, and the order has to be
// explicit rather than a matter of which file the compiler saw first.
func fillDomains() {
	cliAgent = cliDomain{
		name: "agent",
		commands: []cliCommand{
			{
				names: []string{"list"},
				ops:   []string{opapi.AgentList},
				run:   shifted(cmdAgentList),
			},
			{
				names: []string{"add", "create"},
				ops:   []string{opapi.AgentAdd},
				run:   shifted(cmdAgentAdd),
			},
			{
				names: []string{"show"},
				ops:   []string{opapi.AgentList, opapi.ApprovalsLog},
				run:   shifted(cmdAgentShow),
			},
			{
				names: []string{"authority"},
				ops:   []string{opapi.AgentList, opapi.EdictShow},
				run:   shifted(cmdAgentAuthority),
			},
			{
				names: []string{"impact"},
				ops:   []string{opapi.AgentImpact},
				run:   shifted(cmdAgentImpact),
			},
			{
				names: []string{"tombstone"},
				ops:   []string{opapi.AgentTombstone},
				run:   shifted(cmdAgentTombstone),
			},
			{
				names: []string{"graveyard"},
				ops:   []string{opapi.AgentGraveyard},
				run:   shifted(cmdAgentGraveyard),
			},
			{
				names: []string{"set", "edit"},
				ops:   []string{opapi.AgentEdit, opapi.AgentList},
				run:   shifted(cmdAgentSet),
			},
			{
				names: []string{"task", "tasks"},
				ops:   []string{opapi.AgentTaskUpdate},
				run:   shifted(cmdAgentTask),
			},
			{
				names: []string{"wake"},
				ops:   []string{opapi.AgentWake},
				run:   shifted(cmdAgentWake),
			},
			{
				names: []string{"repair"},
				ops:   []string{opapi.AgentRepair},
				run:   shifted(cmdAgentRepair),
			},
			{
				names: []string{"repair-status"},
				ops:   []string{opapi.AgentRepairStatus},
				run:   shifted(cmdAgentRepairStatus),
			},
			{
				names: []string{"pause"},
				ops:   []string{opapi.AgentSetEnabled},
				run: func(stdin io.Reader, args []string, stdout, stderr io.Writer) int {
					return cmdAgentSetEnabled(args[1:], stdout, stderr, false)
				},
			},
			{
				names: []string{"resume"},
				ops:   []string{opapi.AgentSetEnabled},
				run: func(stdin io.Reader, args []string, stdout, stderr io.Writer) int {
					return cmdAgentSetEnabled(args[1:], stdout, stderr, true)
				},
			},
			{
				names: []string{"retire"},
				ops:   []string{opapi.AgentRetire},
				run:   shifted(cmdAgentRetire),
			},
			{
				names: []string{"revive"},
				ops:   []string{opapi.AgentRevive},
				run:   shifted(cmdAgentRevive),
			},
			{
				names: []string{"remove", "rm"},
				ops:   []string{opapi.AgentImpact, opapi.AgentRemove},
				run:   shifted(cmdAgentRemove),
			},
			{
				names:  []string{"-h", "--help", "help"},
				hidden: true,
				run: func(stdin io.Reader, args []string, stdout, stderr io.Writer) int {
					return agentUsage(stdout)
				},
			},
		},
	}
	cliArtifact = cliDomain{
		name: "artifact",
		commands: []cliCommand{
			{
				names: []string{"get"},
				ops:   []string{opapi.ArtifactGet},
				run:   shifted(cmdArtifactGet),
			},
			{
				names:  []string{"-h", "--help"},
				hidden: true,
				run: func(stdin io.Reader, args []string, stdout, stderr io.Writer) int {
					fmt.Fprintf(stdout, "usage: %s artifact get <ref> [--out <file>]\n", brand.CLI)
					fmt.Fprintf(stdout, "fetch a content-addressed tool output offloaded from the journal (its raw_ref)\n")
					return 0
				},
			},
		},
	}
	cliCatalog = cliDomain{
		name: "catalog",
		commands: []cliCommand{
			{
				names: []string{"sync"},
				ops:   []string{opapi.CatalogSync},
				run:   shifted(cmdCatalogSync),
			},
			{
				names: []string{"discover"},
				run: func(stdin io.Reader, args []string, stdout, stderr io.Writer) int {
					callArgs := map[string]any{}
					if len(args) > 1 {
						callArgs["endpoint"] = args[1]
					}
					return cmdSimple(opapi.CatalogDiscover, callArgs, stdout, stderr)
				},
			},
			{
				names: []string{"list"},
				ops:   []string{opapi.CatalogList},
				run:   shifted(cmdCatalogList),
			},
		},
	}
	cliCompare = cliDomain{
		name: "compare",
		commands: []cliCommand{
			{
				names: []string{"audit"},
				run:   shifted(cmdCompareAudit),
			},
			{
				names:  []string{"-h", "--help", "help"},
				hidden: true,
				run: func(stdin io.Reader, args []string, stdout, stderr io.Writer) int {
					fmt.Fprintf(stdout, "usage: %s compare <subcommand>\n", brand.CLI)
					fmt.Fprintf(stdout, "  audit [--target openclaw|hermes|all] [--root <repo>] [--json]\n")
					fmt.Fprintf(stdout, "read-only local capability ledger for OpenClaw/Hermes parity claims\n")
					return 0
				},
			},
		},
	}
	cliConfig = cliDomain{
		name: "config",
		commands: []cliCommand{
			{
				names: []string{"show"},
				ops:   []string{opapi.Config},
				run:   shifted(cmdConfigShow),
			},
			{
				names: []string{"ls"},
				ops:   []string{opapi.ConfigValues},
				run:   shifted(cmdConfigLs),
			},
			{
				names: []string{"get"},
				ops:   []string{opapi.ConfigValues},
				run:   shifted(cmdConfigGet),
			},
			{
				names: []string{"set"},
				ops:   []string{opapi.ConfigSet, opapi.ConfigValues},
				run:   shifted(cmdConfigSet),
			},
			{
				names: []string{"schema"},
				ops:   []string{opapi.ConfigSchema, opapi.ConfigSchemaRegister, opapi.ConfigSchemaUnregister},
				run:   shifted(cmdConfigSchema),
			},
			{
				names:  []string{"-h", "--help"},
				hidden: true,
				run: func(stdin io.Reader, args []string, stdout, stderr io.Writer) int {
					fmt.Fprintf(stdout, "usage: %s config <subcommand>\n\n", brand.CLI)
					fmt.Fprintf(stdout, "  show [--json]            resolved config (paths, model, env presence)\n")
					fmt.Fprintf(stdout, "  ls [--json]              every Config Center setting + its state\n")
					fmt.Fprintf(stdout, "  get <ENV>                one setting's value (secrets: presence only)\n")
					fmt.Fprintf(stdout, "  set <ENV> <value>        write a setting (live for provider/model, else restart)\n")
					fmt.Fprintf(stdout, "  schema [--json]          list the editable schema (built-in + registered)\n")
					fmt.Fprintf(stdout, "  schema register <file>   register a skill/plugin schema section (JSON)\n")
					fmt.Fprintf(stdout, "  schema unregister <id>   remove a registered schema section\n")
					return 0
				},
			},
		},
	}
	cliConfigCenter = cliDomain{
		name: "configcenter",
		commands: []cliCommand{
			{
				names: []string{"set"},
				ops:   []string{opapi.ConfigCenterSet},
				run:   shifted(cmdConfigCenterSet),
			},
			{
				names: []string{"get"},
				ops:   []string{opapi.ConfigCenterGet},
				run:   shifted(cmdConfigCenterGet),
			},
			{
				names: []string{"list"},
				ops:   []string{opapi.ConfigCenterList},
				run:   shifted(cmdConfigCenterList),
			},
			{
				names: []string{"delete"},
				ops:   []string{opapi.ConfigCenterDelete},
				run:   shifted(cmdConfigCenterDelete),
			},
			{
				names: []string{"rating"},
				ops:   []string{opapi.ConfigCenterGet, opapi.ConfigCenterSetRating},
				run:   shifted(cmdConfigCenterRating),
			},
			{
				names: []string{"access-log"},
				ops:   []string{opapi.ConfigCenterAccessLog},
				run:   shifted(cmdConfigCenterAccessLog),
			},
			{
				names: []string{"audit"},
				ops:   []string{opapi.ConfigCenterAudit},
				run:   shifted(cmdConfigCenterAudit),
			},
			{
				names: []string{"health"},
				ops:   []string{opapi.ConfigCenterHealth},
				run:   shifted(cmdConfigCenterHealth),
			},
			{
				names:  []string{"-h", "--help"},
				hidden: true,
				run: func(stdin io.Reader, args []string, stdout, stderr io.Writer) int {
					return cmdConfigCenterHelp(stdout)
				},
			},
		},
	}
	cliConfigSchema = cliDomain{
		name: "config",
		commands: []cliCommand{
			{
				names: []string{"register"},
				ops:   []string{opapi.ConfigSchemaRegister},
				run:   shifted(cmdConfigSchemaRegister),
			},
			{
				names: []string{"unregister"},
				ops:   []string{opapi.ConfigSchemaUnregister},
				run:   shifted(cmdConfigSchemaUnregister),
			},
		},
	}
	cliEdict = cliDomain{
		name: "edict",
		commands: []cliCommand{
			{
				names: []string{"show"},
				ops:   []string{opapi.EdictShow},
				run:   shifted(cmdEdictShow),
			},
			{
				names: []string{"overlay"},
				ops:   []string{opapi.EdictOverlay},
				run:   shifted(cmdEdictOverlay),
			},
			{
				names: []string{"compact"},
				ops:   []string{opapi.EdictCompact},
				run:   shifted(cmdEdictCompact),
			},
			{
				names: []string{"test"},
				ops:   []string{opapi.EdictTest},
				run:   shifted(cmdEdictTest),
			},
			{
				names: []string{"deny"},
				ops:   []string{opapi.EdictDenyAdd, opapi.EdictDenyList, opapi.EdictDenyRemove},
				run:   shifted(cmdEdictDeny),
			},
			{
				names: []string{"level"},
				ops:   []string{opapi.EdictSetLevel},
				run:   shifted(cmdEdictLevel),
			},
			{
				names: []string{"mode"},
				ops:   []string{opapi.EdictSetMode},
				run:   shifted(cmdEdictMode),
			},
			{
				names: []string{"log"},
				ops:   []string{opapi.EdictLog},
				run:   shifted(cmdEdictLog),
			},
			{
				names: []string{"stats"},
				ops:   []string{opapi.EdictStats},
				run:   shifted(cmdEdictStats),
			},
			{
				names:  []string{"-h", "--help", "help"},
				hidden: true,
				run: func(stdin io.Reader, args []string, stdout, stderr io.Writer) int {
					fmt.Fprintf(stdout, "usage: %s edict <subcommand>\n", brand.CLI)
					fmt.Fprintf(stdout, "  show [--json]                          display loaded policies\n")
					fmt.Fprintf(stdout, "  overlay [--json]                       net runtime policy overlay (level/mode/deny changes in effect)\n")
					fmt.Fprintf(stdout, "  compact [--json]                       snapshot the overlay so boot replays it + later changes only\n")
					fmt.Fprintf(stdout, "  log [N] [--denied] [--json]            recent policy decisions (allow/deny audit)\n")
					fmt.Fprintf(stdout, "  stats [--since <dur>] [--json]         policy-decision aggregate (denial rate, by capability)\n")
					fmt.Fprintf(stdout, "  test <capability> [<input>] [--json]   dry-run a decision; no side effects\n")
					fmt.Fprintf(stdout, "  deny list|add|rm ...                   manage hard-deny rules at runtime\n")
					fmt.Fprintf(stdout, "  level <capability> <level> [--json]    set a capability's trust level at runtime\n")
					fmt.Fprintf(stdout, "  mode <allow|deny|prompt> [--json]      set the approval mode at runtime\n")
					return 0
				},
			},
		},
	}
	cliEdictDeny = cliDomain{
		name: "edict",
		commands: []cliCommand{
			{
				names: []string{"list"},
				ops:   []string{opapi.EdictDenyList},
				run:   shifted(cmdEdictDenyList),
			},
			{
				names: []string{"add"},
				ops:   []string{opapi.EdictDenyAdd},
				run:   shifted(cmdEdictDenyAdd),
			},
			{
				names: []string{"rm", "remove"},
				ops:   []string{opapi.EdictDenyRemove},
				run:   shifted(cmdEdictDenyRemove),
			},
			{
				names:  []string{"-h", "--help", "help"},
				hidden: true,
				run: func(stdin io.Reader, args []string, stdout, stderr io.Writer) int {
					fmt.Fprintf(stdout, "usage: %s edict deny <subcommand>\n", brand.CLI)
					fmt.Fprintf(stdout, "  list [--json]            list hard-deny rules (with removable flag)\n")
					fmt.Fprintf(stdout, "  add <rule> [--json]      add a rule: \"substring\" or \"<cap>:substring\"\n")
					fmt.Fprintf(stdout, "  rm <name> [--json]       remove a runtime-added rule by name\n")
					return 0
				},
			},
		},
	}
	cliExecProfile = cliDomain{
		name: "execprofile",
		commands: []cliCommand{
			{
				names: []string{"list", "ls"},
				ops:   []string{opapi.ExecutionProfiles},
				run:   shifted(cmdExecProfileList),
			},
			{
				names: []string{"show"},
				ops:   []string{opapi.ExecutionProfileShow},
				run:   shifted(cmdExecProfileShow),
			},
			{
				names: []string{"check", "doctor"},
				ops:   []string{opapi.ExecutionProfileCheck},
				run:   shifted(cmdExecProfileCheck),
			},
		},
	}
	cliJournal = cliDomain{
		name: "journal",
		commands: []cliCommand{
			{
				names: []string{"verify"},
				ops:   []string{opapi.JournalVerify},
				run:   shifted(cmdJournalVerify),
			},
			{
				names: []string{"tail"},
				ops:   []string{opapi.JournalTail},
				run:   shifted(cmdJournalTail),
			},
			{
				names: []string{"grep"},
				ops:   []string{opapi.JournalGrep},
				run:   shifted(cmdJournalGrep),
			},
			{
				names: []string{"head"},
				ops:   []string{opapi.JournalHead},
				run:   shifted(cmdJournalHead),
			},
			{
				names: []string{"export"},
				ops:   []string{opapi.JournalExport},
				run:   shifted(cmdJournalExport),
			},
			{
				names: []string{"import"},
				run:   shifted(cmdJournalImport),
			},
			{
				names: []string{"stats"},
				ops:   []string{opapi.JournalStats},
				run:   shifted(cmdJournalStats),
			},
		},
	}
	cliMCP = cliDomain{
		name: "mcp",
		commands: []cliCommand{
			{
				names: []string{"list"},
				ops:   []string{opapi.MCPList},
				run:   shifted(cmdMCPList),
			},
			{
				names: []string{"add", "register"},
				ops:   []string{opapi.MCPAdd},
				run:   shifted(cmdMCPAdd),
			},
			{
				names: []string{"attach"},
				ops:   []string{opapi.MCPAttach},
				run: func(stdin io.Reader, args []string, stdout, stderr io.Writer) int {
					return cmdMCPRefAction(args[1:], stdout, stderr, "attach")
				},
			},
			{
				names: []string{"detach"},
				ops:   []string{opapi.MCPAttach, opapi.MCPDetach, opapi.MCPRemove},
				run: func(stdin io.Reader, args []string, stdout, stderr io.Writer) int {
					return cmdMCPRefAction(args[1:], stdout, stderr, "detach")
				},
			},
			{
				names: []string{"enable"},
				ops:   []string{opapi.MCPSetEnabled},
				run: func(stdin io.Reader, args []string, stdout, stderr io.Writer) int {
					return cmdMCPSetEnabled(args[1:], stdout, stderr, true)
				},
			},
			{
				names: []string{"disable"},
				ops:   []string{opapi.MCPSetEnabled},
				run: func(stdin io.Reader, args []string, stdout, stderr io.Writer) int {
					return cmdMCPSetEnabled(args[1:], stdout, stderr, false)
				},
			},
			{
				names: []string{"remove", "rm"},
				ops:   []string{opapi.MCPRemove},
				run: func(stdin io.Reader, args []string, stdout, stderr io.Writer) int {
					return cmdMCPRefAction(args[1:], stdout, stderr, "remove")
				},
			},
			{
				names:  []string{"-h", "--help", "help"},
				hidden: true,
				run: func(stdin io.Reader, args []string, stdout, stderr io.Writer) int {
					return mcpUsage(stdout)
				},
			},
		},
	}
	cliMarket = cliDomain{
		name: "market",
		commands: []cliCommand{
			{
				names: []string{"list", "ls"},
				ops:   []string{opapi.MarketList},
				run: func(stdin io.Reader, args []string, stdout, stderr io.Writer) int {
					return cmdMarketList(args[1:], "", stdout, stderr)
				},
			},
			{
				names: []string{"search"},
				run: func(stdin io.Reader, args []string, stdout, stderr io.Writer) int {
					if len(args) < 2 {
						fmt.Fprintf(stderr, "%s market search: a query is required\n", brand.CLI)
						return 2
					}
					return cmdMarketList(args[2:], args[1], stdout, stderr)
				},
			},
			{
				names: []string{"show", "get"},
				ops:   []string{opapi.MarketShow},
				run:   shifted(cmdMarketShow),
			},
			{
				names: []string{"install"},
				ops:   []string{opapi.MarketInstall},
				run:   shifted(cmdMarketInstall),
			},
			{
				names: []string{"uninstall"},
				ops:   []string{opapi.MarketUninstall},
				run:   shifted(cmdMarketUninstall),
			},
			{
				names: []string{"sources"},
				ops:   []string{opapi.MarketSources},
				run:   shifted(cmdMarketSources),
			},
			{
				names: []string{"add"},
				ops:   []string{opapi.MarketAddSource},
				run:   shifted(cmdMarketAddSource),
			},
			{
				names: []string{"remove", "rm"},
				ops:   []string{opapi.MarketRemoveSource},
				run:   shifted(cmdMarketRemoveSource),
			},
			{
				names: []string{"sync"},
				ops:   []string{opapi.MarketSync},
				run:   shifted(cmdMarketSync),
			},
			{
				names: []string{"validate"},
				run:   shifted(cmdMarketValidate),
			},
			{
				names: []string{"publish"},
				run:   shifted(cmdMarketPublish),
			},
			{
				names: []string{"keygen"},
				run:   shifted(cmdMarketKeygen),
			},
			{
				names:  []string{"-h", "--help", "help"},
				hidden: true,
				run: func(stdin io.Reader, args []string, stdout, stderr io.Writer) int {
					fmt.Fprintf(stdout, "usage: %s market <subcommand>\n", brand.CLI)
					fmt.Fprintf(stdout, "  list [--json]                  browse the catalogue + install state\n")
					fmt.Fprintf(stdout, "  search <query> [--json]        filter packs by name/description/tag\n")
					fmt.Fprintf(stdout, "  show <pack> [--json]           a pack's contents (skills, MCP, tools)\n")
					fmt.Fprintf(stdout, "  install <pack> [--marketplace m] [--version v] [--json]   materialize a pack\n")
					fmt.Fprintf(stdout, "  uninstall <pack> [--json]      reverse a pack's footprint\n")
					fmt.Fprintf(stdout, "  sources [--json]               list configured remote marketplaces\n")
					fmt.Fprintf(stdout, "  add <url> [--name n] [--pubkey hex] [--json]   register a remote source\n")
					fmt.Fprintf(stdout, "  remove <source> [--json]       drop a source + its cached catalogue\n")
					fmt.Fprintf(stdout, "  sync [source] [--json]         fetch a source (or all) into the local cache\n")
					fmt.Fprintf(stdout, "  validate <dir>                 compile + validate a pack authoring dir (pack.json)\n")
					fmt.Fprintf(stdout, "  publish <dir> --out <dir> [--key <file>] [--name N]   build (+sign) a pack into a hostable marketplace\n")
					fmt.Fprintf(stdout, "  keygen [--out <prefix>]        generate an Ed25519 signing keypair\n")
					return 0
				},
			},
		},
	}
	cliMemory = cliDomain{
		name: "memory",
		commands: []cliCommand{
			{
				names: []string{"add"},
				ops:   []string{opapi.MemoryAdd},
				run:   shifted(cmdMemoryAdd),
			},
			{
				names: []string{"list", "ls"},
				ops:   []string{opapi.MemoryList},
				run:   shifted(cmdMemoryList),
			},
			{
				names: []string{"log"},
				ops:   []string{opapi.MemoryLog},
				run:   shifted(cmdMemoryLog),
			},
			{
				names: []string{"search"},
				ops:   []string{opapi.MemorySearch},
				run:   shifted(cmdMemorySearch),
			},
			{
				names: []string{"get"},
				ops:   []string{opapi.MemoryGet},
				run:   shifted(cmdMemoryGet),
			},
			{
				names: []string{"forget", "rm"},
				ops:   []string{opapi.MemoryForget},
				run:   shifted(cmdMemoryForget),
			},
			{
				names: []string{"promote"},
				ops:   []string{opapi.MemoryPromote},
				run:   shifted(cmdMemoryPromote),
			},
			{
				names: []string{"audit"},
				ops:   []string{opapi.MemoryAudit},
				run:   shifted(cmdMemoryAudit),
			},
			{
				names: []string{"clean"},
				ops:   []string{opapi.MemoryClean},
				run:   shifted(cmdMemoryClean),
			},
			{
				names: []string{"consolidate"},
				ops:   []string{opapi.MemoryConsolidate},
				run:   shifted(cmdMemoryConsolidate),
			},
			{
				names: []string{"profile"},
				ops:   []string{opapi.ProfileRebuild},
				run:   shifted(cmdMemoryProfile),
			},
			{
				names: []string{"prune"},
				ops:   []string{opapi.MemoryPrune},
				run:   shifted(cmdMemoryPrune),
			},
			{
				names: []string{"bulk-forget"},
				ops:   []string{opapi.MemoryBulkForget},
				run:   shifted(cmdMemoryBulkForget),
			},
			{
				names: []string{"find-related"},
				ops:   []string{opapi.MemoryFindRelated},
				run:   shifted(cmdMemoryFindRelated),
			},
			{
				names:  []string{"-h", "--help", "help"},
				hidden: true,
				run: func(stdin io.Reader, args []string, stdout, stderr io.Writer) int {
					fmt.Fprintf(stdout, "usage: %s memory <subcommand>\n", brand.CLI)
					fmt.Fprintf(stdout, "  add <subject> <content> [--type T] [--tag k=v] [--conf F] [--json]\n")
					fmt.Fprintf(stdout, "  list [--json]\n")
					fmt.Fprintf(stdout, "  log [N] [--op written|forgotten|superseded|promoted] [--since <dur>] [--json]\n")
					fmt.Fprintf(stdout, "  search <query> [N] [--json]\n")
					fmt.Fprintf(stdout, "  get <id> [--json]      (exit 3 = absent)\n")
					fmt.Fprintf(stdout, "  forget <id> [--json]\n")
					fmt.Fprintf(stdout, "  promote <id> [--json]  share a private (agent-scoped) record with every agent\n")
					fmt.Fprintf(stdout, "  audit [--json]         report expired, suspended, and competing memories\n")
					fmt.Fprintf(stdout, "  clean [--execute]      hard-delete low-value log/transient memory records (dry-run by default)\n")
					fmt.Fprintf(stdout, "  consolidate [--json]   one brain-distillation pass: merge related records, supersede originals\n")
					fmt.Fprintf(stdout, "  profile [--json]       rebuild the operator profile from accumulated memory (M1000)\n")
					fmt.Fprintf(stdout, "  prune [--days N] [--execute]   hard-delete soft-deleted records (dry-run by default)\n")
					fmt.Fprintf(stdout, "  bulk-forget <id>...    soft-delete multiple records in one call\n")
					fmt.Fprintf(stdout, "  find-related --id <id> [--limit N] [--json]\n")
					return 0
				},
			},
		},
	}
	cliNetguard = cliDomain{
		name: "netguard",
		commands: []cliCommand{
			{
				names: []string{"test"},
				run:   shifted(cmdNetguardTest),
			},
			{
				names: []string{"log"},
				ops:   []string{opapi.NetguardLog},
				run:   shifted(cmdNetguardLog),
			},
		},
	}
	cliOKR = cliDomain{
		name: "okr",
		commands: []cliCommand{
			{
				names: []string{"list", "ls"},
				ops:   []string{opapi.OKRList},
				run:   shifted(cmdOKRList),
			},
			{
				names: []string{"show", "get"},
				ops:   []string{opapi.OKRShow},
				run:   shifted(cmdOKRShow),
			},
			{
				names: []string{"create", "add"},
				ops:   []string{opapi.OKRCreate},
				run:   shifted(cmdOKRCreate),
			},
			{
				names: []string{"kr", "keyresult"},
				ops:   []string{opapi.OKRKeyResult},
				run:   shifted(cmdOKRKeyResult),
			},
			{
				names: []string{"link"},
				ops:   []string{opapi.OKRLink},
				run: func(stdin io.Reader, args []string, stdout, stderr io.Writer) int {
					return cmdOKRLink(args[1:], stdout, stderr, opapi.OKRLink, "link")
				},
			},
			{
				names: []string{"unlink"},
				ops:   []string{opapi.OKRUnlink},
				run: func(stdin io.Reader, args []string, stdout, stderr io.Writer) int {
					return cmdOKRLink(args[1:], stdout, stderr, opapi.OKRUnlink, "unlink")
				},
			},
			{
				names: []string{"archive"},
				ops:   []string{opapi.OKRArchive},
				run:   shifted(cmdOKRArchive),
			},
			{
				names:  []string{"-h", "--help", "help"},
				hidden: true,
				run: func(stdin io.Reader, args []string, stdout, stderr io.Writer) int {
					return okrUsage(stdout)
				},
			},
		},
	}
	cliPlan = cliDomain{
		name: "plan",
		commands: []cliCommand{
			{
				names: []string{"generate", "gen"},
				ops:   []string{opapi.PlanGenerate},
				run:   shifted(cmdPlanGenerate),
			},
			{
				names: []string{"history", "runs", "ls"},
				ops:   []string{opapi.PlanHistory},
				run:   shifted(cmdPlanHistory),
			},
			{
				names: []string{"stats"},
				ops:   []string{opapi.PlanStats},
				run:   shifted(cmdPlanStats),
			},
			{
				names: []string{"run"},
				ops:   []string{opapi.Plan, opapi.PlanGenerate},
				run:   shifted(cmdPlanRun),
			},
			{
				names: []string{"cost"},
				run:   shifted(cmdPlanCost),
			},
			{
				names: []string{"refine"},
				ops:   []string{opapi.PlanRefine},
				run:   shifted(cmdPlanRefine),
			},
			{
				names: []string{"validate"},
				run:   shifted(cmdPlanValidate),
			},
			{
				names: []string{"visualize", "viz"},
				run:   shifted(cmdPlanVisualize),
			},
		},
	}
	cliPlugin = cliDomain{
		name: "plugin",
		commands: []cliCommand{
			{
				names: []string{"new"},
				run:   shifted(cmdPluginNew),
			},
			{
				names: []string{"hash"},
				run:   shifted(cmdPluginHash),
			},
			{
				names: []string{"list"},
				ops:   []string{opapi.PluginList},
				run:   shifted(cmdPluginList),
			},
			{
				names: []string{"registry"},
				run:   shifted(cmdPluginRegistry),
			},
			{
				names:  []string{"-h", "--help", "help"},
				hidden: true,
				run: func(stdin io.Reader, args []string, stdout, stderr io.Writer) int {
					fmt.Fprintf(stdout, "usage: %s plugin <subcommand>\n", brand.CLI)
					fmt.Fprintf(stdout, "\n")
					fmt.Fprintf(stdout, "  new <name>       scaffold a new Go tool plugin (SDK-based, buildable)\n")
					fmt.Fprintf(stdout, "  hash <path>      print the BLAKE3-256 hex digest of a plugin binary\n")
					fmt.Fprintf(stdout, "                   (use the output in AGEZT_PLUGIN_PINS=\"<prefix>=<hash>\")\n")
					fmt.Fprintf(stdout, "  list [--json]    list the external plugins the daemon spawned at startup\n")
					fmt.Fprintf(stdout, "  registry <dir|url> [--install <name>]   browse/install plugins from a\n")
					fmt.Fprintf(stdout, "                   registry (download + BLAKE3-verify; prints the env to enable)\n")
					return 0
				},
			},
		},
	}
	cliProvider = cliDomain{
		name: "provider",
		commands: []cliCommand{
			{
				names: []string{"creds"},
				run:   shifted(cmdProviderCreds),
			},
			{
				names: []string{"keys"},
				run: func(stdin io.Reader, args []string, stdout, stderr io.Writer) int {
					return keys.Run(args[1:], stdout, stderr)
				},
			},
			{
				names: []string{"connect"},
				ops:   []string{opapi.ConfigSet, opapi.ProviderConnect, opapi.ProviderKeyAdd, opapi.ProviderReload},
				run:   shifted(cmdProviderConnect),
			},
			{
				names: []string{"chatgpt"},
				ops:   []string{opapi.ProviderOAuthImport, opapi.ProviderOAuthLogout, opapi.ProviderOAuthStart, opapi.ProviderOAuthStatus},
				run:   shifted(cmdProviderChatGPT),
			},
			{
				names: []string{"check"},
				run:   shifted(cmdProviderCheck),
			},
			{
				names: []string{"log"},
				ops:   []string{opapi.ProviderLog},
				run:   shifted(cmdProviderLog),
			},
			{
				names: []string{"stats"},
				ops:   []string{opapi.ProviderStats},
				run:   shifted(cmdProviderStats),
			},
			{
				names: []string{"rejections"},
				ops:   []string{opapi.ProviderRejections},
				run:   shifted(cmdProviderRejections),
			},
			{
				names: []string{"reload"},
				ops:   []string{opapi.ProviderReload},
				run: func(stdin io.Reader, args []string, stdout, stderr io.Writer) int {
					return cmdProviderReload(stdout, stderr)
				},
			},
			{
				names: []string{"setup"},
				run:   shifted(cmdProviderSetup),
			},
			{
				names: []string{"import"},
				run:   shifted(cmdProviderImport),
			},
			{
				names: []string{"cost"},
				ops:   []string{opapi.CatalogList},
				run:   shifted(cmdProviderCost),
			},
		},
	}
	cliProviderChatGPT = cliDomain{
		name: "providerchatgpt",
		commands: []cliCommand{
			{
				names: []string{"login", "signin", "connect"},
				ops:   []string{opapi.ProviderOAuthStart, opapi.ProviderOAuthStatus},
				run: func(stdin io.Reader, args []string, stdout, stderr io.Writer) int {
					return chatgptLogin(stdout, stderr)
				},
			},
			{
				names: []string{"import"},
				ops:   []string{opapi.ProviderOAuthImport},
				run:   shifted(chatgptImport),
			},
			{
				names: []string{"logout", "disconnect"},
				ops:   []string{opapi.ProviderOAuthLogout},
				run: func(stdin io.Reader, args []string, stdout, stderr io.Writer) int {
					return chatgptLogout(stdout, stderr)
				},
			},
			{
				names: []string{"status"},
				ops:   []string{opapi.ProviderOAuthStatus},
				run: func(stdin io.Reader, args []string, stdout, stderr io.Writer) int {
					return chatgptStatus(stdout, stderr)
				},
			},
		},
	}
	cliProviderCreds = cliDomain{
		name: "provider creds",
		commands: []cliCommand{
			{
				names: []string{"list", "ls"},
				run:   shifted(credsList),
			},
			{
				names: []string{"set"},
				run:   shifted(credsSet),
			},
			{
				names: []string{"rm", "remove", "del", "delete", "unset"},
				run:   shifted(credsRm),
			},
		},
	}
	cliPulse = cliDomain{
		name: "pulse",
		commands: []cliCommand{
			{
				names: []string{"status"},
				ops:   []string{opapi.PulseStatus},
				run: func(stdin io.Reader, args []string, stdout, stderr io.Writer) int {
					return cmdPulseControl(args[0], args[1:], stdout, stderr)
				},
			},
			{
				names: []string{"pause"},
				ops:   []string{opapi.PulsePause},
				run: func(stdin io.Reader, args []string, stdout, stderr io.Writer) int {
					return cmdPulseControl(args[0], args[1:], stdout, stderr)
				},
			},
			{
				names: []string{"resume"},
				ops:   []string{opapi.PulseResume},
				run: func(stdin io.Reader, args []string, stdout, stderr io.Writer) int {
					return cmdPulseControl(args[0], args[1:], stdout, stderr)
				},
			},
			{
				names: []string{"asks"},
				ops:   []string{opapi.PulseAskResolve, opapi.PulseAsks},
				run:   shifted(cmdPulseAsks),
			},
		},
	}
	cliRateLimit = cliDomain{
		name: "ratelimit",
		commands: []cliCommand{
			{
				names: []string{"log"},
				ops:   []string{opapi.RateLimitLog},
				run:   shifted(cmdRateLimitLog),
			},
			{
				names: []string{"stats"},
				ops:   []string{opapi.RateLimitStats},
				run:   shifted(cmdRateLimitStats),
			},
		},
	}
	cliRedact = cliDomain{
		name: "redact",
		commands: []cliCommand{
			{
				names: []string{"test"},
				ops:   []string{opapi.RedactTest},
				run:   shifted(cmdRedactTest),
			},
		},
	}
	cliReflect = cliDomain{
		name: "reflect",
		commands: []cliCommand{
			{
				names: []string{"run"},
				ops:   []string{opapi.ReflectRun},
				run:   shifted(cmdReflectRun),
			},
			{
				names: []string{"show"},
				ops:   []string{opapi.ReflectShow},
				run:   shifted(cmdReflectShow),
			},
			{
				names:  []string{"-h", "--help", "help"},
				hidden: true,
				run: func(stdin io.Reader, args []string, stdout, stderr io.Writer) int {
					fmt.Fprintf(stdout, "usage: %s reflect <subcommand>\n", brand.CLI)
					fmt.Fprintf(stdout, "  run [--json]      run a reflection pass now (decays stale world-model entities)\n")
					fmt.Fprintf(stdout, "  show [--json]     print the latest reflection report\n")
					return 0
				},
			},
		},
	}
	cliRollback = cliDomain{
		name: "rollback",
		commands: []cliCommand{
			{
				names: []string{"list", "ls"},
				run:   shifted(cmdRollbackList),
			},
			{
				names: []string{"show"},
				run: func(stdin io.Reader, args []string, stdout, stderr io.Writer) int {
					return cmdRollbackShow(args[0], args[1:], stdout, stderr)
				},
			},
			{
				names: []string{"dry-run"},
				run: func(stdin io.Reader, args []string, stdout, stderr io.Writer) int {
					return cmdRollbackShow(args[0], args[1:], stdout, stderr)
				},
			},
			{
				names: []string{"preview"},
				run: func(stdin io.Reader, args []string, stdout, stderr io.Writer) int {
					return cmdRollbackShow(args[0], args[1:], stdout, stderr)
				},
			},
			{
				names: []string{"apply"},
				ops:   []string{opapi.ConfigSet, opapi.SkillRestore, opapi.WorkflowRestore},
				run:   shifted(cmdRollbackApply),
			},
			{
				names:  []string{"-h", "--help", "help"},
				hidden: true,
				run: func(stdin io.Reader, args []string, stdout, stderr io.Writer) int {
					return rollbackUsage(stdout)
				},
			},
		},
	}
	cliRuns = cliDomain{
		name: "runs",
		commands: []cliCommand{
			{
				names: []string{"list"},
				ops:   []string{opapi.RunsList},
				run:   shifted(cmdRunsList),
			},
			{
				names: []string{"show"},
				ops:   []string{opapi.JournalTail, opapi.RunsList},
				run:   shifted(cmdRunsShow),
			},
			{
				names: []string{"last"},
				ops:   []string{opapi.JournalTail, opapi.RunsList},
				run:   shifted(cmdRunsLast),
			},
			{
				names: []string{"stats"},
				ops:   []string{opapi.RunsStats},
				run:   shifted(cmdRunsStats),
			},
			{
				names: []string{"cancel"},
				ops:   []string{opapi.CancelRun},
				run:   shifted(cmdRunsCancel),
			},
			{
				names: []string{"pause"},
				ops:   []string{opapi.RunPause},
				run: func(stdin io.Reader, args []string, stdout, stderr io.Writer) int {
					return cmdRunsSteerVerb(opapi.RunPause, "pause", args[1:], stdout, stderr)
				},
			},
			{
				names: []string{"resume"},
				ops:   []string{opapi.RunResume},
				run: func(stdin io.Reader, args []string, stdout, stderr io.Writer) int {
					return cmdRunsSteerVerb(opapi.RunResume, "resume", args[1:], stdout, stderr)
				},
			},
			{
				names: []string{"step"},
				ops:   []string{opapi.RunStep},
				run: func(stdin io.Reader, args []string, stdout, stderr io.Writer) int {
					return cmdRunsSteerVerb(opapi.RunStep, "step", args[1:], stdout, stderr)
				},
			},
			{
				names: []string{"steer"},
				ops:   []string{opapi.RunSteer},
				run:   shifted(cmdRunsSteer),
			},
			{
				names: []string{"intervene"},
				ops:   []string{opapi.RunIntervene},
				run:   shifted(cmdRunsIntervene),
			},
			{
				names:  []string{"-h", "--help", "help"},
				hidden: true,
				run: func(stdin io.Reader, args []string, stdout, stderr io.Writer) int {
					fmt.Fprintf(stdout, "usage: %s runs <subcommand>\n", brand.CLI)
					fmt.Fprintf(stdout, "  list [N] [--json]            show the last N agent runs (default 20)\n")
					fmt.Fprintf(stdout, "  show <correlation> [--json]  render one run as a task arc\n")
					fmt.Fprintf(stdout, "  last [--json]                shorthand for show <newest correlation>\n")
					fmt.Fprintf(stdout, "  stats [--json]               aggregate run health (counts, success rate, durations)\n")
					fmt.Fprintf(stdout, "  cancel <correlation>         cancel one in-flight run (→ failed/canceled)\n")
					fmt.Fprintf(stdout, "  pause <correlation>          park a live run at its next iteration boundary\n")
					fmt.Fprintf(stdout, "  resume <correlation>         let a paused run continue\n")
					fmt.Fprintf(stdout, "  step <correlation>           advance a run one iteration, then re-pause\n")
					fmt.Fprintf(stdout, "  steer <correlation> <text>   inject an operator directive into a live run\n")
					fmt.Fprintf(stdout, "  intervene <primitive> <correlation> [text] [--lease <dur>] [--key <id>]\n")
					return 0
				},
			},
		},
	}
	cliSchedule = cliDomain{
		name: "schedule",
		commands: []cliCommand{
			{
				names: []string{"add"},
				ops:   []string{opapi.ScheduleAdd},
				run:   shifted(cmdScheduleAdd),
			},
			{
				names: []string{"edit"},
				ops:   []string{opapi.ScheduleEdit},
				run:   shifted(cmdScheduleEdit),
			},
			{
				names: []string{"list", "ls"},
				ops:   []string{opapi.ScheduleList},
				run:   shifted(cmdScheduleList),
			},
			{
				names: []string{"fires", "history"},
				ops:   []string{opapi.ScheduleFires},
				run:   shifted(cmdScheduleFires),
			},
			{
				names: []string{"stats"},
				ops:   []string{opapi.ScheduleStats},
				run:   shifted(cmdScheduleStats),
			},
			{
				names: []string{"test", "preview"},
				ops:   []string{opapi.ScheduleTest},
				run:   shifted(cmdScheduleTest),
			},
			{
				names: []string{"rm", "remove"},
				ops:   []string{opapi.ScheduleRemove},
				run:   shifted(cmdScheduleRemove),
			},
			{
				names: []string{"run", "trigger"},
				ops:   []string{opapi.ScheduleRun},
				run:   shifted(cmdScheduleRun),
			},
			{
				names: []string{"pause"},
				ops:   []string{opapi.ScheduleEnable},
				run: func(stdin io.Reader, args []string, stdout, stderr io.Writer) int {
					return cmdScheduleEnable(args[1:], stdout, stderr, false)
				},
			},
			{
				names: []string{"resume"},
				ops:   []string{opapi.ScheduleEnable},
				run: func(stdin io.Reader, args []string, stdout, stderr io.Writer) int {
					return cmdScheduleEnable(args[1:], stdout, stderr, true)
				},
			},
			{
				names:  []string{"-h", "--help", "help"},
				hidden: true,
				run: func(stdin io.Reader, args []string, stdout, stderr io.Writer) int {
					fmt.Fprintf(stdout, "usage: %s schedule <subcommand>\n", brand.CLI)
					fmt.Fprintf(stdout, "  add \"<agent task>\" --every <dur> [--between HH:MM-HH:MM [--days <spec>]]  wake an agent/task on an interval\n")
					fmt.Fprintf(stdout, "  add \"<agent task>\" --at <HH:MM> [--days <spec>] [--agent <slug>] [--model <id>]    wake at a wall-clock time\n")
					fmt.Fprintf(stdout, "  add [\"<label>\"] --workflow <ref> [--payload JSON] --every <dur>|--at <HH:MM>|--in <dur>\n")
					fmt.Fprintf(stdout, "  add [\"<label>\"] --system-task %s --every <dur>|--at <HH:MM>|--in <dur>\n", scheduleSystemTaskUsage())
					fmt.Fprintf(stdout, "  add [\"<label>\"] --tool <name> [--payload JSON] --every <dur>|--at <HH:MM>|--in <dur>\n")
					fmt.Fprintf(stdout, "  add \"<agent task>\" --continuous <dur> [--agent <slug>]      cycle loop; re-wakes after each completed run plus cooldown\n")
					fmt.Fprintf(stdout, "  add \"<agent task>\" --in <dur> | --once --at <HH:MM>          one-shot (fires once, then removed)\n")
					fmt.Fprintf(stdout, "  edit <id> [--intent <task/label>] [--model <id>] [<cadence flag>]  change a schedule in place\n")
					fmt.Fprintf(stdout, "  list [--json]                                                list all schedules\n")
					fmt.Fprintf(stdout, "  fires [N] [--id <sched>] [--json]                            recent scheduled firings + outcomes\n")
					fmt.Fprintf(stdout, "  stats [--id <sched>] [--since <dur>] [--json]                aggregate firing health (counts, success, spend)\n")
					fmt.Fprintf(stdout, "  test <id> [--count N] [--json]                               preview the next N fire times (dry-run)\n")
					fmt.Fprintf(stdout, "  rm <id> [--json]                                             delete a schedule\n")
					fmt.Fprintf(stdout, "  run <id> [--json]                                            fire a schedule now (next tick)\n")
					fmt.Fprintf(stdout, "  pause <id> / resume <id> [--json]                            disable / re-enable without deleting\n")
					fmt.Fprintf(stdout, "  <dur> is a Go duration (30m, 1h, 24h); <HH:MM> is 24h time.\n")
					fmt.Fprintf(stdout, "  <spec> is weekdays | weekends | a list/range like mon,wed,fri or mon-fri.\n")
					fmt.Fprintf(stdout, "  --tz <IANA> (e.g. America/New_York) sets the zone for --at / --between wall-clock times.\n")
					return 0
				},
			},
		},
	}
	cliSeat = cliDomain{
		name: "seats",
		commands: []cliCommand{
			{
				names: []string{"add", "create"},
				ops:   []string{opapi.SeatCreate},
				run:   shifted(cmdSeatAdd),
			},
			{
				names: []string{"remove", "rm", "delete"},
				ops:   []string{opapi.SeatDelete},
				run:   shifted(cmdSeatRemove),
			},
			{
				names: []string{"list", "ls", "-h", "--help", "help", "--json"},
				ops:   []string{opapi.SeatList},
				run:   direct(cmdSeatList),
			},
		},
	}
	cliSkill = cliDomain{
		name: "skill",
		commands: []cliCommand{
			{
				names: []string{"list", "ls"},
				ops:   []string{opapi.SkillList},
				run:   shifted(cmdSkillList),
			},
			{
				names: []string{"show", "get"},
				ops:   []string{opapi.SkillGet},
				run:   shifted(cmdSkillShow),
			},
			{
				names: []string{"history", "log"},
				ops:   []string{opapi.SkillHistory},
				run:   shifted(cmdSkillHistory),
			},
			{
				names: []string{"promote"},
				ops:   []string{opapi.SkillPromote},
				run: func(stdin io.Reader, args []string, stdout, stderr io.Writer) int {
					return cmdSkillTransition(args[1:], opapi.SkillPromote, "promote", stdout, stderr)
				},
			},
			{
				names: []string{"quarantine"},
				ops:   []string{opapi.SkillQuarantine},
				run: func(stdin io.Reader, args []string, stdout, stderr io.Writer) int {
					return cmdSkillTransition(args[1:], opapi.SkillQuarantine, "quarantine", stdout, stderr)
				},
			},
			{
				names: []string{"archive"},
				ops:   []string{opapi.SkillArchive},
				run: func(stdin io.Reader, args []string, stdout, stderr io.Writer) int {
					return cmdSkillTransition(args[1:], opapi.SkillArchive, "archive", stdout, stderr)
				},
			},
			{
				names: []string{"revert"},
				ops:   []string{opapi.SkillRevert},
				run: func(stdin io.Reader, args []string, stdout, stderr io.Writer) int {
					return cmdSkillTransition(args[1:], opapi.SkillRevert, "revert", stdout, stderr)
				},
			},
			{
				names: []string{"share"},
				ops:   []string{opapi.SkillReassign, opapi.SkillShare},
				run: func(stdin io.Reader, args []string, stdout, stderr io.Writer) int {
					return cmdSkillReassign(args[1:], true, stdout, stderr)
				},
			},
			{
				names: []string{"reassign"},
				ops:   []string{opapi.SkillReassign, opapi.SkillShare},
				run: func(stdin io.Reader, args []string, stdout, stderr io.Writer) int {
					return cmdSkillReassign(args[1:], false, stdout, stderr)
				},
			},
			{
				names: []string{"diff"},
				ops:   []string{opapi.SkillGet},
				run:   shifted(cmdSkillDiff),
			},
			{
				names: []string{"export"},
				ops:   []string{opapi.SkillGet, opapi.SkillList},
				run:   shifted(cmdSkillExport),
			},
			{
				names: []string{"import"},
				ops:   []string{opapi.SkillImport},
				run:   shifted(cmdSkillImport),
			},
			{
				names: []string{"files"},
				ops:   []string{opapi.SkillFiles},
				run:   shifted(cmdSkillFiles),
			},
			{
				names: []string{"cat"},
				ops:   []string{opapi.SkillReadFile},
				run:   shifted(cmdSkillCat),
			},
			{
				names: []string{"registry"},
				ops:   []string{opapi.SkillImport},
				run:   shifted(cmdSkillRegistry),
			},
			{
				names: []string{"hygiene"},
				ops:   []string{opapi.SkillHygiene},
				run:   shifted(cmdSkillHygiene),
			},
			{
				names: []string{"workshop"},
				ops:   []string{opapi.SkillArchive, opapi.SkillGet, opapi.SkillHistory, opapi.SkillHygiene, opapi.SkillImport, opapi.SkillList, opapi.SkillPromote, opapi.SkillQuarantine},
				run:   shifted(cmdSkillWorkshop),
			},
			{
				names:  []string{"-h", "--help", "help"},
				hidden: true,
				run: func(stdin io.Reader, args []string, stdout, stderr io.Writer) int {
					fmt.Fprintf(stdout, "usage: %s skill <subcommand>\n", brand.CLI)
					fmt.Fprintf(stdout, "  list [--json]                 list all skills + lifecycle state\n")
					fmt.Fprintf(stdout, "  show <id> [--json]            read one skill (exit 3 = absent)\n")
					fmt.Fprintf(stdout, "  history <id> [--json]         the skill's lifecycle event chain\n")
					fmt.Fprintf(stdout, "  promote <id> [--json]         advance draft->shadow->active\n")
					fmt.Fprintf(stdout, "  quarantine <id> [--reason R] [--json]   pull from production\n")
					fmt.Fprintf(stdout, "  archive <id> [--reason R] [--json]      retire without restoring a parent\n")
					fmt.Fprintf(stdout, "  revert <id> [--json]          archive + restore lineage parent\n")
					fmt.Fprintf(stdout, "  share <id> [--json]           promote a private (per-agent) skill to the shared pool\n")
					fmt.Fprintf(stdout, "  reassign <id> [--agent S]     change a skill's owning agent (omit --agent to share)\n")
					fmt.Fprintf(stdout, "  diff <id> [<id2>]             diff a skill's body vs its parent (or vs id2)\n")
					fmt.Fprintf(stdout, "  export <id> [--out <file>]    write a portable, verifiable skill bundle\n")
					fmt.Fprintf(stdout, "  export --all [--dir <dir>] [--agent <slug>]   export every skill (or one agent's) into a registry dir\n")
					fmt.Fprintf(stdout, "  import <bundle|dir> [--json]  install a bundle/SKILL.md/skill directory as a draft\n")
					fmt.Fprintf(stdout, "  files <id> [--json]           list a skill's bundle resources + their directory\n")
					fmt.Fprintf(stdout, "  cat <id> <path>               print one bundle resource (reference file or script)\n")
					fmt.Fprintf(stdout, "  registry <dir> [--install <name>]   list/install verifiable bundles in a directory\n")
					fmt.Fprintf(stdout, "  hygiene [--idle-days N] [--json]   report idle/unused skills (epistemic hygiene)\n")
					fmt.Fprintf(stdout, "  workshop <subcommand>         proposal review surface over Forge lifecycle\n")
					return 0
				},
			},
		},
	}
	cliSkillWorkshop = cliDomain{
		name: "skillworkshop",
		commands: []cliCommand{
			{
				names: []string{"list", "ls"},
				ops:   []string{opapi.SkillList},
				run:   shifted(cmdSkillWorkshopList),
			},
			{
				names: []string{"inspect", "show", "get"},
				ops:   []string{opapi.SkillGet, opapi.SkillHistory},
				run:   shifted(cmdSkillWorkshopInspect),
			},
			{
				names: []string{"scan"},
				ops:   []string{opapi.SkillGet},
				run:   shifted(cmdSkillWorkshopScan),
			},
			{
				names: []string{"diff"},
				ops:   []string{opapi.SkillGet},
				run:   shifted(cmdSkillDiff),
			},
			{
				names: []string{"curate", "curator"},
				ops:   []string{opapi.SkillGet, opapi.SkillHygiene, opapi.SkillQuarantine},
				run:   shifted(cmdSkillWorkshopCurate),
			},
			{
				names: []string{"apply", "promote"},
				ops:   []string{opapi.SkillGet, opapi.SkillPromote},
				run:   shifted(cmdSkillWorkshopApply),
			},
			{
				names: []string{"reject"},
				ops:   []string{opapi.SkillArchive, opapi.SkillGet},
				run:   shifted(cmdSkillWorkshopReject),
			},
			{
				names: []string{"quarantine"},
				ops:   []string{opapi.SkillGet, opapi.SkillQuarantine},
				run:   shifted(cmdSkillWorkshopQuarantine),
			},
			{
				names: []string{"propose", "import"},
				ops:   []string{opapi.SkillImport},
				run:   shifted(cmdSkillImport),
			},
			{
				names: []string{"propose-create", "create"},
				ops:   []string{opapi.SkillImport},
				run:   shifted(cmdSkillWorkshopProposeCreate),
			},
			{
				names: []string{"propose-update", "update"},
				ops:   []string{opapi.SkillGet, opapi.SkillImport},
				run:   shifted(cmdSkillWorkshopProposeUpdate),
			},
			{
				names:  []string{"-h", "--help", "help"},
				hidden: true,
				run: func(stdin io.Reader, args []string, stdout, stderr io.Writer) int {
					return skillWorkshopUsage(stdout)
				},
			},
		},
	}
	cliStanding = cliDomain{
		name: "standing",
		commands: []cliCommand{
			{
				names: []string{"list"},
				ops:   []string{opapi.StandingList},
				run:   shifted(cmdStandingList),
			},
			{
				names: []string{"add"},
				ops:   []string{opapi.StandingAdd},
				run:   shifted(cmdStandingAdd),
			},
			{
				names: []string{"edit", "set"},
				ops:   []string{opapi.StandingEdit},
				run:   shifted(cmdStandingEdit),
			},
			{
				names: []string{"pause"},
				ops:   []string{opapi.StandingSetEnabled},
				run: func(stdin io.Reader, args []string, stdout, stderr io.Writer) int {
					return cmdStandingSetEnabled(args[1:], stdout, stderr, false)
				},
			},
			{
				names: []string{"resume"},
				ops:   []string{opapi.StandingSetEnabled},
				run: func(stdin io.Reader, args []string, stdout, stderr io.Writer) int {
					return cmdStandingSetEnabled(args[1:], stdout, stderr, true)
				},
			},
			{
				names: []string{"remove", "rm"},
				ops:   []string{opapi.StandingRemove},
				run:   shifted(cmdStandingRemove),
			},
			{
				names: []string{"why"},
				ops:   []string{opapi.StandingWhy},
				run:   shifted(cmdStandingWhy),
			},
			{
				names:  []string{"-h", "--help", "help"},
				hidden: true,
				run: func(stdin io.Reader, args []string, stdout, stderr io.Writer) int {
					return standingUsage(stdout)
				},
			},
		},
	}
	cliState = cliDomain{
		name: "state",
		commands: []cliCommand{
			{
				names: []string{"list"},
				ops:   []string{opapi.StateList},
				run:   shifted(cmdStateList),
			},
			{
				names: []string{"get"},
				ops:   []string{opapi.StateGet},
				run:   shifted(cmdStateGet),
			},
			{
				names:  []string{"-h", "--help", "help"},
				hidden: true,
				run: func(stdin io.Reader, args []string, stdout, stderr io.Writer) int {
					fmt.Fprintf(stdout, "usage: %s state <subcommand>\n", brand.CLI)
					fmt.Fprintf(stdout, "  list [<namespace>] [--json]   enumerate namespaces, or keys within one\n")
					fmt.Fprintf(stdout, "  get <namespace> <key> [--json]   read a single value (raw JSON)\n")
					return 0
				},
			},
		},
	}
	cliTaste = cliDomain{
		name: "taste",
		commands: []cliCommand{
			{
				names: []string{"list", "ls"},
				ops:   []string{opapi.TasteList},
				run:   shifted(cmdTasteList),
			},
			{
				names: []string{"add", "create"},
				ops:   []string{opapi.TasteCreate},
				run:   shifted(cmdTasteAdd),
			},
			{
				names: []string{"remove", "rm", "delete"},
				ops:   []string{opapi.TasteDelete},
				run:   shifted(cmdTasteRemove),
			},
			{
				names:  []string{"-h", "--help", "help"},
				hidden: true,
				run: func(stdin io.Reader, args []string, stdout, stderr io.Writer) int {
					return tasteUsage(stdout)
				},
			},
		},
	}
	cliTenant = cliDomain{
		name: "tenant",
		commands: []cliCommand{
			{
				names: []string{"create", "add"},
				ops:   []string{opapi.TenantCreate},
				run: func(stdin io.Reader, args []string, stdout, stderr io.Writer) int {
					return cmdTenantByID(args[1:], stdout, stderr, "create", opapi.TenantCreate)
				},
			},
			{
				names: []string{"list", "ls"},
				ops:   []string{opapi.TenantList},
				run:   shifted(cmdTenantList),
			},
			{
				names: []string{"stats"},
				ops:   []string{opapi.TenantStats},
				run:   shifted(cmdTenantStats),
			},
			{
				names: []string{"token"},
				ops:   []string{opapi.TenantToken},
				run: func(stdin io.Reader, args []string, stdout, stderr io.Writer) int {
					return cmdTenantByID(args[1:], stdout, stderr, "token", opapi.TenantToken)
				},
			},
			{
				names: []string{"release", "close"},
				ops:   []string{opapi.TenantRelease},
				run: func(stdin io.Reader, args []string, stdout, stderr io.Writer) int {
					return cmdTenantByID(args[1:], stdout, stderr, "release", opapi.TenantRelease)
				},
			},
			{
				names: []string{"rm", "remove", "delete"},
				ops:   []string{opapi.TenantRemove},
				run: func(stdin io.Reader, args []string, stdout, stderr io.Writer) int {
					return cmdTenantByID(args[1:], stdout, stderr, "rm", opapi.TenantRemove)
				},
			},
			{
				names:  []string{"-h", "--help", "help"},
				hidden: true,
				run: func(stdin io.Reader, args []string, stdout, stderr io.Writer) int {
					fmt.Fprintf(stdout, "usage: %s tenant <subcommand>\n", brand.CLI)
					fmt.Fprintf(stdout, "  create <id> [--json]    create / open an isolated tenant (prints its token)\n")
					fmt.Fprintf(stdout, "  list [--json]           list tenants (id, open state, base dir)\n")
					fmt.Fprintf(stdout, "  stats [--json]          per-tenant run count / spend / last activity (primary token)\n")
					fmt.Fprintf(stdout, "  token <id> [--json]     reveal a tenant's per-tenant credential\n")
					fmt.Fprintf(stdout, "  release <id> [--json]   close a tenant's kernel, keep its state on disk\n")
					fmt.Fprintf(stdout, "  rm <id> [--json]        delete a tenant and ALL its state (destructive)\n")
					fmt.Fprintf(stdout, "  <id> is [a-z0-9_-], 1-64 chars. Requires the daemon started with %sMULTITENANT=on.\n", brand.EnvPrefix)
					fmt.Fprintf(stdout, "  Route to a tenant: HTTP `X-Agezt-Tenant: <id>` + its token, or `%s run --tenant <id>`.\n", brand.CLI)
					return 0
				},
			},
		},
	}
	cliToken = cliDomain{
		name: "token",
		commands: []cliCommand{
			{
				names: []string{"create"},
				run:   shifted(cmdTokenCreate),
			},
			{
				names: []string{"validate"},
				run:   shifted(cmdTokenValidate),
			},
			{
				names:  []string{"help"},
				hidden: true,
				run: func(stdin io.Reader, args []string, stdout, stderr io.Writer) int {
					return tokenUsage(stdout)
				},
			},
		},
	}
	cliTool = cliDomain{
		name: "tool",
		commands: []cliCommand{
			{
				names: []string{"list"},
				ops:   []string{opapi.ToolList},
				run:   shifted(cmdToolList),
			},
			{
				names: []string{"log"},
				ops:   []string{opapi.ToolLog},
				run:   shifted(cmdToolLog),
			},
			{
				names: []string{"stats"},
				ops:   []string{opapi.ToolStats},
				run:   shifted(cmdToolStats),
			},
		},
	}
	cliToolforge = cliDomain{
		name: "toolforge",
		commands: []cliCommand{
			{
				names: []string{"list"},
				ops:   []string{opapi.ToolforgeList},
				run:   shifted(cmdToolforgeList),
			},
			{
				names: []string{"show"},
				ops:   []string{opapi.ToolforgeShow},
				run:   shifted(cmdToolforgeShow),
			},
			{
				names: []string{"draft", "add", "create"},
				ops:   []string{opapi.ToolforgeDraft},
				run:   shifted(cmdToolforgeDraft),
			},
			{
				names: []string{"edit", "set"},
				ops:   []string{opapi.ToolforgeEdit},
				run:   shifted(cmdToolforgeEdit),
			},
			{
				names: []string{"test"},
				ops:   []string{opapi.ToolforgeTest},
				run:   shifted(cmdToolforgeTest),
			},
			{
				names: []string{"promote"},
				ops:   []string{opapi.ToolforgePromote},
				run:   shifted(cmdToolforgePromote),
			},
			{
				names: []string{"quarantine"},
				ops:   []string{opapi.ToolforgeQuarantine},
				run:   shifted(cmdToolforgeQuarantine),
			},
			{
				names: []string{"remove", "rm"},
				ops:   []string{opapi.ToolforgeRemove},
				run:   shifted(cmdToolforgeRemove),
			},
			{
				names:  []string{"-h", "--help", "help"},
				hidden: true,
				run: func(stdin io.Reader, args []string, stdout, stderr io.Writer) int {
					return toolforgeUsage(stdout)
				},
			},
		},
	}
	cliVault = cliDomain{
		name: "vault",
		commands: []cliCommand{
			{
				names: []string{"status"},
				run: func(stdin io.Reader, args []string, stdout, stderr io.Writer) int {
					return cmdVaultStatus(stdout, stderr)
				},
			},
			{
				names: []string{"encrypt"},
				run: func(stdin io.Reader, args []string, stdout, stderr io.Writer) int {
					return cmdVaultEncrypt(stdout, stderr)
				},
			},
			{
				names: []string{"decrypt"},
				run: func(stdin io.Reader, args []string, stdout, stderr io.Writer) int {
					return cmdVaultDecrypt(stdout, stderr)
				},
			},
			{
				names: []string{"rotate"},
				run: func(stdin io.Reader, args []string, stdout, stderr io.Writer) int {
					return cmdVaultRotate(stdout, stderr)
				},
			},
			{
				names: []string{"migrate"},
				run: func(stdin io.Reader, args []string, stdout, stderr io.Writer) int {
					return cmdVaultMigrate(stdout, stderr)
				},
			},
			{
				names:  []string{"-h", "--help", "help"},
				hidden: true,
				run: func(stdin io.Reader, args []string, stdout, stderr io.Writer) int {
					printVaultHelp(stdout)
					return 0
				},
			},
		},
	}
	cliWarden = cliDomain{
		name: "warden",
		commands: []cliCommand{
			{
				names: []string{"log"},
				ops:   []string{opapi.WardenLog},
				run:   shifted(cmdWardenLog),
			},
			{
				names: []string{"stats"},
				ops:   []string{opapi.WardenStats},
				run:   shifted(cmdWardenStats),
			},
		},
	}
	cliWeb = cliDomain{
		name: "web",
		commands: []cliCommand{
			{
				names: []string{"password"},
				ops:   []string{opapi.ConfigSet},
				run: func(stdin io.Reader, args []string, stdout, stderr io.Writer) int {
					return cmdWebPassword(args[1:], os.Stdin, stdout, stderr)
				},
			},
		},
	}
	cliWebPassword = cliDomain{
		name: "web",
		commands: []cliCommand{
			{
				names: []string{"set"},
				run: func(stdin io.Reader, args []string, stdout, stderr io.Writer) int {
					value := ""
					if len(args) >= 2 {
						value = strings.Join(args[1:], " ")
					} else {
						// Prompt + confirm so a typo'd password can't lock the console
						// behind a string nobody knows. Line reads (like `provider creds
						// set`) — nothing lands in shell history.
						reader := bufio.NewReader(stdin)
						fmt.Fprintf(stdout, "new console password: ")
						first, err := readPromptLine(reader)
						if err != nil {
							fmt.Fprintf(stderr, "%s: read stdin: %v\n", brand.CLI, err)
							return 1
						}
						fmt.Fprintf(stdout, "repeat password: ")
						second, err := readPromptLine(reader)
						if err != nil {
							fmt.Fprintf(stderr, "%s: read stdin: %v\n", brand.CLI, err)
							return 1
						}
						if first != second {
							fmt.Fprintf(stderr, "%s: passwords do not match — nothing changed\n", brand.CLI)
							return 1
						}
						value = first
					}
					if strings.TrimSpace(value) == "" {
						fmt.Fprintf(stderr, "%s: password is empty (use `%s web password clear` to remove it)\n", brand.CLI, brand.CLI)
						return 2
					}
					return applyWebPassword(value, stdout, stderr)
				},
			},
			{
				names: []string{"clear", "reset", "rm", "unset"},
				ops:   []string{opapi.ConfigSet},
				run: func(stdin io.Reader, args []string, stdout, stderr io.Writer) int {
					return applyWebPassword("", stdout, stderr)
				},
			},
			{
				names: []string{"status"},
				run: func(stdin io.Reader, args []string, stdout, stderr io.Writer) int {
					store, err := openCredsStore(stderr)
					if err != nil {
						return 1
					}
					if store.Has(webPasswordEnv) {
						fmt.Fprintf(stdout, "console password: SET (stored in the vault; value never shown)\n")
					} else {
						fmt.Fprintf(stdout, "console password: not set — the console is token-only\n")
						if os.Getenv(webPasswordEnv) != "" {
							fmt.Fprintf(stdout, "note: %s is set in this shell's environment; the daemon may still enforce it if started with it\n", webPasswordEnv)
						}
					}
					return 0
				},
			},
		},
	}
	cliWebhook = cliDomain{
		name: "webhook",
		commands: []cliCommand{
			{
				names: []string{"test"},
				run:   shifted(cmdWebhookTest),
			},
			{
				names: []string{"log"},
				ops:   []string{opapi.WebhookLog},
				run:   shifted(cmdWebhookLog),
			},
			{
				names: []string{"stats"},
				ops:   []string{opapi.WebhookStats},
				run:   shifted(cmdWebhookStats),
			},
		},
	}
	cliWorkboard = cliDomain{
		name: "workboard",
		commands: []cliCommand{
			{
				names: []string{"list", "ls"},
				ops:   []string{opapi.WorkboardList},
				run:   shifted(cmdWorkboardList),
			},
			{
				names: []string{"lanes"},
				ops:   []string{opapi.WorkboardLanes},
				run:   shifted(cmdWorkboardLanes),
			},
			{
				names: []string{"show", "get"},
				ops:   []string{opapi.WorkboardShow},
				run:   shifted(cmdWorkboardShow),
			},
			{
				names: []string{"create", "add"},
				ops:   []string{opapi.WorkboardCreate},
				run:   shifted(cmdWorkboardCreate),
			},
			{
				names: []string{"claim"},
				ops:   []string{opapi.WorkboardClaim},
				run:   shifted(cmdWorkboardClaim),
			},
			{
				names: []string{"heartbeat", "beat"},
				ops:   []string{opapi.WorkboardHeartbeat},
				run:   shifted(cmdWorkboardHeartbeat),
			},
			{
				names: []string{"comment"},
				ops:   []string{opapi.WorkboardComment},
				run:   shifted(cmdWorkboardComment),
			},
			{
				names: []string{"block"},
				ops:   []string{opapi.WorkboardBlock},
				run:   shifted(cmdWorkboardBlock),
			},
			{
				names: []string{"fail"},
				ops:   []string{opapi.WorkboardFail},
				run:   shifted(cmdWorkboardFail),
			},
			{
				names: []string{"unblock"},
				ops:   []string{opapi.WorkboardUnblock},
				run: func(stdin io.Reader, args []string, stdout, stderr io.Writer) int {
					return cmdWorkboardActor(args[1:], stdout, stderr, "unblock", opapi.WorkboardUnblock)
				},
			},
			{
				names: []string{"complete", "done"},
				ops:   []string{opapi.WorkboardComplete},
				run: func(stdin io.Reader, args []string, stdout, stderr io.Writer) int {
					return cmdWorkboardActor(args[1:], stdout, stderr, "complete", opapi.WorkboardComplete)
				},
			},
			{
				names: []string{"prove"},
				ops:   []string{opapi.WorkboardProve},
				run: func(stdin io.Reader, args []string, stdout, stderr io.Writer) int {
					return cmdWorkboardActor(args[1:], stdout, stderr, "prove", opapi.WorkboardProve)
				},
			},
			{
				names: []string{"seat"},
				ops:   []string{opapi.WorkboardSeat},
				run:   shifted(cmdWorkboardSeat),
			},
			{
				names: []string{"archive"},
				ops:   []string{opapi.WorkboardArchive},
				run: func(stdin io.Reader, args []string, stdout, stderr io.Writer) int {
					return cmdWorkboardActor(args[1:], stdout, stderr, "archive", opapi.WorkboardArchive)
				},
			},
			{
				names: []string{"link"},
				ops:   []string{opapi.WorkboardLink},
				run:   shifted(cmdWorkboardLink),
			},
			{
				names: []string{"policy"},
				ops:   []string{opapi.WorkboardPolicy},
				run:   shifted(cmdWorkboardPolicy),
			},
			{
				names: []string{"depend"},
				ops:   []string{opapi.WorkboardDepend},
				run:   shifted(cmdWorkboardDepend),
			},
			{
				names: []string{"reclaim"},
				ops:   []string{opapi.WorkboardReclaim},
				run:   shifted(cmdWorkboardReclaim),
			},
			{
				names: []string{"sweep"},
				ops:   []string{opapi.WorkboardSweep},
				run:   shifted(cmdWorkboardSweep),
			},
			{
				names: []string{"dispatch"},
				ops:   []string{opapi.WorkboardDispatch},
				run:   shifted(cmdWorkboardDispatch),
			},
			{
				names: []string{"watch"},
				ops:   []string{opapi.WorkboardWatch},
				run:   shifted(cmdWorkboardWatch),
			},
			{
				names:  []string{"-h", "--help", "help"},
				hidden: true,
				run: func(stdin io.Reader, args []string, stdout, stderr io.Writer) int {
					return workboardUsage(stdout)
				},
			},
		},
	}
	cliWorkflow = cliDomain{
		name: "workflow",
		commands: []cliCommand{
			{
				names: []string{"list"},
				ops:   []string{opapi.WorkflowList},
				run:   shifted(cmdWorkflowList),
			},
			{
				names: []string{"show"},
				ops:   []string{opapi.WorkflowShow},
				run:   shifted(cmdWorkflowShow),
			},
			{
				names: []string{"save", "add", "import"},
				ops:   []string{opapi.WorkflowSave, opapi.WorkflowShow},
				run:   shifted(cmdWorkflowSave),
			},
			{
				names: []string{"run"},
				ops:   []string{opapi.WorkflowRun},
				run:   shifted(cmdWorkflowRun),
			},
			{
				names: []string{"draft"},
				ops:   []string{opapi.WorkflowDraft, opapi.WorkflowSave, opapi.WorkflowShow},
				run:   shifted(cmdWorkflowDraft),
			},
			{
				names: []string{"refine"},
				ops:   []string{opapi.WorkflowRefine, opapi.WorkflowSave, opapi.WorkflowShow},
				run:   shifted(cmdWorkflowRefine),
			},
			{
				names: []string{"runs"},
				ops:   []string{opapi.WorkflowRuns},
				run:   shifted(cmdWorkflowRuns),
			},
			{
				names: []string{"templates"},
				ops:   []string{opapi.WorkflowSave, opapi.WorkflowShow, opapi.WorkflowTemplates},
				run:   shifted(cmdWorkflowTemplates),
			},
			{
				names: []string{"enable"},
				ops:   []string{opapi.WorkflowSetEnabled, opapi.WorkflowShow},
				run: func(stdin io.Reader, args []string, stdout, stderr io.Writer) int {
					return cmdWorkflowSetEnabled(args[1:], stdout, stderr, true)
				},
			},
			{
				names: []string{"disable"},
				ops:   []string{opapi.WorkflowSetEnabled, opapi.WorkflowShow},
				run: func(stdin io.Reader, args []string, stdout, stderr io.Writer) int {
					return cmdWorkflowSetEnabled(args[1:], stdout, stderr, false)
				},
			},
			{
				names: []string{"remove", "rm"},
				ops:   []string{opapi.WorkflowRemove, opapi.WorkflowShow},
				run:   shifted(cmdWorkflowRemove),
			},
			{
				names:  []string{"-h", "--help", "help"},
				hidden: true,
				run: func(stdin io.Reader, args []string, stdout, stderr io.Writer) int {
					return workflowUsage(stdout)
				},
			},
		},
	}
	cliWorld = cliDomain{
		name: "world",
		commands: []cliCommand{
			{
				names: []string{"add"},
				ops:   []string{opapi.WorldAdd},
				run:   shifted(cmdWorldAdd),
			},
			{
				names: []string{"relate"},
				ops:   []string{opapi.WorldRelate},
				run:   shifted(cmdWorldRelate),
			},
			{
				names: []string{"resolve"},
				ops:   []string{opapi.WorldResolve},
				run:   shifted(cmdWorldResolve),
			},
			{
				names: []string{"neighbors", "neighbours"},
				ops:   []string{opapi.WorldNeighbors},
				run:   shifted(cmdWorldNeighbors),
			},
			{
				names: []string{"list", "ls"},
				ops:   []string{opapi.WorldList},
				run:   shifted(cmdWorldList),
			},
			{
				names: []string{"log"},
				ops:   []string{opapi.WorldLog},
				run:   shifted(cmdWorldLog),
			},
			{
				names: []string{"show", "get"},
				ops:   []string{opapi.WorldGet},
				run:   shifted(cmdWorldShow),
			},
			{
				names: []string{"forget"},
				ops:   []string{opapi.WorldForget},
				run:   shifted(cmdWorldForget),
			},
			{
				names: []string{"audit"},
				ops:   []string{opapi.WorldList},
				run:   shifted(cmdWorldAudit),
			},
			{
				names:  []string{"-h", "--help", "help"},
				hidden: true,
				run: func(stdin io.Reader, args []string, stdout, stderr io.Writer) int {
					fmt.Fprintf(stdout, "usage: %s world <subcommand>\n", brand.CLI)
					fmt.Fprintf(stdout, "  add <name> [--kind K] [--alias A ...] [--json]\n")
					fmt.Fprintf(stdout, "  relate <from> <verb> <to> [--json]\n")
					fmt.Fprintf(stdout, "  resolve <phrase> [N] [--json]\n")
					fmt.Fprintf(stdout, "  neighbors <name> [--json]\n")
					fmt.Fprintf(stdout, "  list [--json]\n")
					fmt.Fprintf(stdout, "  show <id> [--json]     (exit 3 = absent)\n")
					fmt.Fprintf(stdout, "  forget <id> [--json]   tombstone an entity (reversible, journaled)\n")
					fmt.Fprintf(stdout, "  audit [--json]         world-model health: entity/relation counts, decayed, untyped\n")
					fmt.Fprintf(stdout, "kinds: project|repo|person|org|account|device|channel|topic|task\n")
					fmt.Fprintf(stdout, "verbs: owns|depends_on|member_of|prefers|relates_to|assigned_to|derived_from\n")
					return 0
				},
			},
		},
	}
}
