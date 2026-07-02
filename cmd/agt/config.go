// SPDX-License-Identifier: MIT

package main

// `agt config show` — the operator's dashboard for "what is this
// daemon ACTUALLY running with?" The handler returns resolved
// paths, model, system-prompt presence, tool/plugin counts,
// ask-policy, and which AGEZT_* env vars are set. Values are
// never returned (presence only) so the JSON view is safe to
// paste into bug reports.

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"os"
	"sort"
	"strings"
	"time"

	"github.com/agezt/agezt/internal/brand"
	"github.com/agezt/agezt/kernel/controlplane"
)

func cmdConfig(args []string, stdout, stderr io.Writer) int {
	if len(args) == 0 {
		fmt.Fprintf(stderr, "%s config: subcommand required (show, ls, get, set, schema)\n", brand.CLI)
		return 2
	}
	switch args[0] {
	case "show":
		return cmdConfigShow(args[1:], stdout, stderr)
	case "ls":
		return cmdConfigLs(args[1:], stdout, stderr)
	case "get":
		return cmdConfigGet(args[1:], stdout, stderr)
	case "set":
		return cmdConfigSet(args[1:], stdout, stderr)
	case "schema":
		return cmdConfigSchema(args[1:], stdout, stderr)
	case "-h", "--help":
		fmt.Fprintf(stdout, "usage: %s config <subcommand>\n\n", brand.CLI)
		fmt.Fprintf(stdout, "  show [--json]            resolved config (paths, model, env presence)\n")
		fmt.Fprintf(stdout, "  ls [--json]              every Config Center setting + its state\n")
		fmt.Fprintf(stdout, "  get <ENV>                one setting's value (secrets: presence only)\n")
		fmt.Fprintf(stdout, "  set <ENV> <value>        write a setting (live for provider/model, else restart)\n")
		fmt.Fprintf(stdout, "  schema [--json]          list the editable schema (built-in + registered)\n")
		fmt.Fprintf(stdout, "  schema register <file>   register a skill/plugin schema section (JSON)\n")
		fmt.Fprintf(stdout, "  schema unregister <id>   remove a registered schema section\n")
		return 0
	default:
		fmt.Fprintf(stderr, "%s config: unknown subcommand %q (show, ls, get, set, schema)\n", brand.CLI, args[0])
		return 2
	}
}

// renderRoutingTable prints a "task → [providers]" table for routes / requires.
func renderRoutingTable(stdout io.Writer, label string, raw any) {
	m, ok := raw.(map[string]any)
	if !ok || len(m) == 0 {
		return
	}
	fmt.Fprintf(stdout, "    %s:\n", label)
	for _, k := range sortedKeys(m) {
		provs, _ := m[k].([]any)
		names := make([]string, 0, len(provs))
		for _, p := range provs {
			if s, ok := p.(string); ok {
				names = append(names, s)
			}
		}
		fmt.Fprintf(stdout, "      %-12s → %v\n", k, names)
	}
}

// sortedKeys returns the keys of a map[string]any sorted for stable output.
func sortedKeys(m map[string]any) []string {
	keys := make([]string, 0, len(m))
	for k := range m {
		keys = append(keys, k)
	}
	sort.Strings(keys)
	return keys
}

