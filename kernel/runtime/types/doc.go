// SPDX-License-Identifier: MIT

// Package types is the shared-type home for the value types the
// kernel/runtime sub-packages pass across the host/sub-package
// boundary. Extracted as a leaf package on Day 17 so the value
// types do not drag the kernel/runtime import graph along with
// them — types are the cheapest possible unit to share.
//
// Types is intentionally tiny: it holds the value types
// (SubAgentLimits, PluginInfo, CouncilMember) and nothing else.
// It imports the kernel packages that define the underlying
// values (skill, roster, agent) and re-exports them through
// type aliases so callers in kernel/runtime can write
// `types.SubAgentLimits` without taking a new dependency on
// the source package.
//
// The Voice type alias for kernel/voicetool.Voice lives in
// kernel/runtime/toolseams.go (a pre-Day-17 location) for
// historical reasons; it could move here in a future slice.
package types
