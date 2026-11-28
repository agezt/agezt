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
	"bytes"
	"encoding/json"
	"fmt"
	"reflect"
	"strings"

	"github.com/agezt/agezt/kernel/contract/boardapi"
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
	Workflow Object `json:"workflow,omitempty"`
	Reason   string `json:"reason,omitempty"`
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

// Object is a request field that carries a JSON object.
//
// The row says "an object" and not "an object or null": a missing object and a
// null one are different answers, and the daemon reads the difference.
type Object struct {
	value map[string]any
}

// ObjectOf wraps a map as a field that carries a JSON object.
func ObjectOf(value map[string]any) Object { return Object{value: value} }

// UnmarshalJSON reads the object as it came.
func (o *Object) UnmarshalJSON(data []byte) error {
	if bytes.Equal(bytes.TrimSpace(data), []byte("null")) {
		return fmt.Errorf("must be an object")
	}
	var decoded map[string]any
	if err := json.Unmarshal(data, &decoded); err != nil {
		return err
	}
	o.value = decoded
	return nil
}

// MarshalJSON writes the object as it came.
func (o Object) MarshalJSON() ([]byte, error) { return json.Marshal(o.value) }

// Value returns the object the field carries.
func (o Object) Value() any { return o.value }

// Map returns the object the field carries.
func (o Object) Map() map[string]any { return o.value }

// JSONObject marks the type as carrying a JSON object, so the schema derives
// the object itself.
func (Object) JSONObject() {}

// BoolFlag is a request field that carries a boolean — or the STRING a query
// string carries for one, because a browser's query carries "true", not true.
//
// The row says both forms are allowed: the schema derived from it is the union
// of the two. The string is kept as it came, so the reader decides what a
// string means: BoolFlagValue refuses a string that is not a boolean, and
// BoolFlagOrFalse reads anything it does not know as false.
type BoolFlag struct {
	present bool
	kind    flagKind
	boolean bool
	text    string
}

// flagKind says which form the flag arrived as.
type flagKind int

const (
	// flagBoolean is a boolean that arrived as a boolean.
	flagBoolean flagKind = iota
	// flagText is a string that arrived as a string.
	flagText
	// flagOther is a value that is neither — the door refuses it before the
	// reader ever sees it, but the reader refuses it too.
	flagOther
)

// UnmarshalJSON reads the flag as it comes: a boolean, or a string to be read
// as one.
func (f *BoolFlag) UnmarshalJSON(data []byte) error {
	var raw any
	if err := json.Unmarshal(data, &raw); err != nil {
		return err
	}
	switch value := raw.(type) {
	case bool:
		f.present, f.kind, f.boolean, f.text = true, flagBoolean, value, ""
	case string:
		f.present, f.kind, f.boolean, f.text = true, flagText, strictFlag(value), value
	default:
		f.present, f.kind, f.boolean, f.text = true, flagOther, false, ""
	}
	return nil
}

// MarshalJSON writes the flag as the boolean it carries.
func (f BoolFlag) MarshalJSON() ([]byte, error) {
	return json.Marshal(f.boolean)
}

// Value returns the flag's boolean — the value it rides the wire as.
func (f BoolFlag) Value() any { return f.boolean }

// BooleanOrString marks the type as carrying a boolean or a boolean string,
// so the schema derives the union of the two.
func (BoolFlag) BooleanOrString() {}

// BoolFlagValue reads a flag field of a request row: a boolean, or the string
// a query string carries for one ("true", "1", "yes" and their negatives).
//
// A value that is neither is refused, with the daemon's own wording — a flag
// that stops working must not look like a flag that was never set.
func BoolFlagValue(flag BoolFlag, key string) (bool, error) {
	switch {
	case !flag.present:
		return false, nil
	case flag.kind == flagBoolean:
		return flag.boolean, nil
	case flag.kind == flagText:
		switch strings.ToLower(strings.TrimSpace(flag.text)) {
		case "true", "1", "yes":
			return true, nil
		case "false", "0", "no":
			return false, nil
		}
	}
	return false, fmt.Errorf("args.%s must be a boolean", key)
}

