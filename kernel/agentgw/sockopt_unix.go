// SPDX-License-Identifier: MIT

//go:build unix

package agentgw

import "syscall"

// setSockOpt sets SO_REUSEADDR on the socket so that consecutive test
// runs (-count=N) don't fail with "address already in use" when the
// previous listener's socket hasn't fully released yet.
func setSockOpt(network, address string, c syscall.RawConn) error {
	var sockErr error
	err := c.Control(func(fd uintptr) {
		sockErr = syscall.SetsockoptInt(int(fd), syscall.SOL_SOCKET, syscall.SO_REUSEADDR, 1)
	})
	if err != nil {
		return err
	}
	return sockErr
}
