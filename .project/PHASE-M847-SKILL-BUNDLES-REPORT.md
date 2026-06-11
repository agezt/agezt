# PHASE M847 — Skill Bundles (agentskills.io shape: reference files + scripts)

**Status:** shipped
**Milestone:** M847
**Theme:** Make a skill more than an inline body — an **agentskills.io-style
bundle**: a `SKILL.md` plus **reference files** and **scripts** on disk, disclosed
progressively and runnable. Owner ask: *"skill mimarimiz agentskills.io stili
olmalı, skiller reference dosyalara ve hatta scripts e sahip olabilir… şu CLI'ı
kur şu app'i kur kullan vs dediğinde yapabilmeliyiz her şeyi"* — and the
follow-up: *"ajanların kendi kapasitelerini bilmesi lazım… python scriptler
javascript çalıştırmak, CLI tool kurmak, npm paketi kurmak… sınırları yok."*

A bundle's `scripts/setup.sh` (npm install, pip install, build a CLI…) is exactly
the "install this and use it" surface — run with the agent's existing
shell/code_exec tools (already max-capability, default-allow), now anchored to a
real on-disk directory the skill ships.

## What shipped

- **A skill can carry a bundle of files.** New `BundleStore`
  (`kernel/skill/bundle.go`) materializes a skill's resources under
  `<home>/skills/bundles/<slug>/…`, keyed by the skill's slug (stable across body
  edits). Write replaces the whole bundle atomically (temp-dir swap), List/Read
  serve it back, Dir is the working directory for scripts. Path-confined: any
  `..`/absolute path is rejected, so a bundle can never read or write outside its
  own directory. Caps: 1 MiB/file, 8 MiB/bundle.
- **`Skill.Resources []string`** — the manifest of bundled paths, on the record
  and in every wire view. `CreateSpec.Resources map[string][]byte` carries the
  files in; `Forge.SetBundles` wires the store (nil → body-only, unchanged).
- **Import a directory.** `agt skill import <dir>` reads `SKILL.md` (frontmatter
  → name/description/body) and sends every other file as a resource; the daemon
  materializes the bundle and registers the skill as a fresh **draft**.
- **Inspect a bundle.** `agt skill files <id>` lists the resources + the dir;
  `agt skill cat <id> <path>` prints one. Control plane: `CmdSkillFiles`,
  `CmdSkillReadFile`. Web UI (read-only): `/api/skill/files`, `/api/skill/file`.
- **The agent reaches its own bundles.** The `skill` tool gains `op=files`
  (list + dir) and `op=read` (one resource). When a bundled skill is retrieved
  into a run, the injected context now lists its resources and tells the agent to
  read references with `op=read` and run scripts with shell/code_exec from the
  reported dir — so "run `scripts/setup.sh` to install the CLI, then use it"
  actually executes.

## Surface

- `kernel/skill/bundle.go` (new) + `bundle_test.go` (new).
- `kernel/skill/skill.go` — `Skill.Resources`.
- `kernel/skill/forge.go` — `bundles` field, `SetBundles`/`Bundles`,
  `CreateSpec.Resources`, `writeBundle`, Create writes/attaches the bundle.
- `kernel/runtime/runtime.go` — wire `OpenBundles` into the Forge; `injectSkills`
  lists bundled resources with usage guidance.
- `kernel/controlplane/{protocol,server,skill}.go` — `CmdSkillFiles` /
  `CmdSkillReadFile` + dispatch; `handleSkillImport` accepts `resources`;
  `argResources` decoder; `skillView` includes resources.
- `kernel/webui/webui.go` — read-only `/api/skill/files`, `/api/skill/file`.
- `plugins/tools/skilltool/{skilltool,tool}.go` + test — `Bundles()` on the forge
  interface; `op=files` / `op=read`.
- `cmd/agt/skill.go`, `skill_import.go`, `skill_md.go`, `skill_files.go` (new) —
  directory import + `agt skill files` / `agt skill cat`.

## Verification

- **Gate:** `go build ./...`, `go vet`, `staticcheck`, linux cross-build all
  clean. Targeted tests green: `kernel/skill`, `plugins/tools/skilltool`,
  `kernel/controlplane`, `kernel/webui`, `kernel/runtime`. No frontend changed
  (the webui change is Go-only), so vitest is unaffected. No new `AGEZT_*` env;
  go.mod unchanged.
- **Unit:** bundle Write/List/Read/Dir, replace-drops-stale, escape rejection,
  size cap, absent-bundle, slugify; tool `op=files`/`op=read` over a real bundle +
  the no-bundle-store path.
- **Live (isolated AGEZT_HOME):** authored a sample bundle (`SKILL.md` +
  `reference/usage.md` + `scripts/setup.sh` that `npm install -g cowsay`):
  1. `agt skill import /tmp/sample-skill` → installed draft, 2 resources.
  2. `agt skill files <id>` → listed both files + the bundle dir.
  3. `agt skill cat <id> scripts/setup.sh` → printed the script.
  4. Bundle materialized under `skills/bundles/cowsay-greeter/`.
  5. `agt skill cat <id> ../../creds.json` → **refused** (escape blocked).
  6. Web UI `/api/skill/files` and `/api/skill/file` returned the same; the
     escaping HTTP read was refused too.

## Notes
- Default-allow preserved: bundles ship with their scripts; running them uses the
  existing shell/code_exec (net-on, allow-by-default). The path-confinement and
  size caps are the only guards, and they bound the bundle store itself, not the
  agent's capability.
- Follow-ups: a Skills-view UI panel to browse/read a bundle (routes already
  exist), and a capability self-awareness briefing so agents know up front they
  may install/run anything to finish a task (next milestone).
