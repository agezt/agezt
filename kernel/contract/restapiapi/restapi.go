// SPDX-License-Identifier: MIT

// Package restapiapi is the single home of the daemon's native REST surface.
//
// The daemon serves two HTTP surfaces. The console's internal one (`/api/*`)
// is owned by kernel/contract/opapi and was given its own catalog in W5.1
// (the OpenAPI document and the generated browser client both read that
// table). This package owns the other one: `/api/v1`, the surface the Python,
// TypeScript and Rust SDKs speak, plus the unauthenticated probes.
//
// Until now the `/api/v1` routes lived as string literals in
// kernel/restapi/restapi_routes.go, every SDK hand-copied the same paths, and
// the only thing that could notice a drift was a regex that scraped both sides
// (tools/sdkparity). This package holds the surface as data:
//
//   - Routes() is what the daemon registers: path, tier, body policy.
//   - Endpoints() is what an SDK calls: verb, path template, query, and the
//     request/response wire rows.
//   - Rows() are the wire rows themselves (field name, type, optionality).
//
// tools/sdkgen generates the SDK catalogs from this table, and the daemon
// registers its routes FROM it, so a path exists in exactly one place.
//
// Two rules keep the table honest:
//
//  1. A row is the STABLE subset of a payload. The server may add fields to a
//     response (the mailbox view adds `acked_by`, for example); SDKs ignore
//     unknown fields, so a row only names the fields every SDK already speaks.
//     This is the same rule docs/EVENT-SCHEMA.md states for events.
//  2. A path template uses `{name}` for a path segment the caller supplies
//     (the correlation id of a run, the id of a message). The daemon registers
//     the prefix; the SDK substitutes the segment.
package restapiapi

import (
	"encoding/json"
	"fmt"
)

// Tier is the authorization tier a route is served at.
type Tier string

const (
	// TierPublic is served without a token (liveness, readiness).
	TierPublic Tier = "public"
	// TierUser is served to any valid user or tenant token.
	TierUser Tier = "user"
	// TierAdmin is served to the daemon's admin token only. The mailbox is
	// daemon-global with no tenant partition, so it lives here (V-011).
	TierAdmin Tier = "admin"
)

// Kind is the shape of a route's answer.
type Kind string

const (
	// KindJSON answers with a JSON body.
	KindJSON Kind = "json"
	// KindSSE answers with a text/event-stream.
	KindSSE Kind = "sse"
	// KindText answers with a plain-text body (Prometheus exposition).
	KindText Kind = "text"
	// KindBytes answers with raw bytes (artifact transfer).
	KindBytes Kind = "bytes"
)

// Type is the type of a wire field.
type Type string

const (
	// TypeString is a JSON string.
	TypeString Type = "string"
	// TypeInt is a JSON integer.
	TypeInt Type = "int"
	// TypeBool is a JSON boolean.
	TypeBool Type = "bool"
	// TypeStringSlice is a JSON array of strings.
	TypeStringSlice Type = "string[]"
	// TypeAnySlice is a JSON array of free-form values.
	TypeAnySlice Type = "any[]"
	// TypeAnyObject is a free-form JSON object.
	TypeAnyObject Type = "any"
	// TypeIntMap is a JSON object whose values are integers.
	TypeIntMap Type = "int_map"
	// TypeMail is the mail row (see Rows). A field of this type is a message
	// on the shared mailbox.
	TypeMail Type = "mail"
	// TypeMailSlice is a list of messages on the shared mailbox.
	TypeMailSlice Type = "mail[]"
)

// Field is one field of a wire row.
//
// Names overrides the local name a language uses for the field; the wire name
// is the default. It exists because a few wire names collide with a
// language's keywords or conventions (Python's `from` is a keyword; the
// TypeScript SDK spells the draft's `reply_to` as `replyTo`).
type Field struct {
	// Name is the wire name — the key in the JSON body.
	Name string
	// Type is the field's type.
	Type Type
	// Required marks a field every answer carries. Optional fields may be
	// absent (the server omits an empty mailbox `id`, for example).
	Required bool
	// Doc is one line of prose about the field.
	Doc string
	// Names maps a language id ("python", "typescript", "rust") to the local
	// name for this field. Empty means "the wire name".
	Names map[string]string
}

// LocalName returns the field's name in the given language.
func (f Field) LocalName(lang string) string {
	if n, ok := f.Names[lang]; ok && n != "" {
		return n
	}
	return f.Name
}

// Row is a named wire shape: a request body, a response body or a nested
// object inside one.
type Row struct {
	// Name identifies the row (referenced by endpoints and fields).
	Name string
	// Doc is one line of prose about the row.
	Doc string
	// Fields are the row's fields, in wire order.
	Fields []Field
	// SDK marks a row the SDK catalog carries — a shape the SDK contract
	// publishes.
	//
	// Most rows are published because an SDK endpoint exchanges them. A row
	// the SDK contract does NOT speak — the self-update body — stays in the
	// vocabulary and out of the SDKs' public API: the SDKs deliberately do not
	// expose the self-update surface, and a published row would be a request
	// body every SDK would be expected to build.
	//
	// The endpoint table says the same about an endpoint (Endpoint.SDK), so
	// the two decisions live together: what the SDKs speak is declared once,
	// and a row nobody publishes never reaches a catalog.
	SDK bool
}

