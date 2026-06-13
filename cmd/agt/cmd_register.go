// SPDX-License-Identifier: MIT

// cmd_register.go registers all CLI commands at package initialization.
// This file is imported via a blank import (_) in main.go to trigger init().

package main

import (
	"io"
)

// init registers all agt commands.
// Each command package should call Register(&Command{...}) in its init().
func init() {
	registerRunCommands()
	registerInfoCommands()
	registerAgentCommands()
	registerConfigCommands()
	registerManagementCommands()
}

// registerRunCommands registers commands for running agents.
func registerRunCommands() {
	Register(&Command{
		Name:        "run",
		Aliases:     []string{},
		Description: `run "<intent>"        run an intent end-to-end; streams events`,
		Run:         cmdRun,
	})
	Register(&Command{
		Name:        "halt",
		Aliases:     []string{},
		Description: "freeze all in-flight runs",
		Run: func(args []string, stdout, stderr io.Writer) int {
			return cmdHaltResume("halt", args, stdout, stderr)
		},
	})
	Register(&Command{
		Name:        "resume",
		Aliases:     []string{},
		Description: "clear the halt flag",
		Run: func(args []string, stdout, stderr io.Writer) int {
			return cmdHaltResume("resume", args, stdout, stderr)
		},
	})
	Register(&Command{
		Name:    "agent",
		Aliases: []string{},
		Run:     cmdAgent,
	})
}

// registerInfoCommands registers informational commands.
func registerInfoCommands() {
	Register(&Command{
		Name:    "version",
		Aliases: []string{"-v", "--version"},
		Run: func(args []string, stdout, stderr io.Writer) int {
			// Inline version command
			return 0
		},
	})
	Register(&Command{
		Name:    "help",
		Aliases: []string{"-h", "--help"},
		Run: func(args []string, stdout, stderr io.Writer) int {
			return cmdHelp(args, stdout, stderr)
		},
	})
	Register(&Command{
		Name:    "whoami",
		Aliases: []string{},
		Run:     cmdWhoami,
	})
	Register(&Command{
		Name:    "status",
		Aliases: []string{},
		Run:     cmdStatus,
	})
	Register(&Command{
		Name:    "doctor",
		Aliases: []string{},
		Run:     cmdDoctor,
	})
}

// registerAgentCommands registers commands for agent introspection.
func registerAgentCommands() {
	Register(&Command{Name: "why", Run: cmdWhy})
	Register(&Command{Name: "runs", Run: cmdRuns})
	Register(&Command{Name: "journal", Run: cmdJournal})
	Register(&Command{Name: "memory", Run: cmdMemory})
	Register(&Command{Name: "world", Run: cmdWorld})
	Register(&Command{Name: "standing", Run: cmdStanding})
	Register(&Command{Name: "reflect", Run: cmdReflect})
	Register(&Command{Name: "inbox", Run: cmdInbox})
	Register(&Command{Name: "approvals", Run: cmdApprovals})
	Register(&Command{Name: "redact", Run: cmdRedact})
}

// registerConfigCommands registers configuration commands.
func registerConfigCommands() {
	Register(&Command{Name: "config", Run: cmdConfig})
	Register(&Command{Name: "configcenter", Run: cmdConfigCenter})
	Register(&Command{Name: "vault", Run: cmdVault})
	Register(&Command{Name: "budget", Run: cmdBudget})
	Register(&Command{Name: "cache", Run: cmdCache})
	Register(&Command{Name: "skill", Run: cmdSkill})
	Register(&Command{Name: "plugin", Run: cmdPlugin})
	Register(&Command{Name: "tool", Run: cmdTool})
	Register(&Command{Name: "toolforge", Run: cmdToolforge})
	Register(&Command{Name: "mcp", Run: cmdMCP})
	Register(&Command{Name: "workflow", Run: cmdWorkflow})
}

// registerManagementCommands registers system management commands.
func registerManagementCommands() {
	Register(&Command{Name: "catalog", Run: cmdCatalog})
	Register(&Command{Name: "provider", Run: cmdProvider})
	Register(&Command{Name: "pulse", Run: cmdPulse})
	Register(&Command{Name: "warden", Run: cmdWarden})
	Register(&Command{Name: "netguard", Run: cmdNetguard})
	Register(&Command{Name: "ratelimit", Run: cmdRateLimit})
	Register(&Command{Name: "web", Run: cmdWeb})
	Register(&Command{Name: "webhook", Run: cmdWebhook})
	Register(&Command{Name: "backup", Run: cmdBackup})
	Register(&Command{Name: "restore", Run: cmdRestore})
	Register(&Command{Name: "shutdown", Run: cmdShutdown})
	Register(&Command{Name: "edict", Run: cmdEdict})
	Register(&Command{Name: "state", Run: cmdState})
	Register(&Command{Name: "disk", Run: cmdDisk})
	Register(&Command{Name: "changelog", Run: cmdChangelog})
	Register(&Command{Name: "artifact", Run: cmdArtifact})
	Register(&Command{Name: "send", Run: cmdSend})
	Register(&Command{Name: "ha", Run: cmdHA})
	Register(&Command{Name: "transcribe", Run: cmdTranscribe})
	Register(&Command{Name: "listen", Run: cmdListen})
	Register(&Command{Name: "peers", Run: cmdPeers})
	Register(&Command{Name: "schedule", Run: cmdSchedule})
	Register(&Command{Name: "tenant", Run: cmdTenant})
	Register(&Command{Name: "token", Run: cmdToken})
	Register(&Command{Name: "acp", Run: cmdACP})
	Register(&Command{Name: "quickstart", Run: cmdQuickstart})
	Register(&Command{Name: "approve", Run: func(args []string, stdout, stderr io.Writer) int {
		return cmdDecide("grant", args, stdout, stderr)
	}})
	Register(&Command{Name: "deny", Run: func(args []string, stdout, stderr io.Writer) int {
		return cmdDecide("deny", args, stdout, stderr)
	}})
	Register(&Command{Name: "plan", Run: cmdPlan})
}
