// SPDX-License-Identifier: MIT

package workflowexec_test

import (
	"testing"

	"github.com/agezt/agezt/kernel/workflowexec"
)

func TestSnippet_Nil(t *testing.T) {
	s, truncated := workflowexec.Snippet(nil)
	if s != "" || truncated {
		t.Fatalf("Snippet(nil) = %q, %v, want empty false", s, truncated)
	}
}

func TestSnippet_String(t *testing.T) {
	s, truncated := workflowexec.Snippet("hello")
	if s != "hello" || truncated {
		t.Fatalf("Snippet('hello') = %q, %v", s, truncated)
	}
}

func TestSnippet_JSON(t *testing.T) {
	s, truncated := workflowexec.Snippet(map[string]int{"a": 1})
	if s != `{"a":1}` || truncated {
		t.Fatalf("Snippet = %q, %v", s, truncated)
	}
}

func TestSnippet_Truncation(t *testing.T) {
	long := string(make([]byte, 5000))
	s, truncated := workflowexec.Snippet(long)
	if !truncated {
		t.Fatal("expected truncated for long input")
	}
	if len([]rune(s)) > workflowexec.SnippetMax+1 { // +1 for ellipsis
		t.Fatalf("snippet length %d exceeds max %d", len([]rune(s)), workflowexec.SnippetMax)
	}
}

func TestStepCap_IsPositive(t *testing.T) {
	if workflowexec.StepCap <= 0 {
		t.Fatal("StepCap must be positive")
	}
}

func TestMaxSubflowDepth_IsPositive(t *testing.T) {
	if workflowexec.MaxSubflowDepth <= 0 {
		t.Fatal("MaxSubflowDepth must be positive")
	}
}
