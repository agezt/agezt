// SPDX-License-Identifier: MIT

package remote

import (
	"context"
	"encoding/json"
	"fmt"
	"testing"

	appnodes "github.com/agezt/agezt/kernel/app/nodes"
	"github.com/agezt/agezt/kernel/event"
)

// published is one payload the mirror published.
type published struct {
	corr      string
	kind      event.Kind
	suffix    string
	payload   map[string]any
	published int
}

// mirrorPorts builds the ports of a mirror whose peer answers with the given
// events and artifacts.
func mirrorPorts(pub *published, events []Event, fetchErr error, artifacts []Artifact, artifactTruncated bool, artifactErr error) MirrorPorts {
	return MirrorPorts{
		LookupPeer: func(name string) (appnodes.Peer, bool, error) {
			return appnodes.Peer{Name: name, URL: "http://peer.invalid"}, true, nil
		},
		FetchEvents: func(ctx context.Context, peer appnodes.Peer, corr string) ([]Event, error) {
			return events, fetchErr
		},
		FetchArtifacts: func(ctx context.Context, peer appnodes.Peer, corr string) ([]Artifact, bool, error) {
			return artifacts, artifactTruncated, artifactErr
		},
		Redact: func(raw json.RawMessage) (any, bool) {
			if len(raw) == 0 {
				return nil, false
			}
			return map[string]any{"redacted": string(raw)}, true
		},
		Publish: func(ctx context.Context, corr string, kind event.Kind, suffix string, payload map[string]any) error {
			pub.corr = corr
			pub.kind = kind
			pub.suffix = suffix
			pub.payload = payload
			pub.published++
			return nil
		},
	}
}

func TestMode(t *testing.T) {
	cases := map[string]string{
		"1": ModeMetadata, "true": ModeMetadata, "yes": ModeMetadata, "on": ModeMetadata,
		"metadata": ModeMetadata, "METADATA": ModeMetadata, " metadata ": ModeMetadata,
		"redacted": ModeRedacted, "payload": ModeRedacted, "payload-redacted": ModeRedacted,
		"REDACTED": ModeRedacted,
		"":         ModeOff, "off": ModeOff, "no": ModeOff, "maybe": ModeOff, "0": ModeOff,
	}
	for raw, want := range cases {
		if got := Mode(raw); got != want {
			t.Errorf("Mode(%q) = %q, want %q", raw, got, want)
		}
	}
}

func TestEnvMode(t *testing.T) {
	t.Setenv("AGEZT_REMOTE_EVENT_MIRROR", "redacted")
	if got := EnvMode(); got != ModeRedacted {
		t.Errorf("EnvMode() = %q, want redacted", got)
	}
	t.Setenv("AGEZT_REMOTE_EVENT_MIRROR", "")
	if got := EnvMode(); got != ModeOff {
		t.Errorf("EnvMode() = %q, want off", got)
	}
}

func TestPayloadMode(t *testing.T) {
	if got := PayloadMode(ModeRedacted); got != "redacted" {
		t.Errorf("PayloadMode(redacted) = %q", got)
	}
	if got := PayloadMode(ModeMetadata); got != "none" {
		t.Errorf("PayloadMode(metadata) = %q, want none", got)
	}
}

// A mirror that is off publishes nothing at all.
func TestMirrorOffPublishesNothing(t *testing.T) {
	pub := &published{}
	Mirror(context.Background(), mirrorPorts(pub, nil, nil, nil, false, nil), ModeOff, "local", map[string]string{
		"remote_peer": "nodeB", "remote_correlation": "run-abc",
	})
	if pub.published != 0 {
		t.Errorf("published = %d, want none", pub.published)
	}
}

