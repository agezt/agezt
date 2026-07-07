# Changelog

All notable changes to the Agezt kernel (`agezt` daemon + `agt` CLI) are
recorded here. Format follows [Keep a Changelog](https://keepachangelog.com/en/1.1.0/);
versioning is [semantic](https://semver.org/spec/v2.0.0.html). Pre-1.0 the
minor version tracks the product milestone (ROADMAP.md).

This is the human, per-component changelog (SPEC-08 §4.1). The machine,
tamper-evident timeline of what actually happened to a running system lives in
the hash-chained journal — `agt journal tail` / `agt why` (SPEC-08 §4.2).

## [Unreleased]

### Fixed
- Corrected Overseer tool schema so agent and reason descriptions cover the wake and bulk_retire operations, and added search and wake to the tool description.


### Added
- New Overseer `op=wake` operation lets you asynchronously trigger an agent with a specified intent and reason, including validation for retired, paused, and managed sub-agents


### Added
- Overseer tool: 5 new batch operations (`bulk_pause`, `bulk_unpause`, `bulk_retire`, `bulk_revive`, `bulk_delete`) to manage multi-agent fleets, with per-agent error isolation so one failure won't stop the rest


### Added
- CLI: new `agt overseer` command with 14 subcommands to manage agents and runs—`status`, `agents`, `runs`, `cancel`, `halt`, `resume`, `pause`, `unpause`, `retire`, `revive`, `delete`, `get`, `impact`, and `help`


### Added
- Overseer tool: fetch a single agent's profile with `op=get`, or create a new agent from an existing profile as a template with `op=clone`, applying overrides as needed


### Documentation
- Expanded the threat model's fleet lock documentation to address agent deletion


### Added
- Overseer tool: added `op=delete` to permanently remove agents, and fixed `op=retire` to pass the reason through to the kernel


See `CHANGELOG/unreleased/current.md` for the active working set and `CHANGELOG/` for historical milestone slices.

## Releases

Released version notes live in per-version files under `CHANGELOG/`.

- `v1.0.0.md` — `1.0.0` (2026-06-03)
- `v0.1.0.md` — `0.1.0` (2026-05-30)
