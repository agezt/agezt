// SPDX-License-Identifier: MIT

package runtime_test

import (
	"context"
	"errors"
	"strings"
	"testing"

	"github.com/agezt/agezt/kernel/agent"
	"github.com/agezt/agezt/kernel/event"
	"github.com/agezt/agezt/kernel/runtime"
)

type failIfCalledProvider struct{}

func (failIfCalledProvider) Name() string { return "fail-if-called" }
func (failIfCalledProvider) Complete(context.Context, agent.CompletionRequest) (*agent.CompletionResponse, error) {
	return nil, errors.New("provider should not be called")
}

func TestRunWith_HeuristicBypassAnswersTimeWithoutProvider(t *testing.T) {
	k, err := runtime.Open(runtime.Config{
		BaseDir:  t.TempDir(),
		Provider: failIfCalledProvider{},
	})
	if err != nil {
		t.Fatalf("Open: %v", err)
	}
	t.Cleanup(func() { k.Close() })

	ans, corr, err := k.Run(context.Background(), "saat kaç?")
	if err != nil {
		t.Fatalf("Run: %v", err)
	}
	if !strings.HasPrefix(ans, "Current time: ") {
		t.Fatalf("answer=%q, want current time fast-path", ans)
	}

	var received, completed, info, llm int
	_ = k.Journal().Range(func(e *event.Event) error {
		if e.CorrelationID != corr {
			return nil
		}
		switch e.Kind {
		case event.KindTaskReceived:
			received++
		case event.KindTaskCompleted:
			completed++
		case event.KindInfo:
			info++
		case event.KindLLMRequest:
			llm++
		}
		return nil
	})
	if received != 1 || completed != 1 || info != 1 || llm != 0 {
		t.Fatalf("events received=%d completed=%d info=%d llm=%d, want 1/1/1/0", received, completed, info, llm)
	}
}
