// SPDX-License-Identifier: MIT

package main

// Provenance: `agt version` — the client's identity and the protocol version
// it speaks. The command used to be a no-op stub in the registry while the
// root dispatcher answered "version" itself; now the registry answers it and
// the root dispatch keeps no list of its own.

import (
	"fmt"
	"io"

	"github.com/agezt/agezt/internal/brand"
)

// cmdVersion prints the client's identity.
func cmdVersion(args []string, stdout, stderr io.Writer) int {
	fmt.Fprintf(stdout, "%s %s (protocol v%d)\n", brand.CLI, brand.Version, brand.ProtocolVersion)
	return 0
}
