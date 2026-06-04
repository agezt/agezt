// SPDX-License-Identifier: MIT

package restapi

import (
	"regexp"
	"strings"
	"testing"
)

// TestMetrics_AuthedPrometheusFormat — /metrics requires a token (it exposes
// spend/activity) and renders the injected gauges in Prometheus text format with
// the agezt_ prefix and HELP/TYPE lines (M135).
func TestMetrics_AuthedPrometheusFormat(t *testing.T) {
	s := newServer(t, &fakeEngine{model: "m"}, "secret")
	s.SetMetrics(func() []Metric {
		return []Metric{
			{Name: "active_runs", Help: "runs in flight", Type: "gauge", Value: 2},
			{Name: "halted", Help: "1 if halted", Value: 0},
			{Name: "spend_today_microcents", Value: 1500000000},
		}
	})

	// No token → 401 (sensitive, unlike /healthz).
	if rec := do(t, s, "GET", "/metrics", "", ""); rec.Code != 401 {
		t.Fatalf("/metrics no-token = %d want 401", rec.Code)
	}

	rec := do(t, s, "GET", "/metrics", "", "secret")
	if rec.Code != 200 {
		t.Fatalf("/metrics with-token = %d want 200", rec.Code)
	}
	if ct := rec.Header().Get("Content-Type"); !strings.HasPrefix(ct, "text/plain") {
		t.Errorf("Content-Type = %q want text/plain…", ct)
	}
	body := rec.Body.String()
	for _, want := range []string{
		"# HELP agezt_active_runs runs in flight",
		"# TYPE agezt_active_runs gauge",
		"agezt_active_runs 2",
		"agezt_halted 0",
		"agezt_spend_today_microcents 1500000000",
	} {
		if !strings.Contains(body, want) {
			t.Errorf("metrics body missing %q\n--- got ---\n%s", want, body)
		}
	}
}

// TestMetrics_NoSourceIsEmpty — without an injected source /metrics is a valid
// empty 200 (Prometheus scrapes it without error).
func TestMetrics_NoSourceIsEmpty(t *testing.T) {
	s := newServer(t, &fakeEngine{model: "m"}, "secret")
	rec := do(t, s, "GET", "/metrics", "", "secret")
	if rec.Code != 200 {
		t.Errorf("/metrics empty = %d want 200", rec.Code)
	}
	if strings.TrimSpace(rec.Body.String()) != "" {
		t.Errorf("/metrics with no source should be empty, got %q", rec.Body.String())
	}
}

func TestPromName_SanitizesToValidIdentifier(t *testing.T) {
	// Prometheus metric names must match [a-zA-Z_:][a-zA-Z0-9_:]*. A name with a
	// dot/dash/space would emit a line Prometheus can't parse, breaking the WHOLE
	// scrape. promName coerces any such name to a valid identifier.
	cases := map[string]string{
		"agezt_up":            "agezt_up", // already valid → unchanged
		"agezt_model.latency": "agezt_model_latency",
		"agezt_tool-calls":    "agezt_tool_calls",
		"agezt_a b":           "agezt_a_b",
		"weird:name":          "weird:name", // ':' is valid in Prometheus names
		"":                    "_",
		"9lives":              "_9lives", // leading digit gets prefixed
	}
	identifier := regexp.MustCompile(`^[a-zA-Z_:][a-zA-Z0-9_:]*$`)
	for in, want := range cases {
		got := promName(in)
		if got != want {
			t.Errorf("promName(%q) = %q, want %q", in, got, want)
		}
		if !identifier.MatchString(got) {
			t.Errorf("promName(%q) = %q is not a valid Prometheus identifier", in, got)
		}
	}
}
