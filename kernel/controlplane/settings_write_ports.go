// SPDX-License-Identifier: MIT
package controlplane

import (
	appsettings "github.com/agezt/agezt/kernel/app/settings"
	"github.com/agezt/agezt/kernel/creds"
	"github.com/agezt/agezt/kernel/settings"
)

type nativeSettingsWriter struct{ server *Server }

func (w nativeSettingsWriter) FieldByEnv(name string) (settings.Field, bool) {
	return settings.NewRegistry(w.server.baseDir).FieldByEnv(name)
}

type nativeSettingsValueStore struct {
	load   func() error
	set    func(string, string)
	remove func(string)
	save   func() error
}

func (s nativeSettingsValueStore) Load() error            { return s.load() }
func (s nativeSettingsValueStore) Set(name, value string) { s.set(name, value) }
func (s nativeSettingsValueStore) Remove(name string)     { s.remove(name) }
func (s nativeSettingsValueStore) Save() error            { return s.save() }
func (w nativeSettingsWriter) ConfigStore() appsettings.ValueStore {
	s := settings.NewStore(w.server.baseDir)
	return nativeSettingsValueStore{load: s.Load, set: s.Set, remove: func(name string) { s.Remove(name) }, save: s.Save}
}
func (w nativeSettingsWriter) VaultStore() appsettings.ValueStore {
	s := creds.NewStore(w.server.baseDir)
	return nativeSettingsValueStore{load: s.Load, set: func(name, value string) { _ = s.Set(name, value) }, remove: func(name string) { s.Remove(name) }, save: s.Save}
}
func (w nativeSettingsWriter) EnvPinned(name string) bool    { return w.server.configEnvPinned[name] }
func (w nativeSettingsWriter) SetLiveEnv(name, value string) { setLiveEnv(name, value) }
func (w nativeSettingsWriter) Reload() error                 { _, _, err := w.server.k.Reload(); return err }
func (w nativeSettingsWriter) Register(sec appsettings.Section) error {
	return settings.NewRegistry(w.server.baseDir).Register(sec)
}
func (w nativeSettingsWriter) Unregister(id string, force bool) (bool, error) {
	return settings.NewRegistry(w.server.baseDir).Unregister(id, force)
}
func (s *Server) settingsWrites() *appsettings.Writes {
	return appsettings.NewWrites(nativeSettingsWriter{server: s})
}
