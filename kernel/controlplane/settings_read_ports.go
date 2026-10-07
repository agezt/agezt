// SPDX-License-Identifier: MIT
package controlplane

import (
	appsettings "github.com/agezt/agezt/kernel/app/settings"
	"github.com/agezt/agezt/kernel/creds"
	"github.com/agezt/agezt/kernel/settings"
	"os"
)

type nativeSettingsReader struct {
	baseDir string
	pinned  map[string]bool
}

func (r nativeSettingsReader) SchemaSections() []appsettings.Section {
	return settings.NewRegistry(r.baseDir).Sections()
}
func (r nativeSettingsReader) PrepareValues() appsettings.ValuesReader {
	store := settings.NewStore(r.baseDir)
	_ = store.Load()
	vault := creds.NewStore(r.baseDir)
	_ = vault.Load()
	reg := settings.NewRegistry(r.baseDir)
	return nativeSettingsValues{registry: reg, store: store, vault: vault, pinned: r.pinned}
}

type nativeSettingsValues struct {
	registry *settings.Registry
	store    *settings.Store
	vault    *creds.Store
	pinned   map[string]bool
}

func (v nativeSettingsValues) Sections() []appsettings.Section        { return v.registry.Sections() }
func (v nativeSettingsValues) EnvPinned(name string) bool             { return v.pinned[name] }
func (v nativeSettingsValues) SecretSet(name string) bool             { return v.vault.Has(name) }
func (v nativeSettingsValues) EnvValue(name string) string            { return os.Getenv(name) }
func (v nativeSettingsValues) StoredValue(name string) (string, bool) { return v.store.Get(name) }
func (s *Server) settingsReads() *appsettings.Reads {
	return appsettings.NewReads(nativeSettingsReader{baseDir: s.baseDir, pinned: s.configEnvPinned})
}
