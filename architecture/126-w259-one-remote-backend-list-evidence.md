# W2.59 one remote-backend list

The remote execution backends — ssh, k8s, modal, daytona and remote-agezt —
were named in **four** places, with three different sets:

| where | the names |
|---|---|
| the application's run path (`kernel/app/runs/run_resolve.go`) | the five, one per `switch` case |
| the control plane's dispatched seat (`kernel/controlplane/workboard_dispatch.go`) | the five, in one `case` |
| the `code_exec` tool's artifact export (`plugins/tools/codeexec/artifacts.go`) | four — all but remote-agezt, which never runs that tool |
| the profile inventory (`kernel/executionprofile/profile_*.go`) | the five, as literal ids |

The two host-side lists had **opposite meanings**: the application supports a
remote backend for a direct run, the control plane refuses it for a dispatched
seat. Two literal lists with opposite meanings are two places to forget a
name. With this slice the *names* are one list; the policies stay where they
belong.

## What moved

`kernel/executionprofile/remote_backends.go` (new) owns the names — the
package that already owns the profile inventory:

```go
const (
	IDSSH         = "ssh"
	IDK8s         = "k8s"
	IDModal       = "modal"
	IDDaytona     = "daytona"
	IDRemoteAgezt = "remote-agezt"
)

var RemoteBackendIDs = []string{IDSSH, IDK8s, IDModal, IDDaytona, IDRemoteAgezt}

func IsRemoteBackend(id string) bool
```

The predicate ignores the case and the surrounding spaces, because the caller
that passes a user-supplied profile id promises neither.

The four consumers now name the backends through it:

- **the inventory** (`profile_remote.go`, `profile.go`) — the five profiles
  carry the constants as their ids, and `RoutableRunProfileIDsFor` looks the
  profiles up by them;
- **the application's run path** (`kernel/app/runs/run_resolve.go`) — the
  switch's cases are the constants, so a name cannot be swapped for another;
- **the dispatched seat** (`kernel/controlplane/workboard_dispatch.go`) — its
  literal `case "ssh", "k8s", …` is gone, replaced by
  `executionprofile.IsRemoteBackend(id)`;
- **the `code_exec` tool** (`plugins/tools/codeexec/artifacts.go`) — its
  literal `profile == "ssh" || profile == "k8s" || …` is gone, replaced by the
  same predicate, with the remote-agezt exception kept (that profile never
  runs the `code_exec` tool — its artifacts live on the peer).

The `code_exec` tool is a plugin and the control plane is an adapter; both
may import the L2 platform, and `kernel/executionprofile` is L2 — so the one
list is reachable from everywhere that needs it.

## What did not move

The **policies** are unchanged and stay where they were:

- the application's run path still resolves a remote backend when it is
  active, and still refuses it with its own message when it is not
  (`execution profile "ssh" requires an active SSH backend …`);
- the dispatched seat still refuses every remote backend with the same
  message (`seat isolation %q is only available on direct runs, not
  workboard dispatch`);
- the `code_exec` tool still exports the remote backends' artifacts from the
  local root and the local profile's from the `.agezt-artifacts` subdirectory.

W2.57's evidence doc had named "unifying that list" as a separate decision.
The decision it referred to was about moving the dispatched seat's *policy*
into the application; this slice unifies the *names* only, and the two
policies keep their opposite meanings on purpose.

## Also removed

`boardReadDefaultLimit` — the dead constant in `kernel/controlplane/board.go`
(no reference anywhere; only `boardReadMaxLimit` is used) — is gone. It was on
the removal list in `architecture/NEXT.md`.

## Runnable evidence

Permanent tests cover:

- the list itself: its content and its order, no duplicate, every id
  recognized by the predicate;
- the predicate: each of the five ids, the upper-case and the padded spellings,
  and the ids that are **not** remote (the empty id, local, warden, container,
  docker, and near-misses like `sshx`);
- the inventory: each remote profile answers to the id the list names, and the
  routable ids name nothing unknown and nothing twice;
- the routable ids with every backend active (each is named, each exactly
  once) and with one backend active at a time (that one is named and the
  others are not);
- the dispatched seat: every remote backend is refused with the same message
  and no label, the empty id is not a profile, and the warden profile still
  resolves;
- the run path: every remote backend reports **its own** missing backend
  (`ssh` says SSH, `k8s` says Kubernetes, `modal` says Modal, `daytona` says
  Daytona, `remote-agezt` asks for the peers), and the id is matched without
  regard to the case.

The `code_exec` tool's rule keeps its existing coverage: the ssh, k8s, modal
and daytona export tests (the artifact comes from the local root) and the
local export test (the artifact comes from the subdirectory).

The mutations fail tests: **19 mutations, 18 caught, 1 recorded equivalent.**
The first pass caught 17 and left two open —

- the routable-ids lookup that answers with another backend: a **test gap**,
  because the test only built an inventory with every backend active, where
  the wrong lookup still produced the right name. The test was strengthened
  (one backend active at a time) and the mutation is caught on the repeat;
- the `code_exec` tool's remote-agezt exception: **equivalent**, because that
  profile never runs the `code_exec` tool, so removing the exception changes
  nothing observable.

The caught mutations: the predicate that forgets the k8s, the remote-agezt or
the daytona backend, the predicate that calls every profile remote or no
profile remote, the predicate that stops ignoring the case or the spaces, the
list that forgets a backend or reorders itself, the run path that resolves k8s
as ssh, modal as daytona or remote-agezt as modal, the dispatched seat that
stops refusing the remote backends or that refuses the warden family too, the
`code_exec` tool that exports the remote artifacts from the subdirectory or
the local ones from the root, the inventory that answers to another ssh id,
and the routable ids that look up the wrong profile.

The gates pass: the build, `gofmt`, `go vet`, `tools/deadcodecheck`,
`tools/archcheck` (270 packages, 129 allowlisted import violations and 13
allowlisted call sites — the ratchet is unchanged), `tools/structure-md` (no
new package) and staticcheck. The affected suites — the execution profile, the
run path, the `code_exec` tool and the control plane — pass at count=20.

One observation is recorded rather than dismissed: an earlier count=20 run of
the control plane's affected tests failed once — the workboard app socket test
hit its five-second pipe deadline ("read pipe: i/o timeout") on one
iteration. The same command then passed (`ok ... 78.370s`), and the test
passes on its own; the slice changes no socket, no deadline and no timeout.

## Not done here

The two **policies** still say opposite things about the same names: the
application supports a remote backend for a direct run, the host refuses it
for a dispatched seat. That is the seat isolation rule, and moving it into
the application is a separate decision (recorded in W2.57's evidence doc).

The cloned-kernel harness of the earlier slices — the pre-slice handler and
the registered operation on two kernels, compared step by step — is not part
of this slice, as in W2.44b through W2.58. The run, profile and control plane
suites are the parity proof here.
