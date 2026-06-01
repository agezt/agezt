// SPDX-License-Identifier: MIT

package controlplane_test

import (
	"context"
	"testing"
	"time"

	"github.com/agezt/agezt/kernel/controlplane"
	"github.com/agezt/agezt/kernel/governor"
	"github.com/agezt/agezt/plugins/providers/mock"
)

// TestConfigSurfacesRouting verifies CmdConfig includes the effective routing
// tables when the provider is a governor with routes configured (M108).
func TestConfigSurfacesRouting(t *testing.T) {
	reg := governor.NewRegistry()
	if err := reg.Register(&governor.ProviderInfo{
		Name:     "mock",
		Provider: mock.New(mock.FinalText("ok")),
		AuthMode: governor.AuthLocal,
	}); err != nil {
		t.Fatalf("Register: %v", err)
	}
	gov, err := governor.New(governor.Config{
		Registry:               reg,
		DailyCeilingMicrocents: 1_000_000_000,
		TaskRoutes:             governor.TaskRoutes{"plan": {"mock"}},
		TaskModelOverrides:     governor.TaskModelOverrides{"code": "some-model"},
		Now:                    func() time.Time { return time.Date(2026, 5, 29, 12, 0, 0, 0, time.UTC) },
	})
	if err != nil {
		t.Fatalf("governor.New: %v", err)
	}
	_, _, c, _ := startPair(t, gov)

	res, err := c.Call(context.Background(), controlplane.CmdConfig, nil)
	if err != nil {
		t.Fatalf("config: %v", err)
	}
	routing, ok := res["routing"].(map[string]any)
	if !ok {
		t.Fatalf("config result has no routing section: %v", res["routing"])
	}
	routes, ok := routing["routes"].(map[string]any)
	if !ok || routes["plan"] == nil {
		t.Fatalf("routing.routes missing plan: %v", routing["routes"])
	}
	if routing["model_overrides"] == nil {
		t.Errorf("routing.model_overrides missing")
	}
}

// TestConfigNoRoutingWhenUnconfigured verifies the routing section is absent on
// a daemon with no routing configured (keeps default output compact).
func TestConfigNoRoutingWhenUnconfigured(t *testing.T) {
	_, _, c, _ := startPair(t, mock.New(mock.FinalText("ok")))
	res, err := c.Call(context.Background(), controlplane.CmdConfig, nil)
	if err != nil {
		t.Fatalf("config: %v", err)
	}
	if _, present := res["routing"]; present {
		t.Errorf("routing section should be absent without a governor/routes")
	}
}
