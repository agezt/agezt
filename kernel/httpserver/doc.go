// SPDX-License-Identifier: MIT

// Package httpserver owns the transport-level HTTP mechanisms shared by
// AGEZT's independently-versioned API surfaces: credential extraction,
// authorization middleware, request-body limits, and route metadata.
//
// It deliberately does not own product routes, response envelopes, tenant
// lifecycle, or OAuth. Those stay in their domain/surface packages.
package httpserver
