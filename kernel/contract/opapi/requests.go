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
	"strconv"
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

// Text is a request field that carries a text — and knows whether it was sent
// at all.
//
// A field that was not sent and a field that was sent as an empty text are
// different answers: one says "leave it alone", the other says "clear it".
// A plain string cannot tell them apart, so the row carries the difference.
type Text struct {
	present bool
	value   string
}

// UnmarshalJSON reads the text as it came.
func (t *Text) UnmarshalJSON(data []byte) error {
	var decoded any
	if err := json.Unmarshal(data, &decoded); err != nil {
		return err
	}
	switch value := decoded.(type) {
	case string:
		t.present, t.value = true, value
	case nil:
		// A null carries no text, but it was sent: the field is present and
		// carries the empty text.
		t.present, t.value = true, ""
	default:
		return fmt.Errorf("must be a string")
	}
	return nil
}

// MarshalJSON writes the text as it came.
func (t Text) MarshalJSON() ([]byte, error) { return json.Marshal(t.value) }

// Value returns the text the field carries.
func (t Text) Value() any { return t.value }

// Present reports whether the field carried a text at all.
func (t Text) Present() bool { return t.present }

// String returns the text the field carries.
func (t Text) String() string { return t.value }

// JSONText marks the type as carrying a text, so the schema derives the text
// itself.
func (Text) JSONText() {}

// TextArray is a request field that carries a list of texts — a list, and
// nothing else — and knows whether it was sent at all.
//
// The members are texts: a member that is not a text is refused, the way the
// door refuses it.
type TextArray struct {
	present bool
	members []string
}

// UnmarshalJSON reads the list as it came.
func (a *TextArray) UnmarshalJSON(data []byte) error {
	var decoded any
	if err := json.Unmarshal(data, &decoded); err != nil {
		return err
	}
	switch value := decoded.(type) {
	case []any:
		members := make([]string, 0, len(value))
		for _, member := range value {
			text, ok := member.(string)
			if !ok {
				return fmt.Errorf("must be a list of texts")
			}
			members = append(members, text)
		}
		a.present, a.members = true, members
	case nil:
		// A null carries no list, but it was sent: the field is present and
		// carries the empty list.
		a.present, a.members = true, nil
	default:
		return fmt.Errorf("must be a list of texts")
	}
	return nil
}

// MarshalJSON writes the list as it came.
func (a TextArray) MarshalJSON() ([]byte, error) { return json.Marshal(a.members) }

// Value returns the list the field carries.
func (a TextArray) Value() any { return a.members }

// Present reports whether the field carried a list at all.
func (a TextArray) Present() bool { return a.present }

// Members returns the texts the list carries.
func (a TextArray) Members() []string { return a.members }

// TextArray marks the type as carrying a list of texts, so the schema derives
// the list itself.
func (TextArray) TextArray() {}

// Integer is a request field that carries an integer — and knows whether it
// was sent at all.
//
// A count that was not sent and a count of zero are different answers, the
// same way they are for a limit.
type Integer struct {
	present bool
	value   int64
}

// UnmarshalJSON reads the integer as it came.
func (i *Integer) UnmarshalJSON(data []byte) error {
	trimmed := bytes.TrimSpace(data)
	if bytes.Equal(trimmed, []byte("null")) {
		// A null carries no integer, but it was sent: the field is present and
		// carries the zero.
		i.present, i.value = true, 0
		return nil
	}
	// The number is read as it was written, so a count that does not fit in a
	// float still reads as itself. A string is not a number, even when it
	// carries one.
	if len(trimmed) == 0 || trimmed[0] == '"' {
		return fmt.Errorf("must be an integer")
	}
	var number json.Number
	if err := json.Unmarshal(trimmed, &number); err != nil {
		return fmt.Errorf("must be an integer")
	}
	value, err := number.Int64()
	if err != nil {
		return fmt.Errorf("must be an integer")
	}
	i.present, i.value = true, value
	return nil
}

// MarshalJSON writes the integer as it came.
func (i Integer) MarshalJSON() ([]byte, error) { return json.Marshal(i.value) }

// Value returns the integer the field carries.
func (i Integer) Value() any { return i.value }

// Present reports whether the field carried an integer at all.
func (i Integer) Present() bool { return i.present }

// Number returns the integer the field carries.
func (i Integer) Number() int64 { return i.value }