// Param is a query parameter of an endpoint.
type Param struct {
	// Name is the query parameter's name.
	Name string
	// Type is the parameter's type.
	Type Type
	// Required marks a parameter the endpoint refuses to answer without.
	Required bool
	// Doc is one line of prose about the parameter.
	Doc string
}

// Method is one verb served on a route.
type Method struct {
	// Verb is the HTTP method ("GET", "POST").
	Verb string
	// Request is the row of the request body, empty when the verb takes none.
	Request string
	// Response is the row of the response body, empty when the verb answers
	// with no JSON body.
	Response string
	// Doc is one line of prose about the verb.
	Doc string
}

// Route is a registered route: one `router.Handle` call.
type Route struct {
	// ID identifies the route ("mailbox_messages").
	ID string
	// Path is the registered path. A trailing slash registers a subtree; the
	// segments below it are dispatched by the handler.
	Path string
	// Tier is the authorization tier.
	Tier Tier
	// Kind is the shape of the answer.
	Kind Kind
	// Methods are the verbs served on the route.
	Methods []Method
	// Query are the query parameters the route reads.
	Query []Param
	// Head marks a route that also answers HEAD (the probes).
	Head bool
	// Mutation marks a route that accepts a request body. Mutating routes get
	// the surface's body cap and are audited as mutations.
	Mutation bool
	// BodyMax overrides the surface's body cap, in bytes. Zero means the
	// surface default.
	BodyMax int64
	// Doc is one line of prose about the route.
	Doc string
}

// Endpoint is an operation an SDK can call: a verb on a path template.
type Endpoint struct {
	// ID identifies the endpoint ("mailbox_ack").
	ID string
	// Route is the ID of the route that serves it.
	Route string
	// Verb is the HTTP method.
	Verb string
	// Path is the path template ("/api/v1/mailbox/messages/{id}/ack").
	Path string
	// Request is the row of the request body, empty when the verb takes none.
	Request string
	// Response is the row of the response body, empty when the verb answers
	// with no JSON body.
	Response string
	// Query are the query parameters the endpoint reads.
	Query []Param
	// SDK marks an endpoint that is part of the SDK contract. An endpoint the
	// SDKs deliberately do not expose (artifact transfer, self-update) says so
	// here instead of going missing — the SDK coverage gate then knows which
	// silence is a decision and which one is a hole.
	SDK bool
	// Doc is one line of prose about the endpoint.
	Doc string
}

// Probe endpoints are served without a token and are not part of the SDK
// contract; they exist so a load balancer can watch the daemon.
const (
	// ProbeHealthz is the unauthenticated liveness probe.
	ProbeHealthz = "healthz"
	// ProbeReadyz is the unauthenticated readiness probe.
	ProbeReadyz = "readyz"
	// ProbeMetrics is the Prometheus exposition endpoint.
	ProbeMetrics = "metrics"
)

// Route IDs of the SDK-facing surface. The daemon's route table and the SDK
// catalogs both name them, so a renamed route is a compile error somewhere.
const (
	RouteHealth        = "health"
	RouteModels        = "models"
	RouteRuns          = "runs"
	RouteRunByID       = "run_by_id"
	RouteArtifacts     = "artifacts"
	RouteArtifactBytes = "artifact_bytes"
	RouteMailbox       = "mailbox_messages"
	RouteMailboxSub    = "mailbox_message"
	RouteMailboxInbox  = "mailbox_inbox"
	RouteMailboxWatch  = "mailbox_watch"
	RouteMailboxTopics = "mailbox_topics"
	RouteUpdate        = "update"
	RouteUpdateApply   = "update_apply"
)

// Endpoint IDs of the SDK-facing operations.
const (
	EndpointHealth        = "health"
	EndpointModels        = "models"
	EndpointRun           = "run"
	EndpointRunArc        = "run_arc"
	EndpointArtifacts     = "artifacts"
	EndpointArtifactBytes = "artifact_bytes"
	EndpointMailboxList   = "mailbox_messages"
	EndpointMailboxSend   = "mailbox_send"
	EndpointMailboxAck    = "mailbox_ack"
	EndpointMailboxReply  = "mailbox_replies"
	EndpointMailboxInbox  = "mailbox_inbox"
	EndpointMailboxWatch  = "mailbox_watch"
	EndpointMailboxTopics = "mailbox_topics"
	EndpointUpdateCheck   = "update_check"
	EndpointUpdateApply   = "update_apply"
)

// Row names of the wire shapes.
const (
	RowHealth         = "health"
	RowModels         = "models"
	RowRunRequest     = "run_request"
	RowRunResult      = "run_result"
	RowRunArc         = "run_arc"
	RowMail           = "mail"
	RowMailDraft      = "mail_draft"
	RowMailAck        = "mail_ack"
	RowMailAcked      = "mail_acked"
	RowMailboxList    = "mailbox_messages"
	RowMailboxSent    = "mailbox_sent"
	RowMailboxReplies = "mailbox_replies"
	RowMailboxInbox   = "mailbox_inbox"
	RowMailboxTopics  = "mailbox_topics"
	RowArtifacts      = "artifacts"
	RowAPIError       = "api_error"
	RowProbeStatus    = "probe_status"
	RowUpdateApply    = "update_apply"
)

