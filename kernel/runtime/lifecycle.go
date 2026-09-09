// SPDX-License-Identifier: MIT

// This file is intentionally empty. The kernel's run-lifecycle
// surface moved to the kernel/runtime/lifecycle sub-package on
// Day 12 of the runtime split. The thin pass-through methods that
// preserve the legacy *Kernel.Halt / *Kernel.Resume / etc. public
// API now live in runtime.go; the actual implementations live in
// the sub-package's Manager.
package runtime
