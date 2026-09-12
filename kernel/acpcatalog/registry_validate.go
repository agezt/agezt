// SPDX-License-Identifier: MIT

// Registry validation + discovery: validateRegistry + packageEnv + validateRegistryEnv + Discover + DiscoverWith + registryStatus + mergeLocalDetection + countInventory.
// Code extracted from registry.go during the Day-70 god-file split. Public API unchanged.
package acpcatalog


import (
	"context"
	"fmt"
	"sort"
	"strings"
	"time"
)


func validateRegistry(reg Registry) error {
	if !strings.HasPrefix(reg.Version, "1.") {
		return fmt.Errorf("unsupported ACP registry schema version %q", reg.Version)
	}
	if len(reg.Agents) == 0 || len(reg.Agents) > registryMaxAgents {
		return fmt.Errorf("ACP registry agent count %d is outside 1..%d", len(reg.Agents), registryMaxAgents)
	}
	seen := make(map[string]struct{}, len(reg.Agents))
	for i, a := range reg.Agents {
		if !registryIDPattern.MatchString(a.ID) {
			return fmt.Errorf("ACP registry agent %d has invalid id %q", i, a.ID)
		}
		if _, exists := seen[a.ID]; exists {
			return fmt.Errorf("ACP registry contains duplicate id %q", a.ID)
		}
		seen[a.ID] = struct{}{}
		if strings.TrimSpace(a.Name) == "" || strings.TrimSpace(a.Version) == "" || strings.TrimSpace(a.Description) == "" {
			return fmt.Errorf("ACP registry agent %q is missing required metadata", a.ID)
		}
		if len(a.Distribution.Binary) == 0 && a.Distribution.NPX == nil && a.Distribution.UVX == nil {
			return fmt.Errorf("ACP registry agent %q has no distribution", a.ID)
		}
		if a.Distribution.NPX != nil && strings.TrimSpace(a.Distribution.NPX.Package) == "" {
			return fmt.Errorf("ACP registry agent %q has an empty npx package", a.ID)
		}
		if a.Distribution.UVX != nil && strings.TrimSpace(a.Distribution.UVX.Package) == "" {
			return fmt.Errorf("ACP registry agent %q has an empty uvx package", a.ID)
		}
		if err := validateRegistryEnv(a.ID, "npx", packageEnv(a.Distribution.NPX)); err != nil {
			return err
		}
		if err := validateRegistryEnv(a.ID, "uvx", packageEnv(a.Distribution.UVX)); err != nil {
			return err
		}
		for platform, target := range a.Distribution.Binary {
			if strings.TrimSpace(target.Archive) == "" || strings.TrimSpace(target.Cmd) == "" {
				return fmt.Errorf("ACP registry agent %q has an incomplete binary target %q", a.ID, platform)
			}
			if err := validateRegistryEnv(a.ID, "binary "+platform, target.Env); err != nil {
				return err
			}
		}
	}
	return nil
}

func packageEnv(d *PackageDistribution) map[string]string {
	if d == nil {
		return nil
	}
	return d.Env
}

func validateRegistryEnv(agentID, distribution string, env map[string]string) error {
	for key := range env {
		if !registryEnvPattern.MatchString(key) {
			return fmt.Errorf("ACP registry agent %q has invalid %s env key %q", agentID, distribution, key)
		}
	}
	return nil
}

// Discover merges local executable probes with the official registry. Remote
// failure is reported in-band and falls back to the built-in local catalog.
func Discover(ctx context.Context, activeCmd string, forceRefresh bool) Inventory {
	// The agent registry CDN and the official clients.mdx source are
	// independent. Fetch them concurrently so a cold UI load costs one network
	// round trip—and a timeout on one source does not serially delay the other.
	registryResult := make(chan Inventory, 1)
	go func() {
		registryResult <- DiscoverWith(ctx, activeCmd, forceRefresh, DefaultRegistry)
	}()
	client := DefaultClients
	entries, revision, fetchedAt, cached, err := client.Fetch(ctx, forceRefresh)
	inv := <-registryResult
	return attachClientsResult(inv, client.URL, entries, revision, fetchedAt, cached, err)
}