// A mirror that never learned the peer or the correlation says so, and names
// neither.
func TestMirrorWithoutMetadataIsUnavailable(t *testing.T) {
	pub := &published{}
	Mirror(context.Background(), mirrorPorts(pub, nil, nil, nil, false, nil), ModeMetadata, "local", map[string]string{})
	if pub.published != 1 {
		t.Fatalf("published = %d, want one", pub.published)
	}
	if pub.payload["phase"] != PhaseUnavailable {
		t.Errorf("phase = %v, want %q", pub.payload["phase"], PhaseUnavailable)
	}
	if pub.payload["error"] != "remote peer/correlation metadata missing" {
		t.Errorf("error = %v", pub.payload["error"])
	}
	if _, ok := pub.payload["remote_peer"]; ok {
		t.Errorf("payload names a peer it never learned: %v", pub.payload)
	}
	if pub.corr != "local" || pub.kind != event.KindInfo || pub.suffix != "remote" {
		t.Errorf("published = %q/%q/%q", pub.corr, pub.kind, pub.suffix)
	}
}

// A peer that is not configured is named, so the operator sees which peer was
// meant.
func TestMirrorWithAnUnknownPeerIsUnavailable(t *testing.T) {
	pub := &published{}
	ports := mirrorPorts(pub, nil, nil, nil, false, nil)
	ports.LookupPeer = func(name string) (appnodes.Peer, bool, error) {
		return appnodes.Peer{}, false, nil
	}
	Mirror(context.Background(), ports, ModeMetadata, "local", map[string]string{
		"remote_peer": "nodeB", "remote_correlation": "run-abc",
	})
	if pub.published != 1 || pub.payload["phase"] != PhaseUnavailable {
		t.Fatalf("payload = %v", pub.payload)
	}
	if pub.payload["error"] != "peer not found" {
		t.Errorf("error = %v, want peer not found", pub.payload["error"])
	}
	if pub.payload["remote_peer"] != "nodeB" || pub.payload["remote_correlation"] != "run-abc" {
		t.Errorf("payload = %v, want the peer and the correlation named", pub.payload)
	}
}

// A peer lookup that fails says why.
func TestMirrorWithAFailingLookupIsUnavailable(t *testing.T) {
	pub := &published{}
	ports := mirrorPorts(pub, nil, nil, nil, false, nil)
	ports.LookupPeer = func(name string) (appnodes.Peer, bool, error) {
		return appnodes.Peer{}, false, fmt.Errorf("peer list is broken")
	}
	Mirror(context.Background(), ports, ModeMetadata, "local", map[string]string{
		"remote_peer": "nodeB", "remote_correlation": "run-abc",
	})
	if pub.published != 1 || pub.payload["phase"] != PhaseUnavailable {
		t.Fatalf("payload = %v", pub.payload)
	}
	if pub.payload["error"] != "peer list is broken" {
		t.Errorf("error = %v", pub.payload["error"])
	}
}

// A peer whose events cannot be read says why.
func TestMirrorWithAFailingFetchIsUnavailable(t *testing.T) {
	pub := &published{}
	ports := mirrorPorts(pub, nil, fmt.Errorf("401 (token rejected)"), nil, false, nil)
	Mirror(context.Background(), ports, ModeMetadata, "local", map[string]string{
		"remote_peer": "nodeB", "remote_correlation": "run-abc",
	})
	if pub.published != 1 || pub.payload["phase"] != PhaseUnavailable {
		t.Fatalf("payload = %v", pub.payload)
	}
	if pub.payload["error"] != "401 (token rejected)" {
		t.Errorf("error = %v", pub.payload["error"])
	}
	if pub.payload["remote_peer"] != "nodeB" || pub.payload["remote_correlation"] != "run-abc" {
		t.Errorf("payload = %v, want the peer and the correlation named", pub.payload)
	}
}

