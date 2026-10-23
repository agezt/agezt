// SPDX-License-Identifier: MIT

// Package catalog is the live provider/model registry. It replaces the
// hardcoded `modelPriceTable` in kernel/governor/pricing.go and the
// per-provider Go packages in plugins/providers/* with a single
// data-driven source of truth (TASKS P1-CONDUIT-04, SPEC-15 §1).
//
// Schema mirrors models.dev/api.json (the community catalog the
// project syncs from): a flat map of provider-id → Provider, where
// each Provider carries its base URL, credential env-var name, npm/SDK
// hint (which the Governor uses to pick the right wire dialect), and
// a map of model-id → Model with prices in USD per-million-tokens.
//
// On disk under <BaseDir>/catalog/:
//
//	api.json        the most-recent remote-synced catalog
//	local.json      provider entries auto-discovered from running
//	                services (Ollama /api/tags, lm-studio, etc.)
//	custom.json     operator-curated overrides; wins over both above
//
// Read precedence: custom > local > api. Writes only ever touch one
// file; the loader merges. This means `agt catalog sync` can refresh
// `api.json` without clobbering local discoveries or hand-edits.
package catalog
