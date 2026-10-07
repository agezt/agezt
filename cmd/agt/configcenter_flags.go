// SPDX-License-Identifier: MIT
package main

import "strings"

func configCenterFlagValue(args []string, at int) (string, bool) {
	if at+1 >= len(args) || args[at+1] == "-h" || strings.HasPrefix(args[at+1], "--") {
		return "", false
	}
	return args[at+1], true
}
