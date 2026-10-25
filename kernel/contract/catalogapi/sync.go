// SPDX-License-Identifier: MIT

package catalogapi

import (
	"context"
	"time"
)

// DefaultSyncURL is the community-maintained source the catalog syncs
// from. Overridable via AGEZT_CATALOG_URL on the daemon side.
//
// The constant lives in the contract layer: the surfaces that start a sync
// (the app layer, the scheduled system task) are above the module and may not
// name it.
const DefaultSyncURL = "https://models.dev/api.json"

// SyncResult summarises one successful Sync for events + reporting.
//
// The row lives in the contract layer, with the port: the surfaces that
// journal a sync's outcome are above the module.
type SyncResult struct {
	URL           string
	Bytes         int
	ProviderCount int
	ModelCount    int
	Duration      time.Duration
	FetchedAt     time.Time
}

// Syncer is the port that fetches a remote catalog.
//
// The port lives in the contract layer because two layers above the module
// drive a sync and neither may name the module's implementation: the app
// layer's `catalog sync` operation and the scheduled system task. The daemon
// builds the syncer over the netguard-screened client and hands the kernel
// the port; a host that was given none holds a nil port and the sync answers
// "the catalog library is not available on this daemon".
//
// The URL and the timeout are the CALLER's to choose — they are parameters,
// not fields, because a port cannot carry mutable state that a caller would
// have to reach through. An empty url means the syncer's default source; a
// timeout of zero or less means the syncer's default cap.
type Syncer interface {
	// Sync fetches the catalog from url, parses and validates it, and
	// returns the raw bytes (for writing to api.json), the parsed catalog,
	// and a summary of the fetch.
	Sync(ctx context.Context, url string, timeout time.Duration) (raw []byte, cat *Catalog, res SyncResult, err error)
}
