// SPDX-License-Identifier: MIT
//
// cmd/agt okr mutation sub-commands: cmdOKRCreate + cmdOKRKeyResult +
// cmdOKRLink + cmdOKRArchive.
// Extracted from okr.go during the Day-209 god-file split.
// Public API unchanged.
package main

import (
	"fmt"
	"io"
	"strconv"
	"strings"

	"github.com/agezt/agezt/internal/brand"
	"github.com/agezt/agezt/kernel/controlplane"
)

func cmdOKRCreate(args []string, stdout, stderr io.Writer) int {
	asJSON := false
	callArgs := map[string]any{}
	for i := 0; i < len(args); i++ {
		a := args[i]
		switch a {
		case "--json":
			asJSON = true
		case "--title", "--desc", "--description", "--owner", "--tenant":
			val, ok := okrFlagValue(args, &i, a, stderr)
			if !ok {
				return 2
			}
			switch a {
			case "--title":
				callArgs["title"] = val
			case "--desc", "--description":
				callArgs["description"] = val
			case "--owner":
				callArgs["owner"] = val
			case "--tenant":
				callArgs["tenant"] = val
			}
		default:
			if strings.HasPrefix(a, "-") {
				fmt.Fprintf(stderr, "%s okr create: unexpected flag %q\n", brand.CLI, a)
				return 2
			}
			if _, exists := callArgs["title"]; !exists {
				callArgs["title"] = a
				continue
			}
			fmt.Fprintf(stderr, "%s okr create: unexpected arg %q\n", brand.CLI, a)
			return 2
		}
	}
	if str(callArgs["title"]) == "" {
		fmt.Fprintf(stderr, "usage: %s okr create --title T\n", brand.CLI)
		return 2
	}
	res, code := callOKR(controlplane.CmdOKRCreate, callArgs, stderr)
	return renderOKRMutation(res, code, asJSON, stdout)
}

func cmdOKRKeyResult(args []string, stdout, stderr io.Writer) int {
	asJSON := false
	callArgs := map[string]any{}
	for i := 0; i < len(args); i++ {
		a := args[i]
		switch a {
		case "--json":
			asJSON = true
		case "--title", "--target":
			val, ok := okrFlagValue(args, &i, a, stderr)
			if !ok {
				return 2
			}
			if a == "--title" {
				callArgs["title"] = val
			} else {
				n, err := strconv.Atoi(val)
				if err != nil || n < 0 {
					fmt.Fprintf(stderr, "%s okr kr: --target needs a non-negative integer\n", brand.CLI)
					return 2
				}
				callArgs["target"] = n
			}
		default:
			if strings.HasPrefix(a, "-") {
				fmt.Fprintf(stderr, "%s okr kr: unexpected flag %q\n", brand.CLI, a)
				return 2
			}
			if _, exists := callArgs["id"]; !exists {
				callArgs["id"] = a
				continue
			}
			fmt.Fprintf(stderr, "%s okr kr: unexpected arg %q\n", brand.CLI, a)
			return 2
		}
	}
	if str(callArgs["id"]) == "" || str(callArgs["title"]) == "" {
		fmt.Fprintf(stderr, "usage: %s okr kr <id> --title T [--target N]\n", brand.CLI)
		return 2
	}
	res, code := callOKR(controlplane.CmdOKRKeyResult, callArgs, stderr)
	return renderOKRMutation(res, code, asJSON, stdout)
}

func cmdOKRLink(args []string, stdout, stderr io.Writer, cmd, name string) int {
	asJSON := false
	callArgs := map[string]any{}
	for i := 0; i < len(args); i++ {
		a := args[i]
		switch a {
		case "--json":
			asJSON = true
		case "--kr", "--key-result", "--task":
			val, ok := okrFlagValue(args, &i, a, stderr)
			if !ok {
				return 2
			}
			if a == "--task" {
				callArgs["task"] = val
			} else {
				callArgs["key_result"] = val
			}
		default:
			if strings.HasPrefix(a, "-") {
				fmt.Fprintf(stderr, "%s okr %s: unexpected flag %q\n", brand.CLI, name, a)
				return 2
			}
			if _, exists := callArgs["id"]; !exists {
				callArgs["id"] = a
				continue
			}
			fmt.Fprintf(stderr, "%s okr %s: unexpected arg %q\n", brand.CLI, name, a)
			return 2
		}
	}
	if str(callArgs["id"]) == "" || str(callArgs["key_result"]) == "" || str(callArgs["task"]) == "" {
		fmt.Fprintf(stderr, "usage: %s okr %s <id> --kr KR --task TASK\n", brand.CLI, name)
		return 2
	}
	res, code := callOKR(cmd, callArgs, stderr)
	return renderOKRMutation(res, code, asJSON, stdout)
}

func cmdOKRArchive(args []string, stdout, stderr io.Writer) int {
	id, asJSON, ok := okrIDArg(args, "archive", stderr)
	if !ok {
		return 2
	}
	res, code := callOKR(controlplane.CmdOKRArchive, map[string]any{"id": id}, stderr)
	return renderOKRMutation(res, code, asJSON, stdout)
}
