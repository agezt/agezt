# W2.60 the peer's wire has one declaration

A delegated run's peer answers two HTTP endpoints. The rows it sends were
described **three times**, once on each side of the call, and the host copied
the fields of every row it read:

| where | the row |
|---|---|
| `kernel/restapi.ArtifactEntry` — the serving side | the peer's artifact metadata, with the wire's json names |
| `kernel/app/remote.Artifact` — the reading side | the same fields again, **without** json names |
| `kernel/app/remote.Event` — the reading side | the peer's event, field by field, without json names |

The host's two peer fetches (`kernel/controlplane/remote_mirror_fetch.go`)
therefore decoded the peer's answer into an anonymous struct and copied every
field into the application's row — nineteen field copies that could drop a
field, swap a name or answer with a value the peer never sent.

With this slice the wire has **one** declaration on both sides, and the host
decodes the peer's answer straight into the application's rows.

## What moved

`kernel/contract/remoteapi` (new, L1) is the contract between two daemons:

```go
// Artifact is one artifact's metadata as a peer serves it.
type Artifact struct {
	ID        string `json:"id"`
	Ref       string `json:"ref"`
	Name      string `json:"name,omitempty"`
	Mime      string `json:"mime,omitempty"`
	Kind      string `json:"kind,omitempty"`
	Source    string `json:"source,omitempty"`
	Sender    string `json:"sender,omitempty"`
	Corr      string `json:"corr,omitempty"`
	Size      int64  `json:"size"`
	CreatedMS int64  `json:"created_ms"`
	Caption   string `json:"caption,omitempty"`
}
```

The three sides are now the same declaration:

- **the serving side** — `restapi.ArtifactEntry` is an alias of the contract
  row;
- **the reading side** — `app/remote.Artifact` is an alias of the contract
  row, and `app/remote.Event` is an alias of the canonical event
  (`kernel/event`) — the peer's events already travel as the canonical event
  JSON;
- **the host** — the two fetches decode the peer's answer into
  `[]appremote.Event` and `[]appremote.Artifact` and hand them over. Both
  field-copy loops are gone.

One spelling changed, and only a spelling: the row says `CreatedMS` now (the
Go initialism the application already used), where the adapter said
`CreatedMs`. The **wire key is unchanged** — `created_ms` — and the on-disk
and on-the-wire shapes are identical.

## The semantics are unchanged

- the peer's answer is read the same way: the same keys, the same values, the
  same truncation flag;
- the mirror publishes the same rows: the same fields, the same bounds (200
  events, 200 artifacts), the same optional caption;
- the peer's payload still travels only in the modes that allow it, and the
  token still never travels.

## Runnable evidence

Permanent tests cover:

- the contract row's **wire shape**: the exact json keys, the omission of the
  empty fields, and the reading of a peer's answer;
- the **identity** of the three sides: the serving adapter's row, the
  application's row and the contract row are the same type, and the
  application's event row is the canonical event;
- the **host's fetches**: a peer that answers with the wire JSON is read into
  the application's rows field by field — the event's id, seq, ts, subject,
  actor, kind, correlation id, hash and payload, the artifact's eleven
  fields, and the peer's truncation flag;
- the mirror's **published rows**: the existing tests keep their coverage, and
  the artifact row's ref, mime, kind, source, sender and correlation are
  asserted too.

The mutations fail tests: **14 mutations, 14 caught.** The first pass caught
ten of twelve (two anchors did not match the row map's formatting and were
re-anchored) and left one test gap open — the published artifact row's sender
was not asserted — which the strengthened test catches on the repeat. The
caught mutations: the renamed wire keys (created_ms, size, corr), the dropped
caption field, the three sides that become their own structs, the event fetch
that drops the payload, the artifact fetch that drops the created_ms, the
artifact fetch that drops the truncation flag, and the four published rows
that drop a field.

The gates pass: the build, `gofmt`, `go vet`, `tools/deadcodecheck`,
`tools/archcheck` (271 packages — the new contract package — with 129
allowlisted import violations and 13 allowlisted call sites, the ratchet
unchanged), `tools/structure-md` (the new package) and staticcheck. The
affected suites — the contract row, the mirror, the REST adapter and the
control plane — pass at count=20.

## Not done here

The artifact row still has a second home: the store's own row
(`kernel/artifact.Entry`, which the REST adapter copies field by field in
`cmd/agezt/api_engine.go`) and the application's own artifact row
(`kernel/app/artifacts.Entry`, which spells the correlation `CorrelationID`).
They are the store's row and the application's row, not the peer's wire, so
they keep their own declarations — but they are the next candidates for the
same treatment.

The cloned-kernel harness of the earlier slices — the pre-slice handler and
the registered operation on two kernels, compared step by step — is not part
of this slice, as in W2.44b through W2.59. The mirror and control plane suites
are the parity proof here.
