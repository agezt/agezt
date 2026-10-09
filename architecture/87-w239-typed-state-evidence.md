# W2.39a typed state inspection

The two state-store inspection reads now run on the shared dispatcher in the
new `kernel/app/state` package:

- `state_list` lists every namespace or, for a named namespace, its keys;
- `state_get` reads one key, decoding the stored value to its JSON type.

`Service` works over a `Store` port (`Namespaces`, `Keys`, `Get`) bound to the
primary kernel's state store. `Operations` declares two primary-only,
primary-tenancy, read-only and unaudited specs with unknown input allowed and no
route, matching the native registration. `agt state` is the only client.

Removed with the move:

- `state.go`, which held `handleStateList` and `handleStateGet`;
- their registrations.

## Preserved behavior

- `namespace` and `key` are read leniently: any non-string, including `null`,
  reads as empty. Present strings pass untrimmed, so the store's own namespace
  validation decides what is valid.
- With no namespace, `state_list` returns `{"namespaces":[…],"namespace":""}`
  in the store's sorted order. A named namespace returns
  `{"keys":[…],"namespace":…}`. An unknown namespace has no keys. Both lists
  are always arrays. The store's validation error passes through unchanged.
- `state_get` requires both names, with the same error text as before. A
  missing key is `found:false` with a `null` value, while a stored `null` is
  `found:true`.
- Values decode the way the native handler decoded them. Numbers go through
  float64, so a stored 2^53+1 still reads as 9007199254740992. A value that
  does not decode is reported as `state value corrupt: …`.
- Any `tenant` argument is ignored, and tenant tokens are refused. Both reads
  stay on the primary store.

The only intended difference is the shared already-canceled admission. It
rejects an authorized call before any read.

## Runnable comparison and regression evidence

The harness clones a fixture kernel. The kernel has three primary namespaces
(one mixed-case with punctuation) holding these value shapes:

- an integer, 2^53+1 and `1e21`;
- a fraction;
- a nested object with an HTML string;
- a stored `null`;
- an HTML string;
- an empty key;
- `true`;
- an empty list.

An open `acme` tenant holds a tenant-only namespace. The harness runs the
pre-slice handlers on one copy and the registered operations on another.

204 steps repeated twenty times cover five sequences under primary, wrong and
tenant tokens, in normal and canceled contexts:

- every listing mode, including non-string and `null` namespaces and an
  unknown namespace;
- invalid and padded namespaces, a named tenant and secret-named arguments;
- every value shape;
- every missing-name form, a missing key, an unknown namespace, an invalid
  namespace and a padded key;
- the tenant-only namespace looked up through a named tenant.

Responses are byte-exact, and neither kernel's journal changes. The harness
asserts the sorted namespace and key listings, the empty key list, the rounded
2^53+1 and the found stored `null`. The exceptions are 34 canceled primary-token
steps. These return the admission error.

Permanent tests cover:

- every non-string namespace listing the namespaces, with arrays even when
  empty;
- store order and an untrimmed name for keys, an unknown namespace, and the
  error passthrough;
- every missing-name form, with no store call;
- nested values in member order, float64 numbers, a stored `null`, a missing
  key, an untrimmed key, a corrupt value and the error passthrough;
- the specs and output schemas.

Native tests cover the registry flags. A native round trip checks that both
reads use the primary store whatever tenant is named and never read a tenant's
store, that a validation error passes through, and that a tenant token is
refused. The existing state suites in `kernel/controlplane` and `cmd/agt` pass
unchanged.

Fifteen independent mutations fail tests, including the native registration.
One first-draft mutant was replaced because it could not change behavior:
`found || value != nil` equals `found`, since a value is only decoded for a
found key. Its replacement, `found && value != nil`, fails on the stored
`null`.

Sources are restored byte-for-byte. Fixtures use isolated temporary kernels
only.
