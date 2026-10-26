// SPDX-License-Identifier: MIT

package mcpapi

// Provenance: the MCP client's ROWS and its PORTS (W3.58). The extensions
//             module keeps the implementations — the stdio client, the HTTP
//             client and the durable registry — but the shapes the layers
//             above it speak are the contract's, so a caller may hold them
//             without naming the module.
//
// The split is deliberate:
//   - ToolDef is a ROW (the description of one advertised tool);
//   - Conn is a PORT (a live attachment);
//   - Dialer and HTTPDialer are SEAMS (how an attachment is made);
//   - Registry is the durable store's PORT.
//
// The registry's FILE is the daemon's (W3.58): the composition root opens it
// and hands the kernel the port.

import (
	"context"
	"encoding/json"
	"errors"
)

// ToolDef is one tool an attached server advertises.
type ToolDef struct {
	Name        string          `json:"name"`
	Description string          `json:"description,omitempty"`
	InputSchema json.RawMessage `json:"inputSchema,omitempty"`
}

// Conn is a live attachment to one MCP server.
//
// It is an interface so the layers above the module never depend on a real
// child process or a real HTTP endpoint — the tests hand in fakes, and the
// module's own clients satisfy it.
type Conn interface {
	// Tools returns the tool list discovered at attach time.
	Tools() []ToolDef
	// Call forwards one tools/call and returns the flattened text content
	// plus the server's own isError verdict.
	Call(ctx context.Context, tool string, args json.RawMessage) (string, bool, error)
	// Close detaches: asks the child to exit, then kills it. Idempotent.
	Close() error
}

// Dialer spawns + handshakes one stdio server.
//
// The layers above the module take it as a seam so tests can attach fakes;
// the module's Dial is the production implementation. env is the server's
// opt-in extra environment, injected on top of the scrubbed base.
type Dialer func(ctx context.Context, command string, args []string, env map[string]string) (Conn, error)

// HTTPDialer handshakes one REMOTE server over Streamable HTTP.
//
// The layers above the module take it as a seam so tests can attach fakes;
// the module's DialHTTP is the production implementation. headers are the
// operator's opt-in request headers (e.g. an Authorization bearer token),
// applied to every request.
type HTTPDialer func(ctx context.Context, url string, headers map[string]string) (Conn, error)

// ErrNotFound is returned for an unknown server id/name.
//
// The sentinel lives in the contract because the layers above the module
// compare against it: the app layer turns it into "unknown mcp server", and a
// module that raised a look-alike error of its own would leave every
// errors.Is comparison silently false.
var ErrNotFound = errors.New("mcp: server not found")

// Registry is the durable MCP-server registry — the store port.
//
// Six verbs, the registry's whole surface: the lifecycle (add, enable, remove)
// and the reads (get, list, count). A wider port would hand a reader the
// lifecycle; a narrower one would force a second shape to be kept in step
// with this one.
//
// The registry is NOT bus-bound: the lifecycle journaling lives one layer up
// (the kernel presses the journal), so there is no publisher port and no
// builder to inject — the daemon opens the file and hands over the store, and
// that is the whole handover.
type Registry interface {
	// Add validates and persists a new server (enabled by default).
	Add(srv Server) (Server, error)
	// SetEnabled flips auto-attach-at-start for a server by id or name.
	SetEnabled(ref string, enabled bool) (Server, error)
	// Remove deletes a server by id or name; the bool says whether it was
	// there.
	Remove(ref string) (Server, bool, error)
	// Get returns one server by id or name; the bool says whether it is
	// there.
	Get(ref string) (Server, bool)
	// List returns every registered server, sorted by creation time then id.
	List() []Server
	// Count returns the number of registered servers.
	Count() int
}

// RegistrationReader is the read-only half of the registry — the port the
// catalog layer holds.
//
// The catalog only LISTS: a reader that could also add or remove a server
// would be handed the lifecycle it has no business driving. The count is
// not part of this port either — the catalog counts the rows it holds, so a
// second count would be a second answer to the same question.
type RegistrationReader interface {
	// List returns every registered server, sorted by creation time then id.
	List() []Server
}
