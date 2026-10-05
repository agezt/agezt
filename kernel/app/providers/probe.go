// SPDX-License-Identifier: MIT

package providers

import (
	"encoding/json"
	"errors"
	"strings"

	"github.com/agezt/agezt/kernel/platform/netout"
)

type ProbeInput struct {
	URL string `json:"url,omitempty"`
	Key string `json:"key,omitempty"`
}

// EndpointGET is the bounded endpoint transport selected by the host.
type EndpointGET func(string, string, string, int64) ([]byte, int, string, error)
type Probe struct{ get EndpointGET }

func NewProbe(get EndpointGET) *Probe {
	if get == nil {
		get = netout.GatewayGET
	}
	return &Probe{get: get}
}
func (s *Probe) Check(in ProbeInput) (ProbeOutput, error) {
	base := strings.TrimRight(strings.TrimSpace(in.URL), "/")
	if base == "" {
		return ProbeOutput{}, errors.New("args.url is required")
	}
	// OpenAI-compatible servers list models at <base>/models (base usually ends /v1).
	modelsURL := base + "/models"
	key := strings.TrimSpace(in.Key)
	body, code, _, err := s.get(modelsURL, "Authorization", bearer(key), 1<<20)
	if err != nil {
		return ProbeOutput{OK: false, Error: "cannot reach endpoint: " + err.Error()}, nil
	}
	// 2xx = reachable + authorized. 401/403 = reachable but needs/!valid key.
	reachable := code/100 == 2 || code == 401 || code == 403
	count := 0
	if code/100 == 2 {
		var parsed struct {
			Data []json.RawMessage `json:"data"`
		}
		_ = json.Unmarshal(body, &parsed)
		count = len(parsed.Data)
	}
	authorized := code/100 == 2
	return ProbeOutput{OK: true, Reachable: &reachable, Authorized: &authorized, HTTPStatus: &code, Models: &count}, nil
}

func bearer(key string) string {
	if key == "" {
		return ""
	}
	return "Bearer " + key
}
