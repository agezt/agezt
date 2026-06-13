// SPDX-License-Identifier: MIT

package agentgw

import (
	"fmt"
	"strings"

	"github.com/agezt/agezt/kernel/edict"
)

// CapabilityChecker validates capability access.
type CapabilityChecker struct {
	// allowedCaps maps capability names to their edict.Capability counterparts
	allowedCaps map[AgentCapability]edict.Capability
}

// NewCapabilityChecker creates a new capability checker.
func NewCapabilityChecker() *CapabilityChecker {
	cc := &CapabilityChecker{
		allowedCaps: make(map[AgentCapability]edict.Capability),
	}

	// Map agent capabilities to edict capabilities
	cc.allowedCaps[CapEventbusPublish] = edict.CapNotify // reusing notify as closest match
	cc.allowedCaps[CapEventbusSubscribe] = edict.CapNotify

	cc.allowedCaps[CapMemoryRead] = edict.CapMemory
	cc.allowedCaps[CapMemoryWrite] = edict.CapMemory
	cc.allowedCaps[CapMemoryDelete] = edict.CapMemory
	cc.allowedCaps[CapMemorySearch] = edict.CapMemory
	cc.allowedCaps[CapMemoryList] = edict.CapMemory

	cc.allowedCaps[CapLogRead] = edict.CapNotify // logs are like notifications
	cc.allowedCaps[CapLogWrite] = edict.CapNotify

	cc.allowedCaps[CapAgentList] = edict.CapDelegate
	cc.allowedCaps[CapAgentQuery] = edict.CapDelegate

	// DB capabilities are not yet mapped to edict capabilities
	// Channel capabilities are not yet mapped

	return cc
}

// Check validates that the token has the required capability.
func (cc *CapabilityChecker) Check(claims *TokenClaims, required AgentCapability) error {
	// Check if the required capability is in the token's granted caps
	for _, cap := range claims.Caps {
		if AgentCapability(cap) == required {
			return nil
		}
	}

	return fmt.Errorf("agentgw: capability %q not granted", required)
}

// CheckAny validates that the token has at least one of the required capabilities.
func (cc *CapabilityChecker) CheckAny(claims *TokenClaims, required ...AgentCapability) error {
	for _, cap := range required {
		if err := cc.Check(claims, cap); err == nil {
			return nil
		}
	}
	return fmt.Errorf("agentgw: none of the required capabilities %v granted", required)
}

// CheckAll validates that the token has all of the required capabilities.
func (cc *CapabilityChecker) CheckAll(claims *TokenClaims, required ...AgentCapability) error {
	for _, cap := range required {
		if err := cc.Check(claims, cap); err != nil {
			return err
		}
	}
	return nil
}

// ParseCapability parses a capability string into an AgentCapability.
func ParseCapability(s string) (AgentCapability, error) {
	s = strings.ToLower(strings.TrimSpace(s))
	switch AgentCapability(s) {
	case CapEventbusPublish:
		return CapEventbusPublish, nil
	case CapEventbusSubscribe:
		return CapEventbusSubscribe, nil
	case CapChannelSend:
		return CapChannelSend, nil
	case CapChannelRead:
		return CapChannelRead, nil
	case CapChannelList:
		return CapChannelList, nil
	case CapMemoryRead:
		return CapMemoryRead, nil
	case CapMemoryWrite:
		return CapMemoryWrite, nil
	case CapMemoryDelete:
		return CapMemoryDelete, nil
	case CapMemorySearch:
		return CapMemorySearch, nil
	case CapMemoryList:
		return CapMemoryList, nil
	case CapLogRead:
		return CapLogRead, nil
	case CapLogWrite:
		return CapLogWrite, nil
	case CapAgentList:
		return CapAgentList, nil
	case CapAgentQuery:
		return CapAgentQuery, nil
	case CapDBQuery:
		return CapDBQuery, nil
	case CapDBRead:
		return CapDBRead, nil
	case CapDBWrite:
		return CapDBWrite, nil
	case CapConfigAccess:
		return CapConfigAccess, nil
	case CapConfigList:
		return CapConfigList, nil
	case CapConfigSearch:
		return CapConfigSearch, nil
	case CapConfigWrite:
		return CapConfigWrite, nil
	default:
		return "", fmt.Errorf("agentgw: unknown capability %q", s)
	}
}

// CapsSubset returns the capabilities in child that are NOT present in parent.
// An empty result means child is a subset of parent (no escalation).
func CapsSubset(child, parent []string) []string {
	have := make(map[string]bool, len(parent))
	for _, c := range parent {
		have[strings.ToLower(strings.TrimSpace(c))] = true
	}
	var missing []string
	for _, c := range child {
		if !have[strings.ToLower(strings.TrimSpace(c))] {
			missing = append(missing, c)
		}
	}
	return missing
}

// CapsIntersect returns the capabilities present in BOTH want and parent,
// preserving want's order and dropping anything the parent does not grant.
func CapsIntersect(want, parent []string) []string {
	have := make(map[string]bool, len(parent))
	for _, c := range parent {
		have[strings.ToLower(strings.TrimSpace(c))] = true
	}
	out := make([]string, 0, len(want))
	for _, c := range want {
		if have[strings.ToLower(strings.TrimSpace(c))] {
			out = append(out, c)
		}
	}
	return out
}

// NormalizeCaps normalizes and validates a list of capability strings.
func NormalizeCaps(caps []string) ([]string, error) {
	normalized := make([]string, 0, len(caps))
	seen := make(map[string]bool)

	for _, cap := range caps {
		parsed, err := ParseCapability(cap)
		if err != nil {
			return nil, err
		}
		key := string(parsed)
		if seen[key] {
			continue // deduplicate
		}
		seen[key] = true
		normalized = append(normalized, key)
	}

	return normalized, nil
}
