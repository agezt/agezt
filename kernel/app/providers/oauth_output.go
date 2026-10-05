// SPDX-License-Identifier: MIT

package providers

type OAuthStartOutput struct {
	AuthorizeURL string `json:"authorize_url"`
	State        string `json:"state"`
}
type OAuthStatusOutput struct {
	Status       string   `json:"status"`
	Error        string   `json:"error"`
	Connected    bool     `json:"connected"`
	Email        string   `json:"email"`
	Account      string   `json:"account"`
	Models       []string `json:"models"`
	DefaultModel string   `json:"default_model"`
}
type OAuthImportOutput struct {
	OK           bool     `json:"ok"`
	Connected    bool     `json:"connected"`
	Email        string   `json:"email"`
	Account      string   `json:"account"`
	Models       []string `json:"models"`
	DefaultModel string   `json:"default_model"`
}
type OAuthLogoutOutput struct {
	OK        bool `json:"ok"`
	Connected bool `json:"connected"`
}
