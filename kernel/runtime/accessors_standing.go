// SPDX-License-Identifier: MIT

package runtime

// Provenance: Kernel accessors: data (DataLake, ConfigCenter) + the standing
//             library's FACE. Code extracted from accessors.go during the
//             Day-56 god-file split. Public API unchanged.
//
// The standing library's lifecycle left the daemon with W3.76x: the standing
// package owns the library's operations, and the daemon holds the file port
// (Standing) and the library's face (StandingService).

import (
	"github.com/agezt/agezt/kernel/contract/artifactapi"
	"github.com/agezt/agezt/kernel/contract/datalakeapi"
	"github.com/agezt/agezt/kernel/contract/standingapi"
)

// Artifacts returns the content-addressed artifact store (SPEC-04 §3.6), where
// the loop offloads oversized tool outputs. Used by retrieval surfaces.
func (k *Kernel) Artifacts() artifactapi.Blob { return k.artifacts }

// Voice returns the configured voice adapter (STT/TTS), or nil when unset. The
// channel inbound path uses it to auto-transcribe inbound voice notes.
func (k *Kernel) Voice() Voice { return k.cfg.Voice }

// ArtifactIndex returns the metadata index over the blob store (M822) — the
// browsable/deletable per-arrival entries (inbound images, tool outputs) the
// file manager and inbound-image persistence use.
//
// It is the HOST-facing port: the kernel counts, reclaims and guards the
// index, so it needs more than the four verbs a tool gets.
func (k *Kernel) ArtifactIndex() artifactapi.HostIndex { return k.artIndex }

// DataLake returns the Personal Data Lake (M834) — the file-based structured
// collections agents build and share, surfaced by the `db` tool and the Web UI.
func (k *Kernel) DataLake() datalakeapi.Lake { return k.lake }

// The standing library's LIFECYCLE is the standing package's (W3.76x): the
// daemon used to hold the four writes over the store it kept the file port of
// — the library's rules in one place, its lifecycle in another — and it holds
// the library's FACE now.
//
// The refusal for a daemon with no library is the CONTRACT's row
// (standingapi.ErrNoLibrary): the library raises it (it is the library's
// absence), the applications raise it (they are the ones that notice), and a
// caller compares with errors.Is — so every side says the same thing.

// Standing returns the standing-order library (SPEC-16 §4), backing `agt
// standing`.
//
// It is the contract's STORE port (W3.50): the daemon opens the file and
// hands the port over, so the kernel names the contract and not the
// module's implementation. Nil when the host gave no library — callers must
// check, and the surfaces that do answer "not available" instead of calling
// into nothing.
func (k *Kernel) Standing() standingapi.Store { return k.standing }

// StandingService returns the standing library's SERVICE — the face of the
// standing package, which owns the library's lifecycle (W3.76x).
//
// It is the port the standing application, the guardians' orders and the
// agent-facing standing-orders tool bind, and it is nil when the daemon
// holds no library: the callers refuse then, rather than calling into
// nothing.
//
// The daemon does not speak FOR the library: it used to hold four writes
// over the library's own operations — a second spelling of every verb the
// contract's lifecycle port already names — and it holds none now.
func (k *Kernel) StandingService() standingapi.Lifecycle { return k.standingSvc }

// Roster returns the durable agent-profile store (M783). Always non-nil after Open.
