# Refactor C2 — `frontend/src/lib/` keep-vs-colocate classification

> Companion to `docs/REFACTORING-SCAN.md` finding **C2**.
> **Generated:** 2026-07-03. Importer counts are **measured** (scan of every `frontend/src/**`
> non-test file for `/lib/<module>` imports), not estimated.

## Method

For each of the 68 non-test `lib/*.ts` modules, count distinct importing files split by consumer
type (views / components / lib / root). Rule:
- **KEEP** — 3+ distinct consumers OR genuine cross-cutting infra.
- **COLOCATE** — 1–2 consumers all under one view/component → move to `views/<X>/lib/` or the component folder.
- **MERGE** — small module that's really part of a larger shared one.

## KEEP in lib/ (infra; importers >= 3 or cross-cutting)

| Module | Importers | | Module | Importers |
|---|---|---|---|---|
| utils | 115 | | fleet | 5 |
| api | 95 | | nav | 4 |
| format | 18 | | theme | 4 |
| chat | 10 | | advanced | 4 |
| export | 10 | | incidents | 4 |
| usePanel | 9 | | accent | 3 |
| agentdetail | 7 | | brand | 3 |
| incidentevents | 7 | | configbackup | 3 |
| incidentnav | 7 | | queue | 3 |
| models | 7 | | liveruncontext | 3 |
| agentnav | 6 | | | |
| autonomy | 6 | | | |
| rundetail | 6 | | | |
| runfocus | 6 | | | |
| alerts | 5 | | | |
| chains | 5 | | | |
| conversations | 5 | | | |

**cursorPager exception:** 1 importer today (Runs) but deliberately built to generalize to all
list views (scan C3). **KEEP** — infra awaiting consumers, not a view helper.

## COLOCATE to a view (`views/<X>/lib/`)

Single-view importers (mapping verified by grep where ambiguous):

| Module | Target |
|---|---|
| acp | views/ACPAgents/lib/acp.ts |
| catalog | views/Catalog/lib/catalog.ts |
| channelSessions | views/Channels/lib/channelSessions.ts |
| insights (✓ Insights.tsx) | views/Insights/lib/insights.ts |
| market | views/Market/lib/market.ts |
| providerPresets | views/Providers/lib/providerPresets.ts |
| routingSuggest (✓ Routing.tsx) | views/Routing/lib/routingSuggest.ts |
| snapshot (✓ Backup.tsx) | views/Backup/lib/snapshot.ts |
| telemetry (✓ Mission.tsx) | views/Mission/lib/telemetry.ts |

## COLOCATE to a component (component folder)

Single-component importers: `agentactivity`, `agentrepair`, `help`, `notify`, `files` (2 comp),
`language`+`monaco` → `components/MonacoView/`.

## MERGE (fold into a sibling)

| Module(s) | Merge into | Why |
|---|---|---|
| voice + tts + voiceCatalog + voiceSession + sentenceChunker | `lib/voice/index.ts` | 6 modules for ONE voice subsystem — biggest structural win |
| languages | language.ts | ext↔lang mapping is one concern |
| conductorStore | conductor.ts | store+logic split for one feature |
| councilStore | council.ts | same |

## 2-importer middle tier (judgment)

Rule: 2 importers crossing view↔component → **KEEP** (shared contract); both inside one view → colocate.
KEEP: `agent`, `agentlive`, `attach`, `commands`, `conductor`, `council`, `delegation`, `eventmeta`,
`markdown` (parser=infra), `replay`, `activity`, `appearance`, `setup`, `speech` (or merge into voice).

## Net effect

- ~42 KEEP (28 infra + ~14 middle-tier), ~18 COLOCATE (12 view, 6 component), ~6 MERGE (10 files → ~4).
- `lib/` shrinks 68 → ~42; survivors are all genuinely shared.

## Phases (gate: `tsc --noEmit && vitest run && npm run build`)

- **P0** knip/eslint rule: a `lib/` module imported by exactly one view is a warning (prevents regrowth).
- **P1 MERGE voice** — fold the 6 voice modules into `lib/voice/`; update ~6 sites. Biggest reduction.
- **P2 MERGE store pairs** — conductorStore→conductor, councilStore→council, languages→language.
- **P3 COLOCATE view-scoped** — 12 modules, one commit per view (module + `.test.ts` → `views/<X>/lib/`).
- **P4 COLOCATE component-scoped** — 6 modules into their component folder.
- **P5** knip/deadcode verify; update barrels.

**Interaction with C4:** the voice merge (P1) touches Chat's `useVoice` concern — do C2-P1 and C4-P5
sequentially, not concurrently.

## Status

- **P1 MERGE voice — ✅ DONE** (branch `refactor/c2-voice-merge`, commit `48451042`, off `main`).
  Folded **5** modules into `lib/voice/` behind an `index.ts` barrel: `voice.ts→voice/transcribe.ts`,
  `tts.ts→voice/tts.ts`, `voiceCatalog.ts→voice/catalog.ts`, `voiceSession.ts→voice/session.ts`,
  `sentenceChunker.ts→voice/sentenceChunker.ts` (+ their tests). Internal cross-imports became
  relative; `@/lib/chat`, `@/lib/api`, `@/lib/speech` stay absolute. Consumers (`MicButton`,
  `VoiceSetup`, `Voice.tsx`) import from the `@/lib/voice` barrel; `Voice.test.tsx`'s mock was
  narrowed to `@/lib/voice/session`. tsc clean, vitest 1453/1453, `vite build` regenerated
  `kernel/webui/dist` (committed). **Frontend-only** → CI-verifiable on the `setup-node` jobs
  independent of the flaky Go WSL runners.

  **Scope note:** the merge table lists 5 modules, not 6 — `speech.ts` is deliberately left in `lib/`.
  It is a lower-level browser `SpeechSynthesis` primitive imported by both `tts` and `views/Chat`, so
  merging it would (a) pull a non-voice-pipeline primitive into the folder and (b) entangle with the
  C4 `Chat.tsx` surface. Keeping it out honors the C2↔C4 sequencing constraint.

- **P0, P2–P5** — not started.
