// SPDX-License-Identifier: MIT

package agentgw

import "testing"

// TestListenTargetPerTransportBranch pins the socket-path -> (network, address)
// mapping for every transport branch.
//
// The regression it guards is AC-007: the "unix://" case read
// `len(p) >= 7 && p[:6] == "unix://"`, comparing a 6-byte slice against a
// 7-byte literal, which is never equal in Go. The branch was dead, so every
// documented `unix://` path fell through to the TCP default and the gateway
// either failed to bind or, for a host:port-shaped value, came up as a
// cleartext TCP listener with no filesystem permissions gating it.
func TestListenTargetPerTransportBranch(t *testing.T) {
	tests := []struct {
		name        string
		sockPath    string
		wantNetwork string
		wantAddr    string
	}{
		{
			name:        "abstract namespace socket (the default)",
			sockPath:    "@agezt/agentgw.sock",
			wantNetwork: "unix",
			wantAddr:    "@agezt/agentgw.sock",
		},
		{
			// AC-007: this row reached ("tcp", "unix:///run/agezt/gw.sock")
			// before the fix.
			name:        "explicit unix:// prefix is stripped, not routed to TCP",
			sockPath:    "unix:///run/agezt/gw.sock",
			wantNetwork: "unix",
			wantAddr:    "/run/agezt/gw.sock",
		},
		{
			name:        "unix:// prefix on a relative path",
			sockPath:    "unix://gw.sock",
			wantNetwork: "unix",
			wantAddr:    "gw.sock",
		},
		{
			name:        "plain absolute path",
			sockPath:    "/run/agezt/gw.sock",
			wantNetwork: "unix",
			wantAddr:    "/run/agezt/gw.sock",
		},
		{
			name:        "host:port is TCP",
			sockPath:    "127.0.0.1:8790",
			wantNetwork: "tcp",
			wantAddr:    "127.0.0.1:8790",
		},
		{
			name:        "double slash is not treated as a unix path",
			sockPath:    "//host/share",
			wantNetwork: "tcp",
			wantAddr:    "//host/share",
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			network, addr := listenTarget(tc.sockPath)
			if network != tc.wantNetwork || addr != tc.wantAddr {
				t.Fatalf("listenTarget(%q) = (%q, %q), want (%q, %q)",
					tc.sockPath, network, addr, tc.wantNetwork, tc.wantAddr)
			}
		})
	}
}

// TestListenTargetNeverDowngradesUnixToTCP is the security-shaped assertion
// behind AC-007: no spelling of a unix socket may ever select the TCP
// transport, because TCP here is a cleartext listener with no peer check.
func TestListenTargetNeverDowngradesUnixToTCP(t *testing.T) {
	for _, sockPath := range []string{
		"unix:///run/agezt/gw.sock",
		"unix:///tmp/gw.sock",
		"unix://relative.sock",
		"@agezt/agentgw.sock",
		"/run/agezt/gw.sock",
	} {
		if network, addr := listenTarget(sockPath); network != "unix" {
			t.Errorf("sockPath %q selected %q transport (addr %q); a unix socket must never be served over cleartext TCP",
				sockPath, network, addr)
		}
	}
}
