// SPDX-License-Identifier: MIT

package restapi

// The golden fixtures: the daemon's own REST answers, recorded as bytes.
//
// The SDKs parse what this daemon answers. Until now each SDK tested itself
// against payloads its own test wrote — so a change in the daemon's wire shape
// (a renamed field, a different envelope) could pass every SDK test and still
// break every SDK. The fixtures below are the SERVER's answers: this file
// drives the real handlers with a deterministic engine and a deterministic
// board, records the bytes, and the SDK tests parse those bytes with their
// own types.
//
// The files live in sdk/fixtures/ and are compared byte-for-byte by
// TestTheGoldenFixturesAreInSync. Regenerate them with:
//
//	AGEZT_UPDATE_GOLDEN=1 go test ./kernel/restapi/ -run TestUpdateTheGoldenFixtures
//
// The mailbox watch (an open-ended SSE stream) has no fixture: its frames are
// timing-dependent, and every SDK's own suite covers the stream format.

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"testing"

	"github.com/agezt/agezt/kernel/bus"
	"github.com/agezt/agezt/kernel/contract/boardapi"
	"github.com/agezt/agezt/kernel/contract/restapiapi"
	"github.com/agezt/agezt/kernel/event"
	"github.com/agezt/agezt/kernel/journal"
)

// goldenFixture is one recorded answer.
type goldenFixture struct {
	// ID is the endpoint's id in the REST contract.
	ID string `json:"id"`
	// Method is the verb the fixture answers.
	Method string `json:"method"`
	// Path is the contract's path TEMPLATE ("/api/v1/mailbox/messages/{id}/ack")
	// — the same string the SDKs' generated catalogs hold.
	Path string `json:"path"`
	// Request is the concrete path the recorder asked for.
	Request string `json:"request"`
	// Status is the HTTP status the fixture answers.
	Status int `json:"status"`
	// File is the fixture's file name inside sdk/fixtures/. Empty for an
	// endpoint that has no fixture (an open-ended stream).
	File string `json:"file"`
	// Note says why an endpoint has no fixture.
	Note string `json:"note"`
}

// goldenManifest is the index of the fixtures.
type goldenManifest struct {
	// Note says what the fixtures are.
	Note string `json:"note"`
	// Fixtures are the recorded answers.
	Fixtures []goldenFixture `json:"fixtures"`
}

// goldenDir is where the fixtures live, relative to this package.
const goldenDir = "../../sdk/fixtures"

// goldenRequestsFile is the request-body fixture's name inside goldenDir.
const goldenRequestsFile = "requests.json"

// goldenRequests is the request-body fixture: the wire bodies the daemon's
// REST contract declares.
//
// The fixtures above are what the daemon ANSWERS; this one is what it ACCEPTS.
// The contract's request rows are rendered by the contract itself
// (restapiapi.RowBody), so the Python, TypeScript and Rust SDKs all have to
// serialize their rows to the very same bytes — one vocabulary, three
// languages.
type goldenRequests struct {
	// Note says what the fixture is.
	Note string `json:"note"`
	// Rows are the request bodies, by row name.
	Rows map[string]json.RawMessage `json:"rows"`
}

// goldenRequestRows are the request bodies the contract's request rows
// declare, with the values this recorder renders them from. The rows are the
// contract's; the values are this file's.
var goldenRequestRows = []struct {
	// Row is the contract's row name.
	Row string
	// Values are the row's field values.
	Values map[string]any
}{
	{restapiapi.RowRunRequest, map[string]any{
		"intent": "golden intent",
		"model":  "model-a",
		"stream": true,
	}},
	{restapiapi.RowMailDraft, map[string]any{
		"text":           "golden message",
		"from":           "agent-a",
		"to":             "agent-b",
		"topic":          "dm",
		"reply_to":       "m-1",
		"help":           true,
		"correlation_id": "corr-golden",
	}},
	{restapiapi.RowMailAck, map[string]any{
		"by": "agent-b",
	}},
}

// goldenRequestFixture renders the request-body fixture: the contract's own
// rows, rendered by the contract.
func goldenRequestFixture(t *testing.T) []byte {
	t.Helper()
	rows := map[string]json.RawMessage{}
	for _, c := range goldenRequestRows {
		body, err := restapiapi.RowBody(c.Row, c.Values)
		if err != nil {
			t.Fatalf("render the %q row: %v", c.Row, err)
		}
		rows[c.Row] = json.RawMessage(body)
	}
	fixture := goldenRequests{
		Note: "The request bodies the daemon's REST contract declares, rendered by the " +
			"contract itself (kernel/contract/restapiapi's RowBody). Every SDK's request " +
			"row must serialize to these bytes. Regenerate with " +
			"AGEZT_UPDATE_GOLDEN=1 go test ./kernel/restapi/ -run TestUpdateTheGoldenFixtures",
		Rows: rows,
	}
	out, err := json.MarshalIndent(fixture, "", "  ")
	if err != nil {
		t.Fatalf("marshal the request fixture: %v", err)
	}
	return append(out, '\n')
}