func cmdConfigShow(args []string, stdout, stderr io.Writer) int {
	asJSON := false
	for _, a := range args {
		switch a {
		case "--json":
			asJSON = true
		case "-h", "--help":
			fmt.Fprintf(stdout, "usage: %s config show [--json]\n", brand.CLI)
			fmt.Fprintf(stdout, "snapshot of the daemon's effective config — what AGEZT_* env vars,\n")
			fmt.Fprintf(stdout, "model, system prompt (presence only), and on-disk paths it's using.\n")
			fmt.Fprintf(stdout, "  --json   emit the full snapshot (pipe to jq / CI parsers)\n")
			return 0
		default:
			fmt.Fprintf(stderr, "%s config show: unexpected arg %q\n", brand.CLI, a)
			return 2
		}
	}

	c := dial(stderr)
	if c == nil {
		return 1
	}
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	res, err := c.Call(ctx, controlplane.CmdConfig, nil)
	if err != nil {
		fmt.Fprintf(stderr, "%s config show: %v\n", brand.CLI, err)
		return 1
	}

	if asJSON {
		enc := json.NewEncoder(stdout)
		enc.SetIndent("", "  ")
		_ = enc.Encode(res)
		return 0
	}

	paths, _ := res["paths"].(map[string]any)
	model, _ := res["model"].(string)
	sysSet, _ := res["system_prompt_set"].(bool)
	askPolicy, _ := res["ask_policy"].(string)
	toolCount := intOfStatus(res["tool_count"])
	pluginCount := intOfStatus(res["plugin_count"])
	envMap, _ := res["env"].(map[string]any)

	fmt.Fprintf(stdout, "%s config:\n", brand.CLI)
	fmt.Fprintf(stdout, "  paths:\n")
	// Stable path key order so consecutive runs look identical.
	for _, k := range []string{"base", "journal", "state", "runtime", "catalog", "vault"} {
		if v, ok := paths[k].(string); ok {
			fmt.Fprintf(stdout, "    %-8s : %s\n", k, v)
		}
	}
	if model == "" {
		fmt.Fprintf(stdout, "  model           : (provider default)\n")
	} else {
		fmt.Fprintf(stdout, "  model           : %s\n", model)
	}
	if sysSet {
		fmt.Fprintf(stdout, "  system prompt   : set (content not shown)\n")
	} else {
		fmt.Fprintf(stdout, "  system prompt   : unset\n")
	}
	fmt.Fprintf(stdout, "  ask_policy      : %s\n", askPolicy)
	fmt.Fprintf(stdout, "  tools           : %d registered\n", toolCount)
	fmt.Fprintf(stdout, "  plugins         : %d spawned\n", pluginCount)

	// Effective routing tables (M108) — only present when configured, so the
	// common no-routing daemon stays compact.
	if routing, ok := res["routing"].(map[string]any); ok && len(routing) > 0 {
		fmt.Fprintf(stdout, "  routing (effective):\n")
		renderRoutingTable(stdout, "routes", routing["routes"])
		renderRoutingTable(stdout, "requires", routing["requires"])
		if ov, ok := routing["model_overrides"].(map[string]any); ok && len(ov) > 0 {
			fmt.Fprintf(stdout, "    model_overrides:\n")
			for _, k := range sortedKeys(ov) {
				if m, _ := ov[k].(string); m != "" {
					fmt.Fprintf(stdout, "      %-12s → %s\n", k, m)
				}
			}
		}
	}

	if len(envMap) == 0 {
		fmt.Fprintf(stdout, "  env (AGEZT_*)   : none set\n")
		return 0
	}
	keys := make([]string, 0, len(envMap))
	for k := range envMap {
		keys = append(keys, k)
	}
	sort.Strings(keys)
	fmt.Fprintf(stdout, "  env (AGEZT_*)   : %d set (values not shown)\n", len(keys))
	for _, k := range keys {
		fmt.Fprintf(stdout, "    %s\n", k)
	}
	return 0
}

// configValues fetches the Config Center field state (CmdConfigValues). Secrets
// report presence only — the value never leaves the daemon.
func configValues(c *controlplane.Client, stderr io.Writer, label string) ([]any, bool) {
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	res, err := c.Call(ctx, controlplane.CmdConfigValues, nil)
	if err != nil {
		fmt.Fprintf(stderr, "%s %s: %v\n", brand.CLI, label, err)
		return nil, false
	}
	fields, _ := res["fields"].([]any)
	return fields, true
}

// cmdConfigLs lists every Config Center setting and its current state — the
// machine-friendly companion to `config show` that the `config` skill/tool and
// operators use to see what's editable.
func cmdConfigLs(args []string, stdout, stderr io.Writer) int {
	asJSON := false
	for _, a := range args {
		switch a {
		case "--json":
			asJSON = true
		case "-h", "--help":
			fmt.Fprintf(stdout, "usage: %s config ls [--json]\n", brand.CLI)
			return 0
		default:
			fmt.Fprintf(stderr, "%s config ls: unexpected arg %q\n", brand.CLI, a)
			return 2
		}
	}
	c := dial(stderr)
	if c == nil {
		return 1
	}
	if asJSON {
		ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer cancel()
		res, err := c.Call(ctx, controlplane.CmdConfigValues, nil)
		if err != nil {
			fmt.Fprintf(stderr, "%s config ls: %v\n", brand.CLI, err)
			return 1
		}
		enc := json.NewEncoder(stdout)
		enc.SetIndent("", "  ")
		_ = enc.Encode(res)
		return 0
	}
	fields, ok := configValues(c, stderr, "config ls")
	if !ok {
		return 1
	}
	fmt.Fprintf(stdout, "%s config (%d settings):\n", brand.CLI, len(fields))
	for _, fi := range fields {
		m, _ := fi.(map[string]any)
		env, _ := m["env"].(string)
		secret, _ := m["secret"].(bool)
		set, _ := m["set"].(bool)
		pinned, _ := m["env_pinned"].(bool)
		state := "unset"
		if secret {
			if set {
				state = "set (secret)"
			} else {
				state = "unset (secret)"
			}
		} else if val, _ := m["value"].(string); val != "" {
			state = val
		}
		tag := ""
		if pinned {
			tag = " [env-pinned]"
		}
		fmt.Fprintf(stdout, "  %-34s %s%s\n", env, state, tag)
	}
	return 0
}

