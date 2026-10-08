// SPDX-License-Identifier: MIT
package controlplane

import (
	appchannels "github.com/agezt/agezt/kernel/app/channels"
	"strings"
)

// Resolve the currently selected server sender for each request.
func (s *Server) channelOutbound() *appchannels.Outbound {
	return appchannels.NewOutbound(appchannels.Sender(s.channelSend))
}

// stringArg reads a string argument from a request arg map, "" when absent or not
// a string.
func stringArg(args map[string]any, key string) string {
	if v, ok := args[key].(string); ok {
		return strings.TrimSpace(v)
	}
	return ""
}
