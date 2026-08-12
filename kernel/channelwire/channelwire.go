// SPDX-License-Identifier: MIT

// Package channelwire is the channel FACTORY layer (Phase 2.1 of
// docs/REFACTORING-SCAN-2026-08.md): each channel kind registers a Factory
// that builds its configured instances from a Deps bundle, and the daemon
// walks the manifest registry calling BuildKind per kind — one loop instead
// of 27 hand-listed builder call sites (the allInsts drift surface).
//
// The walk lives at the call site (cmd/agezt/main.go) rather than here: a
// BuildAll wrapper over the same three lines existed until 2026-08-12 and
// never had a caller, so it was deleted rather than left as a second way to
// say the same thing.
//
// This package — not kernel/channel — owns the factory types because a
// factory returns a pulse.BriefSink: importing pulse into kernel/channel
// would drag kernel/agent into every transport plugin's transitive deps.
// kernel/channel stays the pure vocabulary (Channel, UnifiedMessage,
// Manifest); channelwire is the wiring above it.
package channelwire

import (
	"context"
	"os"
	"strings"
	"sync"

	"github.com/agezt/agezt/kernel/bus"
	"github.com/agezt/agezt/kernel/channel"
	"github.com/agezt/agezt/kernel/pulse"
	"github.com/agezt/agezt/kernel/settings"
)

// Deps is everything a channel factory may touch. Empirically this is the
// builders' ENTIRE kernel surface (verified across all 27 in cmd/agezt):
// the event bus, the shared inbound handler, and a config getter that
// resolves a base env name for this instance's account label.
type Deps struct {
	// Ctx is the daemon lifetime context; brief-sink closures capture it for
	// their outbound Send calls.
	Ctx context.Context
	// Bus is the kernel event bus (k.Bus()).
	Bus *bus.Bus
	// Handler is the shared inbound handler (built once by the daemon).
	Handler channel.InboundHandler
	// Get resolves a base env name ("AGEZT_TELEGRAM_TOKEN") to this
	// instance's value — the "#label" variant for a labelled account, the
	// plain env for the default instance. Factories read config ONLY through
	// Get; that is what retires the overlayEnv os.Setenv hack.
	Get func(baseEnv string) string
	// Label is this instance's account label ("" = default). For descs and
	// keys only — config access goes through Get.
	Label string
}

// Built is a factory's result. Desc == "" means "not configured — skip",
// which kills the typed-nil-interface landmine that forced a reflect probe
// on the legacy builders. Channels usually holds one element; a factory MAY
// return several from one env family (extras are keyed by their own Name).
type Built struct {
	Channels []channel.Channel
	Sink     pulse.BriefSink
	Desc     string
}

// NotConfigured is the zero Built, returned by a factory whose env is unset.
var NotConfigured = Built{}

// Factory builds the channel instance(s) for one (kind, label) pair.
type Factory func(Deps) Built

var (
	mu        sync.RWMutex
	factories = map[string]Factory{}
)

// Register installs the factory for a channel kind (matching its
// channel.Manifest Kind). Later registrations replace earlier ones, mirroring
// the manifest registry's semantics.
func Register(kind string, f Factory) {
	mu.Lock()
	defer mu.Unlock()
	factories[kind] = f
}

// Lookup returns the factory for kind.
func Lookup(kind string) (Factory, bool) {
	mu.RLock()
	defer mu.RUnlock()
	f, ok := factories[kind]
	return f, ok
}

// Instance is one built, keyed channel instance — the unit the daemon
// starts, registers for `agt send`, and reports on the boot banner.
type Instance struct {
	Kind    string
	Label   string // "" = default
	Key     string // channel.InstanceKey(Kind, Label)
	Desc    string
	Channel channel.Channel
	Sink    pulse.BriefSink // may be nil (outbound target unknown)
}

// Labels returns the configured NON-default account labels for a channel
// kind, discovered from the process env (the daemon injects store+vault
// keys, including "#label" suffixed ones, at boot).
func Labels(kind string) []string {
	baseEnvs := settings.SectionEnvs(kind)
	if len(baseEnvs) == 0 {
		return nil
	}
	env := os.Environ()
	keys := make([]string, 0, len(env))
	for _, kv := range env {
		if k, _, ok := strings.Cut(kv, "="); ok {
			keys = append(keys, k)
		}
	}
	return settings.AccountLabels(keys, baseEnvs)
}

// BuildKind builds every configured instance (default + each labelled
// account) of one kind through its registered factory. Returns nil when the
// kind has no factory or nothing is configured.
func BuildKind(ctx context.Context, kind string, b *bus.Bus, handler channel.InboundHandler) []Instance {
	f, ok := Lookup(kind)
	if !ok {
		return nil
	}
	var out []Instance
	for _, label := range append([]string{""}, Labels(kind)...) {
		built := f(Deps{
			Ctx:     ctx,
			Bus:     b,
			Handler: handler,
			Get:     settings.FieldGetter(label),
			Label:   label,
		})
		if built.Desc == "" || len(built.Channels) == 0 {
			continue
		}
		for i, ch := range built.Channels {
			key := channel.InstanceKey(kind, label)
			// A multi-channel Built (push family) keys extras by their own
			// Name() so each stays individually addressable.
			if i > 0 || (len(built.Channels) > 1 && ch.Name() != kind) {
				key = channel.InstanceKey(ch.Name(), label)
			}
			inst := Instance{
				Kind: kind, Label: label, Key: key,
				Desc: built.Desc, Channel: ch,
			}
			if i == 0 {
				inst.Sink = built.Sink // one sink per Built, not per channel
			}
			out = append(out, inst)
		}
	}
	return out
}

// The manifest-vs-factory drift alarm — "a channel shipped a manifest but no
// wiring" — is NOT here. A MissingFactories() helper lived at this spot until
// 2026-08-12 with no production caller; the check that actually runs is
// TestEveryManifestHasFactoryOrTODO in plugins/builtinchannels, which asserts
// it against the REAL manifest set rather than a synthetic fixture and also
// catches the inverse (a stale factoryTODO entry). One alarm, armed where the
// manifests are.
