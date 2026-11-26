// SPDX-License-Identifier: MIT

package remoteapi

// Artifact is one artifact's metadata as a peer serves it. The artifact's
// bytes never travel; only its description does.
//
// It is the same row on both sides of the call: the serving adapter writes
// it, the mirror reads it.
type Artifact struct {
	ID        string `json:"id"`
	Ref       string `json:"ref"`
	Name      string `json:"name,omitempty"`
	Mime      string `json:"mime,omitempty"`
	Kind      string `json:"kind,omitempty"`
	Source    string `json:"source,omitempty"`
	Sender    string `json:"sender,omitempty"`
	Corr      string `json:"corr,omitempty"`
	Size      int64  `json:"size"`
	CreatedMS int64  `json:"created_ms"`
	Caption   string `json:"caption,omitempty"`
}

// ArtifactList is the answer to "what does the artifact index hold?": the
// entries themselves, how many matched, the limit that was applied and
// whether entries were dropped.
//
// It lives here, beside the row it carries, for the same reason the row does:
// the REST surface serves this answer, the mirror reads it, and the row's
// field names must be written once.
//
// The fields are declared in the order the wire speaks them (alphabetically),
// because the field order IS the member order of the served JSON.
type ArtifactList struct {
	Count      int        `json:"count"`
	Entries    []Artifact `json:"entries"`
	Limit      int        `json:"limit"`
	TotalCount int        `json:"total_count"`
	Truncated  bool       `json:"truncated"`
}
