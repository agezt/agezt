// SPDX-License-Identifier: MIT

//	GET  /api/v1/update            — check for updates (M860)
//	POST /api/v1/update/apply      — validate and stage an update (M860)
//
// Security (SPEC-06): loopback-bound by the operator, token-authed on every
// request (Authorization: Bearer <token>). Empty token fails closed.
package restapi