// BoolFlagOrFalse reads a flag field of a request row the lenient way: a
// boolean, or the string "true"/"1" — anything else reads as false.
//
// This is the reading the "enabled" flag has always had, and it stays: a
// toggle that was always on must not be refused by a spelling.
func BoolFlagOrFalse(flag BoolFlag) bool {
	switch {
	case !flag.present:
		return false
	case flag.kind == flagBoolean:
		return flag.boolean
	case flag.kind == flagText:
		return strings.EqualFold(flag.text, "true") || flag.text == "1"
	}
	return false
}

// strictFlag reads a flag's string the strict way: it is on when it says so,
// and off when it says the other thing.
func strictFlag(text string) bool {
	switch strings.ToLower(strings.TrimSpace(text)) {
	case "true", "1", "yes":
		return true
	}
	return false
}

// Limit is a request field that carries a number, and knows whether it was
// sent at all.
//
// "No limit asked" and "zero" are different answers, and a row that cannot
// tell them apart would answer both the same way.
type Limit struct {
	present bool
	value   float64
}

// UnmarshalJSON reads the number as it comes.
func (l *Limit) UnmarshalJSON(data []byte) error {
	var decoded float64
	if err := json.Unmarshal(data, &decoded); err != nil {
		return err
	}
	l.present, l.value = true, decoded
	return nil
}

// MarshalJSON writes the number as it came.
func (l Limit) MarshalJSON() ([]byte, error) { return json.Marshal(l.value) }

// Value returns the number the field carries.
func (l Limit) Value() any { return l.value }

// Present reports whether the field carried a number at all: a limit that was
// not sent and a limit of zero are different answers.
func (l Limit) Present() bool { return l.present }

// Number returns the number the field carries, as it came.
func (l Limit) Number() float64 { return l.value }

// JSONNumber marks the type as carrying a number, so the schema derives the
// number itself.
func (Limit) JSONNumber() {}

// LimitValue reads a limit field of a request row: the number it carries, or
// the fallback when the row did not ask for one.
func LimitValue(limit Limit, fallback int) int {
	if !limit.present {
		return fallback
	}
	return int(limit.value)
}

// BoardReadRequest is the board_read request row: the topic to read, the
// cursor to continue from, and the page size.
type BoardReadRequest struct {
	Topic  string `json:"topic,omitempty"`
	Cursor string `json:"cursor,omitempty"`
	Limit  Limit  `json:"limit,omitempty"`
}

// BoardHelpRequest is the board_help request row: how many unanswered requests
// to read.
type BoardHelpRequest struct {
	Limit Limit `json:"limit,omitempty"`
}

// BoardInboxRequest is the board_inbox request row: whose mail to read, whether
// to read all of it, and the page size.
type BoardInboxRequest struct {
	To    string `json:"to,omitempty"`
	All   bool   `json:"all,omitempty"`
	Limit Limit  `json:"limit,omitempty"`
}

// BoardGetRequest is the board_get request row: the message to read.
type BoardGetRequest struct {
	ID string `json:"id,omitempty"`
}

// BoardRepliesRequest is the board_replies request row: the message to read
// the replies of, and the page size.
type BoardRepliesRequest struct {
	ID    string `json:"id,omitempty"`
	Limit Limit  `json:"limit,omitempty"`
}

// BoardAckRequest is the board_ack request row: the message to acknowledge,
// and who acknowledges it.
type BoardAckRequest struct {
	ID string `json:"id,omitempty"`
	By string `json:"by,omitempty"`
}

// WorkflowListRequest is the workflow_list request row: whether the listing
// carries the workflows' runs with it.
type WorkflowListRequest struct {
	WithRuns BoolFlag `json:"with_runs,omitempty"`
}

// WorkflowShowRequest is the workflow_show request row: the workflow's ref.
type WorkflowShowRequest struct {
	Ref string `json:"ref,omitempty"`
}

