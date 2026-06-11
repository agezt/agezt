# Phase M838 — delegate never runs on an unkeyed model

**Date:** 2026-06-11 · **Status:** DONE · **Trigger:** owner: "delegate ederken
key girilmemiş provider ve modeller seçiliyor, bunu engelleyelim" — when
delegating, models from providers with no API key were being selected (and then
failing to route mid-delegation).

## Root cause

`runSubAgent` (kernel/runtime/subagent.go) set the child's model from the
explicit `delegate {model}` arg or the roster profile's `model`/`fallbacks`, with
no check that a **credentialed** provider actually serves it. An unkeyed model
therefore reached the Governor and failed to route.

## Fix

- New `Config.ModelAvailable func(modelID string) bool` — the daemon injects it
  (cmd/agezt), reporting whether some **registered + credentialed** provider
  serves the id (bare or `provider/model`). Built exactly like `VisionModel` /
  `CouncilMembers`, so the kernel stays free of credential logic.
- `runSubAgent` now resolves the effective model chain (chosen model + profile
  fallbacks) and passes it through `keyedModelChain(...)`: it **drops unkeyed
  models**, and if nothing keyed survives, falls back to the daemon's active
  (keyed) model so the delegation still runs. Done **before** the
  `subagent.spawned` event is journaled and the child runs, so the recorded and
  executed model is the one actually used. No-op when `ModelAvailable` is unset
  (tests / single-provider setups) — existing behaviour preserved.

## Verification

- **Unit** (`keyedModelChain`): drops an unkeyed primary → keyed default; keeps a
  keyed primary and filters unkeyed fallbacks; single keyed → nil chain; nothing
  keyed + no default → originals unchanged. Existing delegation/roster tests still
  green (filter is a no-op without the predicate).
- **Live** (isolated home, real keyed providers): `delegate {model:
  "totally-fake-unkeyed-model-9000"}` → the sub-agent **ran and returned PONG**,
  and `subagent.spawned` now records `model: deepseek-v4-pro` (the keyed
  fallback) instead of the bogus id. Before the fix the unkeyed id would reach the
  governor.

## Gate

runtime tests green; vet + staticcheck + linux cross-build clean; gofmt swept.
go.mod unchanged. No new env var.
