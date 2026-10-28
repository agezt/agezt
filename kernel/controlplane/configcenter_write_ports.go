// SPDX-License-Identifier: MIT
package controlplane

import appconfigcenter "github.com/agezt/agezt/kernel/app/configcenter"

func (s *Server) configCenterWrites() *appconfigcenter.Writes {
	return appconfigcenter.NewWrites(s.configCenterPorts().Writer)
}
