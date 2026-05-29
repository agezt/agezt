// SPDX-License-Identifier: MIT

package main

import "os"

// readAll wraps os.ReadFile to keep the test helper trivially mockable later.
func readAll(p string) ([]byte, error) {
	return os.ReadFile(p)
}
