// SPDX-License-Identifier: MIT

// Package datalakeapi is the contract layer's home for the Personal Data
// Lake's rows: one collection's definition (its schema and the columns it
// carries), one record (a row with a stable id and a free-form field map),
// the query that narrows a listing, and the summary a listing shows.
//
// The rows live here because the tool that drives the lake is a PLUGIN
// (`plugins/tools/db`), and a plugin may reach the contract and the platform
// layers and nothing else. The implementation that keeps them — the on-disk
// lake — stays in the module (kernel/modules/artifacts/datalake); the module
// keeps the names its own callers use as aliases, so there is one row and one
// spelling of it.
//
// The rules of the road:
//
//   - Rows, constants, errors and pure functions only. No store, no file, no
//     clock.
//   - Pure. It may import the standard library and other contract packages
//     only (archcheck's principle P4).
package datalakeapi
