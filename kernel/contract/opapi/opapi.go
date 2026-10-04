// SPDX-License-Identifier: MIT

// Package opapi defines transport-independent operation metadata and host ports.
package opapi

import (
	"context"
	"encoding/json"
	"reflect"
)

type PrincipalKind string

const (
	Operator PrincipalKind = "operator"
	Tenant   PrincipalKind = "tenant"
	Agent    PrincipalKind = "agent"
	System   PrincipalKind = "system"
)

type Caller struct {
	Credential string
	Tenant     string
	Source     string
}

type Principal struct {
	Kind   PrincipalKind
	Tenant string
	Scopes []string
}

type Authz uint8

const (
	PrimaryOnly Authz = iota
	OwnTenant
)

type Tenancy uint8

const (
	Primary Tenancy = iota
	CallerTenant
)

type Stream uint8

const (
	StreamNone Stream = iota
	StreamEvents
	StreamLive
)

type HTTP struct {
	Method string
	Path   string
}

// Spec is the single operation definition consumed by transports. Reflection
// types and derived schemas describe the same Go handler's input and output.
type Spec struct {
	Name              string
	Authz             Authz
	Tenancy           Tenancy
	Stream            Stream
	ReadOnly          bool
	HTTP              HTTP
	Input             reflect.Type
	Output            reflect.Type
	InputSchema       json.RawMessage
	OutputSchema      json.RawMessage
	AllowUnknownInput bool // explicit legacy compatibility; strict by default
}

type Authenticator interface {
	Authenticate(context.Context, Caller) (Principal, error)
}

type TenantRouter interface {
	Route(context.Context, Principal, Spec) (context.Context, error)
}

// AuditRecord deliberately excludes credentials and output bytes. Hosts own
// redaction and persistence; an audit failure is an admission/settlement error.
type AuditRecord struct {
	Operation string
	Principal Principal
	Input     json.RawMessage
}

type AuditSpan interface {
	End(context.Context, error) error
}

type Auditor interface {
	Begin(context.Context, AuditRecord) (AuditSpan, error)
}

// Emitter is the bounded transport-owned publication port for streaming ops.
type Emitter interface {
	Emit(context.Context, any) error
}
