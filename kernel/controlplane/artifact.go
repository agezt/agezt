// SPDX-License-Identifier: MIT

package controlplane

import (
	appartifacts "github.com/agezt/agezt/kernel/app/artifacts"
	"time"
)

func (s *Server) artifactService() *appartifacts.Service {
	ports := s.artifactPorts()
	return appartifacts.New(ports.Store(), ports.Index(), time.Now)
}
