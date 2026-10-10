// SPDX-License-Identifier: MIT

package controlplane

import (
	"net/http"
	"os"
	"strings"

	"github.com/agezt/agezt/internal/brand"
	"github.com/agezt/agezt/kernel/creds"
	"github.com/agezt/agezt/kernel/platform/netout"
	"github.com/agezt/agezt/kernel/settings"
)

// nodePeerSpec reads AGEZT_PEERS from the environment, then the vault, then
// the settings store, for the node registry and the remote run mirror.
func (s *Server) nodePeerSpec() string {
	if v := os.Getenv(brand.EnvPrefix + "PEERS"); strings.TrimSpace(v) != "" {
		return v
	}
	vault := creds.NewStore(s.baseDir)
	if vault.Load() == nil {
		if v := vault.Get(brand.EnvPrefix + "PEERS"); strings.TrimSpace(v) != "" {
			return v
		}
	}
	store := settings.NewStore(s.baseDir)
	if store.Load() == nil {
		if v, _ := store.Get(brand.EnvPrefix + "PEERS"); strings.TrimSpace(v) != "" {
			return v
		}
	}
	return ""
}

// nodeProbeClient is the operator client the registry probes peers with:
// loopback and private addresses are allowed.
func nodeProbeClient() *http.Client { return netout.OperatorClient(0) }
