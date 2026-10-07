// SPDX-License-Identifier: MIT
package channels

import "github.com/agezt/agezt/kernel/channel"

type ListOutput struct {
	Channels    []ChannelRow `json:"channels"`
	Count       int          `json:"count"`
	ProbeMatrix ProbeMatrix  `json:"probe_matrix"`
	MediaMatrix MediaMatrix  `json:"media_matrix"`
}
type ChannelField struct {
	Env       string `json:"env"`
	Label     string `json:"label"`
	Secret    bool   `json:"secret"`
	Required  bool   `json:"required"`
	Help      string `json:"help"`
	Set       bool   `json:"set"`
	EnvPinned bool   `json:"env_pinned"`
	// nil omits secret values; a non-nil empty string retains public emptiness.
	Value *string `json:"value,omitempty"`
}
type ChannelAccount struct {
	Label      string         `json:"label"`
	Configured bool           `json:"configured"`
	Live       bool           `json:"live"`
	Probe      AccountProbe   `json:"probe"`
	Fields     []ChannelField `json:"fields"`
}
type ChannelRow struct {
	Kind          string            `json:"kind"`
	Display       string            `json:"display"`
	Description   string            `json:"description"`
	Transport     string            `json:"transport"`
	Duplex        bool              `json:"duplex"`
	Media         channel.MediaCaps `json:"media"`
	SetupSteps    []string          `json:"setup_steps"`
	ConnectMethod string            `json:"connect_method"`
	ConfigSection string            `json:"config_section"`
	DocsURL       string            `json:"docs_url"`
	Configured    bool              `json:"configured"`
	Live          bool              `json:"live"`
	Probe         ChannelProbe      `json:"probe"`
	Fields        []ChannelField    `json:"fields"`
	Accounts      []ChannelAccount  `json:"accounts"`
}
type AccountProbe struct {
	Configured      bool   `json:"configured"`
	Live            bool   `json:"live"`
	RoundtripStatus string `json:"roundtrip_status"`
	RoundtripReady  bool   `json:"roundtrip_ready"`
	Mode            string `json:"mode"`
	Note            string `json:"note"`
}
type ChannelProbe struct {
	Accounts           int    `json:"accounts"`
	ConfiguredAccounts int    `json:"configured_accounts"`
	LiveAccounts       int    `json:"live_accounts"`
	RoundtripStatus    string `json:"roundtrip_status"`
	RoundtripReady     bool   `json:"roundtrip_ready"`
	Mode               string `json:"mode"`
}
type ProbeMatrix struct {
	Total          int `json:"total"`
	Configured     int `json:"configured"`
	Live           int `json:"live"`
	RoundtripReady int `json:"roundtrip_ready"`
	RestartNeeded  int `json:"restart_needed"`
	NeedsSetup     int `json:"needs_setup"`
}
type MediaMatrix struct {
	ImageIn  int `json:"image_in"`
	ImageOut int `json:"image_out"`
	VoiceIn  int `json:"voice_in"`
	VoiceOut int `json:"voice_out"`
}
