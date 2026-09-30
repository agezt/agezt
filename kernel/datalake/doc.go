// SPDX-License-Identifier: MIT

// Package datalake is AGEZT's file-based structured store — the "Personal Data
// Lake" (M834). It gives agents (and the operator, via the Web UI) real
// databases without a database: a Lake holds named COLLECTIONS (tables), each a
// set of JSON RECORDS with an optional SCHEMA describing its fields and how the
// UI should render it (a generic table, or a bespoke app view like an expense
// tracker or calendar).
//
// It is deliberately dependency-free and on-disk, matching AGEZT's single-static-
// binary / no-DB architecture: every collection is a directory, every record a
// JSON file, with an in-memory index loaded at Open and guarded by one mutex.
// Collections are shared across all agents on the daemon, so one agent can file
// data another (or the human in chat) later reads.
//
// Layout:
//
//	<base>/datalake/<collection>/_schema.json   — the collection's schema
//	<base>/datalake/<collection>/rec/<id>.json  — one record per file
package datalake
