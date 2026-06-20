// SPDX-License-Identifier: MIT

package main

import (
	"context"
	"fmt"
	"io"
	"strings"
	"time"

	"github.com/agezt/agezt/internal/brand"
	"github.com/agezt/agezt/kernel/controlplane"
)

// cmdProviderChatGPT handles `agt provider chatgpt <login|import|logout|status>`
// — the "Sign in with ChatGPT" subscription provider from the terminal. The
// daemon runs the OAuth redirect listener (127.0.0.1:1455); this command drives
// it over the control plane.
func cmdProviderChatGPT(args []string, stdout, stderr io.Writer) int {
	if len(args) == 0 {
		fmt.Fprintf(stderr, "%s provider chatgpt: subcommand required (login, import, logout, status)\n", brand.CLI)
		return 2
	}
	switch args[0] {
	case "login", "signin", "connect":
		return chatgptLogin(stdout, stderr)
	case "import":
		return chatgptImport(args[1:], stdout, stderr)
	case "logout", "disconnect":
		return chatgptLogout(stdout, stderr)
	case "status":
		return chatgptStatus(stdout, stderr)
	default:
		fmt.Fprintf(stderr, "%s provider chatgpt: unknown subcommand %q (login, import, logout, status)\n", brand.CLI, args[0])
		return 2
	}
}

// chatgptLogin starts the OAuth flow, prints the authorize URL for the operator
// to open, and polls until the daemon's 1455 listener completes the exchange.
func chatgptLogin(stdout, stderr io.Writer) int {
	c := dial(stderr)
	if c == nil {
		return 1
	}
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()
	res, err := c.Call(ctx, controlplane.CmdProviderOAuthStart, map[string]any{"provider": "chatgpt"})
	if err != nil {
		fmt.Fprintf(stderr, "%s provider chatgpt login: %v\n", brand.CLI, err)
		return 1
	}
	authorize, _ := res["authorize_url"].(string)
	state, _ := res["state"].(string)
	if authorize == "" || state == "" {
		fmt.Fprintf(stderr, "%s provider chatgpt login: daemon returned no authorize URL\n", brand.CLI)
		return 1
	}
	fmt.Fprintf(stdout, "Open this URL in a browser on the daemon host to sign in:\n\n  %s\n\n", authorize)
	fmt.Fprintf(stdout, "(the daemon is listening on 127.0.0.1:1455 for the redirect)\n")
	fmt.Fprintf(stdout, "⚠ This uses an unofficial OpenAI backend (the Codex login) and may conflict with OpenAI's terms.\n\nWaiting…")

	deadline := 90 // ~3 min at 2s
	for i := 0; i < deadline; i++ {
		time.Sleep(2 * time.Second)
		sctx, scancel := context.WithTimeout(context.Background(), 15*time.Second)
		st, err := c.Call(sctx, controlplane.CmdProviderOAuthStatus, map[string]any{"state": state})
		scancel()
		if err != nil {
			continue // transient; keep polling
		}
		switch st["status"] {
		case "done":
			email, _ := st["email"].(string)
			fmt.Fprintf(stdout, "\n✓ connected%s — run `%s provider reload` is not needed (applied live)\n", acctSuffix(email), brand.CLI)
			return 0
		case "error":
			msg, _ := st["error"].(string)
			fmt.Fprintf(stderr, "\n%s provider chatgpt login: %s\n", brand.CLI, msg)
			return 1
		}
		fmt.Fprint(stdout, ".")
	}
	fmt.Fprintf(stderr, "\n%s provider chatgpt login: timed out waiting for authorization\n", brand.CLI)
	return 1
}

func chatgptImport(args []string, stdout, stderr io.Writer) int {
	c := dial(stderr)
	if c == nil {
		return 1
	}
	path := ""
	if len(args) > 0 {
		path = args[0]
	}
	ctx, cancel := context.WithTimeout(context.Background(), 20*time.Second)
	defer cancel()
	res, err := c.Call(ctx, controlplane.CmdProviderOAuthImport, map[string]any{"path": path})
	if err != nil {
		fmt.Fprintf(stderr, "%s provider chatgpt import: %v\n", brand.CLI, err)
		return 1
	}
	email, _ := res["email"].(string)
	fmt.Fprintf(stdout, "✓ imported Codex CLI login%s (applied live)\n", acctSuffix(email))
	return 0
}

func chatgptLogout(stdout, stderr io.Writer) int {
	c := dial(stderr)
	if c == nil {
		return 1
	}
	ctx, cancel := context.WithTimeout(context.Background(), 20*time.Second)
	defer cancel()
	if _, err := c.Call(ctx, controlplane.CmdProviderOAuthLogout, nil); err != nil {
		fmt.Fprintf(stderr, "%s provider chatgpt logout: %v\n", brand.CLI, err)
		return 1
	}
	fmt.Fprintf(stdout, "disconnected ChatGPT (applied live)\n")
	return 0
}

func chatgptStatus(stdout, stderr io.Writer) int {
	c := dial(stderr)
	if c == nil {
		return 1
	}
	ctx, cancel := context.WithTimeout(context.Background(), 15*time.Second)
	defer cancel()
	res, err := c.Call(ctx, controlplane.CmdProviderOAuthStatus, map[string]any{"state": ""})
	if err != nil {
		fmt.Fprintf(stderr, "%s provider chatgpt status: %v\n", brand.CLI, err)
		return 1
	}
	if connected, _ := res["connected"].(bool); connected {
		email, _ := res["email"].(string)
		fmt.Fprintf(stdout, "connected%s\n", acctSuffix(email))
	} else {
		fmt.Fprintf(stdout, "not connected — run `%s provider chatgpt login`\n", brand.CLI)
	}
	return 0
}

func acctSuffix(email string) string {
	if strings.TrimSpace(email) == "" {
		return ""
	}
	return " as " + email
}
