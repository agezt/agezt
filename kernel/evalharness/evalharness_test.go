// SPDX-License-Identifier: MIT

package evalharness_test

import (
	"encoding/json"
	"testing"
	"time"

	"github.com/agezt/agezt/kernel/evalharness"
)

func TestEvaluate_StochasticScenarioRejectsExactOutputOracle(t *testing.T) {
	seed := int64(42)
	report := evalharness.Evaluate(evalharness.Scenario{
		Name:            "creative-summary",
		Provider:        "mock",
		Model:           "mock-stochastic",
		Temperature:     0.7,
		Seed:            &seed,
		ContextSnapshot: "task plus relevant memory",
		ToolMocks: []evalharness.ToolMock{{
			Name:   "search",
			Input:  json.RawMessage(`{"q":"agezt"}`),
			Output: "Agezt is an agentic OS.",
		}},
		Expectations: []evalharness.Expectation{{
			Type:  evalharness.ExpectExactOutput,
			Value: "Agezt is an agentic OS.",
		}},
	}, evalharness.RunResult{Output: "Agezt is an auditable agentic OS."})

	if report.Pass {
		t.Fatal("stochastic scenario with exact-output oracle passed; want rejection")
	}
	if report.Provider != "mock" || report.Model != "mock-stochastic" || report.Seed == nil || *report.Seed != seed {
		t.Fatalf("report lost replay metadata: %+v", report)
	}
	if len(report.Assertions) != 1 || report.Assertions[0].Type != evalharness.ExpectExactOutput {
		t.Fatalf("assertions=%+v", report.Assertions)
	}
}

func TestEvaluate_ExpectationBandsPass(t *testing.T) {
	report := evalharness.Evaluate(evalharness.Scenario{
		Name:        "bounded-answer",
		Provider:    "mock",
		Model:       "mock",
		Temperature: 0.8,
		Expectations: []evalharness.Expectation{
			{Type: evalharness.ExpectOutputContains, Value: "Paris"},
			{Type: evalharness.ExpectScoreBand, ScoreName: "semantic_similarity", Min: 0.75, Max: 1},
			{Type: evalharness.ExpectMaxCost, MaxCostMicrocents: 500},
			{Type: evalharness.ExpectMaxLatency, MaxLatency: 150 * time.Millisecond},
			{Type: evalharness.ExpectNoSafetyViolation},
		},
	}, evalharness.RunResult{
		Output:         "The answer is Paris.",
		Scores:         map[string]float64{"semantic_similarity": 0.91},
		CostMicrocents: 120,
		Latency:        25 * time.Millisecond,
	})

	if !report.Pass {
		t.Fatalf("report failed: %+v", report.Assertions)
	}
}

func TestEvaluate_JSONSchemaExpectation(t *testing.T) {
	report := evalharness.Evaluate(evalharness.Scenario{
		Name:        "structured-output",
		Temperature: 0.2,
		Expectations: []evalharness.Expectation{{
			Type: evalharness.ExpectJSONSchema,
			Schema: json.RawMessage(`{
				"type":"object",
				"required":["answer"],
				"properties":{"answer":{"type":"string"}},
				"additionalProperties":false
			}`),
		}},
	}, evalharness.RunResult{JSON: json.RawMessage(`{"answer":"ok"}`)})

	if !report.Pass {
		t.Fatalf("json schema expectation failed: %+v", report.Assertions)
	}
}

func TestEvaluate_BandFailureReportsResidualRisk(t *testing.T) {
	report := evalharness.Evaluate(evalharness.Scenario{
		Name: "unsafe-answer",
		Expectations: []evalharness.Expectation{
			{Type: evalharness.ExpectScoreBand, ScoreName: "faithfulness", Min: 0.8, Max: 1},
			{Type: evalharness.ExpectNoSafetyViolation},
		},
	}, evalharness.RunResult{
		Scores:           map[string]float64{"faithfulness": 0.4},
		SafetyViolations: []string{"attempted secret disclosure"},
	})

	if report.Pass {
		t.Fatal("report passed despite out-of-band score and safety violation")
	}
	if len(report.Assertions) != 2 || report.Assertions[0].Pass || report.Assertions[1].Pass {
		t.Fatalf("assertions=%+v", report.Assertions)
	}
}
