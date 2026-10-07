// SPDX-License-Identifier: MIT
package controlplane

import appconfigcenter "github.com/agezt/agezt/kernel/app/configcenter"

func (s *Server) configCenterReads() *appconfigcenter.Reads {
	center := s.k.ConfigCenter()
	if center == nil {
		return appconfigcenter.NewReads(nil)
	}
	return appconfigcenter.NewReads(center)
}
