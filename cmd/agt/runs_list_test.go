// SPDX-License-Identifier: MIT

package main

import (
	"bytes"
	"strings"
	"testing"
)

// TestRenderRunRow_ShowsAnswerPreview — a run row renders its one-line answer
// preview (M59) when present, and omits the answer line when absent.
func TestRenderRunRow_ShowsAnswerPreview(t *testing.T) {
	withAnswer := map[string]any{
		"correlation_id": "run-A", "intent": "do thing", "status": "completed",
		"started_unix_ms": float64(0), "duration_ms": float64(5), "iters": float64(1),
		"answer_preview": "the module is github.com/agezt/agezt",
	}
	var buf bytes.Buffer
	renderRunRow(&buf, withAnswer, "", false)
	if !strings.Contains(buf.String(), `answer  : "the module is github.com/agezt/agezt"`) {
		t.Errorf("answer preview missing; got:\n%s", buf.String())
	}

	noAnswer := map[string]any{
		"correlation_id": "run-B", "intent": "do thing", "status": "running",
	}
	var buf2 bytes.Buffer
	renderRunRow(&buf2, noAnswer, "", false)
	if strings.Contains(buf2.String(), "answer  :") {
		t.Errorf("no answer line expected when absent; got:\n%s", buf2.String())
	}
}