// goldenCases are the requests the recorder makes, in order.
func goldenCases() []struct {
	id     string
	method string
	path   string
	body   string
} {
	return []struct {
		id     string
		method string
		path   string
		body   string
	}{
		{"health", http.MethodGet, "/api/v1/health", ""},
		{"models", http.MethodGet, "/api/v1/models", ""},
		{"run", http.MethodPost, "/api/v1/runs", `{"intent":"golden intent","model":"model-a"}`},
		{"run_arc", http.MethodGet, "/api/v1/runs/corr-golden", ""},
		{"mailbox_messages", http.MethodGet, "/api/v1/mailbox/messages?topic=dm", ""},
		{"mailbox_send", http.MethodPost, "/api/v1/mailbox/messages",
			`{"text":"golden message","from":"agent-a","to":"agent-b","topic":"dm"}`},
		{"mailbox_ack", http.MethodPost, "/api/v1/mailbox/messages/m-1/ack", `{"by":"agent-b"}`},
		{"mailbox_replies", http.MethodGet, "/api/v1/mailbox/messages/m-1/replies", ""},
		{"mailbox_inbox", http.MethodGet, "/api/v1/mailbox/inbox?name=agent-b", ""},
		{"mailbox_topics", http.MethodGet, "/api/v1/mailbox/topics", ""},
	}
}

// goldenStreams are the SDK-contract endpoints that have NO fixture: their
// answers are open-ended streams, and a recorded frame would be a snapshot of
// a race rather than a contract.
func goldenStreams() []goldenFixture {
	return []goldenFixture{{
		ID:     "mailbox_watch",
		Method: http.MethodGet,
		Path:   "/api/v1/mailbox/watch",
		Note:   "an open-ended SSE stream: every SDK's own suite covers its frames",
	}}
}

// goldenEngine is the deterministic engine the recorder drives the daemon
// with: fixed ids, fixed models, one fixed answer.
type goldenEngine struct {
	b *bus.Bus
}

func (g *goldenEngine) NewCorrelation() string { return "corr-golden" }

func (g *goldenEngine) SubjectForRun(corr string) string { return "run." + corr }

func (g *goldenEngine) RunModel(_ context.Context, corr, intent, model string, _ []string, _ bool) (string, error) {
	if g.b != nil {
		_, _ = g.b.Publish(event.Spec{
			Subject: g.SubjectForRun(corr),
			Kind:    event.KindLLMToken,
			Payload: json.RawMessage(`{"text":"golden token"}`),
		})
	}
	return "golden answer", nil
}

func (g *goldenEngine) DefaultModel() string { return "model-a" }

func (g *goldenEngine) ModelIDs() []string { return []string{"model-a", "model-b"} }

func (g *goldenEngine) EventsForCorrelation(string) ([]*event.Event, error) {
	return []*event.Event{{
		ID:      "evt-golden-1",
		Subject: "run.corr-golden",
		Kind:    event.KindLLMToken,
		Payload: json.RawMessage(`{"text":"golden token"}`),
	}}, nil
}

// goldenBoard is the deterministic board the recorder drives the mailbox
// with: fixed ids, fixed timestamps, two seeded messages.
type goldenBoard struct {
	msgs []boardapi.Message
}

// goldenSeed is the board's state before every recorded request: two
// messages, one direct and one broadcast, so the inbox, the replies and the
// topic counts all have something to answer.
func goldenSeed() []boardapi.Message {
	return []boardapi.Message{
		{ID: "m-1", Topic: "dm", From: "agent-a", To: "agent-b", Text: "first message", TSMS: 1700000000000},
		{ID: "m-2", Topic: "status", From: "agent-a", To: boardapi.Everyone, Text: "announcement", TSMS: 1700000000001},
	}
}

func (g *goldenBoard) seed() {
	g.msgs = goldenSeed()
}

func (g *goldenBoard) Post(topic, from, text string, nowMS int64) (boardapi.Message, error) {
	return g.Send(boardapi.Message{Topic: topic, From: from, Text: text}, nowMS)
}

func (g *goldenBoard) Send(m boardapi.Message, _ int64) (boardapi.Message, error) {
	m.ID = fmt.Sprintf("m-%d", len(g.msgs)+1)
	m.TSMS = 1700000000000 + int64(len(g.msgs))
	g.msgs = append(g.msgs, m)
	return m, nil
}

