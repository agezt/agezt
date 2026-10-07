// SPDX-License-Identifier: MIT
package controlplane

import (
	appchannels "github.com/agezt/agezt/kernel/app/channels"
	"github.com/agezt/agezt/kernel/channel"
	"github.com/agezt/agezt/kernel/creds"
	"github.com/agezt/agezt/kernel/settings"
)

type nativeChannelAccountWriter struct{ baseDir string }
type nativeChannelVault struct{ *creds.Store }

func (v nativeChannelVault) Set(name, value string) { _ = v.Store.Set(name, value) }
func (nativeChannelAccountWriter) LookupManifest(kind string) (channel.Manifest, bool) {
	return channel.LookupManifest(kind)
}
func (w nativeChannelAccountWriter) Sections() []settings.Section {
	return settings.NewRegistry(w.baseDir).Sections()
}
func (w nativeChannelAccountWriter) SectionEnvs(section string) []string {
	// Use the same merged, filtered server-root schema as account Set.
	for _, candidate := range settings.NewRegistry(w.baseDir).Sections() {
		if candidate.ID != section {
			continue
		}
		envs := make([]string, 0, len(candidate.Fields))
		for _, field := range candidate.Fields {
			envs = append(envs, field.Env)
		}
		return envs
	}
	return nil
}
func (w nativeChannelAccountWriter) ConfigStore() appchannels.AccountStore {
	return settings.NewStore(w.baseDir)
}
func (w nativeChannelAccountWriter) VaultStore() appchannels.AccountStore {
	return nativeChannelVault{creds.NewStore(w.baseDir)}
}
func (s *Server) channelAccounts() *appchannels.Accounts {
	return appchannels.NewAccounts(nativeChannelAccountWriter{baseDir: s.baseDir})
}
