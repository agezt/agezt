// SPDX-License-Identifier: MIT

// Package agent: content-addressed artifact offload for large tool outputs
// (ArtifactPutter interface + offloadToolOutput helper). Extracted from
// agent_context.go during the Day-211 god-file split. Public API unchanged.
package agent

// ArtifactPutter is the slice of a content-addressed store the loop needs to
// offload large outputs. kernel/artifact.Store satisfies it. An interface keeps
// kernel/agent decoupled from the storage package.
type ArtifactPutter interface {
	Put(data []byte) (ref string, err error)
}

// DefaultArtifactThreshold is the tool-output size above which the journal event
// offloads to the artifact store (the model still sees the full output). 8 KiB
// keeps ordinary results inline while bounding the event for big dumps.
const DefaultArtifactThreshold = 8 << 10

// artifactPreviewBytes is how much of an offloaded output stays inline on the
// event as a human-readable preview.
const artifactPreviewBytes = 512

// offloadToolOutput decides how a tool output is represented ON THE EVENT. When a
// store is configured and the output exceeds the threshold, it stores the full
// bytes and returns a preview + ref + true; otherwise (no store, small output, or
// a Put error) it returns the output unchanged and offloaded=false. It never
// returns an error — offload is best-effort and must not fail the run.
func offloadToolOutput(store ArtifactPutter, threshold int, output string) (eventOutput, rawRef string, fullBytes int, offloaded bool) {
	fullBytes = len(output)
	if store == nil {
		return output, "", fullBytes, false
	}
	if threshold <= 0 {
		threshold = DefaultArtifactThreshold
	}
	if fullBytes <= threshold {
		return output, "", fullBytes, false
	}
	ref, err := store.Put([]byte(output))
	if err != nil || ref == "" {
		return output, "", fullBytes, false // fall back to inlining
	}
	preview := output
	if len(preview) > artifactPreviewBytes {
		preview = preview[:artifactPreviewBytes] + "…[offloaded; full output in artifact " + ref + "]"
	}
	return preview, ref, fullBytes, true
}
