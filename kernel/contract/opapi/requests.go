// SPDX-License-Identifier: MIT

package opapi

// The request rows the console's operations accept.
//
// A request row is the SHAPE of an operation's request: the fields it names,
// their types, and which of them must arrive. ONE declaration, three readers:
//
//   - the operation registry derives the operation's input schema from it, so
//     the published contract and the row cannot drift;
//   - the application layer decodes it, so the runtime accepts what the
//     schema promises;
//   - the console builds its arguments from it, so a caller cannot spell a
//     field the operation does not have.
//
// The rules the whole tree reads the same way:
//
//   - a field WITHOUT `omitempty` is REQUIRED — the daemon's door refuses a
//     request that does not carry it;
//   - a field WITH `omitempty` may be absent, and Args leaves it out when it
//     holds its zero value, exactly as the wire has always carried it.
//
// A row that is written twice — once here, once in the surface that sends it —
// is a row with two homes, and two homes drift: the surface keeps sending a
// field the operation never declared, and nothing says so.

import (
	"encoding/json"
	"fmt"
	"reflect"
	"strings"

	"github.com/agezt/agezt/kernel/contract/skillapi"
)

// ArtifactGetRequest is the artifact_get request row: the artifact's ref.
type ArtifactGetRequest struct {
	Ref string `json:"ref,omitempty"`
}

// FileMkdirRequest is the file_mkdir request row: the directory to create,
// and whether its missing parents are created too.
type FileMkdirRequest struct {
	Path    string `json:"path,omitempty"`
	Parents bool   `json:"parents,omitempty"`
}

// FileRenameRequest is the file_rename request row: the entry's current name
// and its new one.
type FileRenameRequest struct {
	From string `json:"from,omitempty"`
	To   string `json:"to,omitempty"`
}

// FileDeleteRequest is the file_delete request row: the entry to remove, and
// whether a directory is removed with its contents.
type FileDeleteRequest struct {
	Path      string `json:"path,omitempty"`
	Recursive bool   `json:"recursive,omitempty"`
}

// FileRestoreRequest is the file_restore request row: the checkpoint to
// restore.
type FileRestoreRequest struct {
	ID string `json:"id,omitempty"`
}

// SkillRestoreRequest is the skill_restore request row: the skill, the status
// it is restored to, and why.
//
// `id` and `status` are REQUIRED: a restore without them has nothing to
// restore.
type SkillRestoreRequest struct {
	ID     string          `json:"id"`
	Status skillapi.Status `json:"status"`
	Reason string          `json:"reason,omitempty"`
}

// WorkflowRestoreRequest is the workflow_restore request row: the workflow
// snapshot to restore and why.
type WorkflowRestoreRequest struct {
	Workflow map[string]any `json:"workflow,omitempty"`
	Reason   string         `json:"reason,omitempty"`
}

// ConfigSetRequest is the config_set request row: the setting's name and the
// value to store under it.
type ConfigSetRequest struct {
	Name  string `json:"name,omitempty"`
	Value string `json:"value,omitempty"`
}

// ChannelOAuthCallbackRequest is the channel_oauth_callback request row: the
// provider's authorization code and the state it was asked for.
type ChannelOAuthCallbackRequest struct {
	Code  string `json:"code,omitempty"`
	State string `json:"state,omitempty"`
}

// WorkflowWebhookRequest is the workflow_webhook request row: the workflow's
// name, the secret that authenticates the call, and the payload to run.
type WorkflowWebhookRequest struct {
	Ref     string   `json:"ref,omitempty"`
	Secret  string   `json:"secret,omitempty"`
	Payload AnyValue `json:"payload,omitempty"`
}

// AnyValue is a request field that carries ANY JSON value.
//
// The daemon reads it as it comes, so the row says "any JSON value" — and the
// schema derived from the row names exactly that (the union of the JSON
// types) rather than leaving an empty property schema that says nothing.
type AnyValue struct {
	value any
}

// AnyValueOf wraps a value as a field that carries any JSON value.
func AnyValueOf(value any) AnyValue { return AnyValue{value: value} }

// AnyJSONValue marks the type as carrying any JSON value, so the schema
// derives the union of the JSON types for it.
func (AnyValue) AnyJSONValue() {}

// UnmarshalJSON reads the value as it comes.
func (v *AnyValue) UnmarshalJSON(data []byte) error {
	return json.Unmarshal(data, &v.value)
}

// MarshalJSON writes the value as it came.
func (v AnyValue) MarshalJSON() ([]byte, error) {
	if v.value == nil {
		return []byte("null"), nil
	}
	return json.Marshal(v.value)
}

// Value returns the value the field carries.
func (v AnyValue) Value() any { return v.value }

// RequestRow binds one operation to its request row.
type RequestRow struct {
	// Operation is the operation's name.
	Operation string
	// Row is the request row, as a value: its type is the row's shape.
	Row any
}

// RequestRows are the request rows of the operations the console calls.
//
// The binding lives with the rows: an operation and its request shape are one
// fact, and a list that says which row belongs to which operation is the
// other half of that fact — so the rows and their operations cannot drift
// apart.
var RequestRows = []RequestRow{
	{Operation: ArtifactGet, Row: ArtifactGetRequest{}},
	{Operation: FileMkdir, Row: FileMkdirRequest{}},
	{Operation: FileRename, Row: FileRenameRequest{}},
	{Operation: FileDelete, Row: FileDeleteRequest{}},
	{Operation: FileRestore, Row: FileRestoreRequest{}},
	{Operation: SkillRestore, Row: SkillRestoreRequest{}},
	{Operation: WorkflowRestore, Row: WorkflowRestoreRequest{}},
	{Operation: ConfigSet, Row: ConfigSetRequest{}},
	{Operation: ChannelOAuthCallback, Row: ChannelOAuthCallbackRequest{}},
	{Operation: WorkflowWebhook, Row: WorkflowWebhookRequest{}},
}

// Args returns the operation arguments a request row describes.
//
// The field names come from the row's own json tags, so a caller cannot spell
// an argument the row does not carry. A field the row marks optional and
// that holds its zero value is left out — the wire has always carried only
// the arguments that were set.
func Args(row any) map[string]any {
	args := map[string]any{}
	value := reflect.ValueOf(row)
	for value.Kind() == reflect.Pointer {
		if value.IsNil() {
			return args
		}
		value = value.Elem()
	}
	if value.Kind() != reflect.Struct {
		return args
	}
	shape := value.Type()
	for i := 0; i < shape.NumField(); i++ {
		field := shape.Field(i)
		if !field.IsExported() {
			continue
		}
		name, options, _ := strings.Cut(field.Tag.Get("json"), ",")
		if name == "" || name == "-" {
			continue
		}
		fieldValue := value.Field(i)
		if strings.Contains(options, "omitempty") && fieldValue.IsZero() {
			continue
		}
		// A field that carries any JSON value rides the wire as the value
		// itself, not as the wrapper that named its type.
		if wrapped, ok := fieldValue.Interface().(AnyValue); ok {
			args[name] = wrapped.Value()
			continue
		}
		args[name] = fieldValue.Interface()
	}
	return args
}

// RequiredText reads a text field of a request row: a blank value is refused,
// with the daemon's own wording.
func RequiredText(value, key string) (string, error) {
	if strings.TrimSpace(value) == "" {
		return "", fmt.Errorf("args.%s required", key)
	}
	return value, nil
}