// JSONInteger marks the type as carrying an integer, so the schema derives
// the integer itself.
func (Integer) JSONInteger() {}

// Object is a request field that carries a JSON object.
//
// The row says "an object" and not "an object or null": a missing object and a
// null one are different answers, and the daemon reads the difference.
type Object struct {
	present bool
	value   map[string]any
}

// ObjectOf wraps a map as a field that carries a JSON object.
func ObjectOf(value map[string]any) Object {
	return Object{present: true, value: value}
}

// UnmarshalJSON reads the object as it came.
func (o *Object) UnmarshalJSON(data []byte) error {
	if bytes.Equal(bytes.TrimSpace(data), []byte("null")) {
		return fmt.Errorf("must be an object")
	}
	// The object's numbers are read as they were written: a number that does not
	// fit in a float still reads as itself, because the row carries the object
	// and the domain reads it back.
	decoder := json.NewDecoder(bytes.NewReader(data))
	decoder.UseNumber()
	var decoded map[string]any
	if err := decoder.Decode(&decoded); err != nil {
		return err
	}
	o.value = decoded
	o.present = true
	return nil
}

// MarshalJSON writes the object as it came.
func (o Object) MarshalJSON() ([]byte, error) { return json.Marshal(o.value) }

// Value returns the object the field carries.
func (o Object) Value() any { return o.value }

// Map returns the object the field carries.
func (o Object) Map() map[string]any { return o.value }

// Present reports whether the field carried an object at all: an object that
// was not sent and an empty one are different answers.
func (o Object) Present() bool { return o.present }

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

// NumberFlag is a request field that carries a number — or the STRING a
// query string carries for one.
//
// The row says both forms are allowed: the schema derived from it is the union
// of the two. The string is kept as it came, so the reader decides what it
// means: the number it parses, and nothing at all when it does not parse.
type NumberFlag struct {
	present bool
	isText  bool
	number  float64
	text    string
}

// UnmarshalJSON reads the number as it comes: a number, or a string to be read
// as one.
func (n *NumberFlag) UnmarshalJSON(data []byte) error {
	var raw any
	if err := json.Unmarshal(data, &raw); err != nil {
		return err
	}
	switch value := raw.(type) {
	case float64:
		n.present, n.isText, n.number, n.text = true, false, value, ""
	case string:
		n.present, n.isText, n.number, n.text = true, true, 0, value
	case nil:
		// A null carries no number, but it was sent: the field is present and
		// carries the zero.
		n.present, n.isText, n.number, n.text = true, false, 0, ""
	default:
		return fmt.Errorf("must be a number")
	}
	return nil
}

// MarshalJSON writes the number as it came.
func (n NumberFlag) MarshalJSON() ([]byte, error) {
	if n.isText {
		return json.Marshal(n.text)
	}
	return json.Marshal(n.number)
}

// Value returns the number the field carries.
func (n NumberFlag) Value() any {
	if n.isText {
		return n.text
	}
	return n.number
}

// NumberOrString marks the type as carrying a number or the string a query
// string carries for one, so the schema derives the union of the two.
func (NumberFlag) NumberOrString() {}

// NumberFlagValue reads a number field of a request row: the number it
// carries, or the number its string parses to.
//
// A string that does not parse is no number at all: the parse error is
// ignored, and the value it would have had — a range error's ±Inf — is kept.
func NumberFlagValue(flag NumberFlag) float64 {
	if !flag.present {
		return 0
	}
	if flag.isText {
		number, _ := strconv.ParseFloat(strings.TrimSpace(flag.text), 64)
		return number
	}
	return flag.number
}

// TextList is a request field that carries a list of texts — or the ONE
// comma-separated string a caller sends instead of a list.
//
// The members stay unconstrained on purpose: the handler filters the ones it
// cannot use rather than refusing the whole body, so the list says "texts" and
// the reading says which of them count.
type TextList struct {
	present bool
	isText  bool
	text    string
	members []any
}

// UnmarshalJSON reads the list as it came: a list, or one string to be split.
func (l *TextList) UnmarshalJSON(data []byte) error {
	var raw any
	if err := json.Unmarshal(data, &raw); err != nil {
		return err
	}
	switch value := raw.(type) {
	case []any:
		l.present, l.isText, l.text, l.members = true, false, "", value
	case string:
		l.present, l.isText, l.text, l.members = true, true, value, nil
	default:
		return fmt.Errorf("must be a list of texts")
	}
	return nil
}