// The mirrored payload says what it carries.
func TestMirrorPublishesThePeerEvents(t *testing.T) {
	pub := &published{}
	events := []Event{{
		ID: "e1", Seq: 1, TSUnixMS: 1000, Subject: "agent.agent-run-abc.task",
		Actor: "agent-run-abc", Kind: event.KindTaskCompleted,
		CorrelationID: "run-abc", Hash: "h1", Payload: json.RawMessage(`{"answer":"remote done"}`),
	}}
	Mirror(context.Background(), mirrorPorts(pub, events, nil, nil, false, nil), ModeMetadata, "local", map[string]string{
		"remote_peer": "nodeB", "remote_correlation": "run-abc",
	})
	if pub.published != 1 {
		t.Fatalf("published = %d, want one", pub.published)
	}
	p := pub.payload
	if p["profile"] != MirrorProfile || p["phase"] != PhaseMirrored {
		t.Errorf("payload = %v", p)
	}
	if p["mode"] != ModeMetadata || p["payload_mode"] != "none" {
		t.Errorf("payload = %v, want the metadata mode", p)
	}
	if p["remote_peer"] != "nodeB" || p["remote_correlation"] != "run-abc" {
		t.Errorf("payload = %v, want the peer and the correlation", p)
	}
	if p["count"] != 1 || p["truncated"] != false {
		t.Errorf("payload = %v, want one untruncated event", p)
	}
	rows, ok := p["events"].([]map[string]any)
	if !ok || len(rows) != 1 {
		t.Fatalf("events = %v", p["events"])
	}
	row := rows[0]
	if row["id"] != "e1" || row["kind"] != event.KindTaskCompleted || row["hash"] != "h1" {
		t.Errorf("row = %v", row)
	}
	if row["correlation_id"] != "run-abc" || row["seq"] != int64(1) || row["ts_unix_ms"] != int64(1000) {
		t.Errorf("row = %v", row)
	}
}

// The metadata mode never carries the peer's payload.
func TestMirrorInMetadataModeDropsThePayload(t *testing.T) {
	pub := &published{}
	events := []Event{{ID: "e1", Kind: event.KindTaskCompleted, Payload: json.RawMessage(`{"answer":"remote done"}`)}}
	Mirror(context.Background(), mirrorPorts(pub, events, nil, nil, false, nil), ModeMetadata, "local", map[string]string{
		"remote_peer": "nodeB", "remote_correlation": "run-abc",
	})
	rows := pub.payload["events"].([]map[string]any)
	if _, ok := rows[0]["payload_redacted"]; ok {
		t.Errorf("row carries a payload in the metadata mode: %v", rows[0])
	}
}

// The redacted mode carries the redacted payload.
func TestMirrorInRedactedModeCarriesTheRedactedPayload(t *testing.T) {
	pub := &published{}
	events := []Event{{ID: "e1", Kind: event.KindTaskCompleted, Payload: json.RawMessage(`{"answer":"remote done"}`)}}
	Mirror(context.Background(), mirrorPorts(pub, events, nil, nil, false, nil), ModeRedacted, "local", map[string]string{
		"remote_peer": "nodeB", "remote_correlation": "run-abc",
	})
	if pub.payload["payload_mode"] != "redacted" {
		t.Errorf("payload = %v, want the redacted payload mode", pub.payload)
	}
	rows := pub.payload["events"].([]map[string]any)
	if _, ok := rows[0]["payload_redacted"]; !ok {
		t.Errorf("row carries no redacted payload: %v", rows[0])
	}
}

// A payload the redactor cannot handle does not travel.
func TestMirrorDropsAPayloadTheRedactorCannotHandle(t *testing.T) {
	pub := &published{}
	ports := mirrorPorts(pub, []Event{{ID: "e1", Payload: json.RawMessage(`{}`)}}, nil, nil, false, nil)
	ports.Redact = func(raw json.RawMessage) (any, bool) { return nil, false }
	Mirror(context.Background(), ports, ModeRedacted, "local", map[string]string{
		"remote_peer": "nodeB", "remote_correlation": "run-abc",
	})
	rows := pub.payload["events"].([]map[string]any)
	if _, ok := rows[0]["payload_redacted"]; ok {
		t.Errorf("row carries an unredactable payload: %v", rows[0])
	}
}

