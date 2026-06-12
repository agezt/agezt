// SPDX-License-Identifier: MIT

package configcenter

import (
	"os"
	"path/filepath"
	"time"
)

// Config holds the configuration for the Config Center.
type Config struct {
	// Dir is the directory where config data is stored.
	Dir string

	// StoreType is the type of storage ("json" for file-based, "memory" for testing).
	StoreType string

	// AccessPolicies defines default policies per rating.
	AccessPolicies map[Rating]Policy

	// Hitl contains Human-in-the-Loop configuration.
	Hitl HitlConfig

	// RateLimits contains rate limiting configuration.
	RateLimits RateLimitConfig

	// Audit contains audit logging configuration.
	Audit AuditConfig

	// Vault contains vault integration settings.
	Vault VaultConfig

	// Classifier contains secret classifier settings.
	Classifier ClassifierConfig
}

// DefaultConfig returns a default configuration for the Config Center.
func DefaultConfig(baseDir string) *Config {
	dir := filepath.Join(baseDir, "configcenter")
	
	return &Config{
		Dir:        dir,
		StoreType:  "json",
		
		AccessPolicies: map[Rating]Policy{
			RatingPublic:     PolicyAuto,
			RatingInternal:   PolicyAuto,
			RatingRestricted: PolicyHITL,
			RatingSecret:     PolicyDeny,
		},
		
		Hitl: HitlConfig{
			TimeoutMinutes:       5,
			AutoDenyOnTimeout:   true,
			NotifyChannels:      []string{},
		},
		
		RateLimits: RateLimitConfig{
			PerAgentPerMinute: 120,
			PerKeyPerMinute:   600,
		},
		
		Audit: AuditConfig{
			LogAllAccess:     true,
			LogPublicValues:   false,
			RetentionDays:    90,
		},
		
		Vault: VaultConfig{
			Enabled:     false,
			Provider:    "",
			Address:     "",
			AuthMethod:  "token",
			MountPath:   "",
			RefreshStrategy: map[Rating]RefreshStrategy{
				RatingSecret:     RefreshAlways,
				RatingRestricted: RefreshCache5m,
				RatingInternal:   RefreshCache5m,
				RatingPublic:     RefreshCache1h,
			},
		},
		
		Classifier: ClassifierConfig{
			AutoClassify: true,
			Overrides:    map[string]Rating{},
		},
	}
}

// Rating represents the sensitivity level of a config value.
type Rating string

const (
	RatingPublic     Rating = "public"
	RatingInternal   Rating = "internal"
	RatingRestricted Rating = "restricted"
	RatingSecret     Rating = "secret"
)

// Policy determines how access is granted for a given rating.
type Policy string

const (
	// PolicyAuto allows access automatically (for public/internal).
	PolicyAuto Policy = "auto"
	// PolicyHITL requires human-in-the-loop approval.
	PolicyHITL Policy = "hitl"
	// PolicyDeny always denies access.
	PolicyDeny Policy = "deny"
	// PolicySecretDeny is a special deny policy for secrets.
	PolicySecretDeny Policy = "secret_deny"
)

// RefreshStrategy determines how often to refresh values from vault.
type RefreshStrategy string

const (
	RefreshAlways  RefreshStrategy = "always_refresh" // Always fetch fresh from vault
	RefreshCache5m RefreshStrategy = "cache_5m"    // Cache for 5 minutes
	RefreshCache1h RefreshStrategy = "cache_1h"    // Cache for 1 hour
)

// HitlConfig contains HITL (Human-in-the-Loop) settings.
type HitlConfig struct {
	// TimeoutMinutes is how long to wait for operator response.
	TimeoutMinutes int

	// AutoDenyOnTimeout denies access if operator doesn't respond within timeout.
	AutoDenyOnTimeout bool

	// NotifyChannels is a list of channels to notify for pending approvals.
	NotifyChannels []string
}

// RateLimitConfig contains rate limiting settings.
type RateLimitConfig struct {
	// PerAgentPerMinute limits config accesses per agent per minute.
	PerAgentPerMinute int

	// PerKeyPerMinute limits config accesses per key per minute.
	PerKeyPerMinute int
}

// AuditConfig contains audit logging settings.
type AuditConfig struct {
	// LogAllAccess logs even auto-allowed accesses.
	LogAllAccess bool

	// LogPublicValues logs public value accesses (can be noisy).
	LogPublicValues bool

	// RetentionDays how many days to keep audit logs.
	RetentionDays int
}

// VaultConfig contains vault integration settings.
type VaultConfig struct {
	Enabled          bool
	Provider         string // "hashicorp", "aws-secrets", "azure-keyvault"
	Address          string
	AuthMethod       string // "token", "k8s", "aws", "azure"
	MountPath        string
	Token           string
	K8SRole         string
	RefreshStrategy  map[Rating]RefreshStrategy
}

// ClassifierConfig contains secret classifier settings.
type ClassifierConfig struct {
	// AutoClassify enables automatic secret detection.
	AutoClassify bool

	// Overrides allows manual rating assignments.
	Overrides map[string]Rating
}

// EnsureDefaults sets default values if not configured.
func (c *Config) EnsureDefaults(baseDir string) {
	if c.Dir == "" {
		c.Dir = filepath.Join(baseDir, "configcenter")
	}
	if c.AccessPolicies == nil {
		c.AccessPolicies = map[Rating]Policy{
			RatingPublic:     PolicyAuto,
			RatingInternal:   PolicyAuto,
			RatingRestricted: PolicyHITL,
			RatingSecret:     PolicyDeny,
		}
	}
	if c.Hitl.TimeoutMinutes == 0 {
		c.Hitl.TimeoutMinutes = 5
	}
	if c.RateLimits.PerAgentPerMinute == 0 {
		c.RateLimits.PerAgentPerMinute = 120
	}
	if c.RateLimits.PerKeyPerMinute == 0 {
		c.RateLimits.PerKeyPerMinute = 600
	}
	if c.Audit.RetentionDays == 0 {
		c.Audit.RetentionDays = 90
	}
	
	// Ensure directory exists
	os.MkdirAll(c.Dir, 0755)
}

// Timeout returns the HITL timeout as a duration.
func (h *HitlConfig) Timeout() time.Duration {
	return time.Duration(h.TimeoutMinutes) * time.Minute
}