// cmdConfigGet prints one setting's value (secrets: presence only).
func cmdConfigGet(args []string, stdout, stderr io.Writer) int {
	var env string
	for _, a := range args {
		switch a {
		case "-h", "--help":
			fmt.Fprintf(stdout, "usage: %s config get <ENV>\n", brand.CLI)
			return 0
		default:
			if env != "" {
				fmt.Fprintf(stderr, "%s config get: unexpected arg %q\n", brand.CLI, a)
				return 2
			}
			env = a
		}
	}
	if env == "" {
		fmt.Fprintf(stderr, "%s config get: ENV required\n", brand.CLI)
		return 2
	}
	c := dial(stderr)
	if c == nil {
		return 1
	}
	fields, ok := configValues(c, stderr, "config get")
	if !ok {
		return 1
	}
	for _, fi := range fields {
		m, _ := fi.(map[string]any)
		if m["env"] != env {
			continue
		}
		secret, _ := m["secret"].(bool)
		set, _ := m["set"].(bool)
		if secret {
			if set {
				fmt.Fprintf(stdout, "%s: set (secret, value not shown)\n", env)
			} else {
				fmt.Fprintf(stdout, "%s: not set\n", env)
			}
		} else {
			val, _ := m["value"].(string)
			fmt.Fprintf(stdout, "%s=%s\n", env, val)
		}
		return 0
	}
	fmt.Fprintf(stderr, "%s config get: unknown setting %q\n", brand.CLI, env)
	return 1
}

// cmdConfigSet writes one setting. An empty value clears it. Provider/model apply
// live; everything else needs a restart (the response says which).
func cmdConfigSet(args []string, stdout, stderr io.Writer) int {
	if len(args) >= 1 && (args[0] == "-h" || args[0] == "--help") {
		fmt.Fprintf(stdout, "usage: %s config set <ENV> [value]\n", brand.CLI)
		fmt.Fprintf(stdout, "  omit value (or pass \"\") to clear the setting\n")
		return 0
	}
	if len(args) < 1 {
		fmt.Fprintf(stderr, "%s config set: ENV required (usage: config set <ENV> [value])\n", brand.CLI)
		return 2
	}
	env := args[0]
	value := strings.Join(args[1:], " ")
	c := dial(stderr)
	if c == nil {
		return 1
	}
	ctx, cancel := context.WithTimeout(context.Background(), 15*time.Second)
	defer cancel()
	if _, cerr := saveConfigSettingRollbackCheckpoint(ctx, c, "config.set", env); cerr != nil {
		fmt.Fprintf(stderr, "%s config set: checkpoint: %v\n", brand.CLI, cerr)
		return 1
	}
	res, err := c.Call(ctx, controlplane.CmdConfigSet, map[string]any{"name": env, "value": value})
	if err != nil {
		fmt.Fprintf(stderr, "%s config set: %v\n", brand.CLI, err)
		return 1
	}
	if pinned, _ := res["env_pinned"].(bool); pinned {
		fmt.Fprintf(stdout, "%s saved, but pinned by the environment — unset it in .env to apply\n", env)
		return 0
	}
	switch applied, _ := res["applied"].(string); applied {
	case "live":
		fmt.Fprintf(stdout, "%s applied live\n", env)
	default:
		fmt.Fprintf(stdout, "%s saved — restart to apply\n", env)
	}
	return 0
}

