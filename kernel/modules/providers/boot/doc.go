// SPDX-License-Identifier: MIT

// Package boot owns provider bootstrap for the daemon: primary selection,
// alternate registration, the governor's construction, and the hot-reload path
// — Boot and Reload share ONE registration path (registerAlternates), retiring
// the boot-vs-reload drift class (M928/M816 and the 2026-08 survey's live
// drifts: middleware dropped on reload, the cross-provider down-route
// eligibility set frozen at boot).
//
// It is the providers module's engine, so it does NOT import the wire
// adapters it drives. The compat family, the offline mock and the ChatGPT
// Responses backend live above the module in the plugin layer, and the
// middleware stack lives in the runs module; all of them arrive through
// api.Ports, which the composition root (cmd/agezt) fills in. The module keeps
// the decisions — which provider runs, which alternates are eligible, what the
// catalog says about a model — and asks the adapters for the wire.
package boot
