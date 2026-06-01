# Phase Report — Milestone M108 (`agt config` effective routing)

> Status: **shipped** · Date: 2026-06-01 · SPEC-08 / operability.

## Why

`agt config show` reported which AGEZT_* vars are set (presence only), but for
the COMPLEX routing knobs — `AGEZT_TASK_ROUTES`, `AGEZT_TASK_ROUTE_REQUIRES`,
`AGEZT_TASK_MODEL_OVERRIDES` — presence isn't enough. A typo in the env silently
degrades to the default chain, and the operator had no way to confirm what
actually parsed without reading the boot log. When routing "doesn't work", the
first question is "did my rule even load?" — now answerable.

## What shipped

- **Governor introspection views** — `TaskRoutesView`, `TaskRouteRequiresView`,
  `TaskModelOverridesView` return independent copies of the parsed tables, so
  the control plane can surface them without reaching into governor internals or
  risking mutation.
- **`CmdConfig` routing section** — when the provider is a governor with any
  routing configured, the response gains a `routing` map (`routes` / `requires`
  / `model_overrides`). Absent otherwise, so the common no-routing daemon stays
  compact.
- **`agt config show`** renders the effective tables ("plan → [anthropic,
  openai]", "code → fast-model") when present.

## Tests

- `TestRoutingViews_CopyAndContent` (governor) — views return correct content
  and are copies (mutating a view doesn't leak into config); empty governor →
  empty views, no panic.
- `TestConfigSurfacesRouting` (control plane) — a routed governor surfaces
  `routing.routes` + `model_overrides` via `CmdConfig`.
- `TestConfigNoRoutingWhenUnconfigured` — no routing section without routes.

Test count: **1365 → 1368**. `go vet` clean, `GOOS=linux` builds, `go.mod`
unchanged, gofmt-clean.

## Live proof

```
$ AGEZT_TASK_ROUTES="plan=mock;code=mock" AGEZT_TASK_MODEL_OVERRIDES="code=fast-model" agezt &
$ agt config show
  routing (effective):
    routes:
      code         → [mock]
      plan         → [mock]
    model_overrides:
      code         → fast-model
```
