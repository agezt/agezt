# PHASE M858 — Skill hygiene (idle-skill cleanup)

**Status:** shipped
**Milestone:** M858
**Theme:** Surface and clean up idle skills — the pair to the M857 memory prune.
Owner ask: *"… skilleri de temizleyen…"* (#37 — track skill usage, clean up the
unused ones so the retrieval pool stays sharp).

## What shipped

Skills already track `Metrics.Uses` + `LastUsedMS` (incremented whenever a skill
is retrieved into a run). M858 turns that into a cleanup surface:

- **`Forge.Hygiene(idleCutoffMs)`** → `{total, active, idle[]}` — flags ACTIVE
  skills that are **never used**, or **not used since the cutoff**, while giving
  brand-new skills a grace period (a skill created after the cutoff is never
  flagged, so a freshly promoted one gets a fair chance). Idle skills are sorted
  oldest-seen first (deadest weight on top). Non-active skills are ignored.
- **`CmdSkillHygiene {idle_days}`** + read-only `/api/skills/hygiene` — the report
  (default 30-day idle threshold, floored for safety like the memory prune).
- **Web UI:** a collapsible "N idle skills" strip at the top of the Skills view;
  each idle skill shows its use count and a **retire** action that quarantines it
  (pulls it from the retrieval pool — reversible: promote again to restore). The
  cleanup reuses the existing `/api/skill/quarantine`, so no new write path.

## Surface

- `kernel/skill/forge.go` — `HygieneReport`, `Hygiene`.
- `kernel/controlplane/{skill,protocol,server}.go` — `handleSkillHygiene`,
  `CmdSkillHygiene`, dispatch.
- `kernel/webui/webui.go` — `/api/skills/hygiene` (readArgs, `idle_days`).
- `frontend/src/views/Skills.tsx` — idle strip + retire action.
- `kernel/skill/hygiene_test.go`.

## Verification

- **Gate:** `go build`, `go vet`, `staticcheck`, linux cross-build clean;
  `skill`/`controlplane` green; vitest **517 passed**; dist rebuilt. No new env;
  go.mod unchanged.
- **Unit:** `Hygiene` flags never-used and long-unused active skills, NOT
  recently-used ones, NOT brand-new ones (grace), NOT non-active ones; idle list
  sorted oldest-seen first.
- **Live (isolated home):** `/api/skills/hygiene` returns the right shape
  (`total:2, active:2`); the two freshly-seeded built-in skills are correctly NOT
  flagged (grace period), confirming the safety floor. Idle detection on aged
  skills is unit-covered.

## Notes
- Retire = quarantine, deliberately reversible — cleanup never destroys a skill,
  matching the soft, journaled posture of the rest of the skill lifecycle.
- Like memory prune, a periodic auto-pass (flag/retire on the pulse) is a natural
  follow-up; the report + one-click retire are the operator surface today.
