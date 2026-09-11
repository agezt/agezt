// SPDX-License-Identifier: MIT

// Workboard + OKR + taste + seat + update commands.
// Code extracted from protocol_commands.go during the Day-43 god-file split. Public API unchanged.
package controlplane


const (
	CmdWorkboardList      = "workboard_list"
	CmdWorkboardLanes     = "workboard_lanes"
	CmdWorkboardShow      = "workboard_show"
	CmdWorkboardCreate    = "workboard_create"
	CmdWorkboardClaim     = "workboard_claim"
	CmdWorkboardHeartbeat = "workboard_heartbeat"
	CmdWorkboardComment   = "workboard_comment"
	CmdWorkboardBlock     = "workboard_block"
	CmdWorkboardFail      = "workboard_fail"
	CmdWorkboardUnblock   = "workboard_unblock"
	CmdWorkboardComplete  = "workboard_complete"
	CmdWorkboardProve     = "workboard_prove"
	CmdWorkboardSeat      = "workboard_seat"
	CmdWorkboardArchive   = "workboard_archive"
	CmdWorkboardLink      = "workboard_link"
	CmdWorkboardPolicy    = "workboard_policy"
	CmdWorkboardDepend    = "workboard_depend"
	CmdWorkboardReclaim   = "workboard_reclaim"
	CmdWorkboardSweep     = "workboard_sweep"
	CmdWorkboardDispatch  = "workboard_dispatch"
	CmdWorkboardWatch     = "workboard_watch"

	// OKR spine (Phase 2): objectives + key results rolling up proven tasks.
	CmdOKRList      = "okr_list"
	CmdOKRShow      = "okr_show"
	CmdOKRCreate    = "okr_create"
	CmdOKRKeyResult = "okr_keyresult"
	CmdOKRLink      = "okr_link"
	CmdOKRUnlink    = "okr_unlink"
	CmdOKRArchive   = "okr_archive"

	// Taste overlay (Phase 3): curated "what good looks like" exemplars.
	CmdTasteList   = "taste_list"
	CmdTasteCreate = "taste_create"
	CmdTasteDelete = "taste_delete"

	// Execution seats (Phase 4): the seat catalog + custom-seat CRUD.
	CmdSeatList   = "seat_list"
	CmdSeatCreate = "seat_create"
	CmdSeatDelete = "seat_delete"

	// Self-update (M860):
	//
	// CmdUpdateCheck queries the configured source (GitHub or the endpoint in
	// AGEZT_UPDATE_ENDPOINT) and returns whether an update is available.
	// No args. Returns: {current, update: {version,sha256,url,notes}|null, up_to_date}
	CmdUpdateCheck = "update_check"
	// CmdUpdateApply orchestrates drain → atomic swap → restart. Args:
	// version, sha256, url (all required), notes (optional).
	// Returns: {applied: bool, error?: string}.
	// On success the daemon exits; the watchdog spawns the new binary.
	// On failure the daemon stays running — human must investigate.
	CmdUpdateApply = "update_apply"
)