// updateApplyBodyMax is the body cap of the self-update endpoint. It is
// smaller than the surface default: an update carries a manifest and a
// signature, not a payload.
const updateApplyBodyMax int64 = 64 * 1024

// routes is the daemon's REST route table.
func routes() []Route {
	return []Route{
		{
			ID:   ProbeHealthz,
			Path: "/healthz",
			Tier: TierPublic,
			Kind: KindJSON,
			Methods: []Method{
				{Verb: "GET", Response: RowProbeStatus, Doc: "Liveness: the process is alive and serving."},
			},
			Head: true,
			Doc:  "Unauthenticated liveness probe.",
		},
		{
			ID:   ProbeReadyz,
			Path: "/readyz",
			Tier: TierPublic,
			Kind: KindJSON,
			Methods: []Method{
				{Verb: "GET", Response: RowProbeStatus, Doc: "Readiness: the daemon can serve work."},
			},
			Head: true,
			Doc:  "Unauthenticated readiness probe.",
		},
		{
			ID:   ProbeMetrics,
			Path: "/metrics",
			Tier: TierAdmin,
			Kind: KindText,
			Methods: []Method{
				{Verb: "GET", Doc: "Prometheus exposition of the daemon's counters."},
			},
			Head: true,
			Doc:  "Daemon-wide metrics; admin token only (they carry spend).",
		},
		{
			ID:   RouteHealth,
			Path: "/api/v1/health",
			Tier: TierUser,
			Kind: KindJSON,
			Methods: []Method{
				{Verb: "GET", Response: RowHealth, Doc: "The daemon's health summary."},
			},
			Doc: "Daemon health.",
		},
		{
			ID:   RouteModels,
			Path: "/api/v1/models",
			Tier: TierUser,
			Kind: KindJSON,
			Methods: []Method{
				{Verb: "GET", Response: RowModels, Doc: "The tenant's model catalog."},
			},
			Doc: "Model catalog.",
		},
		{
			ID:   RouteRuns,
			Path: "/api/v1/runs",
			Tier: TierUser,
			Kind: KindJSON,
			Methods: []Method{
				{Verb: "POST", Request: RowRunRequest, Response: RowRunResult,
					Doc: "Run an intent; stream it with `stream: true` or an SSE Accept header."},
			},
			Mutation: true,
			Doc:      "Run creation.",
		},
		{
			ID:   RouteRunByID,
			Path: "/api/v1/runs/",
			Tier: TierUser,
			Kind: KindJSON,
			Methods: []Method{
				{Verb: "GET", Response: RowRunArc, Doc: "The journaled event arc of one run."},
			},
			Doc: "Run event arc by correlation id (subtree route).",
		},
		{
			ID:   RouteArtifacts,
			Path: "/api/v1/artifacts",
			Tier: TierUser,
			Kind: KindJSON,
			Methods: []Method{
				{Verb: "GET", Response: RowArtifacts, Doc: "Artifact metadata."},
			},
			Query: []Param{
				{Name: "kind", Type: TypeString, Doc: "Filter by artifact kind."},
				{Name: "source", Type: TypeString, Doc: "Filter by artifact source."},
				{Name: "corr", Type: TypeString, Doc: "Filter by correlation id."},
				{Name: "limit", Type: TypeInt, Doc: "Maximum number of entries."},
			},
			Doc: "Artifact listing.",
		},
		{
			ID:   RouteArtifactBytes,
			Path: "/api/v1/artifacts/",
			Tier: TierUser,
			Kind: KindBytes,
			Methods: []Method{
				{Verb: "GET", Doc: "The bytes of one artifact."},
			},
			Doc: "Artifact byte transfer (subtree route).",
		},
		{
			ID:   RouteMailbox,
			Path: "/api/v1/mailbox/messages",
			Tier: TierAdmin,
			Kind: KindJSON,
			Methods: []Method{
				{Verb: "GET", Response: RowMailboxList, Doc: "Recent messages, newest first."},
				{Verb: "POST", Request: RowMailDraft, Response: RowMailboxSent, Doc: "Leave a message."},
			},
			Query: []Param{
				{Name: "topic", Type: TypeString, Doc: "Filter by topic."},
				{Name: "limit", Type: TypeInt, Doc: "Maximum number of messages."},
			},
			Mutation: true,
			Doc:      "The shared inter-agent mailbox (admin tier: the board is daemon-global).",
		},
		{
			ID:   RouteMailboxSub,
			Path: "/api/v1/mailbox/messages/",
			Tier: TierAdmin,
			Kind: KindJSON,
			Methods: []Method{
				{Verb: "GET", Response: RowMailboxReplies, Doc: "The answers to one message."},
				{Verb: "POST", Request: RowMailAck, Response: RowMailAcked, Doc: "Mark one message read."},
			},
			Query: []Param{
				{Name: "limit", Type: TypeInt, Doc: "Maximum number of replies."},
			},
			Mutation: true,
			Doc:      "One message's replies and ack (subtree route).",
		},
		{
			ID:   RouteMailboxInbox,
			Path: "/api/v1/mailbox/inbox",
			Tier: TierAdmin,
			Kind: KindJSON,
			Methods: []Method{
				{Verb: "GET", Response: RowMailboxInbox, Doc: "What waits for one inbox."},
			},
			Query: []Param{
				{Name: "name", Type: TypeString, Required: true, Doc: "Whose inbox to read."},
				{Name: "all", Type: TypeBool, Doc: "Include answered and acked messages."},
				{Name: "limit", Type: TypeInt, Doc: "Maximum number of messages."},
			},
			Doc: "One inbox (admin tier).",
		},
		{
			ID:   RouteMailboxWatch,
			Path: "/api/v1/mailbox/watch",
			Tier: TierAdmin,
			Kind: KindSSE,
			Methods: []Method{
				{Verb: "GET", Doc: "Stream new mail as it lands."},
			},
			Query: []Param{
				{Name: "name", Type: TypeString, Doc: "Watch one inbox."},
				{Name: "topic", Type: TypeString, Doc: "Watch one topic."},
			},
			Doc: "The push counterpart of the inbox (admin tier).",
		},
		{
			ID:   RouteMailboxTopics,
			Path: "/api/v1/mailbox/topics",
			Tier: TierAdmin,
			Kind: KindJSON,
			Methods: []Method{
				{Verb: "GET", Response: RowMailboxTopics, Doc: "The mailbox's topics with their counts."},
			},
			Doc: "Mailbox topics (admin tier).",
		},
		{
			ID:   RouteUpdate,
			Path: "/api/v1/update",
			Tier: TierAdmin,
			Kind: KindJSON,
			Methods: []Method{
				{Verb: "GET", Doc: "Check whether a newer release is available."},
			},
			Doc: "Self-update check (admin tier; SDKs do not expose it).",
		},
		{
			ID:   RouteUpdateApply,
			Path: "/api/v1/update/apply",
			Tier: TierAdmin,
			Kind: KindJSON,
			Methods: []Method{
				{Verb: "POST", Request: RowUpdateApply, Doc: "Apply a downloaded release."},
			},
			Mutation: true,
			BodyMax:  updateApplyBodyMax,
			Doc:      "Self-update apply (admin tier; SDKs do not expose it).",
		},
	}
}

