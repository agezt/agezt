// SPDX-License-Identifier: MIT

// Netguard publisher + voice/stt shims + policy/warden/capability selectors.
// Extracted from main_pulse.go during Day 211 god-file refactor (#57).
// Public API unchanged.
package main

import (
	"context"
	"fmt"
	"os"
	"strings"

	"github.com/agezt/agezt/internal/brand"
	"github.com/agezt/agezt/kernel/bus"
	"github.com/agezt/agezt/kernel/edict"
	"github.com/agezt/agezt/kernel/event"
	kernelruntime "github.com/agezt/agezt/kernel/runtime"
	"github.com/agezt/agezt/kernel/stt"
	"github.com/agezt/agezt/kernel/warden"
)

func netguardPublish(b *bus.Bus) func(tool string) func(ip, reason string) {
	if b == nil {
		return nil
	}
	return func(tool string) func(ip, reason string) {
		return func(ip, reason string) {
			_, _ = b.Publish(event.Spec{
				Subject: "netguard.block",
				Kind:    event.KindNetguardBlocked,
				Actor:   tool,
				Payload: map[string]any{"ip": ip, "reason": reason, "tool": tool},
			})
		}
	}
}
type voiceTranscriberShim struct{ v kernelruntime.Voice }
func (s voiceTranscriberShim) Transcribe(ctx context.Context, filename string, audio []byte) (string, error) {
	return s.v.Transcribe(ctx, audio, filename)
}
func sttTranscriberFromEnv() *stt.Client {
	key := strings.TrimSpace(os.Getenv(brand.EnvPrefix + "STT_API_KEY"))
	if key == "" {
		key = strings.TrimSpace(os.Getenv("OPENAI_API_KEY"))
	}
	url := strings.TrimSpace(os.Getenv(brand.EnvPrefix + "STT_API_URL"))
	if key == "" && url == "" {
		return nil
	}
	return stt.New(stt.Config{
		APIURL: url,
		APIKey: key,
		Model:  strings.TrimSpace(os.Getenv(brand.EnvPrefix + "STT_MODEL")),
	})
}
func selectAskPolicy() (edict.AskPolicy, string) {
	switch strings.ToLower(strings.TrimSpace(os.Getenv(brand.EnvPrefix + "APPROVAL_MODE"))) {
	case "deny":
		return edict.AskDeny, "AskDeny (strict; only L4 calls run)"
	case "prompt", "ask":
		return edict.AskPrompt, "AskPrompt (live HITL via `agt approve|deny`)"
	case "", "allow":
		return edict.AskAllow, "AskAllow (Ask-class folded to Allow + WouldAsk)"
	default:
		// Unknown values fall back to the safe default; surface the
		// fact in the banner so the operator notices the typo.
		return edict.AskAllow, fmt.Sprintf("AskAllow (unknown %sAPPROVAL_MODE=%q ignored)",
			brand.EnvPrefix, os.Getenv(brand.EnvPrefix+"APPROVAL_MODE"))
	}
}
func wardenOptionsFromEnv() (warden.Options, string) {
	raw := strings.ToLower(strings.TrimSpace(os.Getenv(brand.EnvPrefix + "WARDEN_DOCKER")))
	if raw != "1" && raw != "true" && raw != "yes" && raw != "on" {
		return warden.Options{}, ""
	}
	runtimeName := strings.TrimSpace(os.Getenv(brand.EnvPrefix + "WARDEN_DOCKER_RUNTIME"))
	if runtimeName == "" {
		runtimeName = "docker"
	}
	image := strings.TrimSpace(os.Getenv(brand.EnvPrefix + "WARDEN_DOCKER_IMAGE"))
	if image == "" {
		image = "python:3.12-slim"
	}
	network := strings.TrimSpace(os.Getenv(brand.EnvPrefix + "WARDEN_DOCKER_NETWORK"))
	if network == "" {
		network = "none"
	}
	return warden.Options{
		Container: warden.ContainerOptions{
			Enabled: true,
			Runtime: runtimeName,
			Image:   image,
			Network: network,
		},
	}, fmt.Sprintf("; container=%s image=%s network=%s", runtimeName, image, network)
}
func selectAutoApproveCapabilities() (map[string]bool, string) {
	raw := strings.TrimSpace(os.Getenv(brand.EnvPrefix + "AUTO_APPROVE_CAPS"))
	switch strings.ToLower(raw) {
	case "off", "0", "false", "no", "none":
		return nil, "off (set " + brand.EnvPrefix + "AUTO_APPROVE_CAPS=all or a comma list)"
	case "", "all", "1", "true", "yes", "on":
		caps := map[string]bool{}
		for _, c := range edict.AllCapabilities() {
			caps[string(c)] = true
		}
		return caps, fmt.Sprintf("on (%d known capabilities; hard-deny/SSRF/budget guards still apply)", len(caps))
	default:
		caps := map[string]bool{}
		var unknown []string
		for _, item := range splitNonEmpty(raw) {
			if edict.KnownCapability(item) {
				caps[item] = true
			} else {
				unknown = append(unknown, item)
			}
		}
		if len(caps) == 0 {
			return nil, fmt.Sprintf("off (no known capabilities in %sAUTO_APPROVE_CAPS=%q)", brand.EnvPrefix, raw)
		}
		desc := fmt.Sprintf("on (%d selected capabilities)", len(caps))
		if len(unknown) > 0 {
			desc += fmt.Sprintf("; ignored unknown: %s", strings.Join(unknown, ", "))
		}
		return caps, desc
	}
}
var _ = event.GenesisHash
