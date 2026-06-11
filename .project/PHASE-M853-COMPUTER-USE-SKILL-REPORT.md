# PHASE M853 — Computer-use, out of the box (built-in skill bundle)

**Status:** shipped
**Milestone:** M853
**Theme:** Full machine control — install/update/remove software and automate the
desktop GUI (screenshot, click, type, hotkeys) — working out of the box. Owner
ask: computer-use / full-privilege machine control (#44), the second half of the
chosen browser+computer-use area.

## Approach

Same proven path as M852 (browser-use): a **built-in skill bundle** the daemon
seeds active at startup, driven through the always-on `code_exec`/`shell` tools
— zero Go deps, on-ethos. The M852 `plugins/builtinskills` seeder already handles
multiple bundles, so this is a drop-in.

## What shipped

- **The `computer-use` bundle**, added to the startup seeder:
  - `SKILL.md` — the two halves: software management via `shell` (winget/choco/
    brew/apt/npm/pip, full permission) and GUI automation via the desktop driver;
    the see-then-act loop.
  - `scripts/setup.sh` — `pip install pyautogui pillow` (with Linux notes for
    scrot / python3-tk).
  - `scripts/desktop.py` — a stateless driver: an ordered action list
    (screenshot/move/click/double_click/type/press/scroll/wait/locate) plus a
    final screenshot, emitting JSON. Needs a desktop session; on a headless host
    it returns a clear error instead of crashing.
  - `reference/patterns.md` — per-OS package-manager recipes, the see-then-act
    loop, locate-by-template, and safety notes for destructive UI actions.
- `plugins/builtinskills/builtinskills.go` — `computeruse` added to the
  `go:embed` set and `builtinBundles`.

## Verification

- **Gate:** `go build`, `go vet`, `staticcheck`, linux cross-build clean;
  `builtinskills` tests green; `python -m py_compile desktop.py` passes. No new Go
  dep (go.mod unchanged); no new env. `.gitattributes` already forces LF on
  `plugins/builtinskills/**`.
- **Unit:** `SeedAll` installs both `browser-use` and `computer-use` active;
  `TestSeedAll_InstallsComputerUse` asserts the desktop driver + setup + reference
  are materialized on disk; re-seed idempotent.
- **Boot smoke (isolated home):** daemon logs `built-in skills : seeded
  (browser-use, computer-use)`; `agt skill list` shows both `[active]`.
- The live pyautogui install + a real GUI action can't run in the build sandbox
  (no display / no network); the driver is syntax-validated and uses standard
  pyautogui APIs, with an explicit headless error path. Seeding/activation/
  materialization fully verified.

## Notes
- Computer-use was already *possible* (shell + code_exec install/run anything
  under default-allow, and the M848 briefing says so); this packages the GUI
  half as a ready, documented, active skill so the agent does it reliably.
- The seeder is now the home for any built-in bundle; future additions (e.g. a
  docker-services bundle for #51) drop into `builtinBundles`.
