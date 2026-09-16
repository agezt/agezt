// SPDX-License-Identifier: MIT
//
// cmd/agt `exec-profile` typed helpers (callExecProfile, tenantArg, dashJoin).
// Extracted from execution_profile.go during Day 211 god-file refactor (#94).
// Public API unchanged.
package main

import (
	"context"
	"fmt"
	"io"
	"strings"
	"time"

	dialpkg "github.com/agezt/agezt/cmd/agt/dial"
	"github.com/agezt/agezt/internal/brand"
)

func callExecProfile(cmd string, args map[string]any, stderr io.Writer) (map[string]any, bool) {
	c := dialpkg.New(stderr)
	if c == nil {
		return nil, false
	}
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	res, err := c.Call(ctx, cmd, args)
	if err != nil {
		fmt.Fprintf(stderr, "%s exec-profile: %v\n", brand.CLI, err)
		return nil, false
	}
	return res, true
}
func tenantArg(tenant string) map[string]any {
	out := map[string]any{}
	if tenant = strings.TrimSpace(tenant); tenant != "" {
		out["tenant"] = tenant
	}
	return out
}
func dashJoin(items []string) string {
	if len(items) == 0 {
		return "-"
	}
	return strings.Join(items, ", ")
}
