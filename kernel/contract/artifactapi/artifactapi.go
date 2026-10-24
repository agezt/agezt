// SPDX-License-Identifier: MIT

// Package artifactapi is the contract the artifact index and the tools that
// read and write it agree on.
//
// The artifact index is the daemon's metadata index over the blob store
// (M822): the browsable, deletable per-arrival entries — inbound images,
// tool outputs, the files a script exports. Four first-party tools drive it:
// the artifacts tool (list/read/delete), the fetch tool, the browser action
// tool and the code_exec tool (all three write into it).
//
// Every one of those tools is a PLUGIN, and a plugin may reach the contract
// and the platform layers and nothing else — so the index a plugin has to
// name cannot live in a module's implementation package. It lives here, as a
// PORT: the module's *artifact.Index answers it.
//
// The rules of the road:
//
//   - Ports and the rows that belong to the port only.
//   - Pure. It may import the standard library and other contract packages
//     only (archcheck's principle P4).
package artifactapi

import "github.com/agezt/agezt/kernel/contract/remoteapi"

// Index is the artifact-index port: the surface a tool drives to store and
// read back the artifacts it produces or receives.
//
// The port is the union of what the four tools need, and it is deliberately
// ONE port rather than four: the daemon has a single index, and a tool that
// could name a narrower slice of it would still have to be handed the whole
// thing. A tool with a narrower need keeps its own private interface and
// receives this one (an interface value satisfies a narrower interface when
// its method set covers it), so the narrowness lives in the tool and the
// daemon keeps one wiring site.
//
// The rows are the contract's own (`remoteapi`): the stored row and the
// served row cannot drift.
type Index interface {
	// PutEntry stores data and records its metadata. The caller supplies
	// createdMs (the daemon stamps wall-clock; tests pass a fixed value) so
	// the entry's sort key is explicit. The entry's ID, ref and size are
	// filled in by the index.
	PutEntry(meta remoteapi.Artifact, data []byte, createdMs int64) (remoteapi.Artifact, error)
	// List returns the entries matching f, newest first.
	List(f remoteapi.Filter) []remoteapi.Artifact
	// Bytes returns an entry's bytes together with its row.
	Bytes(id string) ([]byte, remoteapi.Artifact, error)
	// Delete removes an entry. A deletion that would orphan bytes another
	// owner still names is refused by the index itself.
	Delete(id string) error
}
