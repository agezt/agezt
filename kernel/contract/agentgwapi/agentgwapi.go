// SPDX-License-Identifier: MIT

// Package agentgwapi is the single home of the agent gateway's surface.
//
// The gateway is the daemon's door to agent subprocess code: a local socket
// (or a loopback port) that a subprocess uses to publish events, write
// memory, write logs, mint a token and read the config center.
//
// Its routes lived as string literals in kernel/agentgw/gateway.go, and its
// request bodies were read into whatever the handler happened to use — four
// of the five into an **anonymous** struct. So what happened to a mistyped
// body was decided by Go's encoding/json, and the answer named a Go struct:
//
//	{"event":42} → 400 "json: cannot unmarshal number into Go struct field .event of type string"
//	[]           → 400 "json: cannot unmarshal array into Go value of type struct { Event string … }"
//
// and a **null** was not refused at all: `{"event":null}` became an empty
// subject and the publish failed with a 500, `{"message":null}` was logged as
// an empty message.
//
// This package holds the surface as data:
//
//   - Routes() is what the gateway registers: path, method, the capability
//     the route needs, the query arguments it reads, and the request body it
//     takes.
//   - RequestSchema() is the JSON Schema of a route's body — the shape the
//     gateway's door measures an arriving body against.
//
// Three rules keep the schemas the operation registry's shape:
//
//  1. A schema names TYPES, not presence. The door checks types; a missing
//     field stays the handler's business ("key is required").
//  2. A schema allows an undeclared field. A body is the stable SUBSET of
//     what the daemon reads.
//  3. The type words are the registry's: an integer is a "number", a boolean
//     is a "boolean".
//
// The gateway is not part of the SDK contract: the SDKs speak `/api/v1`, and
// the callers of this surface are the daemon's own agents.
package agentgwapi

import "encoding/json"

// Route is one route of the gateway's surface.
type Route struct {
	// ID identifies the route ("eventbus_publish"). It is the operation name
	// the door's refusal carries: "operation eventbus_publish input: …".
	ID string
	// Method is the HTTP method the route answers.
	Method string
	// Path is the route's path ("/v1/eventbus/publish").
	Path string
	// Capability is the capability a caller needs for the route
	// ("eventbus.publish"). Empty when the route needs none.
	Capability string
	// Args are the query parameters the route reads.
	Args []string
	// Body is the JSON Schema of the request body the route takes, empty
	// when it takes none.
	Body json.RawMessage
	// Config marks a route the config center serves: the gateway registers
	// it only when a config center is wired.
	Config bool
	// Public marks a route that needs no token (the health probe).
	Public bool
}

// Route IDs of the gateway's operations.
const (
	// RouteEventbusSubscribe streams the bus's events to a subprocess.
	RouteEventbusSubscribe = "eventbus_subscribe"
	// RouteEventbusPublish publishes one event to the bus.
	RouteEventbusPublish = "eventbus_publish"
	// RouteMemoryWrite writes one memory record.
	RouteMemoryWrite = "memory_write"
	// RouteMemoryDelete forgets one memory record.
	RouteMemoryDelete = "memory_delete"
	// RouteMemorySearch searches the memory.
	RouteMemorySearch = "memory_search"
	// RouteLogRead reads one log entry.
	RouteLogRead = "log_read"
	// RouteLogWrite writes one log entry.
	RouteLogWrite = "log_write"
	// RouteAgentList lists the roster.
	RouteAgentList = "agent_list"
	// RouteAgentQuery reads one agent's state.
	RouteAgentQuery = "agent_query"
	// RouteTokenCreate mints a subprocess token.
	RouteTokenCreate = "token_create"
	// RouteConfigGet reads one config value.
	RouteConfigGet = "config_get"
	// RouteConfigList lists the accessible config keys.
	RouteConfigList = "config_list"
	// RouteConfigSearch searches the config keys.
	RouteConfigSearch = "config_search"
	// RouteConfigSet writes one config value.
	RouteConfigSet = "config_set"
	// RouteConfigAudit reads the config audit trail.
	RouteConfigAudit = "config_audit"
	// RouteHealth is the unauthenticated health probe.
	RouteHealth = "health"
)

