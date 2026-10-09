# W2.32d typed guard audit reads

The five guard audit reads folded from the journal now run on the shared
dispatcher, in the new package `kernel/app/audit`:

- `netguard_log`: the egress guard's refusals (`netguard.blocked`);
- `ratelimit_log` and `ratelimit_stats`: run-rate throttles (`rate.limited`);
- `warden_log` and `warden_stats`: sandboxed executions, profile downgrades and
  limit breaches (`warden.*`).

The logs use the shared typed engine `journalview.ProjectValues`, with a common
page codec. The statistics are windowed folds. `Operations` declares five
read-only, unaudited, tenant-owned and caller-tenant-routed specs with unknown
input allowed:

- `netguard_log`, `ratelimit_log` and `warden_log` keep their Web UI GET routes;
- the two statistics have no route, as before.

`netguard_log.go`, `ratelimit_log.go`, `warden_log.go` and the five registrations
are removed. Native `projectJournal` now serves only `edict_log`. Its comment and
the codemap recipe point new journal views at the typed engine.

## Preserved behavior

- The log page: `limit` is a JSON number truncated toward zero, otherwise 20,
  clamped to between 1 and 1,000. `since_ms` windows rows to now minus a positive
  value. The opaque `cursor` pages strictly older rows, and `next_cursor` appears
  only on a full page.
- Every row carries `ts_unix_ms` and `seq`. Malformed payloads give zero-valued
  fields.
- A warden row carries only its kind's keys:
  - an exec row has the profile, argv0, exit code, duration and the downgraded
    and timed-out flags;
  - a downgrade row has the effective profile, the requested profile and the
    reason;
  - a limit row has an empty profile, argv0 and the limit as the reason.

  The typed row keeps this exact key set with omitempty pointers.
- `issues` is a strict boolean, checked before any read; true keeps only
  downgrades and breaches.
- `ratelimit_stats` counts the throttles, keeps the last positive limit in
  journal order (a zero does not overwrite it) and the worst usage.
- `warden_stats` counts executions, downgrades, timeouts and breaches, and gives
  a downgrade rate and a per-effective-profile breakdown, with `unknown` for an
  empty profile.
- Both statistics echo `window_ms` as the truncated number.
- A tenant reads only its own kernel's records.

The only intended difference is the shared already-canceled admission, which
rejects an authorized read before any work.

## Runnable comparison and regression evidence

The payload-rich primary and `acme` journals gain twelve guard records of each
shape: varied profiles, flags, limits and reasons, zero fields, and a malformed
payload per kind. The harness runs the pre-slice handlers on one copy and the
registered operations on another.

240 steps repeated twenty times cover eight sequences under primary, wrong and
tenant tokens and normal and canceled contexts:

- full reads;
- four chained cursor pages;
- every limit form and cursor form;
- the `issues` filter, its errors and its pages;
- windows;
- tenant-routed, padded, foreign, non-string and invalid tenants;
- secret-named arguments.

Responses are byte-exact, and neither side journals anything. The harness asserts
that the populated records were read: metadata IPs, a positive limit, 13
throttles, downgrade rows with argv0, 13 executions and a non-zero downgrade
count. The only exceptions are 43 canceled authorized steps, which return the
admission error.

Permanent tests cover:

- the page codec;
- both logs' exact rows, paging, windows, the empty wire and errors;
- the rate statistics, including a trailing zero limit, windows and echoes;
- the warden log's exact per-kind rows, the strict `issues` with no read on
  error, the filter and its paging, windows and errors;
- the warden statistics with separate downgrade and timeout counts, the rate,
  the profile breakdown, windows and the empty wire;
- the specs, and the output schemas against real outputs;
- the native registry flags;
- per-token routing across a primary and a tenant kernel.

Thirty-six independent mutations fail tests, including the native registration
and the routed-kernel binding. Mutation testing found two gaps, now closed: a
trailing zero rate limit, and a fixture where downgrades and timeouts always
coincided. Sources are restored byte-for-byte. Fixtures use isolated temporary
kernels only.
