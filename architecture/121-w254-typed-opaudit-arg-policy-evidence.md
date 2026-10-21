# W2.54 typed operation-audit argument policy

The operation audit's **argument policy** now lives in the application layer,
in `kernel/app/opaudit/opargs.go`. The control plane keeps only the host's
data access.

The audit step journals every state-changing command: `op.invoked` before
the handler runs, `op.completed` or `op.failed` after. What goes into those
journal payloads — which arguments are secret, how a long value is bounded,
what a nested value looks like when only its shape is journaled — is a
policy, and it was the last policy of that kind the control plane held.

## What moved

`kernel/app/opaudit/opargs.go` owns it now:

```go
const ValueMax = 160

func IsSecretArg(name string) bool
func TruncateValue(s string) string
func SummarizeArgs(args map[string]any) map[string]any
```

The native `auditValueMax`, `secretArgFragments`, `secretArg` and
`auditArgs` are gone from `kernel/controlplane/dispatch_audit.go`, and the
typed path's call site in `kernel/controlplane/app_host.go` asks the
application instead of keeping a second copy.

The package is a new one: `kernel/app/audit` owns the **guard** audit reads
(egress blocks, run-rate throttles, sandboxed executions) folded from the
journal, which is a different audit. The operation audit — what a command's
own arguments look like in the journal — gets its own home.

## The two paths share one implementation

The policy had two callers and therefore one implementation:

- the **native** dispatch path (`dispatch_audit.go`) journals the raw
  transport request's arguments;
- the **typed** path (`app_host.go`) journals the operation's decoded input.

Both now call `opaudit.SummarizeArgs`, so the two audit trails cannot drift
apart. The typed path's error message is bounded by the same
`opaudit.TruncateValue` as the native path's.

## The semantics are unchanged

The policy moves verbatim:

- a secret argument is one whose **name** contains one of thirteen fragments
  — key, token, secret, password, passwd, passphrase, cred, auth, cookie,
  value, private, signature, code, state — matched case-insensitively;
  its value is journaled as `[redacted]`;
- a string argument is journaled verbatim, bounded to 160 **runes** with an
  ellipsis;
- a boolean, a number and an integer are journaled verbatim;
- a list is journaled as `[list]`;
- an object is journaled as `[object: k1,k2]` with its keys sorted, so a
  secret nested inside it cannot reach the journal;
- any other value is journaled as `[value]`;
- a nil argument is not journaled at all;
- an empty argument set summarizes to nothing, so the payload carries no
  empty `args` member.

## The wire shape is unchanged

The journal payloads are byte-for-byte the same: the existing control plane
suite — which asserts that a config-center write's audit payload never
contains the secret, that `args["key"]` and `args["value"]` are
`"[redacted]"`, and that the native and the typed paths journal the same
`op.invoked`/`op.completed`/`op.failed` pairs — passes unchanged.

## Runnable evidence

Permanent tests cover, at the application layer:

- the secret names: every fragment, the case-insensitive match and the plain
  names that are not secret;
- the bound: the unchanged short value, the bounded long one with its
  ellipsis, the rune (not byte) count and the multi-byte value that is not
  cut inside a character;
- the summary: the empty and the nil argument set, the scalars, the two
  secret arguments, the list shape, the sorted object shape, the unknown
  shape, the dropped nil argument, the bounded string value and the check
  that the summary never carries the secret itself.

At the control plane, the existing audit, dispatch and config-center suites
pass unchanged.

The mutations fail tests: the unread key and value fragments, the
unnormalized name, the fragment that must match the whole name, the
argument that is never secret, the unbounded value, the bound that is one
rune too long, the dropped ellipsis, the value cut by bytes, the empty
arguments that summarize to a map, the unredacted secret, the different
redaction marker, the unbounded string value, the journaled nil argument,
the different list shape, the unsorted object keys, the different object
shape, the different unknown shape and the shape-only scalars. 19 mutations,
19 caught. No equivalent mutation is recorded for this slice.

The gates pass: the build, `gofmt`, `go vet`, `tools/deadcodecheck`,
`tools/archcheck` (270 packages, 129 allowlisted import violations and 13
allowlisted call sites) and `tools/structure-md` (the generated structure
document gains the new package).

## Not done here

The audit **event** itself — the `op.invoked`/`op.completed`/`op.failed`
journal payloads, their subject, their actor and their correlation — is the
host's to publish, and it stays there. What moved is the policy that decides
what goes into a payload.

The cloned-kernel harness of the earlier slices — the pre-slice handler and
the registered operation on two kernels, compared step by step — is not part
of this slice, as in W2.44b through W2.53. The control plane's own audit
suite is the parity proof here: it drives real commands over a real kernel
and reads the journaled payloads back.
