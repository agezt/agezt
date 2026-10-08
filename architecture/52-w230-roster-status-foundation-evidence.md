# W2.30b roster status presentation and typed snapshot foundation

Application StatusService now owns30d reaper/24h routing windows and full per-agent
status presentation over a selected typed StatusSnapshot. Native collection retains
primary ReaperScan, repair summaries, escalation/wake views and the existing single-
pass journal fold. The complete rendering body moved without a simplified status
model. No new native operation or typed public list binding is claimed.

## Ownership and native compatibility

App-defined snapshot rows own the data needed for rendering; the application does
not import runtime or controlplane. Native adapters map reaper rows in their original
slice order (last duplicate slug wins), select current maps and forward all relevant
fields. Existing repair/routing/retry/wake/live/activity/policy row names are aliases
to application data types, so native collection/helper signatures and field meanings
remain. Repair phase label presentation also moved to its sole consumer. The unused
native forwarder is removed; repair/escalation history/lifecycle remains native.

Provider selection/order retains reaper→repair→escalation→wake→journal collection.
Two independent clock reads preserve the original30d and24h cutoffs. The list cache
from W2.30a still selects this native collection boundary and caches its app-rendered
output. Cache invalidation, caller cancellation, framing/auth and manual list
registration are unchanged in this move.

Health priority remains retired, paused, degraded, misconfigured, forced exhausted,
forced failed, unstable, forced probation, routing pressure, stale, healthy. Misconfig
issues can supplement a higher-priority health state. Repair latest/error presence,
routing count override, retries/escalations/wakes/live/last-activity/runbook/mailbox/
policy fields retain their original gates and values. Active-run presentation can
override operational state even for retired/paused profiles, preserving the measured
layering. Empty active phase uses running label; raw unknown repair phase text remains.
Large int64 status values are not converted to floats. Default fields are always
present; optional maps/fields are not invented for missing observations.

## Runnable comparison and regression evidence

3,072 complete old-source/current render JSON cases repeated twenty times cover256
health-signal subsets, four retired/enabled states and three live overlays, with rich
supplemental observations and an unknown agent. Bytes match without normalization.
72 actual old/new native collector/list cases repeated twenty times use three owned
initialized kernel states, six inputs, normal/canceled contexts and cache reads.
They include durable repair/retry/policy/fallback/runbook/live events and complete
raw socket response equality; journal head/hash and provider count remain unchanged.
Existing broad native Agent/Roster source tests retain real rich status journeys.

Permanent application tests independently pin health priority, operational overlay,
all supplemental fields/presence/default shape, large integers, selected provider/
clock windows and phase labels. Twenty-one window/priority/presence/overlay/value/
label mutations produce real permanent test failures and restore bytes exactly.
The first runbook-removal fixture left an unused variable; its valid fixture retains
the symbol, and the entire21-case list passes. The initial default-shape fixture
count19 was corrected to the actual18 fields. Initial extraction script import
assumptions were corrected using owned full-path snapshots. Those harness errors
are excluded from product findings.

Source app/core and broad native Agent/Roster suites pass20 and race variants pass20;
whole controlplane race/allGo1/build/vet pass. Staticcheck initially found an unused
phase-label forwarder; it was removed. Current affected native source20/parity20 and
static/arch/dead/deps/format/official generated suffix pass, with the unchanged broad
passing prefix retained. Only that recorded dead-shim source changed afterward;
2,920 input hashes verify current closure. Archcheck245 packages/135 imports/13 calls
is unchanged; gofmt2,915 Go files; official kernel structure136 packages. Documentation/
payload/history secret checks and protected delivery remain separate steps.
Fixtures do not mutate
owner profiles, awaken agents, run a provider or send channel messages. Typed list/
remaining roster/native exit, journal and other order7 domains, wider W3–W5 remain.