func (g *goldenBoard) Broadcast(from, text string, _ int64) (boardapi.Message, error) {
	return g.Send(boardapi.Message{Topic: "broadcast", From: from, To: boardapi.Everyone, Text: text}, 0)
}

func (g *goldenBoard) HelpRequest(from, to, text string, _ int64) (boardapi.Message, error) {
	if to == "" {
		to = boardapi.Everyone
	}
	return g.Send(boardapi.Message{Topic: "help", From: from, To: to, Text: text, Help: true}, 0)
}

func (g *goldenBoard) OpenHelp(int) []boardapi.Message { return nil }

func (g *goldenBoard) Get(id string) (boardapi.Message, bool) {
	for _, m := range g.msgs {
		if m.ID == id {
			return m, true
		}
	}
	return boardapi.Message{}, false
}

func (g *goldenBoard) Ack(id, by string) (boardapi.Message, bool, error) {
	for i := range g.msgs {
		if g.msgs[i].ID != id {
			continue
		}
		g.msgs[i].AckedBy = append(g.msgs[i].AckedBy, by)
		return g.msgs[i], true, nil
	}
	return boardapi.Message{}, false, nil
}

func (g *goldenBoard) Inbox(to string, limit int, _ bool) []boardapi.Message {
	var out []boardapi.Message
	for i := len(g.msgs) - 1; i >= 0; i-- {
		m := g.msgs[i]
		if m.To != to && m.To != boardapi.Everyone {
			continue
		}
		out = append(out, m)
		if limit > 0 && len(out) == limit {
			break
		}
	}
	return out
}

func (g *goldenBoard) Replies(id string, limit int) []boardapi.Message {
	var out []boardapi.Message
	for _, m := range g.msgs {
		if m.ReplyTo == id {
			out = append(out, m)
			if limit > 0 && len(out) == limit {
				break
			}
		}
	}
	return out
}

func (g *goldenBoard) Read(topic string, limit int) []boardapi.Message {
	var out []boardapi.Message
	for i := len(g.msgs) - 1; i >= 0; i-- {
		m := g.msgs[i]
		if topic != "" && m.Topic != topic {
			continue
		}
		out = append(out, m)
		if limit > 0 && len(out) == limit {
			break
		}
	}
	return out
}

// Topics returns the message count per topic.
func (g *goldenBoard) Topics() map[string]int {
	out := map[string]int{}
	for _, m := range g.msgs {
		out[m.Topic]++
	}
	return out
}

// goldenRecord drives the real handlers and returns the bytes they answered.
func goldenRecord(t *testing.T) (map[string][]byte, []goldenFixture) {
	t.Helper()
	dir := t.TempDir()
	j, err := journal.Open(dir, journal.Options{})
	if err != nil {
		t.Fatalf("journal.Open: %v", err)
	}
	t.Cleanup(func() { j.Close() })
	b := bus.New(j)
	t.Cleanup(func() { b.Close() })

	eng := &goldenEngine{b: b}
	const token = "golden-token"
	s := New(eng, b, token, "9.9.9")

	bodies := map[string][]byte{}
	var fixtures []goldenFixture
	for _, c := range goldenCases() {
		board := &goldenBoard{}
		board.seed()
		s.SetMailbox(board, nil)

		var r *http.Request
		if c.body != "" {
			r = httptest.NewRequest(c.method, c.path, strings.NewReader(c.body))
		} else {
			r = httptest.NewRequest(c.method, c.path, nil)
		}
		r.Header.Set("Authorization", "Bearer "+token)
		rec := httptest.NewRecorder()
		s.Handler().ServeHTTP(rec, r)

		if rec.Code != http.StatusOK && rec.Code != http.StatusCreated {
			t.Fatalf("golden case %q answered %d: %s", c.id, rec.Code, rec.Body.String())
		}
		body := append([]byte(nil), rec.Body.Bytes()...)
		if !json.Valid(body) {
			t.Fatalf("golden case %q answered a body that is not JSON: %s", c.id, body)
		}
		bodies[c.id] = body
		fixtures = append(fixtures, goldenFixture{
			ID:      c.id,
			Method:  c.method,
			Path:    goldenTemplate(c.id),
			Request: c.path,
			Status:  rec.Code,
			File:    c.id + ".json",
		})
	}
	sort.Slice(fixtures, func(i, j int) bool { return fixtures[i].ID < fixtures[j].ID })
	return bodies, fixtures
}

