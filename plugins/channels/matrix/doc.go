// SPDX-License-Identifier: MIT

// Package matrix is an in-process duplex Channel (SPEC-04 §1) over the Matrix
// Client-Server API v3, using net/http only — no external dependency. It
// long-polls GET /sync for inbound room messages and PUTs
// /rooms/{id}/send/m.room.message for outbound, mirroring how the Telegram
// channel long-polls getUpdates. Matrix is an open, federated protocol, so this
// reaches any homeserver (matrix.org or self-hosted) with just an access token.
//
// Security (SPEC-04 §1.7): inbound is an injection surface. Only rooms on the
// allowlist may drive the agent; everyone else is journaled and ignored. The
// bot's OWN messages are skipped (by its MXID) so a reply never re-enters the
// loop. Inbound text is passed to the agent as an intent (data), and the agent's
// tool calls still pass through Edict.
//
// Scope: text (m.text) plus inbound image/voice — m.image/m.audio/m.video are
// downloaded from the media repo (mxc:// → /_matrix/media/v3/download) into a
// data: URL for vision / ambient STT, and outbound attachments are uploaded and
// posted as m.image/m.audio/m.file. Media is fetched only for allowlisted rooms.
package matrix
