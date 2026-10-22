// SPDX-License-Identifier: MIT

// Package remote owns the remote run mirror: what a delegated run's peer
// events and artifacts look like when they are mirrored back into the local
// journal.
//
// The application owns the decisions — whether the mirror runs at all, what a
// missing peer or a failing peer looks like, which fields of a peer event
// travel, and whether a payload is redacted before it travels. The host owns
// the data access: the peer configuration, the two peer HTTP fetches, the
// redactor and the journal publish.
package remote

import (
	"context"
	"encoding/json"
	"os"
	"strings"

	"github.com/agezt/agezt/internal/brand"
	appnodes "github.com/agezt/agezt/kernel/app/nodes"
	"github.com/agezt/agezt/kernel/contract/remoteapi"
	"github.com/agezt/agezt/kernel/event"
)

const (
	// ModeOff means the mirror does not run.
	ModeOff = ""
	// ModeMetadata mirrors the peer's event metadata, never its payloads.
	ModeMetadata = "metadata"
	// ModeRedacted mirrors the peer's event metadata and its redacted payloads.
	ModeRedacted = "redacted"
)

const (
	// MaxMirrorEvents bounds how many peer events one mirror carries.
	MaxMirrorEvents = 200
	// MaxMirrorArtifacts bounds how many peer artifacts one mirror carries.
	MaxMirrorArtifacts = 200
)

// MirrorProfile is the execution profile the mirrored events belong to.
const MirrorProfile = "remote-agezt"

// Mirror phases, as they appear in the mirrored payload.
const (
	// PhaseUnavailable is the phase of a mirror that could not reach the
	// peer's events.
	PhaseUnavailable = "peer_events_unavailable"
	// PhaseMirrored is the phase of a mirror that carried the peer's events.
	PhaseMirrored = "peer_events_mirrored"
)

// Mode reads the mirror's mode out of its configured value. The empty mode
// turns the mirror off; "metadata" and "redacted" switch it on.
func Mode(raw string) string {
	switch strings.ToLower(strings.TrimSpace(raw)) {
	case "1", "true", "yes", "on", "metadata":
		return ModeMetadata
	case "redacted", "payload", "payload-redacted":
		return ModeRedacted
	default:
		return ModeOff
	}
}

// EnvMode reads the mirror's mode from the environment. The mirror is off
// unless the environment asks for it.
func EnvMode() string {
	return Mode(os.Getenv(brand.EnvPrefix + "REMOTE_EVENT_MIRROR"))
}

// PayloadMode names what the mirror did with the peer's payloads: "redacted"
// when the payloads travelled redacted, "none" when they did not travel.
func PayloadMode(mode string) string {
	if mode == ModeRedacted {
		return ModeRedacted
	}
	return "none"
}

// Event is one peer event, as the peer reported it. The payload is the raw
// bytes the peer sent; what happens to them is the mirror's decision.
//
// It is the canonical event (kernel/event) — the very row the peer serves —
// so the host decodes the peer's answer into it without a projection.
type Event = event.Event

// Artifact is one peer artifact's metadata. The artifact's bytes never
// travel; only its description does.
//
// It is the contract row (kernel/contract/remoteapi) — the very row the peer
// serves — so the host decodes the peer's answer into it without a
// projection.
type Artifact = remoteapi.Artifact

// Redactor turns a raw payload into its redacted form. It reports false
// when the payload cannot be redacted, and then the payload does not travel.
type Redactor func(raw json.RawMessage) (any, bool)

// MirrorPorts are the host's capabilities the mirror needs.
type MirrorPorts struct {
	// LookupPeer resolves a peer name to its configuration.
	LookupPeer func(name string) (appnodes.Peer, bool, error)
	// FetchEvents reads the peer's events for a correlation.
	FetchEvents func(ctx context.Context, peer appnodes.Peer, corr string) ([]Event, error)
	// FetchArtifacts reads the peer's artifact metadata for a correlation.
	// It reports whether the peer itself truncated the list.
	FetchArtifacts func(ctx context.Context, peer appnodes.Peer, corr string) ([]Artifact, bool, error)
	// Redact turns a raw payload into its redacted form.
	Redact Redactor
	// Publish mirrors the payload into the local journal.
	Publish func(ctx context.Context, corr string, kind event.Kind, suffix string, payload map[string]any) error
}

