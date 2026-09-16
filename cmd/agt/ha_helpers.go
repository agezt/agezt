// SPDX-License-Identifier: MIT
//
// cmd/agt `ha` shared helpers (haCheck, printBodyJSON).
// Extracted from ha.go during Day 211 god-file refactor (#75).
// Public API unchanged.
package main

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strings"

	"github.com/agezt/agezt/internal/brand"
)

func haCheck(status int, body []byte, err error, stderr io.Writer) int {
	if err != nil {
		fmt.Fprintf(stderr, "%s ha: %v\n", brand.CLI, err)
		return 1
	}
	if status/100 != 2 {
		msg := strings.TrimSpace(string(body))
		if msg == "" {
			msg = http.StatusText(status)
		}
		fmt.Fprintf(stderr, "%s ha: HTTP %d: %s\n", brand.CLI, status, msg)
		return 1
	}
	return 0
}
func printBodyJSON(body []byte, raw bool, stdout io.Writer) int {
	if raw {
		fmt.Fprintln(stdout, strings.TrimRight(string(body), "\n"))
		return 0
	}
	var buf bytes.Buffer
	if err := json.Indent(&buf, body, "", "  "); err != nil {
		// Not JSON — echo as-is rather than failing.
		fmt.Fprintln(stdout, strings.TrimRight(string(body), "\n"))
		return 0
	}
	fmt.Fprintln(stdout, buf.String())
	return 0
}