// endpoints is the SDK-facing operation table.
func endpoints() []Endpoint {
	return []Endpoint{
		{
			ID:  EndpointHealth,
			SDK: true, Route: RouteHealth, Verb: "GET", Path: "/api/v1/health",
			Response: RowHealth,
			Doc:      "The daemon's health summary.",
		},
		{
			ID:  EndpointModels,
			SDK: true, Route: RouteModels, Verb: "GET", Path: "/api/v1/models",
			Response: RowModels,
			Doc:      "The default model and the tenant's model catalog.",
		},
		{
			ID:  EndpointRun,
			SDK: true, Route: RouteRuns, Verb: "POST", Path: "/api/v1/runs",
			Request:  RowRunRequest,
			Response: RowRunResult,
			Doc:      "Run an intent and return the final answer.",
		},
		{
			ID:  EndpointRunArc,
			SDK: true, Route: RouteRunByID, Verb: "GET", Path: "/api/v1/runs/{id}",
			Response: RowRunArc,
			Doc:      "The journaled event arc of one run.",
		},
		{
			ID: EndpointArtifacts, Route: RouteArtifacts, Verb: "GET", Path: "/api/v1/artifacts",
			Response: RowArtifacts,
			Query: []Param{
				{Name: "kind", Type: TypeString, Doc: "Filter by artifact kind."},
				{Name: "source", Type: TypeString, Doc: "Filter by artifact source."},
				{Name: "corr", Type: TypeString, Doc: "Filter by correlation id."},
				{Name: "limit", Type: TypeInt, Doc: "Maximum number of entries."},
			},
			Doc: "Artifact metadata.",
		},
		{
			ID: EndpointArtifactBytes, Route: RouteArtifactBytes, Verb: "GET",
			Path: "/api/v1/artifacts/{id}/bytes",
			Doc:  "The bytes of one artifact.",
		},
		{
			ID:  EndpointMailboxList,
			SDK: true, Route: RouteMailbox, Verb: "GET", Path: "/api/v1/mailbox/messages",
			Response: RowMailboxList,
			Query: []Param{
				{Name: "topic", Type: TypeString, Doc: "Filter by topic."},
				{Name: "limit", Type: TypeInt, Doc: "Maximum number of messages."},
			},
			Doc: "Recent messages, newest first.",
		},
		{
			ID:  EndpointMailboxSend,
			SDK: true, Route: RouteMailbox, Verb: "POST", Path: "/api/v1/mailbox/messages",
			Request:  RowMailDraft,
			Response: RowMailboxSent,
			Doc:      "Leave a message on the shared mailbox.",
		},
		{
			ID:  EndpointMailboxAck,
			SDK: true, Route: RouteMailboxSub, Verb: "POST",
			Path:     "/api/v1/mailbox/messages/{id}/ack",
			Request:  RowMailAck,
			Response: RowMailAcked,
			Doc:      "Mark one message read for one reader.",
		},
		{
			ID:  EndpointMailboxReply,
			SDK: true, Route: RouteMailboxSub, Verb: "GET",
			Path:     "/api/v1/mailbox/messages/{id}/replies",
			Response: RowMailboxReplies,
			Query: []Param{
				{Name: "limit", Type: TypeInt, Doc: "Maximum number of replies."},
			},
			Doc: "The answers to one message.",
		},
		{
			ID:  EndpointMailboxInbox,
			SDK: true, Route: RouteMailboxInbox, Verb: "GET",
			Path:     "/api/v1/mailbox/inbox",
			Response: RowMailboxInbox,
			Query: []Param{
				{Name: "name", Type: TypeString, Required: true, Doc: "Whose inbox to read."},
				{Name: "all", Type: TypeBool, Doc: "Include answered and acked messages."},
				{Name: "limit", Type: TypeInt, Doc: "Maximum number of messages."},
			},
			Doc: "What waits for one inbox.",
		},
		{
			ID:  EndpointMailboxWatch,
			SDK: true, Route: RouteMailboxWatch, Verb: "GET",
			Path: "/api/v1/mailbox/watch",
			Query: []Param{
				{Name: "name", Type: TypeString, Doc: "Watch one inbox."},
				{Name: "topic", Type: TypeString, Doc: "Watch one topic."},
			},
			Doc: "Stream new mail as it lands.",
		},
		{
			ID:  EndpointMailboxTopics,
			SDK: true, Route: RouteMailboxTopics, Verb: "GET",
			Path:     "/api/v1/mailbox/topics",
			Response: RowMailboxTopics,
			Doc:      "The mailbox's topics with their counts.",
		},
		{
			ID: EndpointUpdateCheck, Route: RouteUpdate, Verb: "GET", Path: "/api/v1/update",
			Doc: "Check whether a newer release is available (admin tier).",
		},
		{
			ID: EndpointUpdateApply, Route: RouteUpdateApply, Verb: "POST",
			Path:    "/api/v1/update/apply",
			Request: RowUpdateApply,
			Doc:     "Apply a downloaded release (admin tier).",
		},
	}
}

