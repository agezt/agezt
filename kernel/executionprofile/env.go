// SPDX-License-Identifier: MIT

package executionprofile

import (
	"os"
	"sort"
	"strings"

	"github.com/agezt/agezt/kernel/warden"
)

const (
	EnvLocal        = "AGEZT_EXEC_ENV_LOCAL"
	EnvWarden       = "AGEZT_EXEC_ENV_WARDEN"
	EnvDocker       = "AGEZT_EXEC_ENV_DOCKER"
	SecretEnvLocal  = "AGEZT_EXEC_SECRET_ENV_LOCAL"
	SecretEnvWarden = "AGEZT_EXEC_SECRET_ENV_WARDEN"
	SecretEnvDocker = "AGEZT_EXEC_SECRET_ENV_DOCKER"
)

type EnvPassthroughPolicy struct {
	Profile        string
	EnvNames       []string
	SecretEnvNames []string
}

func ProfileIDForWardenProfile(profile warden.Profile) string {
	switch profile {
	case warden.ProfileNone:
		return IDLocal
	case warden.ProfileContainer:
		return "docker"
	default:
		return IDWarden
	}
}

func EnvPassthroughPolicyFromEnv(profile string) EnvPassthroughPolicy {
	profile = envProfileID(profile)
	return EnvPassthroughPolicy{
		Profile:        profile,
		EnvNames:       parseEnvNames(os.Getenv(envPassthroughVar(profile)), false),
		SecretEnvNames: parseEnvNames(os.Getenv(secretEnvPassthroughVar(profile)), true),
	}
}

func AppendEnvPassthrough(base []string, profile string) []string {
	policy := EnvPassthroughPolicyFromEnv(profile)
	extra := policy.Resolve(os.LookupEnv)
	if len(extra) == 0 {
		return base
	}
	out := append([]string(nil), base...)
	index := make(map[string]int, len(out))
	for i, kv := range out {
		name, _, ok := strings.Cut(kv, "=")
		if ok {
			index[strings.ToUpper(name)] = i
		}
	}
	for _, kv := range extra {
		name, _, ok := strings.Cut(kv, "=")
		if !ok {
			continue
		}
		key := strings.ToUpper(name)
		if i, exists := index[key]; exists {
			out[i] = kv
			continue
		}
		index[key] = len(out)
		out = append(out, kv)
	}
	return out
}

func (p EnvPassthroughPolicy) Resolve(lookup func(string) (string, bool)) []string {
	seen := map[string]bool{}
	var out []string
	for _, name := range p.EnvNames {
		up := strings.ToUpper(name)
		if seen[up] || IsSecretEnvName(name) {
			continue
		}
		if value, ok := lookup(name); ok {
			out = append(out, name+"="+value)
			seen[up] = true
		}
	}
	for _, name := range p.SecretEnvNames {
		up := strings.ToUpper(name)
		if seen[up] || strings.HasPrefix(up, "AGEZT_") {
			continue
		}
		if value, ok := lookup(name); ok {
			out = append(out, name+"="+value)
			seen[up] = true
		}
	}
	return out
}

func ProfileEnvironmentSummary(profile, base string) string {
	names := EnvPassthroughPolicyFromEnv(profile).EnvNames
	if len(names) == 0 {
		return base
	}
	return base + "; explicit env passthrough: " + strings.Join(names, ", ")
}

func ProfileSecretsSummary(profile, base string) string {
	names := EnvPassthroughPolicyFromEnv(profile).SecretEnvNames
	if len(names) == 0 {
		return base
	}
	return base + "; explicit secret env passthrough: " + strings.Join(names, ", ") + " (AGEZT_* blocked)"
}

func IsSecretEnvName(name string) bool {
	up := strings.ToUpper(strings.TrimSpace(name))
	for _, frag := range []string{"KEY", "TOKEN", "SECRET", "PASSWORD", "PASSWD", "CRED", "AWS_", "AGEZT_"} {
		if strings.Contains(up, frag) {
			return true
		}
	}
	return false
}

func parseEnvNames(raw string, allowSecretNames bool) []string {
	seen := map[string]bool{}
	var out []string
	for _, part := range strings.Split(raw, ",") {
		name := strings.TrimSpace(part)
		if !validEnvName(name) {
			continue
		}
		up := strings.ToUpper(name)
		if seen[up] {
			continue
		}
		if !allowSecretNames && IsSecretEnvName(name) {
			continue
		}
		if allowSecretNames && strings.HasPrefix(up, "AGEZT_") {
			continue
		}
		seen[up] = true
		out = append(out, name)
	}
	sort.SliceStable(out, func(i, j int) bool {
		return strings.ToUpper(out[i]) < strings.ToUpper(out[j])
	})
	return out
}

func validEnvName(name string) bool {
	if name == "" {
		return false
	}
	for i, r := range name {
		switch {
		case r >= 'A' && r <= 'Z', r >= 'a' && r <= 'z', r == '_':
			continue
		case i > 0 && r >= '0' && r <= '9':
			continue
		default:
			return false
		}
	}
	return true
}

func envProfileID(profile string) string {
	switch normalizeProfileID(profile) {
	case IDLocal:
		return IDLocal
	case "docker":
		return "docker"
	default:
		return IDWarden
	}
}

func envPassthroughVar(profile string) string {
	switch envProfileID(profile) {
	case IDLocal:
		return EnvLocal
	case "docker":
		return EnvDocker
	default:
		return EnvWarden
	}
}

func secretEnvPassthroughVar(profile string) string {
	switch envProfileID(profile) {
	case IDLocal:
		return SecretEnvLocal
	case "docker":
		return SecretEnvDocker
	default:
		return SecretEnvWarden
	}
}
