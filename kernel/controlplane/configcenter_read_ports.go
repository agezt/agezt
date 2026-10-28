// SPDX-License-Identifier: MIT
package controlplane

import appconfigcenter "github.com/agezt/agezt/kernel/app/configcenter"

func (s *Server) configCenterReads() *appconfigcenter.Reads {
	return appconfigcenter.NewReads(s.configCenterPorts().Reader)
}
