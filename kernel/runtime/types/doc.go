// SPDX-License-Identifier: MIT

// Package types is the shared-type home for the value types the
// kernel/runtime sub-packages pass across the host/sub-package
// boundary. Without it, every sub-package that needs SubAgentLimits
// (or PluginInfo, or CouncilMember) would either (a) duplicate the type
// declaration and lose type identity with the host, or (b) import
// kernel/runtime and re-introduce the very circular-import problem the
// Day 12-13 sub-package split solved.
//
// It was extracted as a leaf package on Day 17 so the value types do
// not drag the kernel/runtime import graph along with them — types are
// the cheapest possible unit to share. The package is intentionally
// tiny: it holds the value types and nothing else. It imports the kernel
// packages that define the underlying values (skill, roster, agent) and
// re-exports them through type aliases, so callers in kernel/runtime can
// write `types.SubAgentLimits` without taking a new dependency on the
// source package.
//
// Rules of the road:
//
//   - This package must stay free of kernel/runtime's private fields. A
//     type here is a *value* the kernel passes through its public
//     surface; it never depends on a *Kernel pointer.
//   - It must compile without any kernel/runtime import. Run
//     `go build ./kernel/runtime/types/...` after every change.
//   - New shared types land here, not in a sub-package — that's what
//     "shared" means in this context.
//
// The Voice type alias for kernel/voicetool.Voice lives in
// kernel/runtime/toolseams.go (a pre-Day-17 location) for historical
// reasons; it could move here in a future slice.
package types
