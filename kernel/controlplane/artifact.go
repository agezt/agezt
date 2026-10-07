// SPDX-License-Identifier: MIT

package controlplane

import (
	appartifacts "github.com/agezt/agezt/kernel/app/artifacts"
	"time"
)

func (s *Server) artifactService() *appartifacts.Service {
	var store appartifacts.BlobStore
	var index appartifacts.Index
	if actual := s.k.Artifacts(); actual != nil {
		store = actual
	}
	if actual := s.k.ArtifactIndex(); actual != nil {
		index = actual
	}
	return appartifacts.New(store, index, time.Now)
}
