// SPDX-License-Identifier: MIT

package restapi

// Artifact REST surface. Listing is metadata-only; byte transfer is a separate
// endpoint that fails closed unless AGEZT_REMOTE_ARTIFACT_BYTES explicitly
// allows it on this daemon.

import (
	"encoding/base64"
	"errors"
	"net/http"
	"os"
	"strconv"
	"strings"

	"github.com/agezt/agezt/internal/brand"
)

const (
	artifactDefaultLimit   = 200
	artifactMaxLimit       = 1000
	MaxRemoteArtifactBytes = 64 << 20
)

var (
	ErrArtifactNotFound = errors.New("artifact not found")
	ErrArtifactTooLarge = errors.New("artifact too large")
)

// ArtifactEntry is the REST-safe metadata shape for one artifact index entry.
// It mirrors kernel/artifact.Entry but keeps this package decoupled from the
// artifact store implementation and never includes bytes.
type ArtifactEntry struct {
	ID        string `json:"id"`
	Ref       string `json:"ref"`
	Name      string `json:"name,omitempty"`
	Mime      string `json:"mime,omitempty"`
	Kind      string `json:"kind,omitempty"`
	Source    string `json:"source,omitempty"`
	Sender    string `json:"sender,omitempty"`
	Corr      string `json:"corr,omitempty"`
	Size      int64  `json:"size"`
	CreatedMs int64  `json:"created_ms"`
	Caption   string `json:"caption,omitempty"`
}

// ArtifactLister is an optional Engine extension. Older or minimal engines do
// not have to implement it; the route then reports artifact metadata unavailable.
type ArtifactLister interface {
	ArtifactEntries(kind, source, corr string) ([]ArtifactEntry, error)
}

// ArtifactReader is an optional Engine extension for policy-gated artifact byte
// transfer. The maxBytes argument lets the daemon adapter reject oversized
// entries before reading blobs into memory.
type ArtifactReader interface {
	ArtifactBytes(id string, maxBytes int64) ([]byte, ArtifactEntry, error)
}

// --- GET /api/v1/artifacts?kind=&source=&corr=&limit= ---

func (s *Server) handleArtifacts(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		methodNotAllowed(w, http.MethodGet)
		return
	}
	eng, _, err := s.bind(r)
	if err != nil {
		writeErr(w, http.StatusBadRequest, "invalid_tenant", err.Error())
		return
	}
	lister, ok := eng.(ArtifactLister)
	if !ok {
		writeErr(w, http.StatusServiceUnavailable, "artifact_unavailable",
			"artifact metadata is not available on this daemon")
		return
	}

	q := r.URL.Query()
	limit := artifactLimit(r)
	entries, err := lister.ArtifactEntries(q.Get("kind"), q.Get("source"), q.Get("corr"))
	if err != nil {
		writeErr(w, http.StatusInternalServerError, "artifact_lookup_error", err.Error())
		return
	}
	total := len(entries)
	truncated := total > limit
	if truncated {
		entries = entries[:limit]
	}
	writeJSON(w, http.StatusOK, map[string]any{
		"count":       len(entries),
		"total_count": total,
		"limit":       limit,
		"truncated":   truncated,
		"entries":     entries,
	})
}

// --- GET /api/v1/artifacts/{id}/bytes ---

func (s *Server) handleArtifactBytes(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		methodNotAllowed(w, http.MethodGet)
		return
	}
	if !remoteArtifactBytesAllowed() {
		writeErr(w, http.StatusForbidden, "artifact_bytes_disabled",
			"remote artifact byte transfer is disabled; set AGEZT_REMOTE_ARTIFACT_BYTES=allow on the peer to enable it")
		return
	}
	id, ok := artifactBytesID(r.URL.Path)
	if !ok {
		writeErr(w, http.StatusNotFound, "not_found", "use /api/v1/artifacts/{id}/bytes")
		return
	}
	eng, _, err := s.bind(r)
	if err != nil {
		writeErr(w, http.StatusBadRequest, "invalid_tenant", err.Error())
		return
	}
	reader, ok := eng.(ArtifactReader)
	if !ok {
		writeErr(w, http.StatusServiceUnavailable, "artifact_unavailable",
			"artifact bytes are not available on this daemon")
		return
	}
	data, entry, err := reader.ArtifactBytes(id, MaxRemoteArtifactBytes)
	if err != nil {
		switch {
		case errors.Is(err, ErrArtifactNotFound):
			writeErr(w, http.StatusNotFound, "not_found", "artifact not found: "+id)
		case errors.Is(err, ErrArtifactTooLarge):
			writeErr(w, http.StatusRequestEntityTooLarge, "artifact_too_large",
				"artifact exceeds the remote transfer limit")
		default:
			writeErr(w, http.StatusInternalServerError, "artifact_read_error", err.Error())
		}
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{
		"entry": entry,
		"size":  len(data),
		"data":  base64.StdEncoding.EncodeToString(data),
	})
}

func artifactLimit(r *http.Request) int {
	limit := artifactDefaultLimit
	if raw := r.URL.Query().Get("limit"); raw != "" {
		if v, err := strconv.Atoi(raw); err == nil && v > 0 {
			limit = v
		}
	}
	if limit > artifactMaxLimit {
		limit = artifactMaxLimit
	}
	return limit
}

func artifactBytesID(path string) (string, bool) {
	rest := strings.Trim(strings.TrimPrefix(path, "/api/v1/artifacts/"), "/")
	parts := strings.Split(rest, "/")
	if len(parts) != 2 || parts[0] == "" || parts[1] != "bytes" {
		return "", false
	}
	return parts[0], true
}

func remoteArtifactBytesAllowed() bool {
	switch strings.ToLower(strings.TrimSpace(os.Getenv(brand.EnvPrefix + "REMOTE_ARTIFACT_BYTES"))) {
	case "allow", "1", "true", "yes", "on":
		return true
	default:
		return false
	}
}
