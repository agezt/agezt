// SPDX-License-Identifier: MIT

package marketapi

// Source is a remote marketplace the daemon syncs from: its local handle, the
// URL of its index, and the optional key its packs are verified with.
type Source struct {
	Name    string `json:"name"`             // local handle (kebab); also the cache dir name
	URL     string `json:"url"`              // marketplace.json URL
	PubKey  string `json:"pubkey,omitempty"` // optional hex Ed25519 key to verify this source's packs
	AddedMS int64  `json:"added_ms,omitempty"`
}

// SyncResult summarizes one successful sync: what came from where, how much
// arrived, and when.
type SyncResult struct {
	Source    string `json:"source"`
	URL       string `json:"url"`
	Packs     int    `json:"packs"`
	Bytes     int    `json:"bytes"`
	FetchedMS int64  `json:"fetched_ms"`
}
