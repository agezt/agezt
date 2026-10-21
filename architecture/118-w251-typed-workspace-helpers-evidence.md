# W2.51 typed workspace helpers

The agent **workspace helpers** now live in the application layer, in
`kernel/app/roster/workspace.go` beside the impact service that shows them.
The control plane keeps only the host's data access.

With this slice, the roster domain's derived logic is wholly in the
application layer: what remains native on this domain is data access and the
host's own helpers.

## What moved

`kernel/controlplane/roster_workspace.go` survives, but only as the host's
bindings: the filesystem and the base directory. The DECISIONS — which path
is safe to look at, what the label says, where the workspace root is — moved
to the application layer:

```go
func WorkspaceInfo(ports WorkspacePorts, p core.Profile) (string, bool)
func WorkspaceRoot(baseDir string) string
```

The host binds the filesystem:

```go
type WorkspacePorts struct {
    Root      func() string
    Stat      func(dir string) (exists bool, isDir bool, err error)
    CountTree func(dir string) (files int, bytes int64)
}
```

The file counter (the directory walk) stays in the host: it is the host's
filesystem access, and the application asks for its result.

## The semantics are unchanged

The helpers are moved verbatim:

- an agent with no workdir has no workspace;
- a path outside the workspace root, the root itself, a path that is not a
  directory and a path that is not there all have no description;
- the description is the slash-normalized workdir with the file count and the
  byte count: `ops (3 file(s), 42 bytes)`;
- the root is the `AGEZT_WORKSPACE` override when it is set, and the base
  directory's `workspace` folder otherwise — a blank override falls back to
  the default.

## The wire shape is unchanged

The impact preview's workspace line is untouched: the application already
owned the presentation, and the helper still answers the same question with
the same label. The existing end-to-end suite — which drives the real
operation over a real kernel — passes unchanged.

## Runnable evidence

Permanent tests cover, at the application layer:

- the workspace description: the label with the file count and the bytes;
- a profile without a workdir;
- a path outside the workspace root;
- the workspace root itself;
- a path that is not a directory;
- a path that is not there;
- the root resolution: the base directory's default, the environment override
  and a blank override;
- a real directory tree: the file counter behind the port produces the label
  the impact preview shows.

At the control plane, the existing roster suite passes unchanged.

The mutations fail tests: an outside path described, the root described, a
missing path described, a file described, the file count missing from the
label, the byte count wrong, the environment override ignored, a blank
override winning and the base directory's fallback changed. 10 mutations: 9
caught, and one recorded equivalent — the empty-workdir guard is redundant
with the confinement check below it (an empty path is not confined under the
root), so removing it changes no behavior.

The gates pass: the build, `gofmt`, `go vet`, `tools/deadcodecheck`,
`tools/archcheck` (268 packages, 129 allowlisted import violations and 13
allowlisted call sites) and `tools/structure-md`.

## Not done here

The roster domain is done: every derived decision lives in the application
layer, and what the control plane keeps is the host's data access. The next
slices belong to the other order-7 domains — steer, journal, edict, tenant,
shutdown and remote.

The cloned-kernel harness of the earlier slices — the pre-slice handler and the
registered operation on two kernels, compared step by step — is not part of
this slice, as in W2.44b through W2.50. The roster's own end-to-end suite is
the parity proof here: it drives the real operation over a real kernel and
reads the journal back.
