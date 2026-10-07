// SPDX-License-Identifier: MIT
package controlplane

import (
	appchannels "github.com/agezt/agezt/kernel/app/channels"
	"github.com/agezt/agezt/kernel/channel"
	"github.com/agezt/agezt/kernel/creds"
	"github.com/agezt/agezt/kernel/settings"
)

type nativeChannelInventoryReader struct{ nativeSettingsReader }
type nativeChannelInventoryValues struct{ nativeSettingsValues }

func (r nativeChannelInventoryReader) Prepare() appchannels.InventoryValues {
	// Retain store then vault then registry preparation and ignored read-load errors.
	store := settings.NewStore(r.baseDir)
	_ = store.Load()
	vault := creds.NewStore(r.baseDir)
	_ = vault.Load()
	reg := settings.NewRegistry(r.baseDir)
	return nativeChannelInventoryValues{nativeSettingsValues{registry: reg, store: store, vault: vault, pinned: r.pinned}}
}
func (nativeChannelInventoryReader) Manifests() []channel.Manifest { return channel.Manifests() }
func (nativeChannelInventoryReader) IsLive(kind string) bool       { return channel.IsLive(kind) }
func (nativeChannelInventoryReader) IsLiveInstance(key string) bool {
	return channel.IsLiveInstance(key)
}
func (v nativeChannelInventoryValues) Names() []string {
	return append(v.store.Names(), v.vault.Names()...)
}
func (s *Server) channelInventory() *appchannels.Inventory {
	return appchannels.NewInventory(nativeChannelInventoryReader{nativeSettingsReader{baseDir: s.baseDir, pinned: s.configEnvPinned}})
}
