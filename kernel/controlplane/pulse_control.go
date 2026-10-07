// SPDX-License-Identifier: MIT
package controlplane

// SetPulse wires the live resident engine before Start. Nil reports disabled.
func (s *Server) SetPulse(p PulseController) { s.pulse = p }
