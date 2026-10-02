// SPDX-License-Identifier: MIT

package mcp

// Provenance: Package mcp: env helper for the MCP client (appendEnv merges a
//             key/value map into a string slice). The child's base environment is
//             sandbox.IsolatedEnv — this package used to keep its own copy of the
//             scrub allowlist, which had drifted (it lacked USERNAME/HOMEDRIVE/
//             HOMEPATH).

func appendEnv(base []string, extra map[string]string) []string {
	if len(extra) == 0 {
		return base
	}
	out := append([]string(nil), base...)
	for k, v := range extra {
		out = append(out, k+"="+v)
	}
	return out
}
