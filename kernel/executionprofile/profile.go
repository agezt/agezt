// SPDX-License-Identifier: MIT

// Execution profile core: Build + Inventory.Find + WardenProfileForRun + RoutableRunProfileIDs + RoutableRunProfileIDsFor + localProfile.
// Code extracted from profile.go during the Day-71 god-file split. Public API unchanged.
package executionprofile


import (
	"github.com/agezt/agezt/kernel/warden"
	"runtime"
	"strings"
)



type Status string

const (
	StatusSupported Status = "supported"
	StatusDegraded  Status = "degraded"
	StatusPartial   Status = "partial"
	StatusPlanned   Status = "planned"
)

const (
	IDLocal  = "local"
	IDWarden = "warden"
)

type Profile struct {
	ID                 string        `json:"id"`
	Name               string        `json:"name"`
	Summary            string        `json:"summary"`
	Status             Status        `json:"status"`
	Routed             bool          `json:"routed"`
	RequestedIsolation string        `json:"requested_isolation"`
	EffectiveIsolation string        `json:"effective_isolation"`
	Degraded           bool          `json:"degraded"`
	DegradeReason      string        `json:"degrade_reason,omitempty"`
	Tools              []string      `json:"tools"`
	Backends           []string      `json:"backends"`
	FileSystem         string        `json:"filesystem"`
	Network            string        `json:"network"`
	Environment        string        `json:"environment"`
	Secrets            string        `json:"secrets"`
	SecretPolicy       *SecretPolicy `json:"secret_policy,omitempty"`
	Limits             []string      `json:"limits"`
	BrowserAccess      string        `json:"browser_access"`
	Cleanup            string        `json:"cleanup"`
	PolicyCapability   string        `json:"policy_capability,omitempty"`
	Notes              []string      `json:"notes,omitempty"`
}

type Inventory struct {
	HostOS         string    `json:"host_os"`
	HostArch       string    `json:"host_arch"`
	Profiles       []Profile `json:"profiles"`
	Count          int       `json:"count"`
	RoutedCount    int       `json:"routed_count"`
	SupportedCount int       `json:"supported_count"`
	DegradedCount  int       `json:"degraded_count"`
}

type Options struct {
	Tools            []string
	Warden           warden.Engine
	EffectiveProfile func(warden.Profile) warden.Profile
	SSH              SSHConfig
	K8s              K8sConfig
	Modal            ModalConfig
	Daytona          DaytonaConfig
	HostOS           string
	HostArch         string
}

func Build(opts Options) Inventory {
	hostOS := strings.TrimSpace(opts.HostOS)
	if hostOS == "" {
		hostOS = runtime.GOOS
	}
	hostArch := strings.TrimSpace(opts.HostArch)
	if hostArch == "" {
		hostArch = runtime.GOARCH
	}
	tools := toolSet(opts.Tools)
	effective := opts.EffectiveProfile
	if effective == nil && opts.Warden != nil {
		effective = opts.Warden.EffectiveProfile
	}
	if effective == nil {
		w := warden.New(nil)
		effective = w.EffectiveProfile
	}

	profiles := []Profile{
		localProfile(tools),
		wardenProfile(tools, effective),
		worktreeCodingProfile(tools),
		browserSessionProfile(tools),
		dockerProfile(tools, effective),
		sshProfile(tools, opts.SSH),
		remoteAgeztProfile(tools),
		modalProfile(tools, opts.Modal),
		daytonaProfile(tools, opts.Daytona),
		k8sProfile(tools, opts.K8s),
	}
	inv := Inventory{HostOS: hostOS, HostArch: hostArch, Profiles: profiles, Count: len(profiles)}
	for _, p := range profiles {
		if p.Routed {
			inv.RoutedCount++
		}
		if p.Status == StatusSupported {
			inv.SupportedCount++
		}
		if p.Degraded {
			inv.DegradedCount++
		}
	}
	return inv
}

func (i Inventory) Find(id string) (Profile, bool) {
	id = strings.ToLower(strings.TrimSpace(id))
	for _, p := range i.Profiles {
		if strings.EqualFold(p.ID, id) {
			return p, true
		}
	}
	return Profile{}, false
}

// WardenProfileForRun resolves execution profiles wired into warden-backed
// tools for a single agent run. Some profiles are conditional: docker maps to
// ProfileContainer, but the control plane must still verify the active warden
// backend can satisfy it before accepting the run.
func WardenProfileForRun(id string) (warden.Profile, bool) {
	switch strings.ToLower(strings.TrimSpace(id)) {
	case IDLocal:
		return warden.ProfileNone, true
	case IDWarden:
		return warden.ProfileNamespace, true
	case "docker":
		return warden.ProfileContainer, true
	default:
		return "", false
	}
}

func RoutableRunProfileIDs() []string {
	return []string{IDLocal, IDWarden}
}

func RoutableRunProfileIDsFor(inv Inventory) []string {
	ids := RoutableRunProfileIDs()
	if p, ok := inv.Find("docker"); ok && p.Routed && !p.Degraded && p.EffectiveIsolation == string(warden.ProfileContainer) {
		ids = append(ids, "docker")
	}
	if p, ok := inv.Find("ssh"); ok && p.Routed && !p.Degraded {
		ids = append(ids, "ssh")
	}
	if p, ok := inv.Find("remote-agezt"); ok && p.Routed && !p.Degraded {
		ids = append(ids, "remote-agezt")
	}
	if p, ok := inv.Find("modal"); ok && p.Routed && !p.Degraded {
		ids = append(ids, "modal")
	}
	if p, ok := inv.Find("daytona"); ok && p.Routed && !p.Degraded {
		ids = append(ids, "daytona")
	}
	if p, ok := inv.Find("k8s"); ok && p.Routed && !p.Degraded {
		ids = append(ids, "k8s")
	}
	return ids
}
