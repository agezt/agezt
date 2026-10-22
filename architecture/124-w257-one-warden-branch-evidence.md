# W2.57 one warden branch

The warden-family execution profile was resolved **twice**: once by the
direct run path, in the application layer, and once by the dispatched seat
path, in the control plane. The control plane's copy even said so in its own
comment ("It mirrors the warden branch of the run handler"). With this slice
there is one implementation, and the application owns it.

## What moved

`kernel/app/runs/run_profile.go` (new) gains:

```go
func WardenProfile(ctx context.Context, k *runtime.Kernel, id string) (context.Context, string, error)
```

It resolves a warden-family execution profile id (local, warden, container),
refuses an id the run tools do not route, refuses the container profile
without an active container backend, and layers the profile's sandbox
override onto the context.

Both callers use it now:

- the **direct run path** (`kernel/app/runs/run_resolve.go`) — its inline
  warden branch is gone, replaced by the call;
- the **dispatched seat path** (`kernel/controlplane/workboard_dispatch.go`) —
  its copy of the branch is gone, replaced by the call.

The control plane keeps what is its own: the id's trimming, the empty id's
short-circuit, the environment policy gate and the rejection of the remote
backends (ssh, k8s, modal, daytona, remote-agezt), which only a direct run
can use. Its decision about *which* backends a dispatched seat may not have
stays in the host; the resolution of a warden profile does not.

## The semantics are unchanged — with one wording change

The decision is moved verbatim: the same routability check, the same
container-backend check, the same sandbox override, the same effective label.

One thing changes, and it is operator-visible: the dispatched seat path now
reports the **application's** refusals instead of its own terser ones.

| case | before (dispatched seat) | after |
|---|---|---|
| an id the run tools do not route | `"x" is not a routable execution profile` | `execution profile "x" is not routable for run tools yet (supported: local, warden)` |
| the container profile without its backend | `"x" requires an active container backend` | `execution profile "x" requires an active container backend (set AGEZT_WARDEN_DOCKER=1 and configure AGEZT_WARDEN_DOCKER_IMAGE/runtime)` |

The new wording is the direct run path's wording — the same refusal, said
the same way on both paths, which is the point of the slice. The policy
refusal ("blocked by policy: …") and the remote-backend refusal ("seat
isolation … is only available on direct runs, not workboard dispatch") are
unchanged, and their order is unchanged: the policy gate still runs before
the remote-backend rejection.

## Runnable evidence

Permanent tests cover, at the application layer:

- the warden-family profile: the effective label, the sandbox override
  layered onto the context, and the upper-case id;
- the unroutable id: the refusal, the id named in it and the supported
  profiles named in it;
- the container profile without its backend: the refusal and the message
  that says how to get the backend.

At the control plane, the existing run, profile and workboard suites pass
unchanged.

The mutations fail tests: the accepted unroutable profile, the unsaid
routability refusal, the unsaid supported profiles, the unchecked container
backend, the unsaid container backend refusal, the dropped effective label,
the unlayered sandbox override, the profile that always resolves to the
local one and the override that is always the container profile. Nine of the
ten mutations fail tests; the tenth — trimming the id before the lookup — is
**equivalent**, because `executionprofile.WardenProfileForRun` already trims
(and lower-cases) the id it is given.

The gates pass: the build, `gofmt`, `go vet`, `tools/deadcodecheck`,
`tools/archcheck` (270 packages, 129 allowlisted import violations and 13
allowlisted call sites) and `tools/structure-md` (no new package).

## Not done here

The remote backends' list — ssh, k8s, modal, daytona and remote-agezt — still
appears in two places, but with **opposite** meanings: the application
supports them for a direct run, and the host refuses them for a dispatched
seat. Unifying that list would mean moving the dispatched seat's policy into
the application, which is a separate decision.

The projections that convert **runtime** types (the council's
`runtime.CouncilMember` and the runtime's plugin registration) stay where
they are: the application layer cannot import `kernel/runtime`, and the
runtime cannot import the application layer without breaking the layer
rule. They are listed as candidates in `architecture/NEXT.md`.

The cloned-kernel harness of the earlier slices — the pre-slice handler and
the registered operation on two kernels, compared step by step — is not part
of this slice, as in W2.44b through W2.56. The run and workboard suites are
the parity proof here.
