// SPDX-License-Identifier: MIT

// Package remoteapi is the contract between two AGEZT daemons: the shape of
// the rows one daemon serves to a peer and the other reads back.
//
// The peer's events travel as the canonical event JSON (kernel/event) and the
// peer's artifacts travel as the Artifact below. The serving adapter
// (kernel/restapi) writes these rows and the consuming application
// (kernel/app/remote) reads them, so the wire has one declaration instead of
// one on each side of the call.
//
// Rules of the road:
//
//   - Types only, and only the fields that travel. The bytes of an artifact
//     never travel; only its description does.
//   - The json names here are the wire. A peer of another version reads the
//     same keys, so a key is a promise, not a detail.
package remoteapi
