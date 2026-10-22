// SPDX-License-Identifier: MIT

package workboard_test

import (
	"context"
	"strings"
	"testing"

	"github.com/agezt/agezt/kernel/modules/work/seat"
)

// TestWorkboardExecutionMapsTheSeatContainerSurfaceToTheDockerProfile: a seat
// names the isolation *surface* it wants ("container" — the seat store accepts
// exactly "", "local", "warden" and "container"), while a run profile names
// the implementation that provides it ("docker"). The translation was missing,
// so a seat pinned to "container" reached the resolver as "container" — which
// is not a run profile — and the dispatch degraded to the agent's own
// isolation with the comment "isolation "container" unavailable".
func TestWorkboardExecutionMapsTheSeatContainerSurfaceToTheDockerProfile(t *testing.T) {
	f := runningExecution()
	f.selected = seat.Seat{ID: "selected", ExecutionProfile: "container"}
	service := f.service(func(ctx context.Context, corr, intent string) (string, error) {
		return " answer ", nil
	})
	if _, err := service.Run(context.Background(), executionInput()); err != nil {
		t.Fatalf("run: %v", err)
	}
	joined := strings.Join(f.trace, " ")
	if !strings.Contains(joined, "isolation:docker") {
		t.Errorf("the seat's container surface was not mapped to the docker profile: %v", f.trace)
	}
	if strings.Contains(joined, "isolation:container") {
		t.Errorf("the seat's surface reached the resolver unrouted: %v", f.trace)
	}
}
