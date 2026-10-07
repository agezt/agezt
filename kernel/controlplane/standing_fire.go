// SPDX-License-Identifier: MIT
package controlplane

// SetStandingFire supplies the daemon's resident standing-runner callback.
// Typed operations resolve it at dispatch time, so later injection is visible.
func (s *Server) SetStandingFire(fn func(id string) bool) { s.standingFire = fn }
