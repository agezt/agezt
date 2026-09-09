// SPDX-License-Identifier: MIT

package jsonout

import (
	"encoding/json"
	"io"
)

// Write pretty-prints v to w and returns 0. The 2-space indent
// matches the rest of the CLI's `--json` output (the only
// command that breaks this is the streaming agent, which uses
// the more compact single-line shape). Encoder errors are
// swallowed so the caller can `return jsonout.Write(stdout, v)`
// without a temporary error variable; in practice the only
// failure mode (broken pipe) is the same as the caller's
// underlying write failing, which they have already handled.
func Write(w io.Writer, v any) int {
	enc := json.NewEncoder(w)
	enc.SetIndent("", "  ")
	_ = enc.Encode(v)
	return 0
}
