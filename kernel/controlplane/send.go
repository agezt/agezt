// SPDX-License-Identifier: MIT
package controlplane

import (
	appchannels "github.com/agezt/agezt/kernel/app/channels"
)

// Resolve the currently selected server sender for each request.
func (s *Server) channelOutbound() *appchannels.Outbound {
	return appchannels.NewOutbound(appchannels.Sender(s.channelSend))
}
