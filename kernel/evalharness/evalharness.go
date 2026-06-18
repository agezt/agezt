// SPDX-License-Identifier: MIT

// Package evalharness contains the behavioral-eval primitives used for
// stochasticity-aware replay. Recorded journal replay can still compare exact
// recorded outputs; behavioral re-runs must assert expectation bands instead of
// byte equality when temperature is non-zero.
package evalharness

import (
	"encoding/json"
	"fmt"
	"regexp"
	"strings"
	"time"

	"github.com/agezt/agezt/kernel/agent"
)

// Scenario captures the replay context for a behavioral re-run.
type Scenario struct {
	Name            string
	Provider        string
	Model           string
	Temperature     float64
	Seed            *int64
	ContextSnapshot string
	ToolMocks       []ToolMock
	Expectations    []Expectation
}

// Stochastic reports whether exact output equality is an invalid oracle for the
// scenario.
func (s Scenario) Stochastic() bool {
	return s.Temperature > 0
}

// ToolMock records a deterministic tool fixture available to the scenario.
type ToolMock struct {
	Name    string
	Input   json.RawMessage
	Output  string
	IsError bool
}

// RunResult is the observed behavioral re-run output evaluated against a
// Scenario's expectations.
type RunResult struct {
	Output           string
	JSON             json.RawMessage
	Scores           map[string]float64
	CostMicrocents   int64
	Latency          time.Duration
	SafetyViolations []string
}

// ExpectationType names one assertion in an expectation-band oracle.
type ExpectationType string

const (
	ExpectExactOutput       ExpectationType = "exact_output"
	ExpectOutputContains    ExpectationType = "output_contains"
	ExpectOutputRegex       ExpectationType = "output_regex"
	ExpectScoreBand         ExpectationType = "score_band"
	ExpectMaxCost           ExpectationType = "max_cost"
	ExpectMaxLatency        ExpectationType = "max_latency"
	ExpectNoSafetyViolation ExpectationType = "no_safety_violations"
	ExpectJSONSchema        ExpectationType = "json_schema"
)

// Expectation is intentionally broad enough for first-pass eval suites while
// staying simple to serialize from YAML/JSON later.
type Expectation struct {
	Name string
	Type ExpectationType

	Value string

	ScoreName string
	Min       float64
	Max       float64

	MaxCostMicrocents int64
	MaxLatency        time.Duration

	Schema json.RawMessage
}

// AssertionResult is the outcome of one Expectation.
type AssertionResult struct {
	Name    string
	Type    ExpectationType
	Pass    bool
	Message string
}

// Report summarizes a Scenario evaluation.
type Report struct {
	Scenario    string
	Provider    string
	Model       string
	Temperature float64
	Seed        *int64
	Pass        bool
	Assertions  []AssertionResult
}

// Evaluate checks result against scenario expectations. It rejects exact-output
// oracle use for stochastic scenarios so eval authors are forced to express
// acceptable behavior bands.
func Evaluate(s Scenario, result RunResult) Report {
	report := Report{
		Scenario:    s.Name,
		Provider:    s.Provider,
		Model:       s.Model,
		Temperature: s.Temperature,
		Seed:        s.Seed,
		Pass:        true,
	}
	if s.Stochastic() && len(s.Expectations) == 0 {
		report.add(AssertionResult{
			Name:    "stochastic_oracle",
			Type:    ExpectExactOutput,
			Pass:    false,
			Message: "stochastic behavioral re-run requires expectation-band assertions",
		})
		return report
	}
	for _, exp := range s.Expectations {
		report.add(evaluateOne(s, exp, result))
	}
	return report
}

func (r *Report) add(assertion AssertionResult) {
	r.Assertions = append(r.Assertions, assertion)
	if !assertion.Pass {
		r.Pass = false
	}
}

func evaluateOne(s Scenario, exp Expectation, result RunResult) AssertionResult {
	name := exp.Name
	if name == "" {
		name = string(exp.Type)
	}
	fail := func(format string, args ...any) AssertionResult {
		return AssertionResult{Name: name, Type: exp.Type, Pass: false, Message: fmt.Sprintf(format, args...)}
	}
	pass := func(msg string) AssertionResult {
		return AssertionResult{Name: name, Type: exp.Type, Pass: true, Message: msg}
	}

	switch exp.Type {
	case ExpectExactOutput:
		if s.Stochastic() {
			return fail("exact output equality is invalid for temperature %.3g", s.Temperature)
		}
		if result.Output != exp.Value {
			return fail("output %q != expected %q", result.Output, exp.Value)
		}
		return pass("exact output matched")
	case ExpectOutputContains:
		if !strings.Contains(result.Output, exp.Value) {
			return fail("output did not contain %q", exp.Value)
		}
		return pass("output contained expected text")
	case ExpectOutputRegex:
		re, err := regexp.Compile(exp.Value)
		if err != nil {
			return fail("invalid regex: %v", err)
		}
		if !re.MatchString(result.Output) {
			return fail("output did not match %q", exp.Value)
		}
		return pass("output matched regex")
	case ExpectScoreBand:
		score, ok := result.Scores[exp.ScoreName]
		if !ok {
			return fail("missing score %q", exp.ScoreName)
		}
		if score < exp.Min || score > exp.Max {
			return fail("score %q=%g outside [%g,%g]", exp.ScoreName, score, exp.Min, exp.Max)
		}
		return pass("score inside expectation band")
	case ExpectMaxCost:
		if result.CostMicrocents > exp.MaxCostMicrocents {
			return fail("cost %d > max %d microcents", result.CostMicrocents, exp.MaxCostMicrocents)
		}
		return pass("cost within bound")
	case ExpectMaxLatency:
		if result.Latency > exp.MaxLatency {
			return fail("latency %s > max %s", result.Latency, exp.MaxLatency)
		}
		return pass("latency within bound")
	case ExpectNoSafetyViolation:
		if len(result.SafetyViolations) > 0 {
			return fail("safety violations: %s", strings.Join(result.SafetyViolations, "; "))
		}
		return pass("no safety violations")
	case ExpectJSONSchema:
		raw := result.JSON
		if len(raw) == 0 {
			raw = json.RawMessage(result.Output)
		}
		if err := agent.ValidateToolInput(agent.ToolDef{Name: "eval.output", InputSchema: exp.Schema}, raw); err != nil {
			return fail("json schema mismatch: %v", err)
		}
		return pass("json output matched schema")
	default:
		return fail("unknown expectation type %q", exp.Type)
	}
}
