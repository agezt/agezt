// SPDX-License-Identifier: MIT

package research

import (
	"context"
	"strings"
	"testing"

	"github.com/agezt/agezt/kernel/modules/reasoning/api"
)

// The research engines REFUSE when there is no daemon.
//
// The engines are filled from the daemon's own kernel, by type: there is no
// foreign handle to fill them with any more. A host with no daemon holds no
// research capability, and the engines say so rather than answering as if a
// model had answered.
func TestTheResearchEnginesRefuseWithoutADaemon(t *testing.T) {
	engines := EnginesFromKernel(nil, func() string { return "corr" })

	_, err := engines.Research(context.Background(), "corr", "what is the weather", api.ResearchOptions{})
	if err == nil {
		t.Fatal("a host with no daemon researched a question")
	}
	if !strings.Contains(err.Error(), "not available") {
		t.Errorf("error = %v, want the research capability's refusal", err)
	}
}
