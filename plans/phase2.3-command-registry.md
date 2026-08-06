# Phase 2.3 — Controlplane command registry (working plan, survey 2026-08-06)

## Verified facts
- handleConn (server.go:481-1185): one request per conn (NO read loop), defer conn.Close +
  defer s.recoverConn(conn,&req) BEFORE parse (pre-auth panic containment; must stay in the
  goroutine frame), 10-min read deadline, readBoundedLine 16MiB ("request too large"),
  "bad request: " parse envelope, auth gate (tokenIsPrimary → tenants.Authorize →
  tenantTokenAllows → req.Args["tenant"] PINNING incl. map alloc), then a 319-case switch
  (1:1 with protocol.go's 319 Cmd* consts), default "unknown command: "+cmd (pinned by test).
- Handler shapes: 300× (conn,req), 19× (ctx,conn,req). All *Server methods (320 handle*;
  +handleAgentSetRetired shared impl off-switch).
- Streaming is TRI-STATE: StreamNone (300) / StreamEvents (plan, market_install/uninstall,
  toolbox_install — emit RespEvent, keep deadline) / StreamLive (pulse_subscribe clears
  deadline + tick watcher; 6× cancelOnConnClose: chat_summarize, conductor_ask, council_ask,
  plan_generate, plan_refine, research_ask). handleRun is special: cancelOnDisconnect-GATED
  goroutine cancelling the RUN (not ctx) — model as StreamEvents + keep inline logic.
- tenantTokenAllows (tenant.go:68): 47-command allowlist, deny-default. Invariant (comment
  only today): TenantAllowed ⟹ handler routes via kernelFor. ~44/319 tenant-routed
  (28 direct kernelFor calls + edictFor(7) + projectJournal(9)); ~275 use s.k.
  tenant_stats loops ALL tenants (legit exception).
- Server injected nilable deps: 10 (standingFire, diskWatch, probeWatch, diskFree,
  channelSend, boardNotify + pulse, tenants, updateSvc, boardStore). All handler state
  file-local (roster cache, oauth maps, provLogin) — subpackage split is viable later.
- Only 2 command-string switches exist (handleConn + tenantTokenAllows). webui has 4
  route→Cmd tables (follow-on: derive from registry metadata; NOT this phase).
  cmd/agt already has the registry pattern (commands.go + cmd_register.go) — mirror it.
- Tests: tenant_auth_test.go is load-bearing; fuzz_test mirrors handleConn's read path;
  TestUnknownCommand pins the envelope; startPair fixture exercises everything E2E.

## Design
DispatchCtx{Ctx, Conn, Req, S, K (pre-resolved), Tenant, Primary} — kills whoami re-check.
commandSpec{Cmd, Handler func(*DispatchCtx), TenantAllowed, TenantRouted, Streaming}.
register(specs...) panics on dup; explicit registerAll() calling per-subsystem
registerXSpecs() living NEXT TO the handlers (NOT per-file init()) — cmd_register.go shape.
Dispatch: lookup AFTER auth gate (preserve "forbidden" for unknown cmd from tenant token —
do NOT move lookup before auth). StreamLive → dispatch owns cancelOnConnClose + deadline.
TenantRouted → dispatch resolves dc.K = kernelFor(tenant) once.

## Commits
A (mechanical mirror): dispatch.go types + register; per-subsystem registerXSpecs with ALL
  319 specs as thin closures over existing methods (TenantAllowed copied verbatim from
  tenantTokenAllows; TenantRouted/Streaming per survey). handleConn UNTOUCHED. Tests:
  Registry_MatchesProtocolConstants (regexp protocol.go ↔ registry keys, both directions),
  Registry_MatchesOldSwitch (regexp server.go case arms ↔ keys; DELETE in B),
  Registry_TenantAllowedMatchesLegacyAllowlist (DELETE in B),
  Registry_TenantAllowedImpliesTenantRouted (PERMANENT — real tenant-isolation invariant).
B (the swap): delete 641-line switch + tenantTokenAllows; handleConn ≈60 lines using the
  registry; StreamLive dispatch-owned; delete the two commit-A bridge tests; extend
  tenant_auth_test with registry-driven exhaustive table (every TenantAllowed spec reachable
  by tenant token, every other → "forbidden").
C+ (unbounded, later): convert closures to native handlers per subsystem; kernelFor
  source-scan ratchet (baseline 21 files, lower as converted); subpackage split.

## Gotchas
- recoverConn must stay deferred in handleConn's frame pre-parse.
- Preserve exact strings: "unauthorized", "forbidden: a tenant token cannot run %q
  (primary token required)", "unknown command: ", "bad request: ", "request too large".
- Tenant arg pinning (req.Args["tenant"] = reqTenant, alloc if nil) must survive.
- handleRun keeps its bespoke disconnect goroutine (cancels run, opt-in).
