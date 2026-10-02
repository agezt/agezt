// SPDX-License-Identifier: MIT

// Package file is the in-process file tool. It reads, writes, lists, and
// searches files inside a configured workspace root and refuses to operate
// outside it (no `..` escape, no absolute paths outside root, no symlink
// escape).
//
// Ops: read, write, append, list, search, stat, delete, replace, glob.
// `replace` does a surgical find/replace edit so the model need not rewrite a
// whole file (M114). A unified-diff `patch` op is still deferred — `replace`
// covers small edits. The advertised op enum and the dispatch switch are kept in
// lockstep by TestFile_EveryAdvertisedOpIsDispatched.
//
// Containment policy: the root directory is resolved with filepath.Abs +
// EvalSymlinks at New(); every requested path is resolved the same way
// and rejected if its absolute, symlink-resolved form does not have the
// root as a prefix. This is the M1 minimum; Warden namespace isolation
// (TASKS P1-WARD-01) provides deeper containment when it lands.
package file
