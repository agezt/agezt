# W2.39e typed Mission Control reads

The two Mission Control status reads behind the console's spend tile and
"needs your attention" panel now run on the shared dispatcher in the new
`kernel/app/missioncontrol` package:

- `spend_today` returns the governor's spend so far today;
- `attention` returns one time-sorted, capped feed of pending approvals and
  pulse asks.

`Service` works over four ports bound to the primary kernel:

- today's spend, reported only when the kernel's provider is the governor;
- the approval registry's pending requests, none without a registry;
- the injected pulse's pending asks, none when pulse is disabled;
- the daemon clock.

`Operations` declares two primary-only, primary-tenancy specs, read-only and
unaudited, with unknown input allowed:

- `spend_today` on `GET /api/spend/today`;
- `attention` on `GET /api/attention`.

These match the native registration and the Web UI read routes.

Removed with the move:

- `handle_spend_attention.go`, which held both handlers, the argument parser
  and the approval-summary helper;
- their registrations;
- the internal `attentionArgs` test. Its defaults, happy-path, bad-input, cap
  and numeric cases now sit in the app package's argument test, beside the
  added edge cases.

## Preserved behavior

- **`spend_today`** returns exactly `{"total":…}` in microcents. The total is
  zero when no governor tracks spend, so the tile stays a calm "0¢".
- **`attention`** never rejects its arguments:
  - `window` is a trimmed positive duration string or a positive number of
    seconds; otherwise it is 24 hours.
  - `limit` is a trimmed positive count string or a positive number truncated
    toward zero; otherwise it is 8, capped at 50. A numeric limit below one
    therefore truncates to an empty feed, as before.
- **Feed contents:**
  - Every pending approval is included whatever the window, with a one-line
    summary: the tool and reason, the capability and requester ("agent" by
    default) and reason, or the approval id.
  - Pulse asks are included only with an issue key, an integer raise time and
    a time inside the window.
- **Feed order and shape:** items run newest first, with id order breaking
  ties, then truncate to the limit. The list is always an array.
- Any `tenant` argument is ignored, and tenant tokens are refused.

The only intended difference is the shared already-canceled admission. It
rejects an authorized call before any read.

## Runnable comparison and regression evidence

The harness opens fresh kernels for every run, because approvals are live
requests. It covers five fixtures:

- plain;
- a governor that has accrued real spend (1,050,000 microcents) through a
  priced mock completion;
- four live approvals submitted a few milliseconds apart, covering every
  summary form, including an HTML reason;
- a pulse with asks that are fresh, older, stale, keyless, float-timed and
  tied;
- all three together.

It runs the pre-slice handlers on one copy and the registered operations on
another.

450 steps repeated twenty times send the spend tile three requests and the feed
twelve argument shapes, under primary, wrong and tenant tokens, in normal and
canceled contexts. The feed shapes are:

- the defaults;
- padded strings;
- negative, zero and unparsable values;
- numeric seconds and a fractional limit;
- sub-one values;
- boolean and `null` values;
- caps by number and by string;
- a named tenant with secret-named arguments.

Responses are byte-exact, apart from the fresh approval ids and their creation
times. The approval order still shows in the summaries. No journal writes occur
beyond the fixtures' own approval requests.

The harness asserts:

- the zero and governed totals;
- the eight-item default feed with the escaped tool summary;
- the three-item narrowed feed;
- the empty feed for a sub-one limit.

The exceptions are 75 canceled primary-token steps. These return the admission
error.

Permanent tests cover:

- the spend total, including 2^53+1;
- every argument form;
- every summary form;
- the feed's inclusion rules, cutoff edge, order, ties, truncation and empty
  forms;
- the specs and output schemas.

Native tests cover the registry flags and the three bindings:

- the governor's real spend;
- the live approval registry;
- an injected pulse whose fresh ask leads a one-item feed.

The existing Mission Control suites pass unchanged.

Twenty-seven independent mutations fail tests. They cover:

- the defaults and cap;
- every argument rule;
- the summary forms;
- the window rules for approvals and asks;
- the key filter and the ask link;
- both orderings;
- truncation and the array shape;
- the route and provider guard;
- the native registration, spend, approval and pulse bindings.

Sources are restored byte-for-byte. Fixtures use isolated temporary kernels
only.
