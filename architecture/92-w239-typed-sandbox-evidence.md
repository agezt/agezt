# W2.39f typed sandbox inspection

The three sandbox commands now run on the shared dispatcher in the new
`kernel/app/sandbox` package. They let the operator see, read and remove the
persistent projects agents build with the code_exec tool (M686), under
`<home>/sandbox/projects/<name>`:

- `sandbox_list` lists every project and its files;
- `sandbox_file` reads one file;
- `sandbox_delete` removes one project.

This completes the daemon-ops group.

`Service` works over the primary kernel's projects root. `Operations` declares
three primary-only, primary-tenancy specs with unknown input allowed, matching
the native registration and the Web UI routes:

- `sandbox_list`, read-only, on `GET /api/sandbox`;
- `sandbox_file`, read-only, on `GET /api/sandbox_file`;
- `sandbox_delete`, audited, on `POST /api/sandbox/delete`.

The lexical `confineUnder` check moves unchanged to
`platform/fileworkspace.ConfineUnder`. Agent teardown and the workspace file
counts already used it, and now call it there.

Removed with the move:

- `sandbox.go`, which held the three handlers, the project view, the confinement
  helper and `asInt64`;
- their registrations, and with them `registerDaemonOpsCommands`, which had no
  native command left.

The planted-link escape pin, `TestSandboxFile_PlantedLinkEscapeFailsClosed`,
now drives the typed operation through the dispatcher.

## Preserved behavior

- **`sandbox_list`:**
  - Only directories are projects, at most 500, most recently modified first.
  - Each lists at most 500 files by slash path and size in name order, with the
    file count, total bytes and newest modification time.
  - A missing projects root is an empty list.
- **`sandbox_file`:**
  - `project` and `file` are strict: absent or blank is required, a non-string
    is an error. They pass untrimmed.
  - The project is confined under the root, and the file under the project.
  - Links are resolved, and the resolved path must still sit under the
    projects root.
  - Directories and missing files are refused.
  - Content is capped at 256 KiB, with `truncated` set and the full size
    reported. The file echoes in slash form.
  - Every error text is unchanged.
- **`sandbox_delete`:** the target must be a direct child of the projects root,
  never the root itself or a nested path, and must be a directory. The project
  echoes untrimmed.
- Any `tenant` argument is ignored, and tenant tokens are refused.

The only intended difference is the shared already-canceled admission. It
rejects an authorized call before any read, audit or removal.

## Runnable comparison and regression evidence

Each run builds its own project tree with fixed modification times, because
cloning would reset them. The tree holds:

- nested files;
- an HTML-bearing file;
- a file 7 bytes over the cap;
- two tied projects;
- a project with a space in its name;
- an empty project;
- a loose file at the root.

The harness runs the pre-slice handlers on one copy and the registered
operations on another.

186 steps repeated twenty times cover five sequences under primary, wrong and
tenant tokens, in normal and canceled contexts:

- listings with ignored and tenant arguments;
- nested, backslash, padded, capped and spaced reads;
- every argument error and traversal form, including a NUL byte, a
  cross-project path, a directory and a missing file;
- deletes interleaved with listings, including a repeat;
- every refused delete form, with tenant and secret arguments.

The comparison checks three things:

- responses are byte-exact; only the per-run root is masked, where error texts
  name it;
- the project trees are equal;
- both journals, grouped by correlation, are equal (the delete's operation
  audit).

The harness asserts file contents, the slash form, the padded project echo,
the cap and full size, the illegal-path and resolve errors, the deletes and the
shrinking listings. The exceptions are 31 canceled primary-token steps. These
return the admission error and leave no audit record or removal.

Permanent tests cover:

- **`ConfineUnder`:** accepted, rejected and platform-specific absolute forms,
  plus a sibling directory sharing the root's prefix.
- **List:** a missing root, ordering, the empty-project array, and both caps.
- **File:**
  - every argument error, confinement and the cross-project path;
  - a directory and a missing file;
  - untrimmed and slash echoes;
  - the cap, and a planted link where links are available.
- **Delete:** every refusal leaving the tree unchanged, the untrimmed echo, and
  the removal.
- **Operations:** the specs and output schemas.

The escape pin passes through the typed path, as do the existing sandbox, agent
teardown, workspace, impact and remove suites.

Thirty-one independent mutations fail tests. Three first survived and closed
real gaps:

- the 500-project cap had no test;
- confining a file to the projects root instead of its project, which would let
  one project's read reach another project's file, had no test;
- a prefix check without the path separator, which accepts a sibling such as
  `../projects-x`, had no test.

Each now has a test.

Sources are restored byte-for-byte. Fixtures use isolated temporary kernels
only.