// MarshalJSON writes the list as it came.
func (l TextList) MarshalJSON() ([]byte, error) {
	if l.isText {
		return json.Marshal(l.text)
	}
	return json.Marshal(l.members)
}

// Value returns the list the field carries.
func (l TextList) Value() any {
	if l.isText {
		return l.text
	}
	return l.members
}

// TextList marks the type as carrying a list of texts or one comma-separated
// string, so the schema derives the union of the two.
func (TextList) TextList() {}

// TextListValue reads a list field of a request row: the texts it carries.
//
// A list's members are trimmed and the blank ones dropped; a string is split
// on commas, the way a caller sends one list as one field.
func TextListValue(list TextList) []string {
	if !list.present {
		return nil
	}
	if list.isText {
		if strings.TrimSpace(list.text) == "" {
			return nil
		}
		return strings.Split(list.text, ",")
	}
	out := make([]string, 0, len(list.members))
	for _, member := range list.members {
		if text, ok := member.(string); ok && strings.TrimSpace(text) != "" {
			out = append(out, strings.TrimSpace(text))
		}
	}
	return out
}

// WorkboardListRequest is the workboard_list request row: which tasks to
// list, and how many.
type WorkboardListRequest struct {
	Status          string `json:"status,omitempty"`
	Tenant          string `json:"tenant,omitempty"`
	Assignee        string `json:"assignee,omitempty"`
	IncludeArchived bool   `json:"include_archived,omitempty"`
	Limit           Limit  `json:"limit,omitempty"`
}

// WorkboardShowRequest is the workboard_show request row: the task to read.
type WorkboardShowRequest struct {
	ID string `json:"id,omitempty"`
}

// WorkboardCreateRequest is the workboard_create request row: the task to
// create, who owns it, and how it may be retried.
type WorkboardCreateRequest struct {
	Title          string   `json:"title,omitempty"`
	Description    string   `json:"description,omitempty"`
	Status         string   `json:"status,omitempty"`
	Priority       float64  `json:"priority,omitempty"`
	Tenant         string   `json:"tenant,omitempty"`
	Assignee       string   `json:"assignee,omitempty"`
	Owner          string   `json:"owner,omitempty"`
	IdempotencyKey string   `json:"idempotency_key,omitempty"`
	Tags           TextList `json:"tags,omitempty"`
	Artifacts      TextList `json:"artifacts,omitempty"`
	Criteria       TextList `json:"criteria,omitempty"`
	Seat           string   `json:"seat,omitempty"`
	EscalateTo     string   `json:"escalate_to,omitempty"`
	CorrelationID  string   `json:"correlation_id,omitempty"`
	MaxAttempts    Limit    `json:"max_attempts,omitempty"`
}

// WorkboardClaimRequest is the workboard_claim request row: the task to
// claim, who claims it, and the run it belongs to.
type WorkboardClaimRequest struct {
	ID            string `json:"id,omitempty"`
	Agent         string `json:"agent,omitempty"`
	RunID         string `json:"run_id,omitempty"`
	CorrelationID string `json:"correlation_id,omitempty"`
}

// WorkboardCommentRequest is the workboard_comment request row: the task to
// comment on, and the comment.
type WorkboardCommentRequest struct {
	ID            string `json:"id,omitempty"`
	Author        string `json:"author,omitempty"`
	Body          string `json:"body,omitempty"`
	CorrelationID string `json:"correlation_id,omitempty"`
}

// WorkboardReasonRequest is the request row of the workboard's reasoned
// mutations — block, fail, unblock, complete and archive: the task, who acts,
// and why.
type WorkboardReasonRequest struct {
	ID            string `json:"id,omitempty"`
	Actor         string `json:"actor,omitempty"`
	Reason        string `json:"reason,omitempty"`
	CorrelationID string `json:"correlation_id,omitempty"`
}

// WorkboardProveRequest is the workboard_prove request row: the task to prove
// and the answer that proves it.
type WorkboardProveRequest struct {
	ID            string `json:"id,omitempty"`
	Answer        string `json:"answer,omitempty"`
	CorrelationID string `json:"correlation_id,omitempty"`
}