// rows is the wire vocabulary.
func rows() []Row {
	return []Row{
		{
			Name: RowHealth,
			SDK:  true,
			Doc:  "The daemon's health summary.",
			Fields: []Field{
				{Name: "status", Type: TypeString, Required: true, Doc: "\"ok\" when the daemon answers."},
				{Name: "version", Type: TypeString, Required: true, Doc: "The daemon's version."},
				{Name: "default_model", Type: TypeString, Required: true, Doc: "The tenant's default model."},
				{Name: "model_count", Type: TypeInt, Required: true, Doc: "How many models the tenant may use."},
			},
		},
		{
			Name: RowModels,
			SDK:  true,
			Doc:  "The tenant's model catalog.",
			Fields: []Field{
				{Name: "default", Type: TypeString, Required: true, Doc: "The default model id."},
				{Name: "models", Type: TypeStringSlice, Required: true, Doc: "Every model id the tenant may use."},
			},
		},
		{
			Name: RowRunRequest,
			SDK:  true,
			Doc:  "The body of a run request.",
			Fields: []Field{
				{Name: "intent", Type: TypeString, Required: true, Doc: "What the agent should do."},
				{Name: "model", Type: TypeString, Doc: "The model to run with; the tenant's default otherwise."},
				{Name: "stream", Type: TypeBool, Doc: "Answer with Server-Sent Events instead of one JSON body."},
			},
		},
		{
			Name: RowRunResult,
			SDK:  true,
			Doc:  "A completed (non-streaming) run.",
			Fields: []Field{
				{Name: "correlation_id", Type: TypeString, Required: true, Doc: "The run's correlation id."},
				{Name: "model", Type: TypeString, Required: true, Doc: "The model that answered."},
				{Name: "status", Type: TypeString, Required: true, Doc: "\"completed\" or \"failed\"."},
				{Name: "answer", Type: TypeString, Required: true, Doc: "The agent's final answer."},
			},
		},
		{
			Name: RowRunArc,
			SDK:  true,
			Doc:  "The journaled event arc of one run.",
			Fields: []Field{
				{Name: "correlation_id", Type: TypeString, Required: true, Doc: "The run's correlation id."},
				{Name: "count", Type: TypeInt, Required: true, Doc: "How many events the arc holds."},
				{Name: "events", Type: TypeAnySlice, Required: true, Doc: "The raw journal events."},
			},
		},
		{
			Name: RowMail,
			SDK:  true,
			Doc:  "One message on the shared mailbox (the inter-agent board).",
			Fields: []Field{
				{Name: "id", Type: TypeString, Doc: "The message's id; pass it to ack or replies."},
				{Name: "topic", Type: TypeString, Required: true, Doc: "The topic (\"dm\" for a direct message)."},
				{Name: "text", Type: TypeString, Required: true, Doc: "The message body."},
				{Name: "from", Type: TypeString, Names: map[string]string{"python": "from_"}, Doc: "The sender's name."},
				{Name: "to", Type: TypeString, Doc: "The recipient's name; \"*\" means every inbox."},
				{Name: "reply_to", Type: TypeString, Doc: "The message this one answers."},
				{Name: "help", Type: TypeBool, Doc: "The message asks for assistance."},
				{Name: "ts_unix_ms", Type: TypeInt, Doc: "When the message was sent, in milliseconds."},
			},
		},
		{
			Name: RowMailDraft,
			SDK:  true,
			Doc:  "A message to leave on the shared mailbox.",
			Fields: []Field{
				{Name: "text", Type: TypeString, Required: true, Doc: "The message body; required."},
				{Name: "from", Type: TypeString, Names: map[string]string{"python": "from_"}, Doc: "The sender's name."},
				{Name: "to", Type: TypeString, Doc: "The recipient's name; \"*\" broadcasts."},
				{Name: "topic", Type: TypeString, Doc: "The topic; a directed message defaults to \"dm\"."},
				{Name: "reply_to", Type: TypeString, Names: map[string]string{"typescript": "replyTo"},
					Doc: "The message this one answers."},
				{Name: "help", Type: TypeBool, Doc: "Raise an assistance request instead of a message."},
				// The daemon reads this one: it is the correlation id the message's
				// board event carries (the header X-Agezt-Correlation is the
				// fallback). It was read before it was named here — a field the
				// daemon speaks and the contract could not see.
				{Name: "correlation_id", Type: TypeString, Doc: "The correlation id the message's board event carries."},
			},
		},
		{
			Name: RowMailAck,
			SDK:  true,
			Doc:  "The body of an ack.",
			Fields: []Field{
				{Name: "by", Type: TypeString, Required: true, Doc: "Whose inbox to clear."},
			},
		},
		{
			Name: RowMailAcked,
			SDK:  true,
			Doc:  "The answer to an ack.",
			Fields: []Field{
				{Name: "acked", Type: TypeBool, Required: true, Doc: "Always true when the ack succeeded."},
				{Name: "id", Type: TypeString, Required: true, Doc: "The acked message's id."},
				{Name: "by", Type: TypeString, Required: true, Doc: "The reader the message left for."},
			},
		},
		{
			Name: RowMailboxList,
			SDK:  true,
			Doc:  "Recent messages on the shared mailbox.",
			Fields: []Field{
				{Name: "messages", Type: TypeMailSlice, Required: true, Doc: "The messages, newest first."},
				{Name: "count", Type: TypeInt, Required: true, Doc: "How many messages the answer holds."},
			},
		},
		{
			Name: RowMailboxSent,
			SDK:  true,
			Doc:  "The answer to leaving a message.",
			Fields: []Field{
				{Name: "message", Type: TypeMail, Required: true, Doc: "The message that was left."},
			},
		},
		{
			Name: RowMailboxReplies,
			SDK:  true,
			Doc:  "The answers to one message.",
			Fields: []Field{
				{Name: "id", Type: TypeString, Required: true, Doc: "The answered message's id."},
				{Name: "replies", Type: TypeMailSlice, Required: true, Doc: "The answers, oldest first."},
				{Name: "count", Type: TypeInt, Required: true, Doc: "How many answers the answer holds."},
			},
		},
		{
			Name: RowMailboxInbox,
			SDK:  true,
			Doc:  "What waits for one inbox.",
			Fields: []Field{
				{Name: "name", Type: TypeString, Required: true, Doc: "Whose inbox this is."},
				{Name: "waiting", Type: TypeMailSlice, Required: true, Doc: "The messages that wait, newest first."},
				{Name: "count", Type: TypeInt, Required: true, Doc: "How many messages wait."},
			},
		},
		{
			Name: RowMailboxTopics,
			SDK:  true,
			Doc:  "The mailbox's topics with their message counts.",
			Fields: []Field{
				{Name: "topics", Type: TypeIntMap, Required: true, Doc: "Message count per topic."},
			},
		},
		{
			Name: RowArtifacts,
			SDK:  true,
			Doc:  "Artifact metadata.",
			Fields: []Field{
				{Name: "count", Type: TypeInt, Required: true, Doc: "How many entries the answer holds."},
				{Name: "total_count", Type: TypeInt, Required: true, Doc: "How many entries matched."},
				{Name: "limit", Type: TypeInt, Required: true, Doc: "The limit that was applied."},
				{Name: "truncated", Type: TypeBool, Required: true, Doc: "Whether entries were dropped."},
				{Name: "entries", Type: TypeAnySlice, Required: true, Doc: "The entries themselves."},
			},
		},
		{
			Name: RowAPIError,
			SDK:  true,
			Doc:  "The error body of every non-2xx answer.",
			Fields: []Field{
				// `type` is a Rust keyword, and the Rust SDK has always called
				// this field `kind` (Error::Api's field), so the local name is
				// declared here rather than spelled `r#type` in the catalog.
				{Name: "type", Type: TypeString, Required: true, Names: map[string]string{"rust": "kind"},
					Doc: "The machine-readable error kind."},
				{Name: "message", Type: TypeString, Required: true, Doc: "The human-readable error text."},
			},
		},
		{
			Name: RowProbeStatus,
			SDK:  true,
			Doc:  "The answer of a liveness or readiness probe.",
			Fields: []Field{
				{Name: "status", Type: TypeString, Required: true, Doc: "\"ok\" or \"ready\"."},
				{Name: "reason", Type: TypeString, Doc: "Why the daemon is not ready."},
			},
		},
		{
			Name: RowUpdateApply,
			Doc:  "The body of a self-update request.",
			Fields: []Field{
				{Name: "version", Type: TypeString, Required: true, Doc: "The release version to install."},
				{Name: "sha256", Type: TypeString, Required: true, Doc: "The SHA256 of the release archive."},
				{Name: "url", Type: TypeString, Required: true, Doc: "Where the release archive is fetched from."},
				{Name: "notes", Type: TypeString, Doc: "The release notes."},
			},
			// The SDKs do not expose the self-update surface (Endpoint.SDK
			// says so about the endpoint), so this row is NOT published to the
			// SDK catalogs: a published request row is a body every SDK would
			// be expected to build.
			SDK: false,
		},
	}
}

