// SPDX-License-Identifier: MIT

// Package controlplane is the local control protocol between the agezt
// daemon and the agt CLI.
//
// Transport: TCP localhost, line-delimited JSON. The daemon binds to an
// ephemeral port; the client discovers it via two files under
// <BaseDir>/runtime/:
//
//	control.addr    text: "127.0.0.1:NNNNN\n"
//	control.token   text: hex token; clients must send it on every request
//
// Wire format — every line is one JSON object.
//
//	Request : {"id":"q1","cmd":"<name>","token":"<hex>","args":{...}}
//	Response: {"id":"q1","type":"event",  "event":{...}}        zero or more
//	Response: {"id":"q1","type":"result", "result":{...}}       exactly one
//	Response: {"id":"q1","type":"error",  "error":"reason"}     alternative final
//
// One request per connection: open, send, read until type=result|error,
// close. This keeps the protocol trivial and avoids multiplexing concerns.
//
// (Not JSON-RPC: that's the contract for the kernel↔plugin wire per
// DECISIONS B0. The control plane is process-local and uses a thinner,
// purpose-built shape so the CLI binary stays small.)
package controlplane
