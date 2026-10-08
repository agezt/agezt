# W2.30a roster list/presentation/cache foundation

Application ListService owns profile presentation, content-keyed1.5s cache, cached
outer-row ownership, descending pagination and aggregate counts. The native adapter
selects current primary roster/status callbacks and retains strict argument codec,
manual primary ReadOnly registration and shared model-chain/sequence helpers.
Status projection business, typed DTO/operation binding, remaining roster commands
and lifecycle/module extraction remain open.

## Measured invalidation defect

The old native invalidator cleared the key to0 and rows to nil while keeping the
timestamp, total and enabled counts. Empty roster hash is also0: removing the last
profile during the TTL caused the next read to hit the invalidated entry and return
stale total/enabled counts. The before-proof fails three repetitions with total1/
enabled1. The application cache now has explicit validity independent of its hash;
invalidation always rejects an entry, including empty/same-key rosters. The native
regression reads actual framed list responses before/after removal and passes20.

## Ownership and preserved behavior

One service per Server is initialized by sync.Once; its own RWMutex owns cache
validity/key/time/rows/counts. Existing mutation sites retain their invalidation
calls through a thin selected-service adapter. The read always fetches current
profiles, validates fingerprint and respects the original inclusive TTL boundary.
Expired entries and profile UpdatedMS changes rebuild. In-flight misses are not
coalesced or redesigned in this move.

ProfileView retains complete lower-profile JSON marshaling/float64 number semantics
and kind/managed augmentation. Other native CRUD/lifecycle/tool-agent consumers use
the same application presentation helper through their existing shim. Large profile
int64 values retain legacy float rounding in this foundation; status port integers
retain their original native encoding. Richer typed/exact-number migration is later.

The cache build precedes reported limit/cursor decode errors. Native strict numeric
limit still defaults0 (unbounded), ignores nonpositive values and clamps1000; string/
null/bool limit produces the original error. Cursor is optional strict string;
malformed numeric positions are ignored, and raw timestamp/slug parsing behavior is
retained. Page order is descending CreatedMS/Slug. Copying the cached outer slice
before reverse/filter prevents one request from corrupting later pages. Count is
page size; total/enabled_count cover the whole roster. next_cursor appears only when
the page is truncated. Empty profiles remain JSON null, matching the legacy wire.

Status derivation remains the selected native callback: reaper/repair/escalation/
wake/journal logic and all its behavior are covered by the source package tests,
but have not yet moved to application ownership. Manual canceled read behavior is
preserved; shared pre-canceled admission remains a later typed-binding change.

## Runnable evidence

240 legacy/current native cases repeated twenty times cover three roster states,
twenty inputs, normal/canceled contexts and repeated cache reads. Complete raw
framed bytes and provider/profile/status/cache call counts match; journal head/hash
remain unchanged. These use owned profile/status ports over a real initialized
native server and mock provider. The empty invalidation repair is independently
proved and deliberately differs from the old cache hit.

Permanent module tests cover TTL equality/expiry, profile edit and explicit same-key/
empty invalidation, preparation-before-decode, cursor boundaries, aggregate/page
counts, cache reversal ownership, full profile/large numeric presentation and
concurrent reads/invalidation. Fifteen profile/cache/TTL/hash/paging/order/count
mutations make tests fail and restore source bytes exactly. Initial unused native
time import after removing cache fields was corrected; it is not a product finding.

Source app/core and broad native Agent/Roster suites pass20, with race variants also
passing20. Whole controlplane race and all repository Go tests pass once. Build,
vet, scoped staticcheck, archcheck, deadcodecheck, depscheck, gofmt and official
structure generation/check pass. Archcheck places245 packages with unchanged135
import/13 call exceptions; gofmt covers2,912 Go files; kernel structure136 packages.
2,917 source/build/architecture hashes verify unchanged for current closure.
Documentation/payload/history secret gates and protected delivery close separately.
No real user profile is created, edited, removed or awakened; no provider
run or live channel traffic occurs. Typed roster/remaining order7/W3–W5 remain open.
