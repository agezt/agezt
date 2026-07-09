// SPDX-License-Identifier: MIT

package journal

// RunEntry is a single run's folded state, built from journal events.
// It is the shared representation used by the control plane's runs listing
// and stats handlers.
type RunEntry struct {
	CorrelationID     string
	Intent            string
	StartedUnixMS     int64
	StartedSeq        int64
	CompletedUnixMS   int64
	Iters             int
	Completed         bool
	Failed            bool
	FailedUnixMS      int64
	FailReason        string
	Abandoned         bool
	ParentCorrelation string
	SpentMicrocents   int64
	AnswerPreview     string
	Model             string
	Agent             string
	Phase             string
	Tool              string
}

// RunEntryStatus reports a run's terminal status, the single source of
// truth shared across all handlers. Precedence: completed > failed > abandoned > running.
func RunEntryStatus(r *RunEntry) string {
	switch {
	case r.Completed:
		return "completed"
	case r.Failed:
		return "failed"
	case r.Abandoned:
		return "abandoned"
	default:
		return "running"
	}
}
