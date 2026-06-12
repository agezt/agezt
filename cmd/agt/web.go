// SPDX-License-Identifier: MIT

package main

// `agt web password` — set, clear, or inspect the console password (M933) from
// the CLI. The recovery story the Web UI itself can't be: if you forget the
// password (or someone else set one) you fix it from the terminal, no token
// hunt, no editing vault files by hand.
//
//	agt web password set            prompt + confirm (recommended; nothing in
//	                                shell history)
//	agt web password set <value>    one-shot (visible in shell history — fine
//	                                for scripts, discouraged interactively)
//	agt web password clear          remove it (console reverts to token-only)
//	agt web password status         is one set? (presence only, never the value)
//
// Apply path: when the daemon is up, one control-plane config_set round-trip
// stores the secret in the vault AND applies it LIVE (the M933 lazy password
// gate re-reads the env) — the next login uses the new password immediately.
// When the daemon is down, the vault is written directly (encrypted at rest,
// M934) and the password takes effect on the next daemon start.

import (
	"bufio"
	"context"
	"fmt"
	"io"
	"os"
	"strings"
	"time"

	"github.com/agezt/agezt/internal/brand"
	"github.com/agezt/agezt/internal/paths"
	"github.com/agezt/agezt/kernel/controlplane"
)

// webPasswordEnv is the Config Center field the console password lives under —
// a SECRET in the schema, so it is stored in the vault, never config.json.
const webPasswordEnv = "AGEZT_WEB_PASSWORD"

// cmdWeb dispatches `agt web <subcommand>`.
func cmdWeb(args []string, stdout, stderr io.Writer) int {
	if len(args) == 0 || args[0] == "-h" || args[0] == "--help" {
		fmt.Fprintf(stdout, "usage: %s web password <set [<value>] | clear | status>\n\n", brand.CLI)
		fmt.Fprintf(stdout, "  password set [<value>]   set the console password (prompts + confirms when omitted)\n")
		fmt.Fprintf(stdout, "  password clear           remove it — the console reverts to token-only\n")
		fmt.Fprintf(stdout, "  password status          report whether one is set (never the value)\n")
		if len(args) == 0 {
			return 2
		}
		return 0
	}
	switch args[0] {
	case "password":
		return cmdWebPassword(args[1:], os.Stdin, stdout, stderr)
	default:
		fmt.Fprintf(stderr, "%s web: unknown subcommand %q (password)\n", brand.CLI, args[0])
		return 2
	}
}

// cmdWebPassword implements set/clear/status. stdin is injected for tests.
func cmdWebPassword(args []string, stdin io.Reader, stdout, stderr io.Writer) int {
	if len(args) == 0 {
		fmt.Fprintf(stderr, "usage: %s web password <set [<value>] | clear | status>\n", brand.CLI)
		return 2
	}
	switch args[0] {
	case "set":
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
	case "clear", "reset", "rm", "unset":
		return applyWebPassword("", stdout, stderr)
	case "status":
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
	default:
		fmt.Fprintf(stderr, "%s web password: unknown subcommand %q (set, clear, status)\n", brand.CLI, args[0])
		return 2
	}
}

// readPromptLine reads one line off a SHARED bufio.Reader (the two password
// prompts must not each wrap stdin — a second bufio.NewReader would lose the
// bytes the first one buffered ahead), trimming the trailing newline. EOF with
// content is fine (piped input without a final newline).
func readPromptLine(r *bufio.Reader) (string, error) {
	line, err := r.ReadString('\n')
	if err != nil && err != io.EOF {
		return "", err
	}
	return strings.TrimRight(line, "\r\n"), nil
}

// applyWebPassword stores value ("" = clear) — live through the daemon when
// it's up, directly into the vault when it isn't.
func applyWebPassword(value string, stdout, stderr io.Writer) int {
	verbed := "set"
	if value == "" {
		verbed = "cleared"
	}

	// Daemon-up path: one config_set round-trip persists to the vault AND
	// applies live (M933's lazy gate re-reads the env), so the next login uses
	// the new password immediately. Any dial/call failure (daemon down, stale
	// runtime files) falls through to the offline path.
	if base, err := paths.BaseDir(); err == nil {
		if c, cerr := controlplane.NewClient(base); cerr == nil {
			ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
			res, callErr := c.Call(ctx, controlplane.CmdConfigSet, map[string]any{
				"name": webPasswordEnv, "value": value,
			})
			cancel()
			if callErr == nil {
				fmt.Fprintf(stdout, "console password %s (applied live — effective for the next login)\n", verbed)
				if pinned, _ := res["env_pinned"].(bool); pinned {
					fmt.Fprintf(stdout, "note: %s is pinned by the daemon's real environment — the saved value takes over once that pin is removed\n", webPasswordEnv)
				}
				return 0
			}
		}
	}

	// Offline path: write the vault directly (encrypted at rest, M934).
	store, err := openCredsStore(stderr)
	if err != nil {
		return 1
	}
	if value == "" {
		store.Remove(webPasswordEnv)
	} else if err := store.Set(webPasswordEnv, value); err != nil {
		fmt.Fprintf(stderr, "%s: %v\n", brand.CLI, err)
		return 1
	}
	if err := store.Save(); err != nil {
		fmt.Fprintf(stderr, "%s: save vault: %v\n", brand.CLI, err)
		return 1
	}
	fmt.Fprintf(stdout, "console password %s in %s (daemon not running — takes effect on the next start)\n", verbed, store.Path)
	return 0
}
