// SPDX-License-Identifier: MIT

// Package compose is the kernel's composition-root surface extracted
// into a sub-package as the next step of the Day 12-20 sub-package
// split (lifecycle, accessors, types, compose).
//
// The package hosts the Open() factory and the Close()/closeAll()
// drain dance that today live in kernel/runtime/compose.go. The
// extraction is non-trivial: Open() is the only function in the
// kernel that constructs a *Kernel, so the natural sub-package
// boundary is "everything that knows how to open a store" + "the
// factory that ties the stores together". The day-20a slice is the
// skeleton: doc + OpenAPI interface. Day 20b tried to move the
// per-store openers across but rolled back — the kernel's
// store-opening pattern threads the bus (and several other
// post-open wirings) through Open() itself, so per-store helpers
// in the sub-package gain very little. The 332-line Open() and
// the 52-line Close() stay in kernel/runtime/compose.go for the
// rest of the sprint; the sub-package ships the OpenAPI surface
// so future slices can pick the work back up without re-deriving
// the boundary.
//
// Circular-import guard: the package MUST NOT import
// kernel/runtime — it talks to the host through the OpenAPI
// interface only. The factory returns a *Kernel, which means
// OpenAPI must satisfy *Kernel on the runtime side. We bridge
// the two with a *Kernel-shaped host that the runtime package
// implements (similar to the lifecycle.KernelAPI pattern).
package compose