// WorkboardSeatRequest is the workboard_seat request row: the task and the
// execution seat it runs on.
type WorkboardSeatRequest struct {
	ID   string `json:"id,omitempty"`
	Seat string `json:"seat,omitempty"`
}

// WorkboardLinkRequest is the workboard_link request row: the task, the kind
// of link, and what it links to.
type WorkboardLinkRequest struct {
	ID            string `json:"id,omitempty"`
	Type          string `json:"type,omitempty"`
	Target        string `json:"target,omitempty"`
	CorrelationID string `json:"correlation_id,omitempty"`
}

// WorkboardDependRequest is the workboard_depend request row: the task and
// the task it depends on.
type WorkboardDependRequest struct {
	ID            string `json:"id,omitempty"`
	DependsOn     string `json:"depends_on,omitempty"`
	On            string `json:"on,omitempty"`
	CorrelationID string `json:"correlation_id,omitempty"`
}

// WorkboardReclaimRequest is the workboard_reclaim request row: the task to
// reclaim, who reclaims it, and how stale it must be.
type WorkboardReclaimRequest struct {
	ID            string `json:"id,omitempty"`
	Actor         string `json:"actor,omitempty"`
	StaleAfterMS  Limit  `json:"stale_after_ms,omitempty"`
	CorrelationID string `json:"correlation_id,omitempty"`
}

// WorkboardSweepRequest is the workboard_sweep request row: who sweeps, how
// stale a task must be, and how many to sweep.
type WorkboardSweepRequest struct {
	Actor         string `json:"actor,omitempty"`
	StaleAfterMS  Limit  `json:"stale_after_ms,omitempty"`
	Limit         Limit  `json:"limit,omitempty"`
	CorrelationID string `json:"correlation_id,omitempty"`
}

// WorkboardWatchRequest is the workboard_watch request row: the task to
// watch, the run to watch it in, and how many frames to keep.
type WorkboardWatchRequest struct {
	ID    string `json:"id,omitempty"`
	RunID string `json:"run_id,omitempty"`
	Limit Limit  `json:"limit,omitempty"`
}

// WorkboardDispatchRequest is the workboard_dispatch request row: the task
// to dispatch, to whom, why, and with what intent.
type WorkboardDispatchRequest struct {
	ID     string `json:"id,omitempty"`
	Agent  string `json:"agent,omitempty"`
	Reason string `json:"reason,omitempty"`
	Intent string `json:"intent,omitempty"`
}

