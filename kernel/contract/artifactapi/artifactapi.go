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

// Entry is one stored artifact's metadata — a single ARRIVAL. The bytes live
// in the content-addressed blob store under Ref; an Entry is per-arrival, so
// the same image sent twice is two Entries pointing at one blob.
//
// It is the contract row (`remoteapi`): the stored row and the served row
// cannot drift. The name stays in the artifacts module as an alias.
type Entry = remoteapi.Artifact

// Filter narrows List; empty fields impose no constraint (exact match
// otherwise).
type Filter = remoteapi.Filter

// Blob is the content-addressed store the artifact index is a sidecar over.
//
// The index and the blobs are two things: the index is browsable metadata,
// the blobs are the bytes. The host holds both as ports — the blob store is
// what a tool writes a synthesized image into, and the index is what a tool
// lists and deletes.
//
// The four verbs are the ones the host uses: write a blob and get its ref
// back, read one, ask whether one is there, and ask how big it is.
type Blob interface {
	// Put stores data and returns its content-addressed ref.
	Put(data []byte) (string, error)
	// Get returns a blob's bytes.
	Get(ref string) ([]byte, error)
	// Has reports whether the store holds that ref.
	Has(ref string) (bool, error)
	// Size returns a blob's size in bytes.
	Size(ref string) (int64, error)
}

// RefGuard reports which blobs are still referenced from OUTSIDE the index.
//
// The index garbage-collects a blob when no entry names it any more — but a
// blob the JOURNAL still names is not garbage. The guard is how the host (the
// daemon, which owns the journal) says so. It is a port because the host
// installs it on an index it does not own.
type RefGuard interface {
	// ReferencedRefs returns the subset of candidates that are still
	// referenced from outside the index.
	ReferencedRefs(candidates []string) map[string]bool
}

// Index is the artifact-index port a TOOL drives: the surface a plugin uses
// to store and read back the artifacts it produces or receives.
//
// It is deliberately FOUR verbs and not more: a plugin may reach the contract
// and the platform layers and nothing else, so every method on this port is a
// reach a plugin holds. A fifth verb would have to be earned here.
//
// The port is ONE port rather than four: the daemon has a single index, and a
// tool that could name a narrower slice of it would still have to be handed
// the whole thing. A tool with a narrower need keeps its own private
// interface and receives this one (an interface value satisfies a narrower
// interface when its method set covers it), so the narrowness lives in the
// tool and the daemon keeps one wiring site.
//
// The host's own verbs are NOT here — they live in HostIndex, which is wider
// on purpose. SetRefGuard belongs there: a plugin that could clear the
// journal's ref guard would be able to make the collector delete bytes the
// journal still names.
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

// HostIndex is the HOST-facing port: the tool-facing Index plus the verbs the
// daemon and the kernel use to run the index itself.
//
// The split exists because the two audiences are not the same. A tool needs
// to store, list, read and delete an artifact. The host needs to count the
// entries, find one by id, record an entry for bytes that are ALREADY in the
// blob store (a tool that wrote them itself hands the index the ref), count
// the stale ones, reclaim them, and install the ref guard.
//
// Keeping the host verbs off the tool port is what keeps the tool port four
// verbs wide: a plugin that could call Collect would delete entries, and one
// that could call SetRefGuard would switch off the journal's protection of
// the bytes the journal still names.
//
// The daemon holds one index; the kernel holds it as this port; the tool
// registry receives the narrower Index. One index, one wiring site.
type HostIndex interface {
	Index
	// Get returns one entry's metadata by id; the bool is false when there
	// is none.
	Get(id string) (remoteapi.Artifact, bool)
	// Count returns how many entries the index holds.
	Count() int
	// IndexRef records an entry for bytes that are ALREADY in the blob store
	// — a tool that wrote them itself and hands the index the ref.
	IndexRef(ref string, meta remoteapi.Artifact, createdMs int64) (remoteapi.Artifact, error)
	// StaleEntries returns the entries older than the cutoff — what the
	// reaper counts and what the collector reclaims.
	StaleEntries(olderThanMs int64) []remoteapi.Artifact
	// Collect reclaims the stale entries and reports how many were collected
	// and how many bytes they freed.
	Collect(olderThanMs int64) (collected int, bytes int64)
	// SetRefGuard installs the guard that tells the collector which blobs
	// are still referenced from outside the index.
	SetRefGuard(g RefGuard)
}
