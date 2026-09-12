// SPDX-License-Identifier: MIT

// agt config schema subcommand: Schema + SchemaRegister + SchemaUnregister.
// Code extracted from config.go during the Day-105 god-file split.
// Public API unchanged.
package main


import (
	"context"
	"fmt"
	"io"
	"os"
	"time"

	"encoding/json"
	dialpkg "github.com/agezt/agezt/cmd/agt/dial"
	"github.com/agezt/agezt/internal/brand"
	"github.com/agezt/agezt/kernel/controlplane"
)

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
	c := dialpkg.New(stderr)
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
	c := dialpkg.New(stderr)
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
	c := dialpkg.New(stderr)
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
