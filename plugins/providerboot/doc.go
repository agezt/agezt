// SPDX-License-Identifier: MIT

// Package providerboot owns provider bootstrap for the daemon: primary
// selection, alternate registration, the governor's construction, and the
// hot-reload path — Boot and Reload share ONE registration path
// (registerAlternates), retiring the boot-vs-reload drift class (M928/M816
// and the 2026-08 survey's live drifts: middleware dropped on reload, the
// cross-provider down-route eligibility set frozen at boot).
//
// The package is deliberately concrete (imports compat/mock/openairesponses/
// chatgptauth); it lives under plugins/ so the kernel never grows a
// kernel→plugins edge.
package providerboot