// routes is the gateway's route table.
func routes() []Route {
	return []Route{
		{
			ID: RouteEventbusSubscribe, Method: "GET", Path: "/v1/eventbus/subscribe",
			Capability: "eventbus.subscribe", Args: []string{"pattern"},
		},
		{
			ID: RouteEventbusPublish, Method: "POST", Path: "/v1/eventbus/publish",
			Capability: "eventbus.publish",
			Body: json.RawMessage(`{"type":"object","additionalProperties":true,"properties":{` +
				`"event":{"type":"string"},"payload":{"type":"object"},"tags":{"type":"object"}}}`),
		},
		{
			ID: RouteMemoryWrite, Method: "POST", Path: "/v1/memory/write",
			Capability: "memory.write",
			Body: json.RawMessage(`{"type":"object","additionalProperties":true,"properties":{` +
				`"type":{"type":"string"},"subject":{"type":"string"},"content":{"type":"string"},` +
				`"tags":{"type":"object"}}}`),
		},
		{
			ID: RouteMemoryDelete, Method: "DELETE", Path: "/v1/memory/delete",
			Capability: "memory.delete", Args: []string{"id"},
		},
		{
			ID: RouteMemorySearch, Method: "GET", Path: "/v1/memory/search",
			Capability: "memory.search", Args: []string{"q", "limit"},
		},
		{
			ID: RouteLogRead, Method: "GET", Path: "/v1/log/read",
			Capability: "log.read", Args: []string{"id"},
		},
		{
			ID: RouteLogWrite, Method: "POST", Path: "/v1/log/write",
			Capability: "log.write",
			Body: json.RawMessage(`{"type":"object","additionalProperties":true,"properties":{` +
				`"level":{"type":"string"},"message":{"type":"string"},"meta":{"type":"object"}}}`),
		},
		{
			ID: RouteAgentList, Method: "GET", Path: "/v1/agent/list",
			Capability: "agent.list",
		},
		{
			ID: RouteAgentQuery, Method: "GET", Path: "/v1/agent/query",
			Capability: "agent.query", Args: []string{"id"},
		},
		{
			ID: RouteTokenCreate, Method: "POST", Path: "/v1/token/create",
			// The minted token is capped to the caller's own capabilities, so
			// the route needs no capability of its own.
			Body: json.RawMessage(`{"type":"object","additionalProperties":true,"properties":{` +
				`"sub_id":{"type":"string"},"caps":{"type":"array","items":{"type":"string"}},` +
				`"max_rpm":{"type":"number"},"max_burst":{"type":"number"},"expiry_ms":{"type":"number"}}}`),
		},
		{
			ID: RouteConfigGet, Method: "GET", Path: "/v1/config/",
			Capability: "config.access", Config: true,
			Args: []string{"reason", "cached_hash"},
		},
		{
			ID: RouteConfigList, Method: "GET", Path: "/v1/config",
			Capability: "config.list", Config: true,
		},
		{
			ID: RouteConfigSearch, Method: "GET", Path: "/v1/config/search",
			Capability: "config.search", Config: true, Args: []string{"q"},
		},
		{
			ID: RouteConfigSet, Method: "POST", Path: "/v1/config",
			Capability: "config.write", Config: true,
			Body: json.RawMessage(`{"type":"object","additionalProperties":true,"properties":{` +
				`"key":{"type":"string"},"value":{"type":"string"},"rating":{"type":"string"},` +
				`"description":{"type":"string"},"tags":{"type":"array","items":{"type":"string"}},` +
				`"allowed_agents":{"type":"array","items":{"type":"string"}},` +
				`"excluded_agents":{"type":"array","items":{"type":"string"}}}}`),
		},
		{
			ID: RouteConfigAudit, Method: "GET", Path: "/v1/config/audit",
			Capability: "config.access", Config: true,
			Args: []string{"agent_id", "key", "limit"},
		},
		{
			ID: RouteHealth, Method: "GET", Path: "/health",
			Public: true,
		},
	}
}

// Routes returns the gateway's route table.
func Routes() []Route { return routes() }

// RequestSchema returns the JSON Schema of a route's request body.
//
// It answers false for a route that takes no body — and for a route the
// contract does not know, so a caller can tell "this route has no body" from
// "this route is not in the contract".
func RequestSchema(routeID string) (json.RawMessage, bool) {
	for _, r := range routes() {
		if r.ID == routeID {
			if len(r.Body) == 0 {
				return nil, false
			}
			return r.Body, true
		}
	}
	return nil, false
}