// Mirror mirrors a delegated run's peer events into the local journal.
//
// The mode decides whether anything happens at all. A mirror that cannot
// reach the peer's events is still mirrored — as an "unavailable" event that
// says why — so the operator sees the delegation and its failure instead of
// silence.
func Mirror(ctx context.Context, ports MirrorPorts, rawMode string, corr string, meta map[string]string) {
	mode := Mode(rawMode)
	if mode == ModeOff {
		return
	}
	peerName := strings.TrimSpace(meta["remote_peer"])
	remoteCorr := strings.TrimSpace(meta["remote_correlation"])
	if peerName == "" || remoteCorr == "" {
		// The mirror never learned the peer or the correlation, so it has
		// nothing to name them by.
		publishUnavailable(ctx, ports, corr, mode, nil, "remote peer/correlation metadata missing")
		return
	}
	peer, ok, err := ports.LookupPeer(peerName)
	if err != nil || !ok {
		msg := "peer not found"
		if err != nil {
			msg = err.Error()
		}
		publishUnavailable(ctx, ports, corr, mode, map[string]string{
			"remote_peer":        peerName,
			"remote_correlation": remoteCorr,
		}, msg)
		return
	}
	events, err := ports.FetchEvents(ctx, peer, remoteCorr)
	if err != nil {
		publishUnavailable(ctx, ports, corr, mode, map[string]string{
			"remote_peer":        peerName,
			"remote_correlation": remoteCorr,
		}, err.Error())
		return
	}
	rows, truncated := EventRows(events, mode, ports.Redact)
	payload := map[string]any{
		"profile":            MirrorProfile,
		"phase":              PhaseMirrored,
		"mode":               mode,
		"payload_mode":       PayloadMode(mode),
		"remote_peer":        peerName,
		"remote_correlation": remoteCorr,
		"count":              len(rows),
		"truncated":          truncated,
		"events":             rows,
	}
	if artifacts, artifactTruncated, artifactErr := ports.FetchArtifacts(ctx, peer, remoteCorr); artifactErr == nil {
		artifactRows, truncated := ArtifactRows(artifacts, artifactTruncated)
		payload["artifact_count"] = len(artifactRows)
		payload["artifacts_truncated"] = truncated
		payload["artifacts"] = artifactRows
	} else {
		payload["artifacts_unavailable"] = artifactErr.Error()
	}
	_ = ports.Publish(ctx, corr, event.KindInfo, "remote", payload)
}

// publishUnavailable mirrors the failure of a mirror that could not reach
// the peer's events. The peer and correlation are named only when the
// mirror knows them.
func publishUnavailable(ctx context.Context, ports MirrorPorts, corr, mode string, named map[string]string, msg string) {
	payload := map[string]any{
		"profile": MirrorProfile,
		"phase":   PhaseUnavailable,
		"mode":    mode,
		"error":   msg,
	}
	for k, v := range named {
		payload[k] = v
	}
	_ = ports.Publish(ctx, corr, event.KindInfo, "remote", payload)
}

// EventRows maps the peer's events into the rows that travel, and reports
// whether the peer's list was longer than the mirror carries.
//
// The payload travels only in the redacted mode, and only when the redactor
// could redact it.
func EventRows(events []Event, mode string, redact Redactor) ([]map[string]any, bool) {
	truncated := len(events) > MaxMirrorEvents
	limit := len(events)
	if limit > MaxMirrorEvents {
		limit = MaxMirrorEvents
	}
	out := make([]map[string]any, 0, limit)
	for _, ev := range events[:limit] {
		row := map[string]any{
			"id":             ev.ID,
			"seq":            ev.Seq,
			"ts_unix_ms":     ev.TSUnixMS,
			"subject":        ev.Subject,
			"actor":          ev.Actor,
			"kind":           ev.Kind,
			"correlation_id": ev.CorrelationID,
			"hash":           ev.Hash,
		}
		if mode == ModeRedacted && redact != nil {
			if payload, ok := redact(ev.Payload); ok {
				row["payload_redacted"] = payload
			}
		}
		out = append(out, row)
	}
	return out, truncated
}

// ArtifactRows maps the peer's artifact metadata into the rows that travel,
// and reports whether the list was truncated — by the peer or by the mirror.
func ArtifactRows(entries []Artifact, peerTruncated bool) ([]map[string]any, bool) {
	truncated := peerTruncated || len(entries) > MaxMirrorArtifacts
	limit := len(entries)
	if limit > MaxMirrorArtifacts {
		limit = MaxMirrorArtifacts
	}
	out := make([]map[string]any, 0, limit)
	for _, a := range entries[:limit] {
		row := map[string]any{
			"id":         a.ID,
			"ref":        a.Ref,
			"name":       a.Name,
			"mime":       a.Mime,
			"kind":       a.Kind,
			"source":     a.Source,
			"sender":     a.Sender,
			"corr":       a.Corr,
			"size":       a.Size,
			"created_ms": a.CreatedMS,
		}
		// The caption is optional: it is only said when the peer said it.
		if a.Caption != "" {
			row["caption"] = a.Caption
		}
		out = append(out, row)
	}
	return out, truncated
}
