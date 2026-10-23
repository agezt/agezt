// SPDX-License-Identifier: MIT

// Package mcpapi is the contract layer's home for the MCP server row: the
// description of one registered MCP server — a stdio command or a remote HTTP
// endpoint the daemon can spawn and speak MCP to.
//
// The row lives here because the tools and the marketplace that name it are
// PLUGINS, and a plugin may reach the contract and the platform layers and
// nothing else: a row a plugin has to name cannot live in a module's
// implementation package. The extensions module keeps the name as an alias,
// and the registry — the store that persists these rows — stays in the module.
//
// The row's rules travel with it: Validate checks a server's user-supplied
// fields, and it is pure, so it belongs where the row is.
package mcpapi

import (
	"errors"
	"fmt"
	"net/url"
	"regexp"
	"strings"
)

// Server is one registered MCP server: a stdio command the daemon can spawn
// and speak MCP to. Name is the immutable handle and the tool-name prefix
// segment (mcp_<name>_<tool>) — no underscores, so the Edict toolmap can
// parse the server back out of a tool name unambiguously.
type Server struct {
	ID string `json:"id"`
	// Name is the server's immutable handle: lowercase letters/digits only
	// (it becomes a tool-name segment, parsed by prefix).
	Name string `json:"name"`
	// Command + Args spawn the server process (stdio transport), e.g.
	// "npx" ["-y","@modelcontextprotocol/server-everything"]. Mutually
	// exclusive with URL: a server is either a local process (stdio) or a
	// remote endpoint (http).
	Command string   `json:"command,omitempty"`
	Args    []string `json:"args,omitempty"`
	// URL, when set, makes this a REMOTE server over the MCP Streamable HTTP
	// transport (M904, #39) — JSON-RPC POSTed to this endpoint — instead of a
	// spawned process. Exactly one of Command / URL must be set.
	URL string `json:"url,omitempty"`
	// Enabled means "attach automatically when the daemon starts". A live
	// attach/detach is a runtime operation independent of this flag.
	Enabled     bool   `json:"enabled"`
	Description string `json:"description,omitempty"`
	// Env is an OPT-IN set of environment variables injected into THIS server's
	// process on attach (M898) — e.g. {"GITHUB_PERSONAL_ACCESS_TOKEN": "..."}.
	// The base environment stays scrubbed (the daemon's own AGEZT_*/secret vars
	// never leak); only these explicit, operator-supplied entries are added, so a
	// credentialed server (github/brave/slack) can get exactly the key it needs.
	// Stored like Args (plaintext in the registry) — use a dedicated low-scope
	// token. Redacted out of read APIs.
	Env map[string]string `json:"env,omitempty"`
	// Headers is an OPT-IN set of HTTP request headers for a remote (URL)
	// server (M904) — e.g. {"Authorization": "Bearer ..."}. Applied to every
	// request including the handshake. Like Env, these may be secrets, so they
	// are stored plaintext in the registry but redacted out of read APIs (only
	// the key names are exposed). Meaningless for stdio servers.
	Headers map[string]string `json:"headers,omitempty"`
	// Lazy, when true, collapses this server's tools into a SINGLE dispatcher
	// tool (mcp_<name>) for context efficiency (M906) instead of injecting every
	// tool's full input schema into every run. The dispatcher takes
	// {tool, arguments}; the run picks a tool name (the dispatcher lists them)
	// and the remote server validates the arguments. Opt-in — eager injection is
	// the default. Composes with ToolAllow (the dispatcher only lists/forwards
	// allowlisted tools).
	Lazy bool `json:"lazy,omitempty"`
	// ToolAllow is an OPT-IN allowlist of the server's own tool names to expose
	// to runs (M899) — context-efficient MCP management. A chatty server (github
	// exposes ~30 tools) can be trimmed to the few a run actually needs, so its
	// schemas don't bloat every run's context. Empty = expose all the server's
	// tools (the default). Names are the server's bare tool names (not the
	// mcp_<name>_<tool> prefix).
	ToolAllow []string `json:"tool_allow,omitempty"`
	CreatedMS int64    `json:"created_ms"`
	UpdatedMS int64    `json:"updated_ms"`
}

// nameRe: lowercase letter first, then letters/digits. Deliberately NO
// underscore/dash — the name is parsed out of mcp_<name>_<tool> by the
// Edict toolmap, so it must never contain the separator.
var nameRe = regexp.MustCompile(`^[a-z][a-z0-9]{0,15}$`)

// envKeyRe: a POSIX-ish environment variable name (letters/digits/underscore,
// not starting with a digit). Keeps a malformed key from reaching exec.
var envKeyRe = regexp.MustCompile(`^[A-Za-z_][A-Za-z0-9_]*$`)

// headerNameRe: an HTTP field-name (RFC 7230 token). Keeps a malformed header
// name from reaching the request.
var headerNameRe = regexp.MustCompile(`^[A-Za-z0-9!#$%&'*+\-.^_` + "`" + `|~]+$`)

// MaxArgs caps how many arguments one server may carry.
const MaxArgs = 32

// MaxEnv caps how many env vars one server may carry (a sanity bound, not a
// security boundary).
const MaxEnv = 32

// MaxHeaders caps how many request headers a remote server may carry.
const MaxHeaders = 32

// MaxToolAllow caps the per-server tool allowlist length.
const MaxToolAllow = 128

// Validate checks a server's user-supplied fields.
func Validate(s Server) error {
	if !nameRe.MatchString(s.Name) {
		return fmt.Errorf("mcp: name must match %s (it becomes the mcp_<name>_* tool prefix)", nameRe)
	}
	// Transport: exactly one of Command (stdio) or URL (remote http).
	hasCmd := strings.TrimSpace(s.Command) != ""
	hasURL := strings.TrimSpace(s.URL) != ""
	switch {
	case hasCmd && hasURL:
		return errors.New("mcp: set either command (stdio) or url (http), not both")
	case !hasCmd && !hasURL:
		return errors.New("mcp: command (stdio) or url (http) is required")
	case hasURL:
		u, err := url.Parse(strings.TrimSpace(s.URL))
		if err != nil {
			return fmt.Errorf("mcp: invalid url: %w", err)
		}
		if u.Scheme != "http" && u.Scheme != "https" {
			return errors.New("mcp: url must be http or https")
		}
		if u.Host == "" {
			return errors.New("mcp: url must have a host")
		}
	}
	if len(s.Args) > MaxArgs {
		return fmt.Errorf("mcp: at most %d args", MaxArgs)
	}
	for _, a := range s.Args {
		if strings.TrimSpace(a) == "" {
			return errors.New("mcp: empty arg")
		}
	}
	if len(s.Headers) > MaxHeaders {
		return fmt.Errorf("mcp: at most %d headers", MaxHeaders)
	}
	for k := range s.Headers {
		if !headerNameRe.MatchString(k) {
			return fmt.Errorf("mcp: invalid header name %q", k)
		}
	}
	if len(s.Env) > MaxEnv {
		return fmt.Errorf("mcp: at most %d env vars", MaxEnv)
	}
	for k := range s.Env {
		if !envKeyRe.MatchString(k) {
			return fmt.Errorf("mcp: invalid env var name %q", k)
		}
	}
	if len(s.ToolAllow) > MaxToolAllow {
		return fmt.Errorf("mcp: at most %d allowed tools", MaxToolAllow)
	}
	for _, t := range s.ToolAllow {
		if strings.TrimSpace(t) == "" {
			return errors.New("mcp: empty tool name in allowlist")
		}
	}
	return nil
}
