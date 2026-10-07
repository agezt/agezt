// SPDX-License-Identifier: MIT
package channels

import (
	"context"
	"encoding/json"
	"reflect"
	"strings"
	"testing"

	"github.com/agezt/agezt/internal/brand"
	"github.com/agezt/agezt/kernel/acpcatalog"
)

func TestChannelACPSelectedContextActiveRefreshAndCompleteInventory(t *testing.T) {
	want := ACPOutput{OS: "owned-os", Arch: "owned-arch", Platform: "owned-platform", ActiveCommand: "catalog authoritative", InstalledCount: 1, MissingCount: 2, RegistryURL: "owned-registry", RegistryVersion: "1.2.3", RegistryFetchedAt: "owned-time", RegistryCached: true, RegistryError: "owned registry failure", RegisteredCount: 3, CompatibleCount: 4, RunnableCount: 5, ClientsSource: "owned-clients", ClientsRevision: "owned-revision", ClientsFetchedAt: "owned-clients-time", ClientsCached: true, ClientsError: "owned clients failure", ClientCount: 1,
		Agents: []acpcatalog.AgentStatus{{Slug: "owned", Name: "Owned", Bin: "owned-bin", Command: "owned command", Description: "description", Install: "install hint", Docs: "docs", Installed: true, Version: "version", InstalledVersion: "installed version", Path: "owned path", Active: true, Repository: "repository", Website: "website", Icon: "icon", License: "license", Authors: []string{"author"}, RegistryVersion: "registry version", Registered: true, Compatible: true, Runnable: true, Runner: "runner", Archive: "archive", Distributions: []string{"npx", "binary"}}}, Clients: []acpcatalog.ClientEntry{{Name: "owned-client", URL: "client-url", Category: "client category", Description: "client description"}}}
	type ownedKey struct{}
	ctx, cancel := context.WithCancel(context.WithValue(context.Background(), ownedKey{}, "owned"))
	cancel()
	active := " \towned first --arg\n "
	calls, activeCalls := 0, 0
	service := NewACPInventory(func() string { activeCalls++; return active }, func(got context.Context, command string, force bool) ACPOutput {
		calls++
		if got != ctx || got.Value(ownedKey{}) != "owned" || got.Err() != context.Canceled || force || command != strings.TrimSpace(active) {
			t.Fatal("discovery port context/command/refresh")
		}
		return want
	})
	for _, next := range []string{active, " owned second ", " "} {
		active = next
		got, err := service.List(ctx, ACPInput{})
		if err != nil || !reflect.DeepEqual(got, want) {
			t.Fatal(got, err)
		}
	}
	if calls != 3 || activeCalls != 3 {
		t.Fatal(calls, activeCalls)
	}
}

func TestChannelACPDefaultEnvironmentFreshnessEmptyNilAndLargeCount(t *testing.T) {
	commands := []string{}
	service := NewACPInventory(nil, func(_ context.Context, active string, force bool) ACPOutput {
		if force {
			t.Fatal("forced refresh")
		}
		commands = append(commands, active)
		return ACPOutput{OS: "owned", Agents: nil, RegisteredCount: 9007199254740993}
	})
	for _, active := range []string{" owned first ", " \t ", "owned second"} {
		t.Setenv(brand.EnvPrefix+"ACP_AGENT_CMD", active)
		out, err := service.List(context.Background(), ACPInput{})
		raw, _ := json.Marshal(out)
		if err != nil || out.Agents != nil || !strings.Contains(string(raw), `"agents":null`) || !strings.Contains(string(raw), `"registered_count":9007199254740993`) {
			t.Fatal(out, string(raw), err)
		}
	}
	if !reflect.DeepEqual(commands, []string{"owned first", "", "owned second"}) {
		t.Fatal(commands)
	}
	empty, err := NewACPInventory(func() string { return "" }, func(context.Context, string, bool) ACPOutput { return ACPOutput{} }).List(context.Background(), ACPInput{})
	if err != nil || !reflect.DeepEqual(empty, ACPOutput{}) {
		t.Fatal(empty, err)
	}
}
