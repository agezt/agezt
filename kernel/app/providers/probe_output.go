// SPDX-License-Identifier: MIT

package providers

type ProbeOutput struct {
	OK         bool   `json:"ok"`
	Error      string `json:"error,omitempty"`
	Reachable  *bool  `json:"reachable,omitempty"`
	Authorized *bool  `json:"authorized,omitempty"`
	HTTPStatus *int   `json:"http_status,omitempty"`
	Models     *int   `json:"models,omitempty"`
}