// Routes returns the daemon's REST route table.
func Routes() []Route { return routes() }

// Endpoints returns the SDK-facing operation table.
func Endpoints() []Endpoint { return endpoints() }

// Rows returns the wire vocabulary.
func Rows() []Row { return rows() }

// RequestRows returns the rows an endpoint takes as a request body, in the
// order the contract declares them. A request row is a shape the SDK SENDS;
// every other row is a shape the daemon answers with.
//
// This is the WHOLE surface's set: the self-update body is in it, because the
// daemon's door measures that body against it too. SDKRequestRows() is the
// SDK contract's share.
func RequestRows() []Row {
	names := map[string]bool{}
	for _, e := range endpoints() {
		if e.Request != "" {
			names[e.Request] = true
		}
	}
	var out []Row
	for _, r := range rows() {
		if names[r.Name] {
			out = append(out, r)
		}
	}
	return out
}

// SDKRows returns the rows the SDK catalog carries: the rows the SDK contract
// publishes, in contract order.
//
// The catalog is the SDK contract's own view of the vocabulary. A row the SDK
// contract does not speak — the self-update body — stays in the contract and
// out of the SDKs' public API: the SDKs deliberately do not expose the
// self-update surface, and a published request row is a body every SDK would
// be expected to build.
//
// A row an SDK endpoint exchanges is published (the guard lives in this
// package's test), so the catalog can never miss a shape the SDKs speak.
func SDKRows() []Row {
	var out []Row
	for _, r := range Rows() {
		if r.SDK {
			out = append(out, r)
		}
	}
	return out
}

