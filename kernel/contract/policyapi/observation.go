// SPDX-License-Identifier: MIT

package policyapi

// UntrustedObservationTaint is carried from the tool-output boundary to the
// next policy decision. It lets policy see that a proposed action is downstream
// of external data without asking the LLM to self-report that dependency.
type UntrustedObservationTaint struct {
	Sources       []string
	DirectiveLike bool
	Matches       []string
}
