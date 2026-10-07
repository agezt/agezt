// SPDX-License-Identifier: MIT
package controlplane

import appchannels "github.com/agezt/agezt/kernel/app/channels"

// Inbox reads the selected kernel journal, independently of the Server root.
func (s *Server) channelInbox() *appchannels.Inbox {
	return appchannels.NewInbox(s.k.Journal())
}