// goldenManifestFixtures returns the manifest's entries in order: the recorded
// fixtures first, then the endpoints that have none.
func goldenManifestFixtures(fixtures []goldenFixture) []goldenFixture {
	out := append([]goldenFixture(nil), fixtures...)
	out = append(out, goldenStreams()...)
	sort.Slice(out, func(i, j int) bool { return out[i].ID < out[j].ID })
	return out
}

// goldenTemplate is the contract's path template for an endpoint id — the
// string the SDKs' generated catalogs hold. The fixture carries it so an SDK
// can check its own constant against the server's own table.
func goldenTemplate(id string) string {
	for _, e := range restapiapi.Endpoints() {
		if e.ID == id {
			return e.Path
		}
	}
	return ""
}

// TestTheGoldenFixturesAreInSync is the gate: the recorded answers and the
// committed fixtures must be the same bytes. A change in the daemon's wire
// shape fails here, and the SDKs' own tests then read the NEW fixture and
// fail too — which is the point of a fixture that the server produced.
func TestTheGoldenFixturesAreInSync(t *testing.T) {
	bodies, fixtures := goldenRecord(t)
	manifest := goldenManifest{
		Note: "Recorded from the daemon's REST handlers by kernel/restapi's golden test. " +
			"Regenerate with AGEZT_UPDATE_GOLDEN=1 go test ./kernel/restapi/ -run TestUpdateTheGoldenFixtures.",
		Fixtures: goldenManifestFixtures(fixtures),
	}
	want, err := json.MarshalIndent(manifest, "", "  ")
	if err != nil {
		t.Fatalf("marshal manifest: %v", err)
	}
	want = append(want, '\n')
	have, err := os.ReadFile(filepath.Join(goldenDir, "manifest.json"))
	if err != nil {
		t.Fatalf("read manifest: %v", err)
	}
	if !bytes.Equal(have, want) {
		t.Errorf("sdk/fixtures/manifest.json is stale — regenerate the golden fixtures")
	}
	for id, body := range bodies {
		name := id + ".json"
		have, err := os.ReadFile(filepath.Join(goldenDir, name))
		if err != nil {
			t.Errorf("read %s: %v", name, err)
			continue
		}
		if !bytes.Equal(have, body) {
			t.Errorf("sdk/fixtures/%s is stale — regenerate the golden fixtures", name)
		}
	}
	haveRequests, err := os.ReadFile(filepath.Join(goldenDir, goldenRequestsFile))
	if err != nil {
		t.Errorf("read %s: %v", goldenRequestsFile, err)
	} else if want := goldenRequestFixture(t); !bytes.Equal(haveRequests, want) {
		t.Errorf("sdk/fixtures/%s is stale — regenerate the golden fixtures", goldenRequestsFile)
	}
}

// TestUpdateTheGoldenFixtures rewrites the fixtures. It only runs when the
// environment asks for it, so the ordinary test run never writes.
func TestUpdateTheGoldenFixtures(t *testing.T) {
	if os.Getenv("AGEZT_UPDATE_GOLDEN") != "1" {
		t.Skip("set AGEZT_UPDATE_GOLDEN=1 to rewrite the golden fixtures")
	}
	bodies, fixtures := goldenRecord(t)
	if err := os.MkdirAll(goldenDir, 0o755); err != nil {
		t.Fatalf("mkdir %s: %v", goldenDir, err)
	}
	manifest := goldenManifest{
		Note: "Recorded from the daemon's REST handlers by kernel/restapi's golden test. " +
			"Regenerate with AGEZT_UPDATE_GOLDEN=1 go test ./kernel/restapi/ -run TestUpdateTheGoldenFixtures.",
		Fixtures: goldenManifestFixtures(fixtures),
	}
	want, err := json.MarshalIndent(manifest, "", "  ")
	if err != nil {
		t.Fatalf("marshal manifest: %v", err)
	}
	want = append(want, '\n')
	if err := os.WriteFile(filepath.Join(goldenDir, "manifest.json"), want, 0o644); err != nil {
		t.Fatalf("write manifest: %v", err)
	}
	for id, body := range bodies {
		name := id + ".json"
		if err := os.WriteFile(filepath.Join(goldenDir, name), body, 0o644); err != nil {
			t.Fatalf("write %s: %v", name, err)
		}
		t.Log("wrote sdk/fixtures/" + name)
	}
	if err := os.WriteFile(filepath.Join(goldenDir, goldenRequestsFile), goldenRequestFixture(t), 0o644); err != nil {
		t.Fatalf("write %s: %v", goldenRequestsFile, err)
	}
	t.Log("wrote sdk/fixtures/" + goldenRequestsFile)
}
