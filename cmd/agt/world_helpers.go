// SPDX-License-Identifier: MIT

// agt world command: dial/call helper (worldCall).
// Code extracted from world.go during the Day-132 god-file split.
// Public API unchanged.
package main



import (
	"context"
	"fmt"
	"io"
	"time"

	dialpkg "github.com/agezt/agezt/cmd/agt/dial"
	"github.com/agezt/agezt/cmd/agt/jsonout"
	"github.com/agezt/agezt/internal/brand"
)

func worldCall(cmd string, callArgs map[string]any, label string, stdout, stderr io.Writer, asJSON bool) map[string]any {
	c := dialpkg.New(stderr)
	if c == nil {
		return nil
	}
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	res, err := c.Call(ctx, cmd, callArgs)
	if err != nil {
		fmt.Fprintf(stderr, "%s %s: %v\n", brand.CLI, label, err)
		return nil
	}
	if asJSON {
		_ = jsonout.Write(stdout, res)
	}
	return res
}

// renderEntityLine formats an entity map (as returned over the wire) into a
// single human-readable line: "<id12> [kind] name (aka ...)".
