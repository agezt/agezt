// SPDX-License-Identifier: MIT
package controlplane

import (
	appchannels "github.com/agezt/agezt/kernel/app/channels"
	"github.com/agezt/agezt/kernel/platform/netout"
)

// Keep the existing bounded LAN-capable selected HTTP port and its timeout policy.
func (s *Server) channelGateway() *appchannels.Gateway {
	return appchannels.NewGateway(netout.GatewayGET)
}
