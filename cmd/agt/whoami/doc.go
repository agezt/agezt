// SPDX-License-Identifier: MIT

// Package whoami is the `agt whoami` command's home. It is the
// pilot of the "command as its own Go package" refactor: this
// file used to be cmd/agt/whoami.go in package main, alongside
// 200+ other command files, and could not move out because it
// depended on the package-private `dial` and `encodeJSON`
// helpers. After Day 5 extracted `dial` to cmd/agt/dial and
// `encodeJSON` to cmd/agt/jsonout, the only thing keeping
// whoami in package main was the registration line in
// cmd_register.go — and that line is now an import +
// `whoami.Run` reference.
//
// Why bother. The benefit is not visible for a single 70-line
// file. It becomes visible when the same template is applied
// to the 200+ other commands: each command's tests live in
// the same package, each command's imports are local (no need
// to import everything that package main pulled in), and a
// test failure in one command no longer triggers a build of
// every other command. The first ~5 commands moved under this
// pattern will take a similar amount of work; the 50th will
// be mechanical.
package whoami
