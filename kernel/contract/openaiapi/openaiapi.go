// SPDX-License-Identifier: MIT

// Package openaiapi is the single home of the OpenAI-compatible surface.
//
// The surface speaks OpenAI's wire protocol so any OpenAI client, SDK or IDE
// can drive Agezt as if it were OpenAI. Its **routes** were registered by hand
// in kernel/openaiapi/openaiapi_server.go and its **request bodies** were read
// into whatever the handler happened to use, so what happened to a mistyped
// body was decided by Go's encoding/json — and the answer named a Go struct:
//
//	{"model":42,…} → 400 "invalid JSON body: json: cannot unmarshal number into Go struct field chatRequest.model of type string"
//	[]             → 400 "invalid JSON body: json: cannot unmarshal array into Go value of type openaiapi.chatRequest"
//
// and a **null** was not refused at all: it became the zero value, and the
// request went on — `{"model":null,…}` was a run against the default model,
// `{"stream_options":null,…}` was a run without usage.
//
// This package holds the surface as data:
//
//   - Routes() is what the surface registers: path, method and the request
//     body each route takes.
//   - RequestSchema() is the JSON Schema of a route's body — the shape the
//     surface's door measures an arriving body against.
//
// Three rules keep the schemas the operation registry's shape:
//
//  1. A schema names TYPES, not presence. The door checks types; a missing
//     field stays the handler's business (an absent model is still the
//     default model).
//  2. A schema allows an undeclared field. A body is the stable SUBSET of
//     what the daemon reads — and this surface is OpenAI's own wire protocol,
//     so a field Agezt does not read may still be one a client sends.
//  3. The type words are the registry's: an integer is a "number", a boolean
//     is a "boolean", and a field the daemon reads in two meaningful shapes
//     names both ("string|array") — the OpenAI message content, which is
//     either the text itself or an array of typed parts.
//
// The OpenAI names are the vendor's and are not changed here: this surface
// answers in OpenAI's shapes because that is the contract it promises.
package openaiapi

import "encoding/json"

// Route is one route of the OpenAI-compatible surface.
type Route struct {
	// ID identifies the route ("chat_completions"). It is the operation name
	// the door's refusal carries: "operation chat_completions input: …".
	ID string
	// Method is the HTTP method the route answers.
	Method string
	// Path is the route's path ("/v1/chat/completions").
	Path string
	// Body is the JSON Schema of the request body the route takes, empty
	// when it takes none.
	Body json.RawMessage
	// Multipart marks a route whose body is a multipart upload rather than
	// JSON — the audio transcription. Such a body has no JSON Schema: the
	// door measures JSON bodies only.
	Multipart bool
}

// Route IDs of the surface's operations.
const (
	// RouteChatCompletions is OpenAI's Chat Completions endpoint.
	RouteChatCompletions = "chat_completions"
	// RouteResponses is OpenAI's Responses endpoint.
	RouteResponses = "responses"
	// RouteAudioTranscriptions is OpenAI's speech-to-text upload.
	RouteAudioTranscriptions = "audio_transcriptions"
	// RouteModelsList lists the models the engine can route.
	RouteModelsList = "models_list"
	// RouteModelsRetrieve is OpenAI's "retrieve model" — one model by id.
	//
	// Its path keeps the trailing slash on purpose: the list route above is an
	// EXACT match, so without this subtree handler a single-model GET (what
	// the official SDKs' models.retrieve(id) issues for capability probing)
	// would 404.
	RouteModelsRetrieve = "models_retrieve"
)

// chatBodySchema is the shape of a Chat Completions request.
//
// The names are OpenAI's and the daemon reads them one for one: the model to
// run, the messages, whether to stream, the streaming options and the
// response format.
//
// Two fields name two shapes, because the daemon reads them in two shapes:
// a message's content is either the text itself or an array of typed parts
// (an image part among them), and the Responses `input` is either the text
// itself or an array of message items.
const (
	chatBodySchema = `{"type":"object","additionalProperties":true,"properties":{` +
		`"model":{"type":"string"},` +
		`"messages":{"type":"array","items":{"type":"object","additionalProperties":true,"properties":{` +
		`"role":{"type":"string"},` +
		`"content":{"type":["string","array"]}}}},` +
		`"stream":{"type":"boolean"},` +
		`"stream_options":{"type":"object","additionalProperties":true,"properties":{` +
		`"include_usage":{"type":"boolean"}}},` +
		`"response_format":{"type":"object","additionalProperties":true,"properties":{` +
		`"type":{"type":"string"}}}}}`

	responsesBodySchema = `{"type":"object","additionalProperties":true,"properties":{` +
		`"model":{"type":"string"},` +
		`"input":{"type":["string","array"]},` +
		`"instructions":{"type":"string"},` +
		`"stream":{"type":"boolean"},` +
		`"text":{"type":"object","additionalProperties":true,"properties":{` +
		`"format":{"type":"object","additionalProperties":true,"properties":{` +
		`"type":{"type":"string"}}}}},` +
		`"response_format":{"type":"object","additionalProperties":true,"properties":{` +
		`"type":{"type":"string"}}}}}`
)

// routes is the surface's route table.
func routes() []Route {
	return []Route{
		{
			ID: RouteChatCompletions, Method: "POST", Path: "/v1/chat/completions",
			Body: json.RawMessage(chatBodySchema),
		},
		{
			ID: RouteResponses, Method: "POST", Path: "/v1/responses",
			Body: json.RawMessage(responsesBodySchema),
		},
		{
			ID: RouteAudioTranscriptions, Method: "POST", Path: "/v1/audio/transcriptions",
			Multipart: true,
		},
		{
			ID: RouteModelsList, Method: "GET", Path: "/v1/models",
		},
		{
			ID: RouteModelsRetrieve, Method: "GET", Path: "/v1/models/",
		},
	}
}

// Routes returns the surface's route table.
func Routes() []Route { return routes() }

// RequestSchema returns the JSON Schema of a route's request body.
//
// It answers false for a route that takes no JSON body — and for a route the
// contract does not know, so a caller can tell "this route has no body" from
// "this route is not in the contract".
//
// A route that takes an upload (a multipart body) has no JSON schema either:
// the door measures JSON bodies only, and the upload's shape is the vendor's.
func RequestSchema(routeID string) (json.RawMessage, bool) {
	for _, r := range routes() {
		if r.ID != routeID {
			continue
		}
		if len(r.Body) == 0 || r.Multipart {
			return nil, false
		}
		return r.Body, true
	}
	return nil, false
}
