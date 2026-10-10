# W2.42g typed node registry

The node registry, `node_registry`, now runs on the shared dispatcher in the
new `kernel/app/nodes` package. The Web UI reads it through `/api/nodes`. It
lists the local daemon and each configured `AGEZT_PEERS` mesh node with its
probed reachability.

The peer-list parser moved verbatim as `ParsePeers`. The remote run mirror's
peer lookup now uses it too, with `nodePeer` replaced by `nodes.Peer`.

Reading the peer list, `nodePeerSpec`, stays in the controlplane's `nodes.go`
unchanged. It is the shared source for both callers: the environment first,
then the vault, then the settings store.

`Service.Registry` reads four ports:

- the kernel's model;
- whether a `remote_run` tool is present, and whether it is non-nil;
- the peer list;
- the HTTP client for the probes, bound to the operator client, which allows
  loopback and private addresses.

`Operations` declares one read-only, primary-only, primary-tenancy spec on
`/api/nodes`, with unknown input allowed.

Removed with the move:

- the handler, the parser, the probe and the response limit, from `nodes.go`;
- its registration in `registerCognitionCommands`.

## Preserved behavior

- The local row carries the version, the kernel's model (an empty string when
  there is none) and the `controlplane`, `webui` and `agent-runtime`
  capabilities, plus `remote-run` when a tool by that name is present.
- Peers are listed in name order and probed one at a time.
- Each probe asks `/api/v1/health`, sending the token as a bearer when set.
  - It has three seconds and is independent of the request.
  - The row reports `auth` as `token` or `none`, never the token.
- A peer row is unreachable with the transport error, `401 (token rejected)`,
  `status <code>` outside 2xx, or `bad health response: …` for a body that is
  not JSON, read up to 1 MiB.
- Otherwise the row takes the health status, version and model count. It is
  reachable only for `ok`, and other statuses also set `status=<status>` as the
  error.
- `remote_run_registered` reports a non-nil `remote_run` tool.
- An unparsable list returns only the local row, a peer count of 0 and the
  parser's error, without `remote_run_registered`.
- Rows keep the legacy map shape. Keys a row never had stay absent, and keys
  with empty values stay present, through pointer and `omitempty` fields.
- Any `tenant` argument is ignored, and tenant tokens are refused.

The only intended difference is the shared already-canceled admission. It
rejects an authorized call before any probe.

## Runnable comparison and regression evidence

The harness runs the pre-slice handler on one kernel and the registered
operation on another, against the same loopback peers. 216 steps repeated
twenty times cover twelve fixtures under primary, wrong and tenant tokens, in
normal and canceled contexts:

- no list, and a blank one;
- five unparsable lists: no `=`, no URL, a duplicate name, a non-HTTP scheme,
  no host;
- ten peers listed out of order, with padding and trailing slashes. They
  answer ok, degraded, an empty object, garbage, 502, 204, 401 without and with
  a wrong token, or 404 under a nested path, or refuse the connection. This
  fixture runs with and without `remote_run`;
- a list only in the settings store;
- the environment over the store;
- a nil `remote_run` tool.

Responses are byte-exact, the journals are equal, and no read journals
anything. The following are asserted:

- the model, including HTML escaping and the empty model;
- the capabilities, both counts and the flag;
- each parser error and each probe error;
- name order and token redaction.

The exceptions are 36 canceled primary-token steps, which return the admission
error.

Permanent tests cover:

- the parser's trimming, skipping and every error;
- the registry over a live peer for each answer and a refused one, the flag and
  capability split, the unparsable path and the empty model;
- the spec and its schemas;
- a native binding test that checks:
  - the kernel's model and `remote_run` tool;
  - the settings-store list, overridden by the environment;
  - token redaction;
  - the registry flags.

The existing registry and remote mirror suites pass unchanged.

Twenty-five independent mutations fail tests.

Sources are restored byte-for-byte. Fixtures use isolated temporary kernels and
loopback test servers only.
