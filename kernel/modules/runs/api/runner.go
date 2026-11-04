// SPDX-License-Identifier: MIT

package api

import (
	"context"

	"github.com/agezt/agezt/kernel/contract/agentapi"
	"github.com/agezt/agezt/kernel/contract/assureapi"
)

// RunnerService is the runs module's RUN ENGINE: the operations that start a
// run, run it under a retry policy, run it under the assurance loop and
// close its agent lifecycle.
//
// The verbs are the MODULE's own names (W3.76ac). The daemon used to answer
// them under names of its own — Run, RunWith, RunWithRetry, RunAssured and
// CompleteAgentLifecycle — which was a second spelling of one family: the
// run engine already answered in exactly these names, and every caller above
// had to learn the daemon's.
//
// The port carries the engine's operations, not the host's identity: the
// correlation id a run carries is minted by the host (the daemon's own
// identity door, `NewCorrelation`), and the engine is asked to run under an
// id the caller already holds.
type RunnerService interface {
	// RunWith runs one run under the correlation and returns its answer.
	RunWith(ctx context.Context, corr, intent string) (string, error)
	// RunWithRetry runs one run under the correlation, retrying under the
	// given policy.
	RunWithRetry(ctx context.Context, corr, intent string, pol agentapi.RetryPolicy) (string, error)
	// RunAssured runs one run under the assurance loop: the run is verified
	// and retried until the verdict is complete or the attempts run out. It
	// returns the answer and the verification's result.
	RunAssured(ctx context.Context, corr, intent string, maxAttempts int) (string, assureapi.Result, error)
	// CompleteAgentLifecycle closes the agent's lifecycle for the
	// correlation: the cycle is advanced and a retired agent is retired. It
	// is idempotent per correlation.
	CompleteAgentLifecycle(ctx context.Context, corr string)
}
