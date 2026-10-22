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
