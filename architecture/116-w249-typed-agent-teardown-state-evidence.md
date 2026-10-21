# W2.49 typed agent teardown state cleanup

The agent teardown's **state cleanup** now lives in the application layer, in
`kernel/app/roster/teardown_state.go` beside the remove service that drives
it. The control plane keeps only the host's data access.

The native file `kernel/controlplane/roster_teardown_state.go` (205 lines)
survives, but only as the host's bindings: the memory store, the skill forge,
the config-center and the filesystem.

## What moved

The typed `agent_remove` operation was already app-owned and already owned the
decision (forget the agent's memory? archive its skills? delete its config
entries? delete its workspace?). What it did not own was the **state
cleanup** behind those ports — which memory records belong to the agent, which
skills are the agent's, which config entries belong to it, which access
references name it, and whether a workspace path is safe to delete.

`kernel/app/roster/teardown_state.go` (260 lines) owns it now:

```go
func ForgetAgentMemory(ports TeardownStatePorts, p core.Profile, on bool) (int, error)
func ForgetAgentAuthoredSharedMemory(ports TeardownStatePorts, slug string, on bool) (int, error)
func ArchiveAgentSkills(ports TeardownStatePorts, slug string, on bool) (int, error)
func DeleteAgentConfigEntries(ports TeardownStatePorts, slug string, on bool) (int, int, error)
func DeleteAgentWorkspace(ports TeardownStatePorts, p core.Profile, on bool) (int, error)
```

The host binds the stores:

```go
type TeardownStatePorts struct {
    MemoryRecords func() ([]memory.Record, error)
    ForgetMemory  func(id string) (bool, error)
    Skills        func() ([]skill.Skill, error)
    ArchiveSkill  func(id, reason string) error
    ConfigEntries func() []*configcenter.ConfigEntry
    DeleteConfig  func(key string) error
    SetConfig     func(e *configcenter.ConfigEntry) error
    WorkspaceRoot func() string
    WorkspaceDir  func(dir string) (exists bool, isDir bool, err error)
    DeleteDir     func(dir string) error
}
```

The filesystem is a port, not a dependency: the application decides whether
a path is safe to delete, the host does the deleting. That keeps the decision
testable without a disk and keeps the app layer free of `os.RemoveAll`.

## The semantics are unchanged

The cleanup is moved verbatim:

- the agent's own memory records are the ones whose `scope` tag matches the
  profile's memory scope — the slug when the profile has none — and only the
  records the store actually had are counted;
- the shared records are the **unscoped** ones the agent authored or last
  updated (case-insensitively);
- the agent's skills are archived with the reason `agent removed: <slug>`, and
  a skill that refuses the transition (`skill.ErrIllegalTransition`) is left
  alone instead of failing the teardown;
- the config entries that belong to the agent (by the canonical key prefixes)
  are deleted in sorted order, and the entries that stay are pruned of the
  agent's access references — the counts report the deletions and the prunes
  separately;
- the workspace is deleted only when its path is confined under the workspace
  root and is not the root itself; a path that is already gone is nothing to
  do, and a path that is not a directory is an error.

The switch semantics are preserved: with the switch off, nothing is forgotten,
archived, deleted or pruned.

## The predicates the impact cascade shares

Two of the moved predicates are also used by the impact cascade
(`roster_cascade.go`), so they are exported from the application layer:
`MemoryRecordBelongsToAgent` and `MemoryRecordAuthoredSharedByAgent`. The
cascade now asks the application instead of keeping a second copy of the
rule.

## The wire shape is unchanged

The remove operation's answers, its counts and its journal arc are untouched:
the application already owned the decision, and the cleanup still answers the
same questions with the same rules. The existing end-to-end suite — which
drives the real operation over a real kernel — passes unchanged.

## Runnable evidence

Permanent tests cover, at the application layer:

- the agent's memory: the scoped records (case-insensitively), the profile's
  memory scope, the counting of only what existed, and the switch off;
- the shared memory: the unscoped records the agent authored or last updated,
  the scoped records left alone, and the switch off;
- the skills: the agent's skills archived, the already-archived ones and
  another agent's ones left alone, the illegal transition skipped, the real
  failure reported, and the switch off;
- the config entries: the agent's entries deleted in sorted order, the
  remaining entry pruned of the access reference, and the switch off;
- the workspace: deleted when it is a confined directory, refused when the
  path escapes the root or is the root itself, nothing to do when it is
  already gone or the profile has no workdir, refused when it is not a
  directory, and the switch off.

At the control plane, the existing roster suite passes unchanged.

The mutations fail tests: the switch doing nothing (in all five cleanups),
the scope falling back to nothing, the case-sensitive scope match, the
missing records counted, the scoped records forgotten too, the author match
dropped, the already-archived skills archived again, another agent's skills
archived, the illegal transition turning into an error, the config keys not
sorted, a deleted entry pruned too, the access reference not pruned, the
case-sensitive access reference match, an empty workdir deleted, the root
itself deleted, an outside path deleted, a missing workspace deleted and a
file deleted like a directory. 22 mutations, 22 caught.

The gates pass: the build, `gofmt`, `go vet`, `tools/deadcodecheck`,
`tools/archcheck` (268 packages, 129 allowlisted import violations and 13
allowlisted call sites) and `tools/structure-md`.

## Not done here

The impact cascade (`roster_cascade.go`) and the lifecycle helpers
(`roster_lifecycle_helpers.go`) are still native, and so is the workspace
root helper (`roster_workspace.go`). They are the next slices of the same
domain.

The cloned-kernel harness of the earlier slices — the pre-slice handler and the
registered operation on two kernels, compared step by step — is not part of
this slice, as in W2.44b through W2.48. The roster's own end-to-end suite is
the parity proof here: it drives the real operation over a real kernel and
reads the journal back.
