// SPDX-License-Identifier: MIT

// Package providers owns transport-independent provider catalog and keyring operations.
// Socket/auth/audit adaptation remains in the control plane until operation binding.
package providers

import (
	"context"
	"errors"
	"fmt"
	"regexp"
	"strings"

	"github.com/agezt/agezt/internal/brand"
	"github.com/agezt/agezt/kernel/catalog"
	"github.com/agezt/agezt/kernel/creds"
	"github.com/agezt/agezt/kernel/runtime"
)

type ConnectInput struct{ ID, API, Model, Env, Name, NPM string }
type ReloadInput struct{}
type KeyInput struct {
	Provider, Env, Label, Value string
	Active                      bool
}
type Output = map[string]any
type Service struct {
	k       *runtime.Kernel
	baseDir string
}

func New(k *runtime.Kernel, baseDir string) *Service { return &Service{k: k, baseDir: baseDir} }

var providerEnvPattern = regexp.MustCompile(`^[A-Z0-9][A-Z0-9_]*$`)
var providerIDPattern = regexp.MustCompile(`^[A-Za-z0-9][A-Za-z0-9_.-]{0,127}$`)

func validEnv(value string) (string, bool) {
	env := strings.TrimSpace(value)
	if !providerEnvPattern.MatchString(env) || strings.HasPrefix(env, brand.EnvPrefix) {
		return "", false
	}
	return env, true
}
func keyTarget(in KeyInput) (provider, env, target string, ok bool) {
	env, ok = validEnv(in.Env)
	if !ok {
		return "", "", "", false
	}
	provider = strings.TrimSpace(in.Provider)
	if provider != "" {
		if !providerIDPattern.MatchString(provider) {
			return "", "", "", false
		}
		target = catalog.ProviderCredentialName(provider, env)
	} else {
		target = env
	}
	return provider, env, target, true
}
func keyTargetError() error {
	return errors.New("args.env must be a provider env var (UPPER_SNAKE, not AGEZT_*); args.provider, when set, must be a catalog provider id")
}
func (s *Service) loadVault() (*creds.Store, error) {
	vault := creds.NewStore(s.baseDir)
	if err := vault.Load(); err != nil {
		return nil, fmt.Errorf("load vault: %w", err)
	}
	return vault, nil
}

func (s *Service) Connect(_ context.Context, in ConnectInput) (Output, error) {
	sa := map[string]string{"id": in.ID, "api": in.API, "model": in.Model, "env": in.Env, "name": in.Name, "npm": in.NPM}
	id := strings.TrimSpace(sa["id"])
	api := strings.TrimSpace(sa["api"])
	// `model` is now informational — see the catalog-aware note above. The
	// UI/CLI uses it to push AGEZT_MODEL when the user asks for a default
	// brain; the backend no longer seeds it into the catalog Models map.
	_ = strings.TrimSpace(sa["model"])
	// env is OPTIONAL: a keyless local runtime (Ollama, LM Studio, …) connects
	// with no API key. When present it must be a valid provider env var.
	var envs []string
	if strings.TrimSpace(sa["env"]) != "" {
		env, ok := validEnv(in.Env)
		if !ok {
			return nil, errors.New("args.env must be a provider key env var (UPPER_SNAKE, not AGEZT_*)")
		}
		envs = []string{env}
	}
	if id == "" || api == "" {
		return nil, errors.New("args.id and args.api are required")
	}
	name := sa["name"]
	if strings.TrimSpace(name) == "" {
		name = id
	}
	npm := sa["npm"]
	if strings.TrimSpace(npm) == "" {
		npm = "@ai-sdk/openai-compatible"
	}

	// Catalog-aware gate: if the merged catalog already knows this id
	// (from models.dev sync, local.json discovery, or a prior custom.json
	// write), preserve the existing entry. No upsert, no clobber. The
	// caller attaches the key on the separate keys/add path.
	cat := s.k.Catalog()
	if _, exists := cat.Providers[id]; exists {
		_, providersReloaded, rerr := s.k.Reload()
		result := map[string]any{
			"provider_id":        id,
			"added":              false,
			"exists":             true,
			"providers_reloaded": providersReloaded,
			"note":               "id already in catalog; custom.json was NOT written — existing entry preserved. Attach the key via /api/provider/keys/add.",
		}
		if rerr != nil {
			result["reload_error"] = rerr.Error()
		}
		return result, nil
	}

	// New id: minimal partial entry. No synthetic model list — see the
	// type-level doc above for why.
	p := &catalog.Provider{
		ID:     id,
		Name:   strings.TrimSpace(name),
		NPM:    strings.TrimSpace(npm),
		API:    api,
		Env:    envs,
		Models: nil, // unknown coverage — see kernel/governor/modelchain_skip_test.go
	}
	added, err := s.k.CatalogStore().UpsertCustomProvider(p)
	if err != nil {
		return nil, fmt.Errorf("save custom provider: %w", err)
	}
	_, providersReloaded, rerr := s.k.Reload()
	result := map[string]any{
		"provider_id":        id,
		"added":              added,
		"exists":             false,
		"providers_reloaded": providersReloaded,
	}
	if rerr != nil {
		result["reload_error"] = rerr.Error()
	}
	return result, nil
}

