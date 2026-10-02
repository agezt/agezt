// SPDX-License-Identifier: MIT

package channel

import (
	"sort"
	"strings"
	"sync"

	"github.com/agezt/agezt/kernel/contract/channelapi"
)

// mu guards the three process-global maps below (registry/live/liveInstances).
// Writers run during daemon boot (RegisterAll, SetLive, SetLiveInstances) AFTER
// the control-plane listener has already started serving, so a channel-list
// request can race a boot-time write. Unsynchronised map read-vs-write is a
// fatal, unrecoverable Go runtime abort (it bypasses recover()), i.e. a
// remote-triggerable crash DoS — hence the lock (CWE-362, finding VULN-002).
var mu sync.RWMutex

// Manifest and MediaCaps are contract types (kernel/contract/channelapi); the
// registry below is process state and stays here.
type (
	Manifest  = channelapi.Manifest
	MediaCaps = channelapi.MediaCaps
)

// registry is the process-wide set of registered channel manifests. The daemon
// seeds it (plugins/builtinchannels.RegisterAll); adding a channel = one more
// RegisterManifest call, no central edit.
var registry = map[string]Manifest{}

// RegisterManifest adds (or replaces) a channel manifest by kind. Idempotent.
func RegisterManifest(m Manifest) {
	mu.Lock()
	defer mu.Unlock()
	registry[m.Kind] = m
}

// Manifests returns all registered channel manifests, ordered by display name.
func Manifests() []Manifest {
	mu.RLock()
	defer mu.RUnlock()
	out := make([]Manifest, 0, len(registry))
	for _, m := range registry {
		out = append(out, m)
	}
	sort.Slice(out, func(i, j int) bool { return out[i].Display < out[j].Display })
	return out
}

// LookupManifest returns the manifest for a kind, if registered.
func LookupManifest(kind string) (Manifest, bool) {
	mu.RLock()
	defer mu.RUnlock()
	m, ok := registry[kind]
	return m, ok
}

// live is the set of channel kinds the daemon actually started this run. The
// daemon sets it after wiring its live channels; the Channels view reads it to
// show "live" vs merely "configured (restart to start)".
var live = map[string]bool{}

// SetLive records the channel kinds that are running this process. Replaces the
// prior set. Called once by the daemon after it builds its live channels.
func SetLive(kinds []string) {
	next := make(map[string]bool, len(kinds))
	for _, k := range kinds {
		next[k] = true
	}
	mu.Lock()
	defer mu.Unlock()
	live = next
}

// IsLive reports whether a channel kind is currently running: it was started
// AND at least one of its instances has not died since.
func IsLive(kind string) bool {
	mu.RLock()
	defer mu.RUnlock()
	if !live[kind] {
		return false
	}
	known := false
	for key := range liveInstances {
		if base, _, _ := strings.Cut(key, "#"); base != kind {
			continue
		}
		known = true
		if !deadInstances[key] {
			return true
		}
	}
	return !known // no instance-level record: trust the kind-level flag
}

// deadInstances holds instances whose Start returned an error or panicked
// while the daemon was still running. Kept apart from liveInstances so a
// channel that dies BEFORE the daemon records the live set (a port already in
// use fails within milliseconds) is not resurrected by SetLiveInstances.
var deadInstances = map[string]bool{}

// MarkInstanceDead records that a channel instance stopped serving. The
// Channels view, `agt status` and the notify tool's targets then stop
// reporting it as live — they used to keep claiming a channel whose listener
// never bound was running.
func MarkInstanceDead(key string) {
	mu.Lock()
	defer mu.Unlock()
	deadInstances[key] = true
}

// InstanceKey addresses a channel account-instance: the bare kind for the
// default account, "kind#label" for a labelled one.
func InstanceKey(kind, label string) string {
	if label == "" {
		return kind
	}
	return kind + "#" + label
}

// liveInstances is the set of running instance keys ("kind" for the default
// account, "kind#label" for a labelled one). The Channels view reads it to show
// per-account live state in the multi-account UI.
var liveInstances = map[string]bool{}

// SetLiveInstances records the running instance keys. Replaces the prior set.
func SetLiveInstances(keys []string) {
	next := make(map[string]bool, len(keys))
	for _, k := range keys {
		next[k] = true
	}
	mu.Lock()
	defer mu.Unlock()
	liveInstances = next
}

// IsLiveInstance reports whether a specific instance key is running. The default
// account's key is the bare kind; a labelled account's is "kind#label".
func IsLiveInstance(key string) bool {
	mu.RLock()
	defer mu.RUnlock()
	return liveInstances[key] && !deadInstances[key]
}
