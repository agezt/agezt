// SPDX-License-Identifier: MIT

//go:build !unix

package agentgw

import "syscall"

// setSockOpt is a no-op on non-Unix platforms (Windows). SO_REUSEADDR
// has different semantics there and Go sets it by default for TCP.
func setSockOpt(network, address string, c syscall.RawConn) error {
	return nil
}