func (s *Service) Reload(_ context.Context, _ ReloadInput) (Output, error) {
	cat, providersReloaded, err := s.k.Reload()
	if err != nil {
		return nil, err
	}
	result := map[string]any{
		"providers_reloaded": providersReloaded,
		"provider_count":     len(cat.Providers),
	}
	if !providersReloaded {
		// Surface the no-op clearly so operators don't wonder why a
		// creds change didn't take effect: when the daemon was built
		// without OnReload, only the catalog refresh ran.
		result["note"] = "OnReload not configured; only the catalog snapshot was refreshed. Restart the daemon for the new credentials to take effect."
	}
	return result, nil
}
func (s *Service) KeyList(_ context.Context, in KeyInput) (Output, error) {
	provider, env, target, ok := keyTarget(in)
	if !ok {
		return nil, keyTargetError()
	}
	vault, err := s.loadVault()
	if err != nil {
		return nil, err
	}
	return map[string]any{
		"provider": provider, "env": env, "keys": vault.KeyringList(target),
	}, nil
}

func (s *Service) KeyAdd(_ context.Context, in KeyInput) (Output, error) {
	provider, env, target, ok := keyTarget(in)
	if !ok {
		return nil, keyTargetError()
	}
	label := in.Label
	label = strings.TrimSpace(label)
	value, makeActive := in.Value, in.Active

	vault, err := s.loadVault()
	if err != nil {
		return nil, err
	}
	activeChanged, err := vault.KeyringAdd(target, label, value, makeActive)
	if err != nil {
		return nil, err
	}
	if err := vault.Save(); err != nil {
		return nil, fmt.Errorf("save vault: %w", err)
	}
	result := map[string]any{"provider": provider, "env": env, "label": label, "added": true, "active_changed": activeChanged}
	if activeChanged {
		if _, _, err := s.k.Reload(); err != nil {
			result["reload_error"] = err.Error()
		}
	}
	return result, nil
}

func (s *Service) KeyActivate(_ context.Context, in KeyInput) (Output, error) {
	provider, env, target, ok := keyTarget(in)
	if !ok {
		return nil, keyTargetError()
	}
	label := in.Label
	label = strings.TrimSpace(label)

	vault, err := s.loadVault()
	if err != nil {
		return nil, err
	}
	if err := vault.KeyringActivate(target, label); err != nil {
		return nil, err
	}
	if err := vault.Save(); err != nil {
		return nil, fmt.Errorf("save vault: %w", err)
	}
	result := map[string]any{"provider": provider, "env": env, "label": label, "active": true}
	if _, _, err := s.k.Reload(); err != nil {
		result["reload_error"] = err.Error()
	}
	return result, nil
}

func (s *Service) KeyRemove(_ context.Context, in KeyInput) (Output, error) {
	provider, env, target, ok := keyTarget(in)
	if !ok {
		return nil, keyTargetError()
	}
	label := in.Label
	label = strings.TrimSpace(label)

	vault, err := s.loadVault()
	if err != nil {
		return nil, err
	}
	removed, wasActive := vault.KeyringRemove(target, label)
	if err := vault.Save(); err != nil {
		return nil, fmt.Errorf("save vault: %w", err)
	}
	result := map[string]any{"provider": provider, "env": env, "label": label, "removed": removed, "was_active": wasActive}
	if wasActive {
		if _, _, err := s.k.Reload(); err != nil {
			result["reload_error"] = err.Error()
		}
	}
	return result, nil
}
