// SPDX-License-Identifier: MIT

package delegation

// Config is the delegation policy the module reads on every spawn. The daemon
// hands a LIVE provider (Deps.Config), not a snapshot: the caps are operator
// settings, and an operator raising SubAgentMaxDepth at runtime must see the
// change on the next delegation — the daemon read them live before the move,
// and a snapshot would silently freeze them.
//
// Zero means "unbounded" for every cap, exactly as before the move: the
// daemon's default configuration leaves them unbounded and the module keeps
// that meaning rather than inventing a default of its own.
type Config struct {
	// MaxDepth caps how DEEP delegation nests (the daemon's
	// SubAgentMaxDepth). A value <= 0 is read as 1.
	MaxDepth int
	// MaxFanout caps how WIDE one run fans out (SubAgentMaxFanout).
	MaxFanout int
	// MaxTotal caps the sub-agent count of a whole delegation TREE
	// (SubAgentMaxTotal).
	MaxTotal int
	// MaxSpendMicrocents caps the collective spend of a run's sub-agents
	// (SubAgentMaxSpendMicrocents).
	MaxSpendMicrocents int64
	// ModelAvailable reports whether a model id can be routed right now
	// (the daemon's ModelAvailable). nil means "everything is available".
	ModelAvailable func(modelID string) bool
}

// maxDepth returns the effective depth cap: an unset cap reads as 1, so a
// delegation never recurses without an explicit budget.
func (c Config) maxDepth() int {
	if c.MaxDepth <= 0 {
		return 1
	}
	return c.MaxDepth
}
