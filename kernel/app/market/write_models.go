// SPDX-License-Identifier: MIT
package market

import core "github.com/agezt/agezt/kernel/market"

type InstallOutput = core.InstalledPack
type AddSourceOutput = core.Source
type UninstallOutput struct {
	Uninstalled string `json:"uninstalled"`
}
type RemoveSourceOutput struct {
	Removed bool   `json:"removed"`
	Name    string `json:"name"`
}
type SyncOutput struct {
	Results      []core.SyncResult `json:"results"`
	Synced       int               `json:"synced"`
	Packs        int               `json:"packs"`
	PartialError *string           `json:"partial_error,omitempty"`
}