// SDKRequestRows returns the request rows of the SDK contract: the rows an
// SDK endpoint takes as a body.
//
// RequestRows() holds every request row of the surface; this one holds the
// SDKs' share of them. The self-update body is in the first and not in this
// one — the SDKs do not expose that surface, so they neither build nor check
// that body.
func SDKRequestRows() []Row {
	names := map[string]bool{}
	for _, e := range endpoints() {
		if e.SDK && e.Request != "" {
			names[e.Request] = true
		}
	}
	var out []Row
	for _, r := range RequestRows() {
		if names[r.Name] {
			out = append(out, r)
		}
	}
	return out
}

// RequestSchema renders an endpoint's request row as a JSON Schema.
//
// This is the shape the daemon's request door measures a body against: the
// CONTRACT declares the row, so a mistyped body is refused with the same words
// the operation registry uses (`$.<field> has wrong type: got <got>, want
// <want>`) and the contract — not the daemon's private struct — is what the
// body is checked against.
//
// Two rules keep it the registry's shape:
//
//  1. No `required` list. The registry's door checks TYPES; presence is the
//     handler's business (an absent intent is "intent is required", not a
//     schema error).
//  2. Unknown fields are allowed (`additionalProperties: true`). A row is the
//     stable SUBSET of a body — the daemon may read more than the contract
//     names, and a caller may send more than the contract declares.
//
// The type mapping speaks the registry's words: an integer is a `number` (the
// registry says "want number"), a boolean is a `boolean`.
func RequestSchema(endpointID string) ([]byte, bool) {
	row, ok := requestRow(endpointID)
	if !ok {
		return nil, false
	}
	return rowSchema(row)
}

// requestRow returns the request row an endpoint takes, by endpoint id.
func requestRow(endpointID string) (Row, bool) {
	for _, e := range endpoints() {
		if e.ID == endpointID && e.Request != "" {
			return lookupRow(e.Request)
		}
	}
	return Row{}, false
}

// lookupRow returns the row with the given name.
func lookupRow(name string) (Row, bool) {
	for _, r := range rows() {
		if r.Name == name {
			return r, true
		}
	}
	return Row{}, false
}

// schemaDocument is the JSON Schema of a request row.
type schemaDocument struct {
	Type                 string                    `json:"type"`
	AdditionalProperties bool                      `json:"additionalProperties"`
	Properties           map[string]propertySchema `json:"properties"`
}

// propertySchema is one property of a request row's schema.
type propertySchema struct {
	Type  string          `json:"type"`
	Items *propertySchema `json:"items,omitempty"`
}