func DiscoverWith(ctx context.Context, activeCmd string, forceRefresh bool, client *RegistryClient) Inventory {
	local := Detect(ctx, activeCmd)
	local.RegistryURL = OfficialRegistryURL
	if client == nil {
		client = DefaultRegistry
	}
	if strings.TrimSpace(client.URL) != "" {
		local.RegistryURL = client.URL
	}
	reg, fetchedAt, cached, err := client.Fetch(ctx, forceRefresh)
	if err != nil {
		local.RegistryError = err.Error()
	}
	if len(reg.Agents) == 0 {
		return local
	}

	localBySlug := make(map[string]AgentStatus, len(local.Agents))
	for _, st := range local.Agents {
		localBySlug[st.Slug] = st
	}

	inv := Inventory{
		OS: local.OS, Arch: local.Arch, Platform: local.Platform,
		ActiveCommand: local.ActiveCommand,
		RegistryURL:   local.RegistryURL, RegistryVersion: reg.Version,
		RegistryFetchedAt: fetchedAt.UTC().Format(time.RFC3339), RegistryCached: cached,
	}
	if err != nil {
		inv.RegistryError = err.Error()
	}
	inv.Agents = make([]AgentStatus, 0, len(reg.Agents)+len(local.Agents))
	seen := make(map[string]struct{}, len(reg.Agents))
	for _, a := range reg.Agents {
		st := registryStatus(a, reg.Version, local.ActiveCommand)
		if detected, ok := localBySlug[a.ID]; ok {
			mergeLocalDetection(&st, detected)
		}
		inv.Agents = append(inv.Agents, st)
		seen[a.ID] = struct{}{}
	}
	// Preserve an offline fallback entry if the remote registry ever drops it.
	for _, st := range local.Agents {
		if _, ok := seen[st.Slug]; ok {
			continue
		}
		inv.Agents = append(inv.Agents, st)
	}
	sort.SliceStable(inv.Agents, func(i, j int) bool {
		return strings.ToLower(inv.Agents[i].Name) < strings.ToLower(inv.Agents[j].Name)
	})
	countInventory(&inv)
	return inv
}

func registryStatus(a RegistryAgent, schemaVersion, activeCmd string) AgentStatus {
	st := AgentStatus{
		Slug: a.ID, Name: a.Name, Description: a.Description,
		Docs: firstNonEmpty(a.Website, a.Repository), Repository: a.Repository,
		Website: a.Website, Icon: a.Icon, License: a.License, Authors: a.Authors,
		Version: a.Version, RegistryVersion: schemaVersion, Registered: true,
	}
	if a.Distribution.NPX != nil {
		st.Distributions = append(st.Distributions, "npx")
	}
	if a.Distribution.UVX != nil {
		st.Distributions = append(st.Distributions, "uvx")
	}
	if len(a.Distribution.Binary) > 0 {
		st.Distributions = append(st.Distributions, "binary")
	}
	launch, compatible, runnable := launchForRegistryAgent(a)
	st.Compatible, st.Runnable = compatible, runnable
	st.Runner, st.Command, st.Install, st.Archive = launch.Runner, launch.Display, launch.Display, launch.Archive
	st.Bin = launch.Program
	st.Active = strings.EqualFold(strings.TrimSpace(activeCmd), a.ID) || commandMatchesLaunch(activeCmd, launch)
	if launch.InstalledPath != "" {
		st.Installed = true
		st.Path = launch.InstalledPath
	}
	return st
}

func mergeLocalDetection(dst *AgentStatus, local AgentStatus) {
	dst.Installed = local.Installed
	dst.InstalledVersion = local.InstalledVersion
	dst.Path = local.Path
	if local.Installed {
		dst.Runnable = true
		dst.Runner = "binary"
		dst.Bin = local.Bin
		dst.Command = local.Command
		dst.Install = local.Install
	}
	if local.Active {
		dst.Active = true
	}
}
