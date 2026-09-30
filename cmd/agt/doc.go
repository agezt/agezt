// SPDX-License-Identifier: MIT

// Package main provides the command registration system for the Agezt CLI.
// The dispatcher delegates to cmd/agt/router (Day 31 wiring: the inline
// Command + CommandRegistry + ExecuteCommand that lived here since the
// pre-sprint days now thin-wrap the router.Registry type so the package is
// actually exercised by the binary; the previous Day 1-6 extraction left
// it allowlisted-as-dead in tools/deadcodecheck until this slice).
//
// The parallel `CommandRegistry` map exists for test helpers and
// coverage probes that iterate the command list (`commands_test_helpers_test.go`,
// `help_test.go:31` sync-check). Tests keep their existing shape;
// the registry is the actual dispatcher.
package main