// rowSchema renders a row as a JSON Schema object.
func rowSchema(row Row) ([]byte, bool) {
	properties := map[string]propertySchema{}
	for _, f := range row.Fields {
		properties[f.Name] = propertySchema{Type: schemaType(f.Type), Items: schemaItems(f.Type)}
	}
	out, err := json.Marshal(schemaDocument{
		Type:                 "object",
		AdditionalProperties: true,
		Properties:           properties,
	})
	if err != nil {
		return nil, false
	}
	return out, true
}

// schemaType maps a wire type onto the JSON Schema type the registry's
// validator speaks.
func schemaType(t Type) string {
	switch t {
	case TypeString:
		return "string"
	case TypeInt:
		return "number"
	case TypeBool:
		return "boolean"
	case TypeStringSlice, TypeAnySlice, TypeMailSlice:
		return "array"
	case TypeAnyObject, TypeIntMap, TypeMail:
		return "object"
	default:
		return ""
	}
}

// schemaItems is the item schema of an array property, nil for every other
// type.
func schemaItems(t Type) *propertySchema {
	if t == TypeStringSlice {
		return &propertySchema{Type: "string"}
	}
	return nil
}

// RowBody renders a row's wire body from the values it carries.
//
// This is the ONE place the contract's request vocabulary becomes bytes: the
// SDK catalogs render the same rows (tools/sdkgen), the golden fixture is
// recorded from here, and the SDKs' own tests read that fixture. So the
// daemon's contract, the Python catalog, the TypeScript catalog and the Rust
// catalog all say the same thing about a request body.
//
// The rule is the SDKs' rule: a required field is always written; an optional
// one only when it holds something (an empty string, a zero, a false, an empty
// list) — an absent optional field and an empty one are not the same request.
//
// A value that does not match the row's declared type is an error, and so is a
// value for a field the row does not declare: the contract is the whole shape.
func RowBody(rowName string, values map[string]any) ([]byte, error) {
	var row Row
	found := false
	for _, r := range rows() {
		if r.Name == rowName {
			row, found = r, true
			break
		}
	}
	if !found {
		return nil, fmt.Errorf("restapiapi: no %q row", rowName)
	}
	known := map[string]Field{}
	for _, f := range row.Fields {
		known[f.Name] = f
	}
	for name := range values {
		if _, ok := known[name]; !ok {
			return nil, fmt.Errorf("restapiapi: the %q row has no %q field", rowName, name)
		}
	}
	body := map[string]any{}
	for _, f := range row.Fields {
		value, present := values[f.Name]
		if !present {
			if f.Required {
				return nil, fmt.Errorf("restapiapi: the %q row requires %q", rowName, f.Name)
			}
			continue
		}
		normalized, err := rowValue(f, value)
		if err != nil {
			return nil, fmt.Errorf("restapiapi: the %q row's %q: %w", rowName, f.Name, err)
		}
		if !f.Required && rowValueEmpty(normalized) {
			continue
		}
		body[f.Name] = normalized
	}
	out, err := json.Marshal(body)
	if err != nil {
		return nil, fmt.Errorf("restapiapi: render the %q row: %w", rowName, err)
	}
	return out, nil
}

// rowValue checks a value against a field's declared type and normalizes it.
func rowValue(f Field, value any) (any, error) {
	switch f.Type {
	case TypeString:
		s, ok := value.(string)
		if !ok {
			return nil, fmt.Errorf("want a string, got %T", value)
		}
		return s, nil
	case TypeInt:
		switch n := value.(type) {
		case int:
			return int64(n), nil
		case int64:
			return n, nil
		default:
			return nil, fmt.Errorf("want an integer, got %T", value)
		}
	case TypeBool:
		b, ok := value.(bool)
		if !ok {
			return nil, fmt.Errorf("want a boolean, got %T", value)
		}
		return b, nil
	case TypeStringSlice:
		s, ok := value.([]string)
		if !ok {
			return nil, fmt.Errorf("want a list of strings, got %T", value)
		}
		return s, nil
	case TypeAnySlice:
		s, ok := value.([]any)
		if !ok {
			return nil, fmt.Errorf("want a list, got %T", value)
		}
		return s, nil
	case TypeAnyObject:
		m, ok := value.(map[string]any)
		if !ok {
			return nil, fmt.Errorf("want an object, got %T", value)
		}
		return m, nil
	case TypeIntMap:
		m, ok := value.(map[string]int)
		if !ok {
			return nil, fmt.Errorf("want an object of integers, got %T", value)
		}
		return m, nil
	default:
		return nil, fmt.Errorf("has no wire body yet")
	}
}

// rowValueEmpty reports whether a value holds nothing: an empty string, a
// zero, a false, an empty list or an empty object.
func rowValueEmpty(value any) bool {
	switch v := value.(type) {
	case string:
		return v == ""
	case int64:
		return v == 0
	case bool:
		return !v
	case []string:
		return len(v) == 0
	case []any:
		return len(v) == 0
	case map[string]any:
		return len(v) == 0
	case map[string]int:
		return len(v) == 0
	default:
		return false
	}
}
