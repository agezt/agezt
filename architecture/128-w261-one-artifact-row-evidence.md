# W2.61 the artifact row has one declaration

The artifact metadata row was described **four times**, in four places that
each had their own copy of the same eleven fields:

| where | the row |
|---|---|
| `kernel/artifact.Entry` — the store's row, persisted per arrival | the fields, with the json names |
| `kernel/contract/remoteapi.Artifact` — the peer's wire (W2.60) | the same fields, with the json names |
| `kernel/restapi.ArtifactEntry` — the serving adapter (W2.60) | an alias of the contract row |
| `cmd/agt/peers_fetch.go`'s `peerArtifactEntry` — the CLI's reader | the fields again, with the json names |

And the store's row was **copied** into the wire row by the daemon's REST
adapter (`cmd/agezt/api_engine.go`'s `restArtifactEntry`) — eleven fields,
copied one by one, on every artifact the API served.

With this slice the store's row and the CLI's row are the contract row, and
the copy is gone.

## What moved

- **the store's row** — `artifact.Entry` is an alias of the contract row. The
  store (L3) may import the contract (L1) ✓. The **on-disk shape is
  unchanged**: the contract row carries exactly the same json names, in the
  same order, with the same `omitempty` behaviour.
- **the CLI's row** — `peerArtifactEntry` is an alias of the contract row: the
  CLI reads a peer's wire with the same declaration the serving daemon writes
  it with.
- **the copy** — `cmd/agezt/api_engine.go`'s `restArtifactEntry` is deleted;
  the store hands its row to the REST adapter as it is.

One spelling changed, and only a spelling: the store's row now says
`CreatedMS` (the Go initialism), where it said `CreatedMs`. The **wire key is
unchanged** — `created_ms` — and so is the on-disk key.

## What did not move

The **application's own artifact row** (`kernel/app/artifacts.Entry`) keeps its
declaration: it is the typed operation's own output wire — it spells the
correlation `CorrelationID` and it keeps the empty keys (`"name":""` rather
than an absent key). That is a different promise from the peer's wire, so it
stays a different row; the copy from the store's row into it remains, with the
renamed field.

## Runnable evidence

Permanent tests cover:

- the **identity** of the three sides that now share a declaration: the
  store's row, the CLI's peer row and the contract row are the same type (the
  REST adapter's and the mirror's identities are already covered by W2.60);
- the **on-disk shape**: the stored metadata file keeps the keys it always
  had, and the empty ones stay out of it;
- the **contract row's wire**: the exact json keys, the omitted empty fields
  and the reading of a peer's answer (W2.60's tests);
- the **application's listed row**: the test now asserts the *values* the
  store holds under every key, not only that the keys are there — so a field
  that arrives empty, wrong or under another name is visible.

The mutations fail tests: **12 mutations, 12 caught** — the renamed wire keys
(created_ms, corr, size), the dropped caption and sender fields, the name that
keeps its empty key, the store's row and the CLI's row that become their own
structs, the listed row that drops the sender or the created time, the listed
row that swaps the correlation for the sender, and the arrival that is not
stamped with the clock.

The gates pass: the build, `gofmt`, `go vet`, `tools/deadcodecheck`,
`tools/archcheck` (271 packages, 129 allowlisted import violations and 13
allowlisted call sites — the ratchet is unchanged), `tools/structure-md` (no
new package) and staticcheck. The affected suites — the store, the
application's artifacts, the contract row, the mirror, the REST adapter and
the two commands — pass at count=20.

## Not done here

The application's own artifact row (`kernel/app/artifacts.Entry`) and the
data-lake's records (`kernel/datalake`, `kernel/app/data`) still have their
own declarations and their own spellings (`CreatedMs`); they are their own
wires, not the peer's.

The cloned-kernel harness of the earlier slices — the pre-slice handler and
the registered operation on two kernels, compared step by step — is not part
of this slice, as in W2.44b through W2.60. The store and control plane suites
are the parity proof here.
