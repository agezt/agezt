// SPDX-License-Identifier: MIT

package main

import (
	"encoding/json"
	"flag"
	"fmt"
	"io"
	"strings"
	"time"

	"github.com/agezt/agezt/internal/brand"
	"github.com/agezt/agezt/kernel/agentgw"
)

// cmdToken dispatches `agt token <subcommand>` — manages JWT capability tokens
// for agent subprocess communication.
func cmdToken(args []string, stdout, stderr io.Writer) int {
	if len(args) == 0 {
		return tokenUsage(stderr)
	}
	switch args[0] {
	case "create":
		return cmdTokenCreate(args[1:], stdout, stderr)
	case "validate":
		return cmdTokenValidate(args[1:], stdout, stderr)
	case "help":
		return tokenUsage(stdout)
	default:
		fmt.Fprintf(stderr, "%s token: unknown subcommand %q\n", brand.CLI, args[0])
		return tokenUsage(stderr)
	}
}

func tokenUsage(w io.Writer) int {
	fmt.Fprintf(w, "usage: %s token <create|validate>\n", brand.CLI)
	fmt.Fprintf(w, "  create [--run-id ID] [--caps CAPS] [--max-rpm N] [--burst N] [--expiry DURATION]\n")
	fmt.Fprintf(w, "                     create a scoped capability token for subprocess communication\n")
	fmt.Fprintf(w, "  validate <TOKEN>   validate a token and show its claims\n")
	fmt.Fprintf(w, "\nCapabilities (comma-separated):\n")
	fmt.Fprintf(w, "  eventbus.publish, eventbus.subscribe\n")
	fmt.Fprintf(w, "  memory.read, memory.write, memory.delete, memory.search, memory.list\n")
	fmt.Fprintf(w, "  log.read, log.write\n")
	fmt.Fprintf(w, "  agent.list, agent.query\n")
	fmt.Fprintf(w, "\nExample:\n")
	fmt.Fprintf(w, "  %s token create --run-id run_abc123 --caps memory.write,memory.search,eventbus.publish\n", brand.CLI)
	return 1
}

// cmdTokenCreate creates a new capability token.
func cmdTokenCreate(args []string, stdout, stderr io.Writer) int {
	fs := flag.NewFlagSet("token create", flag.ContinueOnError)
	fs.SetOutput(stderr)
	fs.Usage = func() {}

	var runID string
	var capsCSV string
	var maxRPM int
	var maxBurst int
	var expiryStr string

	fs.StringVar(&runID, "run-id", "", "Run ID for correlation")
	fs.StringVar(&capsCSV, "caps", "", "Comma-separated capabilities")
	fs.IntVar(&maxRPM, "max-rpm", 60, "Max requests per minute")
	fs.IntVar(&maxBurst, "burst", 10, "Burst allowance")
	fs.StringVar(&expiryStr, "expiry", "1h", "Token expiry (e.g., 1h, 30m, 3600s)")

	if err := fs.Parse(args); err != nil {
		if err == flag.ErrHelp {
			return 0
		}
		return 1
	}

	if runID == "" {
		fmt.Fprintf(stderr, "%s token create: --run-id is required\n", brand.CLI)
		return 1
	}
	if capsCSV == "" {
		fmt.Fprintf(stderr, "%s token create: --caps is required\n", brand.CLI)
		return 1
	}

	// Parse capabilities
	caps := parseCaps(capsCSV)
	if len(caps) == 0 {
		fmt.Fprintf(stderr, "%s token create: no valid capabilities specified\n", brand.CLI)
		return 1
	}

	// Parse expiry
	expiry := parseDuration(expiryStr)
	if expiry <= 0 {
		fmt.Fprintf(stderr, "%s token create: invalid expiry %q\n", brand.CLI, expiryStr)
		return 1
	}

	// Create the token
	tm := agentgw.NewTokenManager(getTokenSecret())
	claims := &agentgw.TokenClaims{
		RunID:     runID,
		Caps:      caps,
		MaxRate:   maxRPM,
		MaxBurst:  maxBurst,
		ExpiresAt: time.Now().Add(expiry),
	}

	token, err := tm.CreateToken(claims)
	if err != nil {
		fmt.Fprintf(stderr, "%s token create: failed to create token: %v\n", brand.CLI, err)
		return 1
	}

	// Output the token
	fmt.Fprintf(stdout, "%s\n", token)
	return 0
}

// cmdTokenValidate validates a token and prints its claims.
func cmdTokenValidate(args []string, stdout, stderr io.Writer) int {
	fs := flag.NewFlagSet("token validate", flag.ContinueOnError)
	fs.SetOutput(stderr)

	if len(args) < 1 {
		fmt.Fprintf(stderr, "%s token validate: missing TOKEN argument\n", brand.CLI)
		return 1
	}

	token := args[0]

	tm := agentgw.NewTokenManager(getTokenSecret())
	claims, err := tm.ValidateToken(token)
	if err != nil {
		fmt.Fprintf(stderr, "%s token validate: %v\n", brand.CLI, err)
		return 1
	}

	// Print claims as JSON
	enc := json.NewEncoder(stdout)
	enc.SetIndent("", "  ")
	enc.Encode(map[string]interface{}{
		"valid": true,
		"claims": map[string]interface{}{
			"run_id":     claims.RunID,
			"caps":       claims.Caps,
			"max_rpm":    claims.MaxRate,
			"max_burst":  claims.MaxBurst,
			"expires_at": claims.ExpiresAt.Format(time.RFC3339),
		},
	})
	return 0
}

// parseCaps parses a comma-separated capability string.
func parseCaps(capsCSV string) []string {
	if capsCSV == "" {
		return nil
	}
	parts := splitCSV(capsCSV)
	result := make([]string, 0, len(parts))
	for _, p := range parts {
		cap := agentgw.AgentCapability(p)
		// Validate capability
		switch cap {
		case agentgw.CapEventbusPublish,
			agentgw.CapEventbusSubscribe,
			agentgw.CapChannelSend,
			agentgw.CapChannelRead,
			agentgw.CapChannelList,
			agentgw.CapMemoryRead,
			agentgw.CapMemoryWrite,
			agentgw.CapMemoryDelete,
			agentgw.CapMemorySearch,
			agentgw.CapMemoryList,
			agentgw.CapLogRead,
			agentgw.CapLogWrite,
			agentgw.CapAgentList,
			agentgw.CapAgentQuery,
			agentgw.CapDBQuery,
			agentgw.CapDBRead,
			agentgw.CapDBWrite:
			result = append(result, p)
		}
	}
	return result
}

// splitCSV splits a comma-separated string.
func splitCSV(s string) []string {
	var result []string
	var current strings.Builder
	for _, c := range s {
		if c == ',' {
			if current.Len() > 0 {
				result = append(result, current.String())
				current.Reset()
			}
		} else {
			current.WriteRune(c)
		}
	}
	if current.Len() > 0 {
		result = append(result, current.String())
	}
	return result
}

// parseDuration parses a duration string like "1h", "30m", "3600s".
func parseDuration(s string) time.Duration {
	// Try standard time.ParseDuration first
	if d, err := time.ParseDuration(s); err == nil {
		return d
	}
	// Try just a number (seconds)
	var seconds int64
	fmt.Sscanf(s, "%d", &seconds)
	return time.Duration(seconds) * time.Second
}

// getTokenSecret returns the token signing secret.
// In production, this should come from a secure config source.
func getTokenSecret() []byte {
	// Use the same default as kernel/agentgw for consistency
	return []byte(agentgw.DefaultTokenSecret)
}
