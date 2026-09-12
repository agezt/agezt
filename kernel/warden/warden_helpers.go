// SPDX-License-Identifier: MIT

// Warden helpers: actorOrDefault + downgradeReason.
// Code extracted from warden.go during the Day-72 god-file split. Public API unchanged.
package warden






func actorOrDefault(a string) string {
	if a == "" {
		return "warden"
	}
	return a
}

func downgradeReason(req Profile) string {
	switch req {
	case ProfileNamespace:
		return "linux full-namespace backend (CLONE_NEWUSER + cgroups + seccomp) not built; M1.d ships only setpgid + rlimit hardening on linux, full downgrade elsewhere"
	case ProfileContainer:
		return "OCI container backend is an M2+ optional plugin (SPEC-06 §2.2)"
	case ProfileMicroVM:
		return "microVM backend is an M2+ optional plugin (SPEC-06 §2.2)"
	}
	return "unknown profile"
}

type ctxKey int

const (
	ctxKeyCorrelation ctxKey = iota
	ctxKeyProfileOverride
)

// WithCorrelation returns a child context carrying corr so a tool that runs
// commands through the warden (e.g. the shell tool) can stamp it onto Spec.
// CorrelationID without threading the run id by hand. The runtime sets this on
// every run's context; the resulting warden.executed / warden.profile_downgraded
// events then carry the run correlation, so they appear in the run-detail
// timeline and `agt why <id>` walks back through them. Without it the warden
// events still record (just unlinked from a run).