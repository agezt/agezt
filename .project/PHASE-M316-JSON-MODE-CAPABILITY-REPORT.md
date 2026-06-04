# M316 — JSON-mode capability advertising (`agt provider check --caps`)

## Why
SPEC-04 ("providers advertise feature support … JSON mode") and SPEC-15
("capability fallback") call for JSON mode to be a *negotiated* capability, not
assumed. M311–M315 made structured output work and silently fall back on
providers without a native JSON mode; this exposes which providers actually have
one, so an operator can see it before relying on it — the advertising half of the
spec requirement.

## What
- **`kernel/catalog/types.go`**: `FamilySupportsNativeJSONMode(Family) bool` —
  true for the families Agezt sends a native JSON-mode switch for (OpenAI &
  compatibles, Mistral, Azure → `response_format`; Google + Vertex →
  `responseMimeType`; Ollama → `format:json`), false for the prompt-fallback
  families (Anthropic, Cohere, Anthropic-on-Bedrock).
- **`cmd/agt/check.go`**: `agt provider check --caps` now reports `json mode :
  yes/no` alongside vision / attachments / tool-use, and the `--json` output
  carries a `json_mode` field (both the single-provider and `--all` paths).

## Verification
- **`kernel/catalog/catalog_test.go`**: `TestFamilySupportsNativeJSONMode` — the
  seven native-JSON families report true; Anthropic / Cohere / Bedrock / Unknown
  report false.
- **Live demo** (offline, catalog-only): `provider check --caps openai` →
  `json mode : yes`; `provider check --caps anthropic` → `json mode : no`.
- Full suite **1979** passing, `go test ./...` exit 0; `gofmt -l` clean; `go vet`
  clean; `go.mod` / `go.sum` unchanged.

## Scope notes
- Family-level (matches how the encoders are wired). The one case it doesn't
  capture is Anthropic-on-Vertex (claude-* under google-vertex), which has no
  native JSON mode but reports true at family granularity — noted in the helper's
  doc; an informational display, so acceptable.
- This is the advertising half of SPEC-04/15. The remaining half — emitting a
  `degraded` event when a JSON-mode request lands on a non-supporting provider —
  is left as future work; today the fallback is correct (prompt-instructed JSON)
  but unrecorded, and recording it needs the resolved-model capability at the
  governor/loop seam.
- With M311–M316 the structured-output capability is built end to end: encoders,
  internal + external consumers, and capability advertising.