// WorkboardPolicyRequest is the workboard_policy request row: the task, the
// retry policy, and whether it is cleared.
type WorkboardPolicyRequest struct {
	ID            string `json:"id,omitempty"`
	Actor         string `json:"actor,omitempty"`
	Clear         bool   `json:"clear,omitempty"`
	MaxAttempts   Limit  `json:"max_attempts,omitempty"`
	EscalateTo    string `json:"escalate_to,omitempty"`
	CorrelationID string `json:"correlation_id,omitempty"`
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

// AgentRefRequest is the request row of the operations that name ONE agent:
// the agent's ref.
//
// The ref is the family's own text: it is required, and it is not trimmed —
// the agent's name is the name it was given.
type AgentRefRequest struct {
	Ref string `json:"ref,omitempty"`
}

// AgentPageRequest is the request row of the operations that read one agent's
// page: the agent's ref, the page size, and the cursor to continue from.
type AgentPageRequest struct {
	Ref    string `json:"ref,omitempty"`
	Limit  Limit  `json:"limit,omitempty"`
	Cursor string `json:"cursor,omitempty"`
}

// AgentListRequest is the agent_list request row: the page size and the
// cursor to continue from.
type AgentListRequest struct {
	Limit  Limit  `json:"limit,omitempty"`
	Cursor string `json:"cursor,omitempty"`
}

// AgentGraveyardRequest is the agent_graveyard request row: how old a
// retirement must be to be reported.
//
// The age is a number — or the string a query string carries for one.
type AgentGraveyardRequest struct {
	OlderThanDays NumberFlag `json:"older_than_days,omitempty"`
}

// AgentCapabilitiesRequest is the agent_capabilities request row: the agent's
// ref, and the capability fields the operator patches.
//
// Every patch field knows whether it was sent: a field that was not sent is
// left alone, and a field that was sent is patched — even when it was sent
// empty.
type AgentCapabilitiesRequest struct {
	Ref             string    `json:"ref,omitempty"`
	TrustCeiling    Text      `json:"trust_ceiling,omitempty"`
	ToolAllow       TextArray `json:"tool_allow,omitempty"`
	ToolDeny        TextArray `json:"tool_deny,omitempty"`
	NoisePolicy     Object    `json:"noise_policy,omitempty"`
	ConfigOverrides Object    `json:"config_overrides,omitempty"`
	MemoryScope     Text      `json:"memory_scope,omitempty"`
	Workdir         Text      `json:"workdir,omitempty"`
	MaxCostMc       Integer   `json:"max_cost_mc,omitempty"`
	MaxDailyMc      Integer   `json:"max_daily_mc,omitempty"`
}

// AgentProfileAddRequest is the agent_add request row: the profile to create.
type AgentProfileAddRequest struct {
	Profile Object `json:"profile,omitempty"`
}

// AgentProfileEditRequest is the agent_edit request row: the agent's ref, and
// the profile to write.
type AgentProfileEditRequest struct {
	Ref     string `json:"ref,omitempty"`
	Profile Object `json:"profile,omitempty"`
}

// AgentRemoveRequest is the agent_remove request row: the agent's ref, and
// what the removal cleans up besides the profile.
type AgentRemoveRequest struct {
	Ref     string `json:"ref,omitempty"`
	Cascade Object `json:"cascade,omitempty"`
}

// AgentRepairRequest is the agent_repair request row: the agent's ref, why it
// is repaired, and the incident chain it belongs to.
type AgentRepairRequest struct {
	Ref              string `json:"ref,omitempty"`
	Reason           string `json:"reason,omitempty"`
	IncidentID       string `json:"incident_id,omitempty"`
	RootIncidentID   string `json:"root_incident_id,omitempty"`
	ParentIncidentID string `json:"parent_incident_id,omitempty"`
}

// AgentResolveRequest is the agent_resolve request row: the agent's ref, the
// resolution to apply, and the routing it carries.
type AgentResolveRequest struct {
	Ref              string    `json:"ref,omitempty"`
	Resolution       string    `json:"resolution,omitempty"`
	Summary          string    `json:"summary,omitempty"`
	DelegateTo       string    `json:"delegate_to,omitempty"`
	TaskType         string    `json:"task_type,omitempty"`
	TaskModelChain   TextArray `json:"task_model_chain,omitempty"`
	IncidentID       string    `json:"incident_id,omitempty"`
	RootIncidentID   string    `json:"root_incident_id,omitempty"`
	ParentIncidentID string    `json:"parent_incident_id,omitempty"`
}

// AgentSetEnabledRequest is the agent_set_enabled request row: the agent's
// ref, and whether it is enabled.
//
// The flag is a boolean — or the string a query string carries for one.
type AgentSetEnabledRequest struct {
	Ref     string   `json:"ref,omitempty"`
	Enabled BoolFlag `json:"enabled,omitempty"`
}

// AgentSetRetiredRequest is the request row of the roster's retirement
// mutations — retire and revive: the agent's ref, and why.
type AgentSetRetiredRequest struct {
	Ref    string `json:"ref,omitempty"`
	Reason string `json:"reason,omitempty"`
}

// AgentTaskUpdateRequest is the agent_task_update request row: the agent's
// ref, the operation, the task it carries, and the task's own fields.
//
// The task's fields know whether they were sent: presence, not value, decides
// which fields an update touches.
type AgentTaskUpdateRequest struct {
	Ref         string `json:"ref,omitempty"`
	Op          string `json:"op,omitempty"`
	Task        Object `json:"task,omitempty"`
	ID          Text   `json:"id,omitempty"`
	Title       Text   `json:"title,omitempty"`
	Description Text   `json:"description,omitempty"`
	Scope       Text   `json:"scope,omitempty"`
	Status      Text   `json:"status,omitempty"`
}

// AgentWakeRequest is the agent_wake request row: the agent's ref, the
// intent, why, and the incident chain it belongs to.
type AgentWakeRequest struct {
	Ref              string `json:"ref,omitempty"`
	Intent           string `json:"intent,omitempty"`
	Reason           string `json:"reason,omitempty"`
	IncidentID       string `json:"incident_id,omitempty"`
	RootIncidentID   string `json:"root_incident_id,omitempty"`
	ParentIncidentID string `json:"parent_incident_id,omitempty"`
}

// RequestRow binds one operation to its request row.
type RequestRow struct {
	// Operation is the operation's name.
	Operation string
	// Row is the request row, as a value: its type is the row's shape.
	Row any
}

// RequestRows are the request rows of the contract's operations.
//
// The binding lives with the rows: an operation and its request shape are one
// fact, and a list that says which row belongs to which operation is the
// other half of that fact — so the rows and their operations cannot drift
// apart.
//
// The table holds the console's calls and the daemon's own operations alike:
// a row that is not called from the console is still a row the daemon reads,
// and a row the contract does not name is a shape somebody wrote by hand.
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
	// The workboard's twenty-one operations share fifteen rows: the lanes
	// read what the listing reads, the heartbeat carries what the claim
	// carries, and the five reasoned mutations — block, fail, unblock,
	// complete, archive — are one reasoned mutation.
	{Operation: WorkboardList, Row: WorkboardListRequest{}},
	{Operation: WorkboardLanes, Row: WorkboardListRequest{}},
	{Operation: WorkboardShow, Row: WorkboardShowRequest{}},
	{Operation: WorkboardCreate, Row: WorkboardCreateRequest{}},
	{Operation: WorkboardClaim, Row: WorkboardClaimRequest{}},
	{Operation: WorkboardHeartbeat, Row: WorkboardClaimRequest{}},
	{Operation: WorkboardComment, Row: WorkboardCommentRequest{}},
	{Operation: WorkboardBlock, Row: WorkboardReasonRequest{}},
	{Operation: WorkboardFail, Row: WorkboardReasonRequest{}},
	{Operation: WorkboardUnblock, Row: WorkboardReasonRequest{}},
	{Operation: WorkboardComplete, Row: WorkboardReasonRequest{}},
	{Operation: WorkboardArchive, Row: WorkboardReasonRequest{}},
	{Operation: WorkboardProve, Row: WorkboardProveRequest{}},
	{Operation: WorkboardSeat, Row: WorkboardSeatRequest{}},
	{Operation: WorkboardLink, Row: WorkboardLinkRequest{}},
	{Operation: WorkboardPolicy, Row: WorkboardPolicyRequest{}},
	{Operation: WorkboardDepend, Row: WorkboardDependRequest{}},
	{Operation: WorkboardReclaim, Row: WorkboardReclaimRequest{}},
	{Operation: WorkboardSweep, Row: WorkboardSweepRequest{}},
	{Operation: WorkboardWatch, Row: WorkboardWatchRequest{}},
	{Operation: WorkboardDispatch, Row: WorkboardDispatchRequest{}},
	// The roster's read half: the operations that name one agent, the ones
	// that read one agent's page, the listing, and the graveyard's age.
	{Operation: AgentActivity, Row: AgentPageRequest{}},
	{Operation: AgentEscalations, Row: AgentPageRequest{}},
	{Operation: AgentRepairStatus, Row: AgentPageRequest{}},
	{Operation: AgentImpact, Row: AgentRefRequest{}},
	{Operation: AgentTombstone, Row: AgentRefRequest{}},
	{Operation: AgentPermissions, Row: AgentRefRequest{}},
	{Operation: AgentList, Row: AgentListRequest{}},
	{Operation: AgentGraveyard, Row: AgentGraveyardRequest{}},
	// The roster's write half: the capability patch, the profile writes, the
	// removal, the repair, the resolve, the enable, the retirement, the task
	// update and the wake.
	{Operation: AgentCapabilities, Row: AgentCapabilitiesRequest{}},
	{Operation: AgentAdd, Row: AgentProfileAddRequest{}},
	{Operation: AgentEdit, Row: AgentProfileEditRequest{}},
	{Operation: AgentRemove, Row: AgentRemoveRequest{}},
	{Operation: AgentRepair, Row: AgentRepairRequest{}},
	{Operation: AgentResolve, Row: AgentResolveRequest{}},
	{Operation: AgentSetEnabled, Row: AgentSetEnabledRequest{}},
	{Operation: AgentRetire, Row: AgentSetRetiredRequest{}},
	{Operation: AgentRevive, Row: AgentSetRetiredRequest{}},
	{Operation: AgentTaskUpdate, Row: AgentTaskUpdateRequest{}},
	{Operation: AgentWake, Row: AgentWakeRequest{}},
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
