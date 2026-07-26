// SPDX-License-Identifier: MIT

// Package httpserver owns the transport-level HTTP mechanisms shared by
// AGEZT's independently-versioned API surfaces: credential extraction,
// authorization middleware, request-body limits, route metadata, and the
// lifecycle of hardened streaming-safe servers on caller-owned listeners.
//
// It deliberately does not own product routes, response envelopes, tenant
// lifecycle, or OAuth. Those stay in their domain/surface packages.
package httpserver
