// SPDX-License-Identifier: MIT

package opapi

// Provenance: the Workboard, OKR, taste, seat and update half of the control plane's command vocabulary,
// moved here from kernel/controlplane/protocol_commands_commands_misc.go when the
// operation vocabulary got one home (W3.79). The values are unchanged; the
// identifiers lost the `Cmd` prefix that named the transport's own alias.
//
// Every name below is declared exactly once, in this package. The application
// layer binds these constants into the operation's spec, and the transports
// refer to them by these names. A name that can be written in two places is a
// name that can drift, and a name that drifts is a route the daemon no longer
// answers to.

const (
	WorkboardList      = "workboard_list"
	WorkboardLanes     = "workboard_lanes"
	WorkboardShow      = "workboard_show"
	WorkboardCreate    = "workboard_create"
	WorkboardClaim     = "workboard_claim"
	WorkboardHeartbeat = "workboard_heartbeat"
	WorkboardComment   = "workboard_comment"
	WorkboardBlock     = "workboard_block"
	WorkboardFail      = "workboard_fail"
	WorkboardUnblock   = "workboard_unblock"
	WorkboardComplete  = "workboard_complete"
	WorkboardProve     = "workboard_prove"
	WorkboardSeat      = "workboard_seat"
	WorkboardArchive   = "workboard_archive"
	WorkboardLink      = "workboard_link"
	WorkboardPolicy    = "workboard_policy"
	WorkboardDepend    = "workboard_depend"
	WorkboardReclaim   = "workboard_reclaim"
	WorkboardSweep     = "workboard_sweep"
	WorkboardDispatch  = "workboard_dispatch"
	WorkboardWatch     = "workboard_watch"
	// OKR spine (Phase 2): objectives + key results rolling up proven tasks.
	OKRList      = "okr_list"
	OKRShow      = "okr_show"
	OKRCreate    = "okr_create"
	OKRKeyResult = "okr_keyresult"
	OKRLink      = "okr_link"
	OKRUnlink    = "okr_unlink"
	OKRArchive   = "okr_archive"
	// Taste overlay (Phase 3): curated "what good looks like" exemplars.
	TasteList   = "taste_list"
	TasteCreate = "taste_create"
	TasteDelete = "taste_delete"
	// Execution seats (Phase 4): the seat catalog + custom-seat CRUD.
	SeatList   = "seat_list"
	SeatCreate = "seat_create"
	SeatDelete = "seat_delete"
	// Self-update (M860):
	//
	// UpdateCheck queries the configured source (GitHub or the endpoint in
	// AGEZT_UPDATE_ENDPOINT) and returns whether an update is available.
	// No args. Returns: {current, update: {version,sha256,url,notes}|null, up_to_date}
	UpdateCheck = "update_check"
	// UpdateApply orchestrates drain → atomic swap → restart. Args:
	// version, sha256, url (all required), notes (optional).
	// Returns: {applied: bool, error?: string}.
	// On success the daemon exits; the watchdog spawns the new binary.
	// On failure the daemon stays running — human must investigate.
	UpdateApply = "update_apply"
)
