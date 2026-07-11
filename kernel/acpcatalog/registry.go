// SPDX-License-Identifier: MIT

package acpcatalog

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"os/exec"
	"regexp"
	"runtime"
	"sort"
	"strings"
	"sync"
	"time"
)

// OfficialRegistryURL is the stable v1 index published by the ACP project.
const OfficialRegistryURL = "https://cdn.agentclientprotocol.com/registry/v1/latest/registry.json"

const (
	registryMaxBytes  = 2 << 20
	registryMaxAgents = 512
	registryCacheTTL  = 15 * time.Minute
)

var registryIDPattern = regexp.MustCompile(`^[a-z][a-z0-9-]*$`)
var registryEnvPattern = regexp.MustCompile(`^[A-Za-z_][A-Za-z0-9_]*$`)

// PackageDistribution is an npx/uvx launch recipe from the official registry.
type PackageDistribution struct {
	Package string            `json:"package"`
	Args    []string          `json:"args,omitempty"`
	Env     map[string]string `json:"env,omitempty"`
}

// BinaryTarget describes one downloadable binary for a concrete OS/architecture.
type BinaryTarget struct {
	Archive string            `json:"archive"`
	Cmd     string            `json:"cmd"`
	Args    []string          `json:"args,omitempty"`
	Env     map[string]string `json:"env,omitempty"`
}

// Distribution mirrors the v1 ACP registry distribution union.
type Distribution struct {
	Binary map[string]BinaryTarget `json:"binary,omitempty"`
	NPX    *PackageDistribution    `json:"npx,omitempty"`
	UVX    *PackageDistribution    `json:"uvx,omitempty"`
}

// RegistryAgent is one official ACP registry manifest.
type RegistryAgent struct {
	ID           string       `json:"id"`
	Name         string       `json:"name"`
	Version      string       `json:"version"`
	Description  string       `json:"description"`
	Repository   string       `json:"repository,omitempty"`
	Website      string       `json:"website,omitempty"`
	Authors      []string     `json:"authors,omitempty"`
	License      string       `json:"license,omitempty"`
	Icon         string       `json:"icon,omitempty"`
	Distribution Distribution `json:"distribution"`
}

// Registry is the official aggregated v1 document.
type Registry struct {
	Version string          `json:"version"`
	Agents  []RegistryAgent `json:"agents"`
}

type registrySnapshot struct {
	registry  Registry
	fetchedAt time.Time
}

// RegistryClient fetches and validates the official index with a bounded body
// and an in-memory TTL cache. A stale snapshot remains usable when refresh
// fails, so a transient CDN outage never erases the UI catalog.
type RegistryClient struct {
	URL  string
	HTTP *http.Client
	TTL  time.Duration
	Now  func() time.Time

	mu       sync.Mutex
	snapshot *registrySnapshot
}

func NewRegistryClient(url string) *RegistryClient {
	return &RegistryClient{
		URL:  url,
		HTTP: &http.Client{Timeout: 6 * time.Second},
		TTL:  registryCacheTTL,
		Now:  time.Now,
	}
}

// DefaultRegistry is shared by the control-plane inventory and the acp_agent
// selector, so a UI refresh also warms delegation lookups.
var DefaultRegistry = NewRegistryClient(OfficialRegistryURL)

// Fetch returns the current validated registry. cached reports whether the
// returned data came from memory. When a forced refresh fails after a previous
// success, the stale snapshot is returned together with the refresh error.
func (c *RegistryClient) Fetch(ctx context.Context, force bool) (reg Registry, fetchedAt time.Time, cached bool, err error) {
	if c == nil {
		return Registry{}, time.Time{}, false, errors.New("ACP registry client is nil")
	}
	c.mu.Lock()
	defer c.mu.Unlock()

	now := time.Now()
	if c.Now != nil {
		now = c.Now()
	}
	ttl := c.TTL
	if ttl <= 0 {
		ttl = registryCacheTTL
	}
	if !force && c.snapshot != nil && now.Sub(c.snapshot.fetchedAt) < ttl {
		return c.snapshot.registry, c.snapshot.fetchedAt, true, nil
	}

	reg, fetchErr := c.fetch(ctx)
	if fetchErr != nil {
		if c.snapshot != nil {
			return c.snapshot.registry, c.snapshot.fetchedAt, true, fetchErr
		}
		return Registry{}, time.Time{}, false, fetchErr
	}
	c.snapshot = &registrySnapshot{registry: reg, fetchedAt: now.UTC()}
	return reg, c.snapshot.fetchedAt, false, nil
}