// cmdConfigSchema lists the editable schema, or dispatches register/unregister.
func cmdConfigSchema(args []string, stdout, stderr io.Writer) int {
	if len(args) >= 1 {
		switch args[0] {
		case "register":
			return cmdConfigSchemaRegister(args[1:], stdout, stderr)
		case "unregister":
			return cmdConfigSchemaUnregister(args[1:], stdout, stderr)
		}
	}
	asJSON := false
	for _, a := range args {
		switch a {
		case "--json":
			asJSON = true
		case "-h", "--help":
			fmt.Fprintf(stdout, "usage: %s config schema [--json | register <file> | unregister <id>]\n", brand.CLI)
			return 0
		default:
			fmt.Fprintf(stderr, "%s config schema: unexpected arg %q\n", brand.CLI, a)
			return 2
		}
	}
	c := dial(stderr)
	if c == nil {
		return 1
	}
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	res, err := c.Call(ctx, controlplane.CmdConfigSchema, nil)
	if err != nil {
		fmt.Fprintf(stderr, "%s config schema: %v\n", brand.CLI, err)
		return 1
	}
	if asJSON {
		enc := json.NewEncoder(stdout)
		enc.SetIndent("", "  ")
		_ = enc.Encode(res)
		return 0
	}
	sections, _ := res["sections"].([]any)
	for _, si := range sections {
		s, _ := si.(map[string]any)
		id, _ := s["id"].(string)
		name, _ := s["name"].(string)
		source, _ := s["source"].(string)
		tag := ""
		if source != "" && source != "builtin" {
			tag = " (registered)"
		}
		fmt.Fprintf(stdout, "[%s] %s%s\n", id, name, tag)
		flds, _ := s["fields"].([]any)
		for _, fld := range flds {
			f, _ := fld.(map[string]any)
			env, _ := f["env"].(string)
			typ, _ := f["type"].(string)
			label, _ := f["label"].(string)
			sec := ""
			if secret, _ := f["secret"].(bool); secret {
				sec = " (secret)"
			}
			fmt.Fprintf(stdout, "  %-34s %-8s%s  %s\n", env, typ, sec, label)
		}
	}
	return 0
}

// cmdConfigSchemaRegister reads a JSON schema section from a file and registers it
// — the "a skill drops a schema into the Config Center" path. The daemon validates
// it (slug id, namespaced AGEZT_* fields, no shadowing of a built-in).
func cmdConfigSchemaRegister(args []string, stdout, stderr io.Writer) int {
	var file string
	for _, a := range args {
		switch a {
		case "-h", "--help":
			fmt.Fprintf(stdout, "usage: %s config schema register <file.json>\n", brand.CLI)
			return 0
		default:
			if file != "" {
				fmt.Fprintf(stderr, "%s config schema register: unexpected arg %q\n", brand.CLI, a)
				return 2
			}
			file = a
		}
	}
	if file == "" {
		fmt.Fprintf(stderr, "%s config schema register: FILE required\n", brand.CLI)
		return 2
	}
	raw, err := os.ReadFile(file)
	if err != nil {
		fmt.Fprintf(stderr, "%s config schema register: %v\n", brand.CLI, err)
		return 1
	}
	var section map[string]any
	if err := json.Unmarshal(raw, &section); err != nil {
		fmt.Fprintf(stderr, "%s config schema register: invalid JSON: %v\n", brand.CLI, err)
		return 1
	}
	c := dial(stderr)
	if c == nil {
		return 1
	}
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	res, err := c.Call(ctx, controlplane.CmdConfigSchemaRegister, map[string]any{"section": section})
	if err != nil {
		fmt.Fprintf(stderr, "%s config schema register: %v\n", brand.CLI, err)
		return 1
	}
	id, _ := res["id"].(string)
	fmt.Fprintf(stdout, "registered schema section %q\n", id)
	return 0
}

// cmdConfigSchemaUnregister removes a registered schema section by id (stored
// values are left untouched).
func cmdConfigSchemaUnregister(args []string, stdout, stderr io.Writer) int {
	var id string
	force := false
	for _, a := range args {
		switch a {
		case "-h", "--help":
			fmt.Fprintf(stdout, "usage: %s config schema unregister <id> [--force]\n", brand.CLI)
			fmt.Fprintf(stdout, "  --force   remove even a locked (system-approved) section\n")
			return 0
		case "--force":
			force = true
		default:
			if id != "" {
				fmt.Fprintf(stderr, "%s config schema unregister: unexpected arg %q\n", brand.CLI, a)
				return 2
			}
			id = a
		}
	}
	if id == "" {
		fmt.Fprintf(stderr, "%s config schema unregister: ID required\n", brand.CLI)
		return 2
	}
	c := dial(stderr)
	if c == nil {
		return 1
	}
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	res, err := c.Call(ctx, controlplane.CmdConfigSchemaUnregister, map[string]any{"id": id, "force": force})
	if err != nil {
		fmt.Fprintf(stderr, "%s config schema unregister: %v\n", brand.CLI, err)
		return 1
	}
	if removed, _ := res["removed"].(bool); removed {
		fmt.Fprintf(stdout, "unregistered %q\n", id)
	} else {
		fmt.Fprintf(stdout, "%q was not registered\n", id)
	}
	return 0
}
