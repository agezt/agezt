// SPDX-License-Identifier: MIT

package tools

// The tools application's MCP BINDING — the ratchet of W3.76w.
//
// The MCP registry's lifecycle and the live attachments are the EXTENSIONS
// MODULE's. The application used to bind the DAEMON for both: the tools
// package's `MCPLifecycle` port was the kernel handle, and the kernel held
// seven forwarders over the module's own operations to answer it.
//
// Now the application binds the MODULE — the face the daemon hands over
// (`k.MCPRegistryService()`) — and the daemon spells none of the operations.
// The vocabulary is the contract's, so the module answers in the names the
// application's own port already speaks (`AddMCPServer`, `AttachMCPServer`,
// `DetachMCPServer`, `SetMCPServerEnabled`, `RemoveMCPServer`) with no
// adapter in between.
//
// The package still names the daemon for the READ gate over the registry
// (`MCP: k.MCPReader()`) and for the tool registry, so this ratchet is
// narrower than a whole-package one: it pins the MCP LIFECYCLE's binding.
//
// A ratchet that is never probed never fires (the lesson W3.76i paid for),
// so the probe is part of the slice: binding the daemon for the MCP lifecycle
// has to make this file fail.

import (
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"testing"
)

// daemonAsMCP is the binding this package must not carry any more: the MCP
// lifecycle (or the attachment status) taken from the daemon's handle.
//
// The value must BE the handle — `Attachment: k` — not a call on it
// (`Attachment: k.MCPRegistryService()`, which is the module's face).
var daemonAsMCP = regexp.MustCompile(`(MCPLifecycle|Attachment)\s*:\s*k\s*[,}]`)

// TestTheToolsApplicationBindsTheRegistryModuleNotTheDaemon pins the MCP
// lifecycle binding: the application asks the MODULE.
func TestTheToolsApplicationBindsTheRegistryModuleNotTheDaemon(t *testing.T) {
	entries, err := os.ReadDir(".")
	if err != nil {
		t.Fatalf("read the tools application package: %v", err)
	}
	for _, e := range entries {
		if e.IsDir() || !strings.HasSuffix(e.Name(), ".go") || strings.HasSuffix(e.Name(), "_test.go") {
			continue
		}
		src, err := os.ReadFile(filepath.Join(".", e.Name()))
		if err != nil {
			t.Fatalf("read %s: %v", e.Name(), err)
		}
		if daemonAsMCP.Match(src) {
			t.Errorf("%s takes the MCP lifecycle from the daemon — the tools application binds the extensions module's face, not the host (W3.76w)", e.Name())
		}
	}
}