func (c *RegistryClient) fetch(ctx context.Context) (Registry, error) {
	url := strings.TrimSpace(c.URL)
	if url == "" {
		return Registry{}, errors.New("ACP registry URL is empty")
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, url, nil)
	if err != nil {
		return Registry{}, fmt.Errorf("build ACP registry request: %w", err)
	}
	req.Header.Set("Accept", "application/json")
	req.Header.Set("User-Agent", "agezt-acp-registry/1")
	client := c.HTTP
	if client == nil {
		client = &http.Client{Timeout: 6 * time.Second}
	}
	resp, err := client.Do(req)
	if err != nil {
		return Registry{}, fmt.Errorf("fetch ACP registry: %w", err)
	}
	defer resp.Body.Close()
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return Registry{}, fmt.Errorf("fetch ACP registry: HTTP %d", resp.StatusCode)
	}
	if resp.ContentLength > registryMaxBytes {
		return Registry{}, fmt.Errorf("ACP registry exceeds %d bytes", registryMaxBytes)
	}
	body, err := io.ReadAll(io.LimitReader(resp.Body, registryMaxBytes+1))
	if err != nil {
		return Registry{}, fmt.Errorf("read ACP registry: %w", err)
	}
	if len(body) > registryMaxBytes {
		return Registry{}, fmt.Errorf("ACP registry exceeds %d bytes", registryMaxBytes)
	}
	var reg Registry
	if err := json.Unmarshal(body, &reg); err != nil {
		return Registry{}, fmt.Errorf("decode ACP registry: %w", err)
	}
	if err := validateRegistry(reg); err != nil {
		return Registry{}, err
	}
	return reg, nil
}

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

func countInventory(inv *Inventory) {
	inv.RegisteredCount, inv.CompatibleCount, inv.RunnableCount, inv.InstalledCount = 0, 0, 0, 0
	for _, st := range inv.Agents {
		if st.Registered {
			inv.RegisteredCount++
		}
		if st.Compatible {
			inv.CompatibleCount++
		}
		if st.Runnable {
			inv.RunnableCount++
		}
		if st.Installed {
			inv.InstalledCount++
		}
	}
	inv.MissingCount = len(inv.Agents) - inv.InstalledCount
}

type registryLaunch struct {
	Program       string
	Args          []string
	Env           map[string]string
	Runner        string
	Display       string
	Archive       string
	InstalledPath string
}

// LaunchSpec is a resolved ACP subprocess recipe. Registry entries are always
// represented as a program plus argument vector (Shell=false), so no registry
// field is interpolated into a command shell. Shell=true is reserved for the
// operator-controlled AGEZT_ACP_AGENT_CMD fallback.
type LaunchSpec struct {
	Program string
	Args    []string
	Env     map[string]string
	Display string
	Shell   bool
}

// ResolveLaunch resolves an untrusted per-call selector strictly as either a
// built-in slug or an exact official registry id. It never treats selector text
// as a command. Package distributions run through pinned npx/uvx recipes; binary
// distributions must already be on PATH. Only the empty-selector fallback may
// contain an operator-authored shell command.
func ResolveLaunch(ctx context.Context, ref, fallback string) (LaunchSpec, error) {
	ref = strings.TrimSpace(strings.ToLower(ref))
	if ref == "" {
		fallback = strings.TrimSpace(fallback)
		if fallback == "" {
			return LaunchSpec{}, errors.New("no default ACP agent is configured; select a registry agent")
		}
		return LaunchSpec{Program: fallback, Display: fallback, Shell: true}, nil
	}
	if !registryIDPattern.MatchString(ref) {
		return LaunchSpec{}, fmt.Errorf("invalid ACP registry agent id %q", ref)
	}

	// Prefer an already-installed direct binary for the common offline catalog.
	if a, ok := Find(ref); ok && Installed(a) {
		fields := strings.Fields(a.Command)
		if len(fields) == 0 {
			return LaunchSpec{}, fmt.Errorf("ACP agent %q has an empty launch command", ref)
		}
		program := fields[0]
		if path, err := exec.LookPath(a.Bin); err == nil {
			program = path
		}
		return LaunchSpec{Program: program, Args: fields[1:], Display: a.Command}, nil
	}

	reg, _, _, fetchErr := DefaultRegistry.Fetch(ctx, false)
	for _, a := range reg.Agents {
		if a.ID != ref {
			continue
		}
		launch, compatible, runnable := launchForRegistryAgent(a)
		if !compatible {
			return LaunchSpec{}, fmt.Errorf("ACP agent %q has no distribution for %s", ref, platformID())
		}
		if !runnable {
			if launch.Runner == "binary" && launch.Archive != "" {
				return LaunchSpec{}, fmt.Errorf("ACP agent %q is not installed; binary: %s", ref, launch.Archive)
			}
			return LaunchSpec{}, fmt.Errorf("ACP agent %q needs %s on PATH", ref, launch.Runner)
		}
		return LaunchSpec{
			Program: launch.Program, Args: launch.Args, Env: launch.Env,
			Display: launch.Display,
		}, nil
	}
	if fetchErr != nil {
		return LaunchSpec{}, fmt.Errorf("resolve ACP agent %q: registry unavailable: %w", ref, fetchErr)
	}
	return LaunchSpec{}, fmt.Errorf("ACP agent %q is not registered", ref)
}