// The peer's artifacts travel with the events.
func TestMirrorPublishesThePeerArtifacts(t *testing.T) {
	pub := &published{}
	artifacts := []Artifact{{ID: "art-1", Ref: "abc", Name: "result.txt", Mime: "text/plain",
		Kind: "tool-output", Source: "run", Sender: "agent-run-abc", Corr: "run-abc",
		Size: 12, CreatedMS: 1002, Caption: "the result"}}
	Mirror(context.Background(), mirrorPorts(pub, nil, nil, artifacts, false, nil), ModeMetadata, "local", map[string]string{
		"remote_peer": "nodeB", "remote_correlation": "run-abc",
	})
	if pub.payload["artifact_count"] != 1 || pub.payload["artifacts_truncated"] != false {
		t.Errorf("payload = %v", pub.payload)
	}
	rows, ok := pub.payload["artifacts"].([]map[string]any)
	if !ok || len(rows) != 1 {
		t.Fatalf("artifacts = %v", pub.payload["artifacts"])
	}
	if rows[0]["id"] != "art-1" || rows[0]["name"] != "result.txt" || rows[0]["caption"] != "the result" {
		t.Errorf("row = %v", rows[0])
	}
	if rows[0]["size"] != int64(12) || rows[0]["created_ms"] != int64(1002) {
		t.Errorf("row = %v", rows[0])
	}
}

// Artifacts that cannot be read do not sink the mirror: the events still
// travel, and the failure is said.
func TestMirrorWithFailingArtifactsStillMirrorsTheEvents(t *testing.T) {
	pub := &published{}
	events := []Event{{ID: "e1", Kind: event.KindTaskCompleted}}
	Mirror(context.Background(), mirrorPorts(pub, events, nil, nil, false, fmt.Errorf("status 500")), ModeMetadata, "local", map[string]string{
		"remote_peer": "nodeB", "remote_correlation": "run-abc",
	})
	if pub.payload["phase"] != PhaseMirrored {
		t.Errorf("payload = %v, want the mirrored phase", pub.payload)
	}
	if pub.payload["artifacts_unavailable"] != "status 500" {
		t.Errorf("payload = %v, want the artifact failure named", pub.payload)
	}
	if _, ok := pub.payload["artifacts"]; ok {
		t.Errorf("payload carries artifacts it could not read: %v", pub.payload)
	}
}

// The mirror carries at most MaxMirrorEvents events, and says that it cut.
func TestEventRowsAreBounded(t *testing.T) {
	events := make([]Event, 0, MaxMirrorEvents+5)
	for i := 0; i < MaxMirrorEvents+5; i++ {
		events = append(events, Event{ID: fmt.Sprintf("e%d", i)})
	}
	rows, truncated := EventRows(events, ModeMetadata, nil)
	if len(rows) != MaxMirrorEvents {
		t.Errorf("rows = %d, want %d", len(rows), MaxMirrorEvents)
	}
	if !truncated {
		t.Errorf("truncated = false, want true")
	}
	if rows[0]["id"] != "e0" || rows[MaxMirrorEvents-1]["id"] != fmt.Sprintf("e%d", MaxMirrorEvents-1) {
		t.Errorf("rows = %v, want the first %d events", rows[:1], MaxMirrorEvents)
	}
}

// A peer list that fits is not truncated.
func TestEventRowsAreNotTruncatedWhenTheyFit(t *testing.T) {
	rows, truncated := EventRows([]Event{{ID: "e1"}}, ModeMetadata, nil)
	if truncated {
		t.Errorf("truncated = true, want false")
	}
	if len(rows) != 1 {
		t.Errorf("rows = %d, want one", len(rows))
	}
}

// The artifact list is bounded too, and the peer's own truncation counts.
func TestArtifactRowsAreBounded(t *testing.T) {
	entries := make([]Artifact, 0, MaxMirrorArtifacts+3)
	for i := 0; i < MaxMirrorArtifacts+3; i++ {
		entries = append(entries, Artifact{ID: fmt.Sprintf("a%d", i)})
	}
	rows, truncated := ArtifactRows(entries, false)
	if len(rows) != MaxMirrorArtifacts || !truncated {
		t.Errorf("rows = %d, truncated = %v", len(rows), truncated)
	}
	rows, truncated = ArtifactRows([]Artifact{{ID: "a1"}}, true)
	if len(rows) != 1 || !truncated {
		t.Errorf("rows = %d, truncated = %v, want the peer's truncation kept", len(rows), truncated)
	}
}

// The caption is said only when the peer said it.
func TestArtifactRowsOmitAnEmptyCaption(t *testing.T) {
	rows, _ := ArtifactRows([]Artifact{{ID: "a1"}}, false)
	if _, ok := rows[0]["caption"]; ok {
		t.Errorf("row carries an empty caption: %v", rows[0])
	}
}
