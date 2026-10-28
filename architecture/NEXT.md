# NEXT — handoff for the next coding agent

## Current checkpoint — W5.4x the journal family's ports, 2026-10-28

The owner resumed work after the report break. The W0–W5 objective remains open.

### Delivery and checkout

- Shared checkout: `D:\Codebox\PROJECTS\AGEZT`, branch `main`. Do not create or
  switch branches/worktrees. PRs are published by pushing `main`'s head to the
  existing remote transport branch `arch/w2-dispatch` (`git push origin
  HEAD:arch/w2-dispatch`), never by switching this checkout.
- W2.30b (roster status presentation/typed snapshot) is delivered through PR #710
  at `74727b6bd72c2d05da8db0e7f0c60d47e7cd561c` after all 24 exact-head CI jobs
  succeeded (run 37750892298); the merge tree equalled the tested head and local
  `main` fast-forwarded with the W2.30c work preserved byte-for-byte.
- W2.30c (typed `agent_list`) is delivered through PR #711 at
  `b29dd422b1a77f4e01af2a474485b16096714065` after all 24 exact-head CI jobs
  succeeded; merge tree equalled the tested head.
- W2.30d (typed `agent_graveyard`) is delivered through PR #712 at
  `53c7dd19e45e46498465b54cf211ae97c2f83251` after all 24 exact-head CI jobs.
- W2.30e (activity summary move) is delivered through PR #713 at
  `fb6a445051df0cf19b04c1ea3cb3bbd9600eec2d` after all 24 exact-head CI jobs.
- W2.30f (typed `agent_activity`) is delivered through PR #714 at
  `bb8cc8fa64c06e8d70a24b61fceec0ba4e9a47c4` after all 24 exact-head CI jobs.
- W2.30g (typed `agent_repair_status` + cursor current-state fix) is delivered
  through PR #715 at `6067c311fd94b44f2c23481cd45f214e36080cc9` after all 24 jobs.
- W2.30h (typed `agent_escalations`) is delivered through PR #716 at
  `2d10795e49624861cf3c4f88e0f299372f720a34` after all 24 exact-head CI jobs.
- W2.30i (`agent_resolve` audit fix) is delivered through PR #717 at
  `a844d1e710c6a34e6351e5b07d23647c371e9094` after all 24 exact-head CI jobs.
- W2.30j (typed `agent_set_enabled`) is delivered through PR #718 at
  `2515e47d57b91b3120d2153c1d077203de60d2f8` after all 24 exact-head CI jobs.
- W2.30k (typed `agent_add`/`agent_edit`) is delivered through PR #719 at
  `6e98d253001c5b3c0b22c1c2f81a7770e4877fda` after all 24 exact-head CI jobs.
- W2.30l (typed `agent_task_update`) is delivered through PR #720 at
  `db04620a1ecd5667a3a26515ad4d462780246513` after all 24 exact-head CI jobs.
- W2.30m (typed `agent_wake`) is delivered through PR #721 at
  `ebdcda45258986ffe8f8c311d4c8b0477eb8da19` after all 24 exact-head CI jobs.
- W2.30n (typed `agent_repair`) is delivered through PR #722 at
  `71a72a834e4445e711431b1fe4cc322058b41183` after all 24 exact-head CI jobs.
- W2.30o (typed `agent_resolve`) is delivered through PR #723 at
  `b6f6293c123873983f423634a8af5d9d982228fe` after all 24 exact-head CI jobs.
- W2.30p (typed `agent_impact`/`agent_tombstone`) is delivered through PR #724 at
  `e6ee56a9d16cd5c362c8df578a6b7ae947f170f5` after all 24 exact-head CI jobs.
- W2.30q (typed `agent_retire`/`agent_revive`) is delivered through PR #725 at
  `8904eee67ee22c4c086754307a8ac390f8a7cd2d` after all 24 exact-head CI jobs.
- W2.30r (typed `agent_remove`) is delivered through PR #726 at
  `a8db319906f306098a080326fec8a395f483f9de` after all 24 exact-head CI jobs.
- W2.31a (typed live run steering) is delivered through PR #727 at
  `5fa174d5334ee0cc7ba3f9dcec8b3931b2d28887` after all 24 exact-head CI jobs,
  together with an owner-approved `ci(lint)` commit: GitHub `stable` Go became 1.27.2, whose
  export data staticcheck 2026.2.1 cannot read (dominikh/go-tools#1832), so only
  the staticcheck step runs with `GOTOOLCHAIN=go1.27.1`. Drop that pin and bump
  `SC_TAG` once staticcheck supports Go 1.27.2.
- W2.31b (typed `cancel_run`) is delivered through PR #728 at
  `42511ea28bfdb8fd8efa29f377cf22451e7eae25` after all 24 exact-head CI jobs.
- W2.31c (typed `runs_list`/`runs_stats`) is delivered through PR #729 at
  `5d893f5d644add906c73e31d0d56ba0c60c38623` after all 24 exact-head CI jobs.
- W2.32a (typed `journal_head`/`journal_tail`/`journal_stats`, one commit) is
  delivered through PR #730 at `aff7857bca02daa82c53c9ab7b2e24e25ed3a49d` after
  all 24 exact-head CI jobs.
- W2.32b (typed `journal_grep`/`journal_export`) is delivered through PR #731 at
  `48ba2de9b53dec381a6160205144d0134dddc445` after all 24 exact-head CI jobs.
- W2.32c (typed `changelog`/`cache_stats`) is delivered through PR #732 at
  `a6ecb06774fdf3921db1e9d7c98895a1ae8d01b9` after all 24 exact-head CI jobs.
- W2.32d (typed guard audit reads) is delivered through PR #733 at
  `583edb20c1a9706c4318b4cf0e3e34143891c94e` after all 24 exact-head CI jobs.
- W2.33a (typed edict reads) is delivered through PR #734 at
  `6563755588054d649496328d4225dfb4b213519a` after all 24
  exact-head CI jobs.
- W2.33b (typed edict writes) is delivered through PR #735 at
  `8b5e2f97c788bc4d0d43e6cc0c07c9f33614f16f` after all 24 exact-head CI jobs. The
  first head failed `test (linux)` five times on a W2.32b clock race
  (`TestJournalNativeRouting` asserted an empty `since_ms: 1` export moments
  after journaling); a test-only commit now waits until the clock passes the
  newest event.
- W2.33c (typed `edict_log`/`edict_stats`) is delivered through PR #736 at
  `fca5700bbf8df87fc2efb0d7921e4869756bee3b` after all 24 exact-head CI jobs.
- W2.33d (typed `edict_overlay`/`edict_compact`) is delivered through PR #737 at
  `4e6cb5414de6fceacc905128f85f8d7ada7c6f2b` after all 24 exact-head CI jobs.
- W2.34a (typed `approvals_log`/`approvals_stats`) is delivered through PR #738 at
  `70c63bd9c625a28ef2f7a57996b8bdda7a50e9de` after all 24 exact-head CI jobs.
- W2.34b (typed `approvals`/`decide`) is delivered through PR #739 at
  `b6fa81ae6ffe029b9291aaa5bf450f7a728d8dea` after all 24 exact-head CI jobs.
- W2.35a (typed `plan_history`/`plan_stats`) is delivered through PR #740 at
  `5b53e9600b41a6b95825c72c55c69c69cc517810` after all 24 exact-head CI jobs.
- W2.36a (typed tenant management) is delivered through PR #743 at
  `7212e4da9f4e4140f8e0c701cce5f7c8b895f184` after all 24 exact-head CI jobs
  (#741/#742 are unrelated Dependabot PRs; leave them).
- W2.37a (typed halt/resume/journal_verify/shutdown, plus a test-only fix that
  gives the toolforge runner-deadline test a load-safe 500 ms deadline) is
  delivered through PR #744 at `3a6c2fa3348af5c7cacebb1c517f8632fe412dca` after
  all 24 exact-head CI jobs.
- W2.37b (typed `why`) is delivered through PR #745 at
  `767e7f961572043cc11b27cd3537458a59af002f` after all 24 exact-head CI jobs.
- W2.38a (typed `agent_permissions`/`agent_capabilities`) is delivered through
  PR #746 at `19b4623c94aeface60d1d6b87eecf4290dbba6ba` after all 24 exact-head
  CI jobs.
- W2.39a (typed `state_list`/`state_get`) is delivered through PR #747 at
  `15e89a40b72d199c6d680940219d72f4dba18d30` after all 24 exact-head CI jobs.
- W2.39b (typed `disk_stats`) is delivered through PR #748 at
  `5aedafe5ea82064edc8186baebc0c4e6adff870b` after all 24 exact-head CI jobs;
  local `main` fast-forwarded to it.
- W2.39c (typed `redact_test`) is delivered through PR #749 at
  `747129982f75d2584bb0926e925757f88b2da255` after all 24 exact-head CI jobs;
  local `main` fast-forwarded to it.
- W2.39d (typed `reaper_scan`) is delivered through PR #750 at
  `a3c5d939b04548ee71803e21db3566d8c3cebe4d` after all 24 exact-head CI jobs;
  local `main` fast-forwarded to it.
- W2.39e (typed `spend_today`/`attention`) is delivered through PR #751 at
  `251f88f25bbfa024e2f7b063521b8ee0654676e0` after all 24 exact-head CI jobs;
  local `main` fast-forwarded to it.
- W2.39f (typed sandbox inspection) is delivered through PR #752 at
  `2ae3e5229c6ca69fdec6ae3c5f8f0f903e8695c5` after all 24 exact-head CI jobs;
  local `main` fast-forwarded to it. Its first head failed the Linux test legs:
  a new test read a backslash-separated path, which only Windows splits, so
  keep new path tests platform-aware.
- W2.40a (typed `budget`/`budget_set`) is delivered through PR #753 at
  `ef382717f6337fa44071feda55bebeeb89ad5ab5` after all 24 exact-head CI jobs;
  local `main` fast-forwarded to it.
- W2.40b (typed routing and chains) is delivered through PR #754 at
  `88182afabe71a9bd2ad490ed2a8386da725ecc52` after all 24 exact-head CI jobs;
  local `main` fast-forwarded to it.
- W2.40c (typed execution profiles) is delivered through PR #755 at
  `49714a8e20545566bae16a8e89a492d97a855317` after all 24 exact-head CI jobs;
  local `main` fast-forwarded to it.
- W2.41a (typed data lake) is delivered through PR #756 at
  `b49c09271270de2f62a520df11f4420ac5a5bcd6` after all 24 exact-head CI jobs;
  local `main` fast-forwarded to it.
- W2.42a (typed persona and prompts) is delivered through PR #757 at
  `5c3929a5ebf34099c03c3eefe3b2da19d9fad4d5` after all 24 exact-head CI jobs;
  local `main` fast-forwarded to it.
- W2.42b (typed seats) is delivered through PR #758 at
  `84a446cf60537426016b305af072f555ee385bff` after all 24 exact-head CI jobs;
  local `main` fast-forwarded to it.
- W2.42c (typed council membership) is delivered through PR #759 at
  `a3b85022cd3053374a4b29050f292b897017925d` after all 24 exact-head CI jobs;
  local `main` fast-forwarded to it.
- W2.42d (typed reflection) is delivered through PR #760 at
  `9c80efd7cbe4f9f5e390d1d36db873f0413c61dc` after all 24 exact-head CI jobs;
  local `main` fast-forwarded to it.
- W2.42e (typed conductor roles) is delivered through PR #761 at
  `28ce807406c59024855967e45a3bdf25b0847d83` after all 24 exact-head CI jobs;
  local `main` fast-forwarded to it.
- W2.42f (typed chat suggestions) is delivered through PR #762 at
  `1733503f2012295b68505ebc0b9503314e4c797d` after all 24 exact-head CI jobs;
  local `main` fast-forwarded to it.
- W2.42g (typed node registry) is delivered through PR #763 at
  `11c4b812e4576b2832472cc715dcd4559eb645e1` after all 24 exact-head CI jobs;
  local `main` fast-forwarded to it.
- W2.43 (typed file group) is delivered through PR #764 at
  `f593e7c215c8f34e137aa0393899aa8764f47090` after all 24 exact-head CI jobs;
  local `main` fast-forwarded to it.
- W2.44a (typed chat summarize) is delivered through PR #765 at
  `850310388eb65ac3c2787b06ac1334d23b10c7de` after all 24 exact-head CI jobs;
  local `main` fast-forwarded to it.
- W2.44b (typed council ask and conductor ask) is committed on `main` as a code
  commit plus a docs commit and published from `arch/w2-dispatch` (`gh pr list
  --head arch/w2-dispatch`). Require all 24 exact-head jobs including `CI` and
  `ci.yml`, then merge normally with a matching head and fast-forward `main`.
- W2.44c (typed research ask) is committed on `main` as a code commit plus a
  docs commit and published from `arch/w2-dispatch` the same way. The lenient
  readers and the correlation sanitizer move to `kernel/app/lenient.go`, so
  `kernel/controlplane/args_lenient.go` and the `stringArg` helper in
  `kernel/controlplane/send.go` are gone.
- W2.44d (typed plan generate and plan refine) is committed on `main` as a
  code commit plus a docs commit and published from `arch/w2-dispatch` the
  same way. `kernel/controlplane/planner.go` is gone, the archcheck allowlist
  tightens to 130 import violations, and the unused `requiredArgString` leaves
  the control plane.
- W2.44e (typed plan execution) is committed on `main` as a code commit plus a
  docs commit and published from `arch/w2-dispatch` the same way.
  `kernel/controlplane/server_handlers_plan.go` is gone, the request id that
  names a plan moves to `kernel/app` (`app.WithRequestID`), and the archcheck
  allowlist tightens to 129 import violations.
- W2.44f (typed run) is committed on `main` as a code commit plus a docs commit
  and published from `arch/w2-dispatch` the same way.
  `kernel/controlplane/server_handle_run.go` and `server_handle_run_remote.go`
  are gone, the strict argument readers (`argBool`, `argInt64`, `argStringList`,
  `parseCapList`) and `argTruthy` move to `kernel/app/runs/run_args.go`, and the
  execution-profile inventory builder moves to
  `kernel/app/execprofile/inventory.go`. The archcheck allowlist stays at 129
  import violations and 13 call sites.
- W2.46 (typed operator wake and repair launches) is committed on `main` as a
  code commit plus a docs commit and published from `arch/w2-dispatch` the
  same way. `kernel/app/roster/launch.go` (140 lines) gains
  `LaunchOperatorWake` and `LaunchOperatorRepair`: the application now
  resolves whether the run retries (a profile retries only with a retry
  policy of more than one attempt), what cost ceiling it carries, what the
  terminal event says (the phase, the answer or the error, the autonomy
  runbook, the whole incident lineage) and how a panicking governed repair is
  contained (the WF-001 firewall). The host keeps the data access: the
  kernel's `RunWith`/`RunWithRetry`, the governed `overseertool` repair source
  and the bus publisher; the retry policy and the ceiling reach it as
  decisions. `kernel/controlplane/roster_repair.go` (64 lines) is gone, the
  native `runAgentWake`/`runAgentRepair` bodies leave the control plane, and
  the control plane's `truncate` helper leaves with them. The archcheck
  allowlist stays at 129 import violations and 13 call sites.
- W2.47 (typed operator resolve journal reads) is committed on `main` as a code
  commit plus a docs commit and published from `arch/w2-dispatch` the same
  way. `kernel/app/roster/resolve_journal.go` (123 lines) gains
  `LatestOperatorForceGeneration` and `LatestExhaustedRoutingChain` over a
  journal port, moved verbatim with their filters and their "latest wins"
  rules; the incident-lineage matching moves with them, and the native
  `operatorWakeLineage` transport type is gone (the adapters and the resolve
  ports speak the app's `IncidentLineage` end to end).
  `kernel/controlplane/roster_wake_helpers.go` (107 lines) is gone, and with
  it `kernel/controlplane/roster_helpers.go` — the payload accessors whose
  last readers the moved helpers were. The archcheck allowlist stays at 129
  import violations and 13 call sites.
- W2.48 (typed agent teardown orchestration) is committed on `main` as a code
  commit plus a docs commit and published from `arch/w2-dispatch` the same
  way. `kernel/app/roster/teardown.go` (163 lines) gains the teardown's
  orchestration over a store port set: `RetireSubagents`,
  `RemoveStandingOrders`, `PauseStandingOrders`, `CountPausedStandingOrders`,
  `RemoveSchedules`, `PauseSchedules` and `CountPausedSchedules`, moved
  verbatim with the switch semantics, the case-insensitive agent match, the
  "only what the store actually had" counting, the enabled-only pauses and
  the sorted retired slugs. `kernel/controlplane/roster_crud_internal.go`
  (12 lines — a one-line wrapper around the app's own
  `ManagedDirectCallError`, whose three call sites now call the app directly)
  and `kernel/controlplane/roster_activity_text.go` (a header-only file left
  by an earlier split, with no code) are gone. The archcheck allowlist stays
  at 129 import violations and 13 call sites.
- W2.49 (typed agent teardown state cleanup) is committed on `main` as a code
  commit plus a docs commit and published from `arch/w2-dispatch` the same
  way. `kernel/app/roster/teardown_state.go` (260 lines) gains the teardown's
  state cleanup over a store port set: `ForgetAgentMemory`,
  `ForgetAgentAuthoredSharedMemory`, `ArchiveAgentSkills`,
  `DeleteAgentConfigEntries` and `DeleteAgentWorkspace`, moved verbatim with
  the switch semantics, the scope rules, the unscoped shared records, the
  skipped illegal transition, the sorted config deletions, the separate
  deletion and prune counts and the confined-workspace safety rules. The
  filesystem is a port, not a dependency: the application decides whether a
  path is safe to delete, the host does the deleting. Two of the moved
  predicates are shared with the impact cascade and are now exported
  (`MemoryRecordBelongsToAgent`, `MemoryRecordAuthoredSharedByAgent`). The
  archcheck allowlist stays at 129 import violations and 13 call sites.
- W2.50 (typed impact cascade) is committed on `main` as a code commit plus a
  docs commit and published from `arch/w2-dispatch` the same way.
  `kernel/app/roster/impact_cascade.go` (341 lines) gains the impact
  derivation: `ScheduleImpact`, `MemoryImpact`,
  `AuthoredSharedMemoryImpact`, `SkillImpact`, `ConfigImpact`,
  `WorkspaceImpact`, `WorkflowImpact`, `MailboxImpact`,
  `RemovalMailboxImpact` and `Subagents`, plus the predicates they apply
  (`MailboxImpactLabel`, `WorkflowNodeConfigReferencesAgent`,
  `MemoryRecordLabel`), moved verbatim with the label fallbacks, the ratings,
  the workflow node labels, the removal list's label-level deduplication and
  the sorted sub-agent tree. The board's read limit stays in the host.
  `kernel/controlplane/roster_lifecycle_helpers.go` (121 lines) is gone. The
  archcheck allowlist stays at 129 import violations and 13 call sites.
- W2.51 (typed workspace helpers) is committed on `main` as a code commit plus
  a docs commit and published from `arch/w2-dispatch` the same way.
  `kernel/app/roster/workspace.go` gains `WorkspaceInfo` (which path is safe
  to look at and what the label says) and `WorkspaceRoot` (the
  `AGEZT_WORKSPACE` override, else the base directory's `workspace` folder);
  the control plane keeps the bindings and the directory walk. With this
  slice the roster domain's derived logic is wholly in the application layer
  — what the control plane keeps is the host's data access. The archcheck
  allowlist stays at 129 import violations and 13 call sites.
  `whoami` is now the only native command left, and it stays native by design:
  it echoes the transport principal and must not route the tenant it names.
- W2.52 (typed run fold) is committed on `main` as a code commit plus a docs
  commit and published from `arch/w2-dispatch` the same way.
  `kernel/app/runs/collect.go` gains `Collect` (the single journal walk that
  turns a run's events into a row) and `RunRows` (the conversion into the
  roster status' row type), plus the payload extractors the fold feeds on
  (the intent, the agent, the tool, the iters, the cost, the model, the
  answer excerpt, the spawn link and the failure reason), moved verbatim with
  the forward walk and its post-sort limit, the `task.received` start, the
  `task.completed`/`task.failed`/`task.abandoned` terminals, the spawn's
  parent link, the spend's "existing row only" rule and its first-wins model,
  the last-wins activity phase (`thinking`, `using tool`, `observing tool`,
  `continuing`) and the 80-rune answer excerpt. The native `runEntry` row type
  is gone — the fold produces the application's own `Run` row — and
  `kernel/controlplane/runs_extract.go` (the payload extractors) is gone too;
  `kernel/controlplane/runs.go` keeps only the host's journal read. All three
  readers — the run service, the roster status and the schedule/tenant
  activity — now ask the application. The archcheck allowlist stays at 129
  import violations and 13 call sites.
  The mutation run was honest about its order: the first pass caught 21 of 26
  and left five open (four test gaps and one anchor that did not match the
  source), the tests were strengthened and the run repeated — 28 mutations, 28
  caught.
- W2.53 (typed remote mirror) is committed on `main` as a code commit plus a
  docs commit and published from `arch/w2-dispatch` the same way.
  `kernel/app/remote/mirror.go` gains `Mirror` (the whole decision: whether
  the mirror runs, what a mirror that cannot reach the peer looks like, and
  what the mirrored payload carries), `EventRows` and `ArtifactRows` (the wire
  rows), plus the mode policy (`Mode`, `EnvMode`, `PayloadMode`), moved
  verbatim with the mode parsing and its off state, the metadata check, the
  peer-lookup and fetch failure shapes, the payload mode, the event and
  artifact row fields, the caption that is said only when the peer said one
  and the 200-event/200-artifact bounds. The host keeps the data access: the
  peer configuration, the two peer HTTP fetches (which now return typed rows
  instead of wire maps), the redactor and the journal publish. The native
  `remoteEventMirrorMode` and `remoteMirrorPayloadMode` are gone.
  31 mutations, 31 caught: the first pass caught 27 of 28 and left one anchor
  skipped, the second and third passes caught the rest — the last one only
  after the test was strengthened (the failing fetch's payload had not
  asserted that it names the peer and the correlation). The archcheck
  allowlist stays at 129 import violations and 13 call sites; the package
  count is 269 because `kernel/app/remote` is new, and the generated
  structure document gains it.
  The order-7 list in this handoff was stale: journal, edict, tenant and
  shutdown are already typed (the whole protocol command registry is typed —
  `whoami` is the only native command left, and it stays native by design).
  The remote mirror was the last real decision logic of that list, and this
  slice takes it.
- W2.54 (typed operation-audit argument policy) is committed on `main` as a
  code commit plus a docs commit and published from `arch/w2-dispatch` the
  same way. `kernel/app/opaudit/opargs.go` (a new package) gains
  `IsSecretArg`, `TruncateValue` and `SummarizeArgs`, moved verbatim with the
  thirteen secret-name fragments, the case-insensitive match, the 160-rune
  bound with its ellipsis, the scalar/list/object/unknown shapes, the sorted
  object keys and the dropped nil argument. The native `auditValueMax`,
  `secretArgFragments`, `secretArg` and `auditArgs` are gone; the native
  dispatch path and the typed operation path now share one implementation, so
  the two audit trails cannot drift apart. The package is new because
  `kernel/app/audit` owns the GUARD audit reads, which are a different audit.
  19 mutations, 19 caught, no equivalent recorded. The archcheck allowlist
  stays at 129 import violations and 13 call sites; the package count is 270
  because `kernel/app/opaudit` is new.
  The control plane's remaining derived logic is now thin: the re-projections
  between the application's row types (`app_operations.go` for the council,
  budget, routing and plugin rows, `schedule_helpers.go` for the schedule
  firing rows, `tenant.go` for the tenant run rows), the one confirmed
  duplicate (`workboard_dispatch.go:66-86` mirrors the warden branch of
  `app/runs/run_resolve.go:311-319`) and one dead constant
  (`boardReadDefaultLimit` in `board.go:16`).
- W2.55 (typed run-derived rows) is committed on `main` as a code commit plus
  a docs commit and published from `arch/w2-dispatch` the same way. With the
  fold in the application layer (W2.52), two consumers still re-projected
  the folded run into a second copy of a run row: the schedule's firing
  views copied nine fields into `appschedule.FiringRun`, and the tenant's
  activity summary copied six into `apptenants.Run`. Both copies are gone —
  `kernel/app/schedule` and `kernel/app/tenants` read `appruns.Run`
  directly, so the firing view, the tenant summary and the runs view cannot
  disagree about a run, and the control plane passes the fold straight
  through. `kernel/app/schedule` and `kernel/app/tenants` now import
  `kernel/app/runs` (safe: `app/runs` imports `app/roster`,
  `app/execprofile` and the approval package, none of which import them).
  20 mutations, 20 caught: the first pass caught 18 and left two open (the
  status filter's failed and abandoned branches were never exercised), the
  second caught 19 and left the failed run's duration open, and the third
  caught all 20 after both tests were strengthened. The archcheck allowlist
  stays at 129 import violations and 13 call sites; the package count stays
  at 270 (no new package).
  The projections that are left in the control plane convert RUNTIME types —
  the council's `runtime.CouncilMember`, the governor's snapshot, the
  runtime's plugin registration — into the application's row types. Those
  conversions stayed where they were for the W2.56 slice; they are the next
  candidates. (The reason recorded here at the time — "the application layer
  cannot import `kernel/runtime`" — was wrong, and is corrected in W2.58:
  the layer rule allows L4 → L3, and `app/system` already imports
  `kernel/runtime`. The prohibition that matters is the module reach, and a
  shared value type belongs in the contract layer.)
- W2.56 (typed profile and snapshot projections) is committed on `main` as a
  code commit plus a docs commit and published from `arch/w2-dispatch` the
  same way. `kernel/app/routing/agents.go` (new) gains
  `Agents(profiles []roster.Profile) []Agent` and
  `kernel/app/budget/snapshot.go` (new) gains
  `SnapshotOf(snap governor.BudgetSnapshot) Snapshot`: the roster's profiles
  and the governor's counters become the application's rows in the
  application, and the control plane's two inline conversions now call it.
  The host reads its own data and hands it over; the application's row stays
  the application's business. The two new imports are safe in this
  direction (`kernel/roster` and `kernel/governor` import no app package).
  16 mutations, 16 caught, no equivalent recorded. The archcheck allowlist
  stays at 129 import violations and 13 call sites; the package count stays
  at 270 (no new package).
  What is left of the control plane's derived logic: the one confirmed duplicate
  (the warden branch in `workboard_dispatch.go`, which mirrors
  `app/runs/run_resolve.go`), the dead `boardReadDefaultLimit` constant, and
  the remote mirror's own environment read (which is not a second source of
  truth: the settings store writes the same variable into the process
  environment as an `ApplyLive` setting).
- W2.57 (one warden branch) is committed on `main` as a code commit plus a
  docs commit and published from `arch/w2-dispatch` the same way. The
  warden-family execution profile was resolved twice — once by the direct run
  path in the application layer, once by the dispatched seat path in the
  control plane, whose own comment said it "mirrors the warden branch of the
  run handler". `kernel/app/runs/run_profile.go` (new) gains
  `WardenProfile(ctx, k, id)`: it resolves the profile, refuses an id the run
  tools do not route, refuses the container profile without an active
  container backend and layers the sandbox override onto the context. Both
  callers use it now. The control plane keeps what is its own: the id's
  trimming, the empty id, the environment policy gate and the rejection of
  the remote backends (ssh, k8s, modal, daytona, remote-agezt), which only a
  direct run can use.
  10 mutations: 9 caught, 1 recorded equivalent (trimming the id is
  redundant with `WardenProfileForRun`'s own trimming).
  **One operator-visible wording change**, documented in the evidence doc and
  the changelog: the dispatched seat path now reports the application's
  refusals instead of its own terser ones — the routability refusal and the
  container-backend refusal now read exactly like the direct run path's.
  The policy refusal and the remote-backend refusal, and their order, are
  unchanged. The archcheck allowlist stays at 129 import violations and 13
  call sites; the package count stays at 270 (no new package).
- W2.58 (one shared row type for the council and the plugins) is committed on
  `main` as a code commit plus a docs commit and published from
  `arch/w2-dispatch` the same way. The council's membership and the plugin
  manifest were carried across the host/application boundary twice — once as
  the runtime's value types (`kernel/runtime/types`), once as the
  application's row types (`app/council.Member`, `app/plugins.Registration`)
  — so the host copied the fields of every row it handed over. The three
  shared value types move down to the contract layer, where a shared VALUE
  type belongs: `kernel/contract/types` (new) holds `CouncilMember`,
  `PluginInfo` and `SubAgentLimits` verbatim, `kernel/runtime/types` is
  deleted, the runtime keeps its public aliases (so the host's API is
  unchanged), and `app/council.Member` and `app/plugins.Registration` are now
  aliases of the contract types. The four projections that existed only
  because the types differed are gone: the control plane's council read and
  write, the control plane's manifest reader, and the council answer's member
  list. The semantics are unchanged: the wire shapes, the plugin rows' prefix
  ordering, the manifest's defensive copies and the empty-array-never-null
  rule for an empty panel.
  18 mutations, 18 caught, no equivalent recorded. The archcheck allowlist
  stays at 129 import violations and 13 call sites; the package count stays
  at 270 (the old package moved, the new one took its place).
  The control plane suite passed at count=20 twice; an earlier count=20 run
  failed once and the failing test's name was not captured (that run's output
  was piped through the tail of the log, so only the suite's deliberate
  operator-repair panic lines and the `FAIL` summary survived). The
  observation is recorded in the evidence doc, not dismissed — the name is
  still unknown, and the slice carries no timing, concurrency or I/O change.
- W2.59 (one remote-backend list) is committed on `main` as a code commit plus
  a docs commit and published from `arch/w2-dispatch` the same way. The remote
  execution backends — ssh, k8s, modal, daytona and remote-agezt — were named
  in four places with three different sets: the application's run path, the
  control plane's dispatched seat, the `code_exec` tool's artifact export
  (four of them) and the profile inventory. The two host-side lists had
  opposite meanings — the application supports a remote backend for a direct
  run, the control plane refuses it for a dispatched seat — and two literal
  lists with opposite meanings are two places to forget a name.
  `kernel/executionprofile/remote_backends.go` (new) owns the names: the five
  id constants, `RemoteBackendIDs` and `IsRemoteBackend(id)` (case- and
  space-insensitive). The inventory's profiles and its routable ids, the run
  path's switch, the dispatched seat's refusal and the `code_exec` tool's
  artifact rule all name the backends through it. The **policies** stay where
  they were — the application still resolves a remote backend for a direct run
  with its own refusal when the backend is inactive, the host still refuses it
  for a dispatched seat with the same message, and the `code_exec` tool still
  exports the remote artifacts from the local root.
  The dead `boardReadDefaultLimit` constant in `kernel/controlplane/board.go`
  is removed with it.
  19 mutations: 18 caught, 1 recorded equivalent (the `code_exec` tool's
  remote-agezt exception — that profile never runs the tool). The first pass
  left one test gap open (the routable-ids lookup, invisible while every
  backend was active); the strengthened test catches it on the repeat. The
  archcheck allowlist stays at 129 import violations and 13 call sites; the
  package count stays at 270 (no new package).
  One observation is recorded rather than dismissed: an earlier count=20 run
  of the control plane's affected tests failed once, when the workboard app
  socket test hit its five-second pipe deadline ("read pipe: i/o timeout") on
  one iteration; the same command then passed and the test passes on its own.
  The slice changes no socket, no deadline and no timeout.
- W2.60 (the peer's wire has one declaration) is committed on `main` as a code
  commit plus a docs commit and published from `arch/w2-dispatch` the same
  way. A delegated run's peer answers two HTTP endpoints, and the rows it
  sends were described three times — once on the serving side
  (`restapi.ArtifactEntry`), once on the reading side (`app/remote.Artifact`
  and `app/remote.Event`), each without the other's names. The host's two
  peer fetches therefore decoded the peer's answer into an anonymous struct
  and copied every field into the application's row: nineteen field copies
  that could drop a field or swap a name.
  `kernel/contract/remoteapi` (new, L1) is the contract between two daemons
  — the artifact row with the wire's json names. The serving adapter's row,
  the application's row and the contract row are now the same declaration,
  and the application's event row is the canonical event, the very row the
  peer serves. The host's fetches decode the peer's answer straight into the
  application's rows, and both field-copy loops are gone. Only a spelling
  changed: the row says `CreatedMS`, the wire key `created_ms` does not.
  14 mutations, 14 caught: the first pass left one test gap open (the
  published artifact row's sender was not asserted) and the strengthened test
  catches it on the repeat. The archcheck allowlist stays at 129 import
  violations and 13 call sites; the package count is 271 (the new contract
  package).
  **What is left of the artifact row:** the store's own row
  (`kernel/artifact.Entry`, which the REST adapter copies field by field in
  `cmd/agezt/api_engine.go`) and the application's own artifact row
  (`kernel/app/artifacts.Entry`, which spells the correlation
  `CorrelationID`). They are the store's row and the application's row, not
  the peer's wire, so they keep their own declarations — but they are the next
  candidates for the same treatment.
- W2.61 (the artifact row has one declaration) is committed on `main` as a
  code commit plus a docs commit and published from `arch/w2-dispatch` the
  same way. The artifact metadata row was described four times — the store's
  persisted row, the peer's wire, the REST adapter's row and the CLI's peer
  reader — and the daemon's REST adapter copied the store's row into the wire
  row field by field on every artifact it served.
  The store's row (`kernel/artifact.Entry`) and the CLI's peer row
  (`cmd/agt/peers_fetch.go`) are now the contract row, and the copy
  (`cmd/agezt/api_engine.go`'s `restArtifactEntry`) is gone. The on-disk shape
  is unchanged (the same json names, in the same order, with the same
  `omitempty` behaviour) and the wire key `created_ms` is unchanged; only the
  Go spelling changed (`CreatedMs` → `CreatedMS`), which is a mechanical
  rename across the store, the application, the commands and the plugin
  tests.
  The application's own artifact row (`kernel/app/artifacts.Entry`) keeps its
  declaration: it is the typed operation's own output wire — it spells the
  correlation `CorrelationID` and keeps the empty keys — so the copy into it
  remains.
  12 mutations, 12 caught. The application's listed-row test now asserts the
  *values* the store holds under every key, not only that the keys are there
  — that was the test gap the mutation run found. The archcheck allowlist
  stays at 129 import violations and 13 call sites; the package count stays
  at 271 (no new package).
  **One claim was corrected in place:** the W2.55/W2.56/W2.57 notes said the
  application layer "cannot import `kernel/runtime`". That is wrong — the
  layer rule allows L4 → L3 and `app/system` already imports
  `kernel/runtime`. The prohibition that matters is the module reach, and a
  shared value type belongs in the contract layer. The correction is in
  `architecture/124-w257-one-warden-branch-evidence.md` and in the W2.55
  bullet above.
- W2.62 (the boundary rows have one declaration) is committed on `main` as a
  code commit plus a docs commit and published from `arch/w2-dispatch` the same
  way. Three rows crossed the host/application boundary and each was described
  **twice**: the council opinion (`kernel/runtime.Opinion` and
  `kernel/app/council.OpinionRow`), the tenant listing snapshot
  (`kernel/tenant.Info` and `kernel/app/tenants.Row`) and the delegation
  ceilings (`kernel/contract/types.SubAgentLimits`,
  `kernel/app/system.DelegationStatus` and
  `plugins/tools/introspecttool.Delegation`). Each copy was filled in **field
  by field** — five fields per opinion, three per tenant, five per ceiling row.
  `kernel/contract/types` (L1) now declares each row once: `Opinion` (new),
  `TenantInfo` (new) and `SubAgentLimits` (which gains the json names the wire
  already used). The runtime keeps its public names (`runtime.Opinion`,
  `tenant.Info`), the application and the plugin alias them, and the three
  field-copy loops are gone: the council answer, the tenant listing, the status
  row and the introspection tool publish the host's rows **unchanged**.
  No wire changed — the same json names, in the same order, with the same
  `omitempty` behaviour and the same values; the empty-list rule (an empty
  array, never `null`) is kept.
  14 mutations, 14 caught. The archcheck allowlist stays at 129 import
  violations and 13 call sites; the package count stays at 271 (no new
  package).
- W2.63 (the reaper's findings have one declaration) is committed on `main` as
  a code commit plus a docs commit and published from `arch/w2-dispatch` the
  same way. Every finding the reaper publishes — the dead, degraded,
  misconfigured, retry-pressure, routing-pressure, forced and unstable rows —
  was described **twice**: once where the runtime finds it
  (`kernel/runtime/reaper.go`) and once where the application publishes it
  (`kernel/app/reaper/reaper.go`), and the application's copy was filled in
  **field by field** (about seventy field copies across seven helpers). The
  forced findings were worse: `RoutingForcedProbationAgent`,
  `RoutingForcedFailedAgent` and `RoutingForcedExhaustedAgent` were three
  byte-identical declarations of the same row.
  The findings are declared **once** now, in the runtime: the application's
  seven row types are aliases of the runtime's seven rows, the three forced
  phases are one row (`RoutingForcedAgent`, with the three old names kept as
  aliases so the runtime's public API is unchanged) and the seven copy loops
  are gone — the scan hands the runtime's lists over with a slice copy. The
  runtime's finding rows also carry every field (the optional ones lost their
  `omitempty`); they were never marshalled, so nothing observable changes.
  **No wire changed**: the operator's scan keeps the same keys in the same
  order, the unset lists stay `null` and an empty finding stays an empty array.
  The introspection tool keeps its own compact reaper view for the model — its
  own wire, not the operator's scan.
  12 mutations, 12 caught. The archcheck allowlist stays at 129 import
  violations and 13 call sites; the package count stays at 271 (no new
  package).
- W2.64 (the execution profile's rows have one declaration) is committed on
  `main` as a code commit plus a docs commit and published from
  `arch/w2-dispatch` the same way. The profile, the health check, the secret
  policy, the inventory and the health report were described **twice**: once in
  the module that builds them (`kernel/executionprofile`) and once in the
  operation that publishes them (`kernel/app/execprofile`), and the
  operation's copy was filled in **field by field** (thirty-seven field
  copies).
  They are declared **once** now, in the module: the operation's `Profile`,
  `Check`, `SecretPolicy`, `ListOutput` and `CheckOutput` are aliases of the
  module's `Profile`, `HealthCheck`, `SecretPolicy`, `Inventory` and
  `HealthReport`. The module's rows carry every field (the optional ones lost
  their `omitempty`), the profile row keeps the operator's key order (the
  secret policy last), and the operation keeps only the list rule — an empty
  list is `null`, an empty profile list is an empty array — instead of
  rebuilding the row. The module's rows were never marshalled, so the
  `omitempty` change is invisible outside the module.
  **No wire changed**: the same keys in the same order with the same values,
  the unset lists stay `null` and the empty profile list stays an empty array.
  14 mutations, 14 caught. The mutation run found a **test gap** — the
  operation's fixture exercised the null rule for the profile's tools and
  notes only — and it is fixed and named. The archcheck allowlist stays at 129
  import violations and 13 call sites; the package count stays at 271 (no new
  package).
- W3.1 (the reasoning module) is committed on `main` as a code commit plus a
  docs commit and published from `arch/w2-dispatch` the same way. The Council
  of Elders, the Conductor and the deep-research harness were three features
  of the composition root — `*runtime.Kernel` methods (`k.Council`,
  `k.Conduct`, `k.Research`) over ten files in `kernel/runtime`. They are now a
  module (`kernel/modules/reasoning`, L3) and the ten files moved out of the
  runtime. The module is **stateless**: everything it needs arrives through
  explicit ports (`api.Dependencies` — the aux-completion funnel, the
  provider's presence, the governed tool invoker, the tool lookup, the event
  bus, the default panel, the web-grounding flag and the sandboxed code
  runner), so every call keeps the host's policy checks, its audit arc and its
  spend attribution. A module may import another module's **api** package and
  nothing else (P5), and the runtime is a module too, so the two sides meet in
  exactly one place: `kernel/modules/reasoning/api` (new) holds the rows, the
  shared helpers, the ports and the module's surface, and
  `kernel/runtime/reasoning.go` (new) keeps the runtime's public names as
  **aliases** of the module's rows and delegates the three calls to the
  injected service. The composition root (`cmd/agezt`'s `openAppKernel`) builds
  the service over the kernel's ports and injects it, so the primary kernel
  and every tenant kernel are wired.
  **No behaviour changed**: the same error texts, the same event subjects and
  kinds, the same token budgets, the same audit arcs and the same wire JSON.
  23 mutations, 23 caught. The mutation run found a **test gap** — the
  council's default deliberation round was not asserted — and it is fixed and
  named. The archcheck allowlist stays at 129 import violations and 13 call
  sites; the package count grows by two (271 → 273).
- W3.2 (the knowledge module) is committed on `main` as a code commit plus a
  docs commit and published from `arch/w2-dispatch` the same way. The knowledge
  module was already a module — the layer map named `kernel/memory`,
  `kernel/worldmodel` and `kernel/taste` as layer 3 in the module `knowledge` —
  but the packages sat at the top of `kernel/`, outside the `kernel/modules/`
  tree W3 establishes as the target home of the bounded contexts. They moved to
  `kernel/modules/knowledge/{memory,worldmodel,taste}`. The package names do not
  change, so the move is an import-path change and nothing else (68 files
  repointed); the placement is now *derived* (`kernel/modules/*/...` → module
  `knowledge`) and the three hand-written layer rules are gone; the 13
  allowlist entries moved with their edges, so the ratchet is unchanged.
  The roadmap's second ask — the profile-facet supersede bug — is fixed: a
  memory record's identity is its CONTENT, so a facet whose synthesized wording
  changed was written under a new id and the previous version stayed active
  forever, leaving the operator with two conflicting versions of the same facet
  injected into every run while the package doc promised "updated across passes
  (never duplicates)". `DistillProfile` now retires the previous version before
  writing the new one (the superseded record keeps its audit trail), only the
  shared facets are candidates, and an absent record reports the named
  `memory.ErrRecordNotFound` instead of silently succeeding. The old
  `supersede_bug_test.go` — a proof-of-failure that had outlived its bug — is
  now a regression test. 8 mutations, 8 caught; the run also produced an honest
  finding (the scope guard in the supersede loop was unreachable, so it was
  deleted and the invariant it protected is now asserted where it is
  enforced). Nothing else changed: the same packages, the same exported API,
  the same records on disk, the same events and the same injection order.
- W3.3 (the skills module) is committed on `main` as a code commit plus a
  docs commit and published from `arch/w2-dispatch` the same way. The Forge
  was already a module — the layer map named `kernel/skill` as layer 3 in the
  module `skills` — but the package sat at the top of `kernel/`, outside the
  `kernel/modules/` tree W3 establishes as the target home of the bounded
  contexts; it moved to `kernel/modules/skills/skill`. The package name does
  not change, so the move is an import-path change and nothing else (59 files
  repointed). The 16 embedded agentskills.io bundles were a **plugin**
  (`plugins/builtinskills`) — the module's own seed data lived outside the
  module, and the plugin reached back into the module to seed it; they are
  now in the module too: the bytes in `kernel/modules/skills/builtins` and the
  seeding in `kernel/modules/skills/skill/builtins.go`, over a `ForgeAPI`
  interface so the failure path is testable. The placement is now *derived*
  (`kernel/modules/*/...` → module `skills`) and the hand-written layer rule
  is gone; nine allowlist entries moved with their edges and one was deleted
  with the plugin, so the ratchet **drops** from 129 to 128.
  The roadmap's ask — stop re-promoting operator-quarantined built-ins — is
  fixed: the seeder promoted every built-in unconditionally, and
  `quarantined → active` is a legal edge, so an operator's quarantine (and
  the automatic regression quarantine) was undone by the daemon's own seeding
  on the next boot; the seeder now stops at a quarantined or archived
  built-in and reports the status it left the skill in. The move also broke
  two live references **silently**, and both are fixed: the browser action's
  driver lookup still pointed at the old bundle path (its e2e test skips
  when the driver is missing, so nothing failed) and `.gitattributes` no
  longer forced LF on the embedded bundles, which a Windows checkout would
  have embedded as CRLF. 17 mutations, 16 caught, and the seventeenth is
  recorded as an **equivalent** (the `archived` half of the guard is
  defensive — the transition matrix already refuses `archived → active`). The
  run also produced honest findings about the **tests** — the seeding's
  failure path and the module's public seam were untested — and they are
  fixed and named. Nothing else changed: the same package name, the same
  exported API, the same records on disk, the same events and the same
  injection into a run.
- W2.45a (typed roster status collection) is committed on `main` as a code
  commit plus a docs commit and published from `arch/w2-dispatch` the same way.
  `kernel/controlplane/roster_status.go` (371 lines) is gone: the journal-derived
  half of the roster status collection moves to
  `kernel/app/roster/status_sources.go`, the run-row status precedence moves to
  `roster.RunRow.Status()`, and the control plane's `runEntryStatus`,
  `firstNonEmpty` and `agentModelChain` leave the control plane (the model
  chain is now `approster.AgentModelChain`, shared with the run resolution).
  The wake views, the escalation views, the repair summaries and the reaper
  mapping stay native for now — they read host stores and are the next slice
  of the same domain.
- W2.45b (typed roster status stores) is committed on `main` as a code commit
  plus a docs commit and published from `arch/w2-dispatch` the same way.
  `kernel/controlplane/roster_status_wake.go` (107 lines),
  `roster_status_escalation.go` (73 lines) and `roster_repair_summary.go`
  (98 lines) are gone: the store-backed half of the roster status collection
  moves to `kernel/app/roster/status_stores.go` (329 lines) — the wake views,
  the escalation views, the repair summaries, the reaper mapping and the
  assembly (`approster.CollectStatus`). `StatusSources` grows the schedules,
  the standing orders, the board, the reaper scan and the repair cooldown as
  ports; the host keeps the stores and the board's `OpenHelp` read. The
  doctor's repair events now share the single journal walk (two walks became
  one), the cooldown's environment reading moves to
  `approster.AutoRepairCooldown` (the typed `roster repair status` operation
  binds it), and the four native type aliases over the app status rows plus
  the `plInt` payload helper they kept alive leave the control plane. The
  archcheck allowlist stays at 129 import violations and 13 call sites.

### W2.30c summary

`agent_list` is a primary-only ReadOnly shared operation over the app ListService;
the native wrapper/manual row are removed. Concrete ListOutput/ProfileOutput/
StatusOutput (81 typed status fields, 18 required) keep the full lower Profile,
legacy float64 profile/cursor projection and exact status integers. An explicit-null
drift in the partial slice (schedule-only wake rows emit `"wake_event_subjects":null`)
was found and fixed. 528 native cases x20 (440 raw-byte equal, 88 intentional
already-canceled admissions), 29/29 mutations and the full gates pass. Details:
[53-w230-typed-agent-list-evidence.md](53-w230-typed-agent-list-evidence.md);
harness scripts/logs under `.temp_files/architecture-delivery/w53/`.

W2.30d moved `agent_graveyard` the same way (GraveyardService over roster list +
clock, no HTTP route; 468 native cases x20, 22/22 mutations):
[54-w230-typed-agent-graveyard-evidence.md](54-w230-typed-agent-graveyard-evidence.md),
harness under `.temp_files/architecture-delivery/w54/`.

W2.30e moved the per-event activity text unchanged into `approster.ActivitySummary`
(2M-event parity, 21/21 mutations):
[55-w230-activity-summary-move-evidence.md](55-w230-activity-summary-move-evidence.md),
harness under `.temp_files/architecture-delivery/w55/`. W2.30f then bound
`agent_activity` as a typed operation (792 native cases x20, 23/23 mutations):
[56-w230-typed-agent-activity-evidence.md](56-w230-typed-agent-activity-evidence.md),
harness under `.temp_files/architecture-delivery/w56/`. W2.30g bound
`agent_repair_status` and fixed a real bug: the legacy in-place cursor filter
(`rows[:0]`) overwrote the full row list, so `latest`/`next_eligible_ms`/`next_action`
moved with the page (native regression red 3/3, now green):
[57-w230-typed-agent-repair-status-evidence.md](57-w230-typed-agent-repair-status-evidence.md),
harness under `.temp_files/architecture-delivery/w57/`. W2.30h bound
`agent_escalations` (756 native cases x20 on a shared board, 28/28 mutations):
[58-w230-typed-agent-escalations-evidence.md](58-w230-typed-agent-escalations-evidence.md),
harness under `.temp_files/architecture-delivery/w58/`. All roster reads except
`agent_impact`/`agent_tombstone` (cascade) and `agent_resolve` are now typed. W2.30i fixed an
audit gap: the mutating `agent_resolve` was registered `ReadOnly`, so dispatch never
journaled it (regression red 3/3 → green 20/20):
[59-w230-agent-resolve-audit-evidence.md](59-w230-agent-resolve-audit-evidence.md).
Next: the lifecycle writes, with typed `agent_resolve` among them (set_enabled,
retire/revive, add/edit, task_update, wake, repair, resolve, remove + cascade with
impact/tombstone). Writes need the mandatory-audit admission pattern of the W2.27
writer slices. W2.30j moved
`agent_set_enabled` that way; its harness (cloned kernels, response + full journal
delta parity) is the template for the remaining writes:
[60-w230-typed-agent-set-enabled-evidence.md](60-w230-typed-agent-set-enabled-evidence.md),
`.temp_files/architecture-delivery/w60/`. W2.30k moved `agent_add`/`agent_edit`
the same way (114 cloned-kernel steps x20, 27/27 mutations; owner-before-parent
validation replaces random map order):
[61-w230-typed-agent-add-edit-evidence.md](61-w230-typed-agent-add-edit-evidence.md).
W2.30l moved `agent_task_update` (192 cloned-kernel steps x20 with raw-byte and
journal parity, 31/31 mutations; the host keeps the legacy `AgentTask` member order,
and missed tasks still journal the profile edit as before):
[62-w230-typed-agent-task-update-evidence.md](62-w230-typed-agent-task-update-evidence.md),
`.temp_files/architecture-delivery/w62/`. The w62 harness also compares normalized
raw response bytes; the earlier decoded-only comparison would not catch member order.
W2.30m moved `agent_wake` (138 cloned-kernel steps x20 with scripted mock providers,
36/36 mutations). The run stays native behind a non-blocking launch port. The w63
harness compares the journal grouped by correlation id, because the async run
interleaves with the audit tail; reuse it for `agent_repair`/`agent_resolve`:
[63-w230-typed-agent-wake-evidence.md](63-w230-typed-agent-wake-evidence.md),
`.temp_files/architecture-delivery/w63/`. W2.30n moved `agent_repair` the same way
(120 steps x20, 30/30 mutations; the governed repair stays native in
`runAgentRepair`, and a new test proves its WF-001 panic firewall; wake and repair
share `directTarget` and `IncidentLineage`):
[64-w230-typed-agent-repair-evidence.md](64-w230-typed-agent-repair-evidence.md).
W2.30o moved `agent_resolve` (222 steps x20 with journal, board and routing-chain
parity, 52/52 mutations). The app service owns the decision; native effects come in
through `ResolvePorts`:
[65-w230-typed-agent-resolve-evidence.md](65-w230-typed-agent-resolve-evidence.md),
`.temp_files/architecture-delivery/w65/`. Lesson from its harness: assert that each
success path really ran (here that a routing change happened and a delegation was
posted), or an identical failure on both sides passes as parity.
W2.30p moved the two cascade reads `agent_impact`/`agent_tombstone` (120 steps x20
on a fully populated three-level tree with retire included, 36/36 mutations):
[66-w230-typed-agent-impact-tombstone-evidence.md](66-w230-typed-agent-impact-tombstone-evidence.md),
`.temp_files/architecture-delivery/w66/`. Retire builds its summary through the
app (`agentImpactResult` wrapper), and remove builds its retained sub-agent
workflow labels the same way (`subagentImpact`). W2.30q moved `agent_retire`/`agent_revive` (102 steps x20 on the w66 rich fixture,
29/29 mutations plus one recorded equivalent):
[67-w230-typed-agent-retire-revive-evidence.md](67-w230-typed-agent-retire-revive-evidence.md),
`.temp_files/architecture-delivery/w67/`. W2.30r moved `agent_remove` (84 steps
x20 with response, journal and full post-run state parity, 36/36 mutations):
[68-w230-typed-agent-remove-evidence.md](68-w230-typed-agent-remove-evidence.md),
`.temp_files/architecture-delivery/w68/`. **The roster domain is complete:** every
command of the former native roster group is a typed shared operation and
`registerRosterCommands` is gone. Two agent-adjacent native commands remain in the
misc registry group (`agent_permissions`, `agent_capabilities`); classify them with
their own domain.

W2.31a moved the five live run steering commands into the new `kernel/app/steer`
(222 steps x20 on cloned kernels with live blocked runs in the primary and a tenant
kernel, 33/33 mutations plus one recorded equivalent):
[69-w231-typed-steer-evidence.md](69-w231-typed-steer-evidence.md),
`.temp_files/architecture-delivery/w69/`. They are the first tenant-routed writes
of order 7. Their harness is the template for the remaining tenant-routed run
controls: compare both kernels' journals grouped by correlation, and expect the
W2.1 tenant label on operator-selected tenant audit records (the legacy native
audit omitted it for the primary token). If a long `-count=20` run is killed by
the timeout while blocked in `FlushFileBuffers`, the host suspended; rerun it.
W2.31b moved `cancel_run` into the same package (156 steps x20, 9/9 mutations) and
fixed the caller-tenant audit label: the adapter now passes the trimmed tenant:
[70-w231-typed-cancel-run-evidence.md](70-w231-typed-cancel-run-evidence.md),
`.temp_files/architecture-delivery/w70/`. Invalid tenant ids fail native routing
before any argument check, on both paths.
W2.31c moved `runs_list`/`runs_stats` into the new `kernel/app/runs` (288 steps
x20 byte-exact, 47/47 mutations); native `collectRuns` stays as the shared
snapshot port: [71-w231-typed-run-reads-evidence.md](71-w231-typed-run-reads-evidence.md),
`.temp_files/architecture-delivery/w71/`. For a deterministic same-millisecond
journal in tests, open `journal.Open(dir, journal.Options{Now: fixed})` first and
then the kernel on the same base dir.
W2.32a moved `journal_head`/`journal_tail`/`journal_stats` into the new
`kernel/app/journal` (132 steps x20 byte-exact, 26/26 mutations plus one recorded
equivalent): [72-w232-typed-journal-reads-evidence.md](72-w232-typed-journal-reads-evidence.md),
`.temp_files/architecture-delivery/w72/`. Two reusable lessons: a typed output
holding `event.Event` needs an explicit schema (derive it from a wire mirror with
`Payload any`), and a legacy handler that embedded structs needs an adapter
special case to keep their member order. The rest of the journal group
(`journal_grep`, `journal_export`, `changelog`, `cache_stats` and the folded
audit logs) follows.
W2.32b moved `journal_grep`/`journal_export` (216 steps x20 byte-exact, 35/35
mutations plus one recorded equivalent):
[73-w232-typed-journal-search-evidence.md](73-w232-typed-journal-search-evidence.md),
`.temp_files/architecture-delivery/w73/`. Next in the journal group: `changelog`,
`cache_stats` and the journal-folded audit logs (approvals, netguard, edict,
ratelimit, warden logs and stats).
W2.32c moved `changelog`/`cache_stats` (174 steps x20 byte-exact):
[74-w232-typed-changelog-cache-evidence.md](74-w232-typed-changelog-cache-evidence.md),
`.temp_files/architecture-delivery/w74/`. **Harness lesson:** journal crafted
payloads as `json.RawMessage` — a Go `[]byte` in `event.Spec.Payload` is stored
as a base64 string, so every payload-derived field silently goes empty on both
sides. Assert that payload-derived values reach the response. W2.31c and W2.32b
were re-proven with the fix.
W2.32d moved the guard audit reads (`netguard_log`, `ratelimit_log`/`_stats`,
`warden_log`/`_stats`) into the new `kernel/app/audit` (240 steps x20
byte-exact, 36/36 mutations): [75-w232-typed-guard-audit-evidence.md](75-w232-typed-guard-audit-evidence.md),
`.temp_files/architecture-delivery/w75/`. The journal group's remaining logs move
with their domains: `edict_log`/`edict_stats` (the last `projectJournal` user)
with edict, `approvals_log`/`approvals_stats` with approvals. Order 7 then
continues with edict, tenant, shutdown and remote.
W2.33a moved the edict reads (`edict_show`, `edict_deny_list`, `edict_test`) into
the new `kernel/app/edict` (162 steps x20 byte-exact, 25/25 mutations):
[76-w233-typed-edict-reads-evidence.md](76-w233-typed-edict-reads-evidence.md),
`.temp_files/architecture-delivery/w76/`. Policy state is per-kernel runtime
state, so the harness applies identical engine changes to every clone after it
opens.
W2.33b moved the four audited edict writes (`edict_deny_add`/`_rm`,
`edict_set_level`/`_mode`, each journaling `policy.changed`) into the same
package; `edict.go`, `edict_deny.go`, `edict_set.go`, `edictFor` and
`askPolicyLabel` are gone (312 steps x20: byte-exact responses, grouped journals
and final engine state equal; 36/36 mutations):
[77-w233-typed-edict-writes-evidence.md](77-w233-typed-edict-writes-evidence.md),
`.temp_files/architecture-delivery/w77/`. A write harness must compare final
engine state too, and for canceled authorized steps assert the typed side kept
the starting policy (legacy writes ignored the context). Next:
`edict_log`/`edict_stats` (policy_log.go, the last `projectJournal` user), then
`edict_overlay`/`edict_compact`, approvals, tenant, shutdown and remote.
W2.33c moved `edict_log`/`edict_stats` into `app/edict` (`Decisions`, over the
routed journal + clock) and retired `policy_log.go` and `projections.go`: no
native `projectJournal` caller remains (306 steps x20 byte-exact, 37/37
mutations):
[78-w233-typed-edict-decisions-evidence.md](78-w233-typed-edict-decisions-evidence.md),
`.temp_files/architecture-delivery/w78/`. Next: `edict_overlay`/`edict_compact`
(edict_overlay.go), then `approvals_log`/`approvals_stats` (approvals_log.go,
the last `sinceCutoff` user), then tenant, shutdown and remote.
W2.33d moved `edict_overlay`/`edict_compact` into `app/edict` (`Overlay`, over the
routed journal, snapshot file and bus); both stay audited, and compaction stays
primary-only. The edict family is complete: no native edict handler remains
(102 steps x20 byte-exact with equal grouped journals and snapshot files,
29/29 mutations):
[79-w233-typed-edict-overlay-evidence.md](79-w233-typed-edict-overlay-evidence.md),
`.temp_files/architecture-delivery/w79/`. Next: `approvals_log`/`approvals_stats`
(approvals_log.go, the last `sinceCutoff` user; a new `kernel/app/approvals`
read package or `app/audit`), then tenant, shutdown and remote.
W2.34a moved `approvals_log`/`approvals_stats` into the new `kernel/app/approvals`
(`History`, over the routed journal + clock) and removed `approvals_log.go`,
`registerJournalLogCommands`, `sinceCutoff` and `int64Arg` (216 steps x20
byte-exact, 40/40 mutations):
[80-w234-typed-approval-history-evidence.md](80-w234-typed-approval-history-evidence.md),
`.temp_files/architecture-delivery/w80/`. The only native journal-folded audit
log left is `plan_history`/`plan_stats` (plan_history.go). Next: the live
approval operations (`approvals` pending list and decide, in
`server_handle_run_remote.go`) into `app/approvals`, then `plan_history`,
tenant, shutdown and remote.
W2.34b moved `approvals`/`decide` into `app/approvals` (`Live`, over the primary
approval registry); the approvals domain is complete (114 steps x20 with live
waiting requests, ids/times normalized, grouped journals equal; 23/23
mutations): [81-w234-typed-live-approvals-evidence.md](81-w234-typed-live-approvals-evidence.md),
`.temp_files/architecture-delivery/w81/`. Next: `plan_history`/`plan_stats`
(plan_history.go, the last native journal-folded audit log), then tenant,
shutdown and remote.
W2.35a moved `plan_history`/`plan_stats` into `app/runs` (`Plans`, over the routed
journal); no native journal-folded audit log remains (180 steps x20 byte-exact,
35/35 mutations; start-less plans now tie-break by correlation instead of an
arbitrary `sort.Slice` order):
[82-w235-typed-plan-history-evidence.md](82-w235-typed-plan-history-evidence.md),
`.temp_files/architecture-delivery/w82/`. Next: tenant (`tenant_*`), shutdown and
remote, then agent_permissions/agent_capabilities classification.
W2.36a moved the six `tenant_*` operations into the new `kernel/app/tenants`
(Registry + Activity ports; `tenant_stats` stays CallerTenant so routing still
resolves a named tenant first; a disabled registry binds as a real nil): 222
steps x20 byte-exact with registry state and grouped journals equal, 31/31
mutations: [83-w236-typed-tenants-evidence.md](83-w236-typed-tenants-evidence.md),
`.temp_files/architecture-delivery/w83/`. Next: `shutdown`, the remote
operations, then agent_permissions/agent_capabilities classification.
W2.37a moved halt/resume/journal_verify/shutdown into `app/system` (`Lifecycle`;
shutdown acknowledges and the native `scheduleShutdown` keeps the grace delay):
108 steps x20 with halt/exit state and grouped journals equal, 16/16 mutations
(+1 recorded timing equivalent):
[84-w237-typed-lifecycle-evidence.md](84-w237-typed-lifecycle-evidence.md),
`.temp_files/architecture-delivery/w84/`. "remote" in order 7 is not a command:
it is the remote-mirror helper code under the streaming `run` path. Next: `why`
and `whoami` (server_commands.go), then agent_permissions/agent_capabilities
classification, then the streaming `run`/`plan` family.
W2.37b moved `why` into `app/journal` (`Trace` over a `Tracer` port; OwnTenant +
CallerTenant, both event lists in member order): 120 steps x20 byte-exact on
journals with a cross-correlation causation chain, 16/16 mutations (one
member-order gap closed, +1 recorded equivalent):
[85-w237-typed-why-evidence.md](85-w237-typed-why-evidence.md),
`.temp_files/architecture-delivery/w85/`. `whoami` stays native by design (typing
it would route the tenant it only names).
W2.38a moved `agent_permissions`/`agent_capabilities` into `app/roster`
(`PermissionService` over primary-kernel ports; the config-ownership predicate
is now shared with teardown): 294 steps x20 with roster state and grouped
journals equal, 62/62 mutations:
[86-w238-typed-agent-permissions-evidence.md](86-w238-typed-agent-permissions-evidence.md),
`.temp_files/architecture-delivery/w86/`. No native command of the former misc
group remains.
W2.39a moved `state_list`/`state_get` into the new `kernel/app/state` (204 steps
x20 byte-exact, 15/15 mutations):
[87-w239-typed-state-evidence.md](87-w239-typed-state-evidence.md),
`.temp_files/architecture-delivery/w87/`. W2.39b moved `disk_stats` into
`app/storage` (144 steps x20 byte-exact under six probes, 14/14 mutations):
[88-w239-typed-disk-stats-evidence.md](88-w239-typed-disk-stats-evidence.md),
`.temp_files/architecture-delivery/w88/`. W2.39c moved `redact_test` into the
new `kernel/app/redaction` (216 steps x20 byte-exact under three redactor
states, 14/14 mutations):
[89-w239-typed-redaction-evidence.md](89-w239-typed-redaction-evidence.md),
`.temp_files/architecture-delivery/w89/`. W2.39d moved `reaper_scan` into the
new `kernel/app/reaper` (60 steps x20 byte-exact over every finding family the
fixture can raise, 24/24 mutations):
[90-w239-typed-reaper-evidence.md](90-w239-typed-reaper-evidence.md),
`.temp_files/architecture-delivery/w90/`. W2.39e moved `spend_today`/`attention`
into the new `kernel/app/missioncontrol` (450 steps x20 over five fixtures
including a governor with real spend, 27/27 mutations):
[91-w239-typed-mission-control-evidence.md](91-w239-typed-mission-control-evidence.md),
`.temp_files/architecture-delivery/w91/`. W2.39f moved the three sandbox commands
into the new `kernel/app/sandbox` and `confineUnder` into
`platform/fileworkspace` (186 steps x20 with trees and journals equal, 31/31
mutations, three gaps closed):
[92-w239-typed-sandbox-evidence.md](92-w239-typed-sandbox-evidence.md),
`.temp_files/architecture-delivery/w92/`. The daemon-ops group is complete.
W2.40a moved `budget`/`budget_set` into the new `kernel/app/budget` (504 steps
x20 with journals equal, including the governor's ceiling-set events, 32/32
mutations): [93-w240-typed-budget-evidence.md](93-w240-typed-budget-evidence.md),
`.temp_files/architecture-delivery/w93/`. W2.40b moved `routing_get`/`routing_set`
and `chains_get`/`chains_set` into the new `kernel/app/routing` (558 steps x20
with config stores and journals equal, 57/57 mutations):
[94-w240-typed-routing-evidence.md](94-w240-typed-routing-evidence.md),
`.temp_files/architecture-delivery/w94/`. W2.40c moved the three execution-profile
reads into the new `kernel/app/execprofile` (264 steps x20 with tenant routing,
35/35 mutations, one gap closed):
[95-w240-typed-execution-profile-evidence.md](95-w240-typed-execution-profile-evidence.md),
`.temp_files/architecture-delivery/w95/`. The provider-config group is complete.
W2.41a moved the seven data lake commands into the new `kernel/app/data` (276
steps x20, 44/44 mutations, five gaps closed):
[96-w241-typed-data-lake-evidence.md](96-w241-typed-data-lake-evidence.md),
`.temp_files/architecture-delivery/w96/`. The datalake group is complete.
W2.42a moved `persona_get`/`persona_set` and `prompts_get`/`prompts_set` into the
new `kernel/app/persona` (378 steps x20, 31/31 mutations):
[97-w242-typed-persona-evidence.md](97-w242-typed-persona-evidence.md),
`.temp_files/architecture-delivery/w97/`. W2.42b moved the three seat commands
into the new `kernel/app/seats` (132 steps x20, 21/21 mutations):
[98-w242-typed-seat-evidence.md](98-w242-typed-seat-evidence.md),
`.temp_files/architecture-delivery/w98/`. W2.42c moved `council_members` and
`council_set` into the new `kernel/app/council` (288 steps x20, 28/28 mutations):
[99-w242-typed-council-membership-evidence.md](99-w242-typed-council-membership-evidence.md),
`.temp_files/architecture-delivery/w99/`. W2.42d moved `reflect_run` and
`reflect_show` into the new `kernel/app/reflection` (72 steps x20, 12/12
mutations): [100-w242-typed-reflection-evidence.md](100-w242-typed-reflection-evidence.md),
`.temp_files/architecture-delivery/w100/`. W2.42e added `conductor_roles` to
`kernel/app/council` (90 steps x20, 9/9 mutations):
[101-w242-typed-conductor-roles-evidence.md](101-w242-typed-conductor-roles-evidence.md),
`.temp_files/architecture-delivery/w101/`. W2.42f moved `chat_suggestions`
into the new `kernel/app/chat` (432 steps x20, 14/14 mutations):
[102-w242-typed-chat-suggestions-evidence.md](102-w242-typed-chat-suggestions-evidence.md),
`.temp_files/architecture-delivery/w102/`. W2.42g moved `node_registry` into
the new `kernel/app/nodes` (216 steps x20, 25/25 mutations):
[103-w242-typed-node-registry-evidence.md](103-w242-typed-node-registry-evidence.md),
`.temp_files/architecture-delivery/w103/`. W2.43 moved the four file commands
into `kernel/app/files`; the shared adapter now carries a domain `error_code`
and the request id (672 steps x20, 20/20 mutations):
[104-w243-typed-file-group-evidence.md](104-w243-typed-file-group-evidence.md),
`.temp_files/architecture-delivery/w104/`. W2.44a moved `chat_summarize` into
`kernel/app/chat`, the first typed LIVE operation that never streams (90 steps
x20, 15/15 mutations):
[105-w244-typed-chat-summarize-evidence.md](105-w244-typed-chat-summarize-evidence.md),
`.temp_files/architecture-delivery/w105/`. W2.44b moved the live cognition pair
into `kernel/app/council` — `council_ask` and `conductor_ask`, the second typed
LIVE operations that never stream (15 mutations, one gap closed: the opinion
row's seat and model):
[106-w244-typed-council-ask-evidence.md](106-w244-typed-council-ask-evidence.md).
W2.44c moved the deep-research harness into `kernel/app/research`, the third
typed LIVE operation that never streams, and moved the lenient flag, count and
text readers and the correlation sanitizer into `kernel/app/lenient.go`, so the
control plane's own arg readers are gone:
[107-w244-typed-research-ask-evidence.md](107-w244-typed-research-ask-evidence.md).
W2.44d moved the plan authoring surface into `kernel/app/runs` — `plan_generate`
and `plan_refine`, the fourth and fifth typed LIVE operations — and with the
native handlers gone the control plane's last direct import of `kernel/planner`
is gone too, so the archcheck allowlist tightens to 130 import violations:
[108-w244-typed-plan-authoring-evidence.md](108-w244-typed-plan-authoring-evidence.md).
W2.44e moved the plan execution into the same package — the first typed
operation that really streams the plan's own event frames — and with the
native handlers gone the control plane's last direct import of
`kernel/scheduler` is gone too, so the allowlist tightens to 129:
[109-w244-typed-plan-execution-evidence.md](109-w244-typed-plan-execution-evidence.md).
Two native commands remain: `run` and `whoami`; `whoami` stays native by
design (it echoes the transport principal). Next: the streaming
cognition commands,
the roster status source collection and the streaming `run` family.
W2.44f moved the streaming `run` family — the run itself, its resolution, its
execution and its dry-run plan — into `kernel/app/runs`, the last native
handler of the streaming family, and with it the strict argument readers
(`argBool`, `argInt64`, `argStringList`, `parseCapList`) and `argTruthy` left
the control plane, and the execution-profile inventory builder moved to
`kernel/app/execprofile/inventory.go`:
[110-w244-typed-run-execution-evidence.md](110-w244-typed-run-execution-evidence.md).
One native command remains: `whoami`, which stays native by design.
W2.45a moved the roster's journal-derived status collection into
`kernel/app/roster` — the single journal walk that fills the live run
statuses, the last activity, the last autonomy runbook, the mailbox wakes,
the policy denials and the routing/retry pressure, plus the live wake
context, the live event summary and the routing-profile match. The
application now owns what a run row MEANS: `roster.RunRow.Status()` carries
the terminal precedence that the control plane's `runEntryStatus` used to
carry, so the roster status, the run reads and the run operation cannot
disagree:
[111-w245-typed-roster-status-collection-evidence.md](111-w245-typed-roster-status-collection-evidence.md).
W2.45b moved the rest of the same collection into `kernel/app/roster` — the
wake views, the escalation views, the repair summaries, the reaper mapping and
the assembly — so the roster status collection is now wholly in the
application layer. The doctor's repair events share the single journal walk
(two walks became one), the repair cooldown's environment reading moved to
`approster.AutoRepairCooldown`, and the four native type aliases over the app
status rows and the `plInt` helper they kept alive are gone:
[112-w245-typed-roster-status-stores-evidence.md](112-w245-typed-roster-status-stores-evidence.md).
W2.46 moved the operator wake and repair LAUNCHES into the same package — the
typed operations were already app-owned, but the launch they handed their
work to was native, and the launch is where the decisions live: the retry
decision, the cost ceiling, the terminal event's shape and the panic firewall
around a governed repair. The host keeps the run, the repair source and the
publisher:
[113-w246-typed-operator-launch-evidence.md](113-w246-typed-operator-launch-evidence.md).
W2.47 moved the operator resolution's journal reads into the same package —
the exhausted-chain lookup and the force-generation lookup, with the
incident-lineage matching — and with them the native `operatorWakeLineage`
transport type:
[114-w247-typed-operator-resolve-journal-evidence.md](114-w247-typed-operator-resolve-journal-evidence.md).
W2.48 moved the agent teardown's orchestration into the same package — which
standing orders and schedules belong to the agent, and what a teardown
retires, pauses or removes — and with it the last native wrappers:
[115-w248-typed-agent-teardown-orchestration-evidence.md](115-w248-typed-agent-teardown-orchestration-evidence.md).
W2.49 moved the teardown's state cleanup into the same package — the memory,
skill, config-center and workspace cleanup — with the filesystem behind a port
so the safety decision stays in the application layer:
[116-w249-typed-agent-teardown-state-evidence.md](116-w249-typed-agent-teardown-state-evidence.md).
W2.50 moved the impact derivation into the same package — which labels belong
to the agent, how a mailbox message reads, whether a workflow node references
the agent and which agents are its sub-agents:
[117-w250-typed-impact-cascade-evidence.md](117-w250-typed-impact-cascade-evidence.md).
W2.51 moved the workspace helpers into the same package — which path is safe
to look at, what the label says and where the workspace root is — and with it
the roster domain is done:
[118-w251-typed-workspace-helpers-evidence.md](118-w251-typed-workspace-helpers-evidence.md).
Next: the remaining order-7 domains — steer, journal, edict, tenant, shutdown
and remote. (Since then the whole protocol command registry has become typed
and the remote mirror's decisions have moved to the application layer; see
the W2.52 and W2.53 entries above.)

Follow-up recorded, not done: CI's `race-depth` stress runs
`./kernel/controlplane/...` with `-race -count=20 -timeout=20m`; that package
takes 790–930s on a healthy runner, and PR #764's first attempt hit the 20-minute
budget five times on a loaded runner (the rerun passed in 927s, with no
goroutine growth across iterations). As typed slices add binding tests, raise
the budget or shard the package before the margin closes.

The margin has closed: on 2026-10-19, PR #770's `race-depth` passed in 16m09s
and PR #771's run was still inside the same job after 25 minutes, retrying
under `scripts/ci-go-retry.sh` (up to five attempts, each with its own 20-minute
budget, so a single slow runner can hold the workflow for hours). The package
itself is healthy — a local `-count=20 -timeout=25m` run of
`./kernel/controlplane/...` passes, and the default 10-minute `go test` budget
is what fails it locally, not a test. The fix is on the CI side: raise the
`-timeout` to 30m, shard the eight packages into two jobs, or drop the count to
10 on the slow package. Until then, expect PR waits to be decided by the
runner, not by the change.

Follow-up recorded, not done: `validateOperationOutput` re-parses each declared
output schema per call (about 0.68 ms of a 1.5 ms fifty-agent `agent_list`); a
compile-once validator in `platform/schema` would benefit every migrated operation.

Preserve unrelated dirty changes: deletions of
`security-report/sc-auth-session-client-results.md` and
`security-report/sc-injection-results.md`, plus the stat-only
`kernel/controlplane/skill_app_test.go` entry. Exclude them from migration commits.

### Next

Continue roadmap §4.5/order 7 after the completed roster, steer, run-fold,
remote-mirror, operation-audit, run-derived-row, projection, warden,
shared-row-type, remote-backend-list, peer-wire-contract, artifact-row,
boundary-row, reaper-row and execution-profile-row domains: the application's
own artifact row and
the data-lake's records (their
own wires, their own spellings), the introspection tool's compact reaper view
(its own wire, for the model), the skill's metrics row (the store's row is the
on-disk format), the remote
backends' two **policies** (the
application supports them for a direct run, the host refuses them for a
dispatched seat), the wider run/tool adapter convergence, W3 modules and
journal raw-ref GC, W4 triggers/channel supervision/config/signing, and W5
generated SDK/frontend convergence and zero allowances remain open.

**W3.1 — the reasoning module (delivered).** The council, the conductor and the
research harness were three features of the composition root — `*runtime.Kernel`
methods over ten files in `kernel/runtime`. They are now a module
(`kernel/modules/reasoning`, L3) and the ten files moved out of the runtime:

1. The module is **stateless**. Everything it needs arrives through explicit
   ports (`api.Dependencies`): the aux-completion funnel, the provider's
   presence, the governed tool invoker, the tool lookup, the event bus, the
   default panel, the web-grounding flag and the sandboxed code runner. Every
   call therefore keeps the host's policy checks, its audit arc and its spend
   attribution.
2. A module may import another module's **api** package and nothing else (P5),
   and `kernel/runtime` is a module (`runs`) just like the reasoning module is.
   So the two sides meet in exactly one place: `kernel/modules/reasoning/api`
   (new) holds the rows, the shared helpers, the ports and the module's
   surface; `kernel/runtime/reasoning.go` (new) keeps the runtime's public
   names as **aliases** of the module's rows and delegates the three calls to
   the injected service.
3. The composition root builds the service: `cmd/agezt`'s `openAppKernel`
   calls `k.SetReasoning(reasoning.New(k.ReasoningDependencies()))`, so the
   primary kernel **and** every tenant kernel are wired.
4. The app layer names the module's vocabulary (`kernel/app/council`,
   `kernel/app/research` now import `kernel/modules/reasoning/api`). The
   **control plane keeps the kernel's public names**: an adapter may not import
   a module at all (archcheck's adapter-bypass rule), so it reaches the module
   through the kernel's API.
5. The plugins keep the kernel as their runner, unchanged — a plugin may not
   import a module either (plugin-reach is an allowlisted line with a fixed
   count).

**No behaviour changed**: the same error texts, the same event subjects and
kinds, the same token budgets, the same audit arcs and the same wire JSON. The
only new string is the host's guard for a kernel whose composition root forgot
the wiring ("runtime: reasoning module not wired"). 23 mutations, 23 caught;
the mutation run found a test gap (the council's **default** deliberation round
was not asserted) and it is fixed and named. The archcheck allowlist stays at
129 import violations and 13 call sites; the package count grows by two
(271 → 273).

**W3.2 — the knowledge module (delivered).** The module was already a module —
the layer map named `kernel/memory`, `kernel/worldmodel` and `kernel/taste` as
layer 3 in the module `knowledge` — but the packages sat at the top of
`kernel/`, outside the `kernel/modules/` tree W3 establishes as the target home
of the bounded contexts. They moved to `kernel/modules/knowledge/{memory,
worldmodel,taste}`; the package names do not change, so the move is an
import-path change and nothing else (68 files repointed). The placement is now
*derived* (`kernel/modules/*/...` → module `knowledge`) and the three
hand-written layer rules are gone; the 13 allowlist entries moved with their
edges, so the ratchet is unchanged.

The roadmap's second ask — the profile-facet supersede bug — is fixed. A memory
record's identity is its CONTENT, so a facet whose synthesized wording changed
was written under a new id and the previous version stayed active forever: the
operator got two conflicting versions of the same facet injected into every
run, while the package doc promised "updated across passes (never
duplicates)". `DistillProfile` now retires the previous version before writing
the new one (the superseded record keeps its audit trail), only the shared
facets are candidates, and an absent record reports the named
`memory.ErrRecordNotFound` instead of silently succeeding. The old
`supersede_bug_test.go` — a proof-of-failure that had outlived its bug — is now
a regression test. 8 mutations, 8 caught; the run also produced an honest
finding (the scope guard in the supersede loop was unreachable, so it was
deleted and the invariant it protected is now asserted where it is enforced).

**W3.3 — the skills module (delivered).** The Forge was already a module — the
layer map named `kernel/skill` as layer 3 in the module `skills` — but the
package sat at the top of `kernel/`, outside the `kernel/modules/` tree W3
establishes as the target home of the bounded contexts; it moved to
`kernel/modules/skills/skill`. The package name does not change, so the move
is an import-path change and nothing else (59 files repointed).

The 16 embedded agentskills.io bundles were a **plugin**
(`plugins/builtinskills`): the module's own seed data lived outside the
module, and the plugin reached back into the module to seed it. They are now
in the module too — the bytes in `kernel/modules/skills/builtins`, the
seeding in `kernel/modules/skills/skill/builtins.go`, over a `ForgeAPI`
interface so the failure path is testable. The `builtins` package does not
import the `skill` package (that would be a cycle), so the module's own seam
(`BuiltinNames`, `BuiltinBundle`) is what the marketplace plugin reads.

The placement is now *derived* (`kernel/modules/*/...` → module `skills`) and
the hand-written layer rule is gone; nine allowlist entries moved with their
edges and one was deleted with the plugin, so the ratchet **drops** from 129
to 128 (a plugin-reach edge fewer, because the plugin that reached into the
module no longer exists).

The roadmap's ask — stop re-promoting operator-quarantined built-ins — is
fixed. The seeder promoted every built-in to `active` on every boot, and
`quarantined → active` is a legal edge of the lifecycle matrix, so a built-in
the operator (or the regression guard) pulled out of production came straight
back into the retrieval pool on the next boot. The seeder now stops at a
quarantined or archived built-in, and the report it returns names the status
the skill was actually left in, so the boot log can explain the absence.

The move also broke two live references **silently**, and both are fixed: the
browser action's driver lookup still pointed at the old bundle path (its e2e
test skips when the driver is missing, so nothing failed) and `.gitattributes`
no longer forced LF on the embedded bundles, which a Windows checkout would
have embedded as CRLF.

17 mutations, 16 caught; the seventeenth is recorded as an **equivalent** (the
`archived` half of the guard is defensive — the transition matrix already
refuses `archived → active`, so the promote fails and the loop stops either
way). The run also made three test gaps **visible** — the seeding's failure
path (one bundle failing must not stop the others, and the *first* failure is
the one reported), the module's public seam and the SKILL.md leaking into the
bundle resources — and they are fixed and named; without the three new test
files, five of the seventeen mutations would have escaped.
- W3.4 (the artifacts module) is committed on `main` as a code commit plus a
  docs commit and published from `arch/w2-dispatch` the same way. The blob
  store and the Personal Data Lake were already a module — the layer map named
  `kernel/artifact` and `kernel/datalake` as layer 3 in the module `artifacts`
  — but both packages sat at the top of `kernel/`, outside the
  `kernel/modules/` tree W3 establishes as the target home of the bounded
  contexts; they moved to `kernel/modules/artifacts/{artifact,datalake}`. The
  package names do not change, so the move is an import-path change and nothing
  else (44 files repointed). The placement is now *derived*
  (`kernel/modules/*/...` → module `artifacts`) and the two hand-written layer
  rules are gone; the thirteen allowlist entries moved with their edges, so the
  ratchet stays at 128/13.
  The roadmap's second ask — GC respects the journal's `raw_ref` — was a
  **data-loss defect the codemap had already written down**. A tool output too
  large to inline is offloaded into the blob store and the journaled
  `tool.result` carries the blob's `raw_ref`; the collector removed the blob
  whenever no other *index entry* referenced it, never looking at the journal —
  so a collected run's output was gone while the journal still claimed the
  bytes were there, and `agt artifact_get <ref>` failed. The module cannot read
  the journal (a module sees another module's api package and nothing else), so
  it now **asks**: the index takes a `RefGuard` port, the host answers from the
  journal's own events (one pass per operation, the whole candidate batch at a
  time), the collector's candidates exclude the blobs the journal names, and an
  explicit delete of such an artifact is refused with `ErrStillReferenced` and a
  sentence the operator can act on. The protection is narrow: an artifact the
  journal never named is collected exactly as before.
  The cost is stated plainly in the evidence: the journal is append-only with
  full retention, so it names those refs forever — a journal-named artifact
  stays until the journal forgets it (freeing it needs a journal retention,
  named as future work), and the guard costs one journal pass per collect or
  delete (the boot path is untouched).
  11 mutations, 11 caught. Nothing else changed: the same on-disk layout, the
  same package names, the same exported API (three things added: the port, its
  setter and the sentinel), the same wire and the same events.
- W3.5 (the board module) is committed on `main` as a code commit plus a docs
  commit and published from `arch/w2-dispatch` the same way. The shared
  message board was already a module — the layer map named `kernel/board` as
  layer 3 in the module `board` — but the package sat at the top of `kernel/`,
  outside the `kernel/modules/` tree W3 establishes as the target home of the
  bounded contexts; it moved to `kernel/modules/board/board`. The package name
  does not change, so the move is an import-path change and nothing else (49
  files repointed). The placement is now *derived*
  (`kernel/modules/*/...` → module `board`) and the hand-written layer rule is
  gone; the six allowlist entries moved with their edges, so the ratchet stays
  at 128/13.
  The roadmap named no defect for this step, and the slice found one anyway:
  the store appended to its in-memory list and **then** saved, so a write that
  failed left the message on the board anyway — and the next successful save
  wrote it to disk — while at the cap the failed write had also dropped the
  oldest message from memory, which the next successful save made permanent.
  The board now persists the **prospective** state and adopts it in memory
  only once the write succeeded (the ack follows the same rule), so a failed
  write changes nothing: not the message, not the cap, not the reader's
  inbox.
  10 mutations, 10 caught. The run also found two honest gaps in the
  **tests** — the cap test asserted only the *count* (a cap that kept the
  oldest would have passed it), and the ack's failed-write path was not
  covered — and they are fixed and named. Nothing else changed: the same
  package name, the same exported API, the same messages, the same
  `board.json`, the same cap, the same inbox semantics, the same events and
  the same single-shared-store invariant.
- W3.6 (the work module) is committed on `main` as a code commit plus a docs
  commit and published from `arch/w2-dispatch` the same way. Five packages
  were already one module — the layer map named `kernel/workboard`,
  `kernel/okr`, `kernel/proof`, `kernel/assure` and `kernel/seat` as layer 3 in
  the module `work` — but all five sat at the top of `kernel/`, outside the
  `kernel/modules/` tree W3 establishes as the target home of the bounded
  contexts; they moved to `kernel/modules/work/{workboard,okr,proof,assure,seat}`.
  The package names do not change, so the move is an import-path change and
  nothing else (54 files repointed). The placement is now *derived*
  (`kernel/modules/*/...` → module `work`) and the five hand-written layer
  rules are gone; the eight allowlist entries moved with their edges, so the
  ratchet stays at 128/13.
  The roadmap's second ask — fix the `"container"` seat → profile mapping — was
  a **silently ignored seat**: a seat names the isolation *surface*
  ("container"), a run profile names the *implementation* ("docker"), and the
  dispatch handed the seat's value straight to the profile resolver — where
  "container" is not a run profile, so the resolver refused it and the dispatch
  **degraded**: the task ran with the agent's own isolation instead of the
  seat's. The two vocabularies are now translated once, where the seat's value
  becomes a run profile (`executionprofile.SeatProfileID`).
  6 mutations, 6 caught — and the mutations on the moved packages prove the
  suites still test the module after the move (the proof gate is not
  bypassable, a custom seat still wins over the built-in one). Nothing else
  changed: the same package names, the same exported APIs, the same records,
  the same proof gate, the same custom-first seat resolution, the same
  degradation behaviour for a genuinely unavailable profile, the same wire and
  the same events.

- W3.7 (the workflows module) is committed on `main` as a code commit plus a
  docs commit and published from `arch/w2-dispatch` the same way. The workflow
  engine was in two places and neither was the module home: the store, schema,
  validation, templating, template gallery and trigger runner sat at the top of
  `kernel/` (`kernel/workflow`), and the *engine* — the graph walk, the per-node
  dispatchers, the reliability wrapper, the copilot and the single-node test —
  sat in the composition root (`kernel/runtime/workflowrun*.go`,
  `workflowdraft.go`, `workflowtestnode.go`, `*runtime.Kernel` methods reaching
  into the kernel's own fields). Both moved to
  `kernel/modules/workflows/workflow`; the package name does not change, so the
  store move is an import-path change and nothing else (41 files repointed).
  The engine could not simply be repointed — a module may not import another
  module (P5) — so the module now holds **no provider, no tool registry, no bus
  and no sandbox of its own**, and every capability arrives through
  `workflow.Ports`. The runtime keeps its public names as delegations in
  `kernel/runtime/workflow_host.go`, and the run's provenance — which used to
  be read from the runtime's wake context, invisible to a module — now travels
  as an explicit `workflow.Provenance` the host fills in.
  The module is **one package**: the target architecture lists four homes and
  the archcheck ratchet forbids the split the runtime's own imports would cause
  (each extra package is one more cross-module edge, and the ratchet only
  tightens). That package split belongs to the slice that stops the runtime
  from importing the module at all.
  The roadmap's second ask — fix the trigger `source` labels — was a **silent
  trigger**: the trigger runner is the only place that knows what fired a
  workflow and it told the host only a human label, so the daemon's fire
  closure ran the workflow with no wake context and the provenance fell back
  to `"manual"` — every cron and event run in the journal looked like a human
  had pressed the button. The runner now hands the host what fired it
  (`Trigger{Source, Subject, Reason}`), the host maps it into the wake
  context, and a cron fire is journalled as `source:"cron"`, an event fire as
  `source:"event"` with the matched subject. A run nobody triggered is still
  `"manual"`.
  10 mutations, 10 caught (the first M09 run was a build failure, not an
  escape; it was re-run with a compiling anchor and caught). The move broke two
  live references and both are fixed: the OpenClaw/Hermes parity ledger
  (`cmd/agt/compare_data.go`) asserts that its evidence files exist and three
  rows still pointed at `kernel/workflow/workflow.go`, and `kernel/agent`'s
  `ValidateToolInput` forwarder lost its only caller (the engine's pipeline
  schema check now calls `kernel/platform/schema` directly, because a module
  may not import `kernel/agent`) and was removed. Nothing else changed: the
  same events, the same store file, the same node types, the same reliability
  semantics, the same errors, the same public API.

- W3.8 (the agents module) is committed on `main` as a code commit plus a
  docs commit and published from `arch/w2-dispatch` the same way. Two packages
  were already one module — the layer map named `kernel/roster` and
  `kernel/selfrepair` as layer 3 in the module `agents` — but both sat at the
  top of `kernel/`, outside the `kernel/modules/` tree W3 establishes as the
  target home of the bounded contexts; they moved to
  `kernel/modules/agents/{roster,selfrepair}`. The package names do not
  change, so the move is an import-path change and nothing else (161 files
  repointed). The placement is now *derived* (`kernel/modules/*/...` → module
  `agents`) and the two hand-written layer rules are gone.
  The roadmap's second ask — fix guardian reconcile resetting operator-lowered
  caps — was a **ceiling the daemon raised behind the operator's back**: the
  built-in fleet's reconcile assigned the built-in spend ceilings ($5/run,
  $10/day) outright on every boot, so an operator who tightened a guardian's
  budget got it silently raised back — the daemon undid the operator's own
  policy, and the guardian-budget guardian's own contract ("pause or lower
  max_daily_mc; never raise ceilings") was broken by the daemon itself. The
  package's own test already stated the rule ("the reconcile only tightens —
  clamps high / fills zero — it never raises an operator's tighter limit") but
  only exercised the clamping half; the built-in caps are now **ceilings** (a
  cap above the maximum is clamped down, an unset one is filled, a lowered one
  is left alone), the same rule the trust ceiling right below it already
  followed.
  The roadmap's third ask — remove the `kernel/controlplane`/`selfrepair` →
  `plugins/tools/overseertool` edge — is done by giving the row all three
  callers speak (the outcome of one agent repair) to the module
  (`roster.RepairResult`, with the overseer tool keeping its own name as an
  alias) and by injecting the repair source from the daemon: the control plane
  and the self-repair module now ask for a repair instead of importing the tool
  that provides one. **Both `upward` edges are gone and the ratchet DROPS from
  128 to 126** import violations (13 call sites unchanged).
  8 mutations, 8 caught — and the run found an honest gap in the **tests**:
  the operator's repair test asserted only that the repair was accepted and
  journaled, so a repair that did nothing at all still satisfied it; the test
  now asserts the repair reported the agent's own answer, and the mutations are
  caught on the repeat (two mutations' first anchors did not compile — a build
  failure is not an escape and is not counted as one). The move broke one live
  reference and it is fixed: the OpenClaw/Hermes parity ledger
  (`cmd/agt/compare_data.go`) asserts that its evidence files exist and one row
  still pointed at `kernel/roster/roster.go`. Nothing else changed: the same
  profiles, the same store, the same events, the same caps for a guardian that
  never had a lowered one.
- W3.9 (the reaper's home and the agents module's api package) is committed on
  `main` as a code commit plus a docs commit and published from
  `arch/w2-dispatch` the same way. The fleet's read-only health scan lived in
  the composition root (`kernel/runtime/reaper*.go` — the scan, the health
  classification, the routing and retry pressure, the forced probation) and
  reached into the kernel's own fields for the journal, the roster, the
  artifact index, the schedules, the standing orders and the provider. It moved
  to the agents module — `kernel/modules/agents/reaper` — with the findings it
  publishes declared **once**, in the module's vocabulary
  (`kernel/modules/agents/api`).
  A module may import another module's api package and nothing else (P5), so
  the module holds **no journal, no roster, no artifact index and no provider
  of its own**: every fact it judges arrives through the host's ports
  (`api.ReaperPorts` — the journal, the fleet, the runtime-override
  validation, the artifact index, the governor's live model chains, the
  trigger bindings). The runtime keeps its public names as **aliases** of the
  module's rows and holds the module as `api.ReaperScanner`; the composition
  root (`cmd/agezt: openAppKernel`) builds the scanner over the kernel's ports
  and injects it, exactly as it already does for the reasoning module, so the
  primary kernel and every tenant kernel are wired. The host's own
  runtime-config validation is what the module asks about a profile's
  overrides, so **the doctor and the reaper can never disagree** about what a
  valid override is.
  The ratchet is **unchanged** at 126/13: the runtime's api import is free (an
  api package is exempt from the cross-module rule), the module's
  implementation is reached only by the composition root, and no adapter or
  plugin grew an edge (275 packages now — two new ones, no new edge).
  21 mutations, 21 caught — and the run found three honest gaps in the
  **tests**, all fixed and named: the binding findings were only asserted on
  their own (so an agent with a broken override *and* a broken binding lost one
  of them), the retired agent's *schedule* binding was never exercised (only
  its standing order), and neither "an agent that ran recently is not a
  candidate" nor the stale-artifact totals were covered at all.
  Nothing else changed: the same findings, the same rows, the same wire JSON,
  the same env knobs.
- W3.10 (the extensions module) is committed on `main` as a code commit plus a
  docs commit and published from `arch/w2-dispatch` the same way. Five
  packages were already one module — the layer map named `kernel/mcp`,
  `kernel/plugin`, `kernel/toolforge`, `kernel/toolbox` and `kernel/acpcatalog`
  as layer 3 in the module `extensions` — but they sat at the top of `kernel/`,
  outside the `kernel/modules/` tree W3 establishes as the target home of the
  bounded contexts; they moved to
  `kernel/modules/extensions/{mcp,plugin,toolforge,toolbox,acpcatalog}`. The
  package names do not change, so the move is an import-path change and nothing
  else (83 files repointed). The placement is now *derived*
  (`kernel/modules/*/...` → module `extensions`) and the five hand-written
  layer rules are gone.
  The module's name does not change, so every edge keeps its shape and only its
  path changes: the **ratchet stays at 126/13** and the nine allowlist entries
  moved with their edges. A module that is five packages is not a new problem
  — the five packages already were one module, so the host already reached them
  through five edges; it is *splitting* a module into more packages that would
  cost edges.
  The moved packages keep their own suites, and they are the proof that the
  move did not quietly stop testing anything. One test was **strengthened**:
  the malformed-pin test asserted only that *an* error came back, while a
  malformed pin must be rejected as a malformed **digest** ("your pin is not a
  digest" and "the file changed" are different sentences to an operator).
  12 mutations, 12 caught — the run found an honest gap in the **tests** (the
  malformed-pin test said only that something failed) and one **equivalent
  mutant** (removing the empty-command guard changes nothing, because the next
  guard refuses the same case); both are recorded.
  The move broke four kinds of live reference and all are fixed: the
  OpenClaw/Hermes parity ledger (`cmd/agt/compare_data.go`) asserts that its
  evidence files exist and two rows pointed at the old paths; the dead-code
  checker's own path-prefix allowlist moved with the package; the layer map's
  five hand-written rules are gone; and eleven live documents (plus the
  plugin-governance example and the SDK's doc comment) named the old paths —
  the **dated** reports (`.project/PHASE-*`, `CHANGELOG/unreleased/m*.md`, the
  dated scans and audits) describe the tree as it was and are left alone.
  Nothing else changed: the same packages, the same exported APIs, the same
  stores, the same protocol, the same recipes and the same wire.
- W3.11 (the providers module's first half) is committed on `main` as a code
  commit plus a docs commit and published from `arch/w2-dispatch` the same
  way. `kernel/catalog` (12 files) and `kernel/chatgptauth` (8 files) were
  already one module — the layer map named them as layer 3 in the module
  `providers` — but they sat at the top of `kernel/`, outside the
  `kernel/modules/` tree W3 establishes as the target home of the bounded
  contexts; they moved to `kernel/modules/providers/{catalog,chatgptauth}`. The
  package names do not change, so the move is an import-path change and
  nothing else (77 files repointed). The placement is now *derived*
  (`kernel/modules/*/...` → module `providers`) and the two hand-written layer
  rules are gone. The module's name does not change, so every edge keeps its
  shape and only its path changes: the **ratchet stays at 126/13** and the
  eight allowlist entries moved with their edges.
  The moved packages keep their own suites, and **four tests were added** —
  the mutation run says why, and the gaps were on the critical paths: a
  provider that needs credentials with a **nil** lookup must be refused (not
  crash on a nil call); a stored refresh token with **no** access token must
  fetch one (not return empty); a token near expiry with **no refresh token**
  must not burn a provider round trip that cannot succeed (the test counts the
  calls, not just the token); and a logout must remove the secret from the
  **vault**, not only from the manager's memory — the old test asserted the
  in-memory state and would have passed with the bytes still on disk.
  10 mutations, 10 caught — the run found those four honest gaps in the
  **tests** (all fixed and named) and one mutation's first anchor did not
  compile (a build failure is not an escape).
  The move broke one **silent** reference and it is fixed: the CI race job
  runs `go test -race` over `./kernel/chatgptauth/...`, and a stale path there
  is not a failure — it is a package that quietly stops being race-checked.
  The layer map's two rules are gone, and the architecture set, the provider
  plans and specs, the docs and the code comments that named the old paths are
  updated — the **dated** reports (`.project/PHASE-*`, `.project/AUDIT-*`,
  `.project/REVIEW-MAP.md`, `CHANGELOG/unreleased/m*.md`,
  `docs/ARCHITECTURAL-REPORT.md`) describe the tree as it was and are left
  alone. Nothing else changed: the same packages, the same exported APIs, the
  same stores, the same prices and the same wire.

- W3.12 (the providerboot half of the providers module) is committed on `main`
  as a code commit plus a docs commit and published from `arch/w2-dispatch` the
  same way. `plugins/providerboot` was **not** a library that happened to sit
  under `plugins/`: it imported the **concrete** provider packages
  (`plugins/providers/{compat,mock,openairesponses}`) so the kernel would never
  grow a kernel→plugins edge, it was typed against them
  (`chatgptModelSet.Info` was a `map[string]openairesponses.ModelInfo`) and it
  reached into `kernel/agent` for the middleware stack. Moving it into the
  module as-is would have added **four** edges the ratchet refuses to grow
  (three upward, one cross-module providers→runs), so the slice is a **ports
  rewrite** — the shape W3.7 and W3.9 proved.
  The engine is now `kernel/modules/providers/boot` and the module grew a gate,
  `kernel/modules/providers/api`: the module asks the adapters for what it
  needs through `api.Ports` (an **interface**, so a missing adapter is a
  compile error at the wiring site, not a nil dereference on a boot path), the
  module keeps **its own vocabulary** (`api.ModelInfo` replaces the adapter's
  row, and the module owns the picker and ordering rules), the middleware
  stack stays the host's (`agent.MiddlewareFromEnv`, asked for through the
  `Wrap` port, so it is built once per boot cycle and reaches the primary and
  every alternate alike), and the composition root
  (`cmd/agezt/boot_providerports.go`) binds the ports to the real adapters.
  The **ratchet went DOWN**: 126/13 → **123/13**, because the three edges the
  plugin needed are gone, and **no** allowlist entry was added.
  The engine's own suites moved with it and now drive the engine through
  **fake ports** — the module never imports a plugin, so its tests must not
  pretend to — and the fake mirrors the real adapters' contracts (an empty
  model id and a model the entry does not serve are both refused), because a
  fake that accepts what the real adapter rejects hides exactly the bugs the
  run looks for. The **real** adapters are measured where they are bound, in
  the root (the demo echo's answer, the compat construction, the empty-model
  refusal, the family gate, the placeholder model, the ChatGPT base URL, the
  backend-reply mapping, the token function's two branches).
  **Nine tests were added** where the port seam opened a gap: an unsupported
  family must not be registered as an alternate; the middleware must reach the
  alternates and not only the primary; a nil wrapper and an unwired `Deps` must
  be **loud** errors rather than a silently unwrapped provider; the catalog
  entry must record the base URL the **adapter** serves; signing in must drop
  the weaker Codex-CLI memo; the module's own picker and ordering rules needed
  tests of their own; and the middleware stack — which moved packages — needed
  six.
  19 mutations, 19 caught; two needed a second round and both are recorded
  (M03's anchor pointed at the wrong file, M07's plain removal was a **build
  error** because `fmt` became unused, so the guard was neutered instead).
  Nothing else changed: the same selection rules, the same registration order,
  the same live down-route eligibility set, the same prices and the same wire.

- W3.13 (the reasoning rows move to the contract layer) is committed on `main`
  the same way. Archcheck''s plugin rule is strict — a plugin (L6) may reach
  the **contract** and the **platform** layers and nothing else — and the
  council, the conductor and the research tools each reached into
  `kernel/runtime` for exactly one reason: to name the reasoning module''s
  rows in their `Runner` interface. The interface was already the right shape
  (a port!); only its **types** were on the wrong side of the layer line. A
  row the agent is shown is a contract between the module that produces it and
  the tool that shows it, so it belongs at the bottom: the rows moved to
  `kernel/contract/reasoningapi` (L1), the module''s api re-exports them as
  **aliases** (so the host''s public names and the module''s surface do not
  move), and the tools name the contract''s rows directly.
  The **ratchet went DOWN again**: 123/13 → **120/13**, three plugin-reach
  edges gone, no allowlist entry added.
  The module, the host and the tools now speak **one** type, and the wiring
  only compiles because it is one type — which is now **asserted at the seam**:
  `kernel/modules/reasoning/api/identity_test.go` states the identity as a
  **function type** on purpose, because an assignment would also accept a copy
  with the same fields while a function type requires identity, so a module
  that ever declares its own copy is reported at the seam instead of at the far
  end of the wiring.
  The three tools'' tests gained the assertion that was missing: the **whole**
  result must reach the agent (the research tool''s claims, verdicts,
  verification flag, cited-source count and source rows; the conductor''s
  transcript steps, resolved role models and verdict; the council''s panel,
  opinions, dissent and dated brief).
  9 mutations, 9 caught, and 1 build error recorded separately: M10 (the
  module copies the contract row instead of aliasing it) **escaped the first
  round**, because the module''s own tests speak the module''s names
  consistently and the divergence only shows where the host satisfies the
  tools'' `Runner` interfaces — the identity assertion is what closed it.

- W3.14 (the roster's rows move to the contract layer) is committed on `main`
  the same way. The agents module's profile row (and the policies that hang off
  it, and the repair outcome) and the board's message row move down to the
  **contract** layer (`kernel/contract/agentapi`, `kernel/contract/boardapi`).
  The reason is the plugin rule: a plugin (L6) may reach the contract and the
  platform layers and nothing else, and the **overseer tool** reached into the
  roster module for **types only** — its `Source` interface returns
  `[]roster.Profile` and its edit payload is a `roster.Profile`. A row the
  overseer must name is a contract between the module that owns the roster and
  the tool that steers it, so it belongs at the bottom. The roster and the
  board keep their names as aliases, so every existing caller is unchanged,
  and the Profile's three pure rules (`Kind`, `AllowsDirectCall`,
  `AllowsDelegationFrom`) moved with the row.
  The **ratchet went DOWN**: 120/13 → **119/13** — the overseer's edge to the
  roster module is gone, and the **stale allowlist entry was deleted**, because
  a leftover entry fails the archcheck gate ("the debt was paid, the list must
  shrink").
  The permanent tests: the **identity assertions** (the W3.13 recipe, applied
  twice) state that the module's row and the contract's row are the same type
  — as a **function type**, because an assignment would also accept a copy
  with the same fields while a function type requires identity, so a module
  that ever declares its own copy is reported at the seam instead of at the far
  end of the wiring — and the **wire shape got measured, which it never had**:
  the profile's and the message's JSON keys are what the Web UI, the SDK and
  the persisted files read by name, so a renamed key is a silent break for
  every consumer outside the package, and no test in the package would notice
  because the package writes and reads the same struct.
  8 mutations: 6 caught, 2 build errors recorded separately (the two
  mutations that make a module copy the contract row instead of aliasing it —
  the identity assertion fires at the seam), and **M07/M08 escaped the first
  round** for exactly the wire-shape reason above; the two wire-shape tests
  closed them.
  The overseer tool still holds the live kernel and opens the board store, so
  two of its plugin-reach edges remain — the **second half** of the slice.

- W3.15 (the overseer's implementation moves to the app layer) is committed on
  `main` the same way, and it closes that second half. W3.14 moved the tool's
  **rows** down to the contract layer; what was left were two edges that could
  not be closed by moving a type, because they are not types: the overseer tool
  held the **live kernel** (`kernel/runtime`) and opened the **board store**
  (`kernel/modules/board/board`) itself.
  The **port** moves to the contract layer (`kernel/contract/overseerapi`: the
  25-method `Source` plus `BulkResult` and `SearchFilter`) and the
  **implementation** moves to the app layer (`kernel/app/overseer`: the
  kernel-backed source, the bulk/wake/search/repair ops, the task-model chains
  and the repair brief) — the app layer is the only one that may see both the
  runs module and the board module. The tool keeps the tool (the op dispatcher,
  the schema, the views) and names the contract; `toolreg.KernelDeps` gains
  `Overseer`, and the daemon builds the implementation and injects it. That is
  the W3.12 ports recipe applied to a plugin instead of a module.
  The **ratchet went DOWN**: 119/13 → **117/13** — both plugin-reach edges of
  the overseer tool are gone, and the **two stale allowlist entries were
  deleted** ("the debt was paid, the list must shrink").
  The permanent tests: the **identity assertion** states that the tool's
  `Source`/`BulkResult`/`SearchFilter` ARE the contract's, as a **function
  type** (an assignment would also accept a copy with the same method set,
  while a function type requires identity); the **injected port** is measured
  through the real spec — the fake the daemon supplies must reach the tool, and
  an **unwired** port must be a clear error, not a panic and not a silent
  success; and the **live kernel** is driven directly, because the tool's own
  suite proves the op → port mapping with a fake and nothing proved that the
  implementation *behind* the port still reads and steers the live system.
  The move also **exposed a gap that is now closed**: the guardian guard, the
  forced `System=false` on create and the PATCH semantics of the agent-facing
  edit live in the implementation, and the tool's fake never reached them, so
  they had **never** been measured.
  12 mutations: 10 caught, 2 build errors recorded separately (one of them the
  identity assertion firing at the seam; the other a first anchor whose
  deletion left an unused variable — a build failure is not an escape, and the
  re-anchored mutation is caught).

- W3.16 (three tools name the artifact row where it lives) is committed on
  `main` the same way. The browser, the code executor and the fetch tool each
  reached into the artifacts module for **one type** — the artifact row — and
  nothing else. That row was already a contract row
  (`kernel/contract/remoteapi`); the module merely aliased it
  (`artifact.Entry = remoteapi.Artifact`). The tools named the alias, and the
  alias lives in a module's implementation package, so each of the three paid
  a `plugin-reach` edge for a type that was never theirs to begin with. Now
  they name `remoteapi.Artifact` — the row's real home — and the module keeps
  its name, so every other caller is unchanged.
  The **ratchet went DOWN**: 117/13 → **114/13** — all three plugin-reach edges
  are gone, and the **three stale allowlist entries were deleted**.
  The permanent tests: the **wire conformance** states that the artifact
  record a tool reports IS the contract row's wire, key for key — every key
  the tool emits must be a key the contract row carries, the value must be
  the row's own, and the **exact key set** is pinned. Nothing tied them
  together before: the record is a hand-written map or struct, so a renamed
  contract field would have left the tool emitting an empty value under a key
  nobody reads any more — **silently**, for every consumer of the tool's
  answer. The module's identity assertion was also **strengthened to the
  function-type form** (the W3.13/W3.14 recipe): the old `reflect.TypeOf`
  comparison would also have accepted a copy with the same fields.
  9 mutations: 8 caught, 1 build error recorded separately (the module's row
  stops being the contract's row — the identity assertion fires at the seam).

- W3.17 (two more tools name the agent profile where it lives) is committed on
  `main` the same way. The config tool and the schedule tool each reached into
  the roster module for **one type** — the agent profile — and nothing else.
  That row moved to the contract layer in W3.14, so the module only kept an
  alias; the two tools named the alias, and the alias lives in a module's
  implementation package. Now they name `agentapi.Profile`.
  The slice also found what the rename exposed: **the "who manages this
  agent?" rule was implemented twice** — once in the schedule tool, once in the
  overseer's repair surface — with the same trigger and the same lookup. The
  rule moved to the row it is about: `agentapi.Profile.ManagedBy()`. The two
  surfaces keep their own wording, but they now agree on **who** manages the
  agent.
  The **ratchet went DOWN**: 114/13 → **112/13** — both plugin-reach edges are
  gone, and the **two stale allowlist entries were deleted**.
  9 mutations: 9 caught, 0 build errors — and the last one is the honest proof
  of the slice's shape: a plugin that names the module's alias instead of the
  contract row compiles and behaves **identically**, so only the layer rule can
  tell the difference.

- W3.18 (the standing order's row moves to the contract layer) is committed on
  `main` the same way. The **introspector** (the tool that reports the daemon's
  own live state) and the **guardians** (the plugin that seeds and reconciles
  the autonomous fleet) both reach into the standing module for its rows, and
  both are plugins — a row a plugin has to name cannot live in a module's
  implementation package. The rows moved to `kernel/contract/standingapi`
  (`Order`, `Trigger`, `Initiative`, the two mode types, the pure `Validate`
  and the `MaxAutonomyTrust` rule); `kernel/standing` keeps every name as an
  alias, so the store, the runner, the control plane and the standing tool are
  unchanged.
  The **ratchet went DOWN**: 112/13 → **110/13** — both plugin-reach edges are
  gone, and the **two stale allowlist entries were deleted**.
  The **wire shape got measured, which it never had**: the standing order's
  JSON keys are what the control plane serves, the Web UI reads and the store
  persists, and they are read by name — a renamed key is a silent break for
  every consumer outside the package.
  9 mutations: 8 caught, 1 build error recorded separately — and **M03 escaped
  the first round**: the omits-empty assertion looked at the order's top level
  and the initiative's `max_trust`, but not at the initiative's second field.
  A mutation that survives is a missing assertion, so the assertion was added
  and the mutation re-run.

- W3.19 (the schedule entry's row moves to the contract layer) is committed on
  `main` the same way and is **delivered through PR #814 at `8a36b75a`** after
  all 24 exact-head CI jobs (the merge tree equalled the tested head
  `27e3e625`). The same shape as W3.18, on the cadence side. The
  introspector and the guardians both name the schedule entry's row, and both
  are plugins. The row and its rules moved to `kernel/contract/cadenceapi`: the
  entry, `Validate`, `Forecast`, `Advance`, `SafeInterval`, `UsesInterval`, the
  day-mask helpers, the modes, the targets, the sources, and the
  **system-task catalog** (`Entry.Validate` checks a system-task target, so the
  catalog has to be where the row is). `kernel/cadence` keeps every name as an
  alias, so the store, the engine, the ticker, the CLI and the schedule tool
  are unchanged.
  Three rules about the row had to be **exported** rather than kept
  unexported (the module's store and engine call them, and a method can only
  be defined in its type's package) — the same rules about the same row, with
  the W3.14 recipe's shape.
  The **ratchet went DOWN**: 110/13 → **108/13** — both plugin-reach edges are
  gone, and the **two stale allowlist entries were deleted**.
  9 mutations: 8 caught, 1 build error recorded separately — and three of the
  caught mutations are the **timing rules** (the busy-loop floor, the daily
  slot, the window walk), caught by the module's own DST and interval tests,
  which now run against the contract's implementation of the same rules.

- W3.20 (the two trigger tools name their rows where they live) is committed
  on `main` the same way. The **schedule tool** (the agent's own cron) and the
  **standing tool** (the agent's own durable wake rules) each reached into a
  module — one for the rows they speak, one for the store they drive — and
  both are plugins. Nothing moved **out** of a module this time: W3.18 and
  W3.19 had already put the standing and the schedule rows in the contract
  layer, and the modules kept their names as aliases, so what was left was the
  plugin's half — each tool still named the *alias*, and the alias lives in a
  module's implementation package, so each paid a `plugin-reach` edge for a row
  that was never theirs to name.
  The schedule tool now takes the store as a **port** (`cadenceapi.Store` — the
  eleven methods it drives; the module's `*cadence.Store` implements it), and
  the standing tool takes the **profile lookup** by injection (the shape the
  schedule tool already used in W3.17) and lists the orders through the
  kernel's `StandingList()`. The standing tool's third copy of the "who
  manages this agent?" rule was dropped: W3.17 had already moved it onto the
  profile row as `agentapi.Profile.ManagedBy()`.
  The **ratchet went DOWN**: 108/13 → **105/13** — all three plugin-reach edges
  are gone, and the **three stale allowlist entries were deleted**.
  The permanent tests: the **identity assertions** (the module's four standing
  rows and the roster's profile ARE the contract's, as function types), the
  **store port** as a compile-time promise plus a test that the store still
  works when it is held as the port, and — the new part — the **wiring**
  measured against a **real kernel**: a managed sub-agent is still refused with
  its manager named (so the roster lookup really crossed the wire), and a
  schedule the tool creates lands in the kernel's own cadence store (so the
  port really carries the live store).
  18 mutations: 17 caught, 1 build error recorded separately — and one
  mutation **survived the first round** (the day mask: the test asserted only
  that it was non-zero, which any wrong mask satisfies), so the test was
  strengthened to assert the exact weekday and weekend masks and the mutation
  was run again. Three mutations are caught **only** by the wiring tests: the
  tool's own suite proves the tool, and nothing but a test against a live
  kernel proves that the daemon still hands it what it needs.

- W3.21 (two more tools name their rows where they live) is committed on
  `main` the same way. The **board tool** (how agents talk to each other) and
  the **artifacts tool** (the files an agent saved) each reached into a module:
  for the rows they speak, for the filter they query with, and for the store
  they drive. Both are plugins.
  The board tool now names the board row and the **wildcard recipient** where
  they live (`kernel/contract/boardapi`) and takes the store as a **port**
  (`boardapi.Store`). Its old `Bind(dir)` — which opened a store of its own —
  is **gone**: the daemon opens the one shared store and hands it over, because
  two writer instances silently drop each other's messages. The artifacts tool
  now names the artifact row and the filter beside it
  (`kernel/contract/remoteapi`).
  The **ratchet went DOWN**: 105/13 → **103/13** — both plugin-reach edges are
  gone, and the **two stale allowlist entries were deleted**.
  The interesting row was the **wildcard recipient**: `board.Everyone` was a
  value the module used consistently, so **no test could see it change** — but
  the value is read **by value** across the boundary (the persisted board file
  stores it, the daemon's boot replay special-cases it, and the SDK's mailbox
  filter compares the literal `"*"`), so a change to the constant would have
  silently broken every reader that knows the value and not the constant. The
  SDK now names the contract's constant instead of repeating the literal, and
  a test pins the value itself.
  The permanent tests: the **store port** as a compile-time promise plus a test
  that the store still works when it is held as the port, the **identity
  assertion** for the filter, the **wiring** measured against a real board
  store and a real kernel, and — never measured before — the **wire shapes**
  of the board view and the artifacts row, pinned key for key.
  17 mutations: 14 caught, 3 build errors recorded separately — and one
  mutation **survived the first round** (the wildcard's value), so the gap was
  closed and it was run again.

- W3.22 (the MCP server's row moves to the contract layer) is committed on
  `main` the same way. Two plugins name the description of one registered MCP
  server — a stdio command or a remote HTTP endpoint the daemon can spawn and
  speak MCP to: the **MCP tool** and the **marketplace** that ships the
  builtin server packs. The row moves to `kernel/contract/mcpapi`, and **the
  rule travels with it**: `Validate` (the name format, the one-transport rule,
  the URL's shape, the caps) is pure, so it belongs where the row is, and the
  caps are the contract's too.
  The MCP tool may no longer name the **registry** — it asks the kernel for the
  **rows** instead (`MCPServers()` / `MCPServer(ref)`, the same shape the
  standing tool got in W3.20) — and the registry, the store that persists the
  rows, stays in the module.
  The **ratchet went DOWN**: 103/13 → **101/13** — both plugin-reach edges are
  gone, and the **two stale allowlist entries were deleted**.
  The row's **wire had never been measured**, and this is the row where it
  matters most: the registry **persists** it and the control plane and the Web
  UI read it back **by key**, so a renamed key is a silent break — the old
  registry file would lose the field and the UI would read nothing. The wire
  is now pinned (the exact key set, and the always-fields that must stay on
  the wire even when they are empty).
  The **caps are measured at the boundary** (32 accepted / 33 rejected, 128 /
  129): a test that builds `Cap+1` items passes whatever the cap is, so the
  policy is spelled out instead of read back from the constant — and two
  mutations that survived the first round were exactly that shape.
  The permanent tests: the **identity assertion** (the module's row IS the
  contract's, as a function type), a test that the module's `Validate` and the
  contract's agree on every verdict **and** on the message (a second copy of
  the rule would drift silently), and the **wiring** measured against a real
  kernel.
  13 mutations: 11 caught, 2 build errors recorded separately.
  One **operator-visible** change is recorded honestly: a JSON decode error
  now says `mcpapi.Server` where it used to say `mcp.Server`.

- W3.23 (the catalog's rows move to the contract layer) is committed on `main`
  the same way. The **compatibility adapter** turns a catalog row into a wire
  provider, and it is a plugin — so the description of one provider and the
  models it serves now live in `kernel/contract/catalogapi`, together with the
  rules that belong to them: the NPM → family mapping, the native-JSON-mode
  families, the credential key shape, the model's own helpers and the cost
  conversion. The module keeps the names its callers still use as one-line
  delegations, and the family mapping it no longer uses is **gone** — a name
  nobody calls is dead code, and the dead-code gate says so. The **catalog**
  itself — the merge of the sources, the sync from models.dev, the store —
  stays in the module.
  The **ratchet went DOWN**: 101/13 → **100/13** — the plugin-reach edge is
  gone, and the **stale allowlist entry was deleted**.
  This is the row where the **wire matters most**: the catalog is fetched
  from **models.dev** and unmarshalled **directly** — the field names are
  models.dev's — and it is persisted as `api.json`, so a renamed key would
  silently drop every provider's models and the catalog would come back empty.
  The wire had never been measured; it is now pinned key for key.
  The permanent tests: the **identity assertions**, a test that the module's
  names and the contract's agree on every verdict, the **rules measured where
  the rows are**, and the **scoped-credential lookup** (the provider-scoped
  vault key first, the bare env var second).
  20 mutations: 18 caught, 2 build errors recorded separately — and two
  mutations **survived the first round** (the scoped-credential lookup and the
  model's vision helper), so both gaps were closed and they were run again.

- W3.24 (the introspector's rows move to the contract layer, and its
  implementation to the app layer) is committed on `main` the same way. The
  **introspector** was the one plugin that reached into the **kernel itself**:
  it held both the rows it published (`Overview`, `ReaperOverview`, the three
  agent rows, `ReaperReport`) **and** the adapter that assembled them from the
  live `*runtime.Kernel`. The rows now live in `kernel/contract/introspectapi`
  and the adapter in the **app layer** (`kernel/app/introspect`). The tool
  reads the live state through the contract's **port**
  (`introspectapi.Source` — the overview, the reaper scan, the schedules, the
  standing orders), and the **daemon** hands the adapter over; the plugin no
  longer builds its own.
  The **ratchet went DOWN**: 100/13 → **99/13** — the plugin-reach edge is
  gone, and the **stale allowlist entry was deleted**.
  The tool's **wire is byte-identical** and is now **pinned** (the overview's
  17 keys, the always-fields, the three agent rows).
  The interesting gap was the **wiring**: a wiring nobody witnesses is a
  wiring that can be dropped silently — the tool would answer "not available
  on this daemon" on every live box, while every test that built the tool by
  hand still passed. So the composition root's dependency assembly was pulled
  out of `runDaemon` into a named function (`cmd/agezt/tooldeps.go`) and
  **measured there**: the daemon must supply the introspect port and the
  overseer port, and both must answer the **live** kernel.
  The permanent tests: the **identity assertions**, the **adapter against the
  live kernel** (the delegation ceilings, the journal head, the tool names, and
  a schedule and a standing order the operator added), and the **projection
  measured field for field** (the reaper's row → the contract's row is a
  hand-written copy; a field left out of it would be silent).
  23 mutations: 23 caught — and one **survived the first round** (the sorted
  tool list: the adapter's test registered only one tool, so a reversed list
  looked sorted), so the test was strengthened and the mutation was run again;
  six mutations were **first authored as build errors** (a compile error is
  not a catch) and were re-authored so the code still compiles.

- W3.25 (the skill's rows move to the contract layer) is committed on `main`
  the same way. The **skill tool** — the agent's self-modification primitive
  — was a plugin that reached into the **skills module**: for the **rows** it
  speaks (the lifecycle state, the record, the create input) and for the
  **store** it drives. The rows now live in `kernel/contract/skillapi`, and
  the **rules travel with them** — the lifecycle state machine is pure, so it
  belongs where the row is, and it is the rule the whole daemon enforces. The
  retrieval-pool predicate travels too (a method can only be defined in its
  type's package). The module keeps its names as **aliases** and its rules as
  one-line delegations, so there is one row and one spelling of it.
  The tool now drives the contract's **ports** (`skillapi.Forge`,
  `skillapi.BundleStore`), and the **daemon hands the bundle port over
  separately** — so the tool never names an implementation. The Forge, the
  store, the content-addressing (it hashes with BLAKE3, and a contract
  package may not import a third-party package) and the on-disk bundle store
  stay in the module.
  The **ratchet went DOWN**: 99/13 → **98/13** — the plugin-reach edge is
  gone, and the **stale allowlist entry was deleted**.
  This is the row where the **wire matters**: a skill is written to
  `<home>/skills/skills.json` and read back **by key** — the CLI, the Web UI
  and the retrieval layer all read that file — so the row's JSON keys are a
  contract with every skill file already on disk. The wire had never been
  measured; it is now pinned key for key, and the persisted **values** of the
  five states are pinned too (a renamed state would orphan every stored
  skill). The rules are measured **edge by edge**, and the module's
  delegations against the contract on **every pair of states**.
  The permanent tests: the **identity assertions** (the module's four rows
  ARE the contract's, and the module's Forge and bundle store ARE the
  contract's two ports), the **plugin speaks the contract's rows**, and the
  **wiring, twice** — a skill the agent learns lands in the **kernel's own
  Forge**, and the **second** wire (the bundle port) really reaches the tool.
  24 mutations: 24 caught — one **survived the first round** (the bundle port
  was never handed over and the wiring test only proved the Forge wire), so
  the gap was closed and it was run again; and one was an **equivalent
  mutation** (a rule rewrite that changed nothing for any input),
  re-authored into a real disagreement and run again.

- W3.26 (the marketplace's built-in catalogue reads the bundles through the
  contract's port) is committed on `main` the same way. The built-in
  "Official" catalogue — the packs built from the skill bundles **baked into
  the binary** — is a plugin, and it reached into the skills module for two
  things: the module's public seam for the embedded bundles, and the
  **SKILL.md parser** it reads them with. The parser is pure, so it moves to
  the contract layer with the rest of the skill rows, and the module keeps
  its name as a **delegation** — one parser, so the module's own CLI
  (`agt skill import`) and the marketplace cannot drift apart. The embedded
  **bytes** stay in the module; the plugin reads them through the contract's
  **port** (`skillapi.Builtins`), which the module implements
  (`skill.NewBuiltins()`).
  The **ratchet went DOWN**: 98/13 → **97/13** — the plugin-reach edge is
  gone, and the **stale allowlist entry was deleted**.
  The interesting gap was the **wiring**: the catalogue is built over
  whatever the port hands over, so a daemon that handed over nothing would
  offer **no packs at all** — and every test that builds the plugin by hand
  would still be green. So the daemon's wiring was pulled out of `runDaemon`
  into a named function (`cmd/agezt/marketlib.go`) and **measured there**.
  The parser's errors are measured **by which error they are** — a parser
  that rejects a document for the *wrong* reason still rejects it.
  The permanent tests: the parser's own tests **moved with the parser** to
  the contract, a test that the module's delegation and the contract's agree
  on every document, the module's built-ins implementation IS the contract's
  port, the **plugin reading through the port** (a bundle that cannot be read
  — or whose SKILL.md does not parse — is **skipped**, not fatal), and the
  **wiring**.
  18 mutations: 18 caught — one **survived the first round** (the parser's
  "no frontmatter" check removed: the test only asserted that *an* error came
  back), so the error is now measured and it was run again; two were **first
  authored as build errors** (a compile error is not a catch) and were
  re-authored so the code still compiles.

- W3.27 (the market's rows move to the contract layer) is committed on `main`
  the same way. The built-in "Official" catalogue is a plugin, and it reached
  into the **market module** for the rows it speaks: the installable
  **pack**, the **skill and MCP servers** it carries, the **catalogue entry**
  and the **catalogue**. Those rows now live in `kernel/contract/marketapi`,
  and the **rules travel with them** (a method can only be defined in its
  type's package): what makes a pack valid, its canonical bytes, its content
  hash, the catalogue row it projects to, the **version comparator** and the
  **skill summary** — both pure, and both the ONE rule their callers must
  share. The catalogue **port** (`marketapi.Library`) moves too: the built-in
  catalogue implements the contract's port rather than naming the market
  module. The **manager, store, syncer and composite library** stay in the
  module.
  The **ratchet went DOWN**: 97/13 → **96/13** — the plugin-reach edge is
  gone, and the **stale allowlist entry was deleted**.
  This is the row where the **wire matters**: a pack is **signed**, **hashed**
  and **published**, so the row's JSON keys are a contract with every
  published pack and every signature ever made — a renamed key would
  silently change what the signature covers. The wire had never been
  measured; it is now pinned key for key, with the always-fields pinned too.
  The rules are measured **rule by rule** — `Validate` must refuse a name
  that is not kebab-case, a version that is not semver, an empty pack, a
  skill with an empty or unparseable SKILL.md, a resource that escapes its
  bundle, and an MCP server the contract's own check refuses.
  The permanent tests: the **identity assertions**, a test that the module's
  comparator and the contract's agree on **every pair of versions** and its
  summary and the contract's agree on **every skill**, the **plugin**
  implementing the contract's port, and the **wiring**.
  26 mutations: 26 caught — two **survived the first round** (the
  malformed-version fallback and the summary's no-description branch: both
  branches of a rule were untested), so the tests were strengthened and they
  were run again; three were **first authored as build errors** (a compile
  error is not a catch) and one had a stale anchor, so all four were
  re-authored and run again.

- W3.28 (the script-tool rows move to the contract layer) is committed on
  `main` the same way. The **tool_forge** tool — the agent's self-tooling
  primitive — is a plugin, and it reached into the **extensions module** for
  the rows it speaks (the script-tool record and its lifecycle state) and for
  the store it reads. Those rows now live in `kernel/contract/toolforgeapi`,
  and the **rules travel with them** (the draft gate is pure): the name
  pattern, the language pattern and the three size caps. The module keeps its
  names as **aliases** and its rule as a one-line **delegation**.
  The tool's **read path changes shape**: the store used to be reached
  through the kernel (`k.ToolForge()` — a concrete implementation a plugin
  should never hold); now it arrives as the contract's **port**
  (`toolforgeapi.Store`), handed over by the **daemon** as a second argument
  to `Bind(k, store)`. The store, the lifecycle transitions and the on-disk
  persistence stay in the module.
  The **ratchet went DOWN**: 96/13 → **95/13** — the plugin-reach edge is
  gone, and the **stale allowlist entry was deleted**.
  This is the row where the **wire matters**: a script tool is **persisted**
  (`<home>/scripttools.json`) and read by key, so the row's JSON keys are a
  contract with every tool file already on disk. The wire had never been
  measured; it is now pinned key for key, with the always-fields and the
  optional ones pinned too, and the three **lifecycle values** pinned (a
  renamed state would orphan every stored tool). The rules are measured
  **rule by rule**, and every **limit** is measured **at its boundary**; the
  cap **numbers** are pinned separately, because a test that measures a cap
  against the constant itself passes any value at all.
  The interesting gap was the **wiring**: a port nobody hands over is
  silently nil, so the tool would answer "not available on this daemon" on
  every live box while every test that built the tool by hand stayed green.
  The daemon's wiring was pulled out into named functions
  (`cmd/agezt/tooldeps.go`) and **measured there** — the port must answer the
  **live** store — and the **nil-port rule** is stated once: a nil store must
  stay a nil *port*, checked on the **concrete** type on purpose.
  The permanent tests: the **identity assertions**, a test that the module's
  rule and the contract's agree on every verdict **and message**, a test
  that the **plugin** names the contract and **not** the module, the tool's
  **wire** (the view's key set, the test record measured as a value, the
  list's count) and the **wiring, twice**.
  41 mutations: 41 caught — four **survived the first round** (the three cap
  values were measured against the constant itself, and the test record was
  only ever read on an untested tool), so the tests were strengthened and
  they were run again; one had a **stale anchor**, so it was re-authored and
  run again.

- W3.29 (the workflow rows move to the contract layer) is committed on
  `main` the same way. The `workflow` tool — the agent's automation
  primitive: it authors durable, named, acyclic graphs of typed nodes and
  runs them — is a plugin, so the description of one workflow, its typed
  nodes, the edges that wire them, the trigger configuration that starts it,
  the per-node config payloads and the outcome of a run now live in the
  contract layer (`kernel/contract/workflowapi`). The **rule travels with
  them** (the graph rule is pure — stdlib only): exactly one trigger, known
  node types, edges that resolve, legal ports, per-type configs, an acyclic
  graph. The rule's own tests moved **with** it, because a proof that stays
  behind while the rule travels would be a proof of nothing. The module
  keeps its names as **aliases** and its rule as a one-line **delegation**;
  the store, the engine, the trigger runner and the templates stay in the
  module — they are the implementation.
  The tool's **read path changes shape**: the workflow library used to be
  reached through the kernel (`k.Workflows()` — a concrete implementation a
  plugin should never hold); now it arrives as the contract's **port**
  (`workflowapi.Store`), handed over by the **daemon** as a second argument
  to `Bind(k, store)`. The outcome the tool reports (`RunResult`) is a row
  of the contract too — so the plugin no longer names the kernel either.
  The **ratchet went DOWN twice**: 95/13 → **93/13** — two `plugin-reach`
  edges are gone (to the workflows module and to the kernel), and the
  **stale allowlist entries were deleted**.
  The **wire matters** here as much as it did for the script tools: a
  workflow is **persisted** and read back by key, so the row's JSON keys are
  a contract with every workflow file already on disk. The wire had never
  been measured; it is now pinned key for key, with the always-fields and
  the optional ones pinned too, and the fifteen **node types** pinned (a
  renamed type would orphan every stored graph that uses it). The rules are
  measured **rule by rule**, each with **its own message**, and every
  **limit** is measured **at its boundary**; the limit **numbers** are
  pinned separately, because a test that measures a limit against the
  constant itself passes any value at all.
  The interesting gap was the **wiring**: a port nobody hands over is
  silently nil, so the tool would answer "not available on this daemon" on
  every live box while every test that built the tool by hand stayed green.
  The daemon's wiring was pulled out into a named function
  (`cmd/agezt/tooldeps.go`) and **measured there** — the port must answer
  the **live** library — and the **nil-port rule** is stated once.
  The permanent tests: the **identity assertions**, a test that the module's
  rule and the contract's agree on every verdict **and message**, a test
  that the **plugin** names the contract and **neither** the module **nor**
  the kernel, the tool's **wire** and the **wiring, twice**.
  The **parity ledger** (`cmd/agt/compare_data.go`) names the files that
  back each parity claim and asserts they exist, so the rows' old path
  there was repointed at their new home.
  79 mutations: 77 caught — seven **survived the first round** (every one a
  **test gap**, not a rule gap), so the tests were strengthened and they
  were run again; two were **equivalent** (rules that cannot fire, proven
  and recorded as such) and were re-authored as reachable rules; one had a
  **stale anchor**, two were **compile errors** and one was **misclassified**
  by the harness, so the classifier was fixed and it was run again.

- W3.30 (the data-lake rows move to the contract layer) is committed on
  `main` the same way. The `db` tool — the agent's database: it creates
  collections, writes records, queries them, and the console and the Web UI
  read the same lake — is a plugin, so the description of one collection, one
  record, the query, the listing's summary and the three refusals the lake
  answers with now live in the contract layer
  (`kernel/contract/datalakeapi`). The tool's own store interface is
  replaced by the contract's **port** (`datalakeapi.Lake`) — one port, one
  spelling — and the module keeps its names as **aliases**; the lake, its
  persistence, the built-in collection gallery and the collection-name rule
  stay in the module — they are the implementation.
  The registry stops holding the lake by its **concrete** type too:
  `KernelDeps.Lake` is now the contract's port, so the registry names the
  contract and not the module.
  The **ratchet went DOWN twice**: 93/13 → **91/13** — two edges are gone
  (the plugin's and the registry's), and the **stale allowlist entries were
  deleted**. The second edge closed because the registry held the lake by its
  concrete type.
  The **wire matters** here too: a collection and its records are
  **persisted** and read by the console's tables and the Web UI's views, so
  the rows' JSON keys are a contract with every collection file already on
  disk. The wire had never been measured; it is now pinned key for key,
  with the **optional** fields pinned too — on the schema, on the column and
  on the record.
  The **refusals** are the tool's answers: the tool maps the lake's three
  refusals to three **different** answers, each naming the collection, so
  the messages are pinned too — not only the refusal but the tool's own
  phrasing.
  The **wiring** is measured at the composition root, and the **nil-port
  rule** is stated once: a nil lake must stay a nil *port*, checked on the
  **concrete** type on purpose — so the rule is a named function that can be
  tested on its own.
  The permanent tests: the **wire shape** (including the **port's** method
  set — exactly nine methods with their signatures), the **identity
  assertions** (the module's rows ARE the contract's rows, its errors ARE
  the contract's by identity and by message, its lake IS the contract's
  port), a test that the **plugin** names the contract and **not** the
  module, the tool's **answers** (a create keeps the declared columns, a
  second create is refused with the tool's own answer that names the
  collection) and the **wiring**.
  36 mutations: 36 caught — two **survived the first round** (the column's
  optional keys were never exercised, and the create op's refusal mapping
  was masked because the lake's raw message also says "already exists"), so
  the tests were strengthened and they were run again; one had a **stale
  anchor** and one was a **compile error**, so both were re-authored.

- W3.31 (the workboard chain moves to the contract layer) is committed on
  `main` the same way. The `workboard` tool — the agent-facing durable typed
  task queue — is a plugin, so the description of one task and everything that
  hangs off it now live in the contract layer. The chain moves **as a whole**,
  because a task carries its acceptance criteria, a criterion carries a
  **proof**, and a proof carries the **verdict** the completion judge reached:
  so the rows live in `kernel/contract/workboardapi`, `kernel/contract/proofapi`
  (moved with `git mv`, so the history follows the rows) and
  `kernel/contract/assureapi`. The **rules travel with them** — the status
  rule (`ParseStatus`, the lifecycle order, the known-status set) is pure, so
  it belongs where the row is, and the module keeps it as a one-line
  **delegation**; the assurance **loop** stays in the module, because it is
  the implementation.
  The tool's read path changes shape: the task store used to be reached
  **through the kernel** (`k.Workboard()` — a concrete implementation a plugin
  should never hold), and now it arrives as the contract's **port**
  (`workboardapi.Store`: `Get`, `List`), which the **daemon hands over** as
  a second argument to `Bind(k, store)`.
  The **ratchet went DOWN**: 91/13 → **90/13** — the plugin-reach edge is
  gone, and the **stale allowlist entry was deleted**.
  The **wire matters most here**: a task is **persisted**
  (`$AGEZT_HOME/workboard.json`) and read back by the `agt workboard` CLI,
  the console, the Web UI's task detail and the tool, so the row's JSON keys
  are a contract with every task file already on disk. The wire had never been
  measured; it is now pinned key for key (the task's five always-fields and
  twenty optional ones, the proof's, the criterion's, the evidence's), and the
  eight **lifecycle values** are pinned too.
  The **refusals say which refusal they are**: the empty status is refused as
  the bare contract error, an unknown status is refused naming itself — and an
  unknown status has **no position** in the lifecycle (`-1`), it is not "the
  first one", which is triage's place. That last one was a **real defect** the
  new tests found: `StatusOrder` documented `-1` and returned `0`, so an
  unknown status would have sorted as the triage lane. The implementation now
  honours the documented contract.
  The **wiring** is measured twice, because a daemon that dropped either
  would leave the tool answering "not available" on every live box while
  every hand-built test stayed green: the composition root's own test asserts
  the port is present and answers the **live** board, and the built-in tools'
  test asserts the registered tool is bound to that same port.
  The permanent tests: the **wire shape** (the task's, the proof's, the
  criterion's and the evidence's key sets, the lifecycle values, the port's
  method set), the **identity assertions** (the modules' rows ARE the
  contract's rows), a test that each module's rule and the contract's agree
  on **every** verdict, a test that the **plugin** names the contract and
  **not** the module, the tool's **read path** (the filter it asks the port
  for, the **bounds** of the limit, the failed-attempt count on the wire) and
  the **wiring**.
  37 mutations: 33 caught, 4 equivalent — nine were re-authored (seven
  **compile errors**, two **stale anchors**) so the code still compiles; the
  four equivalent ones are recorded as such rather than dressed up as
  catches (the verdict parser's fence trimming is redundant with its span
  isolation; a live kernel always opens its board, so that nil-branch is
  unreachable).

- W3.32 (the config tool's reach into the runtime becomes a port) is committed
  on `main` the same way. The `config` tool — the agent's window into the
  Config Center, which reads and writes the settings the daemon, the
  `agt config` CLI and the HTTP routes share, and which can write a setting
  **live** (rebuilding the provider in place) or **agent-scoped** (on one
  agent's profile) — is a plugin, so it may not hold the daemon's
  implementation. It reached for exactly three things: the roster's profile
  edit, the provider rebuild, and the run's own config overrides.
  The port now lives in the contract layer (`kernel/contract/configapi`) with
  those three methods and nothing else; the **adapter** that answers it lives
  in the app layer (`kernel/app/configtool`), because answering it needs the
  live kernel and the app layer is the only layer that may see it; and the
  daemon hands the port over at the composition root, exactly like the forge,
  workflow, data-lake and workboard ports before it.
  The **ratchet went DOWN**: 90/13 → **89/13** — the plugin-reach edge is
  gone, and the **stale allowlist entry was deleted**.
  This is the first slice in this run that moves **no row at all**, so what
  the slice pins is the **shape of a reach**: the port has exactly three
  methods, the reload answers the **error only** (the catalogue the reload
  produced is the daemon's business, and the plugin may not name the catalog
  module — that narrowing is what made an adapter necessary), and the profile
  edit answers the contract's own `agentapi.Profile` row (the roster module's
  `Profile` is that row's alias, so the kernel's method already fits the port
  without a copy).
  The **nil-port rule** is stated twice on purpose — at the composition root
  and in the adapter — and each is measured on its own: a daemon with no
  kernel hands over a **nil** port, not a port wrapping a nil implementation,
  and the tool degrades instead of dereferencing nothing (no daemon → no
  agent overrides; an agent-scoped write without a daemon → "agent-scoped
  config needs a live kernel").
  The **behaviour that changed** is recorded honestly: the run's overrides
  used to be read by a package-level function, so a tool built without a
  daemon still saw them; now they arrive through the port, so a daemon-less
  tool sees none — in the daemon nothing changes, and the only difference is
  for a tool that was never wired to a daemon at all.
  The permanent tests: the **port's shape** (exactly three methods, each
  signature pinned), the **adapter** (a missing kernel is a nil port, the
  profile edit reaches the **live** roster, an unknown agent is answered as
  "not found" and no error, the overrides are a **copy**), the **tool names
  the contract** (the port field IS the contract's port, `SetKernel` takes
  the contract's port), the tool's **behaviour through the port** (no daemon
  → no overrides, the agent-scoped read, the unknown agent, the live-apply
  reload, the restart-apply field that must NOT reload, the failed reload
  that must be reported) and the **wiring, twice**.
  The plugin's own test was **rebuilt on a fake port** — the point of a port
  is that the tool is testable without a daemon — and the real delegation is
  covered by the adapter's test.
  19 mutations: 17 caught, 2 equivalent — seven were re-authored (two
  **compile errors**, five **stale anchors**) so the code still compiles; the
  two equivalent ones are recorded as such (the provider rebuild's failure is
  not reachable from a test; the composition root's nil check is redundant
  with the adapter's — defence in depth, kept on purpose).

- W3.33 (the guardian seeder's two reaches become one port) is committed on
  `main` the same way. The guardians are the daemon's internal fleet, and the
  seeder that plants and reconciles them is a plugin. It reached into two
  places, and both were the same mistake in different clothes: it carried a
  `kernelHost` adapter **inside itself**, holding the live `*runtime.Kernel`
  (a plugin may not hold the daemon's implementation), and it named the
  roster module for two rows — `roster.Profile` and `roster.NoisePolicy` —
  which are **aliases of the contract's own rows**, so the plugin was
  reaching into a module to name rows that already lived in the contract.
  The seeder's **port** moved to the contract layer
  (`kernel/contract/guardianapi`) with the eleven methods the fleet needs
  across the roster, the standing orders and the schedules, carrying the
  contract's own rows; the **adapter** moved to the app layer
  (`kernel/app/guardians`); and the plugin keeps no implementation at all.
  The **ratchet went DOWN by TWO**: 89/13 → **87/13** — both `plugin-reach`
  edges are gone and **both stale allowlist entries were deleted**.
  The port is the whole of the reach: exactly eleven methods, and a twelfth
  would be a new reach into the daemon and would have to be earned.
  A **nil host is refused, not called into** — a daemon that handed over
  nothing must hear that the fleet was not seeded, not panic on the first
  read — and the daemon's seeding is measured against a **live** kernel: the
  guardians the seeder reports are in the **roster** afterwards.
  The **rules that were never measured** are measured now: the guardians'
  quiet defaults (silent on success, no memory writes) are the operator's
  contract, and they had never been asserted.
  The plugin's test files now speak the contract as well; the only module
  left in the plugin is the roster's **validation rule** (`roster.Validate`),
  which reads the edict's trust ceiling and is therefore not pure — it stays
  in the module.
  13 mutations: 13 caught — one **survived** the first round (the adapter
  dropped the profile edit and the test did not notice, because the roster's
  own system-guardian defaults had filled a noise policy of their own and the
  assertion was looking at exactly that field), so the test was strengthened
  (the edit is now measured on a field the roster does not default) and it
  was caught on the second round; one produced a **compile error** and was
  re-authored; and one is measured by the **archcheck gate** rather than by
  a test, because the roster module's `Profile` is an alias of the contract's
  row — a plugin that named it again would compile and pass every test in
  the package, so the gate is the only thing that can see it.

- W3.34 (the briefing rows move to the contract layer) is committed on
  `main` the same way. The **pulse** engine composes the brief — the message
  the daemon pushes at the operator when something is worth saying — and the
  surfaces that deliver it (the daemon log, the Telegram chat, the Slack
  channel, the push surface, the webhook) are the built-in channels, which
  are a **PLUGIN**. The plugin reached into the pulse **module** for exactly
  one thing: the row. `pulse.Brief` is the message the engine composes and
  the plugin delivers, and `pulse.BriefSink` is the seam between them, so a
  plugin that names them reaches into a module to name something that was
  never the module's to own.
  The row therefore moved to the contract layer
  (`kernel/contract/pulseapi`): the brief, its **disposition** with its five
  values, the sink port and the three sink implementations (`LogSink`,
  `SinkFunc`, `MultiSink`) — all of it pure, so it belongs where the row is.
  The module keeps every name as an **alias**, so there is one row and one
  spelling of it, and the **composition** stays behind in the module
  (`composeBrief`, `composeDigest`, the quiet-hours window, the salience
  rules, the engine's loop) — that is the implementation, not the row.
  The **channel factory layer** (`kernel/channelwire`) named the pulse
  module for the same row and now names the contract too — which closed a
  **second** edge and removed `kernel/agent` from that layer's transitive
  dependencies, which is exactly what the layer's own package doc said it
  was there to prevent.
  The **ratchet went DOWN by TWO**: 87/13 → **85/13** — both edges are gone
  and **both stale allowlist entries were deleted**.
  The **wire matters** here: the brief is delivered to a human, so its JSON
  keys are a contract with every consumer outside the package, and they had
  never been measured. They are now pinned key for key — the four
  always-present keys (`title`, `disposition`, `issue_key`, `items`) and the
  two that are **dropped when empty** (`body`, `correlation_id`).
  The five **dispositions** are wire too, and all five are pinned by value.
  The **format the operator reads** is pinned in both its renderings — the
  daemon log's `📣 BRIEF [notify] the fleet is quiet` line and the chat
  surface's `📣 three things` message — two renderings of one row, both
  previously unpinned.
  The **sinks' rules** are the operator's contract and are pinned too: a
  sink with **no writer** is a no-op rather than a panic, a **multi sink**
  fans out to **every** sink even when one fails and answers the **first**
  error (a Telegram outage must not suppress the daemon log line), and a
  **nil sink** is skipped rather than called into.
  The **identity** is pinned as well: the module's rows **are** the
  contract's rows by type, not by shape, and the plugin's rows are the
  contract's — a copy would drift.
  14 mutations: 14 caught — none survived, none was a compile error, none
  was skipped on a stale anchor; and two of them are measured by the
  **archcheck gate** rather than by a test, because the module's `Brief` is
  an alias of the contract's row, so a plugin (or the channel factory
  layer) that named the module again would compile and pass every test in
  the package.

- W3.35 (the ACP bridge's two reaches become a platform client and a port) is
  committed on `main` the same way. The **acp_agent** tool — the bridge that
  delegates a task to an *external* agent that speaks the Agent Client
  Protocol — is a plugin, and it reached into two places that were two
  different mistakes: it named the **ACP protocol client**, which lived in
  `kernel/acp` (the daemon's ACP **server**, the inbound adapter an IDE
  drives), and it named the **catalogue** of installed ACP agents, which is a
  module's business.
  The two halves are handled the two ways the architecture allows: the
  client is protocol infrastructure with no domain knowledge, so it **moved
  down** to the platform layer (`kernel/platform/acpclient`, with `git mv`,
  so the history follows the code) — a plugin may reach the platform — and
  the shared JSON-RPC wire and message framing moved with it, because the
  server and the client speak the same wire; the catalogue answers through
  the contract's **port** (`kernel/contract/acpapi`, three methods), the
  app-layer adapter (`kernel/app/acpbridge`) answers that port from the
  module, and the daemon hands the port over at the composition root.
  The **ratchet went DOWN by TWO**: 85/13 → **83/13** — both `plugin-reach`
  edges are gone and **both stale allowlist entries were deleted**.
  The **security rule travels with the port**: the catalogue's resolution is
  a CWE-78 rule (the selector arrives in agent/LLM tool input, so it is
  treated strictly as a slug and the resolved command comes from the trusted
  catalogue, never from the caller's string), and it is now measured **at
  the port** (an injected selector never resolves to a command) and **at the
  tool** (it asks the port, and a tool with no port says so instead of
  calling into nothing).
  The slice found a **measurement gap of its own**: the adapter's test first
  compared the adapter's answers against the catalogue's own, which is
  vacuous on a host where no ACP agent is installed — every CI machine — so
  an adapter that lied about it passed. The test now pins a **known**
  catalogue state, and the two mutations that survived the first round were
  caught.
  The plugin's **test** no longer reaches the adapter either: the fake
  external agent now speaks the JSON-RPC wire directly (test imports do not
  enter archcheck, and a test-only reach is one edit away from a production
  one), while the client↔server round-trip proof moved to the **server's**
  package, which may reach the client.
  13 mutations: 13 caught — two survived the first round (the vacuous
  comparison) and were caught after the test was strengthened; one produced a
  **compile error** and was re-authored; and the last two are measured by the
  **archcheck gate** rather than by a test.

- W3.36 (the channel factory layer moves down to the platform layer) is
  committed on `main` the same way. The **channel factory layer** — the seam
  between the daemon and the channel plugins, where each channel *kind*
  registers a `Factory` and the daemon walks the manifest registry calling
  `BuildKind` per kind — was classified as a module
  (`kernel/channelwire`, L3, module "channels"), so the built-in channels
  plugin could not name it without an allowlist entry. It was never a
  module's business: it depends only on the contract layer (the channel
  vocabulary, the briefing row) and the platform layer (the bus, the
  settings), it carries no domain knowledge beyond the vocabulary itself,
  and it is the wiring between the daemon — which owns the bus, the shared
  inbound handler and the configuration — and the channel plugins that
  build their transports against it.
  So it moved **down** to the platform layer
  (`kernel/platform/channelwire`, with `git mv`, so the history follows the
  code), and the explicit L3 rule in `layers.json` was deleted — it would
  have matched nothing and the "channels" module would have been left empty.
  The **ratchet went DOWN**: 83/13 → **82/13** — the `plugin-reach` edge is
  gone and the **stale allowlist entry was deleted**.
  This is the same shape as W3.35 (the ACP client): a package that mixes an
  **adapter's** job with **infrastructure's** job ends up unreachable from
  the plugins that legitimately need the infrastructure half — the fix is to
  split the layer, not to allowlist the reach.
  A pure move must not change a behaviour, so the slice's proof is about
  the reach and about the behaviours the move could have broken: the
  registration, the lookup, the built instances, the brief sink on the
  instance, the config getter and the **account label** the factory
  receives.
  The slice found a **measurement gap of its own**: the mutation "the
  factory hands over the empty account label instead of the labelled one"
  **survived** the first round, because no test built a *labelled* account
  through `BuildKind` — and the label is what tells a factory WHICH account
  it is building (its config getter reads `<base>#<label>`), so a factory
  that always saw the empty label would build the default instance's
  configuration twice and the labelled account would never be configured at
  all. The test now pins a labelled account of a real kind
  (`AGEZT_TELEGRAM_TOKEN#bot2`) and asserts which label each factory call
  saw.
  8 mutations: 8 caught — one survived the first round (the label) and was
  caught after the test was strengthened; one was **ineffective** (it added
  the layer rule where the first-match rule already won, so the
  classification never changed) and was re-authored; and the two gate-gated
  ones say what only the gate can say — that the **classification** is the
  rule, and that the gate still sees this plugin's reaches at all.
  The remaining plugin-reach edges are the **registration seams**: the
  built-in tools reach `kernel/toolreg` (the tool registry), `kernel/runtime`
  (the live kernel) and the plugin host. Those are the hard ones to invert:
  the plugin *registers into* a registry the daemon owns, which is a
  dependency in the opposite direction of the one the rule forbids.

- W3.37 (the plugin host's reach becomes a contract's rows and a spawn port)
  is committed on `main` the same way. The built-in tools plugin hosts
  **out-of-process plugins** — the child processes the daemon spawns, whose
  tools register into the daemon's tool registry — and it named the module
  that owns the child process. It named it for two different reasons, and
  the two want different answers.
  The operator's declarations (`AGEZT_PLUGINS`, `AGEZT_PLUGIN_PINS`,
  `AGEZT_PLUGIN_TOOLS`) are **pure rows with pure parsers**, and a row a
  plugin names belongs in the contract layer — so they moved there whole,
  with `git mv`: the `PinSpec`, the `ToolAllowlistSpec`, the
  `PluginSpecEntry`, their three parsers and the BLAKE3 pin format check
  (the parser that accepts a pin and the host that verifies the binary
  against it must agree on what a pin looks like — one spelling of it).
  The **spawn** itself is the host's implementation, so it goes through the
  contract's **port** (`pluginapi.Spawner`, one method) and the daemon hands
  the implementation over at the composition root; the app-layer adapter
  (`kernel/app/pluginhost`) answers the port from the module. The running
  plugin answers **three verbs** through the port's other side
  (`pluginapi.Spawned`): its tools, their declared capabilities and the
  close — the host's own verbs (invoke, reload, liveness) stay with the
  host, because a plugin that could name them could drive the process
  directly.
  The **ratchet went DOWN**: 82/13 → **81/13** — the `plugin-reach` edge is
  gone and the **stale allowlist entry was deleted**.
  The slice was careful about **which rules travel**: the *slug*-style rules
  moved with the rows (a malformed pin is refused, a **duplicate prefix is a
  hard error** — a duplicate pin would otherwise silently keep the last
  hash, shadowing the operator's intended one, M216 — and an empty tool
  allowlist is refused rather than read as "deny everything"), while the
  *binary* rules stayed with the module (the digest of the file is compared
  against the operator's pin and a mismatch is a refusal, M1.ff; an oversized
  frame tears the plugin down rather than letting an untrusted child drive
  the host to OOM, M177; a callback the operator did not wire is rejected;
  the child's environment is the **scrubbed** base plus exactly the
  variables `AGEZT_PLUGIN_ENV` grants).
  The **test moved with the promise — and split in two**: the old test
  proved the strongest thing (a real probe plugin, spawned, asked what it
  saw in its environment), but it needed the host's module, which the plugin
  may not name and which the gate cannot see anyway (test imports do not
  enter archcheck). So the promise was **split into the two halves that make
  it**, and both are measured: the plugin side measures the **request** it
  hands the port (no `OPENAI_API_KEY`, no `AGEZT_VAULT_PASSPHRASE`, the
  granted variable present, `PATH` present, and the operator's **pin** and
  **tool allowlist** carried along), and the host side measures the child
  against a **real child process** (the same probe, now a shared fixture in
  `kernel/internal/testfixtures`), including that a binary which does not
  match the operator's pin **does not start**.
  12 mutations: 12 caught — none survived, none was a compile error, none
  was skipped on a stale anchor; and the last one is measured by the
  **archcheck gate** rather than by a test.

- W3.38 (the registry's pre-Open phase becomes a contract's port) is
  committed on `main` the same way. The tool registry drives every first-party
  tool through four phases — build, **pre-Open**, configure and late — and the
  pre-Open phase was the one whose hook signature leaked the daemon:
  `PreOpen func(tool toolapi.Tool, cfg *runtime.Config)`. The built-in tools
  plugin is a PLUGIN, and `*runtime.Config` is the daemon's **live
  configuration type**, so the hook forced the
  `plugin-reach plugins/builtintools -> kernel/runtime` edge into the
  allowlist. But the hook did not need the configuration — it needed **one
  verb** on it: install the **script runner** the script-tool forge (M794)
  executes forged tools through, before the kernel opens, so the forge is live
  from the first run.
  That verb moved to the contract layer (`kernel/contract/toolregapi`), which
  declares **its own** `ScriptRunner` interface (the daemon's runner type
  lives in a module, and a contract may not name a module — the two have the
  same method set, so the daemon's runner satisfies the contract's one and the
  value is assigned directly) and the **port** `Config` with **exactly one**
  method, `SetScriptRunner`. The hook now answers the port and the plugin's
  import of `kernel/runtime` is **gone**. The registry — the one place that
  knows both the contract and the daemon — wraps the daemon's configuration in
  a small adapter (`runtimeConfig`) and hands the port to each spec.
  The **ratchet went DOWN**: 81/13 → **80/13** — the `plugin-reach` edge is
  gone and the **stale allowlist entry was deleted**.
  The **shape of the reach is pinned**: the port has exactly one method, the
  runner exactly one, with its four arguments and three answers pinned by type
  — written the way Go actually declares an interface method (**no receiver**:
  `m.Type.In(0)` is the first *argument*).
  The **rules that travelled** are the *shape* rules; the **rules that stayed**
  are the *runtime* ones — when the phase runs (after the tools are built,
  before `runtime.Open`), which spec's hook runs at all (only the specs that
  actually built a tool), and what the daemon does with the installed runner.
  The **wiring is measured against the built instance** — after
  `ApplyPreOpen`, `cfg.ScriptRunner` must be the `*codeexec.Tool` the spec
  built — and the **gated-off** case is measured too: with
  `AGEZT_SANDBOX=off` the tool is never built, so the phase must leave
  `cfg.ScriptRunner` **nil** rather than install a runner for a tool that does
  not exist.
  6 mutations: 6 caught — one was **re-authored** because it produced a
  *compile error* rather than a measured failure (deleting the hook left the
  contract import unused, so the import was deleted with it and the mutation
  became a real "the plugin no longer takes part in the pre-Open phase"),
  and the last one is measured by the **archcheck gate** rather than by a test.
  **One plugin-reach edge remains in the whole tree**
  (`plugins/builtintools -> kernel/toolreg`) — the registration seam: the
  plugin *registers into* a registry the daemon owns, which is a dependency in
  the opposite direction of the one the rule forbids. The registry's rows carry
  the live kernel, so that slice is bigger: the registry's rows have to be
  broken apart first, one port family at a time.

- W3.39a (the registry's two module dependencies become contract's ports) is
  committed on `main` the same way. The tool registry is a module (L3,
  "runs"), and its **dependency rows** — the `KernelDeps` and `LateDeps` the
  daemon hands to the tools — named two OTHER modules' concrete types:
  `Artifacts *artifact.Index` and `Board *board.Store`. The registry never
  called a method on either of them: it holds them, hands them to the tools
  and gets out of the way, so it was carrying a module's **implementation** in
  a row that is really a row. The tools were never the problem — each one
  already held its own narrow private interface (`artifacts.Index`,
  `fetch.Indexer`, `browser.actionArtifactIndexer`,
  `codeexec.artifactIndexer`) and received the concrete index by assignment;
  the leak was entirely in the row that carried it. So the artifact index's
  port is **new** in the contract layer (`kernel/contract/artifactapi`, four
  verbs — `PutEntry`, `List`, `Bytes`, `Delete` — over the contract's own
  `remoteapi` rows), and the board store's port **already existed**
  (`boardapi.Store` — the board tool has been taking the store through it
  since it was written).
  The **ratchet went DOWN by TWO**: 80/13 → **78/13** — both `cross-module`
  edges are gone and **both stale allowlist entries were deleted**.
  The slice found the trap the change was walking into: the rows were
  concrete pointers, so `d.Board != nil` meant "there is a board" — they are
  interfaces now, and a nil pointer handed to an interface parameter is a
  **non-nil interface**. This is not theoretical: **`lateBoard` really is nil
  on a live path** (the daemon opens the board late and leaves the store nil
  when it could not be opened), so a naive change would have made the board
  tool bind a store that was not there and **panic on the first post** instead
  of reporting that the board is unavailable. The daemon therefore grew the
  same nil-port rule the lake and the forge store already follow
  (`artifactPort`, `boardPort`), and the rule is measured at **both** ends.
  The **shape of the port is pinned** (exactly four verbs; every type it
  names is a contract row or a builtin), and the **tools keep their narrow
  interfaces**: an interface value satisfies a narrower interface when its
  method set covers it, so the daemon hands over one port and each tool still
  receives exactly the slice it declares.
  7 mutations: 7 caught — two measured by the **archcheck gate** (the
  module's index and store are concrete, so a registry naming them again
  would compile and pass every test in the package), and two of them the ones
  that matter most — the nil-safety of the daemon's ports, which is the
  difference between "the board is unavailable" and "the daemon panics on the
  first post".
  **What is left of the registry's rows**: `KernelDeps.K` (the live kernel —
  the heavy one, eleven specs and nineteen call sites), `KernelDeps.Bus`,
  `KernelDeps.Journal` and `BuildDeps.Warden`. Each is the next sub-slice.

- W3.39b (the reasoning runners become the contract's ports) is committed on
  `main` the same way. The registry's `KernelDeps` carries the **live
  kernel** because eight tools need something only the kernel has — and three
  of them needed it for exactly one reason: their **runner**. The runner
  interface was already the right shape (a port, one verb, naming only the
  contract's rows since W3.13) but it lived in the **tool's** package, and
  the tool's package is a plugin (L6) — so the registry, a module (L3),
  could not hand the runner over: a module may not name a plugin's type, and
  the row had to be the live kernel instead. That was the whole leak — a
  runner the tool already declared as a port, carried by the kernel because
  the port was in the wrong layer.
  So the three runners moved to the contract layer
  (`kernel/contract/reasoningapi`: `CouncilRunner`, `ConductorRunner`,
  `ResearchRunner` — one verb each — and `Runner`, the three together, which
  is the row the registry carries), the tools keep their names as **aliases**
  (so every caller and every fake in every tool's test is unchanged),
  `KernelDeps` gained `Reasoning reasoningapi.Runner`, the three specs bind
  `d.Reasoning` instead of `d.K`, and the daemon supplies the port with the
  same nil-port rule the other ports follow.
  **The ratchet did NOT move — 78/13, unchanged — and that is the point of
  the slice, not an omission**: the registry's **rows** are still in the wrong
  layer. The last `plugin-reach` edge
  (`plugins/builtintools -> kernel/toolreg`) falls only when the rows (the
  `Spec`, the `Built`, the `BuildDeps`, the `KernelDeps`, the `LateDeps` and
  the registry itself) live where a plugin may name them, and that needs
  **every** field of every row to be a contract row or a contract port. This
  slice removes the first of the five families that block it; the other four
  (workflow/workboard, MCP, forge, schedule/standing/skill/journal, and the
  platform rows — the warden, the bus and the journal) are each the same
  shape: a row that names a type a plugin may not name, replaced by a port.
  The ports are **aliases**, so the tool's runner and the contract's port are
  the *same type*, and the identity is asserted by comparing the **types
  themselves** (`reflect.TypeOf`), not by an assignment — an assignment would
  accept a copy with the same method set, while two interface types with
  identical method sets are still different types.
  The **plugin's** measurement records the call: "the tool did not complain"
  is not enough, because a tool could report unavailability for another
  reason and still look wired.
  7 mutations: 7 caught — and there is **no gate-measured mutation in this
  slice**, which is a fact about the slice rather than an omission: the
  registry's row for the live kernel is a **same-module** edge
  (`kernel/toolreg` and `kernel/runtime` are both the "runs" module), so
  archcheck cannot see it — there is no violation for it to see, and the
  shape is pinned by the **compiler** instead (the kernel must implement the
  port, or nothing builds) and by the shape test.

- W3.39c (the workflow and workboard runners become the contract's ports) is
  committed on `main` the same way. W3.39b moved the reasoning runners; the
  same shape was sitting in two more places, and this slice takes both. The
  workflow tool's port (three verbs: save, arm, run) and the workboard
  tool's port (thirteen verbs) were **already the right shape** — a port
  naming only the contract's rows — but both lived in the **tool's**
  package, which is a plugin (L6), and the registry is a module (L3): a
  module may not name a plugin's type, so the registry's row had to be the
  **live kernel** instead of the runner. These two were the cleanest of the
  five families: every row they name is already the contract's.
  So the two ports moved to the contract layer
  (`kernel/contract/workflowapi.Runner`, `kernel/contract/workboardapi.Runner`),
  the tools keep their names as **aliases** (every caller and every fake
  unchanged), `KernelDeps` gained `WorkflowRunner` and `WorkboardRunner`, the
  two specs bind the new rows instead of `d.K`, and the daemon supplies both
  ports under the same nil-port rule the other ports follow. The **read**
  side was already a port (the W3.39a store) and stays one: the store is the
  read side, the runner is the write-and-run side, and the tool needs both.
  **The ratchet did NOT move — 78/13, unchanged — for the same reason as
  W3.39b**: the registry's **rows** are still in the wrong layer. This slice
  removes the **second** of the five families that block it.
  The workboard port is **thirteen verbs wide**, and that is worth naming: it
  is not a "host" interface that swallows the kernel, it is the board's whole
  surface — the workboard tool is a first-class actor on it — and every one
  of the thirteen names a contract row.
  Two **existing tests** had to be re-pointed at the new row
  (`workflow_ports_test.go`, `workboard_ports_test.go`): they measured the
  **store** port's wiring and had been reaching the live kernel through
  `d.K` to get the runner as well; the runner now arrives through its own
  port, so the tests supply it there — what they measure and what they
  assert are unchanged.
  8 mutations: 8 caught — one was **re-authored** because it produced a
  *compile error* rather than a measured failure (the mutation declared only
  one of the thirteen verbs, so the tool's other twelve calls did not
  compile; the re-written mutation declares the whole port and the identity
  assertion caught it).
  **Three families remain**: the MCP row (`mcptool.Kernel` already names
  only `mcpapi` rows, so it is a clean candidate like these two); the
  schedule/standing/skill rows; and the platform rows — the warden, the bus
  and the journal, where the bus and the journal carry `event.Event` and
  therefore need an **adapter** rather than an alias. The forge row is a
  fourth: its port names the toolforge rows plus `approval.Decision`, which
  is a platform type — the port can move only after that row moves with it.

- W3.39d (the MCP and standing hosts become the contract's ports) is
  committed on `main` the same way. The same leak, two more times: the MCP
  tool's port (seven verbs — add, attach, detach, remove, list, look up,
  report what is attached) and the standing-orders tool's port (three verbs)
  were **already ports** naming only the contract's rows, but both lived in
  the **tool's** package — a plugin (L6) — while the registry is a module
  (L3): a module may not name a plugin's type, so the registry's row had to
  be the **live kernel**.
  So the two ports moved to the contract layer
  (`kernel/contract/mcpapi.Runner`, `kernel/contract/standingapi.Host`), the
  tools keep their names as **aliases**, `KernelDeps` gained `MCP` and
  `Standing`, the two specs bind the new rows instead of `d.K`, and the
  daemon supplies both ports under the same nil-port rule.
  The standing tool also checks an **agent profile** (it refuses a managed
  sub-agent's direct self-wake), and that lookup came from the roster
  module — but the lookup is not a **type**, it is a function value, and a
  function value names no type at all, so the registry carries it as a plain
  closure (`AgentLookup func(ref string) (agentapi.Profile, bool)`) and the
  daemon builds it from the live roster. A daemon with no roster hands over
  **nothing** — a closure over a nil roster would panic on the first lookup
  where a nil closure leaves the tool without the lookup it can do without.
  **The ratchet did NOT move — 78/13, unchanged — for the fourth time** and
  for the same reason: the registry's **rows** are still in the wrong layer.
  This slice removes the **third** of the five families that block it.
  Two **existing tests** had to be re-pointed at the new rows
  (`mcp_ports_test.go`, `trigger_ports_test.go`): they had been reaching the
  live kernel through `d.K` to get the runner as well, and the runner now
  arrives through its own port — what they measure and what they assert are
  unchanged.
  10 mutations: 10 caught — two were **re-authored** because they produced
  *compile errors* rather than measured failures (each declared only part of
  the port, so the tools' other calls did not compile; the re-written
  mutations declare the whole port and the identity assertions caught them).
  There is **no gate-measured mutation**, for the same reason as W3.39b and
  W3.39c: the registry's row for the live kernel is a **same-module** edge,
  so archcheck cannot see it — the shape is pinned by the **compiler** (the
  kernel must implement the ports, or nothing builds) and by the shape tests.
  **Two families and the final move remain**: the schedule/skill rows (the
  schedule tool already takes the cadence store through the contract's port
  `cadenceapi.Store`, but the spec reaches the store and the roster through
  the live kernel, so the registry needs a store row and a lookup row), and
  the forge row plus the platform rows (the warden, the bus and the journal,
  where the bus and the journal carry `event.Event` and therefore need an
  **adapter** rather than an alias). When the last of them is gone, every
  field of every row is a contract row or a contract port — and the
  registry's rows can move to the contract layer, which is where the last
  `plugin-reach` edge falls with them.

- W3.39e (the schedule and skill rows become contract's ports) is committed
  on `main` the same way. Unlike the three slices before it, nothing had to
  be *designed* here: the ports the two tools need were already in the
  contract layer — the schedule tool takes the cadence store through
  `cadenceapi.Store`, the skill tool takes the forge and its bundle store
  through `skillapi.Forge` and `skillapi.BundleStore` — but the **spec**
  reached them through the live kernel (`d.K.Schedules()`, `d.K.Forge()`,
  `d.K.Roster().Get`), so the registry's row had to be the live kernel and
  the row's type was a module's.
  So `KernelDeps` gained three rows, all of them **existing** contract ports
  (`Schedules`, `SkillForge`, `SkillBundles`), the two specs bind them
  instead of `d.K`, and the daemon supplies them nil-safely. The roster
  lookup the schedule tool needs is the **same closure** the standing tool
  already takes (W3.39d) — one row, two readers.
  **The ratchet did NOT move — 78/13, unchanged — for the fifth time** and
  for the same reason. This slice removes the **fourth** of the five families.
  **The slice's own finding is worth more than its number**: the mutation
  run produced a **survivor** — "the bundle store port stops being
  nil-safe" changed nothing — and it was not a weak mutation but a **real
  rule with no measurement**. The bundle store is **optional** (the forge's
  own doc says "nil leaves skills body-only", and `SetBundles` wires it only
  when the daemon wants bundles), so a daemon without bundles hands over a
  nil `*skill.BundleStore`, and without the check that nil pointer becomes a
  **non-nil interface**: the skill tool would bind a bundle store that is not
  there and panic on the first resource read. The rule was reachable but **no
  test walked that path**, because every test ran a daemon that had bundles —
  the measurement gap this project keeps naming ("a path nobody walks is not
  a path").
  The fix was **a test, not a weaker mutation**: the daemon's test now
  unwires the bundles (`k.Forge().SetBundles(nil)`) and asserts that the
  port stays a **nil** port, checked on the concrete type because a nil
  `*skill.BundleStore` handed to an interface parameter is already a non-nil
  interface.
  9 mutations: 9 caught — after the gap was closed.
  **One family and the final move remain**: the forge row and the platform
  rows. `forgetool.Kernel` names the toolforge rows plus `approval.Decision`,
  which is a **platform** type — the port can move only after that row moves
  with it, and moving it means moving the decision row to the contract layer
  with an alias. The **warden**, the **bus** and the **journal** are platform
  types too: the bus and the journal carry `event.Event`, which cannot move
  to the contract layer (it hashes with blake3), so those two need an
  **adapter** rather than an alias.
- W3.39f (the platform rows become the contract's ports) is committed on
  `main` the same way. The **fifth and last** family: the **isolation
  engine**, the **event bus** and the **journal** — the three rows the
  registry carried as *platform* types, which a plugin may never name. They
  were the hard ones because they do not merely name a platform type, they
  name one that carries rows the contract layer may not name at all: the
  bus's event carries a BLAKE3 hash chain, and the journal's event carries
  the durable envelope.
  So the rows were **split** rather than moved: the sandbox's `Profile`,
  `Limits`, `Spec` and `Result` moved **verbatim** to
  `kernel/contract/wardenapi` (the platform keeps them as **aliases**, so
  there is one row and one spelling of it); the event's `Kind` **type** and
  `Spec` moved to `kernel/contract/eventapi` (again as aliases — and that is
  what makes the bus take the contract's row: the daemon's adapter translates
  **nothing**, so what the code_exec tool publishes is exactly what the bus
  writes); and the journal's **read projection** — the four fields a reader
  folds — became `kernel/contract/journalapi.Record` behind a one-verb
  `Reader` port. The ports are deliberately **narrower** than the platforms
  they stand for: the warden's port is `Run` + `EffectiveProfile` and **not**
  `SetBus` (that verb names the bus, and the contract layer may not import
  it), and the publish port answers one thing — *did it land?* — where the
  bus answers two (the event it wrote carries the hash chain).
  **The ratchet did NOT move — 78/13, unchanged — for the sixth time** and for
  the same reason: the registry's **rows** are still in the wrong layer. What
  this slice changes is the **shape of the blocker**: after it, **every field
  of every row of the registry is a contract row or a contract port, with
  exactly one exception** — `KernelDeps.K`, the live kernel — and the registry
  imports `kernel/channel` and `kernel/runtime` and **no platform package at
  all**.
  The slice's finding is again a **rule nobody measured**: the mutation "the
  journal read port swallows the read error" survived, because the adapter's
  error path had no test walking it — every test ran a journal that could be
  read — and the rule matters: a port that answers "nothing found" for a read
  that **failed** tells the operator the daemon has no past runs, when the
  truth is that the history could not be read at all. That is the difference
  between "there is nothing" and "I could not look".
  The fix was **a test, not a weaker mutation**: the daemon's test now puts a
  segment the journal cannot decode into its directory and asserts that the
  port **reports** the failure.
  18 mutations: 18 caught — including a **gate-measured** one (a helper in the
  contract's warden package taking a `*bus.Bus`: archcheck's principle P4
  caught the contract naming the platform again); one mutation was
  **re-authored** because it produced a *compile error* rather than a measured
  failure (it pointed the runs tool's switch at a kind the switch already
  carried — a duplicate case).
  **The final move remains**: the live kernel's three uses — the forge tool's
  runner, the kernel's code executor, and the artifact bytes the send_media
  tool resolves — and the registry's own functions move down to the contract
  layer, and the last `plugin-reach` edge falls with them.
- W3.39g (the live kernel leaves the registry) is committed on `main` the same
  way. After W3.39f, **every field of every row of the registry was a contract
  row or a contract port with exactly one exception** — `KernelDeps.K`, the
  live kernel — and that one field was the whole remaining leak: the plugin
  never imported `kernel/runtime` (the **field's type** was the dependency),
  so removing it removes the first-party tools' last type-level dependency on
  the composition root's runtime.
  The field served three uses, and each got its own shape. The **forge's
  runner** was four verbs, and the fourth one named a **platform** type
  (`approval.Decision`) — so the row had to move with the port: the verdict
  moved to the contract layer (`kernel/contract/approvalapi`) with the
  platform keeping its name as an **alias** (all 92 usages across 30 files
  keep compiling unchanged), the port moved to `toolforgeapi.Runner`, and the
  tool's own `Kernel` became an alias of it. The kernel satisfies the new port
  **with no adapter at all** — its own methods already speak the contract's
  rows (the script tool and the verdict are both aliases), so the port is
  satisfied by identity rather than by a translation. The **Verifier's code
  executor** was one pure verb, and the contract **already had it**
  (`toolregapi.ScriptRunner`, from W3.38) with the exact signature — so the
  runtime's `CodeExecutor` became an alias of it, and the registry's row is a
  plain closure (a function value names no type). The **artifact bytes** were
  already a closure, so the row is the closure itself — the daemon builds it
  over the live blob store, and the store's hash re-verification travelled
  with it.
  The code_exec tool hands **itself** to the row, and a nil
  `*codeexec.Tool` in an interface parameter is a **non-nil interface** — so
  the hand-over is a named function with the check in it (`installExecutor`).
  **The ratchet did NOT move — 78/13, unchanged** for the seventh time and for
  the same reason: the registry's **rows** are still in the wrong layer. What
  this slice changes is that the registry's rows are now **all**
  contract-shaped — **with no exception left**.
  **15 mutations: 14 caught** — one is a **build error** rather than a measured
  failure, and that is the honest shape of it: the alias's identity is
  enforced by the **type system**, not by a test (the platform's own methods
  stop compiling when it stops speaking the contract's row); one was
  **gate-measured** (the contract naming the platform again — archcheck's
  principle P4).
  **Only the final move remains**: the registry's rows and its own functions
  (`Register`, `Names`, `BuildAll`, `Set`, ...) move down to the contract layer,
  the plugin imports the contract package instead of `kernel/toolreg`, and the
  last `plugin-reach` edge falls (**ratchet 78 → 77**). The only thing standing
  in that move is the registry's own reach for the daemon's configuration
  (`Set.ApplyPreOpen(cfg *runtime.Config)`), and even that has a home: the
  adapter moves to the composition root, which may name `*runtime.Config` —
  and `runtime.Config.ScriptRunner` is **already** the contract's runner.
  The slice's own full test run also surfaced **two Windows-only flakes**,
  both fixed in the same slice and both measured locally (no CI job runs on
  Windows): the skill bundle swap raced a scanner's handle and was refused
  with "Access is denied" (now a bounded Windows-only retry, the same remedy
  `internal/atomicfile` carries), and the warden's timeout test measured the
  machine's process-creation latency instead of the kill path (the deadline
  also pays for PATH resolution and `os/exec` creation, so the budget could
  be gone before the process existed — the run is now repeated with a budget
  that covers the start, and the wall-clock bound is relative to the budget).
  Both were weakened in no way: the measured run still has to start, run past
  its deadline, be killed and be flagged.
- W3.39h (the registry itself moves down to the contract layer) is committed on
  `main` the same way. After W3.39g the registry's **rows** were all
  contract-shaped, with no exception left — but the **registry itself** was
  still in `kernel/toolreg` (L3, module "runs"), and the built-in tools are a
  **PLUGIN**: a plugin may name the contract layer and nothing else. So the
  last `plugin-reach` edge was structural, not a row, and a half-move would
  have split the package across two layers for no reason — the plugin names
  **both** halves (it declares a `Spec` and it registers it) — so the rows
  **and** the registry's own functions (`Register`, `Names`, `BuildAll`,
  `Set`, ...) moved down together, into the existing
  `kernel/contract/toolregapi`. The four files went with `git mv`, so the
  history follows the code, and the 35 importers were rewritten (354
  references; seven of them imported **both** packages, so their two imports
  collapsed into one).
  Three names had to change with it, and two of them were free:
  `channel.Attachment` → `channelapi.Attachment` and
  `runtime.PluginInfo` → `types.PluginInfo` are both **aliases** the platform
  already kept, so the move changed the *spelling*, not the type. The third
  was the real work: `ApplyPreOpen(cfg *runtime.Config)` became
  `ApplyPreOpen(cfg Config)`, and the `runtimeConfig` adapter that answers
  the port over the daemon's configuration **moved to the composition root**
  (`cmd/agezt`) — because the registry could only wrap the daemon's
  configuration while it was a module that knew both, and as a contract
  package it may not name `*runtime.Config` at all. The plugin's own test
  stopped reaching for the daemon's configuration too: it speaks the **port**
  with a fake of it, so there is one spelling of the wire.
  **The ratchet went DOWN: 78/13 → 77/13.** The `plugin-reach` edge is gone
  (`plugins/builtintools -> kernel/toolreg`) and the **stale allowlist entry
  was deleted** — the check fails when a listed edge no longer occurs, so it
  could not stay. The `layers.json` rule for `kernel/toolreg` went with the
  directory, and the deadcodecheck seam key followed the file
  (`kernel/contract/toolregapi/registry.go|Names`).
  **14 mutations: 14 caught**, and **four of them are gate-measured** — the
  claim "the plugin no longer reaches a module" is not something a test can
  see, it is what archcheck counts. The slice also found a **gap in itself**:
  the daemon's pre-Open call sat in the middle of `runDaemon`, where no test
  reaches it, so "the daemon hands the phase a port built over its own
  configuration" was true and **unmeasured** — a change to `ApplyPreOpen(nil)`
  would have compiled, booted, and left the script-tool forge (M794)
  permanently unavailable with no error anywhere. The fix is the one W3.39g
  used: a **named function** at the composition root (`applyPreOpen`), so the
  call site is a thing a test can hold.
  The **reach was checked before it was built**: all 23 contract packages the
  registry names import the standard library and other contract packages only,
  so the move could not buy its edge by pulling a module in through a contract
  package — the indirect leak archcheck's direct-import rule cannot see was
  measured, not assumed.
  The full test run also dropped `tools/deadcodecheck`'s own tests — their
  fixture strings still named the old path — and the gate and its fixtures
  now name one path.
  **What remains**: W3's exit condition is that `kernel/runtime` and
  `kernel/controlplane` no longer exist and no type has more than 60 methods.
  The registry was the last thing standing between the built-in tools and the
  contract layer; what stands between the daemon and the contract layer now
  is the kernel itself.
- W3.40 (the kernel's dead accessors sub-manager is deleted) is committed on
  `main` the same way. The kernel used to wire a sub-manager that **nothing
  ever called**: `k.accessors = accessors.New(k)` in `compose.go`, and no
  reader anywhere in the tree. The package was a **second copy of the kernel's
  own read surface** — 48 production functions, 898 lines, 51 tests of its own
  — and it was unreachable **by construction**: the `*Kernel` bodies were
  supposed to become thin delegations to the `Accessor`, but an `Accessor`
  dispatches back into `*Kernel` through the implicit `KernelAPI` interface,
  so delegating would have recursed. The `*Kernel` kept the bodies and read
  the fields directly; the copy sat beside it and was never called. Both
  copies published the **same nine bus events** (standing created / updated /
  removed, roster created / updated / removed), so the deletion removes a
  forward, not a behaviour. The package went with `git rm` (five files),
  together with the `accessors` field, the import and the construction.
  `ConfigMu()` went with it — the deleted package was its only caller — while
  `Scheduler()` and `AgentGateway()` were **kept**, because they are required
  members of the live `runexec.KernelAPI`. The two comments that claimed the
  delegation were corrected to describe what was true.
  **The ratchet went DOWN by ELEVEN: 77/13 → 66/13.** Ten `cross-module`
  edges and one `upward` edge were deleted from the allowlist, and **every
  one of them was an edge of the deleted package**: the kernel's module reach
  was being counted **twice**, once for `kernel/runtime` and once for a copy
  that never ran. The ratchet counts edges, not whether the edges mean
  anything — so a dead package priced the allowlist for years. This is the
  largest single drop of the W3 series and the cheapest: no behaviour changed
  and the reach got smaller.
  The **loss of a compile-time check was compensated by name**: the deleted
  package's `KernelAPI` (≈45 methods) had proved that `*Kernel` answered
  every one of them, so `kernel/runtime/submanager_test.go` now pins the **42
  accessor names** by name, pins `ConfigMu`'s **absence** (a getter nobody
  calls is the same defect as a copy nobody calls), and pins the kernel's
  **sub-manager set** off the type — so a third sub-manager that is wired but
  never called is caught at **wiring** time.
  **6 mutations: 6 caught**, two of them **gate-measured** by archcheck (the
  package comes back with a module edge; the stale allowlist entry comes
  back), because "the kernel's reach is smaller" is not something a test can
  see. The **first harness run was invalid and thrown away**: the created
  file was saved as if it had an original, so it was written empty and
  archcheck failed for the wrong reason — the harness was fixed to mark a
  created file **create-only** and to refuse an empty package, and the run
  was repeated.
  **What remains**: the kernel root still reaches **22** modules and still
  returns concrete module types (`Memory() *memory.Manager`,
  `World() *worldmodel.Graph`, `Forge() *skill.Forge`, …). Those rows are the
  next slice — they are the rows that have to become contract ports.
- W3.41 (the kernel names the contract's rows, and holds the contract's port)
  is committed on `main` the same way. The kernel still reached 22 modules;
  three of those edges were three shapes of the same mistake, and all three
  are gone. The **proof** and **assure** edges were free: every row the kernel
  named was *already* the contract's row — the modules keep their names as
  **aliases** of `proofapi` / `assureapi` — so naming the contract instead of
  the module is a spelling change, not a type change, and no caller moved.
  The **seat** edge was the real work and is the shape the remaining
  nineteen edges need: **the kernel holds a port, the composition root owns
  the file**. `kernel/contract/seatapi` is new — the `Seat` row (pure data)
  and the `Store` port with the five verbs the host uses; the module keeps
  `type Seat = seatapi.Seat` and asserts `var _ seatapi.Store = (*Store)(nil)`,
  so a port change is a compile error in the module rather than a runtime
  surprise at the composition root. The kernel's field, accessor and `Config`
  now carry the **port**, `compose.go` opens nothing, and `cmd/agezt`'s
  `openAppKernel` — which already binds the reasoning module and the reaper
  over the kernel's own ports, exactly this pattern — opens the store and
  injects it. A store that cannot be opened is still a **startup error**: it
  is the daemon that refuses to boot now, not the kernel, so the daemon's
  behaviour is unchanged.
  A host given no catalog holds a **nil port**, and **both ends** of that
  rule are measured: the daemon's catalog is live (the built-ins are there,
  an unknown seat is refused), and the app side says *"the seat catalog is not
  available"* instead of calling into nothing — the workboard execution
  **degrades** to agent defaults (the posture an unknown seat takes), and the
  seat validity check **mirrors the port's own semantics**: an empty id means
  "no seat" and stays valid, a named seat cannot be confirmed and is refused.
  That last one was the slice's one subtlety: the first nil-validator
  refused *every* id and broke two existing workboard tests, because a task
  with no seat is not asking for an unknown seat.
  **The ratchet went DOWN by THREE: 66/13 → 63/13**
  (`kernel/runtime -> kernel/modules/work/{proof,assure,seat}`).
  **7 mutations, 7 caught**, two of them **gate-measured** by archcheck (the
  kernel opens the seat store again; a stale allowlist entry comes back),
  because "the kernel's reach is smaller" is not something a test can see: a
  test would happily accept a kernel that opens the store itself, as long as
  the store works. The new permanent tests pin the row's nine fields and JSON
  tags, the port's five verbs, that the kernel holds **exactly** the port it
  was given (not a wrapper of its own), and that a kernel given no catalog
  holds **nil** — and not a non-nil interface holding a nil pointer, which is
  the typed-nil trap.
  **What remains**: the kernel still reaches **19** modules. The next
  candidates are the ones whose rows are already in the contract and whose
  kernel-side use is a handful of verbs — `workflow`, `workboard`,
  `toolforge`, `artifact`, `datalake` all have contract ports today — and the
  three media tool seams (`imagetool`, `reranktool`, `voicetool`), which are
  blocked behind the artifact store: the voice and image tools need it, so the
  daemon has to own that store before the daemon can own those tools.
- W3.42 (the data lake is the daemon's file) is committed on `main` the same
  way. It takes **two** edges at once — the kernel's own and the run engine's
  — and it is the cheapest of the remaining ones **because the contract side
  was already done**: `kernel/contract/datalakeapi.Lake` (nine verbs)
  existed, the module already asserted it
  (`var _ datalakeapi.Lake = (*Lake)(nil)`), and every row the lake carries
  was already an alias of the contract's. **Nothing had to move.** What was
  missing was the kernel's own use: it opened the lake, seeded it, and held
  the concrete type. Now the kernel holds the **port** and the composition
  root owns the **file** — the shape W3.41 established for the seat catalog.
  The **seeding moved with the file**, and it is worth saying why:
  **seeding is not part of the port.** A port says how to drive a thing; it
  does not say how the thing is prepared. The daemon seeds the lake while it
  still holds the concrete one. The posture is unchanged — a lake that cannot
  be opened is a **startup error**, and a seeding hiccup is still
  best-effort.
  The daemon's `lakePort` wrapper — the one place where a missing lake could
  have become a non-nil port — is **gone with the wrapping**: the kernel
  holds the port, so there is nothing to wrap, and the nil-port rule moved
  into the kernel, where the field is an interface and a kernel given no lake
  holds a **nil** interface rather than a nil pointer in an interface.
  **The ratchet went DOWN by TWO: 63/13 → 61/13.**
  **6 mutations, 6 caught**, two of them **gate-measured** by archcheck (the
  kernel opens the lake again; a stale allowlist entry comes back), because
  "the kernel's reach is smaller" is not something a test can see.
  **What remains**: the kernel still reaches **17** modules. Next, in order:
  **`artifact`** — the only one that unblocks the three media tool seams
  (`imagetool`, `reranktool`, `voicetool`), because the voice and image tools
  are bound to the artifact store *inside the kernel*; it needs new contract
  surface (a blob port, a wider index port, and `artifact.RefGuard`).
  **`toolforge`** — the rows are aliases, but the kernel calls **eight** store
  methods the contract's two-verb port does not cover. **`workflow` and
  `workboard`** — both need wide new ports: the kernel names
  `workflow.Engine`, `workflow.Ports`, `workflow.Provenance` and
  `workflow.Trigger` (no contract home), and it calls **seventeen** workboard
  store methods.
- W3.43 (the artifact store is the daemon's file) is committed on `main` the
  same way. It takes **two** edges at once — the kernel's own and the run
  engine's — and it is the one that unblocks the three media tool seams
  (`imagetool`, `reranktool`, `voicetool`). Unlike W3.42 the contract side did
  **not** exist, so the slice had to write it: `kernel/contract/artifactapi`
  gains the **Blob** port (four verbs), the **RefGuard** port (one verb) and
  the **index** ports. The index needed **two**, not one, and the first attempt
  was **wrong**: widening the existing four-verb `Index` to ten broke
  `TestTheIndexPortHasExactlyFourMethods`, a guard that is not a style rule — a
  plugin holds every method of the port it is given, and two of the host's
  verbs are dangerous (`Collect` deletes entries; `SetRefGuard` switches off
  the journal's protection of the bytes the journal still names). So the port
  is **split**: `Index` keeps exactly the four tool verbs, `HostIndex` carries
  those four plus the six host verbs, and the module answers both with two
  assertions. The kernel's fields, its accessors and the run engine's
  `KernelAPI` carry the ports; `compose.go` opens nothing; `cmd/agezt`'s
  `openAppKernel` opens the store and the index and injects both. The **ref
  guard did not move to the daemon**: it answers from the **journal**, and the
  journal is the kernel's, so the kernel installs its own guard on the index it
  was given — only the *port* moved, because the host has to install a guard
  on an index it does not own. The daemon's `artifactPort` wrapper is **gone
  with the wrapping**, exactly like the lake's in W3.42.
  The slice's real cost is the tests: a kernel opened without the store has no
  artifact store at all, so every test that expected artifacts had to
  **become a host** (open the store, inject the ports). Two suites failed the
  first run for exactly that reason — `plugins/builtintools` and
  `kernel/cadence/systemtasks` — and were fixed the same way: the test became
  the host, not a weaker assertion.
  **The ratchet went DOWN by TWO: 61/13 → 59/13.**
  **8 mutations, 8 caught**, three of them **gate-measured** by archcheck (the
  kernel opens the store again; the run engine names the concrete index; a
  stale allowlist entry comes back).
  The mutation run was **honest about itself**: the first pass passed
  `go test <pkg> -run <name>` as one argument, so `go test` read the whole
  string as a directory name and **five mutations were "caught" by a malformed
  command** whose output happened to contain the expected test name — a catch
  for the wrong reason is not a catch, so the run was discarded; the second
  pass found one **build error** (removing the daemon's block left the
  `artifact` import unused, and a compile error is not a catch either), so the
  run was repeated a third time: **8 caught, 0 survived, 0 skipped, 0 build
  errors**.
  **What remains**: the kernel still reaches **17** modules. Next, in order:
  **the three media tool seams** (`voicetool`, `imagetool`, `reranktool`) —
  the voice and image tools are bound to the artifact store *inside the
  kernel*, and with the store now a port the daemon can build the tools and
  inject the closure itself. **`toolforge`** — the rows are aliases, but the
  kernel calls **eight** store methods the contract's two-verb port does not
  cover. **`workflow` and `workboard`** — both need wide new ports: the kernel
  names `workflow.Engine`, `workflow.Ports`, `workflow.Provenance` and
  `workflow.Trigger` (no contract home), and it calls **seventeen** workboard
  store methods.

- W3.44 (the media tools are the daemon's) is committed on `main` the same
  way. It takes **four** edges at once, and they are one thing: the **voice,
  image and rerank tools** (`kernel/runtime -> kernel/voicetool`, `->
  kernel/imagetool`, `-> kernel/reranktool` and `kernel/runtime/runexec ->
  kernel/voicetool`). The kernel built the three tools because the tools live
  in their own packages and the kernel held the adapters they were built over
  — and naming the tool package meant naming the **seam** with it. The way out
  is not a narrower import but a different owner: **the composition root
  builds the three tools** (`cmd/agezt/media_tools.go`) and hands them over as
  ordinary tools in `cfg.Tools`, the way the daemon already hands over its
  reasoning module and its reaper. The **seams became contract rows**:
  `kernel/contract/mediaapi` is new (`Voice`, `ImageGen`, `Reranker`), the
  tool packages keep their names as aliases, and the kernel names the
  contract's row. The seams stay **structural** — every method set speaks only
  in standard library types — which is what lets a provider plugin satisfy
  them without importing the kernel, and the contract now pins that with a
  test. The kernel keeps the **one** accessor it needs: `k.Voice()` answers
  "is speech configured?" for the daemon's channel paths and the web UI. The
  two tools that persist bytes bind **the daemon's own artifact store** — the
  same store the kernel holds (W3.43) — so the bytes land exactly where they
  did before. The tool map is **copied, never mutated**: the primary kernel
  and every tenant share one map, and each kernel must see its own tools bound
  to its own store.
  **The ratchet went DOWN by FOUR: 59/13 → 55/13.**
  **8 mutations, 8 caught**, four of them **gate-measured** by archcheck (the
  kernel names the voice package again; the run engine names the concrete
  voice type; the kernel builds the voice tool again; a stale allowlist entry
  comes back).
  **What remains**: the kernel still reaches **17** modules. Next, in order:
  **`toolforge`** — the rows are aliases, but the kernel calls **eight** store
  methods the contract's two-verb port does not cover. **`workflow` and
  `workboard`** — both need wide new ports: the kernel names
  `workflow.Engine`, `workflow.Ports`, `workflow.Provenance` and
  `workflow.Trigger` (no contract home), and it calls **seventeen** workboard
  store methods. **`market`** — the manager's contract port does not exist
  yet; the tool has to move to the daemon with it. **`cadence`, `standing`,
  `mcp`** — many callers each; the shape is known (a contract port the host
  opens), the work is in the port's width.
- W3.45 (the script-tool store is the daemon's file) is committed on `main`
  the same way. It takes **one** edge — the last of the *toolforge* family —
  and it closes the family: the kernel used to open the store, hold the
  module's concrete type and call **eight** methods on it. The kernel now
  holds the **port** and the composition root owns the **file** (the fourth
  store to move, after the seats, the lake and the artifacts): `cmd/agezt`'s
  `openAppKernel` opens `toolforge/scripttools.json` and injects it, and a
  store that cannot be opened is the daemon's **startup error**.
  The port had to be **split**, and the contract already said so:
  `toolforgeapi.Store` had **two** verbs (`Get`, `List`) and a guard test
  that said why — *a plugin holds every method of the port it is given*, and
  one of the host's verbs is `Promote`, so a plugin that could promote would
  be able to put its own code in front of the operator's agent. So
  `toolforgeapi.HostStore` (new) carries the ten verbs the host uses, and the
  module answers **both** ports with two assertions.
  The **sentinels became rows**: `ErrNotFound` and `ErrUntested` are the
  answers the host gives, so they moved to the contract (the module keeps its
  names as aliases), and the kernel's config now names `toolregapi.ScriptRunner`
  — the registry's own row — instead of the module's `toolforge.Runner` (now
  an alias of it). The app layer speaks the contract too:
  `kernel/app/tools`'s forge catalogue names the contract's row and no longer
  imports the module at all.
  The part the slice had to **teach** the kernel: the store used to be
  guaranteed, so the code never had to say what happens without one — and the
  first version of the kernel **panicked** on `k.toolForge.Add(st)`. The tests
  caught it, and every forge operation now **refuses** with a plain message;
  the app layer learned the same rule ("the forge is not available on this
  daemon" instead of a nil dereference). As in W3.43, moving the file out of
  the kernel made every test that expected a store **become a host**: open the
  store, inject the port.
  **The ratchet went DOWN by ONE: 55/13 → 54/13.**
  **8 mutations, 8 caught**, four of them **gate-measured** by archcheck (the
  kernel opens the store again; the kernel names the concrete store; the
  kernel names the module's runner seam; a stale allowlist entry comes back).
  The mutation run was honest on the first pass — the harness had already
  been fixed after W3.43's two false starts (the package and the `-run`
  filter are separate arguments, and a build error is not a catch).
  **What remains**: the kernel still reaches **17** modules. Next, in order:
  **`market`** — the manager is already the daemon's (the daemon creates it and
  injects it with `SetMarket`), but the kernel builds the **market tool** and
  holds the concrete `*market.Manager`; the app layer already speaks two
  narrow interfaces (a three-verb reader and a six-verb writer), so a
  host-facing contract port is a small piece of work. **`workflow` and
  `workboard`** — both need wide new ports: the kernel names
  `workflow.Engine`, `workflow.Ports`, `workflow.Provenance` and
  `workflow.Trigger` (no contract home), and it calls **seventeen** workboard
  store methods. **`cadence`, `standing`, `mcp`** — many callers each; the
  shape is known (a contract port the host opens), the work is in the port's
  width. **`memory`, `worldmodel`, `roster`, `skill`, `okr`, `taste`,
  `catalog`** — the kernel holds each module's concrete store; each one is the
  same slice (a contract port, the daemon opening the file, the kernel's
  accessor carrying the port).
- W3.46 (the marketplace is the daemon's, the kernel holds the port) is
  committed on `main` the same way. It takes **one** edge — the last of the
  *market* family (`kernel/runtime -> kernel/market`) — and it closes that
  family: the kernel used to hold the concrete `*market.Manager`, build the
  `market` agent tool itself and name the module's rows. The kernel now holds
  the **contract's port** — `marketapi.Manager`, **eight** verbs (`List`,
  `Show`, `Sources`, `InstallContext`, `UninstallContext`, `AddSource`,
  `RemoveSource`, `Sync`), the width the control plane actually drives — and
  the **five rows** the port carries (`Listing`, `InstalledPack`, `Event`,
  `Source`, `SyncResult`) moved to the contract, with the module keeping every
  name as an **alias** (one row, one spelling).
  The **tool is the daemon's** now: it drives the contract's port, so the
  composition root builds it and hands it over in `cfg.Tools` like the media
  tools (W3.44), and the `MarketTool` knob is **gone** from `runtime.Config` —
  the kernel no longer builds a tool, so a knob that said "build it" would be
  a lie.
  The part the slice had to be **careful** about: the primary kernel and every
  tenant share **one** tool map, so a tool written in place — or one instance
  handed to both — would give the tenant the **primary's** manager, and with
  it the primary's Forge and MCP registry to install packs into. The rule:
  **every kernel gets its own tool, bound to its own kernel's port**; the tool
  resolves the port lazily, which is also why it can be bound *after* `Open`
  (the built-in catalogue installs into the kernel's own Forge and MCP). A
  tenant's kernel never gets a manager, so its tool reports the marketplace as
  unavailable — exactly as before.
  The part the slice had to **teach** the kernel: the manager used to be
  reachable, so `k.marketMgr` was never nil and the code never had to say
  what happens without one — now the port is **nil** and the tool answers
  "marketplace is not available on this daemon" instead of reaching for a
  manager the kernel does not have.
  **The ratchet went DOWN by ONE: 54/13 → 53/13.**
  **8 mutations, 8 caught**, three of them **gate-measured** by archcheck (the
  kernel's field names the module's manager again; the kernel builds the
  market tool again; a stale allowlist entry comes back).
  The mutation run needed **two** passes, and the first was not honest in
  either direction: the "caller's map" mutation **survived** (the daemon
  copies the tool map twice, so removing one copy is not observable) and the
  "one tool instance for every kernel" mutation was a BUILDERR (it orphaned an
  import — a compile error is not a catch).
  **What remains**: the kernel still reaches **16** modules. Next, in order:
  **`workflow` and `workboard`** — both need wide new ports: the kernel names
  `workflow.Engine`, `workflow.Ports`, `workflow.Provenance` and
  `workflow.Trigger` (no contract home), and it calls **seventeen** workboard
  store methods. **`cadence`, `standing`, `mcp`** — many callers each; the
  shape is known (a contract port the host opens), the work is in the port's
  width. **`memory`, `worldmodel`, `roster`, `skill`, `okr`, `taste`,
  `catalog`** — the kernel holds each module's concrete store; each one is the
  same slice (a contract port, the daemon opening the file, the kernel's
  accessor carrying the port).
- W3.47 (the task board is the daemon's file) is committed on `main` the same
  way. It takes **two** edges — the same family, and the first slice in a
  while that takes two at once (like W3.43): `cross-module kernel/runtime ->
  kernel/modules/work/workboard` (the kernel held the concrete
  `*workboard.Store`, drove **seventeen** of its methods through it and
  **opened the file** itself) and `adapter-bypass kernel/controlplane ->
  kernel/modules/work/workboard` (the control plane named the module's rows).
  The kernel now holds the **contract's HOST port** — `workboardapi.HostStore`,
  **twenty** verbs — and the port's width is **not** a copy of the store's
  method list: it is the **host's** width, and the **plugin** (the agent-facing
  `workboard` tool) keeps the **read side only** (`Store`, two verbs: `Get`,
  `List`), because a plugin that could claim, complete or archive a task would
  be driving another agent's work item. That is the same shape W3.45 gave the
  script-tool store (`Store` for the plugin, `HostStore` for the host) and the
  same rule: **a port's width is decided by the weakest holder of it.**
  Two **rules** moved with the port: **`ErrNotFound`** (the host reports it,
  and the host may not name a module's error) and the **retry rule**
  (`RetryDecisionFor`, `NormalizeRetryPolicy` — pure, and it reads the task,
  so it travels with the task; the module delegates, so there is one rule: a
  second copy would drift, and the drift would show up as one task retrying
  forever next to another that gives up after a single failure).
  The store file is the **daemon's**: `cmd/agezt` opens it and hands over
  `cfg.Workboard`; a store that cannot be opened is a **startup error** — the
  board is part of the daemon, not a convenience.
  The part the slice had to **teach** two layers: the board used to be opened
  by the kernel, so the field was **never nil** and nothing had to say what
  happens without one — now the **kernel** refuses every one of its sixteen
  board operations with "the task board is not available on this daemon", and
  the **app layer** refuses the same way on all four surfaces (service, watch,
  dispatch, execution) instead of dereferencing nil. One detail worth naming:
  `ProveTask` used to answer **"not found"** when the board was missing,
  because the missing board and the missing task looked the same from the
  inside — now they are different: no board → "not available"; a board without
  that task → `workboard: not found`.
  **The ratchet went DOWN by TWO: 53/13 → 51/13.**
  **8 mutations, 8 caught**, **four** of them **gate-measured** by archcheck
  (the kernel's field names the module's store again; the kernel opens the
  board again; the control plane names the module's rows again; a stale
  allowlist entry comes back), because "the kernel's reach is smaller" is not
  something a test can see.
  The mutation run needed **two** passes: the first pass's third mutation was
  caught for the **wrong reason** (the harness replaces whole LINES, so the
  mutation had replaced a function signature with a bare expression —
  archcheck failed with a **parse error**, and a parse error is not a catch of
  anything).
  **What remains**: the kernel still reaches **14** modules. Next, in order:
  **`workflow`** — the widest remaining edge: the kernel holds the store AND
  builds the engine (`workflow.NewEngine(k.workflows, k.workflowPorts())`),
  and it names `workflow.Engine`, `workflow.Ports`, `workflow.Provenance` and
  `workflow.Trigger` — none of which has a contract home. The engine is the
  hard part: its ports are built from the kernel's own state, so the
  composition root cannot build it before the kernel exists.
  **`cadence`, `standing`, `mcp`** — many callers each; the shape is known (a
  contract port the host opens), the work is in the port's width.
  **`memory`, `worldmodel`, `roster`, `skill`, `okr`, `taste`, `catalog`** —
  the kernel holds each module's concrete store; each one is the same slice
  (a contract port, the daemon opening the file, the kernel's accessor
  carrying the port).
- W3.48 (the workflow library is the daemon's file) is committed on `main` the
  same way. It takes **two** edges — the same family again (like W3.43 and
  W3.47): `cross-module kernel/runtime -> kernel/modules/workflows/workflow`
  (the kernel held the concrete `*workflow.Store`, **opened the file** itself
  and **built the engine**) and `adapter-bypass kernel/controlplane ->
  kernel/modules/workflows/workflow` (the control plane named the module's
  rows).
  This was the **widest** edge left, and the first slice where a port had to
  be built around an *implementation* the kernel was not allowed to name: the
  engine's ports are the kernel's own bus, governor, tool runner and
  approvals, so the engine could not be built before the kernel existed — and
  could not be built by the kernel without making the kernel a module. The
  answer is the one the reasoning module and the reaper scanner already have,
  one layer up: the kernel holds the **contract's port** —
  `workflowapi.Engine`, **eight** verbs (`Save`, `Restore`, `SetEnabled`,
  `Remove`, `Run`, `TestNode`, `Draft`, `Refine`) — and the **composition
  root** builds the engine over the library it opened and over the kernel's
  own ports (`k.WorkflowPorts()`), handing it over with `SetWorkflowEngine`
  **after** `Open`.
  The **width is the weakest holder's**: the plugin (the agent-facing
  `workflow` tool) keeps the **read side only** (`workflowapi.Store`, two
  verbs — `Get`, `List`), because a plugin that could remove or disable a
  stored workflow would be editing the daemon's library; the host holds
  `workflowapi.HostStore`, and the kernel's own face is the read side —
  checked **statically**, because a widened face still compiles and still
  satisfies the read port, so only the declared return type can tell the two
  apart.
  The **rows that travel with the port** moved to the contract: `Ports` (the
  ten host capabilities), `Provenance`, `Trigger`, `ErrHalted` and the
  approval rows `approvalapi.SubmitSpec`/`Outcome` (the platform keeps its
  names as aliases) — one row, one spelling.
  The part the slice had to **teach** two layers: the engine used to be built
  by the kernel, so `k.wfEngine` was **never nil** and nothing had to say
  what happens without one — now the **kernel** refuses all eight lifecycle
  operations with "the workflow engine is not available on this daemon", and
  the **app layer** refuses the same way with "the workflow library is not
  available on this daemon", instead of dereferencing nil.
  Every test that expected a store became a **host**: it opens the library,
  injects the port and builds the engine over its own kernel's ports.
  **The ratchet went DOWN by TWO: 51/13 → 49/13.**
  **8 mutations, 8 caught** in two honest passes, **three** of them
  **gate-measured** by archcheck (the control plane names the module's rows
  again; the runtime names the module's rows again; a stale allowlist entry
  comes back), because "the kernel's reach is smaller" is not something a test
  can see.
  **What remains**: the kernel still reaches **12** modules. Next, in order:
  **`cadence`, `standing`, `mcp`** — many callers each; the shape is known (a
  contract port the host opens), the work is in the port's width.
  **`memory`, `worldmodel`, `roster`, `skill`, `okr`, `taste`, `catalog`** —
  the kernel holds each module's concrete store; each one is the same slice
  (a contract port, the daemon opening the file, the kernel's accessor
  carrying the port).
- W3.49 (the typed schedule library is the daemon's file) is committed on
  `main` the same way. It takes **three** edges — the most of any candidate,
  for only six call sites in the runtime:
  `cross-module kernel/runtime -> kernel/cadence` (the kernel held the
  concrete `*cadence.Store`, **opened the file** itself and held the concrete
  `*cadence.Engine`), `cross-module kernel/runtime/runexec -> kernel/cadence`
  (the run-executor interface named the module's store) and
  `adapter-bypass kernel/controlplane -> kernel/cadence` (the control plane
  named the module's rows).
  **The ratchet went DOWN by THREE: 49/13 → 46/13.**
  The rows were already in the contract (`cadenceapi.Entry`, the
  `Target*`/`Source*`/`Mode*` vocabularies, the system-task catalog); what
  was missing was a **port**. The contract now carries `cadenceapi.HostStore`
  — **twenty** verbs: the tool's eleven plus the nine the host drives on its
  own (the control plane retires a schedule, the guardians re-arm one, the
  app layer edits its cadence, the status view counts what is armed).
  The **width rule** made mechanical: the host's port **embeds** the tool's
  eleven-verb `cadenceapi.Store`, so the two shapes cannot drift — if the
  tool's port grows a verb, the host's port grows it too, and the module's
  implementation is the one that has to answer.
  The **resident** is a port too (`cadenceapi.Engine`, **four** verbs:
  `Start`, `Wait`, `WaitIdle`, `RunningCount`): the host only reads the
  resident's state, and the firing logic belongs to the module.
  The **library's file is the daemon's** — `cmd/agezt` opens it
  (`cadence/schedules.json`), and a library that cannot be opened is a
  **startup error**; the **resident is the daemon's too**, built by the
  composition root over **that very store** and handed over with
  `SetScheduleEngine`, so the kernel's port and the resident read the same
  rows (a second instance over the same file would have been a silent
  split-brain).
  The part the slice had to **teach** two layers: before it the kernel opened
  the library itself, so `k.Schedules()` was **never nil** and nothing had to
  say what happens without one — now the **app layer** refuses on every
  surface (creation, editing, lifecycle, read) with "the schedule library is
  not available on this daemon", the **guardians' host** answers the same
  way, and the control plane's three roster surfaces read the port
  defensively.
  Every test that expected a store became a **host**: it opens the library,
  injects the port and — where the resident is involved — builds the resident
  over that very store.
  **8 mutations, 8 caught** in two honest passes, **three** of them
  **gate-measured** by archcheck (the runtime names the module again; the
  control plane names the module's rows again; a stale allowlist entry comes
  back), because "the kernel's reach is smaller" is not something a test can
  see; the first attempt at the eighth mutation was a **build error** (the
  store became an unused variable) and a build error is not a catch — the
  mutation was wrong, not the test.
  The new permanent tests pin that the host port is **exactly twenty** verbs
  wide, that it is a **superset** of the tool's eleven, that the resident's
  port is **exactly four**, that the module's store and resident answer
  **both** contract ports, that a kernel without a library hands over a
  **nil** port, that the kernel's face is **statically** the host port
  (narrowing it would still compile and still pass every behaviour test),
  that the port answers the **live** rows, that all **six** app surfaces
  refuse instead of panicking, and that the daemon's library is a **file on
  disk** and is the **injected** one.
  **What remains**: the kernel still reaches **9** modules. Next, in order:
  **`standing`** (three edges, `standingapi` exists), **`skill`** (three edges
  plus the market's, `skillapi` exists), **`catalog`** (two edges,
  `catalogapi` exists), then **`memory`, `worldmodel`, `roster`, `okr`,
  `taste`** — each one the same slice (a contract port, the daemon opening the
  file, the kernel's accessor carrying the port).

- W3.50 (the standing-order library is the daemon's file) is committed on
  `main` the same way. It takes **three** edges — the same family again, and
  the **widest** edge the measurement had left, for only eight call sites in
  the runtime:
  `cross-module kernel/runtime -> kernel/standing` (the kernel held the
  concrete `*standing.Store` and **opened the file** itself),
  `cross-module kernel/runtime/runexec -> kernel/standing` (the run-executor
  interface named the module's store) and
  `adapter-bypass kernel/controlplane -> kernel/standing` (the control plane
  named the module's rows).
  **The ratchet went DOWN by THREE: 46/13 → 43/13.**
  The rows were already in the contract (`standingapi.Order`, `Trigger`,
  `Initiative`, `Validate`, the `TriggerCron`/`TriggerEvent` and
  `Initiative*` vocabularies); what was missing was a **port**. The contract now
  carries `standingapi.Store` — **seven** verbs (`Add`, `SetEnabled`, `Update`,
  `Remove`, `Get`, `List`, `Count`), the module's whole public surface and no
  more: a wider port would let a caller reach past the library's own rules (the
  store validates, assigns ids and protects the identity fields the mutator
  must not touch), and a narrower one would force a second shape to be kept in
  step with this one.
  **`ErrNotFound` moved to the contract**: the kernel compares against it (the
  edit path turns "not found" into "the order does not exist") and the kernel
  may not name a module. The module keeps the name as an **alias**, so the two
  are the same error value and every `errors.Is` still holds.
  The **library's file is the daemon's** — `cmd/agezt` opens it
  (`standing/standing.json`), and a library that cannot be opened is a
  **startup error**; the **residents are the daemon's too** — the event runner
  and the cron ticker are built by the composition root over **that very
  store**, so the kernel's port and the residents read the same rows (a second
  instance over the same file would have been a silent split-brain).
  The part the slice had to **teach** four layers: before it the kernel opened
  the library itself, so `k.Standing()` was **never nil** and nothing had to
  say what happens without one — now the **kernel** refuses all **five** of its
  standing operations with "the standing library is not available on this
  daemon", the **app layer** refuses the same way on its **five** surfaces
  (read, creation, editing, lifecycle), the **guardians' host** answers the
  same way, and the control plane's two roster surfaces read the port
  defensively. The layers above the module also name the **contract's** rows
  now (`standingapi.Order` in the app layer, the app's roster surfaces and the
  control plane) — the two are the same type, so this is one row with one
  spelling, not a copy.
  Every test that expected a store became a **host**: it opens the library and
  injects the port.
  **8 mutations, 8 caught** in two honest passes, **three** of them
  **gate-measured** by archcheck (the runtime names the module again; the
  control plane names the module's rows again; a stale allowlist entry comes
  back), because "the kernel's reach is smaller" is not something a test can
  see. The run earned two honest corrections: the first attempt at the fifth
  mutation was a **build error** (a whole function signature replaced with a
  bare expression — a build error is not a catch, the mutation was wrong) and
  the second **survived**, because the harness replaces the **first** matching
  line and `for _, o := range s.orders` sits in **three** functions — the anchor
  was narrowed to the sort inside `List()`; the harness itself was corrected so
  a missed anchor reports SKIP and can never look like a pass.
  The sixth mutation is the one worth naming: it replaces the kernel's face with
  an **anonymous** interface carrying the same seven verbs — that still
  compiles (the run engine's own declaration moves with it) and no runtime
  behaviour would notice, so only a **static** check on the declared return type
  catches it.
  The new permanent tests pin that the store port is **exactly seven** verbs
  wide and keeps its exact signatures, that the module's store answers the
  contract's port and that its rows and its error **are** the contract's, that
  a kernel without a library hands over a **nil** port, that the kernel's face
  is **statically** the store port (a narrowed face would still compile and
  still pass every behaviour test), that the port answers the **live** rows,
  that all **five** kernel operations and all **five** app surfaces refuse
  instead of panicking, and that the daemon's library is a **file on disk** and
  is the **injected** one.
  **What remains**: the kernel still reaches **8** modules. Next, in order:
  **`skill`** (three edges plus the market's, `skillapi` exists),
  **`catalog`** (two edges, `catalogapi` exists), then **`memory`,
  `worldmodel`, `roster`, `okr`, `taste`** — each one the same slice (a
  contract port, the daemon opening the file, the kernel's accessor carrying the
  port).

- W3.51 (the catalog directory and its syncer are the daemon's) is committed on
  `main` the same way. It takes **four** edges — the most of any slice so far,
  and the best ratio the measurement had (four edges for ten call sites):
  `cross-module kernel/runtime -> kernel/modules/providers/catalog` (the kernel
  held the module's `*catalog.Store` and the `*catalog.Catalog` row),
  `cross-module kernel/runtime/runexec -> kernel/modules/providers/catalog` (the
  run-executor interface named the module's store and row),
  `cross-module kernel/cadence/systemtasks -> kernel/modules/providers/catalog`
  (the scheduled sync built the module's syncer itself) and
  `upward kernel/governor -> kernel/modules/providers/catalog` (the governor's
  pricing table held a `*catalog.Catalog`).
  **The ratchet went DOWN by FOUR: 43/13 → 39/13.**
  The catalog's rows were **already** in the contract and the module already
  kept them as aliases — so this slice was mostly about the two things that
  were missing: **the aggregate row** and **the ports**. The contract now
  carries the `Catalog` aggregate AND its **nine rules**
  (`ProviderList`, `FindModel`, `StrictToolArgsNative`,
  `ToolCapableAlternative`, `ToolCapableAlternativeAmong`, `VisionCapableAmong`,
  `BestModelsAcross`, `DuplicateCredentialEnvs`, `Merge`), with the four
  helpers they lean on and `MarshalAPI`.
  **The rules had to travel, and that is the point**: a Go type alias cannot
  carry methods, so a rule that stayed in the module could not be reached by
  the layers above it — and the rules decide **which model a run routes to**
  (the down-routing that picks a tool-capable sibling, the vision sidecar's
  choice, the council's seats, the pricing table's lookup). Keeping them in the
  module would have left the governor reaching into a module for the price of
  a model.
  The **store** port is **exactly five** verbs — what the layers above actually
  drive; the module's own store has more (`SaveCustom`, `SaveMeta`) and those
  stay on the module's type, because no layer above the module drives them.
  The **syncer** port is **one** verb, and its URL and timeout are
  **parameters, not fields**: a port cannot carry mutable state that a caller
  would have to reach through.
  The **directory is the daemon's** and the **syncer is the daemon's too** — the
  scheduled system task reaches it **through the kernel** (`k.CatalogSyncer()`),
  which is what closed the `systemtasks -> catalog` edge.
  The slice had to answer a question the earlier ones did not: what does a
  daemon with **no** catalog say? The **read** surface answers with the
  **EMPTY** catalog ("no providers" is an honest answer, and a nil snapshot
  would make every reader a potential panic — `k.Catalog()` is never nil),
  while the **write** surfaces (a sync, a discovery, a custom provider)
  **refuse** with "the catalog library is not available on this daemon" —
  there is nowhere to write, so the refusal is the answer. The app layer
  learned the same rule, and the discovery refuses **before** the network
  call: a daemon with no library should not go and ask Ollama what it finds
  when it has nowhere to put the answer.
  Every test that expected a store became a **host**: it opens the directory
  and injects the ports.
  **8 mutations, 8 caught** in two honest passes, **three** of them
  **gate-measured** by archcheck (the kernel names the module again; the
  governor names the module again; a stale allowlist entry comes back), because
  "the kernel's reach is smaller" is not something a test can see. The run
  earned two honest corrections: the first attempt at the fifth mutation
  narrowed the kernel's face to a one-verb interface, which **broke the
  callers** and was a **build error** (a build error is not a catch — the
  mutation was wrong, not the test), and the second drifts the face to an
  **anonymous** interface with the **same five verbs** — that still compiles
  and no runtime behaviour would notice, so only a **static** check on the
  declared return type catches it; the sixth mutation first landed on the
  kernel's **Open** path, where the empty catalog is *correct*, so the test
  never saw it — it now lands on the **reload**, which is where the live rows
  actually come from.
  The new permanent tests pin that the store port is **exactly five** verbs
  wide and keeps its exact signatures, that the syncer port is **one** verb
  and takes the caller's URL, that the module's store and syncer answer
  **both** contract ports and that its rows are the contract's, that the
  kernel's face is **statically** the contract's ports, that a kernel without
  a library hands over **nil** ports, that the snapshot is **never nil**, that
  the reload refuses instead of panicking, that the ports answer the **live**
  rows, that both app surfaces refuse instead of panicking, and that the
  daemon's catalog is a **directory on disk** and is the **injected** one.
  **What remains**: the kernel still reaches **7** modules. Next, in order:
  **`skill`** (five edges, `skillapi` exists — the `Forge` and `BundleStore`
  ports are already there), **`roster`** (four edges and 73 importer files by
  far the largest fan-in; the rows are already in `agentapi`, only the store
  port and the sentinels are missing), then **`memory`, `worldmodel`, `taste`,
  `okr`** — each one the same slice (a contract port, the daemon opening the
  file, the kernel's accessor carrying the port).

- W3.52 (the curated exemplar library is the daemon's) is committed on `main`
  the same way. It takes **one** edge — the smallest slice so far — and it
  takes it **completely**:
  `cross-module kernel/runtime -> kernel/modules/knowledge/taste` (the kernel
  held the module's concrete `*taste.Store` and **opened the file** itself).
  **The ratchet went DOWN by ONE: 39/13 → 38/13.** The kernel still reaches
  **7** modules.
  The slice is small **on purpose**: the measurement had six candidates left
  (`skill`, `roster`, `memory`, `worldmodel`, `taste`, `okr`), and the first
  thing measured was the **width** of each port, not the number of edges.
  `skill` would have to carry the **Forge** — 15+ verbs, including `Propose`,
  `ShadowEvaluate`, `RecordOutcome`, `ActivateFor`, `SetAutoQuarantine` and
  `Bundles()` — and it is journaled through the bus, so the port and the
  publisher would have to travel together; `roster` spans **73 importer
  files**; `memory` is 6 edges wide and bus-bound; `worldmodel` is the graph
  engine. **`taste`** is the same shape as the slices that came before it
  (`standing`, the catalog) at a **fifth** of the width: a contract port, the
  daemon opening the file, the kernel's accessor carrying the port. A pattern
  proven on a one-edge slice is cheaper than a pattern proven on a fifteen-verb
  port.
  The contract now carries `tasteapi` — the `Exemplar`, `CreateSpec` and
  `Filter` rows, the `ErrNotFound` sentinel, and the **`Store` port: exactly
  five verbs** (`Create`, `Get`, `List`, `ForScope`, `Delete`), the module's
  whole public surface and no more. The module keeps every name as an
  **alias**, so there is one row and one spelling of it.
  **The port carries no clock**: `Create` takes `nowMS int64`, not a
  `time.Time` — the contract layer may not name a clock (principle P4), and a
  port that reads one cannot be tested deterministically. The module's own
  `Create` changed shape with it: the caller supplies the time, the store
  records it.
  **The library's file is the daemon's**: the composition root
  (`cmd/agezt`'s `openAppKernel` — the same place that opens the workboard and
  the tool-forge stores) opens `taste/taste.json` and hands the kernel the
  port; a library that cannot be opened is a **startup error**. Because the
  library lives under the **base dir**, a **tenant** opens **its own**
  exemplars — the config copy in the tenant closure carries the primary's
  port, and the composition root drops it, so a tenant can never curate the
  primary's taste anchors. That leak would have been invisible to every test
  on the primary.
  A daemon with **no** library gets two different answers, on purpose: the
  **read** surface answers the **EMPTY** list ("no exemplars" is an honest
  answer, and a nil list would make every reader a potential panic), while the
  **write** surfaces (create, delete) **refuse** with "the exemplar library is
  not available on this daemon" — there is nowhere to write, so the refusal is
  the answer. The kernel injects **no** exemplars into a run rather than
  calling into nothing.
  Every test that expected a store became a **host**: it opens the library and
  injects the port.
  **8 mutations, 8 caught** in two honest passes, **three** of them
  **gate-measured** by archcheck (the kernel names the module again; the
  control plane names the module again; a stale allowlist entry comes back),
  because "the kernel's reach is smaller" is not something a test can see.
  One correction is worth naming: the obvious version of the fourth mutation
  — **removing** a verb from the port — **breaks the app layer that calls
  it**, which is a **build error**, not a catch. The mutation therefore
  **grows** the port by a sixth verb and implements it in the module too: the
  code still compiles, and only a **static** width test can tell the two
  apart.
  The new permanent tests pin that the store port is **exactly five** verbs
  wide and keeps its exact signatures, that no verb takes a `time.Time`, that
  the sentinel the layers compare against is the contract's, that the module's
  rows and sentinel **are** the contract's (by **type identity**), that the
  kernel's face is **statically** the contract's port, that a kernel without
  a library hands over a **nil** port, that the port answers the **live**
  rows, that both app write surfaces refuse instead of panicking, that the
  read surface answers the empty list, and that the daemon's library is a
  **file on disk** and is the **injected** one.
  **What remains**: the kernel still reaches **7** modules. Next, in order:
  **`skill`** (five edges — the **width** is the work: the Forge's 15+ verbs
  and its bus-bound publisher have to travel together), **`roster`** (four
  edges, 73 importer files, the largest fan-in), then **`memory`,
  `worldmodel`, `okr`** — each one the same slice this one was (a contract
  port, the daemon opening the file, the kernel's accessor carrying the
  port).

- W3.53 (the objective library is the daemon's) is committed on `main` the
  same way. It takes **one** edge — the smallest slice shape there is — and it
  takes it **completely**:
  `cross-module kernel/runtime -> kernel/modules/work/okr` (the kernel held the
  module's concrete `*okr.Store` and **opened the file** itself).
  **The ratchet went DOWN by ONE: 38/13 → 37/13.** The kernel still reaches
  **7** modules.
  It is the same slice as W3.52 (`taste`), one step wider: the objective
  library carries **nine** verbs instead of five, and — unlike the exemplar
  library — it carries a **rule**.
  The contract now carries `okrapi` — the `Objective`, `KeyResult`,
  `ObjectiveProgress`, `KeyResultProgress`, `CreateSpec` and `Filter` rows,
  the `Status` row with its three constants, the three sentinels
  (`ErrNotFound`, `ErrKRNotFound`, `ErrInvalidStatus`), and the **`Store`
  port: exactly nine verbs** (`Create`, `Get`, `List`, `AddKeyResult`,
  `LinkTask`, `UnlinkTask`, `SetStatus`, `Archive`, `ObjectivesForTask`), the
  module's whole public surface and no more. The module keeps every name as an
  **alias**, so there is one row and one spelling of it.
  **The RULE travels with the ROW** — that is the part this slice adds. The
  objective's rollup ("does this goal count as met?") was a **method** on the
  objective, and a Go type alias **cannot carry methods**: had the rule stayed
  in the module, the alias would have lost it and the layers above would have
  had to reach back into the module to compute a rollup. So the rule moved
  **with the row** into the contract, and the module's `Objective` is an alias
  that keeps it. The rule is not decoration — it decides whether a goal counts
  as met, and a mutation that halves the percentage (`done * 50 / target`)
  still compiles, still passes every structural check, and still answers "a
  percentage" — just a wrong one.
  **The port carries NO CLOCK**: six of the nine verbs stamp a row, and all
  six take `nowMS int64`, not a `time.Time` — the contract layer may not name
  a clock, and a port that reads one cannot be tested deterministically. The
  module's own verbs changed shape with it.
  **The library's file is the daemon's**: the composition root
  (`cmd/agezt`'s `openAppKernel`) opens `okr/okr.json` and hands the kernel the
  port, and a library that cannot be opened is a **startup error**. Because the
  library lives under the **base dir**, a **tenant** opens **its own**
  objectives.
  A daemon with **no** library answers three ways, on purpose: the **list**
  answers the **empty** list ("no objectives" is an honest answer); the
  **named** read **refuses** — "unknown objective" would blame the id for a
  library that is not there, and a named thing cannot be answered from an empty
  library; and the **write** surfaces (create, add key result, link, unlink,
  archive) refuse with "the objective library is not available on this
  daemon" — there is nowhere to write.
  Every test that expected a store became a **host**: it opens the library and
  injects the port.
  **9 mutations, 9 caught** in two honest passes, **three** of them
  **gate-measured** by archcheck (the kernel names the module again; the
  control plane names the module again; a stale allowlist entry comes back),
  and the **ninth** is the one this slice added: **the rollup rule is wrong** —
  the percentage is halved. A static width test cannot see a rule, and a wrong
  rule still answers "a percentage".
  The new permanent tests pin that the store port is **exactly nine** verbs
  wide and keeps its exact signatures, that no verb takes a `time.Time`, that
  the three sentinels the layers compare against are the contract's, that the
  **rollup rule is on the contract's row and computes the right answer** (the
  per-key-result percentages, the effective target when none is given, the
  objective's percentage and its achieved flag — and that an objective with no
  key results is never "achieved"), that the module's rows, status constants
  and sentinels **are** the contract's (by **type identity**), that the
  aliased objective **keeps** the contract's rule, that the kernel's face is
  **statically** the contract's port, that a kernel without a library hands
  over a **nil** port, that all four write surfaces refuse instead of
  panicking, that the port answers the **live** rows, that the read surface
  answers the empty list and the named read refuses, and that the daemon's
  library is a **file on disk** and is the **injected** one.
  **What remains**: the kernel still reaches **7** modules. Next, in order:
  **`skill`** (five edges — the **width** is the work: the Forge's 15+ verbs
  and its bus-bound publisher have to travel together), **`roster`** (four
  edges, 73 importer files, the largest fan-in), then **`memory`,
  `worldmodel`** — each one the same slice this one was (a contract port, the
  daemon opening the file, the kernel's accessor carrying the port), with the
  memory manager's bus-bound journal as the extra work.

- W3.54 (the world model is the daemon's) is committed on `main` the same way.
  It takes **four** edges — the first multi-edge slice since the catalog — and
  it takes them **completely**:
  `cross-module kernel/runtime -> kernel/modules/knowledge/worldmodel` (the
  kernel held the module's concrete `*worldmodel.Graph` and **opened the file**
  itself), `cross-module kernel/runtime/runexec -> ...` (the run-executor
  declared the module's concrete graph in its `KernelAPI`),
  `cross-module kernel/contextselect -> ...` (the context selector named the
  module's rows) and `cross-module kernel/reflect -> ...` (the reflection
  engine held the module's concrete graph).
  **The ratchet went DOWN by FOUR: 37/13 → 33/13.** The kernel still reaches
  **7** modules.
  This slice is the **pilot for the bus-bound modules**: the graph journals
  every mutation through the kernel's bus, so the graph cannot be built by the
  layer that holds the bus without that layer naming the module. The pattern
  it establishes — a **builder injected by the daemon**, a **publisher port**
  the kernel hands over — is what the remaining bus-bound modules (the skill
  forge, the memory manager) will reuse.
  The contract now carries `worldmodelapi` — the `Kind` and `Verb` rows with
  their open vocabularies, the `Entity`, `Relation`, `ScoredEntity`,
  `Neighbor`, `UpsertSpec` and `DecayOptions` rows, the **`Store` port
  (eight verbs)**, the **`Graph` port (fourteen verbs** — exactly what the
  layers above drive, no more**) and a **`Publisher` port of one verb**. The
  module keeps every name as an **alias**, so there is one row and one spelling
  of it.
  **The RULES travel with the ROWS**: `Entity.Active()` and
  `Relation.Active()` decide whether a node still counts, and a Go type alias
  cannot carry methods — had the rules stayed in the module, the aliases would
  have lost them and the layers above would have had to reach back into the
  module to ask. The content-addressing helpers (`EntityID`, `RelationID`)
  **stay in the module**: they hash with BLAKE3 and the contract layer may not
  reach for a third-party module, and nothing above the module needs them.
  The **publisher is a port, not the bus**: the bus answers with the durable
  envelope, which carries the hash chain and which the contract layer may not
  name; the port answers with the event's **id**, which is exactly what the
  graph records as the entity's provenance (`SourceEvent`) and what `agt why`
  reads.
  **The daemon names the module; the kernel does not**: the daemon opens
  `worldmodel/worldmodel.json` and hands over the **builder**, and the kernel
  calls that builder with the store it was given and its own publisher, so the
  graph journals through the kernel's bus without either side naming the
  other's implementation.
  A kernel with **no** graph registers **no world tool** (a tool that calls into
  nothing would answer the agent with a panic), the reflection engine with no
  graph decays nothing and reports what it could see, and the app layer
  **refuses** every surface with "the world model is not available on this
  daemon".
  Every test that expected a store became a **host**: it opens the file and
  injects the ports.
  **9 mutations, 9 caught** in two honest passes, **three** of them
  **gate-measured** by archcheck (the kernel names the module again; the
  context selector names the module again; a stale allowlist entry comes back),
  and the **ninth** measures a **rule**: an active rule that quietly forgets the
  tombstone lets a forgotten entity back into resolve, and no structural check
  can see it. One correction is worth naming: the fifth mutation — the
  kernel's face drifting to an anonymous shape — had to travel with the **run
  executor's `KernelAPI` declaration**, because drifting the kernel's face
  alone is a **build error**, not a catch (the same lesson as the catalog
  slice, met again: a face that the run executor declares cannot drift
  alone).
  The new permanent tests pin that the store port is **exactly eight** verbs
  and the graph port **exactly fourteen**, that the publisher is **one** verb
  answering the event's id, that the verbs keep their exact signatures, that
  the active rules are **on the contract's rows** and decide right, that the
  normalizers keep their permissive rules, that the module's rows and constants
  are the contract's (by **type identity**), that the **aliased rows keep the
  contract's rules**, that the graph's core **does not import the kernel bus**,
  that the kernel's face is **statically** the contract's port, that a kernel
  without a world model hands over a **nil** port, that the world tool is
  registered **only** with a graph, that the port answers the **live** rows,
  that every app surface refuses instead of panicking, and that the daemon's
  model is a **file on disk** and is the **injected** one.
  **What remains**: the kernel still reaches **7** modules. Next, in order:
  **`skill`** (five edges — the forge is bus-bound too, so it needs the same
  builder-plus-publisher pattern this slice established; the **width** is the
  work: 15+ verbs), **`roster`** (four edges, 73 importer files, the largest
  fan-in, and a 28-file app layer), then **`memory`** (six edges; the manager
  is bus-bound AND LLM-bound — distill and embeddings — so its port is the
  widest of all).

- W3.55 (the skill library is the daemon's) is committed on `main` the same
  way. It takes **five** edges — the **widest** slice since the standing-order
  library — and it takes them **completely**:
  `cross-module kernel/runtime -> kernel/modules/skills/skill` (the kernel
  held the module's concrete `*skill.Forge` and **opened the file** itself),
  `cross-module kernel/runtime/runexec -> ...` (the run-executor declared the
  module's concrete forge in its `KernelAPI`),
  `cross-module kernel/contextselect -> ...` (the context selector named the
  module's rows), `cross-module kernel/market -> ...` (the marketplace named
  the module's rows) and `adapter-bypass kernel/controlplane -> ...` (the
  control plane named the module's rows).
  **The ratchet went DOWN by FIVE: 33/13 → 28/13.** The kernel still reaches
  **7** modules.
  This is the **second bus-bound module** — the first was the world model
  (W3.54) — and it reuses that slice's pattern: a **builder injected by the
  daemon**, a **publisher port** the kernel hands over.
  The contract `skillapi` already existed (the agent-facing skill tool is a
  PLUGIN and may reach the contract and nothing else); this slice **widens**
  it with the rows the layers above the module share (`Scored`,
  `ActivationDirective`, `HygieneReport`), the sentinels they compare against
  (`ErrIllegalTransition`, `ErrNotFound`, `ErrEmptyBody`), the **pure rules**
  (the retrieval ranking and the directive parser) and the **ports**: a
  five-verb `Store`, a one-verb `Publisher`, and the **`Curator` — exactly
  twenty verbs**, the module's whole public surface as the kernel, the control
  plane and the app layer see it. The module keeps every name as an
  **alias**, so there is one row and one spelling of it.
  **The RULES travel with the ROWS**: the ranking decides which skill is
  offered to a run and the parser decides what the run's intent really says
  once the control line is stripped; both are pure functions over the
  contract's rows, so they moved **with** the rows into the contract, because
  the layers above the module rank skills and parse directives too and they
  may not name the module to reach either.
  **The port is SPLIT, not widened**: `skillapi.Forge` stays at its **five**
  verbs — it is the seam the agent-facing tool drives and every plugin's fake
  implements it — and the wider surface is the `Curator`, composed of narrow
  ports (`Lifecycle` 4, `Activator` 2, `Outcome` 1, `Proposer` 2, `Observer`
  2, `Bundler` 1, `Tuner` 3) so each keeps a single meaning; widening the
  five-verb seam would have broken every plugin's fake for no benefit.
  The **publisher is a port, not the bus**: the bus answers with the durable
  envelope, which carries the hash chain and which the contract layer may not
  name; the port answers with the event's **id**, which is exactly what a
  skill records as its provenance (`SourceEvent`) and what `agt why` reads.
  **The daemon names the module; the kernel does not**: the daemon opens
  `skills/skills.json` and hands over the **builder**, and the kernel calls
  that builder with the store it was given and its own publisher, so every
  transition journals through the kernel's bus without either side naming the
  other's implementation; because the library lives under the **base dir**, a
  **tenant** curates **its own** skills.
  A daemon with **no** library answers, on purpose: the **kernel** holds a
  **nil** port (the run carries no skill, the run executor proposes nothing,
  the status and the metrics report **zero**), the **app layer REFUSES** every
  surface that needs the library with "the skill library is not available on
  this daemon", and the observations' **history** still answers because it
  folds the **journal** — the rows are there whether or not this daemon has a
  library.
  Two things the slice had to **fix on the way**: the module's
  `WithCorrelation` wrote a context key **nothing ever read**, and the
  kernel's `SkillStore()` accessor was **called from nowhere** while being
  the only reason the kernel needed the file store's *type* — both are gone;
  and `k.World().Count()` / `k.Forge().Count()` were called **without a
  guard** in the status, the introspect overview and the metrics — the world
  one was a latent hole from W3.54 and the skill one would have been the
  same — so both now report **zero** when the port is absent.
  Every test that expected a library became a **host**: it opens the file and
  injects the ports.
  **9 mutations, 9 caught** in two honest passes, **three** of them
  **gate-measured** by archcheck (the kernel names the module again; the
  control plane names the module again; a stale allowlist entry comes back),
  and the **ninth** measures a **rule**: a ranking rule that quietly admits a
  DRAFT would put a skill that was never promoted in front of the agent, and
  no structural check can see it.
  Two honest corrections are worth naming: the first attempt at the fourth
  mutation **survived** because the harness replaces the **first** matching
  line and `Count() int` sits in **two** ports — the mutation landed on the
  store port, which the curator does not embed — so the anchor was narrowed
  to the unique hygiene verb; and the third run hit a **file lock** (a
  `go run` from the previous pass still had the file mapped), so the
  harness's write now retries like its restore already did — a locked file is
  a harness failure, never a silent pass. The fifth mutation again had to
  travel with the **run executor's `KernelAPI` declaration**, because
  drifting the kernel's face alone is a **build error**, not a catch (the
  same lesson as the catalog and world-model slices, met for the third time).
  The new permanent tests pin that the curator port is **exactly twenty**
  verbs and each narrow port keeps its own width, that the verbs keep their
  exact signatures, that the publisher is **one** verb answering the event's
  id, that the **ranking rule decides right** (only the active pool is
  offered, a non-matching intent retrieves nothing, the shadow pool is ranked
  but never injected), that the **directive rule decides right** (the control
  line is stripped, a bare directive keeps its intent, a plain intent stays
  plain), that the three sentinels are the contract's and **distinct**, that
  the module's rows and sentinels **are** the contract's (by **type
  identity**), that a forge without bundles answers a **real nil** (a typed
  nil in an interface would look wired to the caller and panic on first use),
  that the kernel's face is **statically** the contract's port, that a kernel
  without a library hands over a **nil** port, that the port answers the
  **live** rows, that every app surface refuses instead of panicking, and that
  the daemon's library is a **directory on disk** and is the **injected** one.
  **What remains**: the kernel still reaches **7** modules. Next, in order:
  **`roster`** (four edges, 73 importer files, the largest fan-in, and a
  28-file app layer; the rows are already in `agentapi` — the store port and
  the sentinels are what is missing), then **`memory`** (six edges; the
  manager is bus-bound AND LLM-bound — distill and embeddings — so its port
  would be the widest of all, and it is the **third** bus-bound module, so
  the builder-plus-publisher pattern applies), then **`mcp`** (two edges, the
  smallest remaining family).

- W3.56 (the roster is the daemon's) is committed on `main` the same way. It
  takes **four** edges and it takes them **completely**:
  `cross-module kernel/runtime -> kernel/modules/agents/roster` (the kernel
  held the module's concrete `*roster.Store` and **opened the file** itself),
  `cross-module kernel/runtime/runexec -> ...` (the run-executor declared the
  module's concrete store in its `KernelAPI`),
  `adapter-bypass kernel/controlplane -> ...` (the control plane named the
  module's rows) and `adapter-bypass kernel/agentgw -> ...` (the agent gateway
  named the module's rows).
  **The ratchet went DOWN by FOUR: 28/13 → 24/13**; the **13** allowlisted call
  sites are unchanged.
  This is the **cheapest** slice of the W3.5x series, and it is cheap for a
  reason worth naming: the roster's **rows were already in the contract**
  (`agentapi.Profile` moved there in W3.14), so what was missing was only the
  **port**, the **sentinels** and the **rule** — the three things the layers
  above the module could not reach.
  It is also the first slice in the series that is **not bus-bound**: the
  roster does not journal itself (the **kernel** presses the journal), so there
  is no publisher port and no builder to inject — the daemon opens
  `roster/roster.json` and hands over the store, and that is the whole
  handover; because the roster lives under the **base dir**, a **tenant** keeps
  **its own** agents.
  The contract `agentapi` gains the **sentinels** (`ErrNotFound`, `ErrRetired`),
  the **ports** and the **rule**: an **eight-verb** `Store` (Add, SetEnabled,
  SetRetired, Update, Remove, Get, List, Count — and **no verb takes a
  `time.Time`**, P4), a **two-verb** `Reader` and a **one-verb**
  `RepairSource`. The module keeps every name as an **alias**, so there is one
  row and one spelling of it and `errors.Is` matches on **both** sides of the
  module boundary.
  **The port is SPLIT, not widened**: the gateway only looks agents up, so it
  takes the two-verb `Reader`; the control plane drives the lifecycle, so it
  takes the eight-verb `Store`; a repair pass takes the one-verb
  `RepairSource` — a single wide port would have handed the gateway the whole
  agent lifecycle.
  **The rule travels with the row**: `AutonomyRunbook` — the machine-readable
  wake contract of a profile (trigger / route / recovery / sleep) — moved from
  the module **with** the profile row it judges into the contract, because the
  control plane's wake evidence, the run executor's sub-agent prep and the
  daemon's cadence all build the **same** payload and may not name the module
  to reach the rule. The **test seam stays out of the port**:
  `SetNowForTest` is deliberately not on `agentapi.Store`, because a port is
  what the layers above the module may call and a clock hook is not theirs.
  The kernel holds the port and **never opens a roster of its own** (the
  compose root no longer touches `roster.Open`).
  A daemon with **no** roster answers, on purpose, and **the answers differ by
  direction**: the **reads** answer **"no agents"** — `RosterGet` finds nothing
  and `RosterList` is empty, both through the kernel's **nil-safe** helpers, so
  every reader above the module is covered by **one** decision — while the
  **writes refuse** with "the roster is not available on this daemon" rather
  than calling into nothing; the **port itself** is still returned, and a
  kernel with no roster holds a **nil** port, which is a different thing from a
  typed nil.
  That nil port is exactly why the reads had to move: the app layer's overseer
  source, the guardians' host and the control plane's operation builders took
  a **method value** from the port, and a method value taken from a nil
  interface **panics before the first call** — the typed-nil trap the standing
  and schedule slices already met. All of them now go through the kernel's
  nil-safe readers (`s.k.RosterGet(ref)` instead of `s.k.Roster().Get(ref)`).
  Every test that expected a fleet became a **host**: **twelve** test packages
  (`cmd/agezt`, `cmd/agt`, `kernel/app/configtool`, `kernel/app/guardians`,
  `kernel/app/overseer`, `kernel/app/runs`, `kernel/cadence/systemtasks`,
  `kernel/controlplane`, `kernel/modules/agents/reaper`,
  `kernel/modules/agents/selfrepair`, `kernel/runtime`, `plugins/builtintools`)
  now open the roster themselves and inject it, the same way the daemon does;
  the store is opened **once per config**, so a test that reopens the same
  config — a **restart** — keeps the same store and a profile written before
  the restart is still there after it, which is the whole point of the file.
  **11 mutations, 11 caught** in two honest passes, **four** of them
  **gate-measured** by archcheck (the gateway names the module again; the
  control plane names the module again; the kernel names the module again; the
  run engine names the module again), and **three** of them measure a **rule**
  rather than a shape: the runbook rule decides whether a **managed** agent may
  be woken from outside (a rule that quietly admitted one would let a
  delegation-only agent be woken by an operator, and no structural check can
  see it), and the sentinels are what the layers above the module **compare
  against** (a module that raised a look-alike error of its own, or that kept
  a sentinel of its own, would leave every `errors.Is` comparison silently
  false, and the layer would report "no such agent" as some other failure).
  The sixth mutation again had to travel with the **run executor's `KernelAPI`
  declaration**, because drifting the kernel's face alone is a **build error**,
  not a catch (the same lesson as the catalog, world-model and skill slices,
  met for the **fourth** time).
  Three honest corrections are worth naming: the **first pass was invalid** —
  the second mutation's anchor pointed at the wrong file, so the case reported
  **SKIP**, and a SKIP is neither a catch nor a pass, so the anchor was
  corrected and the whole run repeated; the run engine's mutation was a
  **parse error** (the reference was inserted inside the interface body, where
  a declaration cannot sit), so it was moved after the interface; and the
  sentinel mutation was aimed at the **wrong test** — the alias test only
  compares the two **values**, so it stayed green and the mutation
  **survived** — and the expectation was moved to the test that actually reads
  the error the store raises; a **second** sentinel mutation (the module
  keeping a sentinel of its own) was added afterwards, so the alias half is
  measured too.
  The new permanent tests pin that the store port is **exactly eight** verbs
  and keeps their exact signatures, that the reader is **two** and the repair
  surface is **one**, that the two sentinels exist and are **distinct**, that
  the **runbook rule decides right** (the trigger, the route, the recovery and
  the sleep contract, each in its own case), that the module's rows and
  sentinels **are** the contract's (by **type identity**), that the store's
  refusals **are** the contract's sentinels, that the kernel's face is
  **statically** the contract's port, that the kernel opens **no** roster
  directory of its own, that a kernel without a roster answers "no agents" on
  the reads and **refuses** on the writes (never a panic, never a swallowed
  write), that the injected roster is the live one, and that the daemon's
  roster is a **directory on disk** and is the **injected** one.
  **A correction to a carried number**: the previous slices' documents say "the
  kernel still reaches **7** modules". Recomputing the metric — the distinct
  `kernel/modules/...` packages the allowlist still names — gives **six**
  after W3.55, not seven: the number had been carried forward without
  recomputation since W3.51. This slice takes it to **five**
  (`agents/roster` is gone; `board/board`, `extensions/mcp`,
  `extensions/toolbox`, `knowledge/memory` and `work/assure` remain).
  **What remains**: the ratchet stands at **24/13**. Next, in order:
  **`memory`** (six edges; the manager is bus-bound AND LLM-bound — distill and
  embeddings — so its port would be the widest of all, and it is the **third**
  bus-bound module, so the builder-plus-publisher pattern applies), then
  **`mcp`** (two edges, the smallest remaining family).

- W3.57 (the memory is the daemon's) is committed on `main` the same way. It
  takes **six** edges and it takes them **completely**:
  `cross-module kernel/runtime -> kernel/modules/knowledge/memory` (the kernel
  held the module's concrete `*memory.Manager` and **opened the file** itself),
  `cross-module kernel/runtime/runexec -> ...` (the run-executor declared the
  module's concrete manager in its `KernelAPI`),
  `adapter-bypass kernel/controlplane -> ...` (the control plane named the
  module's rows), `adapter-bypass kernel/agentgw -> ...` (the agent gateway
  named the module's rows), `cross-module kernel/contextselect -> ...` (the
  context selector named the module's rows) and `cross-module
  kernel/modules/knowledge/memory -> kernel/agent` (**the module reached into
  the runs module for the structured-output path**).
  **The ratchet went DOWN by SIX: 24/13 → 18/13**; the **13** allowlisted call
  sites are unchanged.
  This is the **third bus-bound module** (after the world model and the skill
  forge) and the **first one that is also LLM-bound**: the manager distils
  facts, brains and profiles through a model and embeds text through an
  embedder, so both seams had to become **ports**.
  The contract `memoryapi` is **new** — the memory had no contract before, and
  that was the whole reason the layers above it had to name the module. It
  carries the rows (`Record`, `RememberSpec`, `Scored`, the type and evidence
  vocabularies, the profile, brain, hygiene, audit and clean reports), the
  **sentinels** (`ErrRecordNotFound`, `ErrEmptyContent`), the **rules**
  (`ValidType`) and the **ports**: a six-verb `Store`, a one-verb `Publisher`,
  a **nine-verb** `Reader`, a **five-verb** `Writer`, a **five-verb**
  `Maintainer`, a **three-verb** `Distiller`, a one-verb `Tooler`, a
  `ReaderWriter` and the `Manager` — the composition of the halves. The module
  keeps every name as an **alias**, so there is one row and one spelling of it
  and `errors.Is` matches on **both** sides of the module boundary.
  **The port is SPLIT six ways, not widened**: the gateway only looks notes up
  and writes them, so it takes the `ReaderWriter`; the context selector only
  reads, so it takes the `Reader`; a single wide port would have handed the
  gateway the whole memory lifecycle (audit, prune, distil).
  **The rules travel with the rows**: `Record.Active()`, `Record.Suspended()`,
  `Record.Expired()` and `Record.Usable()` decide whether a note still counts,
  and a Go type alias **cannot carry methods** — had the rules stayed in the
  module, the alias would have lost them and the layers above would have had
  to reach back into the implementation to judge a record. `ValidType` is the
  same story: the membership set is part of the type's meaning.
  The **structured-output sentinel moved too**: `llm.ErrNoObjectGenerated` is
  the contract's, and `kernel/agent` keeps the name as an alias — that is what
  lets the `memory -> kernel/agent` edge go at all: the path became a **port**
  (`memoryapi.Generator`) the **daemon injects**, and the module never names
  the runs module.
  The **publisher answers with the event's id**, not the envelope: the durable
  envelope carries the hash chain and the contract layer may not name it, and
  what the manager needs is the id — the record's `SourceEvent` provenance.
  The **context seam lives in the contract** (`WithCorrelation` /
  `CorrelationFrom` / `WithScope` / `ScopeFrom`): a context key is an
  **identity**, and two packages that each define one would not see each
  other's values; the module keeps the two **readers** and the **writers** are
  the contract's alone.
  The **test seam stays off the port**: `SetEmbedder` is deliberately not a
  port verb — the embedder is a live-configuration seam given at construction
  time (`Deps.Embedder`).
  The **file is the daemon's**: `cmd/agezt`'s composition root opens
  `memory/memory.json`, hands over the store, names the module through the
  **builder** and injects the generator; the kernel calls the builder with the
  store and its own publisher, so the manager journals through the kernel's bus
  without either side naming the other's implementation.
  A daemon with **no** memory answers, on purpose, and **the answers differ by
  direction**: the **reads** answer **"no memory"** — `MemoryCount` is 0,
  `MemoryActive` is empty, `MemoryProfileText` is empty, the searches find
  nothing — while the **writes refuse** with `ErrNoMemory` rather than calling
  into nothing; the refusal is deliberately **not** a silent no-op, because
  "nothing was forgotten" and "there is nothing to forget" are different
  answers.
  Every test that expected a memory became a **host**: **nine** test packages
  (`cmd/agezt`, `kernel/app/introspect`, `kernel/app/system`,
  `kernel/cadence/systemtasks`, `kernel/controlplane`, `kernel/runtime`,
  `plugins/builtintools` and the shared `kernel/internal/testfixtures`) now
  open the memory themselves and inject it, the same way the daemon does; the
  store is opened **once per config**, so a test that reopens the same config
  — a **restart** — keeps the same store and a note written before the restart
  is still there after it, which is the whole point of the file.
  **13 mutations, 13 caught** in two honest passes, **five** of them
  **gate-measured** by archcheck (the gateway names the module again; the
  control plane names the module again; the kernel names the module again; the
  run engine names the module again; and **the module names the runs module
  again** — a reach no test can see, because the distillation still works), and
  **four** of them measure a **rule** rather than a shape: the record's
  **usable rule** decides whether a note may enter a run's context (a rule that
  quietly admitted a forgotten or an expired record would put deleted knowledge
  back in front of the agent, and no structural check can see it); the
  **sentinels** are what the layers above the module **compare against** (a
  module that kept a look-alike error of its own would leave every `errors.Is`
  comparison silently false, and the layer would report "no such note" as some
  other failure); the **validity rule** is part of the type's meaning (a module
  that kept a set of its own could drift from the contract's, and the layers
  above would accept a type the contract rejects); and the
  **structured-output path** is the only way a distillation pass can turn a
  model's answer into a typed value (a kernel that dropped it would leave every
  distillation **refusing** — the agent would silently stop learning, with no
  error anywhere).
  The seventh mutation again had to travel with the **run executor's
  `KernelAPI` declaration**, because drifting the kernel's face alone is a
  **build error**, not a catch (the same lesson as the catalog, world-model,
  skill and roster slices, met for the **fifth** time).
  Three honest corrections are worth naming: the **first pass had a broken
  mutation** — the contract-port mutation inserted the reference **inside** a
  `var` block, where a declaration cannot sit, so the file stopped parsing and
  the case reported **BUILDERR**, and a compile error is not a catch; the
  reference was moved **after** the block. The **row mutation could not compile
  at all**: replacing the module's `Record` **alias** with a look-alike struct
  broke the module's own code everywhere, so it measured nothing — it was
  replaced with a mutation that **compiles** (the module keeps a validity set
  of its own instead of the contract's), which is the same promise in a form
  that can be observed. And the **generator mutation was aimed at the wrong
  side**: removing the daemon's `cfg.MemoryGenerator = agent.GenerateObject`
  also **unused** the `agent` import, so the package stopped compiling; the
  mutation was moved to the **kernel's** side (`Generator: nil` in the
  composition root), where it compiles and where the daemon's end-to-end
  distillation test sees it.
  The new permanent tests pin that the store port is **exactly six** verbs, the
  publisher **one**, the reader **nine**, the writer **five**, the maintainer
  **five**, the distiller **three**, the tooler **one**, that the manager is
  **exactly** the union of its halves (no verb of its own), that the
  reader-writer carries neither maintenance nor distillation, that the
  sentinels exist and are **distinct**, that the **record rules decide right**
  (plain, forgotten, superseded, suspended and expired records, each in its own
  case), that the **validity rule** is the contract's over the whole domain,
  that the **context seam is shared** and orthogonal, that the module's rows
  and sentinels **are** the contract's (by **type identity**), that the
  kernel's face is **statically** the contract's port, that the kernel opens
  **no** memory directory of its own, that a kernel without a memory answers
  "no memory" on the reads and **refuses** on the writes (never a panic, never a
  swallowed write), that the injected memory is the live one, and that the
  daemon's memory is a **directory on disk**, is the **injected** one, and that
  its generator really drives a distillation.
  **The module count, recomputed**: the distinct `kernel/modules/...` packages
  the allowlist still names went from **five** to **four** (`board/board`,
  `extensions/mcp`, `extensions/toolbox` and `work/assure` remain). The
  allowlist also names `kernel/agent` and `kernel/runtime` — the runs module
  and the kernel itself — but those are not `kernel/modules/...` packages, so
  they do not count toward the metric the previous slices carried.
  **What remains**: the ratchet stands at **18/13**. Next, in order:
  **`mcp`** (two edges, the smallest remaining family), then
  **`kernel/modules/board/board`** (named by three places — the control plane,
  the REST API and the self-repair module).

- W3.58 (the MCP registry is the daemon's) is committed on `main` the same way.
  It takes **two** edges and it takes them **completely**:
  `cross-module kernel/runtime -> kernel/modules/extensions/mcp` (the kernel held
  the module's concrete `*mcp.Store` and **opened the file** itself) and
  `cross-module kernel/market -> ...` (the marketplace named the module's rows).
  **The ratchet went DOWN by TWO: 18/13 → 16/13**; the **13** allowlisted call
  sites are unchanged.
  The contract `mcpapi` **already existed** — the agent-facing MCP tool is a
  PLUGIN and may reach the contract and nothing else — and it carried the
  `Server` row, its `Validate` rule and the `Runner` port. What was missing was
  everything the **registry** and the **client** needed: the `ToolDef` row, the
  `Conn` port (**three** verbs: `Tools`, `Call`, `Close`), the `Dialer` and
  `HTTPDialer` **seams** (how an attachment is made), the `ErrNotFound` sentinel
  and the `Registry` store port (**six** verbs: `Add`, `SetEnabled`, `Remove`,
  `Get`, `List`, `Count`). The module keeps every name as an **alias**, so there
  is one row and one spelling of it and `errors.Is` matches on **both** sides of
  the module boundary.
  **The port is SPLIT, not widened**: the catalog layer only lists, so it takes
  the **one-verb** `RegistrationReader`; the lifecycle drives the six verbs
  through the kernel. Handing the catalog the whole registry would have given
  it the lifecycle it has no business driving — and the catalog is the layer
  that renders the operator's list.
  **The dialers became the host's seams**: the clients live in the module and
  the kernel may not name them, so the daemon injects `mcp.Dial` and
  `mcp.DialHTTP`. The old **fallback** — the kernel calling `mcp.Dial` itself
  when no dialer was configured — was exactly what kept the edge alive, and it
  is gone: a kernel with no dialer now **refuses** the attach with a readable
  error, because a nil func value would panic at the call.
  The **file is the daemon's**: `cmd/agezt`'s composition root opens
  `mcp/servers.json` and hands over the port; a registry that cannot be opened
  is a **startup error**, and because the file lives under the base dir a
  **tenant** keeps its own registry. The module is **not bus-bound** — the
  lifecycle journaling lives one layer up (the kernel presses the journal), so
  there is **no publisher port and no builder** to inject: the same shape as the
  roster (W3.56).
  A daemon with **no** registry answers, and **the answers differ by
  direction**: the **reads** answer **"no servers"** (`MCPRegistrations`,
  `MCPRegistrationCount`, `MCPRegistration`, `MCPReader`), while the **writes
  refuse** with `ErrNoMCPRegistry` rather than calling into nothing. The
  **nil-port surface lives in the kernel and only there**: the raw port is a
  **nil-able** thing, and the control plane used to hand it to the catalog and
  call `List()` on it — the catalog now reads through the kernel's nil-safe
  reader, so the one place that knows about the absence is the kernel.
  Every test that expected a registry became a **host**: **four** test packages
  (`cmd/agezt`, `kernel/controlplane`, `kernel/runtime` and
  `plugins/builtintools`) now open the registry themselves and inject it, the
  same way the daemon does; the store is opened **once per config**, so a test
  that reopens the same config — a **restart** — keeps the same registry and a
  server registered before the restart is still there after it, which is the
  whole point of the file.
  **13 mutations, 13 caught** in two honest passes, **two** of them
  **gate-measured** by archcheck (the kernel names the module again; the
  marketplace names the module again — a reach no test can see, because the
  servers still show up), and **four** of them measure a **behaviour** rather
  than a shape: the **sentinel** is what the layers above the module **compare
  against** (a module that kept a look-alike error of its own would leave every
  `errors.Is` comparison silently false, and the app would report "unknown mcp
  server" as some other failure); the **shapes** are what the layers above the
  module **hold** (a copy with the same fields satisfies every assignment and
  still is a second shape — and a second shape drifts on its own); the
  **dialer refusal** is the difference between a readable error and a panic (a
  nil func value does not fail when it is called; it crashes the daemon); and
  the **write refusal** is the difference between an honest answer and a panic
  ("there is no registry" and "the write did nothing" are different answers,
  and only the first is true).
  Two harness lessons are worth naming. The **shape mutation had to be
  re-aimed**: the first attempt replaced the module's `ToolDef` **alias** with
  a look-alike struct, but the module's own client returns that row, so the
  package stopped compiling — a mutation that breaks the build measures
  nothing. It was replaced with one on the `Dialer` **seam**, which carries the
  same promise in a form that compiles and can be observed (a named func type
  with the same underlying shape is still a **different type**). And the
  **dialer mutation needed the guard to be found, not just changed**: the
  kernel has **two** dialer guards with the same text — the http one and the
  stdio one — so the harness had to learn to replace the first line matching a
  pattern *after* an anchor; replacing the first occurrence alone would have
  measured the http seam and said nothing about the stdio one.
  The new permanent tests pin that the registry port is **exactly six** verbs,
  the catalog's reader is **one**, a live attachment is **exactly three**, that
  the two dialer seams carry their **transport** signatures, that the sentinel
  lives in the contract and a look-alike with the same message does **not**
  match it, that the advertised tool's shape and wire form are right, that the
  module's shapes and sentinel **are** the contract's (by **type identity**),
  that the kernel's face is **statically** the contract's port, that the kernel
  opens **no** registry of its own, that a kernel without a registry answers
  "no servers" on the reads and **refuses** on the writes, that the injected
  registry is the kernel's registry, that a kernel with no dialer **refuses**
  the attach instead of panicking, that a catalog with no reader answers "no
  servers", and that the daemon's registry is a **file on disk**, is the
  **injected** one, and that its dialers really reach the spawn.
  **The module count, recomputed**: the distinct `kernel/modules/...` packages
  the allowlist still names went from **four** to **three**
  (`board/board`, `extensions/toolbox` and `work/assure` remain).
  **What remains**: the ratchet stands at **16/13**. Next, in order:
  **`kernel/modules/board/board`** (named by three places — the control plane,
  the REST API and the self-repair module).

- W3.59 (the board's port is the contract's) is committed on `main` the same
  way. It takes **three** edges — the **last** board edges — and it takes them
  **completely**: `adapter-bypass kernel/controlplane -> kernel/modules/board/board`
  (the control plane held the module's concrete `*board.Store` **and opened the
  board's file itself** as a read fallback), `adapter-bypass kernel/restapi -> ...`
  (the REST mailbox held the concrete store) and
  `cross-module kernel/modules/agents/selfrepair -> ...` (the self-repair module
  named the module for **one row** — the message). **The ratchet went DOWN by
  THREE: 16/13 → 13/13**; the **13** allowlisted call sites are unchanged.
  This slice is **not** "the file is the daemon's" — the board's file was
  **already** the daemon's (`cmd/agezt` opened it and every holder shared that
  one instance); it is about the **port**, because the board's surface had
  **three shapes** — the contract's port, the module's concrete store and the
  app's own copy of the same **eleven** methods, and the last two are what the
  layers above the module had to name.
  The contract `boardapi` needed **nothing new**: the store's port was already
  there (the board **tool**, a PLUGIN, has taken the store through it since
  W3.21, and the tool registry has held it since W3.39a) — what was left were
  the **adapters** and the **module** naming the module anyway. The one shape
  the slice closed is the **duplicate**: `kernel/app/board`'s own interface is
  now an **alias** of the contract's port, so the board's surface has ONE
  shape instead of three.
  The daemon converts the concrete store to the port **where the value is
  still concrete** (`boardPort`), and that is not academic: the board store is
  opened **late** and the daemon leaves it nil when it could not be opened, so
  a nil `*board.Store` handed to self-repair's `Mailbox` interface would have
  been a **NON-NIL** interface — the callee's nil guard would not have fired
  and the first escalation would have dereferenced a board that was not there.
  The same rule was already in place for the board **tool** (`lateBoard`),
  which is why the tool had never crashed on a board that could not be opened.
  **The control plane's read fallback is GONE, and that is a behaviour
  change**: the control plane used to open a fresh read-only `Open` of the
  board's file when nothing was wired (three tests pinned it, one was named
  for it), so a board that **could not be opened** used to look like a board
  with **no messages** — a silent lie, while the writes were already honest.
  The reads now answer **"the board is not available on this daemon"**,
  exactly like the writes did; in the daemon's real topology nothing changes,
  because the daemon opens the board and wires it. The one fresh read-only
  open that remains is the **overseer app's** `OpenHelp`: the app layer is the
  layer that may see the module, and it opens the same file read-only.
  **10 mutations, 10 caught** in two honest passes, **three** of them
  **gate-measured** by archcheck (a reach no test can see — the board's
  answers would be exactly the same, because the module and the contract carry
  the same row and the same rules), and **four** of them measuring a
  **behaviour** rather than a shape: the **nil-port rule** (the difference
  between a daemon that says "the board is not available" and one that binds a
  board that is not there — and a nil value does not fail when it is called);
  the **refusal answer** (what the operator reads: "nothing was sent" and
  "there is no board" are different answers, and only the second is true); the
  **503** (what an SDK client sees: a successful empty mailbox is not "the
  board is unavailable"); and the **file's path** (the contract with the
  operator: the same file has to be there after a restart).
  The harness needed **one new capability**: the control plane has **two**
  guards with the same shape (the reads and the writes), and the daemon's
  `boardPort` has a nil check that another function also has — so the harness
  replaces the first line matching a pattern *after* an anchor. Replacing the
  first occurrence alone would have measured the wrong guard.
  The new permanent tests pin that the board's port is **exactly eleven**
  verbs, that the wildcard recipient is pinned by **value**, that the app's
  store port **IS** the contract's port, that the control plane's face is
  **statically** the contract's port, that a control plane with **no** board
  refuses on the reads **and** the writes, that an unwired REST mailbox
  answers **503**, that the daemon's board is a **file on disk** and that the
  port it hands over is live, that a board that could not be opened stays a
  **nil PORT**, and that the self-repair mailbox port is served by the
  contract's store surface and carries the contract's row.
  **What remains**: the ratchet stands at **13/13** — and the **13** call
  sites are now the whole of the remaining debt. The next slice is a
  **call-site** slice rather than an import slice.

- W3.60 (the control plane's files are written atomically) is the **first
  **call-site** slice, and it takes the **three** control-plane sites:
  `runtime/control.addr`, `runtime/control.token` (the **primary admin
  token**) and `chat_prompts.json`. The ratchet went **13 → 10** call sites;
  the **import** ratchet stayed at **13**, because `internal/atomicfile` is a
  **foundation** package (layer `L0`) and a downward import costs no edge —
  the control plane names the platform's writer **directly**, not through
  `kernel/platform/filestore`.
  `os.WriteFile` opens the target with `O_TRUNC`, so a reader in another
  process (the CLI, the SDK, the Web UI — none of them under the daemon's
  lock) can see a **torn** file, and it applies its mode only on **CREATE**, so
  the mode on disk was a property of *whoever wrote last* — a leftover
  **read-only** file even made the daemon's own write fail outright
  (measured: "Access is denied"). The prompt library is the sharp end:
  `kernel/app/persona` **drops an unparseable** library instead of failing the
  read, so a torn file does not surface as an error — it surfaces as an
  **empty library**, with no reason given.
  The platform's behaviour was **measured, not assumed** (Windows: a rename
  over a file another handle holds open is **refused**; a path stat does
  **not** carry the file index, so the identity has to come from an **open
  handle**), so the tests assert the **guarantee** — the identity changes, the
  file is writable by its owner, the content is whole — and hold on both
  platforms. The permission-bit checks are POSIX-only; the **owner-writable**
  guarantee is checked on both, because Windows carries the read-only
  attribute.
  **12 mutations, 12 caught** in two independent passes with identical logs;
  **one** of them is **gate-measured** by archcheck (a raw write reappearing
  in the control plane), and the rest measure a **behaviour**: the reader that
  must not see a torn token, the leftover read-only file that must not stop the
  daemon's own write, the mode that must not survive from an earlier run, the
  path that must be the same after a restart, the content that must be whole.
  The harness had to learn two things first: a **wrong test name** is not a
  survival (`go test -run` answers `no tests to run` and exits **0** — the
  harness now classifies that as its own verdict), and a **build error** is not
  a catch (the mutation that reverted the prompt library left the `atomicfile`
  import unused, so the mutation now removes the import too and the **test**
  does the catching).
  **What remains**: the ratchet stands at **13/10** and the **10** remaining
  call sites are the whole of the raw-write debt — `kernel/market` 4 (two of
  them already staged and swapped in), `plugins/tools/codeexec` 3,
  `kernel/executionprofile` 1, `kernel/modules/skills/skill` 1 (already
  staged), `plugins/tools/browser` 1. The counter has one **blind spot**:
  `cmd/agt/market_publish.go` writes an **Ed25519 private key** with
  `os.WriteFile`, and `cmd/...` is layer `L7` (root), which is **exempt** from
  the forbidden-call rules — the most valuable raw write in the tree is one the
  ratchet cannot see. That is the next slice to measure.

- W3.61 (the sandbox's workspace files are written atomically) is the
  **second call-site slice**, and it takes **four** of the ten sites that were
  left, in **two** packages: the code_exec tool's three writes (the **entry
  script** the interpreter runs, the **extra files** the program reads, the
  **stdin file** the program reads) and the execution profile's **secret
  mount** (the **vault's value**, handed to the child process by path). The
  ratchet went **10 → 6** call sites; the **import** ratchet stayed at **13**,
  because `internal/atomicfile` is a **foundation** package and a downward
  import costs no edge.
  The ten sites that were left after W3.60 were **measured first**, and the
  measurement decided the slice: six of them are **already atomic by another
  means** (`kernel/market/sources.go` and
  `kernel/modules/skills/skill/bundle_io.go` write into a **staging
  directory** that is swapped in as a whole), and the four that remained have
  the **widest reader distance** — their files are read by a **child
  process**, or (for the SSH/K8s/Modal/Daytona backends) by **another
  machine** that receives them. A write that truncates in place leaves a
  window in which the file is short, and a short script is not an error the
  interpreter can report: it may be a **valid prefix that runs and stops
  early**. A torn secret mount is a **partial API key** in the child, which
  reads as an authentication failure rather than as a broken file. And the
  **mode** was never the file's — `os.WriteFile` applies its mode only on
  **CREATE**, a **project** workspace keeps its files across calls, and a
  leftover **read-only** file made the write **fail outright** (`EACCES` on
  POSIX, "Access is denied" on Windows).
  The slice also makes one **deliberate behaviour change**: the stdin write's
  error used to be **swallowed** (`_ = os.WriteFile(...)`), and it is now
  reported like every other write in the same function — because the program
  **reads** that file, and a silently missing stdin reads as EOF and looks
  like an operator who typed nothing.
  **10 permanent tests** over the writer's **guarantee** — the file's
  **identity** changes when it is rewritten (measured through an **open
  handle**, because a Windows path stat does not carry the file index), the
  file is writable by its owner, the content is whole, and a failed stdin
  write is reported. **12 mutations, 12 caught** in two independent passes
  with identical logs; **two** of them are **gate-measured** by archcheck.
  The **first** pass reported two survivors, and both were a **test gap**
  rather than a code gap: a test that asserts the **content** cannot tell a
  rename from a truncate, because the bytes a reader sees are whole either
  way — so the guarantee was re-measured where it lives, in the file's
  **identity** (`TestTheWorkspaceExtraFilesAndStdinAreReplacedNotRewritten`).
  **What remains**: the ratchet stands at **13/6**; the six remaining sites
  are `kernel/market` 4 (two of them already staged),
  `kernel/modules/skills/skill` 1 (already staged) and
  `plugins/tools/browser` 1. The interesting one is the market's **publish**
  path: the **sync** path already stages and swaps, but the **publish** path
  writes the pack and the index **in place** into a directory meant to be
  **served statically** to remote clients — and the writer reads the index
  back while **ignoring the parse error**, so a torn index does not fail the
  publish, it **resets** the catalogue and silently drops every previously
  published pack. That is the next slice to measure. The ratchet's **blind
  spot** is still there: `cmd/agt/market_publish.go` writes an **Ed25519
  private key** in place, and `cmd/...` is layer **L7** (root), which is
  exempt from the forbidden-call rules.

- W3.62 (the published marketplace is never half-written) is the **third
  call-site slice**, and it takes **two** of the six sites that were left,
  both in `kernel/market`: the **published pack body** and the **marketplace
  index**. The ratchet went **6 → 4** call sites; the **import** ratchet
  stayed at **13** — this package already imports the platform's writer (its
  own `atomicWrite` is a thin wrapper over `internal/atomicfile`), so the
  slice adds **no** import at all.
  The reason this path is different from the package's other one is that the
  published directory is **meant to be served statically**: two parties read
  what it writes — **remote clients** (`kernel/market/sync.go` fetches a
  remote `marketplace.json` and every pack it lists) and **the publisher
  itself**, which reads the index back on the next publish to merge the new
  pack in. A write that truncates in place leaves a window in which the file
  is short: a client fetches invalid JSON, or a pack it cannot verify.
  And the publisher's read-back was the sharp end, because of what it did
  with a file it could not parse — the parse error was **ignored**
  (`_ = json.Unmarshal(existing, &mp) // a corrupt index is overwritten
  fresh`), so a half-written index did not fail the publish: it was treated
  as "there is no catalogue here" and the index was written fresh with
  **only the pack being published**. Every pack published before it was
  **silently dropped from the catalogue** — the pack files stayed on disk,
  nothing listed them any more, and the operator was never told. The same
  thing happened to an index corrupted by anything else. That is the "silent
  lie" class this series has been closing: a **failure that looks like
  success**.
  Both writes now go through the package's own `atomicWrite`, and the
  read-back **refuses** instead of resetting: a corrupt index fails the
  publish and **names the file**, so the operator can move it aside or repair
  it — the corrupt file is left exactly as it was, because it is their
  evidence and their decision. **A behaviour change, deliberately**: before, a
  corrupt catalogue was emptied and the publish reported success. A
  marketplace directory with **no** index yet still publishes a fresh one —
  "no index" is a first publish, not corruption. **What this does NOT fix**
  is named rather than claimed away: the merge is a read-modify-write with
  no lock, so two **concurrent** publishes can still lose an entry — an
  atomic write prevents a torn file, not a lost update.
  **5 permanent tests** (the pack and the index are **replaced**, the index
  keeps **every** published pack, a corrupt index is **refused** with the
  file left untouched, a leftover **read-only** file cannot stop the next
  publish, and the published files are **readable by others** because the
  directory is served) and **10 mutations, 10 caught** in two independent
  passes with identical logs, **one** of them **gate-measured** by archcheck.
  The **first** pass reported two BUILDERRs, and both were the **mutation's**
  defect rather than the code's: skipping the index write left `idxBytes`
  **declared and not used**, and writing the pack empty left `packBytes`
  unused — a build error says nothing about the behaviour, so both mutations
  now write an empty slice of the same variable and the **test** does the
  catching.
  **What remains**: the ratchet stands at **13/4**, and three of the four are
  **already atomic by another means** (the market's **sync** cache and the
  skill bundle are written into a **staging directory** that is swapped in as
  a whole); the fourth is the browser action's **tab state** — a JSON file
  that survives daemon restarts, is read back by later `browser.action` calls
  to reuse a `tab_id`, and has **no mutex** around its save and read, so a
  truncated state that still parses would carry a **wrong URL** and the
  driver would navigate to it. The ratchet's **blind spot** is now the
  biggest remaining item, and it needs a different kind of slice:
  `cmd/agt/market_publish.go` writes an **Ed25519 private key** in place,
  `cmd/...` is layer **L7** (root) which is **exempt** from the
  forbidden-call rules, and `cmd/agt` has **no test** that calls
  `cmdMarketKeygen` or `cmdMarketPublish` at all — closing it needs the
  **rule** to change before the site can be fixed.

- W3.63 (the browser's tab state is replaced, not rewritten) takes **one** of
  the four sites that were left — `plugins/tools/browser/action_tabs.go`, the
  browser action's **tab state** — and the ratchet goes **4 → 3** call sites.
  The import ratchet stays at **13** (`internal/atomicfile` is a foundation
  package, so the downward import costs no edge). The three that remain are
  the ones the measurement already explained: the market's **sync** cache and
  the skill bundle are written into a **staging directory** that is swapped in
  as a whole.
  The tab state is the browser session's **memory**: one JSON file per session
  and tab, holding the tab's **final URL** and the **snapshot refs** a later
  call can reuse. It is read back by `resolveTabURL` (to fill in the URL of a
  `tab_id` the caller passed) and by `ResolveTabRef` (to turn a `ref` into a
  selector), it **survives daemon restarts**, and there is **no mutex**
  around the save and the read — so two concurrent `browser.action`
  invocations can interleave. A write that truncates in place leaves a window
  in which the state file is short: a state that does **not** parse fails the
  next call loudly, but a state that **does** parse can carry a **truncated
  URL** — and the driver would then navigate to it. That is the silent kind of
  failure. The mode had the same problem as everywhere else in this series:
  `os.WriteFile` applies its mode only on **CREATE**, the state file survives
  across calls and restarts, and a leftover **read-only** state made the next
  save **fail outright** (`EACCES` on POSIX, "Access is denied" on Windows) —
  the browser stopped recording the tab at all. The state is now written
  through the platform's canonical writer: a unique temp file, `fsync`,
  `chmod`, then a rename over the target — so it is **replaced**, the mode is
  **re-asserted** on every write, and a reader always sees a whole state.
  **5 permanent tests** (the state is **replaced** and holds the new URL and
  refs, a reader holding it open sees a **whole** file, a leftover
  **read-only** state cannot stop the next save, the state is not readable by
  others, and it stays inside the session root) and **6 mutations, 6 caught**
  in two independent passes with identical logs — including one that
  **truncates the saved URL** (the hazard itself) and one **gate-measured** by
  archcheck.
  The **first** pass reported one BUILDERR, and it was the **harness's**
  fault: the mutation that reverted the tab state left the `atomicfile`
  import **unused**, and the harness was supposed to remove that import as
  part of the mutation — its `extra` edit block was **missing from the
  script**. The verdict was honest (there *was* a build error), the cause was
  not: the harness had not applied the second half of its own mutation. That
  is the fourth time this series has run into this shape, and the rule is
  mechanical: a mutation that reverts a package's **last** atomic write must
  also remove the import it made unused.
  **What remains**: the ratchet stands at **13/3**, and **all three** remaining
  call sites are **already atomic by another means** — so the call-site debt
  is down to three writes the measurement already showed to be protected, and
  the allowlist is emptied of everything that was **not** already protected.
  The ratchet's **blind spot** is now the whole of the remaining raw-write
  work: `cmd/agt/market_publish.go` writes an **Ed25519 private key** in
  place, `cmd/...` is layer **L7** (root) which is **exempt** from the
  forbidden-call rules, and `cmd/agt` has **no test** that calls
  `cmdMarketKeygen` or `cmdMarketPublish` at all — closing it needs the
  **rule** to change before the site can be fixed.

- W3.64 (the assurance loop lives in the contract, and the module is gone)
  takes the **last** import edge that pointed *into* a module from the kernel:
  `cross-module kernel/runtime/runexec -> kernel/modules/work/assure`. The
  ratchet went **13 → 12** import edges; the forbidden-call ratchet is
  unchanged at **3** (the market's sync cache and the skill bundle, both
  written into a **staging directory** that is swapped in as a whole).
  The edge was **measured before it was touched**, and the measurement said the
  package **was never a module**: no store, no file, no bus, no model, no
  clock — it held **two closures** (`RunFn`, `VerifyFn`), a budget and a loop,
  and the contract layer's own rule ("rows, constants and pure functions only")
  already covered it. The **rows and the parser** had been in the contract
  since W3.31; what was left in `kernel/modules/work/assure` was the loop and
  a set of **aliases and delegations** — a pass-through package whose only
  production reason to exist was a name for something else. So the loop
  **moved** into `kernel/contract/assureapi/loop.go` (`Until`,
  `clampAttempts`, `retryInstruction` — unchanged line for line) and the
  **module package was deleted** (four files; the package count drops 315 →
  **314**). Keeping the module as an alias house was **rejected**: it reverses
  W3.31's documented decision without the loop's nature changing, it keeps the
  edge in the ratchet forever, and a delegation that exists only to be
  delegated to is a shape the dead-code gate has to be taught to forgive.
  **No signature changed** — `KernelAPI.RunAssured` returned
  `(string, assure.Result, error)` where `assure.Result` was a **type alias**
  for `assureapi.Result`, so the interface, the callers and the wire are
  identical; the module's `ParseVerdict` was a delegation to the contract's.
  **12 permanent tests** in the contract package (the seven loop tests moved
  with the loop, the three parse cases moved with the parser, and **two new**
  ones — the attempt number is **one-based**, and the history carries every
  attempt — because the mutation run found the attempt number was never
  asserted) and **10 mutations, 10 caught** in two independent passes with
  **byte-identical** logs, including the two that name the loop's real
  contract: the verifier must judge the **ORIGINAL** task (a gap-augmented
  task is a different task), and the gap must reach the next attempt (a retry
  that does not know what was missing is a blind retry).
  **The milestone**: after this slice the kernel reaches **no module
  implementation at all** — measured over every non-test file in
  `kernel/runtime`, the only module imports left are `agents/api` and
  `reasoning/api`, and an `api` package is the one import a module may make
  across boundaries (P5). The kernel is the **host**, and a host that reaches
  into a guest's internals is a host that cannot be reused.
  **What remains**: the ratchet stands at **12/3** — six `adapter-bypass`
  edges, three `cross-module` edges (module → module) and three `upward`
  edges, none of them a kernel reaching into a module's implementation. The
  three forbidden-call sites are the market's **sync** cache (2) and the
  **skill bundle** (1), both already staged and swapped in as a whole; the
  roadmap's **W5.4** wants the allowlist at **zero**, so they will be taken in
  a slice that makes the swap visible to the gate. The ratchet's **blind
  spot** is untouched: `cmd/agt/market_publish.go` writes an **Ed25519 private
  key** in place, `cmd/...` is layer **L7** (root) which is **exempt** from
  the forbidden-call rules, and `cmd/agt` has **no test** that calls
  `cmdMarketKeygen` or `cmdMarketPublish` at all — closing it needs the
  **rule** to change before the site can be fixed.

- W3.65 (the conversation collapse is the contract's) takes **two** import
  edges at once and the ratchet goes **12 → 10**:
  `adapter-bypass kernel/openaiapi -> kernel/convo` and
  `adapter-bypass kernel/webui -> kernel/convo`. The twelve remaining edges
  were **measured first, against each other**, and the measurement picked
  this one: it is the only candidate that removes **two** ratchet lines with
  the smallest possible port — **one row and one pure function**, no
  publisher, no clock seam, no file, no rule change.
  `kernel/convo` was two symbols in one file (`Turn{Role, Text}` and
  `TranscriptIntent(turns)`) whose only import was `strings` — no store, no
  file, no bus, no model, no clock. The same measurement that closed
  `kernel/modules/work/assure` in W3.64 applies here: **a package that holds
  no state is not a module**. And the two adapters that needed it are
  exactly the case the contract layer exists for: an adapter may reach the
  contract and the platform layers and nothing else, and the OpenAI-compatible
  API and the Web UI Chat view were reaching *into a layer-3 package* to
  render a conversation.
  The row and the rule moved to `kernel/contract/convoapi` (the body
  unchanged, line for line), the **package is deleted** (two files, plus its
  hand-written layer rule in `layers.json`), and the OpenAI-compatible API,
  the Web UI Chat view and the app layer's summarizer (`kernel/app/chat`, a
  legal edge that now names the contract too) all name the contract. The
  package count stays at **314** (one deleted, one added).
  **No behaviour changed**: the rule's body is byte-identical, no exported
  signature names the row, and `Turn` has no JSON tags — it is not a wire
  row.
  **9 permanent tests** — the six that travelled with the rule and **three
  new** ones the mutation run demanded: the rule's **doc comment** promised
  three things no test asserted ("Role is case-insensitive", "system and
  developer are leading guidance", "a lone user turn"), and a rule whose
  contract is only a comment is a contract that can drift.
  **10 mutations, 10 caught** in two independent passes with **byte-identical**
  logs; the eleventh thing this slice moved is **gate-measured** (an adapter
  reaching a module again is not something a test can see — the ratchet is
  the measurement).
  The **first** pass reported 8 caught and 2 skipped, and both skips were the
  **harness's** fault: one anchor did not exist (the rule builds the dialogue
  in a slice, not with a `WriteString` — the SKIP class reported it and never
  counted it as a catch), and the other was the sharp one: the harness
  compared the mutated file with the original using PowerShell's `-eq`, and
  **`-eq` is case-insensitive** — so the mutation that lowercases the "User:"
  label looked like no change at all, and the harness was blind to exactly
  the mutation it was built to see. The fix is one operator (`-ceq`).
  **What remains**: the ratchet stands at **10/3** — four `adapter-bypass`
  edges, three `cross-module`, three `upward`. The measurement also named the
  **trapped** ones: `controlplane → runtime` (the control plane calls **37**
  verbs on the kernel's god-object — a redesign, not a slice),
  `selfrepair → runtime` (the six agent rows could move to the reaper's
  `api` package, but `RunWith`/`RunWithRetry`/`ReaperScan`/`NewCorrelation`
  need a **runs port that does not exist** yet), and `channel → bus` (26
  plugin `Config`s hold `*bus.Bus` and nil-check it — an interface parameter
  turns a nil pointer into a **non-nil** interface, the guard stops firing,
  and the publish panics inside the deferred recover). The smallest **ready**
  edge is `pulse → agent`: one call site, one verb, and the project has
  already written the exact port (`memoryapi.Generator` is
  `agent.GenerateObject`'s signature). The two `agentgw` edges are **free
  riders** — `k.AgentGateway()` has no caller outside its own declaration —
  so their fix is a **move** (the daemon builds the gateway) and the price is
  that the gateway's wiring is untested today.

- W3.66 (the pulse's structured-output path is a port) takes one import edge —
  `cross-module kernel/pulse -> kernel/agent` — and the ratchet goes **10 → 9**.
  It is the smallest ready edge in the tree: **one import line, one call site,
  one verb**. The pulse module's optional LLM salience refine asks a model to
  score a delta, and the model's answer has to become a **typed value** — the
  structured-output path, `agent.GenerateObject` — and the module reached
  **into the runs module** to call it. The rule the architecture already has
  for this is the memory module's (W3.57): a module may not name another
  module; the host injects the path. The memory module got that treatment; the
  pulse module had not.
  **The path is not the memory module's and not the pulse module's** — it is
  everybody's, it is the way a model answer becomes a typed value — so it
  lives in the provider's own contract (`kernel/contract/llm`:
  `llm.Generator`), and `memoryapi.Generator` is now an **alias** of it. One
  spelling: two named func types with the same shape are **not** assignable
  to each other in Go, so a per-module port would have been a second name for
  one thing and a conversion at every call site.
  The salience carries the port, `pulse.Config` gained the field, the engine
  carries it in — and the **daemon injects** it (`Generator:
  agent.GenerateObject`), because the daemon is the composition root and the
  pulse module never names it. **The nil semantics are written down**: a host
  that wires no path gets **no** refine — the severity score stands and
  nothing pretends a model was asked — which is why the refine cannot panic
  on a nil path.
  **6 permanent tests** over a path that was **entirely untested** before
  this slice (nothing in the twelve pulse test files covered the refine): the
  refine asks the **injected** path and the model is asked **about this
  change**; the refine is **opt-in** (`PULSE_LLM=on`) so the path is never
  asked when the switch is off; no path means no refine and **no panic**; an
  unusable answer is a **skip**, not a zero score; the model's score is
  bounded to **[0,1]**. Plus the **daemon's** own end-to-end test: with a
  mock provider the journal must carry a `pulse.salience` event whose reason
  and score are the **model's** — because without it a daemon that forgot to
  inject the path would look exactly like a daemon with the refine switched
  off: the pulse keeps working, it just gets quieter, and nothing reports
  that the refine is dead.
  **10 mutations, 10 caught** in two independent passes with **byte-identical**
  logs, two of them aimed at the **wiring** (the daemon's hand-over and the
  engine's carry) that only the daemon's end-to-end test sees.
  The **first** pass reported three BUILDERRs, all **the mutation's** defect:
  the two clamp mutations removed a brace (a build error says nothing about
  the behaviour — they now move the *threshold* so the file compiles and only
  the guarantee breaks), and the mutation that removes the daemon's injection
  must also remove the `kernel/agent` **import**, whose `extra` edit was
  written in a single-quoted PowerShell string where a backtick-t is a
  literal backtick-t and not a tab — the import survived, `agent` became
  unused, and the package stopped compiling.
  **What remains**: the ratchet stands at **9/3** — four `adapter-bypass`
  edges, two `cross-module`, three `upward`. The traps are unchanged and are
  the map for the next slices: `controlplane → runtime` (the control plane
  calls **37** verbs on the kernel's god-object — a redesign, not a slice),
  `selfrepair → runtime` (the six agent rows could move to the reaper's
  `api` package, but `RunWith`/`RunWithRetry`/`ReaperScan`/`NewCorrelation`
  need a **runs port that does not exist** yet, so that port is a slice of
  its own), and `channel → bus` (26 plugin `Config`s hold `*bus.Bus` and
  nil-check it — an interface parameter turns a nil pointer into a **non-nil**
  interface, the guard stops firing, and the publish panics inside the
  deferred recover). The two `agentgw` edges are **free riders** —
  `k.AgentGateway()` has no caller outside its own declaration — so their fix
  is a **move** (the daemon builds the gateway) and the price is that the
  gateway's wiring is untested today, the same "wiring nobody proves" shape
  this slice just fixed for the pulse. The forbidden-call ratchet is still
  **3**, with two holes the measurement found: the staged writes are not
  **fsynced** (a swapped-in tree can hold a half-written file) and the swap
  **removes the old copy before** the new one is in place (a failure there
  loses the content; a partial removal leaves a half-deleted tree).

- W3.67 (the agent gateway is the daemon's transport) takes **two** import edges
  at once — `upward kernel/runtime -> kernel/agentgw` and
  `upward kernel/runtime/runexec -> kernel/agentgw` — and the ratchet goes
  **9 → 7**. The two edges were **free riders**: `k.AgentGateway()` had **no
  caller** outside its own declaration, so the fix could not be a port — it
  had to be a **move**. The gateway is an **ADAPTER** (L5): the capability-
  scoped HTTP transport the agent SDK's subprocesses talk to (bus publish and
  subscribe, memory read/write/search, logs, the roster, the config center).
  The kernel is a **module** (L3) and may not hold one — that was the upward
  edge, and it was a layering lie besides: a kernel that owns an inbound
  transport serves something it does not own.
  The **composition root** now builds it over the kernel's own ports
  (`k.Bus()`, `k.Memory()`, `k.Roster()`, `k.ConfigCenter()`, `k.Journal()`),
  starts it, and closes it — **one gateway per daemon, not per kernel**.
  The kernel's `agentGW` field, its `AgentGateway()` accessor and the
  `AgentGateway()` verb on `runexec.KernelAPI` are **gone**, and the
  accessor's **absence** is now pinned by name (like `ConfigMu`'s), because an
  accessor that comes back comes back with the edge this slice closed.
  The **close order** became a guarantee rather than an accident: the gateway's
  audit trail is buffered and flushed on Close, and the flush appends to the
  kernel's journal, so the daemon closes the gateway **first**
  (`shutdownAgentGateway`) — inside the kernel it was the other way round, and
  the flush was lost.
  **The daemon-level proof found two real defects**, and they are why this
  slice is more than a move:
  (1) the audit record's actor was the token's **parent** id, which is empty
  for a **top-level** token — the only kind `agt token create` mints — and an
  event without an actor is **rejected by the journal**, so every access made
  with a CLI-minted token was recorded **nowhere**. The trail that answers
  "who asked for what" was empty. A top-level token is now charged to
  itself;
  (2) the memory's type vocabulary is upper-case and validates strictly, so a
  client that wrote `"fact"` was refused with a write error for a type that
  **exists**. The gateway now canonicalises the spelling it accepts.
  **9 permanent tests**: five **daemon-level** end-to-end ones (the gateway
  really listens and answers `/health`; a token minted from the **install
  secret** — the same file the `agt` CLI mints from — is accepted and a memory
  round-trip works through it; an event published through the gateway lands in
  the daemon's **journal**; the config-center and roster routes answer instead
  of 404/503; and after the shutdown the audit trail is **on disk**), plus four
  **unit** ones over the two fixes (the actor is charged to the parent or to
  the token itself; the audit record reaches the journal for a top-level
  token; the type is canonicalised; the record carries its run and payload).
  **12 mutations, 12 caught** in two independent passes with **byte-identical**
  logs — nine aimed at the **wiring** (the socket override, the install
  secret, the memory, the bus, the roster, the config center, the audit
  journal, the listener itself, and the close order) that only the daemon's
  end-to-end test sees, two at the two fixes, and one at the kernel's accessor
  pin. Every mutation keeps its file's imports used, so a mutation that
  broke the build would have been a BUILDERR and never counted as a catch.
  **The dead-code gate found five more things while the tree moved**: once
  the daemon imports the gateway package, the analyzer (which runs **without**
  `-test`) reports the symbols that have no production caller — and
  `CreateSubprocessToken`, `CapsIntersect`,
  `CapabilityChecker.CheckAny/CheckAll` and `AuditLogger.LogSync` are exactly
  that: only the package's own tests use them. They were dead before the
  slice; the gate simply had not been asked about them. The gate's own rule
  (a same-package test-only symbol belongs in a **`_test.go`**, not in an
  allowlist) moved them to `kernel/agentgw/testonly_test.go`, where the file
  says what they are — and where the comparison between the library's copy of
  the subprocess-token rule and the HTTP path's (stricter about escalation,
  laxer about the burst) stays alive.
  **What remains**: the ratchet stands at **7/3** — four `adapter-bypass`
  edges, two `cross-module`, one `upward` (`channel → bus`). The traps are
  unchanged: `controlplane → runtime` (the control plane calls **37** verbs on
  the kernel's god-object — a redesign, not a slice), `selfrepair → runtime`
  (the six agent rows could move to the reaper's `api` package, but
  `RunWith`/`RunWithRetry`/`ReaperScan`/`NewCorrelation` need a **runs port
  that does not exist** yet), and `channel → bus` (26 plugin `Config`s hold
  `*bus.Bus` and nil-check it — an interface parameter turns a nil pointer
  into a **non-nil** interface, the guard stops firing, and the publish panics
  inside the deferred recover). The forbidden-call ratchet is still **3** with
  the same two holes: the staged writes are not **fsynced**, and the swap
  **removes the old copy before** the new one is in place.

- W5.4a (the forbidden-call ratchet is zero) takes the **call-site** half of
  W5.4 — the last **three** `raw-write` call sites — and the ratchet goes
  **3 → 0**. The three were the marketplace cache's two staged writes and the
  skill bundle's staged write, and the measurement that had called them
  "already atomic by another means" was **half right**: they were staged, but
  the staging tree was written with a plain `os.WriteFile` (**not fsynced** —
  a swapped-in tree could hold a half-written file), and the swap **deleted
  the old tree before** renaming the new one over it, so a rename that failed
  left the caller with **nothing**: the content was deleted, not moved, and
  could not be recovered. On Windows a rename is refused while a scanner still
  holds a handle on a file in the tree — and by then the old tree was already
  gone.
  The fix is **one shared policy** (`internal/dirswap`, a **foundation**
  package, so it costs no import edge): the new tree is written into a
  **staging sibling** (every file through the platform's atomic writer, so a
  reader never sees a half-written file and the content is fsynced before it
  is published), the old tree is **moved aside** (renamed, not deleted), the
  staged tree is renamed into place, and only then is the old tree removed.
  A publish that fails **moves the old tree back** — the destination is never
  left missing — and the leftover of a failed cleanup is bounded to **one**
  path per destination, which the next swap removes. The Windows retry that
  the skill package had grown on its own moved into the policy with it, so
  there is one implementation of it rather than two.
  **14 permanent tests** (11 over the policy, 3 over the two callers) and
  **12 mutations, 12 caught** in two independent passes with **byte-identical**
  logs. The **first** pass reported one BUILDERR, and it was the **mutation's**
  defect: skipping the index write left `idx` **declared and not used** — a
  build error says nothing about the behaviour, so the mutation now writes the
  index under the wrong name, which is a real defect the test can catch. Two
  tests are **measurements** rather than shapes: the mode is asserted as
  "writable" everywhere and as exactly `0600` only where POSIX permission bits
  exist (Windows has none), and the skill's failure is a file and a directory
  with the same name — whichever is written first, the other cannot be, so the
  test fails on every platform and in every map order.
  **What remains**: the forbidden-call allowlist is **empty**; the **import**
  ratchet stands at **7** with the same traps (`controlplane → runtime` is a
  redesign, `selfrepair → runtime` needs a runs port that does not exist yet,
  `channel → bus` is a nil-interface trap). W5.4's other two parts are still
  open: the **import** allowlist at zero and the **`docs/`** archiving.

- W5.4b (the toolbox reaches the control plane through the contract's port)
  takes the **first** of the seven import edges — the smallest — and the
  ratchet goes **7 → 6**. The edge was
  `adapter-bypass kernel/controlplane -> kernel/modules/extensions/toolbox`:
  the control plane is an **adapter** (L5), the toolbox is a **module** (L3),
  and an adapter reaches the app layer's typed operations, never a module
  (P2). The measured map had called the control plane's twenty-four-line
  `toolbox.go` a *wrapper* that could move to a contract/api package, and that
  was **half right** on both counts: the control plane did not wrap the module,
  it **implemented** it (`nativeToolboxReader`/`nativeToolboxInstaller` were
  the adapter's own types whose methods called the module's package
  functions), and moving those adapters would not have closed the edge — the
  adapter would still have called `toolbox.Detect`. The edge closes only when
  the **implementation** comes from somewhere the adapter cannot name, so the
  toolbox takes the board's (W3.59) shape: a new contract
  `kernel/contract/toolboxapi` owns the rows (`Inventory`, `ToolStatus`,
  `InstallResult`), the port (`Host`) and the one answer a daemon without a
  toolbox owes (`ErrUnavailable`); the module **implements** the port
  (`toolbox.Host`, `toolbox.NewHost()`) and re-exports the rows as
  **aliases** — one row, one spelling of it; the **daemon** (the composition
  root) calls `toolbox.NewHost()` and hands the port over in
  `controlplane.Deps.Toolbox`; the app layer speaks the contract and no
  longer imports the module **at all**, not even in its tests.
  A missing port is **not** an empty answer: "the toolbox is not available on
  this daemon" and "this machine has no tools" are different facts, so the
  reads and the install answer `ErrUnavailable`, and the install refuses
  **before** it publishes `toolbox.install.requested` — nothing will be
  installed, so nothing may be journaled as requested.
  **8 permanent tests** (the module's rows are the contract's rows and not
  copies; the module's host is the contract's port; the app's guards and its
  "publish nothing" rule; the control plane's setter and deps take the
  **contract's** port, asserted **statically** because an anonymous interface
  with the same verbs would compile and would be a second shape of the same
  port; a daemon without a toolbox says so and journals no toolbox lifecycle
  event; the operations reach the host's rows and not the machine; the
  daemon's deps reach the server) and **13 mutations, 13 caught** in two
  independent passes with **byte-identical** logs. The first calibration pass
  reported one **NO-TESTS** — the mutation was pointed at the app package
  while the test that observes the count lives in the control plane's: a
  defect of the **mutation**, and the two reported passes are the clean ones.
  **What remains**: the **import** ratchet stands at **6** —
  `controlplane → runtime` (a redesign), `controlplane → update` and
  `restapi → update` (the update port that `kernel/app/update` already is),
  `cadence/systemtasks → runtime` (nine accessors, two of them platform types
  the contract may not name), `selfrepair → runtime` (a runs port that does
  not exist yet) and `channel → bus` (the nil-interface trap). W5.4's
  remaining part is the **`docs/`** archiving.

- W5.4c (the update engine reaches the adapters through the contract's port)
  takes **two** of the six import edges at once — `adapter-bypass
  kernel/controlplane -> kernel/update` and `adapter-bypass kernel/restapi ->
  kernel/update` — and the ratchet goes **6 → 4**. The control plane and the
  REST surface are **adapters** (L5) and the update engine is a **module**
  (L3): an adapter reaches the app layer's typed operations, never a module
  (P2). The REST surface's own comment claimed it was *"decoupled from
  kernel/update"* **while importing it** — a claim about the code, not about
  the code's wishes. The measured map had called this pair *"the update
  service's port (`kernel/app/update` already exists)"* and that was right
  about the shape but wrong about the work: the app's `Backend` was built
  from the **module's rows**, so moving the adapters to the app layer would
  not have closed the edge — the port's *shape* was the module's, and an
  adapter holding it had to name the module's types. So the rows went down to
  a new contract `kernel/contract/updateapi` (the rows, the errors that cross
  the module's boundary and the port `Backend`), the module **aliases** the
  rows and its service satisfies the port, the app's `Backend` became an
  **alias** of the contract's port, and both adapters hold the contract's
  port. The daemon — the composition root — converts the concrete engine to
  the port **where the value is still concrete**: a nil `*update.Service`
  handed to an interface parameter is a **non-nil** interface, so the holder
  would hold an engine that is not there and the "update is disabled" answer
  could never fire (the board's rule, W3.59, applied a second time).
  The slice also found a real **crash**: `ErrChecksumMismatch.Error()` sliced
  the two hexes (`e.Want[:8]`) and the **wanted** hash comes from a manifest,
  which may carry a hash shorter than eight characters — so reporting a
  failed checksum **panicked**, in the error path, and
  `POST /api/v1/update/apply` with `"sha256":"abc"` reached it directly. The
  message truncates now. And there is **one** current version in the tree:
  the update module's second copy (`var CurrentVersion = brand.Version`) is
  gone, and the brand's — which the build stamps — is the only one.
  **16 permanent tests** (the checksum message survives a short hash and
  truncates a long one; the module's rows are the contract's rows and not
  copies; the drain timeout is one sentinel and not two; the module's service
  is the contract's port and its row carries the running version; the app's
  port is the contract's port; the app still reports the check error and the
  disabled status; both adapters' setters take the **contract's** port,
  asserted **statically**; the operations answer with the port's rows; a
  daemon with no engine answers "update is disabled" and names the running
  version; the REST surface answers a short-hash mismatch instead of
  crashing; the daemon's conversion turns a nil engine into a nil port) and
  **13 mutations, 13 caught** in two independent passes with
  **byte-identical** logs. The first calibration pass reported one SURVIVED
  and two BUILDERRs, and all three were the **mutation's** defects (the
  truncation assertion was a *prefix* of the untruncated hash, so the
  mutation slipped through; the other two broke the build before the
  behaviour could be measured).
  **What remains**: the **import** ratchet stands at **4** —
  `controlplane → runtime` (a redesign), `cadence/systemtasks → runtime`
  (nine accessors, two of them platform types the contract may not name),
  `selfrepair → runtime` (a runs port that does not exist yet) and
  `channel → bus` (the nil-interface trap). W5.4's remaining part is the
  **`docs/`** archiving.

- W5.4d (the channel guard reaches the bus through a port) takes the
  **channel** edge — the last one that was a single function — and the ratchet
  goes **4 → 3**. The edge was `upward kernel/channel -> kernel/bus`: the
  channel package is a **contract** (layer 1) and the bus is **platform**
  (layer 2), and a contract may not import the platform. The whole edge was
  **one function** — `Guard(b *bus.Bus, channelName string, fn func())` — and
  it existed so that one malformed inbound message could not take the daemon
  down: it recovers a channel handler's panic and journals it as a
  `channel.error` event **through the bus**. The measured map had called this
  edge *"the nil-interface trap"*, and the trap is real but it was **not**
  where the map put it: the trap is the **typed nil** — a plugin's config
  holds `*bus.Bus`, a plugin with no bus wired holds a nil pointer, and a nil
  pointer handed to an interface parameter becomes a **non-nil interface**; the
  old code checked the concrete pointer, so the moment the parameter became an
  interface the same value would have sailed past every nil check and the
  publish inside the recover would have panicked on a nil receiver. That is the
  same trap the update port walked into two slices ago (W5.4c) and the board
  port before that (W3.59). So the guard takes a **one-verb port**
  (`Publisher.Publish(event.Spec) (*event.Event, error)`) that the bus
  satisfies **structurally**, and the **eleven call sites**
  (`plugins/channels/*` and `cmd/agezt`) pass their `*bus.Bus` unchanged — the
  change is one file. The **error path is panic-safe on its own**: the publish
  happens inside the very `recover` that is supposed to contain the damage, so
  a publisher broken in its own right would have ended the process the guard
  exists to protect — the publish carries its own `recover`. And the guard
  **reports whether it attempted** to journal the panic, which is what makes
  the nil check load-bearing rather than decorative: a daemon with no publisher
  says so (false) instead of raising a panic of its own on every error event,
  and a publisher that refuses is still an attempt — the guard tried, and the
  publisher said no. **7 permanent tests** (the panic is recovered and a
  `channel.error` event is journalled; the handler runs and the quiet path
  journals nothing; **no publisher** still recovers; the **typed-nil**
  publisher is contained; a publisher that panics cannot take the daemon
  down; the journalled event names the channel and the actor and carries the
  panic value in its payload; the guard reports the attempt) and **11
  mutations, 11 caught** in two independent passes with **byte-identical**
  logs. The first calibration pass reported **one SURVIVED**, and the fix was
  in the **code**, not in the test: with the nil check and the panic-safe
  publish both in place, removing the check changed nothing observable — the
  two protections **overlapped**, and an overlapping protection cannot be
  observed from outside — so the guard was made to **report** its attempt, and
  the check became part of the contract instead of a decoration.
  **What remains**: the **import** ratchet stands at **3** —
  `controlplane → runtime` (a redesign), `cadence/systemtasks → runtime`
  (nine accessors, two of them platform types the contract may not name) and
  `selfrepair → runtime` (a runs port that does not exist yet). W5.4's
  remaining part is the **`docs/`** archiving.

- W5.4e (the system tasks reach the daemon's capabilities through the
  contract's ports) takes the **system-task** edge — `cross-module
  kernel/cadence/systemtasks -> kernel/runtime` — and the ratchet goes
  **3 → 2**. The cadence module reached the runs module and did not even name
  a *type*: the seven executors took the whole kernel
  (`func Run(ctx, k *kernelruntime.Kernel, corr, scheduleID, task string)`)
  and asked it nine questions — **the kernel was the API**. The edge was
  **measured first**, and the measurement said **seven of the nine accessors
  were ALREADY contract rows**, so the port could carry them verbatim. The
  **hard** two were the platform types a contract may not name (P4) —
  `Bus() *bus.Bus` and `Journal() *journal.Journal` — and the answer was
  already in the tree twice: `eventapi.Publisher` (the platform's event
  package keeps `Spec` and `Kind` as **aliases** of the contract's, so a
  spec built with `event.KindInfo` crosses the port unchanged — the same
  row, not a translation) and `journalapi.Reader` (the journal's own read
  port over its own `Record` projection; the log task needed the **whole**
  history and the chain's head, so the journal's contract grew a **second
  read port** over the **same** record — one vocabulary, two ports). The
  port set lives in the cadence module's **own** contract
  (`cadenceapi.SystemTaskPorts`) and the **kernel hands it over**
  (`k.SystemTaskPorts()`), the way the kernel hands over the memory publisher
  and the reaper's ports: the capability is the kernel's own, and the module
  that consumes it must not name the kernel. The conversion happens **where
  the values are still concrete** — the board's rule, applied for the fourth
  time: a nil `*bus.Bus` or a nil `*journal.Journal` handed to an interface
  parameter is a **non-nil interface**, so a capability the daemon does not
  have is a **nil port**, checked on the concrete value. **The ports are
  NARROW, and that is the point**: the artifact index is a **ten-verb** host
  port and two of its verbs **delete bytes** while a third (`SetRefGuard`)
  switches off the journal's protection of the bytes the journal still names
  — so the collect task gets **one** verb and not the index; and the memory
  port is the contract's **maintainer** half (five verbs), not the whole
  manager. **A missing capability is a REFUSAL, not an empty answer**: nine
  refusals, each naming what is missing — "this daemon has no memory" and
  "the memory is empty" are different facts, and a maintenance job that
  reports the second one is reporting a store it never looked at. The
  publish error is still **discarded** — a summary that could not be written
  must not fail the task it summarises — but a daemon with **no publisher at
  all** refuses. The slice also found a real **crash in the error path**:
  the catalog sync built its failure message from the reload's error
  **without checking it** (`provErr.Error()`), so a reload that answered no
  catalog and no error would have taken the daemon down — the same class as
  the checksum slice's `want[:8]` panic; the message is now built from what
  the port actually answered. **26 permanent tests** (the port set's shape,
  the journal's scan port, one per executor, the **refusal table**, the
  reload that answers no error, the kernel's side — the nil ports, the
  artifact index, the scanner's projection, the publisher — and the daemon's
  dispatch, asserted **statically**) and **27 mutations, 27 caught** in two
  independent passes with **byte-identical** logs. The calibration found
  **three test gaps** and one mutation defect, and all four are fixed and
  named: the memory **tidy's** refusal was never tested; the graveyard test
  could not tell the retired agents from the living ones; the catalog sync's
  **saved bytes** were never asserted; and one mutation orphaned the fetched
  bytes and broke the build before the behaviour could be measured.
  **What remains**: the **import** ratchet stands at **2** —
  `controlplane → runtime` (a redesign) and `selfrepair → runtime` (a runs
  port that does not exist yet). W5.4's remaining part is the **`docs/`**
  archiving.

- W5.4f (the stale docs are archived, and the index keeps them archived) takes
  W5.4's **last** part — the `docs/` archive — and it is **not only a
  documentation edit**: the tree carried four documents that were bannered
  `ARCHIVED` and were still listed in the index as current. **A banner
  archives nothing**, and an index does not remember it. Every `.md` in
  `docs/` was **measured** first (**42 files: 13 KEEP · 23 ARCHIVE · 5 UPDATE
  · 1 UNCLEAR**), and the `UNCLEAR` one was settled by running the thing it
  documents — `agt compare audit --json` answers *19 total, 12 supported, 6
  partial, 1 missing, 0 evidence missing* and `docs/COMPETITOR-PARITY.md:26-29`
  says exactly that, so the ledger is current and stays. The two that mattered
  most were `docs/ARCHITECTURE.md` (the most-cited architecture document in
  `docs/`, naming **six package paths that no longer exist** — `kernel/board`,
  `kernel/memory`, `kernel/skill`, `frontend/src/views/*`) and `docs/NEXT.md`
  (the June 2026 handoff — not a duplicate of `architecture/NEXT.md` but its
  **stale predecessor**). The **convention was chosen explicitly**, because the
  repo has three precedents that disagree: the in-place banner
  (`f86e56a2`), the `PHASE-ARCHIVE` README (*"PHASE files are NOT moved —
  keeping in place maintains git history"*, *"Files moved ❌ Not needed"*) and
  one deletion (`0dbc778f`, whose two documents were re-added a week later
  **with a banner**). Two of the three say **do not move**, and the roadmap's
  word is "archived", not "moved" — so **the archived documents stay where they
  are, carry an ARCHIVED banner that names their canonical successor, and are
  catalogued in one heading**. The decisive cost of the alternative is
  measured: moving them breaks **every inbound link**, and `README.md:23`
  points at `docs/SYSTEM-AUDIT-REPORT.md` while `kernel/journal/cursor.go:28-29`
  cites two of the refactor plans **in a live source comment**. **Nineteen
  banners** were added, the **index was rebuilt** with a live map and one new
  *Archived documents* section, and **five stale-but-wanted documents were
  fixed** (the marker that moved to the contract layer, the link to a plan file
  that was **never written**, two status banners that stopped in July, and the
  AGEZT half of the competitive roadmap). And because **a banner archives
  nothing**, the slice ships the gate that keeps the banner and the index in
  step: `tools/docsindex` checks that every local link points at a file that
  exists, that every document in `docs/` is listed, and that an ARCHIVED
  document is listed **only** under the archive section — and it runs from the
  `Makefile` (`make docs-index`, part of `make check`) and as a test the CI's
  `test (linux)` job picks up. **The gate found real problems on its first
  run**: `CONSOLE-IA.md` was in `docs/` and in **nobody's index** (a live
  document, pinned by `frontend/src/nav-docs.test.ts:100`, invisible to a
  reader since 2026-10-24); the four banner-archived documents were listed as
  current; and a link in the rebuilt index pointed at a file that did not
  exist. **7 permanent tests** and **9 mutations, 9 caught** in two
  independent passes with **byte-identical** logs — the gate *is* the product
  here, so the mutations target the **gate's own rules**. The first
  calibration pass reported **two BUILDERRs** and both were **the mutation's
  fault**: removing a check block left its variable unused and Go refused to
  compile — a compile error is not a caught mutation, it is a mutation that
  never ran. **What remains**: the **import** ratchet stands at **2**
  (`controlplane → runtime` is a redesign, `selfrepair → runtime` needs a runs
  port that does not exist yet). W5.4's other half is the **`cmd/agt` L7 blind
  spot** — the forbidden-call rules exempt layer 7, so the CLI's own
  `os.WriteFile` calls (including the one that writes an Ed25519 **private
  key** in place) are invisible to the gate.

- W5.4g (the CLI's file writes are visible, and the signing key is written
  atomically) takes the **last blind spot** in the forbidden-call gate: the
  scan skipped **layer 7 outright** (*"Packages in layers 0 and 7 are always
  exempt: foundation helpers ARE the implementations, and binaries/tooling
  compose"*), so every `os.WriteFile` in `cmd/` was invisible. **Layer 0 is
  right** — the foundation packages ARE the implementations, the atomic writer
  itself writes. **Layer 7 was not**: a binary legitimately spawns a process
  and calls an API (that is its job), but a **file** a CLI writes is a file
  the daemon may read back, and a plain `os.WriteFile` skips the atomic writer
  and the store framework's permissions. The blind spot hid **seventeen** call
  sites in `cmd/agt`, and **two of them mattered more than the other fifteen**:
  `cmd/agt/market_publish.go:103` wrote the operator's **Ed25519 signing
  key** — the marketplace publishing identity — with a plain `os.WriteFile`, so
  a crash mid-write leaves a **truncated key file** and a truncated signing
  key is an **unrecoverable loss** (the whole publishing identity is gone,
  with no error to show for it), and the `0600` mode is meaningless on Windows
  where the POSIX bits do not exist; and `cmd/agt/doctor_mesh.go:34,47,62,71`
  wrote the daemon's own **`memory.json`** — a real store the daemon reads
  back — with the same plain writer, bypassing the store framework that owns
  it. The exemption became **rule-aware** instead of blanket (the root keeps
  `exec` and `http-client`; it loses `raw-write`), and the rule change alone
  was not enough — it made every layer-7 package visible, and most of them
  should not be: `tools/archcheck` writes its own allowlists, `tools/sdkparity`
  writes `docs/SDK-PARITY.md`, `tools/structure-md` writes the generated
  structure files — **a generator IS the implementation of its own output**,
  which is exactly why layer 0 is exempt — so `layers.json`'s "calls" array
  gained them explicitly and **the CLI is not in that list**. The
  forbidden-call ratchet was **zero** after W5.4a and is now **one entry**
  (`raw-write cmd/agt 15`), and that is the honest reading: **the zero was a
  zero because the rule was not looking**. The import-edge ratchet has the
  same shape — it started at seven and is at two — and a ratchet that starts
  at fifteen may only go down. The signing key and its public half now go
  through the platform's one canonical writer (`internal/atomicfile.WriteFile`
  → **unique** temp, fsync, chmod, rename), so the file on disk is either the
  whole key or not there. **5 permanent tests** — the rule itself, and four
  for `agt market keygen`, which had **no test at all** (the written key is a
  key `market.PrivateKeyFromHex` accepts; no temp is left behind; the key
  file is `0600` — **POSIX only**, and the test says so instead of pretending
  on Windows; an unknown argument is an error that writes nothing). **6
  mutations, 6 caught** in two independent passes with **byte-identical**
  logs, and the proof is the **gate itself**: each mutation must make
  `tools/archcheck` fail **and name the site that regressed**. The
  calibration reported **one FAILED-OTHER** and it was the mutation's fault:
  its anchor also matched the **layer pattern**
  `{"pattern": "tools/...", "layer": 7}`, so the mutation produced a malformed
  `layers.json` and the gate failed with a JSON parse error instead of the
  expected finding. **What remains in the ratchet**: the **4 doctor store
  repairs** (a daemon store — the sharpest of the fifteen, and the one to take
  next), the **1 fixed-name temp** in `rollback_catalog.go` (the pattern
  `internal/atomicfile` replaced in M478), the **1 streaming** `os.Create` in
  `backup.go` (which needs a platform addition first), and the **9
  artifact/report** exports. The **import** ratchet stands at **2**
  (`controlplane → runtime` is a redesign, `selfrepair → runtime` needs a runs
  port that does not exist yet).

- W5.4h (the CLI writes the daemon's stores through the daemon's writers)
  pays down the six of the fifteen counted CLI write sites that are **not "a
  file the operator asked for"** — the ones that land on a file **the daemon
  reads back**. The rule that made them visible says it plainly: *"The CLI is
  NOT exempt: a file it writes is a file the daemon may read back."* All
  fifteen were **classified before a line was changed**: **six are real
  violations** (the doctor's four `memory.json` repairs, the rollback
  catalog, the installed plugin binary), **one is a stream** the platform
  cannot write yet, and **eight are outputs**. The eight outputs are not in
  this slice — deciding between *moving* them and *exempting* them by name is
  the next slice's measurement, not this one's guess, and only `plugin new`
  (a **generator writing its own output**) matches the `tools/...` exemption's
  reasoning verbatim. **The doctor** now writes the memory store through the
  **store framework** (`filestore.Save` → the platform's atomic writer), so
  the CLI's bytes are now **byte-identical** to the daemon's (it wrote
  `"{}\n"`, the daemon writes `{}` — two writers, one file, two formats, and
  the daemon rewrites the file on its next mutation), and the store directory
  is created with the framework's `0700` instead of the doctor's `0755`. The
  BOM repair now **re-serializes** the records it read, which is a real
  behaviour change with a real risk — a repair that empties the store is a
  data loss wearing a repair's clothes — so a permanent test holds the round
  trip. **The rollback catalog** now has **one writer**: `cmd/agt` carried its
  own copy of the format *and* its own writer, one with the **fixed-name
  temp** (`path + ".tmp"`) that two concurrent writers race on, and the
  platform has had the right answer in `kernel/platform/rollbackstore` all
  along — the web UI was already using it. The CLI now **aliases** the
  platform's types and calls the platform's writer, and its second, duplicated
  atomic writer is gone (the platform's is now exported as
  `rollbackstore.WriteSnapshotFile`, without the CLI's missing Windows
  fallback). **The plugin binary** the daemon *executes* now goes through the
  atomic writer too: `os.WriteFile` truncates the target in place and **does
  not change the mode of an existing file**, so a re-install over a file
  whose mode had drifted left the binary non-executable. The forbidden-call
  ratchet goes **15 → 9**. **10 permanent tests** and **11 mutations, 11
  caught** in two independent passes with **byte-identical** logs, and the
  proofs are **two different instruments** because the slice changed two
  kinds of thing: the **writer** (which the gate sees — six mutations) and
  the **bytes** (which only a test can see — five). Two findings from the
  calibration are worth naming: **M08 was first caught by the gate alone**,
  because the stale-temp test used a **read-only file** as the stale scratch
  name and on Windows that does not block a writer — `CREATE_ALWAYS`
  *replaces* the file and drops its read-only attribute — so the test was
  passing against the broken writer, and **a test that cannot fail is not a
  test**; the stale name is now a **directory**, which blocks a fixed-name
  writer on both platforms. And the harness itself had a lesson: it derived
  the test's package from the *mutated file*, so a mutation in the platform
  was judged by a test in the CLI — the package now comes from the mutation,
  not from the file it edits. **What remains in the ratchet**: the **1
  streaming** `os.Create` in `backup.go` (needs `atomicfile.WriteStream` first
  — the platform has no streaming API), the **1 probe** in
  `doctor_ops_env.go` (transient; `os.CreateTemp` is its right shape), and
  the **7 exports**. The **import** ratchet stands at **2**
  (`controlplane → runtime` is a redesign, `selfrepair → runtime` needs a runs
  port that does not exist yet). Evidence:
  `architecture/213-w54h-cli-store-writes-evidence.md`

- W5.4i (the CLI's exports and scaffolds go through the platform's atomic
  writer) takes the last seven "output" write sites. The measurement
  **changed the plan**: the gate's exemption is a **package** allow list, and
  `cmd/agt` is one package — so "exempt the generator" (the one site whose
  reasoning matches the `tools/...` exemption verbatim: `plugin new` writes
  the scaffold, and nothing in the platform depends on those bytes) would
  have meant **"exempt all of `cmd/agt`"** and put the gate back to the
  blind zero W5.4g exposed. **There is no per-site exemption.** The honest
  route is to move the write, or to change the gate — and a per-site
  `//archcheck:allow raw-write <reason>` marker is a separate, small
  archcheck slice. So the scaffold moved too: not because it needed it, but
  because **the exemption unit is the package**, and a package-level
  exemption would have hidden the other six with it. The seven sites: the
  exported skill bundle, the registry index, the single-skill bundle, the
  exported artifact, the journal bundle, the fetched peer artifact and the
  scaffold — of which **two are silent**: nothing verifies an exported
  artifact or a fetched peer artifact, so a truncated file looked fine and
  was wrong. The behaviour changes are named in the evidence: **the mode is
  re-asserted on every write** (`os.WriteFile` applies its `perm` only at
  CREATION, so an export written world-readable once stayed that way — and
  `agt skill export --agent` can lift an agent's **private** skills, so
  `0600` is now a fact rather than a hope), the target is **replaced**
  rather than rewritten, a transient scratch file appears in the target's
  directory during the write, and a symlink at the target path is replaced
  rather than written through. The forbidden-call ratchet goes **9 → 2**.
  **3 permanent tests** and **9 mutations, 9 caught** in two independent
  passes with **byte-identical** logs. The instrument for the seven is a
  **file-grained** test (`TestTheCLIRawWritesAreOnlyTheTwoKnownSites`): the
  GATE counts a *package*, the test counts *files* — so a package that stays
  under the count cannot hide a new raw write in a file the count no longer
  names — and the table says which two sites are left and why. Three
  findings from the calibration are worth naming, all of them the
  **mutation's** fault: a reverted write **breaks the build** (the
  `atomicfile` import becomes unused, the `os` import goes missing), and a
  compile error is **not a caught mutation** — it is a mutation that never
  ran, so the harness now carries the import fix-ups a reverted write needs
  and checks that a fix-up does not remove an import the file still uses; a
  **backtick escape inside a nested quote** (the import anchor was built
  with ``"`t`"io`"`n…"``) matched **zero** times, and a missed anchor reads
  as "the mutation does not compile", which sends you looking in the wrong
  place — the anchors are now built from `[char]9` and `[char]10`; and
  **cancelling a harness mid-run leaves the tree mutated**, because the
  `finally` block that restores the files does not run when the process is
  killed, and the next run then measures an already-broken tree (two runs
  were lost this way before it was noticed). **What remains in the
  ratchet**: the **1 streaming** `os.Create` in `backup.go` (the platform
  has no streaming atomic write — `WriteStream(path, mode,
  func(io.Writer) error)` — and its Windows last-resort must **copy the temp
  onto the target** instead of re-writing from memory, because a stream's
  bytes are not in memory; and `backup.go`'s `os.Remove(outPath)` cleanup
  must **go**, because under an atomic API a failed write leaves the
  previous, good archive intact) and the **1 probe** in
  `doctor_ops_env.go` (transient; `os.CreateTemp` is its right shape — but
  the rule counts only `os.WriteFile` and `os.Create`, so `os.CreateTemp` is
  **invisible to the gate** and the count drops without a new guarantee
  being enforced; the honest fix is a follow-up slice that **extends the
  rule** to `os.CreateTemp` and `os.OpenFile(O_CREATE)`, so that a zero
  means "no raw writes" rather than "no raw writes the counter can see").
  The **import** ratchet stands at **2** (`controlplane → runtime` is a
  redesign, `selfrepair → runtime` needs a runs port that does not exist
  yet). Evidence: `architecture/214-w54i-atomic-exports-evidence.md`

- W5.4j (the platform gains a streaming atomic writer, and the backup uses
  it) takes the last two sites but one in the CLI's raw-write ratchet, and
  the **platform addition** one of them needs. `internal/atomicfile` could
  write a file whose bytes were **already in memory**; the backup archive is
  a **tar+gzip stream** — produced as it is written, and potentially large —
  so it could not use that API at all and used `os.Create`, which truncates
  the target in place: a crash mid-stream left a **torn archive**, and
  `agt restore` reading it would write **half a home** into the destination
  before failing on the truncated gzip stream. The new API is
  `WriteStream(path, mode, fn func(w io.Writer) error)` with the same
  guarantees as `WriteFile` (unique temp, fsync, chmod, rename) and two
  decisions worth naming: the **callback shape makes the commit atomic by
  construction** (no "forgot to close" state; the error path cannot leak a
  temp file; `fn` gets an `io.Writer`, not the `*os.File`, so it cannot
  close or sync a handle the platform owns), and the **Windows last resort
  is different, and deliberately so** — `WriteFile`'s last resort re-writes
  the bytes because they are **in memory**, while a stream's bytes are not
  but they are **on disk**, one rename away in the temp file, so the
  fallback **copies the temp onto the target** instead. The API lives in
  `internal/atomicfile` (L0), not in `filestore` (L2): it is the *same
  mechanism* with the *same* guarantees, and a second atomic writer in
  another package would recreate the divergence this package exists to kill.
  **The `os.Remove(outPath)` cleanup is gone — and that is the point, not a
  detail**: under a plain `os.Create` the target had already been truncated,
  so removing it was the only way to avoid leaving a half-written archive;
  under an atomic write the target still holds the operator's **previous
  good archive**, and removing it would destroy exactly what they need. The
  cleanup was not merely redundant; it became actively **harmful**, and the
  atomic writer is what made it so. The mode also changed (`os.Create` ≈
  `0644` → `0600`): the archive is secret-free by construction (the include
  subtrees are `journal` and `catalog`, the secrets live outside them), but
  a backup is the operator's whole activity history. **7 permanent tests**
  and **5 mutations, 5 caught** in two independent passes with
  **byte-identical** logs. The backup test's failure injection is a
  **symlink to a path that does not exist** — the archive walk Lstats it,
  sees it is not a directory, and fails to READ it, mid-stream; a permission
  trick would not work, because the mode bits do not exist on Windows. The
  calibration reported **one SKIP** and it was the mutation's fault: the
  anchor for M02 matched **two** places in `backup.go` (the `resolveHome`
  error block and the write's error block are textually identical), so the
  harness refused to guess. The forbidden-call ratchet goes **2 → 1**. What
  remains is **one** site — the doctor's writability probe, where
  `os.CreateTemp` is the right shape — but the honest note matters more
  than the number: **the rule counts only `os.WriteFile` and `os.Create`,
  so `os.CreateTemp` is invisible to the gate**, and the count would drop
  without a new guarantee being enforced; a zero would then mean "no raw
  writes **the counter can see**" rather than "no raw writes", and the fix
  is a follow-up slice that **extends the rule** to `os.CreateTemp` and
  `os.OpenFile(O_CREATE)` — which will surface an honest count elsewhere in
  the tree. The **import** ratchet stands at **2**. Evidence:
  `architecture/215-w54j-streaming-atomic-write-evidence.md`

- W5.4k (the rule learns to see `os.CreateTemp`, and the forbidden-call ratchet
  is zero) takes the last site in the CLI's raw-write ratchet and — the part
  that matters — the **rule itself**. W5.4j left the ratchet at one site and
  named the honest reason in the record: the rule counted only `os.WriteFile`
  and `os.Create`, so **`os.CreateTemp` was invisible to the gate**. That is
  not a formatting note. A package could move its write to a temp file, the
  number would fall, and **no new guarantee would exist anywhere** — the count
  was reporting less than it claimed to report. `os.CreateTemp` is in the set
  because a package that creates its **own** temp file is hand-rolling the
  atomic writer (temp → fsync → chmod → rename), and that mechanism belongs
  in exactly one place: `internal/atomicfile` (L0, exempt as the foundation
  it is). The extension does not create debt, it **reveals** it — the
  measurement found three `os.CreateTemp` sites: two in the writer itself and
  **one real violation, a second implementation of the whole mechanism in
  `plugins/tools/file`**. Two writes moved. The **doctor's writability probe**
  (`cmd/agt/doctor_ops_env.go`) was the CLI's last raw write; it now goes
  through the canonical writer, which is the point — the probe's whole purpose
  is to be a **faithful sample** of what a write to that directory does, so it
  must use the same writer a real file would. The **file tool's own copy of
  the atomic writer** is gone: it kept a second implementation for one stated
  reason ("so the `writeAll` seam can inject mid-write failures in tests"), and
  the seam did not need a second implementation to survive, because
  `atomicfile.WriteStream` takes a **callback** (`func(w io.Writer) error`) —
  exactly the shape a mid-write injection needs. The plugin's write is now a
  stream, the seam survives, the guarantee is the canonical one, and the
  plugin gained the Windows fallback the canonical writer already had. The
  rule's **exemption unit stays the package**: the file plugin could not be
  exempted as "a generator that writes its own output" — it writes the
  **agent's** files, and the exemption would have hidden every other raw
  write in the package with it. **5 permanent tests** (2 rewritten) and **6
  mutations, 6 caught** in two independent passes with **byte-identical** logs
  (SHA-256 `79F8E61D…32A256`). The forbidden-call ratchet goes **1 → 0**:
  `tools/archcheck/calls-allowlist.txt` now holds nothing but its header, and
  **one of the two numbers W5.4 asks for is met**. What remains is a
  **different kind of remainder**: the rule still cannot see **`os.OpenFile`
  with a write flag**, because the mode is an *argument* and a selector-based
  rule would count the read-only opens too — **12** non-test sites carry a
  write flag, plus **four** whose flag is a **variable** and cannot be
  classified by looking at the call at all. Counting them needs a
  **flag-aware** counter, and it is deliberately **not smuggled in here**: a
  rule that guesses is worse than a rule that does not exist, because a guess
  that silently under-counts is exactly the blind spot this slice just closed.
  The **import** ratchet stands at **2**. Evidence:
  `architecture/216-w54k-raw-write-createtemp-evidence.md`

- W5.4l (the adapter stops naming the runtime's run decoration) narrows the
  last unblocked import edge. The control plane named **11 distinct runtime
  symbols**; **seven** of them were the run's own **decoration** — the
  profile, the wake provenance, the cost ceiling, the model chain, the tool
  allowlist — and the adapter was building the runs itself, in the module's
  language. The **rows** (the wake provenance, the tool allowlist) moved to
  the **contract** layer, where such values already live (`opapi`,
  `memoryapi`, `toolapi` each carry context values today), and the runtime
  keeps its names as aliases — one row, one spelling. The **application**
  owns the decoration now (`kernel/app/runs.Decorate`,
  `DecorateModels`, `DecorateTools`), and the host asks the application for
  it. `workboard_dispatch.go` and `workflow.go` no longer import the runtime
  at all; the adapter names **4** runtime symbols instead of **11**, and two
  files left the edge. **13 permanent tests**, **6 mutations, 6 caught** in
  two independent passes with **byte-identical** logs. The edge is
  **narrower, not closed**: the adapter still holds the kernel handle in 16
  files, and closing it needs a **port** — one that cannot live in the
  contract layer (most of the kernel's verbs return L3 types) and therefore
  has to live in the application layer, with the adapter's native verbs
  moving down into it. The two rows the adapter still names
  (`CouncilMember`, `CouncilResult`, `ReaperReport`) are the modules' api
  rows, and the adapter may not import a module's api package either — they
  have to reach the contract layer first, the same move this slice made for
  the wake. Evidence:
  `architecture/217-w54l-adapter-run-decoration-evidence.md`

- W5.4m (the adapter names the runtime only as a handle) leaves the control
  plane with **one** runtime symbol: the kernel handle. It had four — the
  handle plus **three module rows** (`CouncilMember`, `CouncilResult`,
  `ReaperReport`). The measurement carried the surprise that made it cheap:
  the **reasoning** module's api rows were **already contract rows**
  (`type CouncilMember = reasoningapi.CouncilMember`, and `reasoningapi`
  lives in `kernel/contract/`), so the adapter was reaching for the
  runtime's *alias* of a contract row — naming the contract row directly is
  a spelling change, not a conversion. The **reaper's** rows were the
  exception, but they cross the boundary too (the module produces them, the
  application layer's operator scan reads them, the adapter hands them on),
  so their home is the contract layer, exactly like the wake provenance and
  the tool allowlist were in W5.4l. The module's api package re-exports them
  as aliases — **one row, one spelling** — and the adapter names the
  contract's rows. **Two of its files stopped importing the runtime at all**
  (18 → 15). **3 permanent tests** (2 moved with the rows) and **6
  mutations, 6 caught** in two independent passes with **byte-identical**
  logs (SHA-256 `07ECFBA0…D19C834BE`); the calibration run found three
  faulty mutations — an anchor matching three rows instead of one, an
  anchor matching nothing, and a mutation that broke the build — all three
  rewritten, because a mutation's fault is not a test's fault. The guard
  test reads each file's own import to learn the runtime's name in it, so
  "the handle and nothing else" is precise rather than approximate. The
  edge is **narrower, not closed**: the handle itself is left, and closing
  it needs a **port** that cannot live in the contract layer (most of the
  kernel's verbs return L3 types), so it has to live in the application
  layer with the adapter's native verbs moving down into it. What this
  slice did is make that work **mechanical** — the adapter no longer knows a
  single module row. Evidence:
  `architecture/218-w54m-adapter-names-the-runtime-only-as-a-handle-evidence.md`

- W5.4n (the application fills the council's own ports) is the **first**
  family of the port programme the family map described. The adapter's kernel
  use is almost entirely **port filling** — it passes kernel methods into the
  application's port structs — and wiring is the application's business: it
  knows which capabilities the council needs and how each of them answers. The
  council's ports moved into `kernel/app/council` (`PortsFromKernel`,
  `EnginesFromKernel`), the shape `app/catalog`, `app/providers` and
  `app/runs` already use. **Six** kernel verbs left the adapter, and the
  host now says one sentence about the council: *here is the handle*. A
  verb-grained guard holds the wiring where it belongs — the gate counts
  packages and cannot see it, and **no behavioural test could**: the wiring
  would work exactly as well in the wrong layer. The ratchet does not move
  yet, but the pattern is proven end to end.
  Evidence:
  `architecture/219-w54n-the-application-fills-the-councils-ports-evidence.md`

- W5.4o (the roster's own ports, and a guard that reads the kernel) is the
  **second** family of the port programme. The roster was the busiest the
  adapter had: **eight** kernel verbs named in **28** call sites across
  **seven** files, all of it port filling, so the roster's ports moved into the
  application (`kernel/app/roster`: `ProfilePortsFromKernel`) and the adapter
  keeps one binding. **No roster verb is named in the adapter's production
  code** any more, and the rows are contract rows (`agentapi.Profile`), so
  nothing was converted. Two holes in W5.4n's guard closed with it: the
  matcher now sees a verb that leaves the adapter as a **method value** (the
  old one saw calls only), and the roster's verbs are **derived from the
  kernel's own accessor file** instead of being hand-written — a verb added to
  the kernel is guarded the day it lands, and a verb the kernel does not have
  cannot sit in the list pretending to guard something. **6 permanent tests**
  (5 new, 1 extended) and **13 mutations, 13 caught** in two independent
  passes with **byte-identical** logs (SHA-256 `0EE5DAE0…C808E`). The
  calibration forced two rewrites, and both are worth keeping: a mutation that
  **survived** because the check meant to catch it called the very helper it
  truncated (the expectation must not be derived by the code path it checks),
  and a **no-op mutation** that changed nothing (a mutation that changes
  nothing is not a mutation). The ratchet does not move yet — the control plane
  still imports the runtime in **15** files — but the guard reads the kernel
  now, so the next family fills in the same rule instead of writing a new one.
  Evidence:
  `architecture/220-w54o-the-rosters-own-ports-evidence.md`

- W5.4p (the standing family's ports, and the nil-port discipline that moved
  with them) is the **third** family of the port programme, and the first one
  that is a set of **stores** rather than plain verbs: the daemon opens the
  standing library and hands the kernel a contract port (W3.50), and a daemon
  with no library holds a **nil** port. The adapter carried that discipline
  itself, spelled **twice** — and a third surface that forgot it would have
  called into nothing. The standing family's ports moved into the application
  (`kernel/app/standing`: `PortsFromKernel`), the application now spells the
  nil-check **once**, and **four** kernel verb sites left the adapter's
  production code. The adapter no longer hands `s.k` to an application
  constructor at all: it hands the application a **narrow** view (four
  methods). The guard needed its **own** rule and could not use the roster's —
  the standing accessor file also holds the data accessors, so a file rule
  would sweep in verbs that are not the standing family's. The standing rule is
  **the name**, and the test that holds it reads the kernel's own method set
  **independently** of the helper the guard uses, for the reason W5.4o's
  calibration forced. The guard now holds **21** application-owned verbs.
  **3 permanent tests** and **12 mutations, 12 caught** in two independent
  passes with **byte-identical** logs (SHA-256 `306EA35F…B9E23`). M05 and M06
  break the pause port in **opposite** directions — do nothing, do the
  opposite — and only the flag's direction in the assertion separates them.
  The ratchet does not move yet — the control plane still imports the runtime
  in **15** files — and the next family is the **schedule** library (W3.49):
  the same shape, with the pattern now proven for both halves.
  Evidence:
  `architecture/221-w54p-the-standing-familys-ports-evidence.md`

- W5.4q (the schedule family's ports, and the fifth nil-check that went with
  them) is the **fourth** family of the port programme and the second one that
  is a **library** rather than a set of plain verbs. The schedule library
  (W3.49) has the same shape as the standing one, and the adapter was carrying
  the same nil-port discipline **five** times — in three files — plus three
  places that handed the raw store to an application constructor. The
  schedule's ports moved into the application (`kernel/app/schedule`:
  `PortsFromKernel`), the application spells the nil-check **once**, and the
  schedule's **per-request** (routed tenant) kernel is bound too, so the
  creation service is built from the kernel the request arrived on. The guard
  needed its **third** rule, and the standing rule generalised into it: the
  helper now takes the family's name. The guard holds **26** application-owned
  verbs. **4 permanent tests** and **12 mutations, 12 caught** in two
  independent passes with **byte-identical** logs (SHA-256 `B7E37BBA…A9396`).
  M06 is worth reading: a removal port that says "removed" and removes nothing
  is a **lie**, and only an assertion that looks at the library afterwards
  catches it. The ratchet does not move yet — the control plane still imports
  the runtime in **15** files — and the next family is the **workflow**
  library, the first one that hands the application the **handle itself**:
  the port the application holds stops being "the kernel's verbs" and becomes
  "the kernel, narrowed to what this application needs".
  Evidence:
  `architecture/222-w54q-the-schedule-familys-ports-evidence.md`

- W5.4r (the workflow family's ports, and the handle that finally reaches the
  application) is the **fifth** family of the port programme, and the first one
  where the **handle itself** reaches the application. The four families
  before it had a shape the application could hold without the kernel; the
  workflow services need to save, restore, enable, remove, draft, refine, run
  and test workflows, and the kernel is what does those — so the port is the
  handle itself, seen through the interfaces the application already declares.
  The workflow family's ports moved into the application (`kernel/app/workflow`:
  `PortsFromKernel`), and **thirteen** kernel verb sites left the adapter's
  production code: the adapter no longer hands the handle to an application
  constructor at all. Two things stayed in the host on purpose — the execution
  host's `WithWake` and `Panic` are the **adapter's** business — and the port's
  `Run` and `TestNode` answer in the **application's** own rows, so the
  kernel's result types stop at the application's edge. The guard needed its
  **fourth** rule, and it matters most here because this is the family where
  the handle is in play; the guard now holds **38** application-owned verbs.
  **4 permanent tests** and **9 mutations, 9 caught** in two independent
  passes with **byte-identical** logs (SHA-256 `E4D5444F…663C9`). M04 and M05
  are worth reading together: they are the two ports that are the handle,
  narrowed, and a port that is **nil** is not a narrow view — it is no view at
  all. Five families are done, and the next ones (tools and MCP, workboard and
  OKR) are the *verbs without libraries* — the same shape as the council's,
  which is the template.
  Evidence:
  `architecture/223-w54r-the-workflow-familys-ports-evidence.md`

- W5.4s (the tools family's ports, and the registry the other families ask) is
  the **sixth** family of the port programme, and the first one the family map
  calls **verbs without libraries**: there is no store the daemon opens and
  hands over, there is a **registry** the kernel holds, a script-tool forge
  and a set of MCP servers beside it. The tools family was the busiest reader
  in the adapter — the registry was named **five** times and the **whole
  handle** was handed to the tools application three times, because the
  application's own ports were shaped like the kernel's method set. The ports
  moved into the application (`kernel/app/tools`: `PortsFromKernel`), and
  **nine** kernel verb sites left the adapter's production code: the adapter
  no longer hands the handle to a tools-application constructor at all. The
  port has the **application's** name, not the kernel's (`Registered()`, not
  `Tools()`), and a port's field names must not collide with kernel verbs
  either — the first draft called the MCP attachment port `MCPAttached`, and
  the guard caught the adapter the moment it did. The registry is read by four
  other families and is now asked **once**: the roster's permission picture,
  the config's tool count, the schedule's tool targets and the nodes'
  remote-run check all ask the tools family. The **routed** kernel stays the
  routed kernel — a tenant's tools are the tenant's tools — while the
  daemon's own ports answer for the root. The guard needed its **fifth** rule
  (**29** verbs: every kernel method whose name mentions a tool or an MCP
  server, which also sweeps the two threshold verbs the memory and the skill
  families own — a verb guarded twice is still guarded) and a **sixth**, because
  the verb rule cannot see a **handle**: `apptools.NewForgeLifecycle(k)` names
  no verb at all, so the tools application takes **ports**, never the handle
  they were filled from. The guard now holds **67** application-owned verbs.
  **11 permanent tests** and **23 mutations, 23 caught** in two independent
  passes with **byte-identical** logs (SHA-256 `E6F662F1…35887`). M14, M17 and M18
  are worth reading together: none of them is a behavioural regression — the
  wiring would work **exactly as well**, it would simply be in the wrong
  layer — so only the two rules can see them. Six families are done, and the
  next one (the workboard and OKR surface) is the last of the *verbs without
  libraries*.
  Evidence:
  `architecture/224-w54s-the-tools-familys-ports-evidence.md`

- W5.4t (the objective family's ports, and the rule that reads a file) is the
  **seventh** family of the port programme: the **objectives**. The adapter's
  objective surface was small — **two** sites — but it was the shape the
  programme is about: `appokr.New(k.OKR(), k)` and
  `appokr.NewLifecycle(k, appokr.New(k.OKR(), k))`, the handle as the rollup
  and as the mutation host. The ports moved into the application
  (`kernel/app/okr`: `PortsFromKernel`) and the adapter keeps the binding, on
  the **routed** kernel — a tenant's objectives are the tenant's objectives.
  **Two** kernel verb sites left the adapter's production code, and the
  adapter no longer hands the handle to the objective application at all. The
  family is found by reading the kernel's own objective file
  (`kernel/runtime/okr.go`), the way the roster family is found by reading its
  accessor file: a **name** rule would have missed the rollup verb, which is
  called `Rollup` and says nothing about objectives, and a verb the rule misses
  is a verb the adapter may name. The rule reads the **exported** methods of
  that file only — the file also holds two private helpers, and a private
  method is not a verb the adapter can name. The guard now holds **75**
  application-owned verbs. The port's field names must not collide with kernel
  verbs: the first draft called the rollup port `Rollup`, because the app's
  own interface is `Rollupper`, and the guard caught the adapter the moment
  it did (the field is `Progress` now) — the **second** time this programme
  learned the same lesson, which is why it is written down. The **handle** rule
  now covers the whole programme: it reads the programme's membership from the
  tree, so every application that declares its own `ports.go` is in scope and
  the list cannot rot. **3 permanent tests** and **10 mutations, 10 caught** in
  two independent passes with **byte-identical** logs (SHA-256 `D528C9DD…86B70`).
  M09 is worth reading twice: the derivation narrows, and the guard's own test
  catches it — because that test reads the kernel's file **independently** of
  the helper the guard uses. A derivation that checks itself is not a
  derivation. Seven families are done, and the **workboard** is next: the
  largest of them, and the last of the family map's *verbs without libraries*
  that the adapter still names.
  Evidence:
  `architecture/225-w54t-the-objective-familys-ports-evidence.md`

- W5.4u (the workboard family's ports, and the last of the board's handle) is
  the **eighth** family of the port programme and the **largest** one: the task
  board, with **seventeen** kernel verbs, **six** services and **three** adapter
  sites. The adapter's workboard surface was not a list of verb calls — it was
  three places where the **whole handle** was handed to the application. The
  ports moved into the application (`kernel/app/workboard`:
  `PortsFromKernel`) and the adapter keeps the binding, on the **routed**
  kernel: a tenant's board is the tenant's board. **Seven** sites left the
  adapter's production code, and the adapter no longer hands the handle to the
  workboard application at all. The dispatcher's **agent** closure and the
  execution's **run** and **publish** closures stay in the adapter: they are
  the host's protocol surface (who the seat is, how the run is decorated, what
  is published) and not the application's business. The family is found by
  reading the kernel's own workboard files with a **glob**, because the verbs
  live in **two** of them and the proof verb is called `ProveTask`, which says
  nothing about the board — a **name** rule would miss it, and a verb the rule
  misses is a verb the adapter may name. The guard now holds **92**
  application-owned verbs. The port's field names must not collide with kernel
  verbs — the **third** time this programme learned it: the journal port is
  `Events` and the seat catalog port is `SeatStore`. The **seat** port
  deserves a word: the workboard family asks "is this seat real?" and the seat
  family owns the answer — the same shape as the registry being the tools
  family's answer for four other families. **3 permanent tests** and **19
  mutations, 19 caught** in two independent passes with **byte-identical**
  logs (SHA-256 `2802D351…05A6C`). M15, M16 and M17 are worth reading together: none
  of them is a behavioural regression — the wiring would work **exactly as
  well**, it would simply be in the wrong layer — so only the two rules can
  see them. M19 is worth reading twice: the rule's glob loses the proof file,
  and the guard's own test catches it, because that test finds the files
  **itself** rather than through the helper the guard uses. A derivation that
  checks itself is not a derivation. With this slice the family map's **verbs
  without libraries** are done — the adapter no longer names a workboard, an
  objective, a tool or an MCP server. What is left is the **handle** itself:
  the families that hold it (the seat catalog, the journal, the sandbox, the
  providers) are the ones the roadmap calls *libraries* — they own a store,
  and the store is the host's file.
  Evidence:
  `architecture/226-w54u-the-workboard-familys-ports-evidence.md`

- W5.4v (the binding does not name the runtime, and the number is finally
  checked) is the port programme's **ninth** slice and the first one about a
  **word** rather than a call. W5.4s, W5.4t and W5.4u each wrote a binding
  helper whose only purpose was to spell the kernel's type
  (`func toolsPortsFor(k *runtime.Kernel) apptools.Ports { ... }`), so the
  control plane's runtime-naming count went from **fifteen** to **eighteen**
  while evidence 224, 225 and 226 kept saying — correctly, when they were
  written — that *the control plane still imports the runtime in **15**
  files*. **Three merged slices carried a false claim and nothing failed**:
  the number was prose, and the archcheck ratchet holds the
  `kernel/controlplane -> kernel/runtime` edge at PACKAGE granularity, so a
  new file inside the same package is invisible to it. Go does not need the
  spelling — a value of the kernel's type can be passed to a function that
  expects it without the caller ever naming the package — so the three
  bindings now call `PortsFromKernel` directly, the **roster** family's shape,
  and the count is back to **15**. Two rules hold it: the **binding rule** (a
  `kernel/controlplane/*_ports.go` file does not import the runtime at all,
  read from the tree so a family's binding is covered the day it is written)
  and the **ratchet** (`controlPlaneRuntimeNamingFiles = 15` is the recorded
  measurement, and the comparison runs in BOTH directions: a count that grows
  means the handle walked back in, a count that shrinks means the constant and
  the documents that state it change in the same slice). This is the **first
  machine-checked statement** of that number. No behaviour changed: the
  daemon's bindings still answer for the root kernel and the request's for the
  routed one. **5 permanent tests** and **13 mutations, 13 caught** in two
  independent passes with **byte-identical** logs (SHA-256 `59FBC268…7EB90`).
  M01, M02 and M03 put the runtime import back into each of the three
  bindings and the two rules catch every one of them — none of them is a
  behavioural regression, because the wiring would work **exactly as well**,
  it would simply be in the wrong layer. M07 is worth reading twice: the
  binding rule's filter widens from `_ports.go` to `_go`, the rule starts
  flagging the whole adapter, and the rule's own test catches it — the rule is
  asked whether it is **sharp**, not whether it is **broad**. M08 and M09 hold
  the ratchet, and M10 and M11 hold the **measurement** (the count jumps to
  166 when the test files are let in, and drops to zero when it is narrowed to
  the binding files). The ratchet does not move yet: the control plane still
  names the runtime in **15** files, and those fifteen are the ones the
  programme's last step is about — the families that own a **store** (the seat
  catalog, the journal, the sandbox, the providers) are the ones the roadmap
  calls *libraries*, and the store is the host's file. The binding rule does
  not touch them: a file that **holds** the handle may name it; what the rule
  says is narrower — a file that merely **hands the handle over** must not
  name it.
  Evidence:
  `architecture/227-w54v-the-binding-does-not-name-the-runtime-evidence.md`

- W5.4w (the config center family's ports, and the first step of the handle) is
  the port programme's **tenth** slice and the **first** one of its last step:
  the handle itself. W5.4n measured the whole remaining programme — the
  adapter's kernel use is almost entirely port filling — and said the work was
  "one family at a time, until the last family is gone and the handle goes
  with it". Eight families are gone (council, roster, standing, schedule,
  workflow, tools, objectives, workboard); this slice takes the **config
  center**, the first of the families that hold a **store**, and the first
  slice in which the adapter stops reaching for a kernel verb and **asks a
  family** instead. The adapter reached for the kernel's config center in
  **eleven** places across **five** files: three were **port filling** (the
  center's own reads and writes) and two were **families asking the kernel**
  for a capability that is not theirs (the roster's teardown, the roster's
  permission picture) — because the config center family had no ports to ask.
  The ports moved into the application (`kernel/app/configcenter`:
  `PortsFromKernel`) and the adapter keeps the binding
  (`kernel/controlplane/configcenter_ports.go`), which does not name the
  runtime. The **nil-port discipline** moved with it: the adapter spelled "a
  daemon with no config center" **five** times across two files — five
  spellings that could disagree with each other — and the application spells
  it **once**, in `PortsFromCenter`. The rule is **by name** — a kernel verb
  that mentions the config center is the config center family's business — and
  the guard now holds **93** application-owned verbs. The adapter's
  handle-verb debt: **115 → 104** call sites, **31 → 30** distinct verbs.
  **4 permanent tests** and **14 mutations, 14 caught** in two independent
  passes with **byte-identical** logs (SHA-256 `19EFE4F1…E445E`).
  **M12 survived the first calibration** and it is worth reading twice: it
  breaks the teardown's config **delete** — the agent's own entries stop being
  deleted — and **nothing caught it**, because the teardown's config path had
  **no test at all**: the cascade test proved the entries were *counted* and
  the permissions test proved the picture was *empty*, but no test ever
  watched an entry disappear. The slice wrote that test, and the mutation is
  caught now — the second time this programme a mutation has found a
  **missing test** rather than a missing rule, and it is why the calibration
  runs before the two clean passes. The handle stays: the adapter still calls
  **104** kernel verbs on it — the journal (**27**), the bus (**19**), the
  forge (**7**), the providers (**7**), the base directory (**6**), the memory
  (**5**), the policy engine (**4**) — and those families own a store, and the
  store is the host's file.
  Evidence:
  `architecture/228-w54w-the-config-center-familys-ports-evidence.md`

- W5.4x (the journal family's ports, and the seventeen families that read it)
  is the port programme's **eleventh** slice and the **second** one of its last
  phase: the handle itself. The journal is the one capability **every** family
  reads, and the adapter reached for the kernel's journal in **twenty-eight**
  places across **eight** files — more sites than any other verb in the
  control plane. Twelve of them were **routed** (a tenant's journal is the
  tenant's journal) and the rest read the daemon's own; the slice keeps that
  split exactly. The ports moved into the application
  (`kernel/app/journal`: `PortsFromKernel`) and the adapter keeps the binding
  (`kernel/controlplane/journal_ports.go`), which does not name the runtime.
  The port carries the **journal itself**, and that is the point of this
  family: every consumer already declares its **own** interface over the part
  of the journal it uses — the roster's range, the channel inbox's append, the
  schedule's firing, the pulse stream's tail — and those narrow views are the
  consumers' business, not the family's. The field is named `Events`, not
  `Journal`: the **fourth** time this programme learned that a port field must
  not answer to a kernel verb's name; the collision this time was the
  **workflow** application's own port field, so it was renamed and the adapter
  reads `ports.Events`. The journal family's trace service was handed the
  **whole handle**, and the **handle rule** caught it the moment the family
  declared its own ports — the fix was free, because the kernel's three
  provenance verbs are **pure delegations to the very same journal**, so the
  trace service reads the routed kernel's journal, which is what the handle
  answered with: the same answer, one layer down. The guard now holds **94**
  application-owned verbs. The adapter's handle-verb debt: **104 → 76** call
  sites, **30 → 29** distinct verbs. **3 permanent tests** and **13 mutations,
  13 caught** in two independent passes with **byte-identical** logs (SHA-256
  `F58AEDF4…D420D3`). M11, M12 and M13 put the kernel back — the routed
  consumer reaches for the kernel's journal again, the daemon's consumer does
  the same, and the binding file names the runtime again — and the **three
  rules** catch every one of them; none is a behavioural regression, because
  the wiring would work **exactly as well**, it would simply be in the wrong
  layer. M08 and M09 are the ones that bite: the runs fold and the tenant's
  activity read the **daemon's** journal instead of the kernel they were
  handed — the routed kernel's rule, broken where a tenant's runs silently
  become somebody else's.
  Evidence:
  `architecture/229-w54x-the-journal-familys-ports-evidence.md`

**W3.12 — the providerboot half of `modules/providers` (delivered).**
`plugins/providerboot` is now `kernel/modules/providers/boot`, and the module
grew a gate, `kernel/modules/providers/api`. The slice was a **ports rewrite**:
the engine asks the wire adapters (the compat family, the offline demo
provider, the ChatGPT Responses backend) and the middleware stack for what it
needs through `api.Ports`, the composition root binds them, and the **ratchet
went down** — 126/13 → **123/13**, with no new allowlist entry.

The middleware stack moved to the runs module (`agent.MiddlewareFromEnv`), so
the engine reaches no plugin, and the module reaches no other module: the
providers module owns the decisions and asks for the wire. The module's own
model row (`api.ModelInfo`) replaced the adapter's, and the module owns the
picker and ordering rules that used to live in the plugin. The daemon's
behaviour is unchanged: the same selection rules, the same registration order,
the same live down-route eligibility set, the same prices and the same wire.

**How the slice was shaped (the scope note, kept).**
`plugins/providerboot` (14 files) was **not** a library that happened to sit
under `plugins/` — it imported the **concrete** provider packages
(`plugins/providers/compat`, `mock`, `openairesponses`) precisely so the kernel
never grows a kernel→plugins edge; its own doc said so. It was also typed
against them (`chatgptModelSet.Info` was a
`map[string]openairesponses.ModelInfo`) and it reached into `kernel/agent` for
the middleware stack. Moving it into `kernel/modules/providers` (L3) as-is
would have added **four** edges the ratchet refuses to grow: three **upward**
ones (module → plugin) and one cross-module one (providers → runs, for
`kernel/agent`).

So the slice was a **ports rewrite**, the same shape as W3.7's workflow engine
and W3.9's reaper:

- the module asks for a provider through a **port** (the composition root
  supplies `compat.Build`, the demo-echo provider and the ChatGPT adapter),
- the module's own vocabulary replaces the adapter's types (a module-local
  model-info row instead of `openairesponses.ModelInfo`),
- the **middleware stack stays the host's** — the module asks the runs module
  for the stack through a port and wraps the primary and every alternate with
  it, on the boot path and on the reload path alike (the M997 drift fix
  wrapped both; the wrap must not fall between the two),
- the daemon's primary selection, the alternate registration and the live
  down-route eligibility set keep their behaviour exactly.

`kernel/catalog` and `kernel/chatgptauth` are already in the module (W3.11),
so the module's own vocabulary had a home to grow into.

**What is next.**
The roadmap's order continues with `modules/runs` last (what remains of
`kernel/runtime` + `runexec` + the agent loop + resume + delegation +
intervention; `kernel/runtime` is deleted when empty). The agents module's
step 8 is **done**: the reaper and the module's api package (W3.9), the rows
the tool shares with the host (W3.13, W3.14) and the tool's call path
(W3.15).

What is left of the overseer is **not** a layer problem: the tool's operations
are still tool-facing rather than transport-facing typed operations in the
control plane's registry. The port is now where such an operation would sit
(`kernel/contract/overseerapi`), and the implementation that answers it is in
the app layer, so the operation can be exposed without moving anything else —
that is a **surface** question, not a **layer** question.

The plugin-reach edges are the remaining W3 debt, and they come in two
shapes. The **cheap** shape is a tool that reaches into a module for a row
that already lives in the contract — W3.16 was exactly that, and the same
recipe applies to the remaining ones: name the contract row, keep the module's
alias, delete the stale allowlist entry. The **expensive** shape is a tool
that needs the module's **implementation** (the config tool, the schedule
tool, the standing tool, the database tool, the workboard tool, the skill
tool, the forgetool, the MCP tool, the introspector, the guardians, the market
plugin): those need a port in the contract and an implementation in the app
layer — the W3.15 recipe, one surface at a time.

The other plugin-reach edges each need their own slice: the config tool, the
schedule tool, the guardians, the built-in tools and the introspector still
reach into modules they should only name.

The overall target is the architecture in `20-target-architecture.md`, delivered
incrementally to protected `main`, with runnable proof and source-aligned
documentation.

---

> **Owner update, 2026-10-04:** continue directly on the shared `main`, without
> new task branches. PR #612 consolidates the original W0–W2.1a stack plus W2.2a.
> W2.2a, W2.2b, W2.3 and File Manager operation binding are complete;
> Catalog/provider, memory and world native migrations plus exit evidence are complete. Taste native migration and exit evidence are complete. Skill native migration and exit evidence are complete. Board native migration and exit evidence are complete. Workboard, OKR and storage/artifact native migrations plus exit evidence are implemented. Schedule native migration and exit evidence are complete. Standing seven-command native migration and exit evidence are complete. Workflow13-command native migration and exit evidence are complete. Pulse fourteen-command native migration and exit evidence are complete locally; Autonomy feed native migration and exit evidence are complete locally; Three tool read operations and native exit evidence are complete locally; all eight toolforge native operations and exit evidence are complete locally. All three toolbox native operations and exit evidence are complete locally. All six MCP native operations and exit evidence are complete locally. All eight market native operations and exit evidence are complete locally. Plugin inventory typed native migration and exit evidence are complete locally. Config read typed native migration and exit evidence are complete locally. All five settings native operations and exit evidence are complete locally. Configcenter native exit is complete locally. Channel inventory/account and OAuth service/state/provider/exchange ownership foundations are implemented, with registered-field removal/status race repairs. Gateway status/QR service foundations are implemented. Inbox service foundation is implemented. Send service foundation is implemented. All eleven channel operations have typed shared native bindings and exit evidence. Webhook observability typed native migration and exit evidence are complete locally. Tunnel premise is measured: boot supervision exists, with no native operation to migrate. Both update commands now have typed shared native bindings and exit evidence. Roster list/presentation/cache ownership foundation and empty-cache invalidation repair are implemented. Roster status presentation/typed snapshot foundation is implemented; native source collection remains. Continue §4.5 with typed list binding and remaining roster status/source/read-command foundations, then steer/runs/journal/edict/tenant/shutdown/remote and the wider roadmap. Cadence resident run execution remains W2.2/W4 work. Journal raw-ref GC protection remains later module work. W2.10g-i through W2.27m are delivered to main via PR #701 (8416d9c8, all24 exact-head CI jobs passed); W2.27n is delivered via PR #702 (a00b580c, all24 exact-head CI jobs passed); W2.27o is delivered via PR #703 (5703f2d5, all24 exact-head CI jobs passed); W2.27p is delivered via PR #704 (ca25f5c7, all24 exact-head CI jobs passed); W2.27q is delivered via PR #705 (c32e8852, all24 exact-head CI jobs passed); W2.28 is delivered via PR #706 (1f5d5c36, all24 exact-head CI jobs passed); W2.29a is delivered via PR #707 (0f97e7a3, all24 exact-head CI jobs passed); W2.29b is delivered via PR #708 (0d5777c5, all24 exact-head CI jobs passed); W2.30a is delivered via PR #709 (d8856048, all24 exact-head CI jobs passed); W2.30b awaits its own delivery. Broader adapters and remaining domains remain open.
> Read this handoff, verify the current state, then measure the next item's premise
> before changing code. The original handoff contained a stale claim about channels:
> they already used the vision sidecar; the API and channel rejection audit differed.

---

## 1. What this project is and what we are doing

AGEZT is a Go daemon (`cmd/agezt`) plus CLI (`cmd/agt`) plus a React console (`frontend/`, embedded via
`go:embed` in `kernel/webui/dist`). It runs governed AI agents: providers, tools, a policy engine
(Edict), a hash-chained journal, memory, channels, workflows and schedules.

The owner asked for a **clean-architecture redesign**, done as a **module-by-module strangler** on
`main`. There is no rewrite branch, and `main` stays shippable after every PR.

Authoritative documents (read in this order):

| File | What it is |
|---|---|
| `architecture/20-target-architecture.md` | The target: layers L0–L7, three pipelines (Operation `app.Dispatch`, Run `runs.Start`, Tool `tools.Invoke`), modules |
| `architecture/21-migration-roadmap.md` | Waves W0–W5, the PR-by-PR plan, **§1 working rules**, **§5 owner decisions**. Rows marked ✅ are done; each records what was measured and what changed |
| `architecture/00-README.md` | Codemap index; **§9 is the findings register** (open defects; ✅ = fixed) |
| `architecture/01…12-*.md` | Codemap of the current system, per area. Update the relevant one in the same PR that changes the structure it describes |
| `CHANGELOG/unreleased/current.md` | User-facing changelog. Security fixes go under `### Security` |

Layer map in code: `kernel/contract/*` (L1, stdlib-only contracts), `kernel/platform/*` (L2: `filestore`,
`netout`, `sandbox`), then modules, `kernel/app/tools` (L4 invocation entry), adapters, `plugins/` (L6), `cmd/` (L7).
`tools/archcheck/layers.json` places every package; the allowlists only shrink.

---

## 2. Where things stand (2026-10-03)

### 2.1 Consolidated delivery and original stack

**Delivery:** #612 contains the entire original stack and the W2.2a follow-up.
It was merged with history preserved at `415f6cea`; the superseded PRs were closed
and the shared checkout returned to `main`. W2.2b was implemented directly on `main`.
The only commits unique to earlier branch heads are identical cherry-picks of the
`TestMarkInstanceDead` repeatability fix; the top branch includes that fix too.
The table below is historical, not an instruction to recreate a stack.

| PR | Branch | Content |
|---|---|---|
| #595 | `arch/w0-archcheck` | codemap 00–12, target 20, roadmap 21, `tools/archcheck` (W0.1–W0.2) |
| #596 | `fix/w0-correctness` | W0.3–W0.5 security/correctness P0s, dead code |
| #597 | `arch/w1-contracts` | W1.1 contracts move to `kernel/contract/{llm,toolapi,channelapi}` |
| #598 | `arch/w1-repoint` | W1.2 importers use contracts (archcheck 196 → 145) |
| #599 | `arch/w1-store` | W1.3 `platform/filestore`, vault/settings merge-on-save under a cross-process lock |
| #600 | `arch/w1-netout` | W1.4 `platform/netout` (Egress / OperatorClient / MetadataClient) |
| #606 | `arch/w1-sandbox` | W1.5 `platform/sandbox`, no child inherits daemon secrets |
| #607 | `arch/w1-eventlog` | W1.6a event-kind registry closed (`TestKindRegistryIsClosed`) |
| #608 | `arch/w1-journal-recover` | W1.6c corrupt journal record quarantined, not fatal |
| #609 | `arch/w1-approvals` | W1.7 approvals re-ask after restart (pinned by test), not persisted |
| #610 | `arch/w1-config` | W1.8 configcenter: restart persistence, secrets in vault, lock fix, `config.access` journaled |
| #611 | `arch/w2-ingress` | W2.0 a direct `agt run --agent` is bound by the whole profile (`WithAgentProfile`) |
| #612 | `arch/w2-dispatch` | W2.1a dispatch journals every non-`ReadOnly` op (`op.invoked/completed/failed`) |

Every branch from `fix/w0-correctness` on also carries the same commit
`test(channel): TestMarkInstanceDead survives -count=N` (cherry-picked down the stack, see §5.3).

### 2.2 CI state when this was written

- All Go gates pass locally on `arch/w2-dispatch`.
- `secrets (gitleaks)` failed on #612 because of a synthetic key literal in a test. Fixed by rewriting the
  branch (the key is now joined at run time), and gitleaks is clean locally.
- `race-depth (linux, cgo)` failed on #596–#612 for two reasons:
  1. `TestMarkInstanceDead` was not repeatable under `-count=20`. **Fixed** on every branch.
  2. `kernel/controlplane` hit the 10-minute `go test` timeout under `-race -count=20`. On #595 it took
     487 s of 600 s, and locally the package takes the same 37–52 s with or without these PRs. The likely
     cause is runner contention: about 13 stacked PRs run race-depth at once on shared self-hosted runners.
     **Not a code bug as far as measured.** If it still fails, rerun that one job when the runners are quiet
     (`gh run rerun <run-id> --failed`). If it fails alone too, report it to the owner. Do not raise the
     timeout or cut the stress count without asking.

---

## 3. First thing to do in a new session

1. Read the current checkpoint at the top. Inspect `git status --short`, HEAD, index
   and the open `arch/w2-dispatch` PR with its exact-head CI; the older PR #612 is
   historical. Preserve every uncommitted file.
2. On the shared checkout, use `main`; fast-forward only to a verified merge whose
   tree equals the tested head.
   **Do not create or switch to another task branch.** The owner's explicit instruction
   supersedes the original branch-per-PR recipe.
3. Read the authoritative target and roadmap, then start the next open item in §4.
4. Use isolated temporary `AGEZT_HOME` directories for runtime verification; never the
   owner's real state or password-protected `.dev-home` console.
5. Read `MEMORY.md` in the Claude memory directory if available
   (`~/.claude/projects/D--Codebox-PROJECTS-AGEZT/memory/`), especially
   `architecture-redesign-2026-10.md` for the standing invariants.

---

## 4. The next work items, in recommended order

The roadmap lists W2.1 (op framework) before W2.2 (runs). I recommend doing the **defect-driven** slices
first, because each one fixes something real and lands safely. The framework work (4.4) has no measured
defect behind it, and it gets easier once 4.1–4.3 have reduced the number of side paths. W2 items are
independent of each other, so this reordering is within the plan. Say so in the PR body.

**Rule for every item: measure the premise before building.** Twice the roadmap's claim was wrong:
approvals were not lost on restart, and the journal index was not urgent. Once the real defect was the
opposite of the claim (W2.0). Write the measurement into the roadmap row.

### 4.1 ✅ W2.2a — shared image admission (completed)

**Measured result:** REST/OpenAI rejected even with a vision sidecar; channels already captioned, contrary to the original claim below. Channels lacked correlated rejection audit and accepted empty captions. `Kernel.AdmitImages` now serves all three adapters, preserves channel artifacts/captions, consumes raw images after captioning, and returns the existing control-plane rejection message. Actual adapter tests failed on old code; four separate mutations prove sidecar use, raw-image removal, rejection correlation and model precedence.

The following is the original task context, retained for the measurement trail.

**Defect (measured in W2.0):** a run with images on a non-vision model behaves differently depending on
where it came from:
- **Control plane** (`kernel/controlplane/server_handle_run.go`, the console and `agt run`): asks a vision
  model to describe the images (`k.DescribeImages`, M821) and continues.
- **REST/OpenAI** (`cmd/agezt/api_engine.go:63`) and **every channel** (`cmd/agezt/main_channels_handler.go:33`):
  hard-reject via `visionGate` → `gateVisionWith` (`cmd/agezt/main_channels.go:18`).

So a photo sent over Telegram/WhatsApp to a daemon whose model lacks vision is refused, while the same
photo in the console works.

**Do:**
- Put one function in `kernel/runtime`, e.g. `(*Kernel).AdmitImages(ctx, corr, model, intent, images)`.
  It returns the new intent, the remaining images and an error, and it performs the confirm-or-caption-or-reject
  logic plus the `governor.capability` / `KindCapabilityRejected` event the control plane emits today.
- Call it from all three sites, and delete `visionGate` / `gateVisionWith` if nothing else uses them.
  Check `cmd/agezt/coverage_media_test.go`, which tests them.

**Tests:** a channel-style and a REST-style run with an image on a non-vision model, plus a configured
vision sidecar, must caption (red on old code). With no sidecar it must still reject with the same message.

**Note:** this is the first slice of the roadmap's `RunRequest`/`runs.Start`, so name it with that in mind.

### 4.2 ✅ W2.2b — agent-profile resume (completed)

**Measured result:** persistent tests interrupted soul-only, model-only and combined profile runs, reopened the stores and invoked the real boot resumer. All three were quarantined on the old code. Model/system values now carry their source profile slug in the run context; profile defaults remain reconstructible, while explicit setters replace the source. The control plane only calls those setters for explicit args. The ticket format and boot resumer are unchanged; older non-resumable tickets are still quarantined. Tests cover same-value explicit overrides, unnamed/retargeted profile values, denied tools, a tighter saved trust ceiling against a looser current profile, saved cost, attempts durable before provider dispatch, ticket cleanup and quarantine rails.

**Evidence:** `resume_profile_internal_test.go`, `controlplane/run_resume_profile_test.go`, `cmd/agezt/resume_profile_test.go`. Independent mutations verify model/system source, explicit-override rejection, control-plane provenance, attempt persistence, trust/cost restoration and the attempt cap. The following is the original task context, retained for the measurement trail.

**Claim** (findings register 9.2; `architecture/04-agent-runtime.md` gotcha 6):
- `buildResumeTicket` (`kernel/runtime/resume.go:96–127`) marks a ticket `Resumable=false` whenever
  `systemFromCtx` or `modelFromCtx` is set.
- `WithAgentProfile` sets both for any agent with a soul or model, so such runs are **quarantined at boot**
  instead of resumed, even though the resumer (`cmd/agezt/main_standing_resume.go`) re-applies
  `WithAgentProfile` and would rebuild them exactly.

**Measure:** write the test first. Start a run AS an agent with a soul, kill it mid-run (the resume tests in
`kernel/runtime` show how), reopen, and assert that it resumes. Expect red.

**Fix direction (target §3.2 step 5):** the ticket stores the *agent slug* plus the *explicit per-run
overrides*, not the resolved context. A system/model that came from the profile is reconstructible and
must not block resume. Only a per-run `--system`/`--model`/`--tools` that the ticket does not record
should block it, or record those too.

**Careful:** the resume path is crash-loop-guarded (attempt counter fsynced before dispatch). Don't weaken
that. Owner decision 5.5′: approvals are *not* persisted, and a resumed run re-asks.

### 4.3 ✅ W2.3 — tool side paths (exit verified)

**W2.3a measured and fixed:** actual registered-tool, HTTP, pipeline and canvas-node calls lacked policy/invoked/result events; direct denials lacked a terminal result. The shared `RunTool`/`toolexec` invoker now serves those paths, keeps forge/MCP lookup, supplies trusted ToolDef metadata (including parameter-dependent capability axes), stamps approval/run identity, catches tool panics and reports audit failures. Pre-invocation audit failure prevents execution. Persistent regressions were red on old code; thirteen independent mutations guard the boundaries.

**W2.3b measured and fixed:** actual Council grounding ignored `web.search=L0`, agent tool deny and trust ceiling, called the search and injected its evidence with no policy/tool audit. It now uses the shared invoker, preserving one search per panel, the 300-rune query/6-hit bounds, unique search IDs and date-only fallback on refusal/failure. Correlated live approval blocks both search and panel calls until resolution. Disabled/missing/nil search tools remain no-ops. Tests: `council_grounding_audit_test.go` (allow, three restriction paths, errors, malformed output, panic, disable/missing, HITL grant/deny and repeated IDs) plus the original Council suite. Seven mutations independently guard the boundaries.

**W2.3c measured and fixed:** the actual Conductor executed worker code and passed verification even with `code.exec=L0`, a profile deny or trust ceiling, with no policy/tool audit. Workflow code nodes enforced policy but left no tool execution audit. Both now adapt their existing sandbox runners into invocation-local `code_exec` tools and use the same governed invoker, with trusted capability/effect metadata and distinct attempt IDs. Denial/preflight audit failure never enters the runner; Conductor reports `Ran=false` in its result and a false live execution flag, failing verification without critique fallback. Backend errors, error results and panics fail with terminal audit. Interpolated inputs, structured output, workflow error ports/retries and Conductor critique-only paths remain covered. Tests: `code_execution_audit_test.go` plus the original Conductor/workflow suites; ten independent mutations guard the boundaries. Sandbox execution/network settings are unchanged.

**W2.3d foundation (move only):** the direct invoker's panic-contained tool call now lives in `platform/toolinvoke.Invoke`. The `toolexec` forwarding helper preserves the same result/error/context and panic error text; policy, admission and audit remain in their callers. This is the move-before-rewrite step for agent convergence, not the full app invoker. Three mutations independently reject missing recovery, lost errors and replaced context.

**W2.3e measured and fixed:** eight actual agent-loop scenarios (panic/cancel, sequential/parallel, fault first/last) lost terminal `tool.result` records; panic also skipped per-call context cancellation. Agent execution now uses the shared platform primitive, releases the call context and settles the whole admitted batch before task failure. A sequential panic still prevents later effects; those calls get failed results marked `not_executed`. Parallel dispatch stays bounded and results retain original order. Terminal batches do not invoke bookkeeping/automation hooks or make another model call; cancellation is rechecked after audit and before return. An audit-write failure is joined with the typed panic/cancellation cause and remaining terminal writes are attempted. Tool-log/stats honor the skipped marker and omit invented execution latency. Actual entry-point and source suites plus twelve independent mutations cover these boundaries; timeout feedback, memo/taint/offload and default allow remain.

**W2.3f foundation (move only):** agent output representation moved to `platform/tooloutput.Offload`. `agent.ArtifactPutter` and `DefaultArtifactThreshold` remain compatibility aliases; its forwarding helper retains the 8 KiB default, byte threshold boundary, preview, full artifact bytes and best-effort inline fallback. Three mutations reject a wrong boundary, lost fallback and storing only a preview. Original agent/source suites remain green.

**W2.3g measured and fixed:** actual direct, workflow, canvas and code paths journaled 20 KB inline without `raw_ref`/`output_bytes`. Runtime now supplies its artifact store and effective threshold to `toolexec.RunWithOptions`, using the same audit-only representation as the loop. Legacy `Run` retains its signature and inline/no-store behavior. Success, reported error, invocation error, panic and denial use preview/ref/byte count; caller and noise hook receive full output and typed error causes remain intact. Nil/unavailable stores, empty refs and inline boundaries retain best-effort fallback. Four actual-path regressions were red; seven independent mutations guard runtime wiring/threshold, metadata, caller/hook bytes and error retention.

**W2.3h foundation (move only):** `PolicyVerdict` and `Policy` moved to the pure `contract/policyapi` package with exact agent aliases. The loop's existing 23-field policy renderer moved to `platform/toolaudit.PolicyDecisionPayload`; a forwarding helper preserves its journal bytes, nil/zero values and event ownership. Full-value and actual-loop regressions plus four mutations cover field retention, call identity, provenance and forwarding. Same-verdict allow/deny tests measured that direct Run/RunWithOptions still omit 15 fields; the behavior repair follows separately.

**W2.3i measured and fixed:** same-verdict allow/deny tests drove actual agent.Run and Run/RunWithOptions: direct records omitted 15 of the loop's 23 fields. Actual direct/workflow/canvas/code allow/deny journal regressions reproduced the loss. Direct invocations now use the shared policy renderer, preserving resource, epistemic and observation/provenance metadata, call/correlation identity and explicit nil/zero values. Eight mutations guard field retention, identity, verdict, event kind and preflight audit error. Decision/admission behavior, default allow, approval and terminal output remain unchanged.

**W2.3j foundation (move only):** measured direct invoker dependence on `agent.ValidateToolInput` and `agent.WithPolicyToolDef`. The unchanged dependency-free schema validator now lives in `platform/schema`; resolved ToolDef/observation context helpers live in `platform/policyctx`, with the taint type in pure `contract/policyapi`. Agent forwarding functions/type alias retain schema errors, nil/empty behavior, parent context and cross-boundary accessor interoperability. Direct importers remain unchanged for this extraction; repointing follows separately. Exact contract/source suites and seven mutations cover JSON admission, nested arrays, registration lint, metadata/taint, empty-taint identity and parent context.

**W2.3k repointing (behavior preserved):** direct Run/RunWithOptions now use `platform/schema` validation and `platform/policyctx` resolved metadata. Production `go list -deps ./kernel/toolexec` no longer reaches `kernel/agent`. Actual invoker tests pass on both old and new source: unknown/invalid/schema-rejected calls reach no policy/backend/events/hooks; allow/deny paths preserve resolved metadata, parent agent/provenance, correlation/call/input and result/audit semantics. Five mutations and 20-repeat entry-point tests guard those boundaries. The loop still owns its admission sequence and remains on the compatible shared helpers.

**W2.3l invocation-port foundation:** `contract/toolapi` owns Invocation/Invoker and lookup/artifact ports. The legacy pipeline implements that port through `toolexec.NewInvoker`; `runtime.Open` constructs one service per kernel via Config.NewToolInvoker (nil factory retains standalone compatibility). RunTool and invocation-local code adapters call the injected port with lookup, effective artifact settings and original identity. Constructors bind each kernel's own policy/audit/hook ports; a nil result fails startup and unwinds stores before the gateway listener starts. Actual direct/workflow/canvas/code offload and two-kernel policy/journal tests cover the boundary; seven mutations guard injection, local lookup, artifact options, identity, host isolation and nil startup. Pipeline bodies/admission/approval remain unchanged. App/daemon factory binding follows separately.

**W2.3m app/daemon binding:** `app/tools.NewInvoker` binds the existing shared pipeline behind the L1 port. Primary and tenant daemon construction use the same `openAppKernel` helper, creating fresh services against each kernel's own dependencies; no global registration or L3 app import. Standalone runtime.Open/legacy Run remain compatible. Actual four-path offload/policy/provenance, two-kernel isolation, command-constructor and live approval identity tests run through app binding. Six mutations guard daemon binding, context, identity, local lookup, effective threshold and per-kernel instances. The pipeline implementation remains in legacy toolexec and the agent loop still owns its separate admission; this app ingress is not full W2.3 completion.

**W2.3n generic-engine move:** the sole schema/policy/audit/execution implementation moved unchanged from legacy toolexec into `platform/toolpipeline`. Host policy remains injected; the engine has no runtime/agent/app import. Public toolexec types remain exact aliases and Run/RunWithOptions retain production-reachable forwarding through the same output decorator. App construction now uses the moved engine, while dependency type consumers remain on compatibility aliases until repointing. Execution/recovery/output/port-adapter bodies were compared to old source; new primitive and original source contracts plus eight mutations guard schema/audit/denial, typed errors, caller bytes, artifact options, context and local lookup.

**W2.3o dependency repointing:** app construction and runtime host/factory declarations now consume `platform/toolpipeline` ports directly. Production app dependency closure excludes legacy toolexec, runtime and agent. Runtime retains one explicit legacy NewInvoker fallback for standalone compatibility; all public legacy Run/RunWithOptions stay reachable through that adapter and execute the same engine. No policy/audit/result/batch behavior changes. Existing primitive, app/compatibility and actual runtime entry-point suites protect the repointing; the engine's eight mutation guards remain.

**W2.3p policy-phase foundation:** `toolpipeline.Decide` separates trusted ToolDef context, policy callback and mandatory decision audit from execution/memo. Direct Run now uses it and retains the returned context for backend/hook metadata. Primitive contracts and six mutations guard resolved metadata, policy/audit ordering, error/call retention and execution context. Actual agent tests on old source fixed the existing contract: whole-batch admission precedes effects in sequential/parallel allow/deny cases, and a memo hit never bypasses a later denial (count=20). Agent production gating is unchanged in this extraction; shared-phase routing follows separately.

**W2.3q shared agent policy admission:** the now-unused agent.WithPolicyToolDef setter shim is removed (use platform/policyctx.WithPolicyToolDef), without a deadcode exception.  agent gating now uses toolpipeline.Decide for resolved metadata, policy callback and mandatory policy audit. The loop scopes directive taint before the phase and retains explicit no-policy default allow, caller event/error envelope, deny counters and policy-before-memo. It consumes only the verdict, preserving its existing execution context. Actual sequential/parallel allow/deny batch ordering, later-denied memo, whole-batch policy-audit failure and causal-window/provenance tests pass count=20. Seven mutations guard memo/effect ordering, audit error, window, metadata, default allow and call envelope. Execution/timeout/terminal settlement and availability/schema/loop guard/memo scheduling remain unchanged.

**W2.3r shared resolution (move only):** direct and agent admission now use `toolpipeline.Resolve` for caller-owned lookup, one trusted definition read and unchanged schema validation. The phase performs no policy/audit/tool execution; callers retain their unavailable/schema error envelopes. Old-source gate and actual mixed-batch tests passed count=20, alongside direct preflight/schema and batch/memo contracts. Registry aliases retain full metadata; missing/invalid calls consume no loop-guard quota and never reach policy/effects. Eight mutations guard lookup identity, availability-first ordering, schema error, metadata, single definition, direct rejection and loop quota/metadata. The enabled loop set and dynamic direct registry remain caller-owned; no shim or allowlist expansion.

**W2.3s shared invocation announcement (move only):** `toolpipeline.Announce` publishes the same tool.invoked kind and three-field call payload through a mandatory caller publisher, returning its error unchanged. Direct and loop callers retain their journal identity/error envelopes and call it only after policy/memo admission. It accepts no executor or context, so announcing a loop call does not begin its effects. Old-source gate/direct contracts plus actual batch/memo-coalescing/later-denial tests passed count=20. Nine mutations guard event kind, name/ID/input, audit cause, single publish, memo ordering, whole-batch audit stop and direct effects after audit. No execution/timeout/settlement behavior change or new shim.

**W2.3t shared execution (move only):** `toolpipeline.Execute` now owns the admitted safe invocation plus the loop’s unchanged optional positive per-call timeout, deadline capture and cleanup. It returns the original result/error, raw panic value and timeout fact. The loop supplies its ErrPanic mapper before context cleanup (including context-sensitive panic formatting); direct calls supply zero timeout and no mapper, retaining caller budget and panic text. Correlation/metadata remain assembled by callers, and batch scheduling/terminal settlement/hooks are unchanged. Old-source loop/direct and actual panic/cancel/timeout contracts pass count=20. Twelve mutations guard context, positive-only budget, result/error/panic facts, deadline-vs-error, cleanup/mapping order, mapper-on-panic and loop/direct wiring. The private direct forwarding helper is removed; shared toolinvoke.Invoke stays production-reachable.

**W2.3u shared terminal publication (move only):** `toolpipeline.Settle` owns the terminal kind and authoritative tool/call/output/error fields, copies caller metadata and returns the mandatory publisher error unchanged. Direct deny/error/panic/success and loop finalization use it. The loop passes an audit-only preview copy, retaining full model/hook bytes, observation provenance/deltas, typed nil-versus-empty matches, artifact/memo/skipped fields and result ordering. Classification, remaining batch write attempts, error joins and terminal hook suppression stay caller-owned. Old-source representation/cause/cancel-hook and actual terminal/delta/memo contracts pass count=20; twelve mutations cover kind/identity/output/error, metadata ownership, cause, one publication, remaining writes, full hooks and direct denial audit.

**W2.3v phase-port foundation:** the pure contract/toolphaseapi package owns the Phases port and unchanged resolution/decision/execution data, with exact platform aliases. It is separate from toolapi because llm already imports toolapi. The constructed platform/app invocation service implements all five shared phases; direct Invoke routes through its own phase port. RunWithOptions can receive an explicit phase service, including through legacy forwarding, while nil retains the original legacy path. Publisher kind strings bridge the pure contract to the registered event kinds without a journal dependency. Trace tests cover every phase, early schema/deny/audit stops, typed causes and per-service/local lookup; eleven mutations guard app exposure, injected selection, each phase, registry ownership, legacy forwarding and kind bridging. Agent/runtime loop routing is unchanged in this foundation.

**W2.3w runtime/loop binding:** an actual app-bound Kernel.RunWith executed its tool while the injected execution phase saw zero calls on old source. Kernel.Open now retains the constructed service’s phase port and BuildLoopConfig supplies it to both root and delegated loops. All five loop phases use that port, retaining enabled lookup, taint/default-policy, guard/memo, batch scheduling, timeout/terminal/error/hook and caller envelopes. Standalone agent.Run gets a per-run canonical default; legacy runtime invokers expose the phase port. Older custom one-shot-only factories retain their previous loop behavior through one host-bound canonical compatibility phase service per kernel. Actual primary/tenant deny, all-phase counters, delegated child correlation and one-shot compatibility pass count=20; ten mutations guard binding, every phase, service retention and compatibility.

**W2.3 exit verified:** side-path and dynamic forge/MCP fixtures now explicitly inject the app constructor and pass count=20, alongside actual root/delegated all-phase traces. Journal identity/stat fixtures pass separately. See [exit evidence](22-w23-exit-evidence.md) for the executable coverage, source boundary and legacy/custom-factory limits. Caller-specific batch/memo/observation/model behavior remains outside the shared mechanism. **Next open slice: §4.4.**

The lower-layer invocation port and per-kernel constructor injection are in place. The app service binds the shared L2 mechanism; L3 runtime/loop must not import app or grow the allowlist. Preserve standalone `runtime.Open`/legacy Run compatibility and the loop's batch admission/memo behavior; the shared preflight helpers no longer require an agent implementation import.

Retry attempts get distinct audit IDs. Tool log/stats join by run plus call ID, so a denied call cannot borrow another run's input/latency. Both guarantees were red before their fixes. Evidence: `kernel/runtime/workflow_tool_audit_test.go`, `kernel/toolexec/toolrun_test.go`, and `kernel/controlplane/tool_audit_identity_test.go`. The original finding list follows; all four original side-path findings are fixed (W2.3a–c).

The findings register (9.1, "Side paths skip governance/audit") lists:
1. ✅ workflow registered-tool/HTTP/pipeline/canvas calls now journal policy/tool events; code nodes joined in W2.3c;
2. ✅ Council grounding calls the shared governed invoker and records policy/tool audit (W2.3b);
3. ✅ Conductor code verification now checks `code.exec` and journals policy/tool events through the shared invoker (W2.3c);
4. ✅ `toolexec` now emits `tool.result` on deny and resolves active forge/MCP tools.

The exit evidence now records actual app-bound tests for these paths and their journal assertions. The original implementation requirement follows: The fix target is one invoker (target §3.3, `app/tools.Invoke`): lookup → policy
decision (Edict, trust ceiling, agent tool policy) → journal `tool.call`/`tool.result` → execute. Do it
in slices, one side path per PR or one invoker PR plus re-pointing PRs.

**Constraints:**
- Default-allow posture: every capability is LevelAllow by default and restriction is opt-out. Don't add new denials.
- Tool capability must be mapped: an unmapped tool name means an unknown capability, which Edict default-denies.

### 4.4 ✅ W2.x — governed File Manager and file snapshot restore

**W2.xa foundation:** an actual authenticated mkdir request returned HTTP 200 and
created the directory, but its durable journal contained zero op events and the
control-plane caller saw zero calls. A separate authenticated file.snapshot
rollback request restored the previous bytes with zero caller requests and zero
op/policy records in its temporary durable journal. Both defects are confirmed. The existing root,
path normalization and resolved containment bodies now live unchanged in
`platform/fileworkspace`; The WebUI resolver method forwards during migration; the unused root wrapper is removed.
Existing HTTP/source tests and target contracts preserve creation paths, exact
errors, home expansion and root containment. Four independent mutations guard
root creation, NUL rejection, missing-tail handling and resolved containment.
The real source junction/symlink fixture remains. This is a move before the
operation rewrite; mkdir/rename/delete and file snapshot restore are still local
and unjournaled. Next: bind their mutations to primary-only control-plane ops
and the canonical policy/audit invoker; remove the temporary resolver forwarding method
when the adapters consume the shared root/resolver directly.

**W2.xb mutation primitive move:** the console's mkdir/rename/delete filesystem
branches now live in `platform/fileworkspace`. HTTP handlers still own decoding,
path resolution, identical status/text mapping and response fields; final symlink
refusal, missing-file OS identity and recursive opt-in are retained. Source and
target tests pass count=20; four mutations guard parents, rename direction,
recursive opt-in and OS error identity. This separate move leaves governance
unchanged. Next bind these primitives through primary-only audited control-plane
operations, then migrate file snapshot restore independently.

**W2.xc File Manager binding complete:** mkdir/rename/delete are now primary-only
control-plane mutations. Dispatch assigns the operation correlation to the
handler; app/files admits an invocation-local adapter through the existing
per-kernel invoker. Mkdir/rename use file.write; delete uses file.delete. Root
creation and path resolution occur only after policy and mandatory audit. Actual
HTTP/control-plane tests inspect the five-event allow arc and four-event deny
arc under one correlation/call identity, verify disk effects, and prove denial
or unavailable audit does not bootstrap a missing root. Source HTTP path/status/
JSON contracts are retained through optional domain error_code transport; the
unused resolver shim is removed. Eight independent mutations guard the port,
capability, correlation, error propagation, policy, audit/tenant metadata and
local lookup. Default-allow remains. **Next: file.snapshot rollback restore.**

**W2.xd rollback-store foundation:** checkpoint/catalog data and the existing
catalog/restore/conversion bodies move mechanically to platform/rollbackstore.
WebUI keeps type aliases and production forwarders until operation binding;
legacy AGEZT_HOME lookup, JSON fields, version defaults, atomic writes, restore
bytes/absence and validation errors are unchanged. Source and target suites pass
count=20; body parity and four mutations guard restore absence, bytes, directory
refusal and zero catalog version. This foundation does not add governance.
Next: a primary-only file snapshot restore op, using the existing invocation
port and trusted catalog data, with no snapshot content in audit payloads.

**W2.xe file snapshot restore binding complete:** the primary-only file_restore
op resolves the checkpoint from the daemon's injected catalog path and passes a
private snapshot adapter through the existing per-kernel invoker. Content restore
uses file.write; absent-state restore uses file.delete. Only checkpoint identity,
path and existence enter audit input; snapshot bytes stay private. The server
marks AppliedMS only after successful governed restore and owns the catalog
write inside the audited operation. Already-applied checkpoints remain no-ops.
Actual HTTP/socket tests assert allow/deny event arcs, correlation/call IDs,
unchanged file/catalog on denial or unavailable audit, repeat behavior and no
snapshot data in any journal payload. A separate fixture proves caller-supplied
Before data and a different AGEZT_HOME cannot redirect the daemon's restoration.
Eight mutations guard the port, privacy, capability, applied order, repeats,
correlation, audit metadata and catalog authority. The file restore forwarding
shim is removed; remaining catalog/type compatibility serves the existing
skill/workflow/config UI until its domain migrates. File mutation findings are
closed. **Next: §4.5 operation framework.** WebUI catalog discovery still uses its
legacy home lookup; file restore mutation authority is daemon-owned.

The following is the original finding context, retained as the measurement trail.
Findings register 9.1: the web console's File Manager and rollback-restore wrote the filesystem directly
from the web UI layer: `kernel/webui/files_route.go` and `kernel/webui/rollback.go`
(+ `rollback_helpers.go`). Confirm with a test that inspects the journal. Now that W2.1a exists, the fix is
to make them control-plane ops: then dispatch journals them automatically (op audit), and they can take
a policy check. The web UI then calls the op instead of touching the disk.

### 4.5 W2.1b — the op framework proper (transport independence)

**W2.1b foundation:** source signatures confirmed status/version results were
coupled to net.Conn. Their status/fallback/version presentation now lives in
app/system with context + explicit Input -> Output/error handlers. The existing
map outputs and host reads are unchanged; the control-plane wrappers keep socket
encoding, auth/tenant/read-only metadata and fresh daemon extras. HTTPBinding and
ChannelInfo aliases retain callers. Actual socket-free handler tests and original
status/version/auth/tenant/op-audit suites pass count=20. Status/fallback body
parity and four mutations retain empty-head clamp, fallback dimensions, optional
tenant binding and build provenance. This is the move before registry/dispatch
rewriting; metadata derivation, typed schema framework and adapter routing are
still open. Do not call the full W2.1 framework complete yet.

**W2.1c typed dispatch pilot:** contract/opapi owns pure operation metadata,
principal/caller and auth/router/audit/emitter ports. app binds Go-typed unary and
emitting handlers, derives supported input/output schemas and owns immutable
registry dispatch: authenticate, authorize, route, decode/validate, mandatory
mutation audit, handler, terminal audit. Audit failure/cancellation prevents
handler entry; panic and typed terminal causes settle through owned spans.
System status/version now register from app specs; control-plane metadata is
derived and old socket handler wrappers are deleted. The adapter keeps native
auth framing, binds fresh service data and encodes output. Explicit unknown-input
compatibility retains the pilot's old args behavior; legacy output maps remain.
Source and pipeline/schema/stream/metadata contracts pass count=20; eight
mutations guard admission/effects/schema ownership and adapter metadata.
Mock-host, no-audit-I/O dispatch benchmark: 3.6–4.5 us/op on local Windows,
GOMAXPROCS=4 (not end-to-end status/journal-fold latency).
At W2.1c, schema derivation rejected nullable pointers, embedding and custom
JSON representations instead of advertising an incorrect contract. W2.1d adds
nullable support; the full framework is still incomplete: output maps remain legacy. Mutating/streaming transport host
adapters and other domains still need migration. Extend these boundaries against
actual types before closing W2.1; generated surfaces remain later work.

**W2.1d nullable/schema refinement:** actual typed registration rejected nullable
pointer fields. FromType now derives null unions for pointer roots/fields, nil
slices/byte slices and maps, retaining required-vs-optional fields and validation
of non-null nested values. Map additionalProperties carries its element schema;
recursive unsupported shapes still fail registration. Actual app.Dispatch tests
retain root/field nulls and reject missing required fields or typed map mismatches
before handlers. Source/target suites, count=20 and four mutations guard pointer,
slice/byte null and typed map contracts. The existing validator already supported
type unions; no tool-validation semantics were rewritten. Embedding/custom
representation, strongly typed outputs and mutation/streaming host adapters
remain open next; full W2.1 is still not complete.

**W2.1e explicit/custom wire schemas:** a time.Time registration with a supplied
schema was red because binding discarded the declaration. Explicit input/output
schemas are now linted and copied at registration; reflection still records the
actual Go types, and absent declarations keep derived schemas. Terminal results
and stream emissions are serialized/validated against the owned output schema
before leaving app dispatch. Custom decoder input restrictions, immutable schema
bytes, invalid-schema registration and bad terminal/stream values have actual
contracts (count=20), with five independent mutation guards. Output validation
raises the mock-host/no-I/O benchmark to 7.0–8.6 us/op, still below 50 us; it does
not measure live I/O. Embedding derivation, typed pilot output models and
mutation/streaming transport hosts remain open; full W2.1 is not complete.

**W2.1f typed pilot outputs:** status/version now return Go output models, including
nested schedule/fallback/delegation metadata. Status derives an actual field
schema; version uses an explicit schema for the brand.Binary custom wire key.
The control-plane adapter serializes typed outputs into its legacy Result map.
Optional metadata omission and an enabled empty tenant registry (`tenants: 0`)
remain compatible; returned metadata slices are copied. Original status JSON
parity, typed wire/schema and source tests pass count=20; five independent
mutations guard zero-tenant presence, snapshot ownership, nested wire names,
output schema specificity and the version brand key. Full Go/build/vet/static
and architecture gates pass without allowlist growth. Embedded derivation and
production mutation/streaming host adapters remain open; full W2.1 is incomplete.

**W2.1g embedded wire fields:** registration tests were red for named/flattened
anonymous fields and silently lost exported fields of private embedded values.
Schema derivation now resolves the encoding/json wire field set before deriving
child types: shallower fields win, tagged fields win at equal depth, unresolved
conflicts disappear, tagged embeddings remain nested and skipped fields stay
absent. Promoted fields beneath nil-capable anonymous pointers are optional.
Private value embeddings retain exported fields; private pointer allocation,
recursive embeddings and custom/quoted representations still require explicit
schemas. Actual MarshalJSON parity, app dispatch admission/terminal output and
source suites pass count=20; six independent mutations guard promotion, depth,
tag priority, conflicts, nil-parent omission and private value promotion. Full
Go/build/vet/static and architecture gates pass with unchanged allowlists.
Mock-host/no-I/O dispatch is 6.4–8.5 us/op (<50 us). Production mutating/streaming
hosts and remaining domains stay open; full W2.1 is not complete.

**W2.1h independent stream contracts:** source signatures confirmed the streaming
builder required the emitted frame and terminal result to share one Go type,
while actual market handlers emit progress events and return a distinct record.
The builder now binds Input/Output/Emission types and independent owned schemas;
frames validate against EmissionSchema and the return value against OutputSchema.
Same-type streams retain their former explicit output-schema fallback. Unary
operations expose no emission metadata and cannot emit even if a transport port
is supplied. Actual dispatch contracts with distinct structs, derived/explicit
schemas, custom time.Time emissions, malformed frames/terminal values, transport
causes and schema-byte ownership pass count=20; six mutations guard independent
binding, frame schema selection, emission metadata/ownership/copies and unary
mode. Full Go/build/vet/static and architecture gates pass without allowlist
growth. No domain or production host has migrated in this slice: mutating audit
and socket streaming host adapters remain next; full W2.1 is incomplete.

**W2.1i control-plane host ports:** real socket fixtures were red because app
mutations had no auditor and streams had no emitter; rejected typed inputs also
entered legacy socket audit. The common adapter now binds mandatory journal
admission/settlement and native event publication. Migrated registrations carry
AppOwned so app dispatch alone owns their audit; legacy handlers retain their
existing path. Routing binds the primary/caller-tenant kernel plus shared actor
and correlation context. Registration checks object terminal schemas and native
kernel event frame types before effects. Actual socket mutation/progress, panic,
invalid input, preflight/terminal persistence failure, tenant-vs-primary journal,
secret argument redaction, transport causes and live-disconnect settlement pass
count=20. Thirteen independent mutations guard the host/ownership/routing/wire
boundaries. Full Go/build/vet/static and architecture gates pass with unchanged
allowlists. Shipped status/version remain read-only; mutation/stream fixtures
exercise the production registration/factory/adapter with test operations.
**Next: verify the complete W2.1 exit against actual supported representations,
then continue the ordered catalog/provider domain migration.** Generated surfaces
and broader adapters remain later work; the architecture goal is not complete.

**W2.1j text codec boundary and framework exit:** exit checks reproduced netip.Addr
being derived as an empty object although encoding/json writes a string. Go text
encoder/decoder method sets now require explicit schemas, including pointer and
nested/collection representations. Actual explicit decode/dispatch/encode tests
pass count=20; separate encoding-only and decoding-only mutations fail. Complete
framework, pilot, native host and source invariants pass count=20; full Go/build/
vet/static and architecture gates pass without allowlist growth. No-I/O dispatch
is 6.9–9.0 us/op (<50 us). [W2.1 exit evidence](23-w21-exit-evidence.md) maps each
framework requirement to source and executable proof and states the supported
schema, principal, native wire and live-verification boundaries. **W2.1 framework
plus status/version pilot is complete; next migrate catalog/provider in roadmap
order.** Other domains, generated surfaces, broader adapters and W3–W5 remain open.

**W2.4a catalog move foundation:** source signatures confirmed sync/list/discover
were coupled to net.Conn. Their existing fetch/persistence/full-reload/event and
wire projection now live in app/catalog behind context + Input -> Output/error
methods. The CP wrappers retain original arg decoding/error text, primary-only
metadata and legacy audit. Output maps, credential lookup, deterministic model
order, optional prices/reload errors and env defaults remain. Actual socket-free
sync/list/discover plus source/catalog/registry/tenant/audit suites pass count=20;
original list socket JSON parity passes count=20. Nine independent mutations
retain full sync/discovery reloads, both optional rebuild errors, scoped/duplicate
credential rules, model order, failed discovery kind and the price wire field.
Full Go/build/vet/static and architecture gates pass: 219 packages, unchanged
145 import/13 call allowlists. This is the move before operation binding; no
policy/audit rewrite or additional provider domain move. **Next: catalog specs,
common app adapter binding and deletion of these compatibility handlers.**

**W2.4b catalog operation binding:** app/catalog owns three typed specs and actual
input/output models, including nested providers/models and optional price values.
Registration derives CP metadata and the per-server dispatcher includes system
plus catalog ops. Old sync/list/discover socket handlers are deleted. Unknown
legacy args remain allowed; known fields validate before fetch or audit. Sync
and discovery use mandatory app-owned audit, list stays read-only, and domain
success/failure events share the host-owned operation correlation. Real socket
fixtures reproduced ignored primary-only tenant args being copied into audit
principal metadata; operator tenant is now set only for caller-tenant ops.
Existing tenant-routed tests retain operator-selected tenant behavior. Native
framing, JSON timestamp/list parity, full reload, optional rebuild errors,
credential projection, known-zero vs unknown price behavior and HTTP proxy
contracts pass count=20. Eight independent mutations retain registry inclusion,
mutation/read-only metadata, ignored args, typed schemas, price wire name, primary
audit scope and domain identity. Full Go/build/vet/static and architecture gates
pass without allowlist growth (219 packages; 145 imports/13 calls). **Catalog
migration is complete; next provider domain, move before binding.** Generated
routes/broader adapters and the overall architecture migration remain open.

**W2.4c provider catalog/keyring move:** connect/reload and four keyring business
handlers now live in app/providers with context + Input -> Output/error methods.
CP wrappers retain primitive arg validation/order, response error text, existing
registration/auth/audit and request-independent service construction. Existing
catalog IDs keep their model entries; new endpoints retain unknown model coverage
instead of seeding a UI hint. Scoped/global key target rules, fingerprint-only
listing, active mirror persistence and reload-on-active-change behavior remain.
Socket-free lifecycle/default/error tests and source/provider/registry/tenant/
audit suites pass count=20; six original socket handler results have JSON parity
count=20. Nine independent mutations retain existing-entry preservation, unknown
models, scoped/namespace validation, active reloads/persistence and rebuild error
contracts. Full Go/build/vet/static and architecture gates pass (220 packages;
unchanged 145 imports/13 calls). **Next: provider catalog/keyring operation binding,
then OAuth lifecycle and tenant-routed log/stats/rejections and primary-only probe.** This partial
provider foundation does not close the complete provider domain or rewrite policy.

**W2.4d provider catalog/keyring binding:** six typed specs/results join the common
app registry and per-server dispatcher. Per-operation inputs validate only fields
used by that operation, retaining ignored legacy args. Old CP catalog/provider/
keyring socket wrappers and their validation-helper copies are deleted. Five
mutations now require app-owned audit before catalog/vault/reload effects; key
listing stays read-only and returns only label/active/last4. Real socket lifecycle,
rejected-input/unavailable-audit effects, scoped privacy and source/auth/tenant/
registry/HTTP contracts pass count=20; six original handler JSON results retain
parity count=20. Eight independent mutations guard inclusion, mutation/read-only
metadata, ignored args, typed active/output fields, redaction and mandatory audit.
Full Go/build/vet/static and architecture gates pass: 220 packages, import debt
145 -> 144 (CP -> catalog removed and officially ratcheted), 13 call sites remain.
**Next: provider OAuth lifecycle, then tenant-routed log/stats/rejections and primary-only probe.**
The entire provider domain and overall architecture goal remain open.

**W2.4e provider OAuth state/business move:** login state, its mutex/listener,
token-manager initialization and start/status/import/logout business now belong
to app/providers.OAuth. CP owns one lazy instance per Server and preserves fresh
model-hook lookup, legacy input coercion, socket framing and auth/audit metadata.
The existing fixed-port callback/expiry/deferred-close/page flow moves with its
state; its HTTP adaptation is deliberately unchanged in this move-only slice.
Socket-free isolated import/status/logout, authoritative hook model surface,
state filtering, denial/concurrent status and HTML escape tests pass count=20,
alongside source OAuth/registry/tenant/audit contracts. Six independent mutations
retain import/token clearing, model hook, state filter, denial state and escaping.
Full Go/build/vet/static and architecture gates pass: CP -> chatgptauth edge is
removed and officially ratcheted (143 imports/13 calls; 220 packages). **Next:
OAuth specs/common operation binding; callback HTTP/lifecycle refinement follows
separately if required by adapter migration.** Tenant-routed observations and the
primary-only provider probe remain open; the whole provider domain is incomplete.

**W2.4f provider OAuth RPC binding:** four typed specs/results now register through
the common app dispatcher; old socket start/status/import/logout wrappers are
removed. The factory resolves the existing per-server OAuth state at handler
entry. Start/import/logout require mandatory app audit before state/listener/
vault/reload effects; status retains the existing read-only metadata and model
hook behavior. Known typed fields validate before admission while unknown legacy
args remain accepted. Actual socket import/status/logout, authoritative empty
models, token privacy, correlation/order and preflight effect barriers pass
count=20 alongside original OAuth/registry/tenant/audit/HTTP and typed schema
contracts. Seven independent mutations retain registry, mutation/read-only flags,
ignored args, rich output schema, null/empty models and mandatory audit. Full
Go/build/vet/static and architecture gates pass: 220 packages, 143 import/13 call
exceptions remain. **Next: tenant-routed provider log/stats/rejections and
primary-only provider probe.** Existing callback HTTP/expiry/lifecycle behavior
is unchanged; later callback adapter/lifecycle refinement and the overall
provider/architecture goal remain open.

**W2.4g shared journal projection move:** newest-first sorting, cursor filtering,
row cutoff, pagination and output stamping now live in platform/journalview.
CP retains limit admission, since_ms conversion, tenant selection and native
result/error framing; every existing log caller still shares the same engine.
Decoder callbacks see every event before cutoff so cross-event inputs outside
the window remain available to results inside it. The moved body matches the
original after reader/error-envelope boundary substitutions. New primitive and
original log/provider suites pass count=20; seven independent mutations reject
lost decoder state, time/sequence ordering, cursor filtering/order, boundary
cursor and journal error causes. Full Go/build/vet/static and architecture gates
pass: 221 packages, unchanged 143 import/13 call exceptions. **Next: move provider
log/stats/rejections business into app/providers, then bind tenant-routed typed
operations; primary-only probe and callback adapter refinement remain open.**

**W2.4h provider observation service move:** app/providers.Observations owns
log/stats/rejections journal folds and wire projections. The host supplies its
selected journal; CP retains tenant selection, legacy limit/window/boolean
admission and socket result/error framing. Log uses the shared platform engine;
rejections retains its existing non-cursor wire shape and stats excludes model-
chain hops from provider fallback rates. Three original/current native handler
outputs match across eight argument sets count=20. Socket-free observation and
source provider/tenant/registry/audit suites, including actual socket primary/own-tenant
data isolation and cross-tenant refusal, pass count=20; nine independent
mutations retain window, provider/model distinction, filters, model identities,
sequence order, chain wire and journal causes. Full Go/build/vet/static and
architecture gates pass: unchanged 221 packages, 143 import/13 call exceptions.
**Next: typed observation specs/results and common tenant-routed binding; then
primary-only probe.** Callback adaptation and the overall provider migration
remain open.

**W2.4i provider observation operation binding:** log/stats/rejections now have
three typed specs/results in the common registry. The operation factory reads
the routed app host's journal; OwnTenant/CallerTenant and read-only metadata
replace the old socket wrappers/registrations. Typed numeric/boolean admission
runs before service entry; unrelated legacy args remain accepted. Numeric limits
retain truncation, absent/null default 20 and clamp 1..1000; permissive opaque
cursors retain malformed/non-string first-page fallback. Relative window
conversion lives at the app boundary, and present-empty optional row fields
remain present through typed pointers. Native JSON parity across eight argument
sets, empty-field/schema/cursor/window contracts and actual socket tenant data
isolation pass count=20 with source registry/audit/HTTP contracts. Ten independent
mutations retain registry, tenant admission/routing, read-only flags, ignored
args, limits/window, field presence and rich row/map schemas. Full Go/build/vet/
static and architecture gates pass: unchanged 221 packages, 143 import/13 call
exceptions. Log retains its existing HTTP hint; stats/rejections remain native
operations without inventing HTTP routes. **Next: primary-only provider probe.**
Callback HTTP/lifecycle refinement and the overall provider migration remain open.

**W2.4j guarded GET move:** the unchanged bounded HTTP helper shared by provider
probe and WhatsApp gateway status/QR now lives in platform/netout.GatewayGET.
CP retains its forwarding helper and all native handlers/admission metadata.
The moved body matches original source after its function rename. Fresh client,
background ten-second timeout, local/LAN posture, redirect limit, header/status/
content-type forwarding, body bounds and best-effort partial reads retain their
existing behavior. Local fixture primitive and actual provider/WhatsApp socket
contracts pass count=20 alongside original netout posture/redirect/client tests;
eight independent mutations retain method, headers, bounds, response fields,
loopback posture, parse identity and native forwarding. Full Go/build/vet/static
and architecture gates pass: unchanged 221 packages and 143 import/13 call
exceptions. **Next: primary-only probe service, then typed operation binding.**
Caller-context/partial-read refinement, callback adaptation and full provider
migration remain open; this mechanical move changes no HTTP posture or effects.

**W2.4k provider probe service move:** app/providers.Probe owns the endpoint
models check behind an injected bounded GET port. Its default uses the unchanged
platform/netout helper; CP retains lenient string admission, native framing and
primary-only/read-only registration. URL/key trimming, models path, one-MiB bound,
reachability versus authorization, 2xx-only model counting and failure-result
shape retain their source behavior. Socket-free six-status fixtures and source
contracts pass count=20; original/current native JSON outputs match across six
statuses and six argument sets count=20. Eight independent mutations retain
missing-URL admission, normalization, path/bound, 401 reachability, model counting
and failure shape. Full Go/build/vet/static and architecture gates pass: unchanged
221 packages and 143 import/13 call exceptions. **Next: typed primary-only probe
spec/output and common operation binding.** Context/callback adaptation and the
overall provider migration remain open; no live provider or paid model was used.

**W2.4l provider probe operation binding:** a typed primary-only/read-only spec
and variant-preserving output now use the common dispatcher. Its HTTP hint
matches the existing POST /api/provider/probe route; the old native wrapper and
registration are deleted. Known URL/key types validate before transport entry;
unrelated legacy args remain accepted. Success preserves explicit false/zero
fields; endpoint failure keeps only ok/error. Actual socket admission, tenant
refusal before requests, read-only audit absence, optional output/schema and
source tenant/registry/audit/HTTP contracts pass count=20. Original/current native
JSON parity spans six statuses and four compatible argument sets count=20;
eight independent mutations retain registry, primary admission/routing,
read-only flags, ignored args, rich schema, field presence and route metadata.
Full Go/build/vet/static and architecture gates pass: unchanged 221 packages and
143 import/13 call exceptions. **Next: separate provider OAuth callback business
from browser HTTP adaptation.** Existing callback lifetime and probe background
transport context remain for subsequent refinement; the provider/architecture
goal is not closed by native RPC binding alone.

**W2.4m OAuth callback business extraction:** callback admission and effects now
live in the socket-free completeProviderLogin helper. The HTTP wrapper supplies
query fields, request context and the actual exchange port, renders the returned
result, and honors its delayed-close signal. Denial/invalid state, code/verifier,
parent context, thirty-second budget/release, failed/canceled exchange, terminal
state before reload, existing ignored reload errors and model hook order retain
source behavior. Socket-free and original OAuth/native contracts pass count=20;
old/new status/headers/HTML/login-state parity spans five denial/invalid requests
count=20. Nine independent mutations retain admission, exchange identity/context/
budget, terminal state, models, close signal and HTTP framing. Full Go/build/vet/
static and architecture gates pass: unchanged 221 packages and 143 import/13 call
exceptions. **Next: extract callback HTTP query/presentation adaptation.** Existing
fixed-port startup, five-minute expiry, delayed close and logout behavior remain
for separate lifecycle work; no live token exchange or paid provider was used.

**W2.4n callback HTTP presentation move:** platform/browsercallback owns query/
request-context projection, the unchanged success/failure HTML renderer/escaping,
and post-render close dispatch. App's callback bridge invokes its business helper
and returns the result; the now-unreachable private page wrapper is removed and
its original source test targets the actual renderer. Render/escape bodies match
original source after renaming. Primitive query/context/close-order/HTML and
source OAuth/native contracts pass count=20; original/current HTTP status/headers/
HTML/login-state parity across five denial/invalid inputs passes count=20. Seven
independent mutations retain query/context, framing, escaping and close decisions.
Full Go/build/vet/static and architecture gates pass: 222 packages, unchanged 143
import/13 call exceptions; the official structure writer records 113 kernel
packages. **Next: callback listener/lifetime ownership, then caller context.**
Fixed-port startup, TTL and delayed-close/logout behavior remain unchanged;
no live exchange, paid provider or browser session was used for this move.

**W2.4o callback listener move:** platform/browsercallback.Prepare owns TCP bind,
callback path, mux and HTTP server/header timeout. App stores the prepared listener,
publishes the login under its mutex, then launches its Serve method; app's bound
completion bridge is socket-free and production app no longer imports net/http
or net. Existing fixed address, status publication, expiry/delayed-close and
server-close semantics remain. Prepared listener path/bind/options/Serve/Close
and source OAuth/native contracts pass count=20; old/new callback HTTP page/state
parity remains count=20. Six independent mutations retain TCP/caller address,
path, header budget, explicit Serve and server close. Full Go/build/vet/static and
architecture gates pass: unchanged 222 packages and 143 import/13 call exceptions.
**Next: prove/fix listener lifetime ownership, including Close before Serve.**
TTL/logout/stale-session refinement and caller context remain open; this move
alone does not claim those lifecycle guarantees or live OAuth exchange.

**W2.4p prepared listener ownership repair:** an owned loopback proof reproduced
that Close before Serve left the bound port unavailable for immediate rebinding.
Listener.Close now closes both HTTP server and prepared socket, normalizes an
already-closed socket, and joins remaining cleanup causes. Permanent regressions
cover Close-before-Serve, repeated close, concurrent Serve/Close, immediate port
reuse and owned close-error identity. Focused/source tests and the full adapter
race suite pass count=20; three independent mutations retain release, idempotent
normalization and error propagation. Full Go/build/vet/static and architecture
gates pass: unchanged 222 packages and 143 import/13 call exceptions. **Next:
provider session/expiry/logout ownership, then caller-context refinement.**
This scoped repair does not claim token/session fencing or TTL/logout semantics;
its sockets are controlled fixtures, with no live provider exchange.

**W2.4q successful logout callback ownership repair:** a controlled prepared
listener proof reproduced successful token logout retaining pending login ownership
and its callback port. Logout now stops the login after successful vault cleanup,
before reload. A failed token cleanup preserves the pending login and propagates
its existing error. Permanent logout/port-reuse and vault-failure regressions,
source OAuth/native tests and callback race tests pass count=20; two independent
mutations retain stop-on-success and failure ordering. Full Go/build/vet/static
and architecture gates pass: unchanged 222 packages and 143 import/13 call
exceptions. **Next: expiry/session ownership and caller-context refinement.**
This fix does not claim cancellation of the five-minute expiry worker or fencing
of a token exchange already in progress; those remain explicit lifecycle work.

**W2.4r expiry worker ownership repair:** an isolated actual Start/Stop goroutine
proof reproduced a retired login retaining its five-minute sleep worker. Login
now owns stop/done channels and idempotent cancellation; stop and delayed callback
close release the timer worker. Normal TTL keeps the existing timeout state;
expiry checks current login identity and cancellation before changing state.
Permanent Start/Stop wiring, stop/callback-close, normal TTL and retired timer
regressions plus source/native and callback race tests pass count=20. Four
independent mutations retain cancellation, current identity, timeout state and
Start wiring. Full Go/build/vet/static and architecture gates pass: unchanged
222 packages and 143 import/13 call exceptions. **Next: in-flight token/session
fencing, then caller-context refinement.** This repair does not claim ownership
of a token exchange already running or cancellation of its persistence effects.

**W2.4s token exchange fetch/persist foundation:** Manager.ExchangeTokens owns
unchanged authorization-code/PKCE HTTP fetching and returns the raw token candidate
without manager/vault effects. Public ExchangeCode forwards fetch then StoreTokens,
retaining legacy persistence/account derivation and error behavior. App's callback
uses the two explicit steps in the same order; no session policy change is claimed
in this foundation. Isolated endpoint form/candidate/no-write/error tests and
original manager/OAuth/native contracts pass count=20. Four independent mutations
retain legacy persistence, candidate fields, verifier form and fetch error cause.
Full Go/build/vet/static and architecture gates pass: unchanged 222 packages and
143 import/13 call exceptions. **Next: admit persistence against the current login
identity after fetch, then caller-context refinement.** In-flight ownership,
logout/replacement fencing and live provider validation remain open.

**W2.4t callback session persistence admission:** a controlled delayed fetch proof
reproduced logout completing before an old callback restored tokens and reported
success. App now admits persistence under the login mutex against current identity,
pending state, open expiry ownership and live context. Logout token cleanup and
retirement use the same mutex, preserving pending ownership on failure. Callback
business checks ownership before exchange and after its await before terminal
state/model effects. Production fetch is an owned constructor-bound port; no live
endpoint is needed for the race proof. Permanent delayed logout/replacement,
context/stopped/terminal/retired admission, current success and post-exchange
retirement tests plus source/native and callback race suites pass count=20.
Five independent mutations retain identity/state/context/stop and post-exchange
admission. Full Go/build/vet/static and architecture gates pass: unchanged 222
packages and 143 import/13 call exceptions. **Next: caller-context refinement and
provider exit evidence.** Effects admitted before retirement retain their normal
ordering; broader generated surfaces/adapters/domains and the architecture goal
remain open.

**W2.4u probe caller-context repair:** controlled typed-dispatch HTTP fixtures
reproduced cancellation leaving the probe on its background timeout, and canceled
response bodies reporting successful probes. Context-aware GatewayGETContext,
Probe.CheckContext and injected port now carry caller cancellation/deadline while
retaining the guarded posture/ten-second ceiling. Caller cancellation after body
read returns its cause; legacy GatewayGET/Check and injected context-free getters
retain their background/best-effort behavior. Typed probe uses the context-aware
entry. Primitive/source/native/context/legacy contracts and focused race tests
pass count=20; four independent mutations retain transport/operation/port context
and canceled-body result. Full Go/build/vet/static and architecture gates pass:
unchanged 222 packages and 143 import/13 call exceptions; no dead-code exception
was added for the live legacy entry points. **Next: provider exit evidence.**
Live provider/browser validation and broader generated transports/domains remain
outside these controlled-fixture and native-host checks.

**W2.4v catalog/provider native exit:** all 17 operations have typed input/output
schemas and common AppOwned native binding. Aggregate registry evidence guards
the complete set, read-only flags and primary/own-tenant routing; old business
handlers are absent. Related source/native/registry/tenant/audit/lifetime/context
suites pass count=20; full Go/build/vet/scoped static and architecture gates pass
with unchanged 222 packages, 143 imports/13 calls and dead-code/dependency ratchets.
No-I/O dispatch is 6.4–6.9 us/op (<50 us). [Exit evidence](24-w24-exit-evidence.md)
maps operation ownership, lifetime repairs and validation boundaries. **Next:
memory, world, taste and skill in roadmap order.** Broader adapters, generated
surfaces, full runs.Start convergence and W3–W5 remain open; controlled fixtures
do not certify live provider/browser behavior.

**W2.5a memory read-service foundation:** current native get/search/find-related
bodies depend on socket response helpers. Their store lookup/search, legacy limit
defaults/cap and seed exclusion now live in app/memory with context + typed input
-> typed output/error; CP retains required/type admission and native framing.
Typed record projection preserves optional expiry (including present zero),
provenance, tombstone/suspension and absent-versus-empty response fields. Existing
source tests caught omitted lifecycle fields during the move; these are retained
and guarded by explicit wire contracts. Three original/current native handlers ×
nine argument sets pass count=20; exact row/field/error parity allows only bounded
wall-clock ranking-score drift. Source/read/registry/tenant/audit suites and the
new package race tests pass count=20; eight independent mutations fail. Full
Go/build/vet/scoped static and all gates pass: 223 packages, unchanged 143
imports/13 calls, official kernel structure 114; no exception expansion.
**Next: remaining memory business (list/log/write/hygiene/distillation), then
typed operation binding and old wrapper deletion.** This foundation retains the
existing native registry/auth/audit behavior; it does not close memory migration.

**W2.5b memory curation-service foundation:** add/supersede/forget/promote/
bulk-forget store business and typed output now live in app/memory. RememberInput
owns field conversion, source-tag merge and explicit operator Actor/Force; caller
tags stay untouched. CP retains required/type/order admission, native framing and
the existing registry/audit behavior. Same-content supersede remains a no-op;
promotion preserves present empty subject, and bulk forget keeps repeated/missing
counts, the 500-ID bound and stop-on-error partial effects. Five original/current
native handlers × ten argument sets match exact JSON count=20. Source/curation/
registry/tenant/audit and new-package race suites count=20 plus ten independent
mutations pass. Full Go/build/vet/scoped static and all gates pass: unchanged 223
packages, 143 imports/13 calls and official kernel structure 114. **Next: memory
list/log/hygiene/distillation, then typed binding and wrapper deletion.** No
correlation, policy or audit rewrite is included in this business-only move.

**W2.5c memory hygiene-service foundation:** prune/tidy/audit/clean business
now lives in app/memory with context + typed input/output/error. Native wrappers
retain nil/tenant selection, lenient day conversion, bool/string dry-run admission
and the existing registry/audit behavior. Prune retains its 30-day default,
pre-mutation stats, separate present-zero prunable/pruned fields and age predicate;
tidy and clean forward the admitted dry-run flag and preserve curated records.
Four original/current native handlers × eight argument sets match count=20 after
normalizing bounded cutoff-clock drift and existing unordered contradiction
groups/members. Source/hygiene/registry/tenant/audit and package race tests pass
count=20; nine independent mutations fail. Full Go/build/vet/scoped static and all
gates pass: unchanged 223 packages, 143 imports/13 calls and structure 114.
**Next: memory list/log/distillation, then typed binding and wrapper deletion.**
This move changes no policy, native registration, audit or correlation behavior.

**W2.5d memory distillation-service foundation:** consolidate/profile-rebuild
orchestration now lives in app/memory.Distillation behind a runtime-supplied
Distiller port. Methods accept context and typed input and return typed reports;
the unchanged orchestration still creates a fresh correlation and owns a bounded
five-minute background context, canceled on success/failure. Native report fields
retain present null arrays/zero counts and active_after naming. Two original/
current handlers × two argument sets × running/halted states match count=20 after
normalizing fresh correlation IDs. Permanent actual-socket no-op/halt and fake-port
report/order/budget/cleanup/cause contracts pass count=20; nine independent
mutations fail. Full Go/build/vet/scoped static and all gates pass: unchanged 223
packages, 143 imports/13 calls and structure 114. **Next: memory list/log move,
typed binding, then measured caller-context/operation-correlation refinement.**
This move preserves background orchestration and legacy audit behavior; it does
not yet promise caller cancellation or shared operation identity for distillation.

**W2.5e memory list-service foundation:** app/memory.PreparedList owns
newest-first record projection and cursor pagination. PrepareList reads active
records before native argument admission, preserving the source order; CP then
admits typed limit/cursor and encodes Page's typed result. Independent pages copy
the prepared slice. Legacy default/cap/fractional limits, timestamp/ID ties, strict
cursor filtering, pre-filter total, terminal cursor omission and empty arrays
retain behavior; the duplicate CP record projection is removed. Original/current
native list matches exact JSON across twelve argument sets count=20. Source/list/
registry/tenant/audit and package race tests pass count=20; eleven independent
mutations fail. Full Go/build/vet/scoped static and all gates pass: unchanged 223
packages, 143 imports/13 calls and structure 114. **Next: memory journal log,
typed binding and measured caller-context/operation-correlation refinement.**
Native registration/auth/audit remain unchanged during this business move.

**W2.5f memory journal-service foundation:** app/memory.LogService owns the
memory event fold, aliases and typed row shaping through platform/journalview;
the native adapter retains lenient limit/window/cursor admission and selects its
tenant journal before the service call. Write/revive aliases, forget/supersede/
promote identity/subject shaping, cutoff/cursor order, present empty/zero row
fields, empty ops and always-present next_cursor retain behavior. Original/current
native log matches exact JSON across thirteen argument sets count=20. Source/log/
registry/tenant/audit and package race suites pass count=20; eight independent
mutations fail. Full Go/build/vet/scoped static and all gates pass: unchanged 223
packages, 143 imports/13 calls and structure 114. The business foundation now
covers the 16 memory/profile native commands. **Next: typed operation binding,
mandatory audit/host metadata, old wrapper deletion and measured caller-context/
operation-correlation refinement.** Native registration/auth/audit remain unchanged
during these moves; the complete memory migration remains open.

**W2.5g memory typed native binding:** a restored legacy-binding proof confirmed
closed-journal requests still added/superseded/forgot/promoted/cleaned records and
reported success. All 16 memory/profile specs now share the common app/native
dispatcher; ten mutating operations require audit admission before service entry.
Six reads stay unaudited; audit/clean/log retain OwnTenant/CallerTenant and every
other operation stays primary-only. Known type/null/required admission precedes
effects; unknown unused args, bool/string dry-run, lenient day/log inputs and
bulk trim/blank filtering retain compatibility. Source wrappers/registrations
and two newly unreachable result helpers are removed without new exceptions.
Actual closed-journal and tenant socket fixtures, typed schema/metadata/effect
contracts, source/registry/tenant/audit and focused race suites pass count=20;
eleven independent mutations fail. Sixteen old/current bindings × three compatible
argument sets pass count=20 with documented cutoff/score clocks, fresh identity
and generic failure-code boundary. Full Go/build/vet/scoped static and all gates
pass: unchanged 223 packages, 143 imports/13 calls and structure 114.
**Next: measured caller-context/operation-correlation refinement, then memory
native exit evidence.** Distillation still owns its legacy background context and
fresh identity, and memory business writes retain their prior domain correlation;
the complete memory/architecture migration remains open.

**W2.5h memory operation/domain identity repair:** an actual typed native add
proof confirmed operation audit had a correlation while memory.written had none;
a controlled distillation port proved the admitted identity was replaced. All
eight store mutations now pass the context operation correlation to their manager.
Both distillation paths retain the admitted identity and mint a fresh one only
when the context has none. Context-free store calls retain their empty domain
correlation, and background distillation retains its legacy fresh identity/budget.
Permanent native invocation -> domain effect -> terminal ordering, eight real
store/bus/journal paths and both controlled distillation paths pass count=20;
ten independent mutations reject each lost identity path. Source/typed/native/
tenant/audit and focused race contracts count=20 plus full Go/build/vet/scoped
static and all gates pass: unchanged 223 packages, 143 imports/13 calls and
structure 114. **Next: measured distillation caller-context refinement, then
memory native exit evidence.** Background model context/lifetime remains for the
next slice; this repair adds no unrelated policy or model invocation behavior.

**W2.5i distillation caller-context repair:** controlled blocked-port proofs
confirmed consolidate/profile rebuild retained model waits after caller cancel,
lost caller deadlines/values and admitted already-canceled direct calls. Both
paths check cancellation before identity/port effects and derive their owned
five-minute ceiling from the caller context. Cancellation, earlier deadline and
model context values now reach the runtime port; deferred cleanup and admitted
operation identity remain. Background callers retain fresh fallback identity and
the five-minute ceiling. Direct/typed blocked cancellation, deadline/value/pre-
canceled, report/cleanup/identity and source/native/tenant/audit contracts plus
focused race suites pass count=20; six independent mutations reject service,
admission and typed binding context loss. Full Go/build/vet/scoped static and all
gates pass: unchanged 223 packages, 143 imports/13 calls and structure 114.
**Next: memory native exit evidence, then world/taste/skill in roadmap order.**
Controlled ports and isolated mock-provider sockets do not certify live model or
provider behavior; broader adapters, generated surfaces and W3–W5 remain open.

**W2.5j memory native exit:** complete common-registry coverage now guards all
16 memory/profile operations, actual schemas/types and native metadata. Both
independent family/operation removal mutations fail; restored coverage and
related source/typed/native/tenant/audit/identity/context suites pass count=20.
Full Go/build/vet/scoped static and all gates pass with unchanged 223 packages,
143 imports/13 calls and structure 114. No-I/O dispatch is 5.7–7.2 us/op (<50 us).
[Exit evidence](25-w25-exit-evidence.md) maps ownership, admission, closed-journal
repair, tenant isolation, domain identity and caller lifetime with explicit
parity/live-provider boundaries. **Next: world, then taste/skill in roadmap order.**
Memory native migration is complete; knowledge/runtime module dissolution,
broader adapters, generated surfaces, runs.Start and W3–W5 remain open.

**W2.6a world graph-service foundation:** add/edit/relate/resolve/neighbors/
list/get/forget business now lives in app/world with context + typed input/output/
error. Native wrappers retain required/type/collection/limit admission, registry,
auth/audit and legacy empty correlation. Entity projection moves with business;
the private duplicate CP view is removed. Content identity, open kind/verb
normalization, alias/attribute replacement, relation endpoint/direction behavior,
quiet resolve, absent/empty/zero and lifecycle/provenance fields retain semantics.
Resolve receives the already-admitted integer limit, preserving fractional input
that truncates to zero. Eight original/current native handlers × eight argument
sets match count=20 with bounded ranking clock drift. Source/graph/registry/tenant/
audit and new-package race tests count=20 plus twelve independent mutations pass.
Full Go/build/vet/scoped static and all gates pass: 224 packages, 142 imports/13
calls. The official writer removes paid CP->worldmodel debt only; official kernel
structure is 115 packages. **Next: world journal fold, then typed operation binding
and old wrapper deletion.** This move changes no policy/audit/correlation behavior;
the complete world migration remains open.

**W2.6b world journal-service foundation:** app/world.LogService owns world
entity/relation/forget event folding, labels and typed rows through journalview.
Native wrappers retain kind type admission, lenient limit/window/cursor behavior
and tenant-selected journal before service entry. Kind annotations, relation
direction labels, forgotten name/verb fallback, upsert default, filtering,
cutoff/cursor boundaries, empty arrays and present zero/empty fields retain source
behavior. Original/current native log across thirteen argument sets matches exact
JSON count=20. Source/log/graph/registry/tenant/audit and package race suites pass
count=20; ten independent mutations fail. Full Go/build/vet/scoped static and all
gates pass: unchanged 224 packages, 142 imports/13 calls and structure 115.
**Next: world typed operation binding, old wrapper deletion and measured domain
identity refinement, then native exit evidence.** Native registry/auth/audit remain
unchanged during this business move; complete world migration remains open.

**W2.6c world typed native binding:** restored legacy binding confirmed four
graph mutations changed state and returned success with a closed journal. All
nine world specs now use the common app/native host; four mutations require audit
before service effects and five reads remain unaudited. World log keeps routed
tenant scope; all graph commands stay primary-only. Known required/type/null
admission precedes effects; unused unknown fields, case-preserving trimmed aliases,
native resolve defaults/cap/fractional zero and lenient log inputs remain compatible.
Old socket wrappers/registrations and the now-unused argStringMap helper are removed
without new exceptions. Actual closed-journal and tenant socket, metadata/schema/
effect/source/registry/audit and focused race suites pass count=20; nine independent
mutations fail. Nine old/current bindings × three compatible inputs match count=20
with bounded score-clock drift and generic failure-code boundary. Final full
Go/build/vet/scoped static and all gates pass: unchanged 224 packages, 142 imports/
13 calls and structure 115. **Next: measured world operation/domain identity
refinement, then native exit evidence.** Graph business retains its prior empty
domain correlation; complete world migration and broader architecture remain open.

**W2.6d world operation/domain identity repair:** actual typed native add
proved operation audit had identity while the graph upsert event had none. Add,
edit, relate and forget now pass the admitted context correlation into the graph;
relation-created endpoint events share it too. Context-free calls retain legacy
empty domain correlation. Permanent native ordered invocation -> graph effect ->
terminal proof and all four real store/bus/journal paths pass count=20; four
independent mutations reject each missing identity path. Source/typed/native/
tenant/audit and focused race suites count=20 plus full Go/build/vet/scoped static
and all gates pass: unchanged 224 packages, 142 imports/13 calls and structure 115.
**Next: world native exit evidence, then taste/skill in roadmap order.** This
repair changes event joins without altering graph normalization, policy or model
behavior; broader domain/adapter/module migration remains open.

**W2.6e world native exit:** complete common-registry coverage guards all nine
world operations, actual schemas/types and native metadata. Independent family/
log removal mutations fail; restored source and related source/graph/log/typed/
native/tenant/audit/identity suites pass count=20. Focused race and final full
Go/build/vet/scoped static plus all gates pass: unchanged 224 packages, 142
imports/13 calls and structure 115. No-I/O dispatch is 10.5–11.7 us/op (<50 us).
[Exit evidence](26-w26-exit-evidence.md) records graph contracts, admission,
closed-journal repair, tenant scope and ordered domain/audit identity with explicit
clock/failure-code/live-provider boundaries. **Next: taste, then skill in roadmap
order.** World native migration is complete; knowledge/runtime module dissolution,
broader adapters, generated surfaces, runs.Start and W3–W5 remain open.

**W2.7a taste-service foundation:** list/create/delete store business now lives
in app/taste with context + typed input/output/error. Native wrappers retain
lenient trimmed string/limit/tag admission, registration/auth/audit and response
framing. Exemplar fields use the actual store type, preserving optional scope/tags,
timestamps and present-empty arrays. Source store filtering/case-sensitive tag
dedupe, creation validation, delete causes and persistence rollback retain behavior.
Three original/current native handlers match count=20 with explicit fresh identity
and bounded create-clock boundaries. Source/service/store/registry/tenant/audit and
package race suites count=20 plus eight independent mutations pass. Full Go/build/
vet/scoped static and all gates pass: 225 packages, 141 imports/13 calls. Official
ratchet removes only paid CP->taste debt; generated kernel structure is 116.
**Next: taste typed operation binding and old wrapper deletion, then exit evidence
and skill.** This move changes no native policy/audit or runtime exemplar selection;
complete taste migration remains open.

**W2.7b taste typed native binding:** a closed-journal fixture confirmed legacy
create/delete returned success and changed exemplars despite unavailable audit.
Three typed app/taste operations now derive native metadata through the common
host. Both mutations require audit before effects; list remains unaudited.
Old wrappers/registrations are removed. Primary-only policy/store selection,
StreamNone, no invented HTTP hints, lenient strings/default-200 limits (including
fractional zero), mixed/CSV tags and unused fields retain native compatibility.
Output schemas derive actual exemplar fields. Three original/current handlers
match count=20 with fresh identity/bounded create-clock boundaries. Source/service/
store/registry/tenant/audit and package race pass count=20; eight independent
mutations fail. Actual closed-journal/tenant sockets and single-owned-audit-arc
regressions pass. Full Go/build/vet/scoped static and all gates pass: unchanged
225 packages, 141 imports/13 calls, structure 116. **Next: taste native exit
 evidence, then skill.** Runtime exemplar selection and broader migration remain.

**W2.7c taste native exit:** exact common-registry coverage guards all three
operations, actual schemas/types, AppOwned binding and primary/read-only metadata.
Independent family/delete removal mutations fail; related source/native/tenant/
audit/exit suites pass count=20. Package race and final full Go/build/vet/scoped
static plus all gates pass: unchanged 225 packages, 141 imports/13 calls and
structure 116. Three benchmark measurements satisfy the <50 us dispatch budget.
[Exit evidence](27-w27-exit-evidence.md) records lenient admission, typed exemplar
fields, closed-journal repair, one owned audit arc, primary store isolation and
fresh-identity/clock/live-provider boundaries. **Next: skill in roadmap order.**
Taste native migration is complete; broader adapters, generated surfaces,
knowledge/runtime dissolution, runs.Start and W3-W5 remain open.

**W2.8a skill read-service foundation:** skill list/get business now lives in
app/skill with context + typed input/output/error and a narrow actual Forge reader
port. Typed records retain the native projection, required empty agent/description
and six zero-valued metric fields, optional body/provenance/arrays, list order,
active count, present-empty arrays and missing found-only shape. Native wrappers
retain ID admission, registrations/auth/audit and response framing. Original/current
native list/get handlers match exactly count=20 across empty/missing/invalid inputs
and draft/shadow/active states. Source/service/store/registry/tenant/audit and package
race suites pass count=20; eight independent mutations fail. Full Go/build/vet/
scoped static and all gates pass: 226 packages, unchanged 141 imports/13 calls,
structure 117. No new exception or early debt removal. **Next: remaining skill
history/lifecycle/file services, then typed binding and exit evidence.** Runtime
skill execution/retrieval and broader migration remain separate work.

**W2.8b skill lifecycle-service foundation:** promote/quarantine/archive/revert/
restore now live in app/skill.Lifecycle with context + typed input/output/error.
Native wrappers retain required/optional string admission, registration/auth/audit
and response framing. Actual Forge transitions, archive idempotence, parent
restoration, restore target validation and cause propagation retain behavior.
Wire outputs preserve present-empty archive/restore reason and restored-parent ID.
Five original/current native handlers x three initial states x six argument sets
match exactly count=20. Source/service/store/native and package race suites pass
count=20; eight independent mutations fail. Final full Go/build/vet/scoped static
and all gates pass: unchanged 226 packages, 141 imports/13 calls, structure 117.
No new exception or early debt removal. **Next: skill ownership/import/history/
file services, then typed binding/domain identity and exit evidence.** This move
retains empty legacy domain correlation; common-host identity binding follows.

**W2.8c skill curation-service foundation:** share/reassign/import now live in
app/skill.Curation with context + typed input/output/error. A narrow actual Forge
port and caller-selected roster lookup retain ownership admission before effects.
Native wrappers keep required/optional/lenient string/array/resource admission,
registration/auth/audit and framing. New imports stay draft/content-addressed;
existing content retains its lifecycle and first owner. Portable bundle contents,
resource manifests, empty target/present name and original causes retain behavior.
Three original/current native handlers match exactly count=20 across admission,
ownership, Unicode resources and dedupe shapes. Source/service/store/native and
package race suites pass count=20; eight independent mutations fail. Final full
Go/build/vet/scoped static plus all gates pass: unchanged 226 packages, 141 imports/
13 calls, structure 117. No new exception or early debt removal. **Next: skill
history/files/hygiene, then typed binding/domain identity and exit evidence.**
Legacy domain correlation and native policy remain until common-host binding.

**W2.8d skill observation-service foundation:** history/files/read_file/hygiene
now live in app/skill.Observations with context + typed input/output/error. Native
wrappers retain ID/path/lenient idle-days admission, registration/auth/audit and
response framing; unused old history/projection helpers are removed. Typed history
rows retain chronology, admitted lifecycle kinds, ID/restored matching and required
empty correlation; nil events remain null. Files prefer nonnil successful disk
listing and retain manifest fallback, directory and Unicode byte count. Hygiene
retains 30-day default, cutoff, projection plus top-level usage and present-empty
idle arrays. Four original/current native handlers match exactly count=20. Source/
service/store/native and package race count=20, eight independent mutations and
final full Go/build/vet/scoped static plus all gates pass: unchanged 226 packages,
141 imports/13 calls and structure 117. This move explicitly retains best-effort
history Range error swallowing and its existing kind set; any behavior repair
requires its own proof. **Next: skill typed binding, measured history/domain
identity repairs and exit evidence.** No new exception or early debt removal.

**W2.8e skill typed native binding:** actual closed-journal fixtures confirmed
all eight legacy mutation commands returned success and changed skill state with
unavailable operation audit. Fourteen typed app specs now derive native metadata
through the common host. Mandatory audit precedes all eight mutations; six reads
remain unaudited. Old wrappers/registrations and resource admission helper are
removed. CLI compare rollback evidence points to the moved lifecycle/spec sources. Primary-only scope/store selection, StreamNone and actual nine HTTP hints
remain; native-only get/history/read_file/restore/reassign gain no invented hints.
Inputs retain byte-preserving IDs/reasons/path, lenient import text, strict nonnull
string arrays, nullable string resource objects, lenient numeric/decimal-string
idle-days and unused fields. Actual output schemas retain native wire projection.
Fourteen old/current bindings x three compatible inputs match count=20 with the
generic typed failure-code envelope explicitly normalized; fields/domain errors
remain exact. Closed-journal/effect/metadata/schema/tenant/source/registry/audit and
package race suites pass count=20; eight independent mutations fail. Final full
Go/build/vet/scoped static and all gates pass: unchanged 226 packages, 141 imports/
13 calls, structure 117. CP->skill debt remains owned by roster teardown sources;
no exception is removed early or expanded. **Next: measured history error/domain
identity repairs, then skill native exit.** History legacy behavior stays open.

**W2.8f skill history read-error repair:** owned corrupt JSONL journals proved
native history returned successful empty/partial results despite real Range decode
failure; a reader fixture also exposed the swallowed original cause. History now
returns the original Range error and discards failed partial output. Valid history
shape/order/kinds and malformed individual payload filtering remain. Permanent
native corrupt-journal empty/partial cases and exact-cause/zero-output service
cases pass count=20; three independent mutations fail. Source/native/tenant/audit
and package race count=20 plus final full Go/build/vet/scoped static and all gates
pass: unchanged 226 packages, 141 imports/13 calls and structure 117. **Next: skill
domain operation identity and measured ownership-history coverage, then exit
evidence.** Fixtures use owned TempDir journals; no owner journal is touched.

**W2.8g skill operation/domain identity and ownership-history repair:** actual
native fixtures proved eight mutations' domain events carried empty correlation
beside admitted audit IDs, and real shared/reassigned events were absent from
history. Lifecycle/curation methods now forward opapi caller identity; context-free
calls retain empty correlation. History admits both ownership kinds without
changing chronology, row projection, original Range failure or malformed-row
filtering. Eight direct caller paths and eight actual native fixtures verify
exactly one trusted ordered invocation/domain/completion arc; client-supplied
correlation input cannot choose the identity. Native ownership-history fixtures
retain event IDs/correlation/sequence and show both changes. Source/native/tenant/
audit/CLI rollback/compare and package race count=20, ten independent mutations
and final full Go/build/vet/scoped static plus all gates pass: unchanged 226
packages, 141 imports/13 calls and structure 117. CLI already renders ownership
kinds; the repaired fold exposes those rows. **Next: skill native exit evidence.**

**W2.8h skill native exit:** exact fourteen-command common-registry coverage
verifies actual schemas/types and native metadata. Independent family/import
removal mutations fail; related source/native/tenant/audit/history/identity/exit
and CLI suites pass count=20. Package race and final full Go/build/vet/scoped
static plus all gates pass: unchanged 226 packages, 141 imports/13 calls,
structure 117. Three dispatch benchmarks satisfy <50 us excluding audit I/O.
[Exit evidence](28-w28-exit-evidence.md) records projection/admission, lifecycle,
ownership/import/bundles, mandatory audit, read errors, tenant isolation and
trusted domain identity. CP->skill debt stays with roster teardown sources.
**Next: board, then workboard/OKR/storage/artifacts in roadmap order 3.** Broader
adapters, generated surfaces, runs.Start and W3-W5 remain open.

**W2.9a board-service foundation:** all seven board business methods now live
in app/board with context + typed input/output/error and the actual store port.
Native wrappers retain store selection/admission/registration/auth/audit/framing:
reads select the shared store or fresh fallback; writes require the shared instance.
Typed message projection retains required topic/text/ts_unix_ms, optional IDs/
addressing/help and owned acknowledgement slices. Read retains full-store cursor
filtering, descending timestamp/ID ties, pre-filter total, topic counts, optional
cursor and admitted zero as unbounded. Send retains reply/help/broadcast/DM/post
precedence, original reply topic/recipient, source failures and one success-only
notifier with the explicit inbound correlation; ack/inbox/replies/help retain
existing case/idempotence/order behavior. Seven old/current native handlers match
count=20 across argument/pagination/routing/unavailable-writer/fresh-reader inputs;
fresh sent IDs/bounded send clocks are documented comparison boundaries. Source/
service/store/native and package race count=20, ten independent mutations and
final full Go/build/vet/scoped static plus all gates pass: 227 packages, unchanged
141 imports/13 calls, structure 118. Old message projection helper is removed;
roster uses still own board limit constants/debt. **Next: board typed binding and
measured audit/notifier behavior, then exit/workboard.** No native policy change.

**W2.9b board typed native binding:** closed-journal fixtures proved send/ack
changed the shared store and returned success; send also notified despite unavailable
operation audit. Seven typed app specs now derive native metadata through the common
host: two mutations require audit before factory/store/notifier effects, five reads
remain unaudited. Host factories retain selected shared/fresh-reader and shared-only
writer behavior plus unavailable-store causes. Lenient strings/limits, fractional
zero as unbounded, strict bool/read strings, cursor/projection/routing and explicit
inbound notifier/result correlation remain. Old native wrappers/registration and
unused limit admission helper are removed; roster-owned constants/debt remain.
Seven old/current native bindings match count=20 with fresh IDs/bounded send clocks
and generic typed failure-code boundary; fields/domain errors remain checked.
Actual closed-journal/fallback/shared-writer/notifier/tenant/schema/metadata/effect
and source/store/package-race count=20, eight mutations plus final full Go/build/
vet/scoped static and all gates pass: unchanged 227 packages, 141 imports/13 calls,
structure 118. Four existing HTTP hints remain; inbox/get/replies stay native-only.
**Next: board native exit evidence, then workboard in roadmap order.** Explicit
inbound correlation is a retained bridge contract; no notifier policy rewrite.

**W2.9c board native exit:** exact seven-command common-registry coverage
verifies actual schemas/types/native metadata. Independent family/ack removal
mutations fail; related source/native/tenant/audit/fallback/notifier/exit suites
pass count=20. Package race and final full Go/build/vet/scoped static plus all
gates pass: unchanged 227 packages, 141 imports/13 calls and structure 118.
Three dispatch benchmarks satisfy <50 us excluding real audit/journal I/O.
[Exit evidence](29-w29-exit-evidence.md) records paging/projection/routing,
shared-writer/fresh-reader ownership, mandatory audit and explicit inbound notifier
correlation as a retained bridge contract. Roster-owned constants/debt remain.
**Next: workboard, then OKR/storage/artifacts in roadmap order 3.** Broader
adapters, generated surfaces, runs.Start and W3-W5 remain open.

**W2.10a workboard read/projection foundation:** list/lanes/show business now
lives in app/workboard with context + typed input/output/error and the actual
read-only store port. Task Record embeds the actual store model plus computed
comment/link/attempt/failed counts, conditional criterion/proof fields and retry
limits; pointer fields preserve present zero/false values. Native task projection
wrapper forwards this view for remaining lifecycle/dispatch outputs. List retains
all selected filters/order; lanes group trimmed assignees with actual per-status
counts, case-insensitive lane sorting and unassigned-last label. Show retains
missing-ID/task errors; empty list/lane arrays remain present. Native wrappers keep
status/lenient string/limit/bool admission, registration/auth/audit/framing.
Three old/current native handlers match count=20 and shared task projection JSON
matches exactly (Go map int/float representations are compared at the wire).
Source/service/store/native parity/CLI and package race count=20, eight independent
mutations plus full Go/build/vet/scoped static and all gates pass: 228 packages,
unchanged 141 imports/13 calls, structure 119. **Next: remaining workboard lifecycle/
relations/dispatch/watch services, then typed binding/audit/exit and OKR.** Existing
native policy and lifecycle/dispatch behavior remain; no early debt removal.

**W2.10b workboard lifecycle-service foundation:** create plus claim/heartbeat/
comment/block/fail/unblock/complete/prove/seat/archive now live in app/workboard.
Lifecycle uses the actual narrow kernel facade plus seat setter; typed inputs/
outputs preserve task projection, creation idempotence and retry-decision fields.
Unused legacy retry projection helper is removed. Native wrappers retain lenient
argument/status/seat admission, explicit/generated
correlation, unknown-task envelope and 90-second prove timeout. Prove receives the
original caller context; seat retains its store clock. Actual kernel transition/
retry/idempotence/correlation fixtures and all eleven facade input/error paths pass.
Eleven old/current native handlers x three inputs match count=20 with generated
root/nested IDs and bounded lifecycle clocks normalized. Source/service/store/CLI
and package race count=20, eight independent mutations plus full Go/build/vet/
scoped static and all gates pass: unchanged 228 packages, 141 imports/13 calls,
structure 119. **Next: relation/maintenance and dispatch/watch services, then
workboard typed binding/audit/exit and OKR.** Kernel transition/journaling mechanisms,
native policy and helper error classification retain their existing behavior.

**W2.10c workboard relation/maintenance foundation:** link/policy/depend/reclaim/
sweep now live in app/workboard.Relations with context + typed input/output/error
over the actual kernel facade. Native wrappers retain explicit/generated correlation,
lenient arguments, clear/max-attempt validation, depends_on/on alias, stale duration
default, sweep actor default/1000 cap and original unknown-task error envelope.
Typed outputs retain task projection, actual reclamation counts, echoed stale
milliseconds and present-empty sweep arrays. Five facade input/cause paths and
real kernel link/policy/dependency-cycle/reclaim/sweep fixtures pass count=20.
Five original/current native handlers x three inputs match count=20 with generated
root/nested IDs and bounded lifecycle clocks normalized. Source/service/store/CLI
and package race count=20, eight mutations plus full Go/build/vet/scoped static and
all gates pass: unchanged 228 packages, 141 imports/13 calls, structure 119.
**Next: workboard dispatch/watch services, then typed binding/audit/exit and OKR.**
Underlying kernel transition/journaling and native policy retain existing behavior.

**W2.10d workboard watch-service foundation:** snapshot/run selection/event fold
and dependency projection now live in app/workboard.Watch with context + typed
input/output/error and actual store/journal reader ports. Native wrappers retain
lenient ID/run/limit admission, 50 default/200 cap, registration/auth/read-only and
single snapshot framing. Typed rows preserve seq/time/kind/subject/empty correlation,
payload absent/null/empty-object distinctions, stable chronological newest-tail
limits, subject-or-run filtering and nil empty events. Run selection retains
explicit -> claim -> latest attempt/link timestamp and tie precedence. Dependency
projection retains required ID/status and positive optional timestamps. Existing
Range and dependency errors remain best-effort for this move; a behavior repair
requires its own proof. Original/current native watch matches exactly count=20;
source/service/store/CLI/package-race count=20 and eight independent mutations plus
full Go/build/vet/scoped static/all gates pass: unchanged 228 packages, 141 imports/
13 calls, structure 119. Old run/fold/dependency view helpers are removed.
**Next: dispatch admission/background execution services, then workboard typed
binding/audit/error repairs/exit and OKR.** Despite historical comments, watch is
currently a unary snapshot; this move introduces no new stream.

**W2.10e workboard dispatch-admission foundation:** admission, dependency/agent
checks, correlation/claim/run link, requested publication and background launch
now live in typed app/workboard.Dispatch over actual store/host ports and a
selected roster projection/callback. The app layer has no runtime/roster import.
Native lenient string admission, error envelope and fresh dispatch correlation
remain. Agent/assignee and reason trimming, retired/paused/managed hints, exact
claim -> link -> publish -> launch order, generated/explicit intent and mutation
causes retain the old contract. Original/current denied native admissions and
intent match exactly count=20 with authentic paused/retired roster states;
accepted launch uses an owned callback fixture. Eight independent mutations,
workboard service/store/CLI/focused native tests and workboard package-race
count=20, complete controlplane race count=1 and full gates pass: unchanged
228 packages/141 imports/13 calls, structure 119. The actual background runner
and execution-profile bridge remain in controlplane for the next extraction.
Unused task-projection/response shims are removed once their last caller moves.
**Next: background execution service, then typed workboard binding/audit,
measured read-error repairs/exit and OKR.**

**W2.10f workboard background-execution foundation:** seat selection/context
axes, degradation comments, run settlement, task retry reclaim/recursion and
proof/review ownership checks now live in typed app/workboard.Execution. Actual
store/seat/kernel ports, selected context bridge and execution/publication
callbacks keep runtime/roster out of the app package. Native bridge still binds
the complete agent profile, wake source/subject/reason, cost ceiling, model/tool
setters, agent retry policy and warden execution-profile policy/backend checks.
Seat-over-agent isolation and model/tool precedence, missing/unavailable-seat
fallbacks, same-correlation task retry, current-claim ownership, proof-error review
fallback, best-effort settlement and Unicode 240/300-byte summaries are retained.
Original/current actual store/mock-provider execution matches count=20 across
review, reader/missing/unavailable seat, failure, task retry, proof and fallback;
only generated record IDs/timestamps are normalized, model/tool requests and
remaining final task fields match. Eight independent execution mutations fail;
workboard/service/store/CLI/focused native/workboard race count=20, complete
controlplane race count=1 and full gates pass: unchanged 228 packages/141 imports/
13 calls, structure 119. Native projection/context bridge remains intentionally;
all-surface runs.Start convergence is still open.
**Next: twenty-one typed workboard native bindings/mandatory mutation audit,
separately proved watch read-error repair and native exit, then OKR.**

**W2.10g workboard typed native binding:** twenty-one typed operations now own
native registration/metadata, replacing the old socket wrappers and registration.
All seventeen mutations require app audit before store, seat, claim, publication
or background effects; four reads remain unaudited unary snapshots. Actual
closed-journal tests cover every mutation independently of production metadata,
and actual tenant requests deny every command without changing primary state.
Request DTOs retain lenient strings/numbers/lists, policy maximum presence/null
and kernel empty-policy normalization, strict booleans, seat validation, limits,
dependency alias and the prove 90-second timeout over the admitted context.
Default lifecycle/domain correlation now matches the host-owned audit operation;
explicit inbound correlation remains the existing native bridge contract.
Dispatch still generates its own fresh run correlation, returned with claim/link;
owned asynchronous execution/review is tested against the actual mock provider.
Twenty-one original/current handlers x three inputs match count=20, normalizing
only generated IDs, bounded clocks and the common generic failure-code boundary.
Native audit/tenant/shape/admission/default and explicit identity/positive dispatch,
service/store/CLI suites pass count=20, workboard race count=20, complete
controlplane race count=1, eight audit-bypass mutations and full Go/build/vet/
scoped static/architecture/dead/deps/doc/changelog/structure/format gates pass.
Unchanged 228 packages/141 imports/13 calls, generated kernel structure 119.
Shared legacy OKR correlation/integer and seat list helpers remain for their
unmigrated callers; CLI comparison evidence points to actual app sources.
Watch Range/dependency errors intentionally remain best-effort until their
separately proved repair. **Next: watch read-error repair, then native exit/OKR.**
**Delivery checkpoint:** W2.10g is local. The current session cannot write Git
metadata and GitHub authentication is unavailable; #701 background-execution CI
was last observed pending before this restriction, so its merge is unverified.

**W2.10h workboard watch read-error repair:** an owned actual JSONL journal with
an invalid complete line returned success with zero/partial events; a failing
dependency port also returned a successful partial snapshot. Both defects were
reproduced before editing. Watch now returns the original Range/dependency cause
and zero output; the native app binding emits one terminal error frame with no
result/event/provider effect. Actual empty/partial journal regressions and the
exact dependency sentinel fail against the original service, pass three verifier
runs count=20, and independently fail when either error gate is suppressed.
Successful snapshot run/filter/order/payload/limit/dependency shapes remain intact;
existing successful-view tests no longer inject an intentionally ignored cause.
Focused native/service/store/CLI suites and workboard race pass count=20, complete
controlplane race count=1 and full Go/build/vet/scoped static/all gates pass.
Unchanged 228 packages/141 imports/13 calls, generated kernel structure 119.
This is an explicit behavior repair after the service move and typed binding.
**Next: native exit evidence, then OKR. Delivery remains local alongside W2.10g.**

**W2.10i workboard native exit:** exact twenty-one-operation aggregate common
registry coverage now guards actual types/schemas, AppOwned metadata, primary/
read-only/unary scope and unknown-input compatibility. Removing the family or
dispatch entry independently fails the exit regression. [Exit evidence](30-w210-exit-evidence.md)
records native compatibility, all seventeen mutation audit gates, four reads,
tenant denial, default/explicit/dispatch identity boundaries, owned execution and
separately proved watch failures. Sixty mutation proofs cover the native wave.
Focused source/service/store/native/audit/identity/tenant/watch/exit/CLI count=20,
workboard race count=20, complete controlplane race count=1 and final full gates
pass. No-audit-I/O framework benchmark: 5932 / 5636 / 6374 ns/op (<50 us).
Unchanged 228 packages/141 imports/13 calls, structure 119; shared OKR/seat helpers
and the selected native runtime context bridge remain for actual callers.
**Next: OKR, then storage/artifacts. Broader migration remains open.**
Native workboard completion is local, with G/H/I code/docs patches preserved as
separate reviewable deliveries; protected main delivery remains unverified.

**W2.11a OKR read/projection foundation:** list/show and live objective progress
projection now live in typed app/okr.Service over actual store reader and kernel
Rollup ports. Durable objective JSON, required progress/percent/achieved/key-result
count, filters, list order/count/empty arrays and show errors retain their native
wire contract. Cached objective status is preserved independently from live task
rollup; actual linked-task completion/unlink fixtures verify changing progress.
Native wrappers keep lenient status/tenant/limit and strict boolean admission,
primary-only registration/read-only policy, existing error envelopes and framing.
Five old writer wrappers use the same typed projection through the remaining
view bridge. All seven original/current native responses x three inputs match
count=20 with generated ID/bounded-clock normalization. Eight independent filter/
projection/empty-array mutations fail; actual service/store/runtime/CLI/native
focused and package-race suites pass count=20, complete controlplane race count=1
and full Go/build/vet/scoped static/all gates pass. Official structure writer
adds the package: 229 placed packages, unchanged 141 imports/13 calls, 120 generated
kernel packages. **Next: five lifecycle services, then OKR typed binding/audit/exit.**
Local code/docs delta is separate from the frozen workboard G/H/I deliveries.

**W2.11b OKR lifecycle foundation:** create/key-result/link/unlink/archive now
live in typed app/okr.Lifecycle over the actual kernel mutation facade and app
projection service. Native wrappers retain required title/ID admission, lenient
strings/integer target, explicit/generated correlation, auth/registration and
unknown-objective error envelopes. Runtime/store transitions, domain journaling,
live workboard rollup and original error causes remain. Link returns the actual
mutation result before cached achievement recomputation; the app projects that
returned snapshot rather than refetching a different durable status. Actual kernel
fixtures verify all six domain events, link/unlink/archiving and this distinction.
Seven original/current native responses x three inputs match count=20; eight
independent input/method/cause/projection mutations fail. Related source/service/
store/runtime/CLI/native and package-race suites pass count=20, complete
controlplane race count=1 and full Go/build/vet/scoped static/all gates pass.
Unchanged 229 packages/141 imports/13 calls, generated kernel structure 120.
Unused objective view/response bridges are removed after their final callers move.
**Next: seven typed native OKR bindings/mandatory mutation audit, then exit.**
Local source/docs delta remains separate from frozen read/projection and workboard
deliveries; Git write/GitHub authentication restrictions still prevent delivery.

**W2.11c OKR typed native binding:** seven app specs replace native wrappers and
registration. Five mutations require app audit before objective/key-result/link/
archive or rollup effects; two reads remain unaudited unary snapshots. Actual
closed-journal tests enumerate every mutation independently of production flags;
actual tenant socket clients reject all seven commands without primary changes.
Request DTOs retain lenient trimmed/nonstring fields and integer target/default
limit admission, unknown status filters and strict booleans. Default domain events
join one host-owned audit span; explicit inbound correlation remains the native
bridge contract. Domain result/error and returned-snapshot semantics retain parity.
Seven original/current handlers x three inputs match count=20 with generated IDs,
bounded clocks, generic failure-code and strict boolean schema-message boundaries.
Eight independent audit/target/identity/error mutations fail. Source/service/store/
runtime/CLI/native/audit/tenant/admission/identity and package-race count=20,
complete controlplane race count=1 and full Go/build/vet/scoped static/all gates
pass. 229 packages/140 imports/13 calls, official structure 120.
The official archcheck updater removes the paid controlplane-to-okr import
allowance (141 -> 140); call debt remains 13. Old OKR wrappers/registration
plus last-caller correlation/integer helpers are
removed; seat's shared string-list helper remains. **Next: OKR native exit.**
C source/docs delta remains a local delivery after frozen A/B and workboard slices.

**W2.11d OKR native exit:** exact seven-operation aggregate common registry now
guards input/output types/schemas, AppOwned metadata, primary/read-only/unary
scope and legacy unknown-input compatibility. Family/archive removal mutations
independently fail the exit regression. [Exit evidence](31-w211-exit-evidence.md)
records live-versus-cached rollup, facade-returned snapshot, five audit gates,
two reads, tenant denial and default/explicit identity/admission boundaries.
Twenty-six independent mutation proofs cover the native wave. Focused source/
service/store/runtime/native/audit/tenant/identity/admission/exit/CLI and package
race count=20, complete controlplane race count=1 and final full gates pass.
No-audit-I/O benchmark: 9757 / 9743 / 12903 ns/op (<50 us); 229 placed packages,
140 import/13 call exceptions and 120 generated kernel packages. Dedicated app
package documentation is generated correctly by the official structure writer.
**Next: storage/artifacts in roadmap order 3; broader migration stays open.**
Native OKR completion is local, with A/B/C/D source/docs preserved separately;
protected main delivery remains blocked by Git write/GitHub access restrictions.

**W2.12a storage inventory foundation:** typed app/storage owns home directory
aggregation, root-file grouping, labels, descending-byte/name-tie order and disk
probe projection. Actual ReadDir/usage ports and the injected selected-base disk
probe keep filesystem access in the native host; context/input/output/error are
transport-independent. Native read-only/primary-only registration, unknown args
and unary framing remain. Existing diagnostic best-effort ReadDir/entry/walk/probe
behavior stays explicit for the move; empty dirs remain null, zero/false fields
stay required and available zero disk values stay present via typed pointers.
Original/current actual owned-home native responses match exactly count=20 across
no/success/zero-free/zero-total/failed disk probes. Eight independent aggregation,
ordering/label/probe mutations fail. Related source/service/CLI/native/package
race count=20, complete controlplane race count=1 and full Go/build/vet/scoped
static/all gates pass. Official structure writer: 230 placed packages, unchanged
140 imports/13 calls, 121 generated kernel packages. **Next: artifact services,
then typed storage/artifact binding/audit and exit.** Local delivery is pending.

**W2.12b artifact service foundation:** typed app/artifacts owns get/list/delete/
collect over actual blob/index ports and injected clock. Native validation,
availability order, day/string and dry-run defaults, auth/audit/framing remain.
Typed records preserve required empty metadata/candidate fields, base64 bytes,
sentinel messages/original causes, empty arrays and dry-run-only candidates.
Actual index fixtures retain strict cutoff equality, unknown-time exclusion and
shared-blob ownership until the final reference is deleted. Four-handler x five-
input native parity passes count=20 with generated IDs/bounded cutoff clocks;
eight ref/size/filter/empty/default/cutoff/bytes/delete mutations fail. Service/
store/source/CLI/native/package race count=20, complete controlplane race count=1
and final full gates pass: 231 packages/139 imports/13 calls, official structure
122. The official updater removes the paid controlplane-to-artifact allowance
(140 -> 139 imports); call debt remains 13. Existing collection policy remains.
**Next: five typed storage/artifact native bindings and mandatory delete/collect
audit, then separately proved cleanup/reference repairs if confirmed and exit.**
Local code/docs delivery remains pending.

**W2.12c storage/artifact native binding:** five app specs replace wrapper/
registration business entry points; three reads remain unaudited unary responses,
while delete and collect require app audit before metadata/blob effects (including
collection dry-run admission). Owned actual closed-journal proofs showed legacy
success and deletion; permanent regressions now preserve index and blob bytes.
Actual tenant clients reject all five commands. Mutation audit is one correlated
invoked/completed span without duplicate native audit. DTOs retain absent/null
strict strings, raw ref/ID (no silent trim), exact filter strings, day number/digit
strings and dry-run default/boolean/exact false-or-0 string behavior. Availability
precedence and native result/error shapes remain. Storage exact five-probe parity
and four artifact handlers x five-input parity pass count=20; eight audit/raw-ref/
day/dry-run/presence mutations fail. Source/service/store/CLI/native/audit/tenant/
admission and package race count=20, complete controlplane race count=1 and final
full gates pass. Unchanged 231 packages/139 imports/13 calls, official structure
122. Actual filesystem/probe/blob/index bridges remain; old wrappers/registration
are gone. **Next: separately measured collection cleanup/reference repairs if
confirmed, then native storage/artifact exit.** Local delivery is pending.

**W2.12d artifact metadata-removal repair:** actual owned nonempty-directory
failure fixtures proved Delete returned nil after forgetting metadata and deleting
the blob; Collect counted that failure as reclaimed, and native delete returned
success. Index.Delete now removes metadata while holding index ownership, returns
the wrapped original non-ENOENT cause before forgetting the entry or touching its
blob, and keeps already-absent metadata idempotent. Collect's existing API counts
only successful deletions and retains failed candidates; it still reports partial
counts rather than a new batch error. Native typed delete returns one error frame
with unchanged entry/blob. Permanent actual core/native regressions are red on
old code, pass three verifiers count=20, and fail independently for ignored removal
and incorrect ENOENT handling. Related service/store/source/CLI/native/package
race count=20, complete controlplane race count=1 and full gates pass: unchanged
231 packages/139 imports/13 calls, official structure 122. This narrow repair does
not certify blob-GC failure handling, every writer race or journal raw_ref leases;
those stay explicit follow-ups under modules/artifacts. **Next: native exit.**
Local source/docs delivery remains pending.

**W2.12e storage/artifact native exit:** exact five-operation common-registry
coverage guards actual schemas/types, AppOwned metadata, primary/read-only/unary
scope and legacy unknown-input admission. Removing storage or artifact collect
independently fails the exit regression. [Exit evidence](32-w212-exit-evidence.md)
records diagnostic best-effort storage, strict raw/presence/default artifact
admission, required empty fields, dry-run and dedup/cutoff semantics, mandatory
audit/tenant isolation and separately proved metadata-removal failure repair.
Twenty-eight independent native-wave mutations pass their failure gates. Focused
service/store/source/CLI/native/audit/tenant/admission/exit and package race pass
count=20, complete controlplane race count=1 and final full gates pass. No-audit-
I/O framework benchmark: 6811 / 8893 / 7451 ns/op (<50 us); unchanged 231 packages/
139 imports/13 calls, official structure 122. **Next: schedule, then standing/
workflow/pulse/autonomy in roadmap order 4.** Core batch error redesign, remaining
writer/blob cleanup races and journal raw_ref leases stay explicit module work.
Native completion is local; A/B/C/D/E code/docs remain separate protected delivery
candidates while Git metadata/GitHub authentication restrictions persist.

**W2.13a schedule read/projection foundation:** typed app/schedule owns list,
system-task catalog, forecast and common entry execution metadata. Actual cadence
reader/clock ports and selected last-firing/runnable/warning callbacks keep native
validation and journal fold integration unchanged. Required empty/zero/false entry
fields, last-firing presence (including empty reason/zero timestamp), blocked/ready
and warning annotations, raw JSON payload and target/authority/identity/LLM
contracts retain their wire shapes. Forecast not-found remains only found:false;
found responses preserve empty mode/false enabled and empty forecast/count fields.
Native ID/numeric count/default/floor/cap admission, auth/read-only registration
and unary framing stay in wrappers. Add/edit retain a projection bridge; dead
metadata shims are removed. Existing best-effort firing annotation errors remain
explicit for the move. Three-handler x seven-input native parity and eight target/
agent projection branches pass exactly count=20; eight annotation/shape/forecast/
metadata mutations fail. Source/service/cadence/CLI/native and package race
count=20, complete controlplane race count=1 and final full gates pass: 232
packages/139 imports/13 calls, official structure 123. **Next: schedule admission/
lifecycle/firing services, then typed native binding/audit/exit.** Local delivery
remains separate from frozen earlier-domain code/docs patches.

**W2.13b schedule remove/run/enable lifecycle:** typed app/schedule now owns
validation-before-effect and success-only operator publication over the actual
cadence store. Run validates a found target before marking it due; enable validates
only when resuming, while pause does not validate. Missing/false results and
original store/validation causes are retained. The selected native bridge publishes
the existing schedule.enable payload with its host correlation only after a
successful update. Required ID and bool/string enabled admission, auth, registration
and unary framing remain native; this slice does not start the cadence engine.
Three-handler x four-target x ten-input response/store/action parity passes count=20;
eight validation/effect/cause/publication/action/ID mutations fail. Actual cadence,
source/service/native/CLI and package race count=20, complete controlplane race
count=1 and full gates pass. Structure remains 123; archcheck 232 packages/139
imports/13 calls. **Next: add/edit admission, target validation and firing services,
then native binding/audit/exit.** Delivery is local; Git metadata is read-only.

**W2.13c schedule runnable/frequency rules:** app/schedule owns live runnable
validation, agent tool policy and frequency warnings. Selected agent/workflow/tool
and managed-agent-message ports preserve native lookup order, untrimmed lookup
references, retired-before-paused and managed checks, tool re-read, normalized
allow/deny with deny precedence/default allow, known system tasks and exact
messages. Warning mode exclusions, task-before-guardian-before-intent precedence
and strict recommended/8-hour/15-minute thresholds are unchanged. Native wrappers
now only construct the selected host or forward; unused errString is removed
after its last caller moved. Add/edit share the moved tool
policy pending their own extraction. Actual selected-runtime parity across nine
agent states, five targets and four bindings (180 validations and 8,640 warning
combinations) passes count=20; ten gate/policy/mode/threshold mutations fail.
Related service/source/native/CLI and package race count=20, complete controlplane
race count=1/full gates pass. Structure 123 and archcheck 232/139/13 remain.
**Next: add/edit admission and firing services, then native binding/audit/exit.**
This slice and its separate source/docs patch remain local.

**W2.13d schedule add target admission:** typed AddTargetInput/AddTarget move
pre-cadence target/agent admission into app/schedule. Selected workflow-name port
retains canonical workflow names; agent references canonicalize to the returned
slug before the second tool-policy lookup. Intent remains the empty target constant;
explicit target/reference override order, conflicting-field errors, default task
text, present-null versus absent workflow/tool payload, ignored intent payload,
retired/paused/managed checks and deny/allow policy retain their original order.
Native argStrings admission remains first, target admission precedes numeric
cadence parsing, and five cadence create/bind/rollback/refetch paths remain native
for the next separate move. Exact response and persisted-entry parity across 36
native inputs passes count=20 (only generated IDs and bounded two-second clock
fields normalized); eleven canonicalization/precedence/presence/lookup/gate/policy/
zero-output mutations fail. Source/service/cadence/native/CLI/package race count=20,
complete controlplane race count=1/full gates pass. Structure 123 and archcheck
232 packages/139 imports/13 calls remain. **Next: cadence creation, edit admission
and firing services, then native binding/audit/exit.** Separate local delivery.

**W2.13e schedule cadence creation/binding:** typed CreationStore/CreateCadence/
CreateInput and injected clock own the five cadence add variants, agent then target
binding, best-effort compensation and final refresh/projection. Native selected-mode
and strict numeric/timezone parsing retain their original order after target
admission; interval/cooldown lower-bound messages move into Create without changing
native results. Fractional-second/integer truncation, continuous core minimum clamp,
operator source, injected creation clock, four binding false/error messages, original
creation/binding causes and zero failure outputs are pinned. Actual-store five-mode
x four-target coverage verifies durable target/payload/model normalization after
refetch; a missing refetch retains the legacy local binding projection. Binding
failure tests cover successful/failed compensation without changing best-effort
semantics (store atomicity remains separate work). Forty-input old native response/
store-entry parity passes count=20, normalizing only generated IDs/bounded clocks;
eleven compensation/cause/refetch/fallback/time/bounds/routing/minimum mutations
fail. Source/service/cadence/native/CLI/package race count=20, complete controlplane
race count=1/full gates pass. Structure 123 and archcheck 232 packages/139 imports/
13 calls remain. **Next: edit admission and firing services, then native binding/
audit/exit.** Separate local delivery; no cadence engine is started by this slice.

**W2.13f schedule edit target admission:** typed EditTargetInput/EditTarget move
current/requested target and agent validation into app/schedule before unchanged
cadence preflight and field/target/reschedule mutations. The last-used tool-policy
forwarding shim is removed. Required-ID and missing-ID
updated:false short circuit remain native before strict target-string decoding;
agent decoding stays lenient with separate presence. Absent/explicit empty intent
target, inherited target/agent, clearing agent, requested-agent canonicalization,
system-task payload/agent rejection, target/reference precedence/conflicts and
agent-before-target-before-tool-policy-before-payload errors retain their order.
Workflow reference remains uncanonicalized during preflight because the later
native second lookup is deliberately retained. Existing inherited-agent tool
checks remain policy-only (no extra paused/retired gate); unrequested workflow/tool
payload remains ignored. Exact response/store-state parity for 42 inputs x four
starting targets passes count=20 (generated IDs/bounded next-run clocks only),
including missing/invalid IDs and retained late-error partial updates. Eleven
presence/inheritance/canonicalization/gate/lookup/policy/payload mutations fail.
Source/service/cadence/native/CLI/package race count=20, complete controlplane race
count=1/full gates pass. Structure 123, archcheck 232 packages/139 imports/13 calls.
**Next: edit cadence/mutation services and firing integration, then native binding/
audit/exit.** Separate local delivery; edit atomicity remains separate work.

**W2.13g schedule edit cadence preflight:** app/schedule ValidateEditCadence
owns timezone/future/minimum/window/day checks over typed CadenceNumber parse
results (value/presence/original cause) and EditCadence. Native numeric decoding
remains a codec; errors are captured without throwing before domain selection.
Timezone validation still runs first with its lenient string cast and trimmed zone,
then once/continuous/window/daily/interval selection retains original parse/error
priority and ignored unselected causes. Float-to-int/duration truncation, future
strictness, interval/cooldown minimum, minute/day bounds and window ordering are
unchanged. Existing numeric-type coverage now calls the app validator through the
native codec; the old business helper is removed. Native missing-ID/target preflight
still precedes cadence input construction, while late strict timezone decoding and
field/target/reschedule writes remain native for the next move. Exact response and
store-state parity for 49 inputs x four starting targets passes count=20 (generated
IDs/bounded next-run clocks only); twelve timezone/variant/cause/minimum/bounds/
presence mutations fail. Source/service/cadence/native/CLI/package race count=20,
complete controlplane race count=1/full gates pass. Structure 123, archcheck 232
packages/139 imports/13 calls. **Next: edit mutation service and firing integration,
then native binding/audit/exit.** Separate local delivery; late-error partial update
and atomicity semantics remain unchanged.

**W2.13h schedule edit mutation service:** typed EditingStore/EditingHost,
Begin/EditState and Apply/EditInput/EditOutput own the initial live read and found-
only clock snapshot, cadence preflight, intent/model/agent setter order, second
workflow/tool lookup and payload serialization, target binding, late strict timezone
cause, five reschedule variants and final projection. Native handler retains required
ID, strict target strings and lenient optional strings/presence decoding; missing ID
still returns only updated:false before any further decode. Original intent/target/
reschedule causes propagate, model/agent errors and false setter results retain
legacy best-effort handling, and late errors retain already-applied fields/targets.
Workflow canonicalization occurs at its second lookup. Final missing refetch still
projects the zero entry with updated:true, while found refetch preserves normalized
payload/model/agent fields. The view/merge forwarding shims are removed after their
last caller moves. Actual-store five-cadence x four-target and phase/false/cause/
missing/partial-update tests pass; exact old response/store-state parity for 49 inputs
x four starting targets passes count=20 (generated IDs/bounded clocks only).
Thirteen clock/preflight/cause/ignored-error/routing/lookup/late-timezone/canonical-
bind/result/refetch/reschedule/payload mutations fail. Source/service/cadence/native/
CLI/package race count=20, complete controlplane race count=1/full gates pass.
Structure 123, archcheck 232 packages/139 imports/13 calls. **Next: firing integration,
then typed native binding/audit/exit.** Separate local delivery; edit atomicity and
best-effort failure repairs remain separate work. No engine is started by editing.

**W2.13i schedule firing journal views:** typed FiringJournal/FiringRun snapshot
ports and Fires/Stats/Latest own history filtering/projection, aggregate counts and
list last-firing annotations. The measured native surface includes ten commands:
eight schedule management/read commands plus tenant-routed read-only fires/stats.
Selected kernel journal and the shared collectRuns fold remain actual host ports;
native strict filter strings, lenient limit/default/cursor/since decoding, pre-route
cutoff anchor and tenant lookup stay in wrappers. Service owns floor-one/cap-1000,
ID/status/case-insensitive intent/cutoff filtering before sort/limit, same-ms sequence
ordering, strict cursor/last-row boundary, outcome precedence/duration/spend/preview,
required empty/zero/false row fields and optional nonempty runbook. Payload/classifier
helpers move unchanged; malformed/legacy payload degrades to zero fields. Stats
retain terminal rate, empty-reason fallback, named schedule cardinality, required
empty map and original window value. Latest retains first same-ms firing and skips
missing schedule IDs; list retains existing best-effort annotation errors. Run-fold
or range errors propagate original causes and zero outputs. Exact three-handler x
27-input owned native response parity passes count=20; socket primary/owner-routed/
tenant-token reads prove selected journal/run joins and foreign denial. Actual owned
journal sequence/tie, required/optional/empty shape, filters/page/cap/status/duration/
error tests and nineteen mutations pass. Source/service/cadence/journal/native/CLI/
package race count=20, complete controlplane race count=1/full gates pass. Structure
123, archcheck 232 packages/139 imports/13 calls. **Next: ten-command native binding/
audit/exit.** Cadence resident run execution belongs to W2.2/W4, not this read-view
wave. Separate local delivery; no engine or external provider is started here.

**W2.13j schedule typed native binding/audit:** ten unary app operation specs
now own native schedule dispatch. Five reads remain unaudited; add/rm/run/enable/
edit require mandatory audit before cadence or provider effects. Eight primary-only
commands and tenant-owned/routed fires/stats derive native metadata from app specs.
Typed raw request fields preserve absence/null, strict strings/numbers, lenient
optional edit strings/enable forms/limit/since, unknown args, selected cadence parse
order and missing-edit early return. Explicit input schema defers per-command
validation; explicit record/list/edit output schemas derive Record fields with raw
JSON payload supported, retaining required payload/updated and optional presence.
Five old native wrapper files and old registration/decode/write helpers are removed;
selected host bridges remain. Numeric source tests are repointed to serialized app
request codecs. Enable operator actions now join the host-owned audit correlation;
raw corr/correlation_id cannot select that identity. Actual closed-journal proofs
block all five mutations, five reads retain unary/no audit, eight-command tenant
denial preserves primary cadence/provider, and existing tenant firing join tests
stay green. Exact old handler versus new dispatch response/store-state parity for
ten commands x thirteen inputs passes count=20, normalizing generated IDs/bounded
clock and generic framework error-code fields only. Twelve audit/scope/decode/form/
correlation/schema/read-only/aggregate mutations fail. Source/service/cadence/journal/
native/CLI/package race count=20, complete controlplane race count=1/full gates pass.
Structure 123, archcheck 232 packages/139 imports/13 calls. **Next: schedule native
exit evidence, then standing/workflow/pulse/autonomy in order 4.** Separate local
delivery; resident run execution and wider trigger/module work remain W2.2/W4/W3.

**W2.13k schedule native exit:** exact ten-spec common registry and metadata,
primary/tenant policies, unary framing, typed schemas and legacy input compatibility
are guarded by TestScheduleNativeMetadataComesFromTenTypedAppSpecs. Removing the
family or tenant firing command independently fails exit coverage. Source/read/
create/edit/firing services and old native wrappers are already moved/deleted;
selected actual host bridges remain. [Exit evidence](33-w213-exit-evidence.md)
records 117 valid wave mutations, native parity/source/owned store/journal/audit/
tenant/schema evidence and preserved failure/partial-update boundaries. Framework
no-I/O dispatch benchmark is 7674/6024/6569 ns/op, 7610/7607/7609 B and 89 allocations,
below 50 us; construction/audit/journal/whole-operation latency is excluded. Final
focused exit/source/package race/CLI reruns count=20 and architecture/static/dead/
dependency/official structure checks pass; unchanged production full Go/build/vet
and complete controlplane race evidence is retained from W2.13j. 232 packages/139
imports/13 calls, 123 generated kernel packages. Resident runs.Start, W3 modules,
W4 triggers and W5 surfaces remain open; no deployment/publication is claimed.
**Next: standing, workflow, pulse and autonomy in order 4.** Local protected delivery
remains pending; source/docs patches stay separate.

**W2.14a standing CRUD/projection foundation:** typed app/standing reader,
actual runtime-writer facade and selected agent/message ports own List/Add/Edit/
SetEnabled/Remove, wire projection, agent validation and frequency warnings. Core
Order embedding keeps required id/name/enabled/triggers/initiative/timestamps and
legacy optional omissions; list adds blocked/ready target and exact cron/event
warnings, counts enabled orders independently of blocked state, and returns [].
Add validates agent before facade persistence without normalizing the stored raw
agent. Edit preserves patch presence, trim-only agent change, initiative/briefing
fields, float-to-int assure/cooldown conversion and updated:false missing result.
Resume validates the found agent; pause skips that gate. Original writer causes
and zero failure outputs are retained. Actual runtime facade lifecycle publications
standing.created/updated/removed remain in order, with core ID/timestamp/enabled
ownership and store rollback behavior untouched. Native ID/order JSON/strict patch
field decoding and registration stay in wrappers. Exact five-handler x fifteen-
input old/new response/store-state parity passes count=20 with generated IDs and
2-second bounded timestamps. Existing non-bool enabled accessor inputs remain
false (the legacy string-form comment was not treated as authoritative behavior).
Twelve list/annotation/count/warning/gate/pause/patch/missing/cause mutations fail;
source/service/core/CLI/native and package race count=20, complete controlplane
race count=1/full gates pass. 233 packages/139 imports/13 calls, official structure
124. **Next: standing why/fire services, then typed binding/audit/exit.** Separate
local delivery; resident execution and broader trigger/module work remain open.

**W2.14b standing why/fire services:** typed Observations/Journal/LifeEvent and
Firing/callback ports own history projection and manual dispatch admission. Why
retains standing.* kind-prefix matching (including future/custom kinds), exact
payload id match, malformed/foreign/nonstring-id skips, journal order, six required
row fields including zero sequence/timestamp/empty identity values, and events:null
when empty. Existing ignored Range errors retain collected partial rows without
an error; this best-effort read policy is explicit and remains separate repair work.
Fire retains callback availability before order lookup, missing order fired:false
with request ID, then agent validation before dispatch; callback result and request
identity are retained. The last validateStandingAgent forwarding helper is removed.
Native required-ID decoding, public SetStandingFire injection and seven-command
registration stay until binding. Actual owned journal sequence/zero timestamp and
recording callback tests pass; exact two-handler x eight-input x three-callback-mode
old/new response/callback parity passes count=20. Nine prefix/id/empty/required-
field/range-policy/availability/validation/identity/result mutations fail. Source/
service/core/journal/native/CLI and package race count=20, complete controlplane
race count=1/full gates pass. 233 packages/139 imports/13 calls, structure 124.
**Next: seven-command typed native binding/audit/exit.** Separate local delivery;
resident runner/run pipeline and broader trigger/module work remain open.

**W2.14c standing typed native binding/audit:** seven explicit primary-only,
unary specs own two unaudited reads (list/why) and five mandatory-audit mutations
(add/edit/set_enabled/remove/fire). Explicit raw-field input schema and typed
RequestInput preserve absence/null/wrong types, required IDs without trimming,
order object re-encoding, strict upfront patch validation order, fractional number
conversion and observed non-bool enabled=false behavior. Derived output schemas
retain embedded order wire/optional omissions, required false/zero fields and
null empty history. Aggregate app/native registry is the only command binding;
old CRUD/why/fire handlers and registration are removed, public SetStandingFire
and selected service bridge remain. Callback is resolved at dispatch time.
Runtime facade lifecycle events retain their original empty correlation; app
invoked/completed/failed share an owned correlation and raw spoof is ignored.
No correlated-domain rewrite or resident execution convergence is claimed.
Actual closed journal blocks all five state/callback effects; actual tenant socket
rejects all seven without state/provider/callback effects. Two reads add no audit;
fire unavailable/declined settle exactly once with joined audit identity.
Exact old/native vs app response/state/callback parity (75 CRUD plus 48 why/fire
combinations) passes count=20, with generated IDs/two-second clock bounds and only
generic error-code boundary normalization. Twelve admission/metadata/decoder/
callback/aggregate mutations fail. Focused/source/native/CLI and package race
count=20, complete controlplane race count=1/full gates pass. Archcheck remains
233 packages/139 imports/13 calls, structure 124. Standing exit evidence follows;
legacy Why partial-error policy and wider runner/trigger/module work remain open.
Separate local delivery; Git metadata unchanged.

**W2.14d standing native exit:** exact seven-operation aggregate and complete
[exit evidence](34-w214-exit-evidence.md) close native management/history/fire
admission. Missing fire or why independently breaks the registry regression.
35 valid wave mutations; source/native/store/journal/runtime-facade/callback/
audit/tenant/parity evidence. Final native/CLI/package-race count=20 and static/
architecture/dead/dependency/official structure pass; unchanged c production full
Go/build/vet/controlplane race evidence retained. No-I/O framework benchmark
6355/6785/6483 ns/op (<50us). 233 packages/139 imports/13 calls, structure 124.
Runtime lifecycle correlation and Why best-effort errors retain existing policy;
resident runner, runs.Start, W3/W4/W5 and other adapters remain open. Separate
local delivery. **Next: workflow, pulse and autonomy in order 4.**

**W2.15a workflow read/projection foundation:** measured native surface is 13
primary-only commands: list/show/runs/templates reads, nine management/copilot/
execution mutations. Typed app/workflow reader/journal/template ports now own
light/full graph DTOs, list/show/template projections and latest-run fold. PrepareList
retains store read before native with_runs admission; journal scan remains opt-in
and once across all names, even an empty list. Native flag/reference/type/error
framing and existing registry remain until operation binding.
Light rows omit nodes/edges; full rows retain raw node Config JSON and edges
omitempty; required identity/false/count/timestamp fields and optional description/
trigger detail/last-run duration survive. Trigger detail precedence stays webhook,
interval, daily, subject, with core daily normalization. List preserves order,
enabled count, [] empty and only matching last-run annotations; template rows keep
all six required fields and built-in gallery, [] empty. Show trims lookup while
retaining raw missing-ref error identity and zero failure output. Latest fold keeps
newest journal started arc, terminal correlation matching, requested subject/nonempty
correlation gates, terminal-without-start skip, finish/start duration guards and
legacy best-effort partial Range error behavior. Original six fold tests move with
production rather than retaining adapter business.
Exact three-handler x fourteen-input native parity plus eight configs x light/full
canonical JSON projection parity passes count=20 without wire normalization.
Actual owned graph store/journal plus projection/opt-in/count/ref/gallery/partial-
error contracts pass; twelve valid mutations fail regression assertions. Service/
core/journal/native/CLI and related package race count=20, complete controlplane
race count=1/full gates pass. 234 packages/139 imports/13 calls, structure 125.
Only shared typed projection/JSON bridge remains for other native methods; save/
restore/enable/remove, run history, copilot, run/node/webhook/detached execution and
13-command binding/audit/exit follow. Wider run/trigger/module/surface work remains
open; separate local protected delivery.

**W2.15b workflow lifecycle foundation:** typed Lifecycle/Writer ports own
Save/Restore/SetEnabled/Remove through the actual runtime facade. Correlation,
posted workflow, reason, raw reference and bool pass unchanged to the four writer
methods; native wrappers still supply empty correlation and preserve JSON/type/
required-ref/reason/enable admission. Workflow bool/string enable transport really
accepts case-insensitive true or exact 1 (without trimming), unlike standing's
observed non-bool false path. Save/Restore return typed full graph and required
created (including false); enable returns light projection; remove keeps required
removed:false for unknown refs. Writer failures return zero output with original
cause. Enable ErrNotFound retains raw-reference unknown-workflow message and
sentinel classification; native custom error framing remains unchanged.
Actual runtime fixture proves save update preserves ID/created/paused enabled,
restore trusts posted checkpoint ID/created/enabled, remove deletes, and five
saved/updated/saved/restored/removed events retain actor and requested correlation.
Core validation remains at the facade/store; lifecycle publication remains best
effort, including a successful durable save after journal close. Mandatory app
operation admission and owned default correlation remain later binding work.
Exact four-handler x eighteen-input native response/store/lifecycle event parity
passes count=20, with only generated IDs and two-second clock bounds normalized.
Port argument/result/required fields/original causes plus actual owned facade/store/
journal regressions pass; twelve valid correlation/reason/ref/bool/presence/result/
classification/cause mutations fail. Service/core/journal/native/CLI and package
race count=20, complete controlplane race count=1/full gates pass. 234 packages/
139 imports/13 calls, structure125. Run-history, copilot and execution services,
then13-command binding/audit/exit follow; native decoder/registry and wider run/
trigger/module/generated-surface work remain open. Separate local delivery.

**W2.15c workflow run-history foundation:** typed History/RunsInput/RunsOutput,
RunRecord and NodeEvent own workflow_runs journal fold. PrepareRuns trims lookup,
keeps raw missing-ref identity and captures canonical workflow subject before
native limit decoding; its snapshot is retained even if the graph later changes.
Native required-ref/custom error framing, lenient numeric admission and registry
remain. Observed present non-number limits (including numeric strings) become
zero through argFloat64 presence, then service default20; no compatibility repair
is silently added. Pure service normalization defaults <=0 to20 and caps at100.
History filters exact subject/nonempty correlation and accepts only start/node/
completed/failed events. It reverses correlation first-seen order, not timestamp
order, and limits after fold; terminal/node-only arcs retain started_ms:0 and
node_events:null. Started metadata and time overwrite, without clearing existing
status/finish/error; success after failure retains previous error. Legacy malformed/
partial payload decoding survives. Node probes/missing IDs are skipped, missing OK
defaults true, explicit OK/handled:false survive, attempts appear only >1, and
nonempty metadata/snippets/executed remain optional. Only positive finish time is
emitted. Empty runs stays []; journal failure returns zero output and journal:
prefix, preserving the underlying cause through the service boundary.
Exact old/current native response parity over27 inputs and125 owned run arcs
passes count=20 without normalization. Owned actual zero-time journal, canonical
snapshot/limits/order/required zero-false-null/metadata/probe/decode/error contracts
pass;18 valid limit/snapshot/filter/presence/decoder/order/cause mutations fail.
Service/core/journal/native/CLI and package race count=20, complete controlplane
race count=1/full gates pass.234 packages/139 imports/13 calls, structure125.
Old native business fold/limits are removed; only required ref/limit framing stays.
Copilot/execution then13-command typed binding/audit/exit follow; wider run/trigger/
module/generated-surface work and separate local protected delivery remain open.

**W2.15d workflow copilot foundation:** typed Copilot/Designer/correlation ports,
DraftInput/RefineInput/Refinement and full CopilotOutput own draft/refine orchestration.
Posted nonnull graph wins without store lookup (including zero graph, whose
validation remains runtime-owned); absent/null resolves trimmed ref before identity
allocation/provider effects. Missing base keeps required-base sentinel and native
failMsg framing; missing ref keeps raw-reference unknown workflow identity.
PrepareRefine captures stored graph before later provider invocation. Native strict
instruction/description/name, posted JSON and optional-ref decode order stay.
Fresh correlation is allocated before each designer call; legacy background context
is bounded at3m and canceled on both success/error. Raw name/description/instruction,
selected base and correlation pass unchanged. Result uses full graph projection;
original causes return zero output, without exposing a failed correlation. No writer
port is present. Actual runtime with mock provider proves draft name override,
refine stored name, two distinct correlations and drafted mode/event joins, while
the stored graphs remain unchanged. Original runtime repair/validation/unsaved contracts
also pass count=20. Context/correlation convergence with operation audit remains
later binding work; this foundation retains the legacy background/fresh policy.
Exact two-handler x22-input native response/provider request/drafted event/unsaved
state parity passes count=20, normalizing only fresh correlation values and checking
the response/request/event joins before normalization.14 valid budget/identity/
cleanup/input/precedence/lookup/preflight/snapshot/cause/zero-output mutations fail.
Service/core/journal/native/CLI/runtime and package race count=20, complete
controlplane race count=1/full gates pass.234 packages/139 imports/13 calls,
structure125. Native copilot timeout/business selection and last unused projection bridge are removed; codec/registry
remain. Run/test-node/webhook/detached execution then13-command typed binding/
audit/exit follow; broader run/trigger/module/generated-surface work and local
protected delivery remain open.

**W2.15e workflow execution foundation:** typed ExecutionHost/Execution/Wake,
RunInput/RunOutput, NodeInput/NodeOutput and WebhookInput/WebhookOutput own manual
sync/async runs, single-node probes, authenticated reply/async webhook dispatch
and detached panic recovery. Selected runtime run/probe methods remain governed;
full seven-field wake conversion, stderr panic diagnostic and existing workflow.panic
publication stay host bridges. No execution logic remains in native wrappers.
Manual sync keeps raw ref/payload, fresh identity, manual wake and background15m;
async allocates identity before trimmed graph lookup, rejects unknown synchronously,
captures canonical name/payload, queues one detached job, returns required accepted/
async/workflow/correlation fields and retains15m detached execution. Observed
present non-bool async values remain false through native argBool presence semantics.
Sync requires executed/outputs even when null; pointer DTOs preserve variant omission.
Missing refs keep raw unknown message; other run failures retain cause and correlation
suffix, with zero service output. Detached failures remain journal-owned; panics
notify once after context cleanup with original wake/correlation/name/value.
Node probe retains posted graph/node/data/payload unchanged, fresh identity,
background3m, original cause/zero failure and required output:null/port:""/attempts:0.
Webhook invalid/blank/empty-secret/unknown/paused/non-webhook/bad-secret all refuse
uniformly before identity/run/detach; constant-time secret comparison remains.
Authenticated reply uses canonical name, webhook trigger provenance and2m budget,
required executed/outputs and prefixed correlation error. Async replies omit run
outputs and queue the same detached15m path. Legacy background/fresh identity policy
is explicit; caller-context/owned operation audit convergence remains later work.
Exact three-handler x28-input old/current response plus lifecycle/node/provenance
parity passes count=20. Only fresh identity and event duration_ms are normalized;
accepted jobs are awaited to terminal event before comparison. Owned transform/probe
runtime and original policy/provenance/node contracts plus actual native panic
containment remain green.24 valid budget/identity/cleanup/ref/payload/field presence/
node inputs/webhook gates/provenance/reply/panic/cause mutations fail. The wrong-kind
fixture was corrected to satisfy the secret gate, making its kind mutation independent.
Service/core/journal/native/CLI/runtime and related package race count=20, complete
controlplane race count=1/full gates pass.234 packages/139 imports/13 calls,
structure125. Native timeouts, manual orchestration, webhook gate and detached
business helper are removed; only codecs/custom error framing/manual registry and
selected host bridges remain. **Next:13-command typed binding/audit, caller context/
correlation refinement and exit.** Wider runs.Start/trigger/module/generated-surface
migration and separate local protected delivery remain open.

**W2.15f workflow typed native binding/audit:**13 primary-only unary specs now
own the full native family: list/show/runs/templates are unaudited reads; save/
restore/remove/set_enabled/run/draft/refine/webhook/test_node require durable audit
before actual graph/provider/probe/webhook/detached effects. Typed raw RequestInput
and explicit input schema retain absent/null/wrong types, required untrimmed refs,
posted JSON re-encoding, strict field/error order and ignored unknown args. Preserve
store read before with_runs flag; graph lookup before history limit; real workflow
enable bool/string forms (case-insensitive true/exact1/no trim), and observed
non-bool async=false plus nonnumber history limit=>default20. Refine posted nonnull
graph wins over ref; null graph uses ref. Probe workflow then node then data type
admission, and webhook malformed inputs retain uniform refusal.
Derived ordinary outputs and explicit full graph/show/save/copilot/template schemas
keep arbitrary raw node Config JSON, full required nodes, created:false, required
identity/count/false/empty fields and optional/variant omissions. Aggregate/native
metadata comes from these specs; five wrapper files, manual registration, common
native write helper, last unused argFlag helper and registry hook are removed. Original three boolean contracts move to the typed request decoder. Selected service/runtime/wake/
logging/publication bridges remain. Four reads add no operation audit; actual
closed journal blocks all nine mutations before graph/provider effects. Real
socket tenant credentials are denied all13 without graph/provider effects.
One owned invoked/completed audit span ignores raw correlation spoof, while runtime
lifecycle still keeps legacy empty correlation. Copilot/execution retain fresh
identity/background budgets. Caller context/domain correlation convergence remains
explicit later work, rather than being claimed by this binding.
All13 native commands pass combined old/native vs typed dispatcher parity count=20:
42 read+72 lifecycle+27 history+44 copilot+84 execution scenarios (269 total), plus
16 canonical graph projections. Store/provider/request/event/async outcome evidence
is retained. Normalize only established generated identity/bounded clock/event
lifetime/generic framework error-code boundaries.18 valid mutation proofs cover
nine mandatory-audit flags, read/primary/unknown-arg policy, bool/limit forms,
raw-config/required nodes/created schemas and aggregate inclusion.
Source/service/core/journal/native/CLI/runtime and related package race count=20,
complete controlplane race count=1/full gates pass.234 packages/139 imports/13
calls, structure125. **Next: caller context/correlation refinement and native exit**,
then pulse/autonomy in order4. Broader runs.Start/trigger/module/generated surfaces
and separate local protected delivery remain open.

**W2.15g workflow caller context/correlation refinement:** before proof on f
reproduced all nine direct mutation paths ignoring a pre-canceled caller, five
blocking designer/run/probe/reply paths ignoring cancellation/budget/values, and
fresh/empty domain identity split from the operation host. Permanent unchanged
proofs now pass count=20: canceled caller blocks writer/designer/run/detach and
fallback identity allocation; in-flight cancellation reaches all five blocking
ports; a10s parent budget/value survives existing15m/3m/2m ceilings.
Lifecycle checks caller state before writes and chooses host-owned correlation
before explicit service ID; direct context-free callers retain explicit/empty IDs.
Copilot/run/probe/authenticated webhook use owned identity when present, fresh
runtime fallback only without a host. Blocking work inherits caller cancellation,
values and earlier deadline; both return paths still cancel child timers.
Accepted manual/webhook jobs use context.WithoutCancel to preserve caller actor/
profile/provenance/identity values, remove caller cancellation/deadline and apply
the original independent15m ceiling. The unused public Detached entry is removed; direct tests cover the private
background path. Uniform webhook gates, codecs/schemas/policy/runtime execution
and response variants remain; caller-canceled service calls return their cause
before effects. Lifecycle writer failures/cancellation retain zero outputs.
Actual nine-command socket fixtures verify one owned invoked/completed audit span
joined to lifecycle/drafted/started/node events and returned correlation, ignoring
raw corr/correlation_id spoof. Actual direct dispatcher cancellation stops an
owned60s delay within the3s verifier bound, produces joined invoked/started/
workflow.failed/op.failed events and no provider call. No new transport-disconnect
or live-provider guarantee is claimed; supplied caller context is what is proved.
16 valid identity precedence/fallback/budget/detached value-cancellation/admission
mutations fail. Original direct fallback/ceilings/presence/gate/panic tests are
retained; legacy tests that expected discarded caller context now assert preserved
values. The f old/native identity parity is historical: these owned identity and
cancellation semantics are explicit approved-architecture changes.
Source/service/core/journal/native/CLI/runtime and related package race count=20,
complete controlplane race count=1/full gates pass.234 packages/139 imports/13
calls, structure125. **Next: workflow native exit**, then pulse/autonomy in order4.
Broader runs.Start/trigger/module/generated surfaces and local protected delivery
remain open.

**W2.15h workflow native exit:** exact13-operation aggregate and complete
[exit evidence](35-w215-exit-evidence.md) close native management/history/copilot/
execution admission. Missing webhook or test_node independently breaks the registry
regression.116 valid wave mutations; source/native/store/journal/designer/run/
probe/panic/audit/tenant/context/correlation evidence. Final native/CLI/package-race
count20 and static/architecture/dead/dependency/official structure pass; unchanged g
production full-check evidence retained after narrow unused-entry cleanup.
No-I/O framework7276/6486/8486 ns/op (<50us).234 packages/139 imports/13 calls,
structure125. **Next: pulse/autonomy in order4**; pulse has13 resident control
commands plus its subscribe/replay stream, so measure the complete surface before
migration. Broader runs.Start/W3/W4/W5, other adapters and local protected delivery
remain open.

**W2.16a pulse resident controls foundation:** complete native surface measured
as14 commands:13 resident controls and pulse_subscribe replay/live stream. Typed
app/pulse Controller/Observers/Controls and setting ports own status/asks, pause/
resume/beat/ask resolve/cadence/dial/quiet/flush/watch/probe/unwatch services.
Public SetPulse/SetPulseObservers boot contracts retain source-compatible aliases;
selected factory and existing best-effort settings load/save stay host bridges.
Native controller/observer availability before argument decode, strict strings,
bool/number/string forms, validation/framing and registration remain until binding.
Status preserves dynamic opaque snapshot and overrides enabled:true on the same
map; disabled status is only enabled:false. Disabled asks stays []; active nil asks
remains null. Pause/resume/resolve retain required false flags, flush/unwatch zero
counts and original error messages/zero service failure output. Resolve preserves
raw issue key and approve; native approval strings are case-sensitive true/exact1.
Cadence keeps duration truncation, live applied milliseconds and persists applied
(not requested) duration string after live change. Dial/quiet persist applied
values; persistence remains best effort. Observer availability is independent of
controller, raw watch path/pct and probe name survive; pct bounds (0,100), strings.Fields
command splitting rather than shell parsing, unchanged false-result errors and
required added/observer outputs remain. Service interfaces do not execute a probe
command or start an engine. Actual owned unstarted core engine proves cadence clamp,
dial normalization, pause/resume/quiet and queued beat without provider calls.
Exact13 handlers x4 availability modes x16 inputs (832 combinations), response/
controller-observer argument/call/actual settings-reload parity passes count=20.
18 valid status/empty/required false-zero/conversion/applied persistence/ref/bool/
observer/split/failure mutations fail. Service/source/native/CLI/journal and package
race count=20, complete controlplane race count=1/full gates pass.235 packages/
139 imports/13 calls, structure126. Subscribe/replay service and14-command binding/
audit/exit follow; resident trigger execution/W3/W4/W5 and other transports remain
open. Separate local protected delivery.

**W2.16b pulse replay foundation:** typed Journal/ReplayInput/ReplayOutput/emitter
port owns historical replay, with no net/socket dependency. Preserve journal order,
all active seq/time/subject/kind/correlation filters as AND, inclusive lower and
exclusive upper boundaries, nil kind map=no filter versus empty map=reject all,
exact correlation and bus wildcard grammar. Events reach emitter as original
pointers/full frames. LastWritten advances only after a successful write, stays
last journal-order emitted seq (not re-sorted/maximized), and remains-1 when none
were sent, including past-head queries. Original range/write cause and partial
checkpoint survive. Positive rate retains duration conversion, first-event immediate,
success-anchored wait, time.Since adjustment and cancellation-aware timer stop;
unlimited replay retains legacy emitter-owned cancellation semantics.
Actual owned journal proves sequence0 is durable and half-open bounds preserve it.
Native old/current replay framing/last seq/error parity across14 modes passes
count=20 without normalization; socket-free filter/order/sentinel/partial-failure/
rate/context and actual-journal tests pass.12 valid cutoff/match/kind/correlation/
sentinel/checkpoint/write/rate/cancellation mutations fail, including a bounded
stalled-wait verifier. Service/source/native/CLI/journal and related package race
count=20, complete controlplane race count=1/full gates pass.235 packages/139
imports/13 calls, structure126. Native replayHistorical is now only selected
journal/emitter framing. Native subscriber still owns argument admission, pre-replay
subscription, live durable dedup/ephemeral pass-through, drop notices, client close
watcher and server-context stop; those move next.14-command binding/audit/exit and
broader trigger/module/generated-surface work/local delivery remain open.

**W2.16c pulse live stream foundation:** typed Stream/Subscription/DropTicker,
selected journal/subscriber/emitter/clientGone ports own subscribe-before-replay,
4096 buffer, bounded replay/live transition, durable checkpoint dedup, kind/
correlation live filters, drop notices and lifetime cleanup. Subscriber is canceled
on every acquired path; bounded replay opens neither watcher nor ticker. Replay
errors retain wrapped cause/pulse replay prefix, subscription errors stay original,
and closed subscription keeps its native message. Clean caller/client close or live
write failure end silently with no terminal result, retaining event-only native wire.
Socket decode/clear-read-deadline and lazy500ms timeout-versus-disconnect watcher
stay adapter mechanics; watcher begins only after successful replay and only live.
Selected bus adapter converts actual channel/drop atomic/cancel ownership. Default
ticker remains1s and stops on return. Durable seq<=last replay checkpoint is skipped;
ephemeral hash-empty events pass even seq0; nil kind map differs from empty, exact
correlation stays. Synthetic per-stream notice is emitted without user filters,
seq0/hash-empty/empty correlation, agezt actor and exact dropped delta/total JSON;
no new notice for unchanged counter, counter progress advances after sample.
Original Replay service is reused; its last native forwarding helper is removed.
Owned actual bus/journal bounded replay and deterministic port tests cover order,
dedup/ephemeral/filter/no-match, errors/writes/cleanup, drop growth/no-growth and
client/server stop.17-mode old/current native full framing parity (bounded/filters/
admission/idle close/replay-to-live) passes count=20 without normalization.16 valid
buffer/cleanup/replay/dedup/filter/drop/framing mutations fail, including a bounded
stalled replay-only verifier. A missing controlled ticker sample in the strengthened
drop fixture was corrected; it is not a product finding. Service/source/native/
CLI/journal and related package race count=20, complete controlplane race count=1/
full gates pass.235 packages/139 imports/13 calls, structure126.
**Next:14-command typed binding/audit with explicit event-only stream adapter
compatibility**, then exit. Wider resident trigger execution/module/generated
surfaces/other transports and separate local protected delivery remain open.

**W2.16d pulse resident typed binding:** thirteen resident controls now derive
native registration, schemas, read policy and primary-only aggregate tenancy from
app.ControlOperations; two reads are unaudited, eleven mutations require audit
before controller, observer or settings effects. Runtime controller/observer ports
are selected on each dispatch; public setters/aliases and best-effort applied-value
settings persistence remain. Raw argument presence, disabled-before-validation,
number/string cadence/min_pct, exact case-sensitive approve, raw strings, Fields
probe splitting, false/zero/null/opaque outputs remain measured native conventions.
Resident handlers, duplicate registry and unused response/availability helpers are
removed; controlplane retains composition/persistence and streaming adapter.
832 legacy/current native response/controller/observer/argument/settings cases pass
count=20. The unavailable-audit before proof fails against all eleven old handlers
(they still acted); unchanged after proof passes count=20 with zero effects. Permanent
actual closed-journal admission, paired invoked/completed/failed correlation, read
policy, output schema, dynamic setter selection, canceled admission, nineteen
argument/persistence cases and thirteen-command real tenant socket denial pass.
Twenty valid audit/presence/availability/argument/registry mutation regressions fail.
The source raw-argument ratchet caught the removed handlers dropping2 casts to0;
its quota is reduced to0 and an additional ratchet mutation fails.
Two temporary proof/harness import/helper dependencies were corrected; no product
finding is inferred from harness compilation failures. Source/service/resident native/
CLI/journal repeats and related race count=20, full controlplane race count=1 and all
Go gates pass.235 packages/139 import/13 call allowances remain; structure126.
**Next: pulse_subscribe typed StreamLive operation with explicit event-only native
adapter compatibility**, then W2.16 exit. Resident trigger execution/module/generated
surfaces/other transports and protected local delivery remain open.

**W2.16e pulse subscription typed binding:** pulse_subscribe is now a typed
primary-only aggregate read operation with canonical StreamLive metadata, independent
Event emission schema and object terminal {}. All fourteen pulse native commands
now derive auth/read/schema policy from app operations: three unaudited reads and
eleven mandatory mutations. SubscribeRequest preserves presence-sensitive strict
string/list/numeric validation order, trimmed pattern/kinds/correlation, blank/empty
kinds as no filter, numeric truncation, negative default bounds and positive-only rate.
Selected SubscribeHost prepares transport only after decoding; bounded replay starts
no watcher, and ClientGone remains lazy after successful replay. Old decoder/business
wrapper and manual stream registry are removed. Shared dispatchAppOperation keeps
canonical auth/routing/schema/cancellation/emission/audit ownership; unary native
framing remains in handleAppOperation. Explicit pulse native projection retains
StreamNone dispatch classification to avoid a second connection reader, the existing
500ms timeout-tolerant watcher, event-only bounded/clean/live-write termination and
cause/error messages. It emits no RespResult; canonical {} is available to future
transports. Native unlimited replay still delegates cancellation to socket writes;
positive rate waits/live lifetime retain caller context. **Explicit refinement:** an
already canceled caller is rejected by shared admission before replay; old bounded
handler still emitted an event. Unchanged before proof fails/after passes count=20;
permanent real native wire regression covers it. No running-replay rewrite is claimed.
28 old/current complete native socket frame modes pass count=20 without normalization.
Permanent default/presence/strict validation order, typed output/emission/admission,
selected port/prepare/cleanup and missing-stream tests; native14 metadata/projection
incompatibility, bounded event+EOF/no audit, single watcher timeout/stray byte/stop,
socket-owned unlimited emitter cancellation and14-command actual tenant denial pass.
22 valid codec/hooks/schema/audit/projection/watcher/emitter mutations fail; source
bytes restored. One obsolete import after extraction was removed before verification.
Source/service/native Pulse+AppHost/CLI/journal repeats and related race count=20,
full controlplane race count=1/all Go gates pass;235 packages139 imports13 calls,
structure126. **Next: W2.16 exit evidence**, then autonomy/order4. Wider resident run
execution/trigger unification/module/generated surfaces/other transports and protected
local main delivery remain open.

**W2.16f pulse native exit:** exact14-operation primary aggregate policy is
pinned: three reads/eleven mandatory mutations, thirteen unary resident commands
and canonical StreamLive subscription with explicit native event-only/single-reader
projection. Independent resident/subscription omission mutations fail; exact e
production bytes restored. Exit/native/service/CLI/race/static/architecture/dependency
gates pass; full native Pulse+AppHost20/whole controlplane race1/all Go gates passed
e. Wave91 valid mutation failures: a18,b12,c16,d21,e22,f2. Generic no-audit-I/O
framework benchmark6796/6995/6662 ns/op (all<50us),7607/7609/7608 B/op,89 allocs;
excludes journal/socket/per-event or resident/provider latency.235 packages139 import/
13 call allowances,28 SDK findings+one test seam,24 dependencies,structure126.
[Full exit evidence](36-w216-exit-evidence.md) maps ownership, native parity/framing,
mandatory audit/no effects, actual tenant denial, replay/drop cleanup and explicit
already-canceled admission behavior. **Next: autonomy in order4.** Wider resident
run/trigger/module/generated surfaces/other transports/protected main delivery stay
open; no resident process/provider deployment is certified.

**W2.17a autonomy feed foundation:** selected Journal.Tail port and explicit
FeedInput/FeedOutput own the recent-history fold, newest-first milestone selection,
clamped1–200 limit and2000-event scan. Default60 remains native codec policy; present
float64 limits truncate, wrong types/null/string retain default. Empty items stay [],
count/seq/time/kind/subject/category/title/correlation remain required even zero/empty.
Doctor/metadata/detail/payload helpers and the complete original private test file
move verbatim to app/autonomy (package/constant homes only), with no other source
callers. Named delegation-only/reactive exclusion, doctor phase/mode titles/fallback,
raw optional enrichment strings, filtered-but-untrimmed chain entries, present zero
numeric metadata and rune-based120 clipping preserve actual behavior. Original Tail
cause returns zero output. Selected actual journal and port tests cover fold/order/
window/limit/empty/error/doctor projection/malformed/Unicode boundaries. Native
adapter retains legacy limit codec, primary ReadOnly registry and socket framing;
row maps deliberately stay the original projection during this move-before-binding.
193 full native response/journal-head/provider parity cases (all doctor phases/modes,
operator actions/milestones/payload shapes/limits/closed journal) pass count=20 with
no normalization; source native and original relocated helper tests pass count=20.
18 valid window/order/limit/count/empty/error/presence/enrichment/selection/Unicode/
detail mutations fail and restore exact source. Initial doctor-default and Unicode
boundary test gaps were strengthened; surviving mutants were not counted as defects.
Source/service/native/CLI/journal repeats and related race20, full controlplane race1/
all Go gates pass;236 packages139 import13 call allowances,structure127. **Next:
typed native binding and explicit row DTO/schema**, then autonomy feed exit/order5.
Wider autonomous execution/run/trigger/module/generated-surface/other-transport work
and protected local main delivery remain open; no resident/provider execution starts.

**W2.17b autonomy feed typed native binding:** one primary-only aggregate
ReadOnly unary app operation derives native registration/input-output types/schema
and unknown-input compatibility. FeedRequest owns the exact default60/numeric-only
limit codec; absent/null/string/bool/object/array stay default, float truncation and
1–200 service clamping remain. FeedItem replaces opaque row maps with23 named fields:
seven required identity/category/title/correlation fields keep zero/empty values;
detail/twelve doctor strings/chain omit only legacy-empty values. Independent *int
metadata preserves absent versus present0, negative/fraction truncation and row/field
independence. Derived schema requires identity/count and retains optional numeric
presence. Original helper/doctor/detail logic stays unchanged. Native wrapper and
manual registry are removed; selected primary journal composition/shared dispatcher
and response framing remain adapters.193 exact original/current full native ingress
response/journal-head/provider parity cases pass count=20 without normalization.
Permanent real native numeric/wrong-type/default/unknown/aggregate read/no-audit/
schema/canceled admission, canonical owned factory/Tail admission/cause/metadata and
actual tenant socket denial tests pass. **Explicit refinement:** already-canceled
operation admission now fails before feed lookup/Tail; the old wrapper returned a
feed result. Unchanged before proof fails/after passes20, with native/canonical
regressions. Direct Feed.List/in-progress non-context Tail behavior is not rewritten.
23 valid required-field/count/numeric presence-independence/raw string/limit/read/
auth-tenancy/unknown/registry/cancel mutation regressions fail; exact source restored.
Source/service/native/CLI/journal repeats and related race20, full controlplane race1/
all Go gates pass;236 packages139 imports13 calls,structure127. **Next: autonomy
feed native exit evidence**, then order5 tool/toolforge/toolbox/MCP/market/plugin.
Wider autonomous execution/trigger/run/module/generated surfaces/other transports
and protected local main delivery remain open.

**W2.17c native numeric precision refinement:** the shared app operation
terminal bridge decoded typed output into map[string]any through float64, so the
native wire rounded int64 sequence/time9007199254740993 to9007199254740992. An actual
native socket proof reproduces it. The bridge now uses Decoder.UseNumber while
retaining the existing object envelope, preserving exact numeric lexemes through
serialization. Unchanged after proof passes20 at2^53+1 and maxint64; its permanent
AppHost regression fails when UseNumber is removed. This explicit correction applies
to all migrated native object replies, not only autonomy rows. Output schemas, domain
folding, auth/read/tenant policy and input/client codecs stay the existing contracts;
generated/client numeric representation remains later work. Source/native AppHost/
autonomy/CLI/journal repeats and related race20, complete controlplane race1/full Go/
static/architecture/dependency/docs/structure/secrets gates pass.236 packages139
imports13 calls,structure127. Original source bytes restored after one valid numeric
mutation; a18/b23/c1=42 valid wave mutations so far. **Next: autonomy feed native
exit**, then order5. Wider autonomous execution/trigger/run/module/generated surfaces/
other transports and protected local main delivery remain open.

**W2.17d autonomy feed native exit:** exact primary aggregate ReadOnly unary
operation/typed23-field schema, native limit codec and unaudited journal fold are
pinned. Operation omission and output-schema weakening independently fail; exact c
production bytes restored.193 final native parity cases20 plus source/AppHost/native/
CLI/service/journal race20/static/architecture/dependency exit checks pass; whole
controlplane race1/full Go gates passed c after shared numeric bridge correction.
Wave44 valid mutations: a18,b23,c1,d2. Generic no-audit-I/O framework benchmark
7098/7304/10239 ns/op (all<50us),7607/7608/7608 B/op,89/89/89 allocs; excludes socket/journal/
feed rendering and row validation latency.236 packages139 imports13 calls,28 SDK
findings+one test seam,24 dependencies,structure127. [Full exit evidence](37-w217-exit-evidence.md) maps ownership, doctor rendering/limits/typed presence,
actual unaudited/tenant admission, pre-canceled refinement and exact-number native
bridge correction. **Next: tool/toolforge/toolbox/MCP/market/plugin in order5.** Wider
autonomous execution/trigger/run/module/generated-client surfaces/other transports
and protected local main delivery remain open.

**W2.18a tool inventory foundation:** app/tools Inventory binds the selected
Tools reader and typed empty input/six-string row/output contract. List reads each
definition once and invokes no tool/provider, preserving empty tools:[]/count0 and
required blank description/effect/rollback notes. Rows use Definition().Name for
presentation/name sorting; registration key selects the original primary capability
with file write/http POST/homeassistant call_service representative probes. Rollback
read-only/reversible/compensable/irreversible/empty/future mappings remain. Duplicate
advertised names retain both entries and the legacy unspecified equal-name tie order;
no deterministic tie or declared-capability-policy rewrite is claimed in this move.
PrimaryCapability is shared with existing agent permission rendering, removing its
controlplane-owned probe dependency. Probe/rollback bodies and all permission logic
remain unchanged except the shared helper call. Native tool_list wrapper/primary
ReadOnly registration/framing stay until typed binding; roster governance stays its
later domain.50 exact original/current native inventory cases plus192 policy-view
comparisons (six effects/eight registration axes/four profile policies) pass count20
without normalization, with actual owned kernels/Edict/definitions and zero tool/
provider/journal-head effects. Permanent name-vs-key, required wire fields/empty,
fresh reader/definition counts, rollback/probe and duplicate retention tests pass20.
20 valid probe/sort/key/count/read-once/empty/rollback/required-field mutations fail;
a mutation harness edit mistakenly discarded its first replacement, was corrected
and excluded as a compile failure. Source/native permission/capability/CLI/app/tools/
Edict repeats and related race20, whole controlplane race1/all Go gates pass.
236 packages139 import13 call allowances,structure127. **Next: tool_list typed binding,
tool log/stat services/binding and native exit**, then toolforge/toolbox/MCP/market/
plugin. Wider run/tool invocation/module/trigger/generated surfaces/other transports
and protected local main delivery remain open.

**W2.18b tool inventory typed native binding:** tool_list now derives native
registration, primary-only aggregate unaudited unary policy and Go input/output/
row schemas from app/tools InventoryOperations. Selected host kernel supplies the
existing definition reader; unknown args retain the native no-argument convention.
Six row strings and tools/count remain required even blank/zero; [] empty output,
definition-name sorting/registration-key representative capabilities/rollback notes,
shared permission rendering and legacy duplicate/tie/classification semantics stay.
The native wrapper and manual registration are removed.50 exact original/current
complete native ingress inventory responses and192 unchanged permission comparisons
pass20 without normalization, with zero tool/provider/journal-head effects. Typed
schema/no-audit/unknown/canceled/tenant canonical admission and actual native empty/
aggregate read/no-audit/schema/canceled/tenant-denial tests pass. **Explicit refinement:**
already-canceled operation admission now rejects before factory/reader/definition
lookup; the old wrapper returned inventory. Unchanged before proof fails/after passes20,
with permanent canonical reader/factory counters and real native wire regression.
Direct Inventory.List/in-progress definition reads retain their previous context
behavior.16 valid row/root required-field/read/auth-tenancy/unknown/registry/schema/
native-cancel/evidence-path mutation regressions fail; exact production bytes restored. The CLI compare gate caught the removed wrapper evidence path; it now points to
app/tools/inventory.go, and reverting the path fails the exact gate. Source/
native inventory/permission/capability/AppHost/CLI/app-tools/Edict repeats and related
race20, full controlplane race1/all Go gates pass;236 packages139 imports13 calls,
structure127. **Next: tool_log/tool_stats services and tenant binding, then tool native
exit** before remaining toolforge/toolbox/MCP/market/plugin order5. Wider invocation/
run/module/trigger/generated surfaces/other transports and protected main delivery
remain open; no actual tool invocation/provider starts in the new fixtures.

**W2.18c tool journal observation foundation:** selected journal Reader and
explicit LogInput/LogOutput/StatsInput/StatsOutput own tool log/stat folds. The
existing platform/journalview engine still owns timestamp-sequence ordering, strict
cursor/limit and decode-before-cutoff mechanics. Invoked inputs/times join by
(correlation_id,call_id), including events before the result window; repeated invokes
keep the actual legacy last-observed entry. Denied/no-invoke/backward-clock spans
stay zero; not_executed calls retain true-only log metadata and no execution latency
samples. Tool/errors/slow filters, required zero/false/null provenance/matches, preview
whitespace/100-rune+ellipsis, malformed-result unknown bucket, error-message buckets/
empty-message fallback and integer averages/nearest-rank percentiles remain.
Decoders/previews move verbatim and their controlplane copies are removed. Shared
pure duration summary/percentile helpers plus original tests move to platform/
journalview; run duration/spend, plan and tool stats now use the single owner with
no shim or algorithm change. Native strict codecs, numeric coercion/default20/cap1000,
clock cutoff, tenant kernel selection/read registry and response framing remain
adapters for the next binding. Row/by-tool/duration maps preserve legacy projection
shape during extraction; typed schemas follow separately.120 exact old/current native
log-stat input/filter/cursor/error/closed-journal responses pass20 without normalization.
Port tests cover scoped pairing/window/cursor/skip/backward clock/provenance/preview/
malformed/empty/cause/statistics/last-observed semantics; owned journal and actual
primary/acme/other socket reads prove tenant selection, cross-tenant denial and no
audit/provider/journal-head changes.26 valid fold/filter/key/latency/cursor/window/
provenance/cause/preview/summary mutations fail, restoring exact source. Source log/
stats/audit/cursor/run/plan/AppHost/CLI/app-tools/journalview/journal repeats and related
race20, whole controlplane race1/full Go gates pass;236 packages139 imports13 calls,
structure127. **Next: typed log/stat tenant binding and explicit output schemas**, then
three-operation tool native exit and remaining order5. Broader invocation/run/module/
trigger/generated surfaces/other transports/protected main delivery stay open.

**W2.18d tool observations typed tenant binding:** tool_log/tool_stats derive
native registration/read/OwnTenant/CallerTenant/unary metadata and input/output schemas
from app/tools operations. Together with primary aggregate tool_list, the exact three
read policies are pinned. ObservationRequest owns strict errors-bool/tool-string
validation order, preserved untrimmed tool filter, numeric-only slow/since coercion,
default20/truncated/clamped1–1000 limit, cursor fallback and a selected/injectable clock.
Stats still ignores errors/slow/limit/cursor args; unknown args remain compatible.
LogItem has14 required zero/false/null metadata/identity fields plus true-only optional
not_executed; ToolSummary requires calls/errors and uses *int64 optional avg_ms so
present0 differs from unmeasurable absence. Stats roots and six latency fields remain
required; duration embedding uses the canonical platform summary type. Row/duration/
by-tool maps are removed from app observations. ProjectValues<T> exposes typed rows
through the same private journalview engine; existing map Project keeps exact stamp-
after-cutoff semantics. Paging/order/decode-before-window/cursor math is single-owned,
with typed/map parity and original projection/source tests. Native codecs/business
wrappers and manual log/stat entries are removed; selected caller journal, shared
dispatcher and terminal framing remain adapters.120 exact original/current complete
native ingress log-stat cases pass20 without normalization; canonical schema/codec/
clock/no-audit/factory-reader admission, typed/map window-cursor/cause and actual
primary/acme/other socket isolation/no-audit/source tests pass. **Explicit refinement:**
both pre-canceled operations now reject before factory/journal/clock entry; old
wrappers returned results. Unchanged before proof fails both/after passes20, with
canonical counters and real native regressions. Direct service/in-progress Range
cancellation semantics are not rewritten.34 valid required-field/optional-average/
not-executed/read/auth/registry/unknown/clock/limit/numeric/tenant-factory/cancel/
projection/schema mutations fail; exact source restored. Staticcheck requested a
direct conversion for identical map-output structs; projection/app tests revalidate
it20 and remaining gates pass. An expired old Temp formatting helper was replaced
by the equivalent current tracked/untracked gofmt gate; setup failure is not a product
finding. Related/source/native/run/
plan/AppHost/CLI/app-tools/journalview/journal repeats/race20, whole controlplane race1/
all Go gates pass;236 packages139 imports13 calls,structure127. **Next: three-operation
tool native exit**, then toolforge/toolbox/MCP/market/plugin order5. Broader invocation/
run/module/trigger/generated surfaces/other transports/protected main delivery remain.

**W2.18e three tool reads native exit:** exact primary aggregate tool_list and
OwnTenant/CallerTenant tool_log/tool_stats unaudited unary specs own native registry,
codecs and typed schemas. Removing each operation independently fails the policy
coverage gate; exact d production bytes restored. Final50 inventory native cases192
permission comparisons and120 log/stat native cases pass20; source/AppHost/CLI and
related race20/static/architecture/dependency/structure/2730-file formatting pass.
99 valid wave mutations; d whole controlplane race1/all Go/build/vet and revalidated
static cleanup remain the full-source boundary. Shared duration summary and typed/map
projection have one owner; legacy codecs/filter/tuple/window/presence/last-observed
semantics stay. Already-canceled admission now blocks lookup for all three (unchanged
before/after proofs); direct/in-progress service reads stay. Generic no-audit-I/O
dispatch6318/6730/8051 ns/op (<50us),236 packages139 imports13 calls,structure127.
[Exit evidence](38-w218-exit-evidence.md) records ownership, compatibility and limits.
**Next: toolforge two reads/six lifecycle mutations**, then toolbox/MCP/market/plugin
order5. Full invocation/run/module/trigger/generated surfaces/other transports and
protected main delivery remain open.

**W2.19a toolforge read foundation:** selected List/Get port and ForgeCatalog
move list/show shaping into app/tools; ForgeToolView moves verbatim apart from name
and is shared by the six retained lifecycle adapters. Store order, [] empty tools,
required counts/fields, active-only callable_as, light code omission/full show code,
optional schema/test timestamp and old float64 view conversion remain unchanged.
Lookup trims ref but unknown errors preserve the raw ref. Native strict ref codec,
primary read registry/terminal framing and lifecycle business adapters remain for
later binding/mutation slices; direct service context behavior stays.152 exact
original/current full native ingress list/show cases pass20 across four owned stores
and19 argument modes, including zero/future/quarantined/active/large-number records;
no store/journal/provider effects. Port tests pin snapshot freshness/order/empty/
required-optional presence/full-light/error/lookup and legacy numeric behavior;
actual primary socket reads/spoofed tenant/tenant-denial/no-audit tests pass.19 valid
mutations fail and source restores exactly; two unused-import harness failures were
repaired and excluded. App/source/native/runtime/CLI repeats and related race20,
whole controlplane race1/all Go/build/vet/static/architecture/dependency/docs/structure/
format/secrets gates pass;236 packages139 imports13 calls,structure127. **Next: two
read typed native bindings and row schemas**, then six lifecycle mutation operations
and toolforge exit before toolbox/MCP/market/plugin. Broader invocation/module/trigger/
surfaces/other transports/protected main delivery remain open.

**W2.19b toolforge typed primary reads:** two primary aggregate unaudited unary
operations derive list/show native registry, request codec and output schemas.
ForgeShowRequest preserves missing/blank required and explicit null/wrong-type string
errors, unknown args, trimmed lookup and raw unknown-ref messages; invalid ref never
looks up the service. Native wrappers and manual list/show entries are removed; six
lifecycle wrappers remain. ForgeItem has11 named fields (eight required zero/false/
empty identity/status/timestamps and three optional input_schema/tested_ms/callable_as);
ForgeDetail adds required code, including empty string. Typed list/detail replace row
maps; one typed projection also serves six remaining lifecycle adapters. Light rows
never include code, optional tested_ms omits exactly0 and callable_as is active-only;
store order, [] empty tools and required count/active_count remain. **Explicit numeric
correction:** old JSON-to-map float64 projection rounded2^53+1/maxint64 timestamps;
new int64 view preserves all three timestamp fields. Actual native unchanged before
fails both reads/after passes20; source/wire regressions and precision mutations guard
it. Shared typed view corrects number rendering for retained lifecycle replies too;
client float decoding is later work. Final152 native cases20:131 byte-exact and21
with only asserted timestamp lexeme corrections; no other normalization. **Explicit
admission refinement:** pre-canceled reads now reject before factory/store; unchanged
before fails both/after passes20 and canonical factory/List/Get counters remain0.
Direct/in-progress non-context store reads remain unchanged. Canonical required-field/
optional/codec/unknown/no-audit/tenant-denial tests and actual primary/tenant socket
fixtures pass;28 valid schema/policy/codec/presence/precision/registry mutations fail,
source restored exactly. One unused-import mutation setup failure was repaired and
excluded. App/store/runtime/native/AppHost/CLI repeats/related race20/whole controlplane
race1/all Go/build/vet/static/full gates pass;236 packages139 imports13 calls,structure127.
**Next: six lifecycle mutation services and bindings**, then toolforge exit and
remaining order5; full invocation/module/trigger/surfaces/other transports/protected
main delivery remain open.

**W2.19c toolforge six lifecycle service foundations:** app/tools ForgeLifecycle
uses a selected six-method kernel writer port for draft/edit/test/promote/quarantine/
remove. Typed mutation/test/remove outputs share ForgeItem; kernel/store still owns
validation, protected identity, test verdict, tested-code promotion, persistence and
scripttool.* publication. The edit callback's tokens move unchanged apart from Tool
input qualification: only nonempty description/language/schema and nonempty code
replace stored fields, whitespace-only code is still forwarded to store validation. Raw refs/sample/
reason, error-first/found precedence, wrapped not-found conversion and backend causes
remain. Service entry rejects canceled contexts before any writer; direct service
host correlation wins explicit fallback and sandbox test inherits its exact context.
Native wrappers retain original strict raw codecs/manual mutation registry/audit,
empty lifecycle correlation and background sandbox context until typed binding;
no native admission/identity change is claimed at this foundation.117 original/current
complete native ingress error/absent-remove cases pass20 byte-exact with unchanged
store rows; successful source and existing native round-trip/test-first suites pass20.
Port tests cover all six methods/typed fields/cause-zero/context/correlation/raw args/
four-field edits/protected fields. Actual owned kernel/store/journal fixture proves
six ordered correlated lifecycle events, untested promotion refusal, runner input
fallback and code never actually executed; real native sample forwarding and tenant
denial preserve state/journal/provider effects.42 valid mutable/protected/identity/
raw/context/cancel/cause/verdict/presence/native-sample mutations fail; exact source
restored. One unused-sample mutation setup failure was repaired and excluded. App/
store/runtime/native/AppHost/CLI repeats/related race20/whole controlplane race1/all
Go/build/vet/static/full gates pass;236 packages139 imports13 calls,structure127.
**Next: six typed native lifecycle bindings**, including canonical audit identity and
caller context, then toolforge exit/remaining order5. Runtime publication semantics,
full invocation/module/trigger/surfaces/other transports/protected delivery remain open.

**W2.19d toolforge six typed native mutations:** all eight toolforge commands now
derive native registration/primary aggregate/unary/read-vs-mandatory-write policy and
schemas from app operations. Six raw typed request codecs preserve absent versus null
tool semantics, exact args.tool JSON error prefix, strict ref-first validation, raw
ref/sample/reason and optional absent versus explicit null/wrong-type strings. Unknown
args remain compatible. Typed mutation/test/remove outputs retain required tool/ok/
output/removed roots and light view fields. The237-line native business/codec/registry
file and manual registration call are removed; shared host/framing/audit remain the
adapter, kernel/store lifecycle ownership remains unchanged.117 exact original/current
native error/absent-remove cases pass20, with one audit pair per ingress and unchanged
store rows; source/native successful lifecycle/sample/tenant suites pass20.
**Explicit admission/identity/context corrections:** closed journal previously allowed
all six real store writes and sandbox test; canceled caller previously allowed all
six writes/audit effects. Native binding now rejects before service factory/writer/
runner entry. Lifecycle events formerly had empty correlation separate from their
operation audit; now exactly one op.invoked/domain/op.completed trio shares a host
identity and ignores caller-poison correlation/tenant args. Test formerly dropped
caller values/deadline; now its runner inherits values/deadline/operation identity.
Unchanged actual native before proof fails19 subcases (six audit failure,six canceled,
six event identity,one runner context); unchanged after passes20, with persistent
store-file/memory/journal/provider/runner assertions. A blocking owned runner confirms
post-admission deadline returns an error without changing the test record and emits
one invoked/failed pair. Real sandbox execution remains runner-owned, not a live test.
Canonical specs/types/schema/root-presence/codec-order/null/unknown/factory-context/
mandatory begin/end/tenant/pre- and post-begin cancellation/settlement cause tests pass.
Settlement errors propagate after a writer has acted; this does not promise rollback.
Official archcheck -update removes the paid controlplane-to-toolforge adapter bypass;
no allowance is added. Full Go/build/vet/static passed before this stale ratchet gate;
archcheck and remaining gates resume on the tightened138-import baseline.
31 valid binding/policy/codec/forwarding/context/root/registry mutations fail; exact
bytes restored. Two unused-variable mutation setup failures were repaired and excluded.
App/store/runtime/native/AppHost/CLI repeats/related race20/whole controlplane race1/
all Go/build/vet/static/full gates pass;236 packages138 imports13 calls,structure127.
120 valid toolforge wave mutations so far. **Next: eight-operation native exit**, then
toolbox/MCP/market/plugin order5. Runtime publication semantics, broader invocation/
module/trigger/surfaces/other transports/protected main delivery remain open.

**W2.19e eight-operation toolforge native exit:** two primary aggregate unaudited
reads and six mandatory audited unary writes own registry/codecs/typed schemas. Eight
independent operation omissions and one full-code schema weakening fail; exact d
production bytes restored. Final152 read cases20:131 byte-exact+21 timestamp-only
corrections; final119 byte-exact native error/absent-remove cases20 add whitespace-code
validation/rollback and retain one audit pair per ingress. Source/AppHost/CLI/related
race20/static/architecture/dependency/structure/2743-file formatting pass; d whole
controlplane race1/all Go/build/vet and official one-edge ratchet reduction remain
full-source evidence.129 valid wave mutations. Native audit/cancel gates, one owned
operation/domain/terminal identity and inherited runner context/deadline are anchored
by19 before failures/unchanged after20 plus blocking deadline/test-record proof.
Settlement errors do not imply rollback. Generic no-audit-I/O framework8587/8775/6950
ns/op (<50us),236 packages138 imports13 calls,structure127.
[Exit evidence](39-w219-exit-evidence.md) records ownership, compatibility and limits.
**Next: toolbox detect/outdated/install**, then MCP/market/plugin order5. Full invocation/
run/module/trigger/generated surfaces/other transports/protected delivery remain open.

**Protected delivery checkpoint after W2.19e:** the authenticated GitHub connection
reads PR #701 as open/mergeable, branch arch/w2-dispatch at the exact local HEAD
#ae063ff145b78991fec15c192e028cb3be713622 (main base e5379d802db6b5ccd67614382146064f6731ab51).
Its old-head CI run37357717652 is successful. A verified55-pair package contains188
live files and35 removals, excluding both unrelated security-report deletions;
normalized UTF-8 payload, file hashes and Git modes are retained in Temp. GitHub
create_tree was denied with “MCP tool call requires approval, but approval policy
is never”. No tree, commit or branch update was created. CLI HTTP401 is separate
from the working authenticated read connection; the write-approval gate and local
read-only Git metadata block this package's delivery. Later write permission must
publish the exact scoped snapshot, rerun final-head CI and use a normal protected
merge; the old-head CI is not evidence for unpublished changes. Local migration
continues with toolbox while this delivery boundary remains open.

**W2.20a toolbox read foundations:** selected ToolboxReader Detect/Outdated ports
and ToolboxReads move inventory pass-through and outdated-key folding into app/tools.
Core toolbox retains catalog, host lookups, bounded version/package-manager probes and
best-effort outcomes; native host adapter binds its existing functions. Inventory
keeps exact fields/optional strings/nil-versus-empty slices/counts/order. Outdated
keeps all map keys (including false values), raw names, [] empty output/required count,
fresh host snapshots and the legacy unspecified map iteration order. Caller context
and canceled direct-service best-effort behavior are preserved; no native admission
change is claimed. Native read registry/policy/no-arg compatibility/framing/map adapter
remain until typed binding. Streaming install and shared structToMap/mustJSONRaw users
in market/ACP remain separate scopes.22 byte-exact original/current complete native
ingress read cases pass20 in owned empty-PATH/working-directory fixtures, with no
journal/provider effects. Port probes cover populated/empty/optional/required/fresh/
raw-key/false-key/context/canceled snapshots; actual tenant socket denial preserves
journal/provider state.10 valid context/snapshot/key/count/empty/presence/cancel
mutations fail and exact source restores; an unused-name mutation setup failure was
repaired and excluded. Source/native/AppHost/CLI/related race20/whole controlplane
race1/all Go/build/vet/static/full gates pass;236 packages138 imports13 calls,structure127.
Controlled no-installed-tool fixtures and mock data do not certify live manager/probe
behavior or host installation. **Next: two typed native read bindings**, then install
service/streaming binding and three-operation toolbox exit before MCP/market/plugin.
Broader invocation/module/trigger/surfaces/other transports and protected delivery remain.

**W2.20b toolbox typed native reads:** detect/outdated derive native registry,
primary aggregate/unaudited/unary policy and input/output schemas from two app specs.
No-argument typed inputs keep unknown args; host inventory retains five required roots,
nine-field tool rows (five required/ four optional), nil-empty slices/counts/raw optional
strings and booleans. Outdated has required array/count, all-key membership including
false, raw names and legacy unspecified map order. Two native read wrappers and manual
entries are removed; selected default host reader and shared framing remain adapters.
Streaming install stays on its existing native path; generic JSON helpers remain for
market/ACP.22 byte-exact complete original/current ingress cases pass20 in owned empty
PATH/CWD fixtures, with unchanged journal/provider head/counters. Canonical specs/types/
schema/root-row required/optional/nil-empty/unknown/no-audit/factory-context/tenant tests
pass; actual tenant socket denial persists. **Explicit admission refinement:** the
old native wrappers returned results for both pre-canceled reads; shared app admission
now rejects before factory/host query. Unchanged actual before fails both/after passes20,
with canonical factory/host counters and permanent native regressions. Direct service/
already-running best-effort probe behavior remains unchanged.24 valid policy/context/
registry/required-optional schema mutations fail; exact source restored. Source/native/
AppHost/CLI/related race20/whole controlplane race1/all Go/build/vet/static/full gates
pass;236 packages138 imports13 calls,structure127.34 toolbox wave mutations so far.
Controlled no-installed-tool native fixtures plus populated mock snapshots do not
certify live manager/probe behavior or installation. **Next: streaming install service
and typed event binding**, then three-op toolbox exit before MCP/market/plugin; full
invocation/module/trigger/surfaces/other transports and protected delivery remain open.

**W2.20c toolbox install service foundation:** selected ToolboxInstaller plus
publisher/emitter callbacks move sequential install orchestration, outcome classes,
progress payload/envelope and typed terminal summary into app/tools. Core retains
catalog/manager resolution, bounded sandbox HelperEnv execution and version probing;
native host adapter binds existing Install. Empty input rejects before publication;
requested event precedes per-item context check, each attempt publishes outcome before
progress, and duplicate/raw requested names remain. OK wins over Skipped; summary uses
requested names while journal/progress keep reported Tool. All seven lifecycle payload
keys, per-tool InstallResult optional fields, zero transient event identity/sequence/
time, and installed/failed/skipped [] arrays remain. New service API stops later effects
on publisher/emitter errors and returns the original cause without partial summary;
an already attempted install is not rolled back. Native compatibility callbacks retain
best-effort publication/write behavior and empty domain identity until typed stream
binding; names codec/manual StreamEvents registration/framing remain. Pre-canceled
native calls still publish requested before failure, and cancellation during an attempt
still reports that outcome/progress before the next-item check.18 byte-exact full native
old/current frame-sequence cases20 use only asserted unknown catalog names in owned
empty PATH/CWD; no host manager install is reached. Port probes cover mixed success/
skip/failure/overlap/raw duplicates/order/context/cancel/failure boundaries, emitted
metadata/payload and typed arrays. Actual native unknown-tool progress/summary/journal
order, raw names codec and tenant-denial regressions pass20.27 valid outcome/order/
payload/context/array/error/native-codec mutations fail; two initially invisible port
fixtures (background context and equal reported/requested fail names) were strengthened,
not counted as product defects. Source/native/AppHost/CLI/related race20/whole
controlplane race1/all Go/build/vet/static/full gates pass;236 packages138 imports13 calls,
structure127.61 toolbox wave mutations so far. **Next: typed StreamEvents install
binding with canonical audit/identity/error handling**, then three-op exit/order5.
No real manager installation/live sandbox claim; wider invocation/module/trigger/
surfaces/other transports/protected delivery remain open.

**W2.20d toolbox typed StreamEvents install:** all three toolbox commands derive
native registry/primary aggregate/read-write/stream metadata and schemas from app.
Install has one mandatory audited StreamEvents spec with raw Names input, three
required terminal arrays and checked event.Event emission. Its names codec preserves
array-only/drop-nonstring-or-empty/raw-whitespace/duplicate semantics and exact empty
error; unknown args remain. Native install wrapper/manual entry/best-effort publisher
and old private codec are removed; original codec regression moves to the new raw
boundary. Host read/install adapters and shared market/ACP JSON helpers remain.
Kernel event.WireSchema now owns the former Pulse envelope literal byte-exactly;
Pulse consumes it. Install specializes that envelope with required payload and typed
InstallResult schema (required tool/ok plus optional fields), keeping transient event
format/zero metadata unchanged.15 normal original/current unknown-catalog native frame
cases pass20 byte-exact. The three former canceled parity cases are superseded by
an explicit no-publication admission correction rather than normalized into parity.
**Four explicit native corrections:** unavailable audit formerly yielded progress/
result; canceled admission formerly wrote requested/audit records; a broken stream
formerly processed all three unknown attempts; lifecycle records formerly had empty
correlation. Shared app now stops before factory/installer/requested publication on
admission failure, propagates emitter errors after the completed attempt and binds
requested/outcome/terminal events to one host operation identity. Four unchanged
actual native before tests fail/after pass20; broken writer gives one attempted
outcome/one failed terminal audit. A journal closed after the first progress now
stops later progress on publication failure and propagates the error; already applied
attempts are not rolled back. Real installation remains untested; native fixtures use
asserted unknown catalog names only. Canonical specs/types/payload/root schemas/codec/
unknown/context/mandatory begin/end/tenant/pre-post-begin cancellation/emitter failure/
settlement cause tests and actual native stream/tenant/source regressions pass20.
18 valid policy/codec/context/required summary-payload/registry/domain correlation/
publisher mutations fail in the final single-writer run; exact source restores and
host matches a reproducible binding snapshot. A missing empty-summary assertion was
strengthened. Two accidentally overlapping mutation rounds were discarded and rerun
serially; their outcomes are not certification.79 toolbox wave mutations so far.
Source/native/AppHost/CLI/related race20/Pulse-event race20/whole controlplane race1/
all Go/build/vet/static/full gates pass;236 packages138 imports13 calls,structure127.
**Next: three-operation toolbox native exit**, then MCP/market/plugin order5. Generic
HTTP/SSE surfaces, live managers, full invocation/module/trigger/surfaces/other transports
and protected main delivery remain open.

**W2.20e toolbox three-operation native exit:** two primary aggregate unaudited
reads and one mandatory audited StreamEvents install own registry/codecs/schemas.
Three independent operation omissions and one required-progress-payload weakening
fail; exact d production restored. Final22 read cases20 and15 normal install frame
cases20 remain byte-exact in owned empty-PATH/CWD/unknown-catalog fixtures. Source/
AppHost/CLI/related race20/Pulse-event race20/static/architecture/dependency/structure/
2757-file formatting pass; d whole controlplane race1/all Go/build/vet remain full-source
evidence.83 valid toolbox wave mutations; overlapped mutation rounds were discarded
and the final serial snapshot-verified run alone counts. Shared event envelope remains
byte-exact for Pulse; install specializes its typed payload. Native audit/cancel/
broken-stream/domain identity corrections have four unchanged before failures/after20;
post-admission publication failure stops later progress without rollback. Generic
no-audit-I/O framework9937/8613/9670 ns/op (<50us),236 packages138 imports13 calls,
structure127. [Exit evidence](40-w220-exit-evidence.md) records ownership and limits.
**Next: MCP one list read/five lifecycle operations**, then market/plugin order5.
Live host installation, HTTP/SSE surfaces, wider invocation/run/module/trigger/
generated surfaces/other transports and protected delivery remain open.

**W2.21a MCP catalog/redacted-view foundation:** selected registration List and
MCPAttached ports move list aggregation/shared registration view into app/tools.
The original view body moves verbatim apart from owner/port names, with no native
view shim; five retained lifecycle adapters use the same app view. Env/header values
never appear in output, only nonempty sorted key-name arrays. Raw optional command/
args/URL/description/lazy/tool_allow fields and required identity/enabled/zero time
fields retain their original JSON projection. Transport uses nonempty raw URL;
attached false is required and tool_count is present for live0/negative counts but
absent for detached rows. Legacy float64 timestamp conversion remains until typed
row binding/explicit precision proof. Store order, [] empty servers/count and initial
attached_count include orphan entries; one initial attachment snapshot plus one fresh
snapshot per row retains the original read cadence rather than introducing a coherent
snapshot rewrite. Direct/canceled best-effort read behavior remains. Native no-arg/
primary read registry/framing and lifecycle codecs/wrappers remain until binding.
54 byte-exact complete original/current native list cases pass20 across six actual
owned kernels/store/mock stdio-http peer states and nine arg shapes. Store/journal/
attachment/provider state stays fixed and reads never Call a remote tool. Port tests
pin redaction/key order/raw fields/required zero/optional/transport/live0-detached/
reader ownership/orphan/fresh-row cadence and legacy number behavior; actual tenant
socket denial preserves store/journal/provider state.16 valid redaction/key/transport/
status/count/fresh-view mutations fail; exact source restored. An unused-attachment
mutation setup failure was repaired and excluded. Source/native/runtime/mock-peer/
AppHost/CLI/related race20/whole controlplane race1/all Go/build/vet/static/full gates
pass;236 packages138 imports13 calls,structure127. **Next: typed native list binding
and redacted row schema**, then five lifecycle services/bindings/native exit before
market/plugin. No live peer/process/network/secret claim; broader invocation/module/
trigger/surfaces/other transports/protected delivery remain open.

**W2.21b MCP typed native list/redacted rows:** one primary aggregate unaudited
unary app spec derives native registry/empty input/output schema. MCPServerView names
16 public fields: seven required id/name/enabled/created_ms/updated_ms/transport/
attached, nine optional command/args/url/description/lazy/tool_allow/env_keys/header_keys/
tool_count. Env/header value fields are absent from the model and declaration. *int
ToolCount keeps live0/negative present versus detached absence. Raw optional strings,
empty collection omission and sorted nonempty key-name arrays remain; Args/ToolAllow/
key-name lists and count pointers are owned per view. List has three required roots,
[] empty servers/store order/initial orphan-inclusive attached count plus fresh per-row
attachment snapshots. Row maps and JSON-to-map projection are removed. All five retained
lifecycle adapters share the same typed view; native list wrapper/manual entry removed.
**Explicit precision correction:** old projection rounded2^53+1/maxint64 timestamps;
new int64 projection preserves native created_ms/updated_ms. Actual before fails/
unchanged after20 and permanent wire/source/ownership tests pin it.63 exact comparison
cases20:54 byte-exact normal store/mock-peer cases plus9 with only asserted timestamp
lexeme changes; no other normalization. **Explicit admission refinement:** old native
list returned results for a canceled caller; app now rejects before factory/registration/
attachment reads. Actual unchanged before fails/after20 plus canonical counters prove
it. Direct service/in-progress non-context attachment reads retain their behavior.
Canonical metadata/types/schema/three roots/seven row fields/optional/null/pointer/
unknown/noaudit/factory-context/tenant tests and actual tenant socket denial pass.
27 valid schema/policy/precision/collection-ownership/live0/registry mutations fail;
source restores exactly.43 MCP wave mutations so far. Source/native/mock peer/runtime/
AppHost/MCP CLI/related race20/whole controlplane race1/all Go/build/vet/static/full gates
pass;236 packages138 imports13 calls,structure127. Shared view corrects lifecycle
number rendering too; client float decoding is later work. **Next: five lifecycle
services/bindings**, then six-op native exit before market/plugin. No live peer/daemon/
process claim; wider invocation/module/trigger/surfaces/other transports/protected
main delivery remain open.

**W2.21c MCP lifecycle service foundation:** five writer methods and selected
attachment reader move add/attach/detach/set_enabled/remove into app/tools, with
explicit inputs and typed public server/attach/detach/remove outputs. Native codecs,
manual lifecycle registrations and legacy empty correlation/background attachment
context remain in this move-only slice. Add forwards the full registration (including
private transport values) only to the writer; public views contain only sorted env/
header key names. Attach forwards context/raw ref/tool names and nil/empty collection
shape; error identity is retained except the original wrapped ErrNotFound translation
for attach/set_enabled. Detach success stays true; absent remove stays false. Runtime
registration validation, durable writes, attach race/dial/close semantics and domain
publication remain unchanged. Port tests pin five writers, explicit correlation,
attach context, raw refs/enabled flags, full input, 2^53+1/maxint64 public timestamps,
live0 and error/no-view behavior.345 native response/state/attachment/close comparison
cases20 use two owned fixtures per case; only positively asserted runtime-assigned
IDs/wall-clock timestamps are normalized. Both stdio/HTTP peers are mocked; no process,
network peer or tool call is made.12 valid mutations detect lost input/correlation/
context/ref/tools/result/enable/error/view behavior; exact source restoration.
The first 5m proof budget expired after passing repetitions; its log is retained.
The unchanged 345-case/count20 proof passes with15m; no production/CI timeout changes.
55 MCP wave mutations so far. Source/native/runtime/MCP CLI/related race20/whole
controlplane race1/all Go/build/vet/static/full gates pass;236 packages138 imports13
calls,structure127. **Next: five typed lifecycle bindings with measured audit/cancel/
correlation/context refinements**, then MCP native exit and market/plugin. This does
not claim lifecycle cancellation/audit admission corrections or protected delivery;
client float decoding and wider invocation/module/trigger/surfaces remain open.

**W2.21d MCP typed native lifecycle bindings:** five primary aggregate mandatory-
audit unary app specs derive native registration and typed public outputs. Raw server/
ref/enabled codecs preserve missing/null/error precedence, raw refs and legacy enable
bool or case-insensitive true/exact1 string semantics (no trimming, other types false).
Add passes private env/header values only to the durable writer, never public views;
unknown arguments remain accepted. Attach nullable/empty/tool order and server/detached/
removed required roots retain their shapes. Five manual registrations/native wrappers
and mcp.go are deleted. **Explicit admission/identity/context refinements:** actual
old native proofs reproduce all five closed-journal effects, all five pre-canceled
effects, five empty domain correlations and background attach context; the same16
permanent cases pass20 after binding. App audit admission blocks writer/dial/close/store
changes; pre-canceled native calls do not begin audit or factory. All five direct service
methods precheck context and prefer operation-owned correlation over explicit fallback;
old-service cancellation/identity proofs fail and after20/mutations pass. Attach keeps
caller value/deadline/identity and a blocking mock dial deadline leaves attachment/store
unchanged with one failed audit pair. Remove's detach+remove events share audit identity.
Canonical metadata/type/schema/root/nullable-tools/50 codec cases/unknown/context/tenant/
20 admission cases/settlement tests pass20. Audit settlement failure after a writer
reports failure without pretending rollback; runtime domain publication behavior is
unchanged.345 native dispatcher response/state/attachment/close comparisons20 use owned
fixtures and mock both peer transports; only positively asserted assigned IDs/times are
normalized.31 valid policy/factory/cancel/correlation/context/codec/root/aggregation
mutations detect regressions; exact source restoration. An initial tools-root mutation
survived a weak assertion; required-root declaration checks were strengthened and the
complete final serial31 mutation run passes.86 MCP wave mutations total. Official
archcheck writer removes exactly controlplane→mcp adapter bypass, with no additions and
byte-identical call ratchet:236 packages137 imports13 calls; structure127. Source/native/
runtime/MCP CLI/related race20/whole controlplane race1/all Go/build/vet/static/full gates
pass. Exact original runtime Attach/Detach/RemoveAndAttachEnabled/Allowlist/Bridged
fixtures and the complete core MCP package also pass20/race20; name-based selectors
cover the source tests that the generic MCP regex missed (all-Go1 already covered them).
**Next: six-operation MCP native exit evidence**, then market/plugin. No real peer,
process, daemon deployment or client float-codec claim; broader transports/invocation/
modules/triggers/surfaces and protected main delivery remain open.

**W2.21e MCP six-operation native exit:** [evidence](41-w221-exit-evidence.md)
closes one primary aggregate unaudited unary list read and five primary mandatory-
audited unary lifecycle writes. Composite native coverage pins exact six input/output
signatures, single AppOwned registration, policies, unary/no-emission/unknown metadata;
each individual operation omission fails. Two private env/header schema additions fail
separately. Actual owned socket tenant denials for all six leave registrations/live
attachments/journal/provider/dial state unchanged.12 exit mutations;98 valid MCP wave
total (a16,b27,c12,d31,e12). Entire production Go path/hash manifest is identical to d;
two new native tests plus one direct CLI protocol test file add exit coverage. Earlier
generic MCP CLI selectors had no MCP tests; Tool/Compare checks were related only.
Exit now has10 CLI wire/alias/rendering cases and16 early invalid-argument cases,
count20/race20/full CLI1 through an owned canned TCP endpoint, not a real daemon/peer.
Four CLI protocol mutations plus six omission/two private-schema mutations give12 exit. App/native20/race20 and whole CP race1/CLI/
static/arch/dead/deps/official structure/2771-file format pass. D all-Go/build/vet/full
and exact runtime Attach/Detach/RemoveAndAttachEnabled/Allowlist/BridgedTool/core MCP20/
race20 remain accepted unchanged; final63 catalog and345 native lifecycle parity20
plus explicit unchanged before/after admission/precision/identity/context proofs
retain their actual d/b records without redundant full writers. Counts236 packages137
imports13 calls,28 SDK+1 test seam,24 deps,structure127. Generic no-I/O dispatch three
100ms runs remain below50us; this excludes audit/socket/lookup/dial/real-peer latency.
Docs/changelog/owned working-source and committed-range gitleaks/diff/index/frozen
patch chain pass. **Next: market/plugin order5**, plus broader runs.Start/invocation/
modules/triggers/surfaces/other transports. No live MCP peer/process/daemon/generated
SDK claim; protected final-head delivery/CI/main merge stays pending.

**W2.22a market read presentation foundation:** list/show/sources move into new
L4 app/market Reads behind one selected core-manager Reader port. Catalogue/installed
provenance/source ownership and core validation/library behavior stay in market.
Native adapters keep trimmed permissive strArg codecs and all eight manual policies;
three reads remain primary aggregate unaudited unary, five writes/streams are untouched.
Service preserves availability before required-name error, raw explicit input query/
marketplace values at its port, name trimming, read-error identity and old ignored
context/canceled-read behavior. List/sources preserve [] empty arrays, manager order,
required counts/rows, every optional core field and original JSON-to-map float64
projection. Show preserves ten roots, int64 installed_at versus projected numeric
fields, full pack manifest/base64 resource representation, raw full SKILL.md and MCP
name/tool order. Valid skill summaries use the first exact em-dash delimiter and keep
empty description; malformed summaries leave only skill_md. Tools retains null versus
[]; installed false/zero remain; informational VetPack, including danger, does not
reject read. Pack-manifest MCP transport values are unchanged in this move; this is
not the separate MCP-management redacted view. Row maps/legacy projection and manual
bindings remain for the next typed step. Shared CP structToMap/mustJSONRaw are still
used by market writes/ACP; strArg/parseCapList remain shared native helpers.
360 byte-exact native cases20 have no normalization: six owned unavailable/empty/
catalogue/installed/corrupt-install/corrupt-source fixtures × three reads × nineteen
normal argument cases, plus18 pre-canceled cases. Stores/journal/attachments/provider
state remain unchanged; no fetch/process/peer/tool call. Permanent actual native
availability/shape/no-effect and three tenant socket denials pass20. Port/read/source
contracts pass20;24 valid query/empty/count/projection/errors/name/summary/content/order/
null/install/vet/availability mutations fail and source restores exactly. An initial
substring compile fixture is excluded; anchored exact error returns plus independent
three-error/count axes pass in the final serial run. Core source tests, new app/native/
related race20/whole controlplane race1/all Go/build/vet/static/full gates pass:
237 packages137 imports13 calls,structure128,2774 formatted Go files. No allowance
added or core/stream behavior rewritten. **Next: typed three read models/operations
and native registration**, then five market writer/stream services/bindings and eight-
operation exit, then plugin. Wider invocation/runs.Start/modules/triggers/surfaces/
other transports and protected final-head/main delivery remain open.

**W2.22b market typed native reads:** three primary aggregate unaudited unary
app specs derive native list/show/sources registration, actual request/result types
and nested output schemas. Permissive raw query/name/marketplace fields retain
missing/null/nonstring→empty, trimming and unknown-field compatibility; unavailable
manager still precedes required name. List has two required roots and17-property/
eight-required embedded Listing rows; sources has two roots/four-property/two-required
Source rows. Show has ten roots,11-property/two-required full pack, typed review and
three-property/one-required skill view: successful summary name/description pointers
keep empty description present, malformed summary omits both but preserves full MD.
Byte resources/null/empty/tools/raw strings/optional fields/order/false/zero/default-
allow informational vet persist. Core Listing/Source/Pack remain full manifest models;
authored pack MCP values are unchanged, not the separate redacted management DTO.
**Explicit precision correction:** before-proof fails native downloads/added_ms/
signed_at/nested MCP created_ms/updated_ms for2^53+1/maxint64; installed_at control
already passed. The unchanged native proof passes20 with typed int64 fields/owned
manifest, preserving the installed_at control. Row maps/JSON-to-map projection are
removed. **Explicit admission refinement:** before returned results for three canceled
read callers; unchanged after20 rejects before factory/reader. Canonical counters
pin no audit/factory/port calls for pre-canceled and tenant callers. Direct service/
in-progress non-context manager reads keep their old behavior. **Ownership repair:**
actual old root tools output mutated reader's collection; typed field-access proof
and mutation now retain its ownership. Former nested projection ownership remains
without float conversion: cloned listing tags/full manifest tags/keywords/tool lists/
skills/resource maps+byte slices/MCP rows+args+allowlists+env+headers/signature; null
versus empty collections are retained.342 normal native cases20:219 byte-exact and123
with only asserted integer lexeme corrections. The18 former canceled parity cases
remain in the a record and are superseded by explicit admission proofs. Stores/
journal/attachments/provider state remain unchanged; no fetch/process/peer/tool call.
Canonical policy/types/schema/roots/conditional summary/byte resources/codec/availability/
context/noaudit/tenant and native before-after/metadata regressions pass20.45 valid
policy/factory/codec/clone/precision/required-root/registry mutations detect regressions;
exact source restoration. Initial proof-loop and unused-import mutation fixtures are
excluded, repaired and the final serial45 run alone counts.69 market wave mutations.
Three native wrappers/manual entries and host read helper are removed; selected
manager binding lives in app host. Five native write/stream handlers and shared
strArg/parseCapList/JSON helpers remain. Source/native/related race20/whole CP race1/
all Go/build/vet/static/full gates pass:237 packages137 imports13 calls,structure128/
2778 Go files. Client float decoding is later work. **Next: five market writer/stream
services and typed bindings**, then eight-op exit/plugin; wider invocation/runs.Start/
modules/triggers/surfaces/other transports/protected delivery remain open.

**W2.22c market writer/stream service foundation:** five selected Writer methods
and optional best-effort publication port move install/uninstall/add_source/
remove_source/sync orchestration into app/market Writes. Core keeps library validation,
signature verification, informational vet/default allow, skill creation/promotion,
MCP registration, provenance, optional reverse APIs, keep-last-good source sync and
network screening. Native codecs/manual five-command metadata and progress framing
remain: two StreamEvents plus three unary writes. Availability precedes required
name/URL. Install/uninstall trim the explicit name; other explicit service fields
keep their raw values while native strArg codecs trim/nonstring→empty. Install/
uninstall pass original caller correlation and void progress callback unchanged;
only Sync has a context-bearing core port and keeps the exact caller context.
Progress precedes terminal publication/result; writer errors can retain earlier
progress but suppress success publication. Install lifecycle payload uses reported
record name/version/marketplace/skill/MCP/tool lists/unsigned flag, not request name.
Uninstall publishes trimmed request name; source-add publishes reported source/URL;
source-remove publishes even absent=false and returns required name/removed roots.
Sync preserves [] empty result arrays, original row/float projection/order/zero fields,
pack sum and source count. Error with no rows fails without publication; nonempty
results plus error succeed with required results/synced/packs and optional partial_error
string plus completion publication. Install/source result projections and numeric
rounding remain in this move. Void progress/publisher failures, pre-canceled direct/
legacy native behavior and publication correlation are not yet refined. No rollback
or live transport/process guarantee is claimed. Shared CP framing/JSON helpers remain.
255 byte-exact native frame/store/materializer/fetch/domain comparisons20 require no
normalization:240 normal+15 canceled across unavailable/available/effect-error modes.
Owned market files reset only within checked fixture roots; skill/MCP materializers
and HTTP RoundTripper are mocked/in-memory, never real host tools or remote peers.
Permanent five actual tenant socket denials preserve store/journal/attachments/provider;
five unavailable failures retain their original audit pairs. Port/input/error/nil
callback/sequence/publication/partial-sync contracts pass20.35 valid writer argument/
callback/required/availability/publication/partial/empty/count/context/projection
mutations fail and exact source restores;104 market wave mutations so far. Source/
native/related race20/whole CP race1/all Go/build/vet/static/full gates pass:
237 packages137 imports13 calls,structure128/2781 Go files. No allowance/dependency
added. **Next: typed five writer/stream bindings with measured audit/cancel/identity/
emitter/publication refinements**, then eight-operation market exit/plugin. Wider
invocation/runs.Start/modules/triggers/surfaces/transports and protected final-head/
main delivery remain open.

**W2.22d market typed writer/stream binding and controlled core:** five primary
aggregate mandatory-audit app specs replace the native writer/frame adapters: install/
uninstall StreamEvents, add/remove_source/sync unary. Raw permissive codecs keep
trim/nonstring→empty/unknown behavior and unavailable-before-required precedence.
Typed InstalledPack/Source/uninstalled/removed+name/sync results retain required/
optional/null/zero/order/partial-error shapes; a pointer partial_error preserves an
empty error string when present. Transient progress keeps original kind/subject/
actor/envelope with typed core.Event payload requiring stage+ok, optional name/detail;
shared event.WireSchema is specialized without adding progress correlation fields.
**Explicit audit/cancel/identity refinements:** actual old17 native cases fail: five
closed-journal admissions, five pre-canceled callers (sync still fetched/audited),
five split audit/domain identities and two first-write failures. The unchanged17
proofs pass20 after binding. Admission stops before factory/manager/materializer/fetch;
all five direct service methods precheck context, install/uninstall choose host-owned
identity over explicit fallback, publication uses the same audit identity, and raw
caller correlation no longer controls materializers. Void callback core APIs retain
legacy behavior through Background/nil-error delegates; new InstallContext/
UninstallContext accept caller context and error-returning progress, checking before
subsequent effects/promotion/provenance and stopping on sink/caller failure. Core still
owns validation/signature/default-allow informational vet/provenance/reverse/sync.
First install vet-write failure prevents all materialization; first uninstall progress
failure retains its first quarantine effect but stops later MCP removal/provenance
removal. No rollback is claimed. Core pre-cancel/progress-cancel/effect-phase/nil sink/
old void behavior tests pass20. Domain+sink causes are joined; an implementation gap
was shown red and repaired before acceptance, with independent join mutations.
Publisher errors propagate after effect and join audit settlement; actual journal
closed after first install progress yields failure after materialization/provenance,
without pretending rollback. Partial sync semantics are retained. **Integer correction:**
typed installed_ms/added_ms/fetched_ms avoid former float projection; arrays/slices are
owned.240 normal frame/store/effect/fetch/domain parity cases20 allow only three
asserted integer lexeme corrections and independently asserted old/new correlation
normalization. Fifteen prior canceled parity cases stay recorded at c and are
superseded by explicit admission proofs; no other normalization. Owned checked-root
state, mocked materializers and memory HTTP only; no real peer/process/fetch/provider.
Canonical five metadata/signature/audit/tenant/context/emitter/schema/payload/root/
null/partial/settlement/ownership/native/core contracts pass20.41 final valid typed
mutations cover those boundaries;145 market wave total. Compile/unused-row fixtures
and a redundant context-guard survivor are excluded; entry-context disconnection
and effect-phase tests strengthen coverage, final serial41 alone counts. Sources
restore exactly. Native market.go, market registration and host helpers are deleted;
parseCapList moves verbatim to args.go; now-unused strArg/mustJSONRaw are removed.
ACP retains structToMap. Staticcheck caught the unused helpers and an unnamed test
context key after full Go/build/vet passed; cleanup uses a named key and scoped
market/ACP/arg20 plus build/vet/static and remaining gates pass without a redundant
full Go writer. Official arch writer removes CP→market only:237 packages136 imports13
calls,structure128/2785 Go files. Source/native/related race20/whole CP race1/all Go/
build/vet/static/full gates pass. Client float decoding/live atomicity remain later.
**Next: eight-operation market native exit evidence**, then plugin; broader
invocation/runs.Start/modules/triggers/surfaces/other transports/protected delivery
remain open.

**W2.22e market eight-operation native exit:** [evidence](42-w222-exit-evidence.md)
closes three primary aggregate unaudited unary reads and five primary mandatory
writers (two StreamEvents/three unary). Exact eight input/output signatures/single
AppOwned registry/read-write/stream/unknown/primary policy are pinned; eight independent
omissions fail. Existing actual owned tenant socket tests cover all eight denials.
**CLI stream repair:** direct wire proof failed because install/uninstall used Call
and aborted on first event. Both use Stream now, draining frames before the unchanged
terminal rendering/JSON.16 normal/alias/request/render cases,16 no-dial invalid inputs
and two terminal failures after progress pass20/race20 through a canned owned TCP
endpoint, not a daemon/store/peer. Reverting either unary call or dropping parsed
version/marketplace fails the test.12 final exit mutations,157 market wave total:
a24,b45,c35,d41,e12; unused-variable compile fixture excluded. Entire production Go
path/hash equals accepted exit state after mutation; relative to d only CLI stream
repair changes production. Two new exit test files. Full CLI/source/app/native20/
related race20/whole CP race1/all Go/build/vet/static (CLI included)/arch/dead/deps/
official structure/2787-file format gates pass. Frozen d17 native before/after proofs,
read342 and writer240 parity20, core phase/cause/legacy/no-rollback and typed schemas
retain their accepted records; no redundant huge parity writer.237 packages136
imports13 calls,28 SDK+1 seam,24 deps,structure128. Generic no-I/O dispatch3 runs
remain below50us, excluding audit/socket/lookup/materialization/sync/real service
latency. Docs/changelog/current owned-source and committed-range gitleaks/diff/index/
frozen patch chain pass. **Next: plugin inventory in order5**; wider invocation/
runs.Start/W3/W4/W5/transports and protected final-head CI/main merge remain open.
No live marketplace/signing authority/host install/MCP process/daemon restart claim.

**W2.23a plugin inventory service foundation:** daemon-supplied six-field
Registration/Reader port and inventory body move into new L4 app/plugins Service.
The selected native adapter translates runtime manifest fields without importing
runtime/agent into app, spawning/inspecting/invoking processes, validating pins or
changing allowlist enforcement. Rows remain maps in this move; six required fields
prefix/path/args/tool_count/hash_pinned/allowed_tools preserve raw strings, zero/
negative counts, false flags and collection order. Args always [] when nil/empty;
allowed_tools nil stays null versus explicit [] stays []; both arrays are copied.
Root plugins is [] empty with required count; original sort.Slice prefix comparator
and duplicate-prefix behavior remain, without sorting/mutating the daemon manifest.
Each call rereads the selected port. Manual primary aggregate unaudited unary native
registration/wrapper and old ignored-context/canceled-read behavior remain for the
next typed step.50 complete byte-exact native cases20 across five owned nil/empty/
zero/rich/duplicate14 manifests × nine ignored/input variants plus five pre-canceled
calls have no normalization; manifest/journal/provider state is unchanged. No plugin
binary starts or runs. Original native manifest tests and permanent actual tenant
socket denial pass20, along with port/field/array/copy/order/freshness tests.12 valid
empty/count/sort/field/pin/array/adapter mutations detect regressions; source restores
exactly. App/native/source/related race20/whole controlplane race1/all Go/build/vet/
static/full gates pass:238 packages136 imports13 calls,structure129/2790 Go files.
No allowance/dependency added. **Next: typed plugin row/output/schema and one app
operation/native registration**, then plugin native exit and remaining domain work.
Broader invocation/runs.Start/modules/triggers/surfaces/transports and protected
final-head CI/main delivery remain open; no live plugin deployment/pin verification
or permission-enforcement claim is made by inventory reads.

**W2.23b typed plugin inventory binding:** Row has six required fields and
ListOutput has two required roots. Args nil/empty still emits []; allowed_tools nil
emits null versus explicit [] emits []; arrays are copied, raw values/zero/negative
counts/false/order and original sort.Slice duplicate-prefix behavior are preserved.
One app/plugins operation owns primary authorization/tenancy, ReadOnly unaudited
unary metadata, input/output signatures and unknown-input compatibility. Native
plugin.go retains only the selected runtime manifest adapter; the socket/business
wrapper and manual registry entry are removed.45 normal complete native byte-exact
cases20 across five manifests and nine input variants pass without normalization.
Five legacy canceled parity cases are superseded by explicit admission evidence:
the unchanged actual native before-proof formerly returned a result; typed dispatch
rejects context.Canceled before the provider/read. Canonical counters pin no read,
factory or audit for pre-canceled/tenant requests. Direct Service.List and in-progress
reader cancellation semantics remain unchanged. Schema tests pin two roots/six row
required fields including zero/false/null; metadata/unknown input/caller context,
copy/order/freshness, native signatures and actual tenant socket denial pass20.
26 valid typed/schema/policy/array/field/adapter/registration mutations fail and exact
sources restore; a nonexistent enum in the initial mutation fixture caused a build
failure and is excluded. Final26 are runnable assertion failures.38 wave mutations
including foundation12. App/native20, related race20, whole controlplane race1,
all Go/build/vet/static/architecture/dead/dependencies/official structure pass:
238 packages136 imports13 calls,structure129/2793 Go files; no allowance/dependency
added. **Next: plugin one-operation native exit evidence**, then order6 config/
settings/configcenter/channels/webhook/tunnel/update. Wider invocation/runs.Start/
modules/triggers/generated surfaces/other transports and protected final-head CI/
main delivery remain open. No live process/pin/enforcement claim follows from reads.

**W2.23c plugin one-operation native exit:** [evidence](43-w223-exit-evidence.md)
closes primary aggregate plugin_list typed/AppOwned unaudited unary binding. Exact
canonical/native signatures/policy/registration and six-required-row/two-root schemas
are pinned. Native wrapper/manual entry are removed; selected manifest adapter remains.
45 normal byte-exact native cases20, actual canceled before-fail/after20, canonical
no-factory/read/audit admission and actual tenant denial preserve manifest/journal/
provider.44 wave valid mutations=foundation12+typed26+CLI6; source restores exactly.
New direct CLI owned TCP fixture covers table/JSON/empty/error and invalid/help8cases20/
race20, status probe then plugin_list nil-args/token, no daemon/process execution.
Initial fixture omitted the normal status probe and was corrected; no product finding.
Production source hashes exactly match b full Go/build/vet/controlplane race acceptance.
Exit entire CLI20/native20/new CLIrace20/scoped static/architecture/dead/dependencies/
official structure2794-file formatting/docs/changelog/secrets/diff gates pass.
238 packages136 imports13 calls,structure129; no allowance/dependency added.
No-I/O benchmark3:21284/20503/28130 ns/op (<50us); socket/audit/process I/O excluded.
**Next: order6 config/settings/configcenter/channels/webhook/tunnel/update**. Wider
invocation/agent-loop/runs.Start/modules/raw-ref GC/triggers/generated surfaces/other
transports and protected final-head CI/main delivery remain open. Inventory evidence
does not verify live deployment, pin checking or permission enforcement.

**W2.24a config read presentation foundation:** order6 begins with one native
config read. New L4 app/config Reader projects selected live base/model/prompt-presence/
tool-count/plugin-count/ask-policy plus optional effective RoutingReader tables.
Environment callback exposes only bool presence; canonical configEnvVars list and
its daemon-wide inventory guard remain native. Service copies configured env names
and builds original map output: seven required roots, six resolved paths, env{} empty
or true-only listed keys (set-empty counts present), no values/system prompt content.
Routing root/submaps appear only for nonempty provider tables; map string values/raw
keys, array nil/empty->[], order and fresh snapshots are copied as before. Native
config_handler.go selects runtime fields and environment presence, then frames output;
manual primary aggregate ReadOnly unaudited unary registration and ignored args/old
context behavior remain for typed binding. Shared native stringSliceMapToAny stays
for chains/routing; app helper is private to this moved presentation, with no layer
bypass/dependency/allowance added.108 complete native byte-exact cases20 cover six
provider routing variants x nine ignored args x normal/pre-canceled calls, no
normalization.54 normal54 legacy canceled; journal/manifest/mock provider unchanged,
fixture env value/private prompt absent from wire. Original config paths/counts,
actual governor routing/absence, env/prompt privacy and daemon inventory tests pass20.
New port tests pin seven roots/six paths/zero-false/raw/count/read-order, listed env
ownership, routing presence/empty arrays/order/copies/freshness and legacy canceled
service behavior. Permanent native actual tenant denial/live model+prompt refresh/
set-empty presence/no-audit/provider contracts pass20.18 valid field/path/presence/
route/array/adapter mutations fail and exact source restores. App/native20/related
race20/whole controlplane race1/all Go/build/vet/static/full gates pass:239 packages
136 imports13 calls,structure130/2797 Go files. **Next: typed config output/schema
and one app operation/native binding**, then config exit and settings/configcenter/
remaining order6. Wider invocation/agent-loop/runs.Start/modules/raw-ref GC/triggers/
generated surfaces/other transports and protected final-head CI/main delivery remain
open. This inventory read does not prove live config reload or secret-store mutation.

**W2.24b typed config read binding:** ShowOutput has eight properties/seven
required roots; Paths has six required fields; optional Routing has three optional
maps. Env is a bool-valued map, created as {} empty with listed true-only presence;
set-empty remains present. Routing root/submaps remain omitted when empty, route/
require slice nil/empty becomes [] and raw keys/model strings/order/zero/false values
stay. Typed slice/maps are copied; reader/env-name ownership/live freshness/read
order remain. One primary aggregate ReadOnly unaudited unary app/config spec owns
input/output/schema, unknown-input compatibility and canonical native metadata.
config_handler.go retains only selected runtime/RoutingReader and bool env-presence
adapters; native wrapper/manual entry and private generic app map helper are removed.
Canonical configEnvVars inventory and shared native chains/routing helper remain.
54 normal complete native byte-exact cases20 across six routing providers/nine args
pass without normalization or journal/manifest/mock provider/content changes.54 old
canceled parity cases are superseded by measured admission refinement: unchanged
real native before-proof returned config; after binding it rejects context.Canceled
before read. Canonical counters pin no provider/reader/env/audit entry on pre-canceled
or tenant requests and same caller context on accepted calls. Direct Service.Show
and in-progress Reader cancellation semantics remain unchanged. Schema golden/field
omission tests pin root/path required and routing optional fields; non-bool env values
reject. Original config/source/governor/privacy/inventory plus typed raw/zero/copy/
array/order/freshness/context/native policy/live refresh/tenant/noaudit contracts pass20.
An initial routing schema fixture assumed [] for zero required fields; the writer
emits null, so fixture correction is excluded from product findings.The initial all-Go run reports three channel packages killed after an anomalous
~24458s elapsed under the unchanged10m timeout. Same-flag focused recheck passes
in ~1s per package; unchanged full all-Go rerun and remaining gates pass. No channel
code/count/timeout change is made or product defect claimed.31 valid required/
optional/policy/context/array/field/ownership/aggregate mutations fail and exact source
restores;49 config wave total including foundation18. App/native20/related race20/
whole controlplane race1/all Go/build/vet/static/full gates pass:239 packages136 imports
13 calls,structure130/2800 Go files. No allowance/dependency added. **Next: config
one-operation native exit evidence**, then five settings operations/nine configcenter
operations and remaining order6. Wider invocation/agent-loop/runs.Start/modules/raw-ref
GC/triggers/generated surfaces/other transports and protected final-head CI/main
delivery remain open; no live reload or secret-store mutation claim follows from reads.

**W2.24c config one-operation native exit:** [evidence](44-w224-exit-evidence.md)
closes typed primary aggregate config ReadOnly unaudited unary binding. Exact
canonical/native registration/signatures/policy, seven-required-root/six-required-
path/optional three-map routing and bool env schema are pinned.54 normal byte-exact
cases20, measured unchanged canceled before-fail/after20 and canonical no factory/
read/env/audit/tenant admission plus actual tenant/live refresh/privacy/source/
governor/env-inventory contracts remain.59 wave valid mutations=foundation18+
typed31+CLI10; exact source restoration. New direct CLI9cases20/race20 uses owned
canned TCP endpoint, status then config nil args/token; successful fixtures validate
canonical output schema. Table/JSON/default/empty/error/path-order/routing/raw/zero/
false/env sorted-key presence and invalid/help no-dial behavior are preserved.
All existing owned production/source hashes match b full Go/build/vet/whole CP race
checkpoint. Initial b channel timeout reports with anomalous elapsed were followed
by unchanged focused/full reruns, no source/count/timeout rewrite. Exit entire CLI20/
native20/new CLIrace20/static/ratchet/structure/2801-file formatting/docs/changelog/
secrets/diff gates pass. Strengthened schema-valid fixture repeats focused20/race20/
static/format/diff.239 packages136 imports13 calls,structure130; no new allowance/
dependency. No-I/O benchmark3:25677/26029/28134 ns/op (<50us), excluding env scans,
socket/audit/live provider work. Lost historical Temp patches are not fabricated:
W39e snapshot hash/base verification plus21 retained pairs and fresh cumulative
patch replay prove294 W44b files; current W44c delivery296 files is stored and verified
under ignored .temp_files/architecture-delivery. Original76-pair chain is incomplete.
**Next: five settings operations, then nine configcenter operations and remaining
order6.** Broader invocation/agent-loop/runs.Start/modules/raw-ref GC/triggers/generated
surfaces/other transports and protected final-head CI/main publication remain open.
No live config reload or secret-store mutation claim follows from inventory reads.

**W2.25a settings read presentation foundation:** two of five settings operations
begin order6 extraction. New L4 app/settings Reads owns original config_schema /
config_values map presentation behind SchemaSections/PrepareValues and ValuesReader
ports. Section aliases the unchanged lower settings declaration; canonical registry
merge/filter/source/locked/Apply normalization and ReloadBoundaries sorting remain
core-owned. Native selected server base directory (distinct from kernel base), pinned
map, fresh store/vault/registry preparation and original ignored read-load errors stay
in adapters. Schema keeps two roots, all required/optional core Section/Field wire
fields and sections nil/null versus [] where supplied; empty boundaries are []. Values
keeps one required fields root [] empty, order and raw names/values; env/secret/
env_pinned/set always present. Secret rows have four fields with bool vault presence
and no value; non-secret rows have five, live nonempty env before stored fallback,
set-empty env falls back and raw whitespace counts set. Reads never call value ports
for secret fields. Manual primary aggregate ReadOnly unaudited unary policy, ignored
args/legacy canceled calls and three original writers/helpers remain for subsequent
steps; no new dependency/layer allowance.144 complete native byte-exact cases20=
4 owned storage states x2 reads x9 input variants x2 contexts, no normalization;
72 normal72 pre-canceled. Server/kernel directories differ; missing/stored/corrupt/
broken-schema fixtures preserve files/journal/mock provider and hide secret marker.
Permanent native selected-root/live-env/store freshness/load-error/pin/privacy/noaudit/
provider and actual two-read tenant denial tests pass20. Port tests pin root/row/
core metadata/null-empty/zero-false/read-order/priority/freshness/canceled behavior;
initial field-count and nil-pin-map fixture errors are corrected and excluded from
product findings.17 valid schema/field/privacy/priority/pin/host-root/load-policy
mutations fail; sources restore exactly. Source app/core settings/native and selected
vault-load20, app/core race20/whole controlplane race1/all Go/build/vet/static/full
gates pass:240 packages136 imports13 calls,structure131/2806 Go files. **Next: three
settings writer/registry service foundations**, then typed five-operation binding/
explicit schemas/cancellation/audit/exit, nine configcenter operations and remaining
order6. Wider invocation/runs.Start/modules/raw-ref GC/triggers/generated surfaces/
other transports and protected final-head CI/main delivery remain open. These reads
do not certify live reload, secret-store writes or durable writer audit/refinement.

**W2.25b settings writer service foundation:** app/settings Writes owns config_set
validation/trim/locked/secret-store selection, load/set-or-remove/save sequencing,
pinned admission and live/restart/reload presentation plus typed core-section register/
unregister orchestration. Selected native ports retain server-root registry/config/
vault construction, ignored legacy Set/Remove returns, process env Set/Unset and
kernel reload callback. Core settings validation, schema normalization/reservation/
locked-force/file persistence and vault merge/encryption remain unchanged. Provider/
model reload classification moves verbatim into app; native setLiveEnv I/O helper
remains. Native codecs preserve required/strict name/value/id/force errors, trim order,
section JSON round-trip and error texts; manual primary mutation registration/legacy
audit/background context remain. Set has env/saved/applied roots, true-only env_pinned
and reload_error presence even when message empty; reload failure remains successful
save + applied restart + reported error. Store/vault load/save errors keep exact
prefixes and original causes. Live secrets/lazy nonsecrets do not rebuild providers;
only provider/model live fields reload. Env-pinned edits save then return restart
without env/reload. Clear removes before Save; registry preserves raw section, trimmed
id, force and false removed.280 complete native cases20=4 owned storage modes x35
inputs across3 writers x2 contexts;140 normal140 legacy canceled, response/file/env/
reload effects byte-exact without normalization. Each audit pair is independently
asserted after connection completion (op.invoked + correct terminal, joined nonempty
identity/subject/actor/op, private value absent), not compared by random identity/time.
Kernel/server roots differ. Mock OnReload callback only; no provider/network, fixture
vault uses explicit owned plaintext opt-out for deterministic file comparison.
Port tests pin validation/read-only/locked before effects, exact load/save/cause/
partial/reload/pin/live/clear order and registry shape/cause/force. Permanent native
selected-root store/vault/register/locked-force/clear/pin and actual three-command
tenant denial tests pass20.24 valid validation/trim/clear/load/save/pin/reload/flag/
registry/host mutations fail and exact source restores;41 settings wave total.
Source app/core settings/native/selected vault-load20/app-core race20/whole CP race1/
all Go/build/vet/static/full gates pass:240 packages136 imports13 calls,structure131/
2811 Go files. No allowance/dependency added. **Next: typed five-operation settings
binding, explicit DTO/schema/input/error compatibility and measured cancellation/
mandatory audit refinements**, then settings exit, nine configcenter operations and
remaining order6. Legacy native beginOpAudit still ignores journal failures; this
foundation does not claim mandatory audit admission or caller-cancel refinement.
Wider invocation/runs.Start/modules/raw-ref GC/triggers/generated surfaces/other
transports and protected final-head CI/main delivery remain open; fake reload/effects
do not certify live daemon/provider rebuild or deployed config propagation.

**W2.25c typed five-operation settings binding:** two primary aggregate ReadOnly
unaudited unary specs (config_schema/config_values) and three mandatory audited unary
specs (config_set/schema_register/schema_unregister) own canonical native registration,
input/output/schema and unknown-input compatibility. Native five wrappers/manual
registrar are removed; selected server-root read/write/env/reload adapters remain.
SchemaOutput has two required roots and aliases unchanged core declarations/boundaries
(Section6props3required,Field10props6required,Boundary2props2required). Typed cloned
sections/fields/options preserve nil/null versus explicit [] and optional core fields.
ValueRow has five properties/four required env/secret/env_pinned/set with *string value:
secret rows omit value, nonsecret empty string stays present. Values root fields []
empty; order/raw/zero/false/env-before-store/pins/load policy are unchanged. Set has
five properties/three required env/saved/applied, true-only optional pin and *string
reload_error retaining present empty message. Register3 and Unregister2 required
roots preserve raw section/trimmed id/force/false removed. Private row/result maps are
removed. Strict RawMessage codecs retain missing/null/nonstring name/value/id/force,
validation order, omitted-value clear, section JSON decode errors/null-to-zero section
and raw metadata. Canonical audit starts before handler codec errors, with one failed
settlement; values remain redacted.8 actual unchanged native before-fail/after20 cases
prove three closed-journal writes and all five pre-canceled calls reject before effects.
Canonical counters pin no factory/read/write/env/audit entry on pre-canceled or tenant
requests, mandatory audit failure before writer factory and same caller context per op.
Direct service/in-progress core cancellation semantics remain unchanged. Three real
post-effect journal-close tests20 prove error terminal with applied effect/no rollback;
Windows journal wording required a fixture correction, not product changes.
72 normal read +140 normal writer native cases20 preserve complete response/state/env/
reload bytes without normalization; previous212 canceled parity cases are superseded.
Audit arcs independently verify identity/actor/subject/op/terminal/private redaction.
Initial schema parity exposed identical JSON values with changed nested member order:
legacy core Section/Field structs retain declared order through a narrow native terminal
codec projection; generic app business/registry remains shared. Permanent byte test and
mutation pin this codec. Source/port/schema required-optional/nil-empty/ownership/codec/
context/audit/selected-root/live/persistence/cause/tenant tests pass20. Context/force
mutation survivors exposed test coverage gaps, then per-operation/raw-id/force assertions
were strengthened; final38 valid mutations all fail and source restores exactly.
79 settings total including foundation17+24. Initial full Go gate exposed stale
compare evidence for removed settings.go; exactly one path was updated to the new
app/settings operation owner, preserving existing compare edits. Focused compare20
and actual full Go rerun/remaining gates pass; initial failed log remains. App/core/native/vault-load20/app-core
race20/whole CP race1/all Go/build/vet/static/full gates pass:240 packages136 imports
13 calls,structure131/2815 Go files. No dependency/allowance added. **Next: settings
five-operation native exit evidence**, then configcenter9 and remaining order6. Wider
invocation/runs.Start/modules/raw-ref GC/triggers/generated surfaces/other transports
and protected final-head CI/main delivery remain open. Mock reload/store fixtures and
inventory contracts do not certify deployed daemon/provider/config propagation.

**W2.25d settings five-operation native exit:** [evidence](45-w225-exit-evidence.md)
closes two primary unaudited reads/three mandatory unary writes. Exact five common
registrations/signatures/policies/no-emission and required/optional DTO/schema/codecs
are pinned. Wrappers/manual registrar are removed; selected server-root ports and
narrow legacy core schema JSON member-order codec remain.72 read+140 writer exact
normal native cases20, eight audit/cancel before-fail/after20, three post-effect journal
failure/no-rollback and source/port/core/tenant/live/pin/privacy/persistence/ownership/
codec contracts remain.91 wave valid mutations=17read+24writer+38typed+12CLI; final
assertion failures/exact restoration. New direct CLI38cases20/race20 cover21 normal/
error/checkpoint plus15 invalid/help+2 malformed files, schema-valid canned output,
status/token/nil-structured args, raw value joins/id/force, field rendering and owned
config.setting checkpoints. Secret checkpoint omits prior value/non-rollbackable;
checkpoint fetch failure prevents mutation, later set failure retains checkpoint.
Generic secret-render assertion was strengthened after a mutation survivor; fixture
prefix/setup corrections are not product findings. Existing source hashes match c
full Go/build/vet/whole CP race acceptance. Final entire CLI20/native20/new CLIrace20/
static/ratchet/structure/2816-file format/docs/changelog/secrets/diff pass.240 packages
136 imports13 calls,structure131; no added allowance/dependency. All five no-I/O
benchmarks3 are12333-23918ns/op (<50us), excluding real store/env/provider/audit/socket
work.317-file durable cumulative delivery replays HEAD exactly; index remains empty,
unrelated changes preserved and protected publication pending. **Next: configcenter
nine operations, then remaining order6**; wider invocation/runs.Start/modules/raw-ref
GC/triggers/generated surfaces/other transports/final-head CI/main delivery remain.
No live daemon/provider/config propagation claim follows from these fixtures.

**W2.26a configcenter read presentation foundation:** five reads (get/list/access-
log/audit/health) move to new L4 app/configcenter Reads behind selected manager Reader.
EntryMap moves verbatim, with a temporary native forwarder shared by three writer
echo paths. Secret-rated values use unchanged creds.MaskValue and true-only masked;
six required key/value/rating/created_at/updated_at/version fields, optional nonempty
description/tags/policy/allowed/excluded shape, raw values/zero timestamps/versions
and ACL-array copies remain. Tags keep original direct-slice semantics for this move.
Core manager still owns entry persistence/classification/vault/agent ACL/policy/HITL,
config.access audit/log filtering and stats. Native codecs validate strings first;
app preserves rating lower-case-without-trim, duration before availability, missing
get error translation, unavailable health2roots versus healthy3roots and checks map.
Get1root/list2roots/access2roots/audit2roots retain [] empty, order and exact raw zero/
empty metadata:8 required access row fields and9 audit row fields; int64 lexemes stay
exact. No new dependency/allowance/upward app import. Manual primary aggregate
ReadOnly unaudited unary registration and ignored args/legacy context remain; four
writer bodies compare byte-for-byte unchanged.550 complete native byte-exact cases20=
5 manager states x5 reads x11 input variants x2 contexts, no normalization;275 normal
275 canceled. Unavailable zero kernel/empty/public/secret/internal owned fixtures
preserve manager/audit files/journal/mock provider and suppress secret middle. Existing
native secret-mask/set-get-list and actual core agent allow/deny/access-log contracts
pass20; permanent actual five-read tenant denial preserves journal/provider. Port
required/optional/raw/mask/ACL copy/availability/error/filter/duration/freshness/canceled/
health/log tests pass20. Initial duplicate legacy method names and unused-variable/
import mutation fixtures are corrected/excluded from product findings; final20 valid
mask/field/copy/rating/filter/duration/log/health mutations fail and source restores.
App/core/native/MaskValue20/app-core race20/whole CP race1/all Go/build/vet/static/full
gates pass:241 packages136 imports13 calls,structure132/2821 Go files. **Next: four
configcenter writer service foundations**, then typed nine-operation binding/explicit
DTO/schema and measured audit/cancellation refinements/native exit before remaining
order6. Wider invocation/runs.Start/modules/raw-ref GC/triggers/generated surfaces/
other transports and protected final-head CI/main delivery remain open. These read
fixtures do not certify deployed credentials, live external vault/HITL or config changes.

**W2.26b configcenter four-writer service foundation:** set/delete/set-rating/set-
access business moves to L4 app/configcenter Writes behind the selected core Center
Writer (Set/GetEntry/Delete/GetAutoRating). The latter is the existing exact
Classifier.Classify forwarder; no classifier/ACL/vault/persistence changes. Native
strict string validation and permissive list cleanup/manual primary mandatory audit
remain; list cleanup is pure and occurs at the native input boundary. The temporary
native entryToMap forwarder is removed: all read/write echoes now share app EntryMap.
Set preserves internal default, lower-case without trim, short invalid-rating error
before availability, raw key/value/description/ACL arrays, NewConfigEntry defaults,
Set followed by GetEntry and masked computed echo. SetRating keeps ParseRating's
longer invalid error before availability, missing-key translation before classify,
auto override boolean including false, and lookup mutation before Set. SetAccess
replaces ACLs including nil/empty, preserves other entry fields, and re-reads after
Set; Delete forwards raw keys/errors and returns deleted:true only after success.
Set/persist and post-write-read errors retain original causes and effects; lookup
failures retain legacy key-not-found translation. Direct service context remains
legacy ignored, including pre-canceled contexts; typed admission refinement is next.
368 old/current native handler parity cases20 (4 ops x23 args x4 unavailable/empty/
public/secret states) compare complete responses, sorted core entry snapshots and
actual entry/audit disk bytes after independently asserting created_at/updated_at
are within each operation's measured interval, then normalizing only those clocks.
Initial parity fixtures incorrectly flagged public echo text and unsorted map-backed
lists; corrected fixtures, retained failure log, no product defect attributed.
Permanent port/error/order/reread/mask/raw/ACL/override tests20, real selected-manager
persistence/reload/classifier override/context20, native tenant denial preserving
state/journal/provider, all four native audited success pairs plus failed lookup and
key/value redaction20. Existing core/native agent allow/deny/privacy remain green.
The existing loader resets loaded versions to1 via Store.Set; this extraction keeps
that behavior and the regression fixture explicitly pins it.17 final valid writer
mutations fail, source restores byte-for-byte. Full Go/race/build/vet/scoped static/
arch/dead/deps/gofmt/structure gates pass:241 packages136 imports13 calls,132 kernel
packages/2826 Go files. **Next: typed nine-operation configcenter DTO/schema/codecs,
app binding and measured cancellation/audit refinement**, then native exit/order6.
Wider invocation/runs.Start/modules/raw-ref GC/triggers/generated surfaces/other
transports and protected final-head CI/main delivery remain open. Owned local test
stores/mock provider do not certify live vault/HITL, daemon deployment or remote CI.

**W2.26c configcenter typed nine-operation native migration:** five read-only
unaudited and four mandatory-audited unary operations are PrimaryOnly/Primary with
unknown-field compatibility, typed per-command RawMessage requests and derived
terminal DTO schemas. Native handlers/helpers/registrar are deleted; selected core
Reader/Writer ports remain. Required/optional/null/non-string/blank validation order
and raw text remain, including short set/list versus long SetRating invalid-rating
errors. ACL codec preserves delimiters without CR splitting, whitespace trimming,
case-insensitive first-casing dedup, non-string element dropping, nil versus empty
and ignored irrelevant fields. EntryRow has12 properties/6 required; secret mask
true-only, optional nonempty fields, borrowed tags and copied ACL arrays retain wire
shape. Entry echoes1root/list2roots/access2roots/audit2roots/delete1/rating1 preserve
zero/false/empty fields, [] empty lists, row order, int64 lexemes and error causes.
Access rows8 required/Audit rows9 required; health3 properties/2 required permits
healthy stats null/empty/object while unavailable omits stats, preserving its2roots.
Canonical nine-op metadata/context/tenant/codec/schema/root/mask/large-int tests20,
actual native wide-integer/zero lexemes, primary signature/aggregate registration and tenant denial contracts
pass20. Unchanged13-case proof fails20 before and passes20 after: four closed-journal
writes and all nine pre-canceled native calls now reject before effects. Direct and
in-progress service cancellation retains legacy semantics; no rollback is introduced.
Four actual writer fixtures close journal after persistence, return terminal audit
failure and retain in-memory plus reloaded disk effects.896 old/current native cases20
(28 input variants x3 real states x9 ops plus unavailable manager x5 reads) compare
byte-exact response/state/disk after independently bounding and replacing only entry
clock fields; writer invocation/terminal audit pairs, actor/subject/correlation and
key/value redaction are independently asserted after connection completion. Reads
remain unaudited, provider zero. A zero-kernel writer's absent audit host is now also
rejected before manager availability; real initialized runtime fixtures retain error
order. Initial mutation survivor missing-get-key fixture is strengthened; final28
valid DTO/presentation/codec/policy mutations fail and source restores byte-for-byte.
Full Go/race/build/vet/scoped static/arch/dead/deps/gofmt/generated gates pass:241
packages136 imports13 calls,132 kernel packages/2829 Go files. **Next: configcenter
native exit measurement and CLI/exit evidence**, then remaining order6 domains.
Wider invocation/runs.Start/modules/raw-ref GC/triggers/generated surfaces/other
transports and protected final-head CI/main delivery remain open. Owned local/mock
fixtures do not prove deployed credentials, live external vault/HITL or remote CI.

**W2.26d configcenter native exit and CLI corrections:** all nine typed primary
unary commands have native exit evidence in architecture/46-w226-exit-evidence.md.
Five ReadOnly unaudited reads and four mandatory writers use canonical app dispatch;
selected core ports remain, native handlers/helpers/registrar are removed. DTO/schema,
validation/raw text/ACL cleanup/availability/lookup/reread/error/masking contracts,
core ACL/classifier/vault/persistence/access audit and direct/in-progress context
semantics remain. Previous550 read/368 writer foundation and896 typed native parity
cases20,13-case before/after native admission proof20, four post-effect journal-close
persisted-state tests20, exact native signatures/aggregate/tenant/privacy/numeric and
port/schema tests remain green. CLI proof11 cases fails20 before and passes20 after:
Unix-second entry/access/audit clocks no longer display1970, nonempty audit text uses
actual agent_id/decision/policy/reason instead of nonexistent actor/action, valid
access-log/audit filter values are consumed, and missing/adjacent values for those
flags/list/rating reject before dial. Flagged/positional ratings remain supported;
set business/body/default internal rating/raw value/access flags stay byte-unchanged.
68 new CLI cases20/race20 validate schema-conforming owned loopback replies, exact
status-probe/operation/token/args, text/JSON/empty/error branches, secret masking,
list order, override false/true and invalid/help no-dial. No real daemon/provider or
owner home is used.13 final CLI plus10 independent domain-operation/native aggregate
omission mutations fail, exact restoration; together with20 read17 writer28 typed,
88 final valid configcenter wave mutations. Initial encoding/nil-map and unused-value
mutation fixtures are corrected/excluded, retained logs do not establish defects.
Nine-operation full typed Dispatch benchmark with fake Center/noop audit, windows/
amd64/GOMAXPROCS4/200ms x3 ranges10300-33862 ns/op, each below50us excluding I/O.
Full CLI20/new CLI race20/app-core/native-source20/app-core race20/whole CP race1/all
Go/build/vet/static/arch/dead/deps/gofmt/generated/full gates pass:241 packages136
imports13 calls,132 kernel packages/2831 Go files. **Next: remaining order6 channel(s),
then webhook/tunnel/update**, measuring existing behavior before extracting services.
Broader invocation/runs.Start/modules/raw-ref GC/triggers/generated surfaces/other
transports and protected final-head CI/main publication remain open. Mocks/owned
stores prove local contracts, not deployed credentials/live vault/HITL or remote CI.

**W2.27a channel inventory presentation foundation:** channel_list map building
and verbatim account/kind probe/mode/media/probe totals move to new L4 app/channels
Inventory behind selected server-root InventoryReader/InventoryValues. Native snapshot
prepares config store then vault then registry, ignores the same read-load errors,
and exposes only vault presence; environment/store/vault names/sections/pinned and
process-global manifest/liveness reads stay native selected ports. The native wrapper
and manual primary ReadOnly unaudited unary binding remain; account writers/OAuth/
gateway/registry source bytes compare unchanged. No new dependency/allowance/upward
import. Four required result roots channels/count/probe_matrix/media_matrix; rows15
required keys/accounts5/account probe6/kind probe6, fields7 plus required non-secret
value retain raw metadata/zero/false/empty, nil versus [] setup steps and media structs.
Default account precedes sorted deduplicated valid stored/vault #labels; env-only
labels are not discovered. Default-only env_pinned, live env before stored non-secret,
secret env/vault presence with no value member, per-account/live-kind distinction,
configured-required env/vault/store precedence (including stored-only secret required
configuration versus false secret field set), status/mode/notes and total matrices
remain. Empty manifests/sections/fields and no-required-env configuration stay valid;
reads remain fresh, direct/canceled context and ignored request args retain legacy.
160 complete old/current native cases20=8 missing/stored/vault/corrupt/broken/env/live/
mixed states x10 ignored args x2 contexts,80 normal80 canceled, byte-exact without
normalization. Owned server/kernel roots differ; settings/vault/schema files/journal/
provider unchanged and secret middle never emitted. Permanent selected-root/presence,
actual native tenant-denial, lower account/probe/liveness/setting source, port/root/
field/raw/label/filter/priorities/matrix/freshness tests20 pass. Initial unused-import
parity fixture and section-kind coincidence mutation survivor are corrected; final20
valid secret/precedence/label/probe/field/media/total mutations fail, exact restoration.
Full app/channel/settings/native20/app-channel race20/whole CP race1/all Go/build/vet/
static/arch/dead/deps/gofmt/generated gates pass:242 packages136 imports13 calls,133
kernel packages/2836 Go files. **Next: two channel account writer service foundations**,
then remaining OAuth/gateway/inbox/send/ACP communication operations as applicable,
typed binding/DTO/schema and measured audit/cancellation refinement/native exit.
Other order6 webhook/tunnel/update, wider invocation/runs.Start/modules/raw-ref GC/
triggers/generated surfaces/other transports and protected final-head CI/main delivery
remain open. Registry/live flags and mock ports do not certify live channel sessions,
OAuth exchanges, external messaging/vault, daemon restart or remote CI.

**W2.27b channel account writer service foundation:** account set/remove business
and selected section-field lookup move to app/channels Accounts behind AccountWriter
and fresh AccountStore ports. Native strict argStrings order/manual primary mandatory
audit/legacy context remain; inventory/OAuth/gateway/registry bytes compare unchanged.
Set trims lower-case kind/label/name, preserves raw value until settings.Validate,
checks required pair/slug/cross-section/readonly before store effects, trims value
and applies SuffixEnv. Selected Registry.Sections field lookup and global manifest
lookup remain. Secret load/set-remove/save routes to selected vault; public routes
config. Ignored vault Set return and lower remove semantics remain, prefix text
load/save config/vault and legacy unwrapped causes remain. Success5 required roots
kind/label/env/saved/applied:restart; no process environment or kernel reload effect.
Remove requires kind/nonempty label (default cannot be removed), uses global
SectionEnvs rather than selected registered field schema, retains no slug validation,
ignores config then vault load errors, counts each config/vault removal, saves config
before vault and preserves partial effects/first failure. Success4 required roots
kind/label/removed/applied including zero. These existing policy differences are
explicit, not silently repaired in a move-only slice; registered-field removal can
be measured separately before refinement. Direct/pre-canceled service context stays
legacy ignored; typed admission/audit/cancellation refinement remains later work.
576 complete old/current native cases20=6 missing/stored/corrupt config/corrupt vault/
broken vault/broken config states x24 args x2 writers x2 contexts (288 canceled),
byte-exact response/owned config/vault/schema files without normalization. Distinct
kernel/server roots, dynamic/global section behavior, partial persistence/errors,
actor/op/subject/nonempty joined correlation/invocation/terminal kind and value
redaction independently asserted after connection completion; provider zero. Port
validation/field/trim/suffix/routing/clear/ignored loads/count/error-text/cause/save-order/
partial effects and real selected-root/vault-versus-config/process-env/tenant source
contracts20 pass.19 final valid mutations fail and source restores byte-for-byte.
An extra native test accidentally compiled during a deliberate mutation; its failure
is excluded as harness overlap, final source/native acceptance is rerun serially after
restoration. Initial all-Go build failed because Temp Go cache artifacts disappeared;
the same all-Go command and remaining gates are rerun with workspace-isolated cache,
without source/count/timeout changes and with both logs retained. Full app/channel/settings/native20/app-channel race20/whole CP race1/
all Go/build/vet/static/arch/dead/deps/gofmt/generated gates pass:242 packages136
imports13 calls,133 kernel packages/2841 Go files. **Next: remaining communication
operations (OAuth/gateway/inbox/send/ACP as applicable)** and measured account policy
refinement, typed binding/DTO/schema/audit/cancellation and native exit; then other
order6. Wider invocation/runs.Start/modules/raw-ref GC/triggers/generated surfaces/
other transports and protected final-head CI/main delivery remain open. Owned stores/
mock providers do not certify deployed credentials/live OAuth/messaging/vault/daemon.

**W2.27c registered channel account removal repair:** the actual native
set->list->remove lifecycle proof fails20 before: remove reports0 while public config
and secret vault #work keys remain and inventory still lists the account. The same
permanent proof passes20 after one selected adapter repair: SectionEnvs now uses the
same merged/filtered server-root Registry.Sections as Set field lookup instead of
global builtin Schema. Registered public/secret keys delete/count correctly while
default/other accounts persist after independent reload. Native manifest override
is restored in test cleanup; no live channel/provider or owner home is used. Accounts
app business bytes remain unchanged, including validation, suffix, restart-only,
count per store, config-before-vault save/partial effects/errors and legacy context.
Permanent selected-root/foreign-root, filtered PATH/reserved builtin field, missing
section/fresh schema and every builtin section/global env-list equivalence tests20
pass. No new dependency/allowance/layer bypass; schema persistence/filtering stays
lower settings.5 final valid global/wrong-section/empty-field/wrong-env/wrong-root
mutations fail and source restores byte-for-byte. Native account/inventory/tenant/
source20, app/channel/settings20/race20/whole CP race1/all Go/build/vet/static/arch/
dead/deps/gofmt/generated/full gates pass:242 packages136 imports13 calls,133 kernel
packages/2843 Go files. Workspace caches remain isolated after prior Temp cache loss.
**Next: remaining communication operations**, starting with measured OAuth/gateway
state/port boundaries then inbox/send/ACP as applicable, typed DTO/schema/native
binding with measured audit/cancellation refinement and native exit; other order6,
wider invocation/runs.Start/modules/raw-ref GC/triggers/generated surfaces/other
transports and protected final-head CI/main delivery remain open. Mocks/owned stores
prove local contracts, not live channel/OAuth/messaging/vault/daemon deployment.

**W2.27d channel OAuth status snapshot repair:** before moving OAuth ownership,
owned in-memory status-writer/polling proof under race mode count3 reproduces data
races and mixed status/error pairs. Status previously retrieved a flow pointer under
mutex then read mutable fields outside it. A narrow repair now captures status/error/
kind/label together under the existing mutex, then releases it before response/socket
write. Known4roots versus unknown1root, raw fields/errors and ID/type wire remain.
The unchanged concurrent proof passes race20 after; permanent blocked-writer test
pins no mutex held across socket backpressure and immutable already-captured snapshot;
known/unknown/raw tests pass20. No OAuth exchange, token, provider, owner home or real
network is used in this proof. Start/callback/set-status/exchange bodies plus helper/
server state bytes compare unchanged. State generation/provider URL/TTL/prune/vault/
legacy context behavior is intentionally left for the separate OAuth move/refinement.
3 final valid unlocked-copy/wrong-error/wrong-kind mutations fail and source restores
byte-for-byte. Native channel/source20/status race20/app-channel/settings20/race20/
whole CP race1/all Go/build/vet/static/arch/dead/deps/gofmt/generated/full gates pass:
242 packages136 imports13 calls,133 kernel packages/2844 Go files. Workspace caches
remain isolated. **Next: channel OAuth start/callback/status service/state foundations**,
then gateway/inbox/send/ACP communication scopes as applicable, typed DTO/schema/
audit/cancellation/native binding/exit, other order6 and wider invocation/runs.Start/
modules/raw-ref GC/triggers/generated surfaces/other transports. Protected final-head
CI/main publication remains open; local fixtures do not prove live OAuth/messaging/
vault, daemon deployment or remote CI.

**W2.27e channel OAuth service/port foundation:** start/callback/status use-case
business moves to app/channels OAuth behind selected provider/nonce/clock/state/
exchange/vault ports. Nonce32-byte RawURLBase64, redirect validation and instance URL
normalization helpers move verbatim; existing source tests now call the app helpers,
with no native forwarding shim. Lenient native string codecs/manual primary unary
read/write policy remain. Server mutex/map, provider table, TTL15min/prune/store and
status-setter plus guarded HTTP exchange (native inner20s/form/headers/1MiB/read-error/
provider-error semantics) remain selected native ownership. Put prunes under existing
mutex, Flow snapshots all scalar fields under it, preserving the previous status race
repair/no lock retention on socket write. Callback carries caller context with outer
20s timeout, exchanges then selected-root vault Load/Set(legacy error ignored)/Save,
updates error/done status in the same order and preserves textual unwrapped errors.
Start validates provider/label/credentials/redirect/instance in original order, trims
same fields, stores pending flow/time and constructs encoded authorize URL/scopes.
Start2 required roots/callback5/status known4 versus unknown1 remain raw/zero/empty;
no token/client secret is emitted. Start/status legacy canceled-context behavior and
callback cancellation/error status persist; state retention/TTL/terminal policy is
left for separate state ownership/refinement, not silently changed in this move.
450 complete old/current native cases20=5 provider-response/vault states x15 input
variants x3 operations x2 contexts. Response bytes match after separately validating
and replacing random32-byte nonce (including authorize query); new flow clocks are
bounded then normalized in state snapshot. State/owned vault/schema bytes/fake HTTP
request counts match; independent writer audit pairs/actor/subject/joined correlation/
private redaction, unaudited status and model provider zero. Fake RoundTripper uses
no external network; real selected-root vault is owned plaintext opt-out fixture.
Port/order/trim/URL/query/flow/nonce/status/raw/error/cause/vault/context/deadline/
cancellation tests20, selected native state prune edge >TTL/copy/fresh status and
original source guarded exchange/cancellation/URL helpers20 pass.17 final valid app/
helper mutations fail/exact restoration; initial duplicate legacy function fixture
is corrected/excluded. Full status/native-source20/race20/app/channel/settings20/
race20/whole CP race1/all Go/build/vet/static/arch/dead/deps/gofmt/generated gates
pass:242 packages136 imports13 calls,133 kernel packages/2849 Go files. **Next: OAuth
state/provider/exchange ownership convergence** with preserved lifecycle, then other
communication gateway/inbox/send/ACP as applicable, typed DTO/schema/audit/cancel/
native binding/exit/order6/7 and wider roadmap. Protected final-head CI/main delivery
remains open. Owned fake HTTP/vault fixtures do not prove live OAuth/messaging/vault,
daemon deployment or remote CI.

**W2.27f channel OAuth state/provider/exchange ownership:** app/channels
OAuthMemory owns the provider table, per-host pending flows/mutex, value snapshots,
status updates and start-only pruning (>15min; equality retained). OAuthMemory.Exchange
owns the previous HTTP form/headers/inner20s timeout/1MiB body bound/provider-error
precedence and body/context cleanup. Native Server selects one memory/service using
sync.Once and binds the existing guarded HTTP client and server-root vault factories.
Native private flow/provider table/status/exchange/prune code and helper file are
removed; three native codec wrappers and app OAuth use-case business are byte-identical
to W2.27e. Scalar snapshots retain status-race protection without socket-held locks.
No implicit lifecycle policy change: read does not prune, missing status does not
create a flow, read errors remain ignored, nonempty tokens retain legacy HTTP-status
handling and callback vault persistence/order/textual errors remain.
450 old/current native cases20=5 response/vault states x15 args x3 operations x2
contexts pass, with independently validated nonce/query and bounded clock normalization,
matching response/state/owned files/fake HTTP counts; independent writer audit privacy,
unaudited status and provider-zero assertions. Owned fake HTTP only. App memory tests
cover exact provider defaults, TTL boundary, lookup retention, copied/fresh state,
owner isolation, concurrent status snapshots, exchange form/headers/deadlines/cleanup,
transport cause/provider error precedence/1MiB/read-error contracts. Native concurrent
32-call initialization/server isolation, paused writer/status snapshots and original
source guarded exchange/cancellation/helper tests pass20.20 valid ownership/exchange
mutations fail with exact source restoration. Full status/native-source20/race20,
app/channel/settings20/race20, whole CP race1, all Go/build/vet/scoped static/arch/dead/
deps/gofmt/official generated gates pass:242 packages136 imports13 calls,133 kernel
packages/2851 Go files. **Next: remaining communication service foundations** starting
with gateway status/QR, then inbox/send/ACP as applicable, typed DTO/schema/audit/cancel/
native binding/exit/order6/7 and wider roadmap. Protected final-head CI/main delivery
remains open; isolated fixtures do not certify live OAuth/messaging/vault/deployment.

**W2.27g channel gateway status/QR service foundation:** app/channels Gateway
owns normalization of URL, backend, session and supplied values. It selects
WAHA/Evolution endpoints and headers, bounded GET arguments and status/QR
presentation behind GatewayGET. Native wrappers
retain lenient typed string accessor and manual primary ReadOnly unaudited binding;
selected factory binds unchanged platform/netout.GatewayGET. Native HTTP forwarding
shim and duplicated use-case business are removed. Platform guarded local/LAN HTTP,
background10s timeout and partial-read semantics remain unchanged; canceled caller
contexts retain legacy behavior in this move. No automatic channel send occurs.
Status preserves early http(s)/host validation; QR leaves that validation to the
selected port/result boundary. Status response precedence status/state/instance,
case-sensitive WORKING versus case-insensitive open, malformed JSON best effort,
success2xx/non2xx http_status error maps remain. QR preserves image content-type
case sensitivity/raw base64, existing data URL/raw JSON base64, missing-image and
HTTP error results. Status1MiB/QR4MiB arguments and raw session path construction
remain. New permanent app contracts pass20;22 valid mutations fail/exact restoration.
480 complete old/current actual native dispatcher cases20=8 owned loopback HTTP
shapes x15 input variants x2 operations x2 contexts. Byte-exact wire with no
normalization and matching request URI/key/owned files; independently checked private
key exclusion, no operation audit/provider calls. Original platform/native source
tests and both primary-only tenant denials pass20. Full native/source20, app/channel/
settings20/race20/status race20/whole CP race1, all Go/build/vet/scoped static/arch/
dead/deps/gofmt/official generated gates pass:242 packages136 imports13 calls,
133 kernel packages/2854 Go files. **Next: inbox/send/ACP service foundations**,
then typed DTO/schema/audit/cancel/native binding/exit/order6/7 and wider roadmap.
Protected final-head CI/main publication remains open; owned loopback/fake ports
do not prove live gateway credentials/WhatsApp/messaging/daemon deployment.

**W2.27h channel inbox service foundation:** app/channels Inbox owns journal
folding, exported message/thread representations, default20/clamp1..1000, channel
normalization/filtering, newest timestamp/correlation descending sort and cursor
pagination/presentation behind InboxJournal.Range. Native typed accessors preserve
float64-only limit/truncation and ignored wrong limit/channel types, raw cursor and
delayed cursor codec error. Factory selects s.k.Journal; manual primary ReadOnly
unaudited binding and legacy background context remain. No store is introduced.
Inbound/outbound only, correlation-or-event-ID grouping, first nonempty channel
metadata, ordered messages and maximum timestamp remain. Malformed payloads retain
best-effort folding; empty threads stays [], sender omission/raw fields retained.
Total counts filtered threads before cursor/page; malformed/overflow/whitespace
cursor falls back, first-colon split retains correlation colons, same-time >=corr
boundary and optional next_cursor/channel remain. Range error preserves its cause
and wins over cursor type error after scan. Large int64 timestamps remain exact.
200 complete actual old/current native dispatcher cases20=5 owned journal states
x20 args x2 contexts: empty, grouped/no-correlation/ignored kind,1005 rows, malformed
payload shapes and owned corrupt segment. Byte-exact wire without normalization;
complete owned file hashes/journal head unchanged/provider zero independently
checked, including corrupt-range versus wrong-cursor precedence. App grouping/
field/order/filter/limit/cursor/large-int64/error/context/source contracts pass20;
selected kernel journal versus unrelated roots and tenant denial pass20.21 final
valid mutations fail/exact restoration; initial unused-variable mutation fixture
corrected and excluded. Full native/source20/status race20/app/channel/settings20/
race20/whole CP race1/all Go/build/vet/scoped static/arch/dead/deps/gofmt/generated
gates pass:242 packages136 imports13 calls,133 kernel packages/2858 Go files.
**Next: send and ACP service foundations**, then typed communication DTO/schema/
audit/cancel/binding/native exit/order6/7 and wider roadmap. Protected final-head
CI/main delivery remains open. Owned journal fixtures do not certify live channel
messages/daemon deployment or journal recovery behavior.

**W2.27i channel outbound send service foundation:** app/channels Outbound
owns channel/to/text trim, channel lowercase, required-field-before-availability
validation, selected Sender invocation, background30s timeout, exact error cause and
three-field result. Native factory resolves the current Server sender per request;
wrapper retains lenient string codecs/manual primary writer audit and socket framing.
Existing stringArg helper, ChannelSender injection/setter and registry bytes remain.
Measured before move: both success/error keep sender context live through a paused
terminal socket write, then cancel after completion (source20). Send uses a terminal
callback to retain that lifecycle and panic cleanup during this foundation; context/
typed operation binding refinements remain separate. Caller values/cancellation stay
ignored as in legacy native send; no live channels/external messages are invoked.
160 old/current actual native dispatcher cases20=4 selected sender states x20 args
x2 contexts: nil, success, error and contained panic. Complete byte-exact wire,
owned effect records/files match; independently bounded sender deadline/cancel
cleanup, correlated invoked/completed-or-failed audit/actor/subject/private argument
redaction/provider zero. Fake senders only. New permanent app validation/normalization/
deadline/error cause/callback lifetime/sender-or-terminal panic cleanup contracts20,
native paused-write lifetime/current sender replacement/nil/server isolation/tenant
no-effect denial and original source20 pass.15 valid mutations fail/exact restoration;
initial unused-import parity fixture corrected/excluded. Full native/source20/status
race20/app/channel/settings20/race20/whole CP race1/all Go/build/vet/scoped static/
arch/dead/deps/gofmt/official generated gates pass:242 packages136 imports13 calls,
133 kernel packages/2863 Go files. **Next: ACP inventory service foundation**, then
typed communication DTO/schema/audit/cancel/native binding/exit/order6/7 and wider
roadmap. Protected final-head CI/main delivery remains open. Owned fake sender/pipe/
file fixtures do not certify live channel delivery, credentials or daemon deployment.

**W2.27j channel ACP inventory service foundation:** app/channels ACPInventory
owns active-command environment selection/trim and ordinary cached discovery through
selected active getter/ACPDiscovery ports, with default catalog owner wiring. List
forwards caller context/cancellation and force=false, returns the complete typed
catalog inventory unchanged, and leaves registry/client failures in-band. Active
environment is read each call; authoritative inventory fields/counts/nil and nested
metadata remain. Native primary ReadOnly unaudited wrapper ignores args and retains
the existing structToMap terminal codec until separate typed binding; no ACP agent
loop/installation is invoked. CP no longer imports acpcatalog. Official archcheck
-update removes that one adapter-bypass exception:135 import exceptions/13 calls.
432 complete old/current actual native dispatcher cases20=6 owned registry/client/
cache/catalog states x4 active commands x9 ignored arg variants x2 contexts. Fresh,
cached, unavailable, malformed registry, stale failure and empty local catalog cases
retain byte-exact wire without normalization; fake source clocks are fixed via the
existing clients' Now ports. Request paths/cancellation/cached refresh policy match;
complete owned file hashes/journal head unchanged/provider zero independently checked.
Fake HTTP and no installed executable/agent launch in parity fixtures. New app full-
value/context/trim/ordinary-refresh/environment freshness/nil/count contracts20 and
original source discovery invoked under owned sources20; primary-only denial checks
no discovery requests/audit/provider.10 valid service/default wiring mutations fail
with exact restoration. Module/native source20/status race20/app/channel/catalog
race20/whole CP race1/all Go/build/vet/scoped static/arch/dead/deps/gofmt/official
generated gates pass:242 packages135 imports13 calls,133 kernel packages/2866 Go
files. **Next: typed communication DTO/schema/operation bindings and native exit**,
preserving source behavior or separating measured validation/cancel refinements,
then remaining order6/7 and wider roadmap. Protected final-head CI/main publication
remains open. Owned fixtures do not certify live catalogs/installed agent versions/
daemon deployment. Native legacy terminal codec precision remains separate work.

**W2.27k channel ACP typed operation/native binding:** ACPInventoryOperations
declares the single acp_agents GET /api/acp/agents spec: primary-only/primary tenancy,
ReadOnly/unaudited/unary, empty typed input with unknown args allowed, full typed
catalog output and generated schemas. CP registers that operation in the shared
dispatcher, removes the native ACP factory/wrapper file/manual row and final
structToMap float round-trip helper. ACP service business and Web UI read route
bytes are unchanged. Constructor rejects a nil service provider; field presence,
nested inventory types/in-band source failures and nil agents remain declared.
Two explicit binding effects: the shared pipeline now rejects an already-canceled
request before provider/discovery (old handler still returned an in-band inventory),
and native typed terminal UseNumber preserves declared large integer counts. The
captured old helper rounded9007199254740993 to9007199254740992 and int64 max to
9223372036854776000; this is a typed wire-contract measurement, not a claim that
real catalogs reach those sizes. Actual ACP native typed frames preserve both values.
432 actual old/current dispatcher cases20=6 owned source/cache/catalog states x4
active commands x9 ignored arg variants x2 contexts.216 normal cases match complete
wire bytes with no normalization/request paths/cache/owned files/head/provider zero;
216 canceled cases explicitly verify old inventory versus new context-canceled
error and zero HTTP requests. Complete owned files/head remain unchanged in both.
Nil-provider/metadata/auth/GET/schema/null/full output/unknown args/malformed roots/
service context/no-audit/non-operator/no-discovery/cancel tests20 pass, with original
source invoked under owned sources/tenant denial20. Captured old precision20 and
current typed ACP actual integer/cancel native tests20;12 valid spec/schema/native
mutations fail/exact restoration. Full module/native source20/status race20/app/
channel/catalog race20/whole CP race1/all Go/build/vet/scoped static/arch/dead/deps/
gofmt/official generated gates pass:242 packages135 imports13 calls,133 kernel
packages/2868 Go files. **Next: remaining channel inventory/account/OAuth/gateway/
inbox/send typed DTO/schema/operation binding and native exit**, with measured
validation/cancel/lifecycle refinements explicit, then order6/7/wider roadmap.
Protected final-head CI/main delivery remains open. Owned source/pipe/model fixtures
do not certify live catalogs/agents/channel messages or daemon deployment.

**W2.27l channel inventory typed DTO/operation/native binding:** ListOutput,
channel/account/field rows, account/kind probes and probe/media matrices are concrete
JSON-tagged models. Secret value pointers stay nil/omitted; public values retain
non-nil pointers even when empty. Empty channels/fields arrays, nil versus empty
setup steps, false fields/raw strings/default account/labels/configuration/liveness/
notes/counts retain their old wire shape. Probe/totals helpers use typed members.
One channel_list GET /api/channels primary-only/primary ReadOnly unaudited spec with
empty typed input/unknown args and fully nested generated output schema now binds
through shared dispatcher. Native wrapper file/manual row removed; selected reader
factory still uses Server root/store-vault-registry order/ignored loads and presence
ports. Web UI route and native reader bytes remain. The native object codec retains
MediaCaps declaration order as it already does core config-schema structs; initial
parity exposed only that member-order difference, corrected without normalization.
Explicit binding effect: already-canceled requests reject before provider/Prepare;
direct service still retains legacy context behavior.160 old/current native cases20
=8 owned store/vault/env/liveness states x10 ignored args x2 contexts.80 normal cases
compare complete raw response bytes with no normalization;80 canceled cases check
old inventory versus new context-canceled error. Selected server/kernel roots remain
distinct; owned files/journal head/provider unchanged and secret contents absent.
Original app map-shaped wire assertions are retained through a test-only JSON view,
with actual typed output mutation/freshness and typed native selected-root assertions.
Full nested schema/field presence/null/arrays/GET/auth/unknown args/no audit/canceled
reader/source/tenant/media-order tests20;27 valid DTO/privacy/logic/schema/native
mutations fail/exact restoration.
CLI compare evidence pointers to the deleted native wrapper are updated to app
inventory; the original all-Go failure is retained, Compare source20 and exact
all-Go command rerun pass after the narrow pointer repair. Full module/native
source20/status race20/app/channel/catalog race20/whole CP race1/all Go/build/vet/
scoped static/arch/dead/deps/
gofmt/official generated gates pass:242 packages135 imports13 calls,133 kernel
packages/2872 Go files. **Next: channel account/OAuth/gateway/inbox/send typed DTO/
schema/operation bindings and native exit**, with validation/cancel/lifecycle changes
measured explicitly, then remaining order6/7/wider roadmap. Protected final-head CI/
main delivery remains open. Owned source/store/pipe fixtures do not certify live
credentials/channels/messages/daemon deployment. Snapshots use full path names.

**W2.27m channel account typed DTO/operation/native binding:** SetAccountOutput
and RemoveAccountOutput are concrete JSON-tagged models; saved/restart, empty label
and zero removed counts remain present. Separate RawMessage request fields preserve
strict kind/label/name/value type admission order and exact native errors, including
null, with unknown input allowed. Two primary-only/primary POST writer specs own
existing account routes; native wrappers/manual rows removed. Selected Server-root
account ports, field/section selection, suffixes, trim/clear, config-vault routing,
ignored removal loads, textual errors, partial persistence and restart behavior
remain unchanged. Shared dispatcher now requires successful audit admission before
writer effects and rejects already-canceled requests before provider/audit; these
binding changes are explicit, while direct service retains legacy context behavior.
576 captured-current/native cases20 =6 store/vault states x24 args x2 operations x2
contexts:288 normal raw response/file bytes exact without normalization,288 canceled
requests assert no file/audit effects. Audit pairs/correlation/actor/privacy retain
normal semantics; selected roots differ and provider remains unused. Original
source/lifecycle/tenant tests plus typed schema/presence/validation/audit/route and
real closed-journal/canceled native tests20 pass.29 valid business/DTO/schema/binding
mutations fail, with exact source restoration; initial fixture actor/error identity
assumptions corrected and excluded. Full Go/source/race/build/vet/static/arch/dead/
deps/format/generated gates pass. **Next: OAuth/gateway/inbox/send typed bindings and
native exit**, then webhook/tunnel/update/order7 and wider W3-W5 roadmap. Protected
final-head CI/main delivery remains open. Owned fixtures do not certify live services.

**W2.27n channel OAuth typed DTO/operation/native binding:** start/callback/status
use concrete JSON-tagged results. Known status always contains error/kind/label,
including empty strings, through non-nil pointers; unknown status contains only
status. Request RawMessage codecs preserve lenient missing/non-string-to-empty
admission and unknown input. Three primary-only/primary specs use shared dispatcher:
POST start/status retain existing routes; internal callback declares no public HTTP
route, because the existing public Web UI GET page forwards with its own credential.
Start/callback writers require successful audit admission; status remains unaudited.
Already-canceled requests reject before provider/state/exchange/vault/audit effects.
Native wrappers/manual rows removed. Selected per-Server OAuthMemory/service/client/
vault factories, token-exchange form/headers/timeouts/bounds/error text, flow lifecycle,
restart outcome and Web UI route/page bytes remain unchanged. Native atomic snapshot
and paused socket/mutex-release assertions now pass through shared output binding.
450 captured-current/native cases20:225 normal complete wire/flow/file/exchange-count
matches and225 canceled/no-effect admissions. Only random32-byte nonce is replaced
after independent decode/query/state validation; new-flow clocks are bounded and
removed from comparisons. Normal audit pair/correlation/actor/privacy retained.
Source/native/schema/presence/auth/lenient input/real closed-journal/context tests20
and29 valid service/helper/DTO/schema/binding mutations pass with exact restoration.
Full Go/source/race/build/vet/static/arch/dead/deps/format/generated gates pass.
**Next: gateway/inbox/send typed bindings and channel native exit**, then remaining
order6/7 and W3-W5. No live credentials/channel messages/deployment certification.

Protected delivery update: PR #701 merged W2.10g-i through W2.27m at8416d9c8
on2026-10-07 after all24 exact407ef3c7-head CI jobs succeeded, including required CI
and ci.yml. Shared local main fast-forwarded to the same merge commit while preserving
local OAuth edits and unrelated security-report deletions. W2.27n is the next local
slice awaiting its own final-head CI/protected merge. Earlier restriction notes below
are historical evidence; current permissions allow normal Git/GitHub delivery.

**W2.27o channel gateway typed DTO/operation/native binding:** separate concrete
status/QR outputs retain optional result member presence through pointers. Successful
status contains connected:false and status:"" when appropriate; transport errors
omit http_status, HTTP errors contain it including zero, and QR/error fields retain
their prior shape. Two primary-only/primary ReadOnly POST specs bind shared dispatcher;
raw-field codecs preserve lenient missing/non-string-to-empty and unknown input.
Native wrappers/manual rows/wgArg removed; selected GatewayGET factory remains a
small native port file, with the same guarded LAN-capable/background-context HTTP
behavior, endpoint/header/bounds/body parsing and Web UI route bytes. Explicit
binding effect: already-canceled requests reject before provider/HTTP calls. In-flight
HTTP cancellation remains a separate refinement; direct service still ignores caller
context as before. Original map-shaped result assertions are retained via a test-only
JSON view; actual typed presence/freshness and full generated schemas are tested.
480 captured-current/native cases20:240 normal complete raw response/HTTP request
bytes exact without normalization,240 canceled/no HTTP calls. Owned loopback states,
files/journal/provider/privacy checked; no external network/credentials. Source/native/
schema/auth/lenient/presence/closed-journal/canceled tests20 and35 valid service/DTO/
schema/binding mutations pass with exact restoration; two compiler-only pointer
fixture attempts retained and excluded. Full Go/race/build/vet/static/arch/dead/deps/
format/generated gates pass. **Next: inbox/send typed bindings and channel native
exit**, then remaining order6/7 and W3-W5; own final-head CI/protected merge remains.

Protected delivery update: OAuth W2.27n merged in PR #702 at a00b580c on2026-10-07
after all24 exact19ca5ce1-head CI jobs succeeded (including CI/ci.yml); shared local
main fast-forwarded to the identical validated merge tree, preserving gateway edits
and unrelated deletions. W2.27o is the next local slice awaiting its own delivery.

**W2.27p channel inbox typed DTO/operation/native binding:** InboxOutput has typed
thread pointers/count/total and optional next_cursor/channel. Existing thread/message
models, empty arrays, sender omission, sorting/filtering/grouping/fallback IDs,
count-before-page and int64 timestamps remain. One primary-only/primary ReadOnly GET
spec binds shared dispatcher; raw codecs retain numeric truncation/ignored wrong
limit types, lenient channel input and delayed strict cursor errors. Journal Range
errors still win over cursor type errors. The native object codec retains nested
thread/message declaration order and exact int64s, like existing inventory/config
struct handling. Wrapper/manual row removed; selected kernel journal factory remains
in small native port file, independent of Server root. Already-canceled requests
reject before provider/Range. Direct service retains legacy context behavior.
200 legacy/current native cases20=5 owned journal states x20 args x2 contexts:
100 normal raw byte-exact responses/no normalization and100 changed canceled
admissions; complete owned files/head/provider unchanged. Permanent schema/nested
presence/freshness/JSON numeric limits/auth/native member-order/int64/range-error/
selected-journal/tenant/source tests20 and32 valid service/DTO/schema/binding mutations
pass with exact restoration. The old private direct-Go-map decoder test is replaced
with real JSON boundary tests: Go int/json.Number arguments are JSON numbers on the
wire, as in the original native Request decoder; this is measured, not a codec
compatibility exception. Full Go/race/build/vet/static/arch/dead/deps/format/generated
gates pass. **Next: send typed binding and channel native exit**, then remaining
order6/7 and W3-W5; own final-head CI/protected main merge remains.

Protected delivery update: gateway W2.27o merged via PR #703 at 5703f2d5 on2026-10-07
after all24 exact3cdf135a-head CI jobs succeeded (including CI/ci.yml). Shared local
main fast-forwarded to the identical validated merge tree, preserving local inbox
edits and unrelated deletions. W2.27p is the next local delivery. Owned fixtures
certify source/wire contracts, not live channel messages or deployed binaries.

**W2.27q send typed binding and channel native exit:** concrete three-field send
result and lenient raw requests use one primary POST writer operation/shared native
dispatcher. Native wrapper/manual row removed; current Server sender factory and
shared stringArg helper remain. stdlib-only opapi TerminalCleanup accepts optional
cleanup ownership, supplied by native terminal scope. Outbound transfers cancellation
only after sender returns; native scope releases after result/error socket write,
including failure/panic, LIFO outside its lock and exactly once. Direct callback
service keeps legacy lifetime and background30s/value isolation; sender panic cancels
immediately before opaque internal-error delivery. Naive return bridge failed the
original paused-write proof3; corrected shared binding passes20. Module/native
lifetime/write-failure/panic/ownership/reentry/concurrent tests20/race20,160 native
cases20=80 normal raw wire/effects/files exact including panic opacity+80 changed
canceled/no sender-audit effects,34 valid mutations/exact restoration pass. Mandatory
audit admission/canceled preflight precede sender factory/effects. Initial panic
assertion gap was strengthened and whole mutation list rerun. Full source/Go/race/
build/vet/static/arch/dead/deps/format/generated gates pass. Eleven channel operations
have unique AppOwned/primary/unary/typed signatures; empty registrar removed and
exit evidence recorded in47-w227-exit-evidence.md. No live channel messages sent.
**Next: webhook, tunnel and update in order6, then order7 and wider W3-W5.** In-flight
sender/gateway context refinement, lifecycle redesign and other transport convergence
remain open; own final-head CI/protected merge remains separate.

Protected delivery update: Inbox W2.27p merged via PR #704 at ca25f5c7 on2026-10-07
after all24 exact32a5975c-head CI jobs succeeded including CI/ci.yml. Shared main
fast-forwarded to identical validated tree preserving send edits/unrelated deletions.
W2.27q merged via PR #705 at c32e8852 on2026-10-08 after all24 exacteaa6f669-head CI jobs succeeded including CI/ci.yml. Local main identical-tree fast-forward preserved webhook edits and unrelated deletions.

**W2.28 webhook observability typed/native exit:** app/webhook owns delivery decode,
log shaping/filtering and stats, using selected routed journal and lower shared
journalview projection. Two ReadOnly/OwnTenant/CallerTenant unary specs replace
native handlers/manual rows. Strict optional failed boolean validation, lenient
numeric limit/since and ignored malformed cursor preserve actual legacy behavior;
count is returned page length. Status/error optional pointers retain zero/empty
presence; nested DTO schemas and exact int64 native output are independently tested.
Pre-canceled native proof red3 on both commands/after20. Actual three-journal tenant
socket isolation and source/schema/codec/race20 pass.240 native cases20=120 normal
complete raw wire/owned files/head exact+120 canceled/no effects,27 valid mutations
including both registration omissions/exact restoration; initial fixture Range-call
count6 corrected to5, excluded from product findings. Full source/Go/race/build/vet/
static/arch/dead/deps/format/generated gates pass; no allowance added. No-I/O dispatch
24.6–27.8us/op x3 (<50us). Evidence48-w228-exit-evidence.md records actual boundaries.
Web UI route bytes unchanged; stats has no invented HTTP route; no external webhook.
Own docs/payload/head CI/protected merge still must close. **Next: measure tunnel's
actual entry (no controlplane command exists), then update/order7/W3-W5.** Inbound
webhook async convergence, dispatcher supervision and trigger unification remain.

**W2.29a update service foundation/tunnel premise:** no native tunnel command exists;
boot target/URL adapter and layer5 supervisor already use platform/sandbox. Boot
helper20/full tunnel race20 prove current boundary; module lifecycle/exposure remains
later work. app/update now owns disabled/check/apply validation/presentation, raw
unverified manifest, background60s/no-deadline contexts and sentinel→callback→100ms
restart ordering. Native selects current Backend/current version/primary drain/
sentinel/delayed shutdown; callback codec and manual primary registrations remain.
Public concrete setter API/nil behavior preserved through canonical interface nil.
Naive return bridge red3 on early context cleanup and restart while socket blocked;
correct callback binding passes20, including failed write versus writer panic,
backend/reply panic and no added release provenance/signature.480 native cases20
preserve raw response/backend effects/manifest/sentinel presence in normal/canceled
contexts; sentinel clock and audit independently bounded/paired.19 valid service
mutations/exact restoration plus caller-context-removal mutation red3; typed-nil
fixture guard repaired and excluded from product findings. Lower HTTP cancellation
fixture10s sleep exhausted the local3m repeat budget; explicit owned-handler release
and context.Canceled assertion now pass20/race20 without reducing count/CI timeout.
Full source/Go/race/build/vet/static/arch/dead/deps/format/generated gates pass.
[Foundation evidence](49-w229-update-foundation-evidence.md). Check/apply results
remain transitional maps; typed operations/shared canceled admission/after-writer-
return ownership/native exit are next. Cleanup release alone cannot encode restart:
returned write error schedules, writer panic does not. No release download/swap/live
restart. W4.5 signing and order7/wider W3-W5 remain. Own protected delivery still open.

Webhook W2.28 delivered via PR #706 at 1f5d5c36 on2026-10-08 after all24 exactb83aef38
head CI jobs succeeded including CI/ci.yml. Shared main identical-tree fast-forward
preserved update edits and unrelated report deletions. W2.29a is next delivery.

**W2.29b typed update/native exit:** concrete check/release/apply results and ordered
RawMessage apply codecs bind two primary unary specs; Check ReadOnly/Apply mandatory
audit. Native wrappers/map codec/manual rows removed; selected backend/current/
primary drain/sentinel/shutdown ports remain. New stdlib-only TerminalWrite accepts
post-response ownership only until writer returns (including returned error); panic
discards callbacks. Native scope finishes every result/error writer-return path and
still releases cleanup on panic. Service retains direct callbacks, transfers cleanup
only after presentation is ready, writes sentinel before result and schedules100ms
restart after actual writer return. Backend panic/nil Check result cleanup precedes
opaque internal-error delivery. Existing paused write/failure/panic/restart proof20
passes; pre-canceled admission/closed-journal-before-Apply and actual tenant denial20
are independent. Terminal audit failure retains successful effects/sentinel, sends
error and schedules restart after error write.480 native cases20=240 normal raw
response/effects/manifest/sentinel exact+240 explicitly changed canceled/no effects;
clock/audit independently bounded/paired.27 valid service/DTO/codec/terminal/registry
mutations restore exact source; both exit omissions fail. Source/race20/full Go/CP
race/build/vet/static/arch/dead/deps/format/generated gates pass. No-I/O dispatch
9.0–13.1us x3 (<50us). Initial DTO member-order/audit-output fixtures corrected;
temporary parity-source cleanup/parallel vet clash serialized, excluded from findings.
[Native exit evidence](50-w229-update-exit-evidence.md). No lower verification or
signing changes/real download/swap/restart. Own protected delivery remains; **next
order7 roster**, then remaining domains/run/tool convergence/W3-W5.

Update foundation W2.29a delivered via PR #707 at 0f97e7a3 on2026-10-08 after all24
exact5db665d7-head CI jobs succeeded including CI/ci.yml. Shared main identical-tree
ff preserved typed update edits/unrelated deletions. W2.29b is next delivery.

**W2.30a roster list/presentation/cache foundation:** app/roster ListService owns
full profile JSON presentation/kind/managed, keyed1.5s cache/RWMutex, copy-before-
reverse/filter/pagination/counts. One stable service per primary Server uses current
profile/status ports; native strict codec/manual ReadOnly registration/model-chain/
sequence helpers and status derivation remain. Old invalidation key0 collided with
empty roster hash0 and retained stale total/enabled counts: before red3 (1/1), actual
native response regression after20. Explicit cache validity rejects empty/same-key
invalidated entries.240 native cases20 preserve raw bytes/read/status/cache/provider
counts/head across3 roster states/20 inputs/2 contexts/2 reads. Module/source20,
TTL/ownership/invalid input/cursor/aggregate/profile fields/large legacy float numbers/
concurrency/race20,15 valid mutations/exact restoration and full source/Go/CP race/
build/vet/static/arch/dead/deps/format/generated gates pass. JSON profiles:null empty
and legacy background/canceled read behavior preserved. [Evidence](51-w230-roster-list-foundation-evidence.md). Status projection/typed DTO/op/native exit and other16
roster commands/lifecycle remain; own protected delivery open. **Next: roster status/
read foundations and typed list binding**, then remaining order7/W3-W5.

Typed update W2.29b delivered via PR #708 at 0d5777c5 on2026-10-08 after all24 exact
bb3b5d6b-head CI jobs succeeded including CI/ci.yml. Local shared main identical-tree
ff preserved roster edits/unrelated deletions. W2.30a is next local delivery.

**W2.30b roster status presentation/typed snapshot:** app StatusService owns30d/
24h clock windows and complete status rendering over selected typed observations.
No app runtime/controlplane dependency; native ReaperScan/repair/escalation/wake/
single-pass journal collector retains order/current sources and maps all used fields.
Existing native row names alias app data; RepairPhaseLabel moves to its sole app
consumer and the unused native forwarder is removed. Health priority, retired/paused/live layering, misconfig supplements,
repair/error presence, routing override, retry/escalation/wake/live/activity/runbook/
mailbox/policy details/large int64/default gates remain.3072 complete old/current
JSON render cases20 and72 actual initialized-kernel native collector/list cases20
match raw bytes without normalization; provider/head unchanged. Independent windows/
complete-field/priority/phase tests20,21 valid mutations/exact restoration, broad
Agent/Roster source/race20 and full Go/CP race/build/vet/static/arch/dead/deps/format/
generated gates pass; initial staticcheck-unused-shim log retained, current source20/
native parity20/suffix rerun after deletion preserve the passing broad prefix.
Extraction import/default-count/unused mutation fixture issues corrected and excluded.
[Evidence](52-w230-roster-status-foundation-evidence.md).
Public list still uses manual codec/registration and transitional maps; typed list/
remaining roster collection/read/lifecycle/native exit and order7/W3-W5 remain.
Own protected delivery open; **next typed list and remaining source/read foundations**.

List/cache W2.30a delivered via PR #709 at d8856048 on2026-10-08 after all24 exact
fd3e261b-head CI jobs succeeded including CI/ci.yml. Local shared main identical-
tree ff preserved status edits/unrelated deletions. W2.30b is next local delivery.

**Historical state after W2.1a (before the pilot):**
- The control plane's `commandSpec` (`kernel/controlplane/dispatch.go`) already does authenticate →
  resolve → tenant authz → tenant routing → stream mode → **audit**.
- Every handler, though, writes straight to `net.Conn` (`s.writeResp(conn, …)`), so nothing outside the
  socket protocol can reuse them.
- The web UI proxies to ops through a fixed 198-entry route table, and REST/OpenAI/agentgw are separate
  hand-written surfaces.

**Do (roadmap W2.1, then W2.4…n):**
- Introduce `kernel/app` with `OpSpec{Name, Input type, Authz, Tenancy, Stream, ReadOnly}` and handlers of
  the shape `func(ctx, Input) (Output, error)`.
- Make the control plane an *adapter* that decodes `Request.Args` into `Input` and encodes the result.
- Pilot the domain `status`/`version` (read-only, low risk), then migrate domains in the order of
  roadmap §3, **one domain per PR, deleting the old handler in the same PR**.
- Benchmark dispatch overhead (budget < 50 µs/op excluding audit I/O).

**Invariants to keep:** `dispatch_registry_test.go` (registry ↔ protocol constants 1:1),
`tenant_auth_test.go`, `TestRegistry_TenantAllowedImpliesTenantRouted`, and the op-audit tests
(`dispatch_audit_test.go`). A new read-only op must be marked `ReadOnly` or it gets journaled on every
console poll.

### 4.6 Later (W3–W5)

See roadmap §2 W3–W5: dissolve `runtime.Kernel` into modules in increasing-coupling order, unify
triggers (W4.1, cadence system tasks through policy — owner-approved), channel supervisor and
conversation store, email DKIM/SPF, self-update signing, generated OpenAPI/SDKs, and allowlists → 0.

Cheap open findings you can fold in when you touch that area (register §9):
- seat `"container"` never maps to the container execution profile;
- `shell` `timeout_ms` is uncapped;
- a changed operator-profile facet text creates a second active record;
- artifact GC can delete blobs still referenced by a journal `raw_ref`;
- `reranktool` can panic on a short `scores` slice;
- the boot banner says 6 guardians, but 7 ship.

---

## 5. How to work here (procedures that worked)

### 5.1 Definition of done for one PR

1. **One concern.** Moves and rewrites are separate PRs, and a move uses type aliases so importers don't churn.
2. **A regression test, mutation-verified.** Show it red against the old code. Do that by restoring the
   file from a copy (`cp f $TEMP/f.bak; <break it>; go test …; cp $TEMP/f.bak f`). **Never use
   `git checkout -- f`**: it also wipes your uncommitted fix. Break each guarantee separately and list the
   mutations in the PR body.
3. **Run the source package's tests**, not only the new package's. An extraction once shipped red that way.
4. **Gates, all green** (exact commands in §5.2).
5. **Docs in the same PR:**
   - the codemap file for the area;
   - the roadmap row (✅ plus what was measured and what changed, or ⏸ deferred with the numbers);
   - the findings register row (✅ when fixed);
   - `CHANGELOG/unreleased/current.md` for anything a user or operator would notice.
6. **Commits:** the code commit first, then a separate `docs(arch): record <wave> — …` commit. The message
   explains *why*, names the measured defect, and says how the test was verified.
7. **PR body:** a "Stacked on #N" line if stacked, then *What was measured*, *Change*, *Verification*
   (tests + mutations + gates), *Docs*, and *Not in this PR*.
8. **Owner override (2026-10-03):** merge the validated stack into `main`, then work directly in the shared `main` checkout. Do not create another task branch.

### 5.2 Gate commands (run from repo root; Windows Git Bash works)

```sh
go build ./... && go vet ./...
GOMAXPROCS=4 go test ./... -count=1            # cap CPU: the owner's machine has 32 cores, don't peg them
git ls-files '*.go' | tr -d '\r' | xargs gofmt -l   # must print nothing (tr strips CRLF on Windows)
go run ./tools/archcheck          # ratchet; `-update` only to TIGHTEN after removing edges
go run ./tools/deadcodecheck
go run ./tools/depscheck
go run ./tools/docclaimscheck
go run ./tools/changelog-lint
go run ./tools/structure-md -check -out .project/STRUCTURE.generated   # after adding/renaming a package or doc.go
gitleaks detect --no-banner --redact -s . -b .gitleaks-baseline --log-opts="origin/main..HEAD"   # CI scans ALL commits
cd frontend && npm test && npm run build      # only when frontend/ is touched; npm, not pnpm
```

`make check` runs most of these. On Windows, `go vet ./...` catches copylocks that `go test` doesn't.

### 5.3 Historical: a fix that belonged to an earlier PR in the stack

The owner has retired the branch stack. The recipe below is retained as historical
context; subsequent work uses the shared `main`.

Commit it on the **earliest** branch that contains the bug, push, then cherry-pick the same commit onto
every later branch in order and push each one:

```sh
for b in <later branches in stack order>; do
  git checkout -q $b && git cherry-pick <sha> >/dev/null && git push -q origin $b || { echo "stop at $b"; break; }
done
```

Check that each branch's local head equals `origin/<branch>` before picking (the loop in the last
session silently did nothing the first time). Rewrite history (`reset --soft` + recommit +
`push --force-with-lease`) **only on your own unmerged PR branch**, and only when the bad content must
leave history, as with a gitleaks hit.

### 5.4 Reading CI logs

```sh
gh pr checks <n>
gh api --allow-escape-sequences repos/agezt/agezt/actions/jobs/<job-id>/logs | sed 's/\x1b\[[0-9;]*m//g' > $TEMP/job.log
```

`gh run view --log-failed` refuses while sibling jobs still run; the jobs API works.

### 5.5 Linux-only behaviour from Windows

The Ubuntu WSL distro has no Go toolchain. Cross-compile the test binary and run it there:

```sh
GOOS=linux go test -c -o $TEMP/x.test ./kernel/<pkg>
cd $TEMP && wsl.exe -d Ubuntu -e sh -c './x.test -test.run <Name>'
```

Tests that build helpers with `go` fail there, so filter them out with `-test.run`. Tests that need
`os.Args[0]` must use `os.Executable()`.

### 5.6 Measuring "which handlers do X without Y" (call graph)

A whole-program VTA call graph over `./cmd/agezt` takes about 6 s. The source of the tool that found the
W2.1a audit gap is in the appendix. Put it in a scratch directory outside the repo with its own `go.mod`
(`require golang.org/x/tools v0.47.0`), then `go mod tidy && go run . <repo-path>`.

**Cut traversal at the module boundary**, or `sync.Once` closures make every handler reach every sink.
It inspects handlers only. Since W2.1a, dispatch journals ops above the handler, so the tool still
reports those 54 handlers; that's expected. Treat the output as evidence, not truth: confirm the interesting cases with a runtime probe test, then
delete the probe.

---

## 6. Gotchas that cost time before

- **Nostr tamper fixtures must change the signature:** assigning the first byte
  to 00 can be a no-op (1/256 random signatures). Full validation reproduced the
  false failure; a deterministic scalar-23 fixture has that prefix. Both test
  corruptions now flip one decoded bit. The fixed fixture rejects a no-op mutation,
  the Nostr suite passes count=200 plus race/staticcheck, and full validation passes.
  Production verification is unchanged; retain the deterministic regression.

- **Stale documents:** security reports, `docs/*AUDIT*` and even this roadmap go stale in weeks.
  Re-verify against the source before acting. **Run** CI gates; don't trust a report about them.
- **The editor's LSP diagnostics are routinely stale** after branch switches and big edits. Trust
  `go build` / `go test`.
- **The dead-code gate does not see unused type aliases.** `tools/deadcodecheck` reports unreachable
  FUNCTIONS; staticcheck's U1000 also reports unused TYPES. Moving native code out of a package
  leaves its type aliases behind, the local gates stay green, and CI's lint job fails. W2.45a hit
  this with five aliases over the app status rows, and W2.45b hit it again with four more (plus a
  payload helper that only the deleted file read — the dead-code gate DOES catch that one).
  Run staticcheck locally before pushing — the
  Windows prebuilt from the same release (`staticcheck_windows_amd64.tar.gz` in the release assets)
  works, and it needs the same Go pin the CI step uses when `stable` is newer than the tool
  supports.
- **Bash heredocs swallow `\n` inside Python string literals**, which turns Go `"\n"` into a real newline
  and breaks the build. Use the Edit tool for Go strings that contain escapes.
- **CRLF:** strip `\r` from Windows-produced file lists before `xargs`. Git warns "LF will be replaced by
  CRLF" on docs; that's harmless.
- **Synthetic secrets in tests** (anything key-shaped) trip gitleaks and GitHub push protection. Build them
  at run time from parts (`strings.Join([]string{"abc", "def"}, "")`). Never click an "unblock secret" URL.
- **Package-global state in tests** must be fully reset in `t.Cleanup`. race-depth runs `-count=20`.
- **Socket responses precede deferred operation audit.** Linux stress exposed
  audit fixtures reading only `op.invoked`, and an export fixture comparing its
  snapshot head with a journal advanced by the run's later `op.completed`.
  Subscribe before the request and wait for the terminal audit before asserting
  a settled journal (`watchOpAudit` / `awaitOpAudit` in control-plane tests).
  Production response/audit ordering is unchanged.
- **A test that turns red after a security fix often pinned the vulnerable behaviour.** Rewrite it; don't
  revert the fix. Check sibling packages for the same pin.
- **"Test-only" is not "dead":** `deadcodecheck` runs without `-test`, so guards look unreachable. Ask what
  the test asserts before deleting.
- **New `AGEZT_*` env vars** read in `cmd/agezt` must be added to the control plane's `configEnvVars`
  (alphabetical), or a guard test fails.
- **New event kinds** go in `kernel/event/kinds.go` as constants and must be emitted or consumed in
  production code (`TestKindRegistryIsClosed`). Never `event.Kind("literal")`.
- **New control-plane ops:** register in the domain's `register…Commands`, add the protocol constant (the
  1:1 test), set `TenantAllowed` + `TenantRouted` only for tenant-safe ops, and set **`ReadOnly`** for reads.
- **A new way of running AS an agent** must call `runtime.WithAgentProfile` and layer explicit per-run flags
  on top. Never copy profile fields by hand (the W2.0 bug).
- **HTTP clients** come from `kernel/platform/netout`, child processes from `kernel/platform/sandbox`, store
  files from `kernel/platform/filestore`. archcheck's forbidden-call ratchet rejects bare
  `http.Client`/`exec.Command`/`os.WriteFile` elsewhere.
- **Query-string args proxied by the web UI are text:** numeric keys must be listed in `numericQueryArgs`
  (`kernel/webui/webui_proxy.go`).

---

## 7. Owner laws and decisions (don't relitigate)

- **Strategy:** strangler; layers under `kernel/`; storage stays JSON files + journal (no SQLite); dead
  `contract/gen` halves deleted; cadence system tasks go through policy (W4.1).
- **5.5′:** approvals are not persisted; a resumed run re-asks.
- **5.6:** a corrupt mid-journal record is quarantined and the daemon continues.
- **W1.8:** do not merge `settings` and configcenter; that's a product decision for the owner.
- **Default-allow:** capabilities are allowed by default; restriction is opt-out. Hard denies, SSRF guards,
  budgets and explicit HITL stay.
- **Rate limiting:** only the token-free `/hooks` path is throttled. Don't add throttles to authenticated
  run endpoints without asking.
- **`code_exec` is deliberately max-capability** (network on). Don't tighten it without asking; secret
  scrubbing, isolation and audit are non-negotiable.
- **No default provider or model** ships in the daemon. Models come only via routing/chains.
- **Boot resilience:** recoverable config mismatches warn and degrade; they never fail boot.
- **Never live-verify against the owner's real `~/.agezt`.** Use an isolated `AGEZT_HOME` in a temp dir.
- **The `.dev-home` console has a password.** Never fetch or enter it, and don't automate the browser
  against it. Verify UI via vitest, or ask the owner.
- **Ask the owner** (one question, recommended option first) only for real product decisions. Otherwise
  pick the conventional option, say so, and proceed.
- The owner writes in Turkish. Answer in Turkish when they write Turkish; code, commits, PRs and docs are
  English.

---

## Appendix — call-graph audit tool (`main.go`)

```go
package main

// Usage: go run . <path-to-agezt-repo>
// Lists control-plane handlers that reach a persistent write but never a journal publish.
import (
	"fmt"
	"go/types"
	"os"
	"sort"
	"strings"

	"golang.org/x/tools/go/callgraph"
	"golang.org/x/tools/go/callgraph/cha"
	"golang.org/x/tools/go/callgraph/vta"
	"golang.org/x/tools/go/packages"
	"golang.org/x/tools/go/ssa"
	"golang.org/x/tools/go/ssa/ssautil"
)

const mod = "github.com/agezt/agezt/"

func main() {
	pkgs, err := packages.Load(&packages.Config{Mode: packages.LoadAllSyntax, Dir: os.Args[1]}, "./cmd/agezt")
	if err != nil {
		panic(err)
	}
	prog, _ := ssautil.AllPackages(pkgs, ssa.InstantiateGenerics)
	prog.Build()
	all := ssautil.AllFunctions(prog)
	cg := vta.CallGraph(all, cha.CallGraph(prog))
	cg.DeleteSyntheticNodes()

	pkgOf := func(f *ssa.Function) string {
		if f.Pkg == nil {
			return ""
		}
		return f.Pkg.Pkg.Path()
	}
	isPublish := func(f *ssa.Function) bool { // the "Y" — change these predicates per question
		p := pkgOf(f)
		return (p == mod+"kernel/bus" && strings.HasPrefix(f.Name(), "Publish")) ||
			(p == mod+"kernel/journal" && f.Name() == "Append")
	}
	isWrite := func(f *ssa.Function) bool { // the "X"
		switch p, n := pkgOf(f), f.Name(); p {
		case "os":
			return n == "WriteFile" || n == "Rename" || n == "Remove" || n == "RemoveAll" || n == "Create" || n == "MkdirAll"
		case mod + "kernel/platform/filestore":
			return n == "Save"
		}
		return false
	}

	var rows []string
	for fn := range all {
		if pkgOf(fn) != mod+"kernel/controlplane" || !strings.HasPrefix(fn.Name(), "handle") {
			continue
		}
		recv := fn.Signature.Recv()
		if recv == nil {
			continue
		}
		if pt, ok := recv.Type().(*types.Pointer); !ok || pt.Elem().(*types.Named).Obj().Name() != "Server" {
			continue
		}
		start := cg.Nodes[fn]
		if start == nil {
			continue
		}
		parent := map[*callgraph.Node]*callgraph.Node{start: nil}
		queue := []*callgraph.Node{start}
		var writes, pub bool
		var path string
		for len(queue) > 0 {
			n := queue[0]
			queue = queue[1:]
			if n != start {
				pub = pub || isPublish(n.Func)
				if !writes && isWrite(n.Func) {
					writes = true
					var p []string
					for c := n; c != nil; c = parent[c] {
						p = append([]string{c.Func.Name()}, p...)
					}
					path = strings.Join(p, " -> ")
				}
				if !strings.HasPrefix(pkgOf(n.Func), mod) {
					continue // cut at the module boundary: stdlib/third-party are sinks only
				}
			}
			for _, e := range n.Out {
				if _, ok := parent[e.Callee]; !ok {
					parent[e.Callee] = n
					queue = append(queue, e.Callee)
				}
			}
		}
		if writes && !pub {
			rows = append(rows, fn.Name()+"\n    "+path)
		}
	}
	sort.Strings(rows)
	fmt.Println(strings.Join(rows, "\n"))
	fmt.Println(len(rows), "handlers write without publishing")
}
```