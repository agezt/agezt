# W2.53 typed remote mirror

The remote run **mirror** now lives in the application layer, in
`kernel/app/remote/mirror.go`. The control plane keeps only the host's data
access.

The mirror is the feature that copies a delegated run's peer events — and the
peer's artifact metadata — back into the local journal, so an operator can
see what the remote peer did. It was the last place in the run domain where
the control plane still made the decisions.

## What moved

`kernel/app/remote/mirror.go` owns it now:

```go
func Mirror(ctx context.Context, ports MirrorPorts, rawMode string, corr string, meta map[string]string)
func EventRows(events []Event, mode string, redact Redactor) ([]map[string]any, bool)
func ArtifactRows(entries []Artifact, peerTruncated bool) ([]map[string]any, bool)
func Mode(raw string) string
func EnvMode() string
func PayloadMode(mode string) string
```

The decisions it owns:

- **whether the mirror runs at all** — the mode is read from the
  configuration (`1`/`true`/`yes`/`on`/`metadata` switch the metadata mode
  on, `redacted`/`payload`/`payload-redacted` the redacted one, everything
  else switches the mirror off);
- **what a mirror that cannot reach the peer looks like** — an
  "unavailable" event that says why, instead of silence;
- **which fields of a peer event travel** — the metadata in both modes, the
  payload only in the redacted mode, and only when the redactor could
  redact it;
- **which fields of a peer artifact travel** — its description, never its
  bytes, and the caption only when the peer said one;
- **how much travels** — at most 200 peer events and 200 peer artifacts, and
  whether the list was truncated.

The host keeps the data access: the peer configuration, the two peer HTTP
fetches, the redactor and the journal publish. The fetches now return typed
rows (`appremote.Event`, `appremote.Artifact`) instead of the wire maps, so
the wire shape is derived in one place — the application.

## The semantics are unchanged

The mirror moves verbatim:

- the mode parsing, including the normalization of the configured value
  (trimmed, lower-cased) and the off state;
- the metadata check: a mirror that never learned the peer or the
  correlation says so and names neither;
- the peer lookup: a peer that is not configured says "peer not found", a
  lookup that fails says why, and both name the peer and the correlation
  that were asked for;
- the fetch failure: the same "unavailable" shape, with the fetch's error;
- the mirrored payload: the profile, the phase, the mode, the payload mode,
  the peer, the correlation, the count, the truncation and the events;
- the artifacts: merged into the payload when they can be read, and the
  failure named (`artifacts_unavailable`) when they cannot — an artifact
  failure never sinks the events;
- the event rows: the id, the sequence, the timestamp, the subject, the
  actor, the kind, the correlation and the hash, plus the redacted payload
  in the redacted mode;
- the artifact rows: the id, the ref, the name, the mime, the kind, the
  source, the sender, the correlation, the size and the creation time, plus
  the caption when the peer said one.

Two of the old helpers leave the control plane with their decisions:
`remoteEventMirrorMode` (the mode policy) and `remoteMirrorPayloadMode` (the
payload mode) are the application's now.

## The wire shape is unchanged

The three end-to-end tests in `kernel/controlplane/remote_mirror_test.go` —
which drive a real run over a real kernel against a real peer server and read
the mirrored payload back — pass unchanged: the metadata mirror, the redacted
payload mirror and the artifact metadata mirror.

## Runnable evidence

Permanent tests cover, at the application layer:

- the mode: every configured value, the normalization, the off state and the
  environment read;
- the payload mode: "redacted" and "none";
- the mirror that is off: it publishes nothing;
- the mirror without metadata: the "unavailable" shape, the error and the
  unnamed peer;
- the unknown peer and the failing lookup: the "unavailable" shape with the
  reason and the named peer and correlation;
- the failing fetch: the "unavailable" shape with the reason and the named
  peer and correlation;
- the mirrored payload: the profile, the phase, the mode, the payload mode,
  the peer, the correlation, the count, the truncation and the event rows;
- the metadata mode: the peer's payload does not travel;
- the redacted mode: the redacted payload travels, and a payload the
  redactor cannot handle does not;
- the artifacts: the merged rows, the count and the truncation, and an
  artifact failure that leaves the events mirrored;
- the bounds: the event list, the artifact list and the reported truncation;
- the artifact caption: said only when the peer said one.

At the control plane, the existing remote mirror suite passes unchanged.

The mutations fail tests: the unread metadata mode, the unread redacted
mode, the unnormalized value, an unknown value that switches the mirror on,
the wrong environment variable, the unnamed payload mode, the honored off
mode, the undetected missing metadata, the undetected unknown peer, the
unsaid lookup error, the unmirrored fetch failure, the wrong mirrored phase,
the dropped profile, the unnamed peer, the unnamed correlation, the wrong
count, the unsaid truncation, the always-none payload mode, the dropped
artifacts, the unsaid artifact failure, the wrong artifact count, the
dropped correlation, the payload that travels in the metadata mode, the
dropped redacted payload, the unbounded event list, the unreported
truncation, the always-said caption, the ignored peer truncation, the
unbounded artifact list, the unnamed peer in the unavailable shape and the
unnamed peer in the unknown-peer shape. 31 mutations, 31 caught.

The run was honest about the order: the first pass attempted 28 mutations
and caught 27 — one anchor matched three places and was skipped. The second
pass fixed the anchors and caught two of the three it attempted; the third
caught the last one, but only after the test was strengthened (the failing
fetch's payload had not asserted that it names the peer and the
correlation). No equivalent mutation is recorded for this slice.

The gates pass: the build, `gofmt`, `go vet`, `tools/deadcodecheck`,
`tools/archcheck` (269 packages, 129 allowlisted import violations and 13
allowlisted call sites) and `tools/structure-md` (the generated structure
document gains the new package).

## Not done here

The mirror's configured value is read from the environment
(`AGEZT_REMOTE_EVENT_MIRROR`). The settings store declares the same variable
(`kernel/settings/schema_builtin.go`) as an `ApplyLive` setting, so a value
set through the store is written into the process environment and the mirror
sees it live — there is one source of truth, not two.

The tenant's run activity still re-projects the folded runs into
`apptenants.Run` in the control plane (`kernel/controlplane/tenant.go`). With
the fold now owned by `kernel/app/runs`, that projection is a duplicate of
the application's own row type; it is the next candidate in the tenant
domain.

The cloned-kernel harness of the earlier slices — the pre-slice handler and the
registered operation on two kernels, compared step by step — is not part of
this slice, as in W2.44b through W2.52. The mirror's own end-to-end suite is
the parity proof here: it drives a real delegated run against a real peer
server and reads the mirrored payload back.
