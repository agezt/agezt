// SPDX-License-Identifier: MIT

package agent

import "github.com/agezt/agezt/kernel/platform/tooloutput"

// ArtifactPutter retains the loop's public artifact-store contract.
type ArtifactPutter = tooloutput.ArtifactPutter

// DefaultArtifactThreshold retains the loop's existing 8 KiB default.
const DefaultArtifactThreshold = tooloutput.DefaultArtifactThreshold

// offloadToolOutput forwards to the shared representation without changing the
// loop's preview, threshold, full-output delivery or best-effort fallback.
func offloadToolOutput(store ArtifactPutter, threshold int, output string) (eventOutput, rawRef string, fullBytes int, offloaded bool) {
	return tooloutput.Offload(store, threshold, output)
}