func launchForRegistryAgent(a RegistryAgent) (registryLaunch, bool, bool) {
	platform := platformID()
	if target, ok := a.Distribution.Binary[platform]; ok {
		program := strings.TrimPrefix(strings.TrimSpace(target.Cmd), "./")
		if program != "" {
			candidate := program
			if i := strings.LastIndexAny(candidate, "/\\"); i >= 0 {
				candidate = candidate[i+1:]
			}
			if path, err := exec.LookPath(candidate); err == nil {
				l := registryLaunch{Program: path, Args: target.Args, Env: target.Env, Runner: "binary", Archive: target.Archive, InstalledPath: path}
				l.Display = renderCommand(candidate, target.Args)
				return l, true, true
			}
		}
	}
	if d := a.Distribution.NPX; d != nil {
		l := registryLaunch{Program: "npx", Args: append([]string{"--yes", d.Package}, d.Args...), Env: d.Env, Runner: "npx"}
		l.Display = renderCommand(l.Program, l.Args)
		_, err := exec.LookPath(l.Program)
		return l, true, err == nil
	}
	if d := a.Distribution.UVX; d != nil {
		l := registryLaunch{Program: "uvx", Args: append([]string{d.Package}, d.Args...), Env: d.Env, Runner: "uvx"}
		l.Display = renderCommand(l.Program, l.Args)
		_, err := exec.LookPath(l.Program)
		return l, true, err == nil
	}
	if target, ok := a.Distribution.Binary[platform]; ok {
		program := strings.TrimPrefix(strings.TrimSpace(target.Cmd), "./")
		l := registryLaunch{Program: program, Args: target.Args, Env: target.Env, Runner: "binary", Archive: target.Archive}
		l.Display = renderCommand(program, target.Args)
		return l, true, false
	}
	return registryLaunch{}, false, false
}

func platformID() string {
	arch := runtime.GOARCH
	switch arch {
	case "amd64":
		arch = "x86_64"
	case "arm64":
		arch = "aarch64"
	}
	return runtime.GOOS + "-" + arch
}

func commandMatchesLaunch(active string, launch registryLaunch) bool {
	active = strings.TrimSpace(active)
	if active == "" || launch.Program == "" {
		return false
	}
	fields := strings.Fields(active)
	if len(fields) == 0 {
		return false
	}
	first := strings.ToLower(fields[0])
	if i := strings.LastIndexAny(first, "/\\"); i >= 0 {
		first = first[i+1:]
	}
	first = strings.TrimSuffix(first, ".exe")
	want := strings.ToLower(launch.Program)
	if i := strings.LastIndexAny(want, "/\\"); i >= 0 {
		want = want[i+1:]
	}
	want = strings.TrimSuffix(want, ".exe")
	if first != want {
		return false
	}
	// A runner executable alone is not an identity: every npx entry starts with
	// npx and every uvx entry starts with uvx. Require the pinned package too so
	// one configured npx default does not mark the entire registry active.
	if launch.Runner == "npx" || launch.Runner == "uvx" {
		packageIndex := 0
		if launch.Runner == "npx" {
			packageIndex = 1 // args[0] is --yes
		}
		if packageIndex >= len(launch.Args) {
			return false
		}
		wantPackage := launch.Args[packageIndex]
		for _, field := range fields[1:] {
			if field == wantPackage {
				return true
			}
		}
		return false
	}
	return true
}

func renderCommand(program string, args []string) string {
	parts := make([]string, 0, len(args)+1)
	parts = append(parts, displayArg(program))
	for _, arg := range args {
		parts = append(parts, displayArg(arg))
	}
	return strings.TrimSpace(strings.Join(parts, " "))
}

func displayArg(s string) string {
	if s == "" {
		return `""`
	}
	if strings.ContainsAny(s, " \t\r\n\"") {
		return `"` + strings.ReplaceAll(s, `"`, `\"`) + `"`
	}
	return s
}

func firstNonEmpty(values ...string) string {
	for _, v := range values {
		if strings.TrimSpace(v) != "" {
			return v
		}
	}
	return ""
}