// WorkflowSaveRequest is the workflow_save request row: the graph to store.
type WorkflowSaveRequest struct {
	Workflow Object `json:"workflow,omitempty"`
}

// WorkflowRemoveRequest is the workflow_remove request row: the workflow's
// ref.
type WorkflowRemoveRequest struct {
	Ref string `json:"ref,omitempty"`
}

// WorkflowSetEnabledRequest is the workflow_set_enabled request row: the
// workflow's ref and whether it is enabled.
type WorkflowSetEnabledRequest struct {
	Ref     string   `json:"ref,omitempty"`
	Enabled BoolFlag `json:"enabled,omitempty"`
}

// WorkflowRunRequest is the workflow_run request row: the workflow's ref, the
// payload to run it with, and whether the run is asynchronous.
type WorkflowRunRequest struct {
	Ref     string   `json:"ref,omitempty"`
	Payload AnyValue `json:"payload,omitempty"`
	Async   bool     `json:"async,omitempty"`
}

// WorkflowDraftRequest is the workflow_draft request row: what the workflow
// is about, and the name it is drafted under.
type WorkflowDraftRequest struct {
	Name        string `json:"name,omitempty"`
	Description string `json:"description,omitempty"`
}

// WorkflowRefineRequest is the workflow_refine request row: the change to
// make, and the graph (or the ref) to make it on.
type WorkflowRefineRequest struct {
	Ref         string `json:"ref,omitempty"`
	Workflow    Object `json:"workflow,omitempty"`
	Instruction string `json:"instruction,omitempty"`
}

// WorkflowRunsRequest is the workflow_runs request row: the workflow's ref and
// how many runs to read.
type WorkflowRunsRequest struct {
	Ref   string `json:"ref,omitempty"`
	Limit Limit  `json:"limit,omitempty"`
}

// WorkflowTemplatesRequest is the workflow_templates request row: the
// template gallery takes no arguments.
type WorkflowTemplatesRequest struct{}

// WorkflowTestNodeRequest is the workflow_test_node request row: the graph,
// the node to try, the data it runs against, and the payload it reads.
type WorkflowTestNodeRequest struct {
	Workflow Object   `json:"workflow,omitempty"`
	Node     string   `json:"node,omitempty"`
	Data     Object   `json:"data,omitempty"`
	Payload  AnyValue `json:"payload,omitempty"`
}

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
	{Operation: WorkflowList, Row: WorkflowListRequest{}},
	{Operation: WorkflowShow, Row: WorkflowShowRequest{}},
	{Operation: WorkflowSave, Row: WorkflowSaveRequest{}},
	{Operation: WorkflowRemove, Row: WorkflowRemoveRequest{}},
	{Operation: WorkflowSetEnabled, Row: WorkflowSetEnabledRequest{}},
	{Operation: WorkflowRun, Row: WorkflowRunRequest{}},
	{Operation: WorkflowDraft, Row: WorkflowDraftRequest{}},
	{Operation: WorkflowRefine, Row: WorkflowRefineRequest{}},
	{Operation: WorkflowRuns, Row: WorkflowRunsRequest{}},
	{Operation: WorkflowTemplates, Row: WorkflowTemplatesRequest{}},
	{Operation: WorkflowTestNode, Row: WorkflowTestNodeRequest{}},
	{Operation: BoardRead, Row: BoardReadRequest{}},
	{Operation: BoardHelp, Row: BoardHelpRequest{}},
	{Operation: BoardInbox, Row: BoardInboxRequest{}},
	{Operation: BoardGet, Row: BoardGetRequest{}},
	{Operation: BoardReplies, Row: BoardRepliesRequest{}},
	{Operation: BoardAck, Row: BoardAckRequest{}},
	// The board's send row is the board's own: the wire row the REST surface
	// decodes and the contract names.
	{Operation: BoardSend, Row: boardapi.Draft{}},
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
		// A field that carries its own wire value rides the wire as the value
		// itself, not as the wrapper that named its type.
		if wrapped, ok := fieldValue.Interface().(interface{ Value() any }); ok {
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
