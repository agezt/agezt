// SPDX-License-Identifier: MIT
package controlplane

import appconfigcenter "github.com/agezt/agezt/kernel/app/configcenter"

func (s *Server) configCenterWrites() *appconfigcenter.Writes {
	center := s.k.ConfigCenter()
	if center == nil {
		return appconfigcenter.NewWrites(nil)
	}
	return appconfigcenter.NewWrites(center)
}
