// SPDX-License-Identifier: MIT

package executionprofile

import "strings"

// The remote execution backends: the execution profiles whose work runs on
// another host. The ids live here, in the package that owns the profile
// inventory, so that every caller that asks "is this a remote backend?" asks
// the same list — the application's run path resolves them, the control plane
// refuses them for a dispatched seat, and the code_exec tool exports their
// artifacts from the local root.
const (
	IDSSH         = "ssh"
	IDK8s         = "k8s"
	IDModal       = "modal"
	IDDaytona     = "daytona"
	IDRemoteAgezt = "remote-agezt"
)

// RemoteBackendIDs lists the remote execution profile ids, in the inventory's
// order.
var RemoteBackendIDs = []string{IDSSH, IDK8s, IDModal, IDDaytona, IDRemoteAgezt}

// IsRemoteBackend reports whether an execution profile id names a remote
// backend — ssh, k8s, modal, daytona or remote-agezt. The comparison ignores
// the surrounding spaces and the case, because the callers that pass a
// user-supplied profile id do not promise either.
func IsRemoteBackend(id string) bool {
	switch strings.ToLower(strings.TrimSpace(id)) {
	case IDSSH, IDK8s, IDModal, IDDaytona, IDRemoteAgezt:
		return true
	}
	return false
}
