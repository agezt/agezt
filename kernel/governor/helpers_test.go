// SPDX-License-Identifier: MIT

package governor_test

import "os"

// osMkdirTempFn is the actual stdlib call; broken out so the test file
// using the helper doesn't pull os into the same file twice.
func osMkdirTempFn(dir, pattern string) (string, error) {
	return os.MkdirTemp(dir, pattern)
}
