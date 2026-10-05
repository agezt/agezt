// SPDX-License-Identifier: MIT

package providers

import "github.com/agezt/agezt/kernel/creds"

type ConnectOutput struct {
	ProviderID        string `json:"provider_id"`
	Added             bool   `json:"added"`
	Exists            bool   `json:"exists"`
	ProvidersReloaded bool   `json:"providers_reloaded"`
	Note              string `json:"note,omitempty"`
	ReloadError       string `json:"reload_error,omitempty"`
}
type ReloadOutput struct {
	ProvidersReloaded bool   `json:"providers_reloaded"`
	ProviderCount     int    `json:"provider_count"`
	Note              string `json:"note,omitempty"`
}
type KeyListOutput struct {
	Provider string          `json:"provider"`
	Env      string          `json:"env"`
	Keys     []creds.KeyInfo `json:"keys"`
}
type KeyAddOutput struct {
	Provider      string `json:"provider"`
	Env           string `json:"env"`
	Label         string `json:"label"`
	Added         bool   `json:"added"`
	ActiveChanged bool   `json:"active_changed"`
	ReloadError   string `json:"reload_error,omitempty"`
}
type KeyActivateOutput struct {
	Provider    string `json:"provider"`
	Env         string `json:"env"`
	Label       string `json:"label"`
	Active      bool   `json:"active"`
	ReloadError string `json:"reload_error,omitempty"`
}
type KeyRemoveOutput struct {
	Provider    string `json:"provider"`
	Env         string `json:"env"`
	Label       string `json:"label"`
	Removed     bool   `json:"removed"`
	WasActive   bool   `json:"was_active"`
	ReloadError string `json:"reload_error,omitempty"`
}
