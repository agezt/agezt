// SPDX-License-Identifier: MIT

package mediaapi

import "context"

// Voice is the seam the `voice` tool drives: speech in, speech out.
//
// It is implemented by the daemon's voice adapter; either half may be
// unconfigured, which the adapter reports through HasSTT/HasTTS.
//
// The tool that drives it lives in kernel/voicetool, which keeps this name as
// an alias — so every existing importer keeps compiling unchanged.
type Voice interface {
	// Transcribe turns audio bytes into text (speech-to-text).
	Transcribe(ctx context.Context, audio []byte, filename string) (string, error)
	// Speak turns text into audio bytes + their MIME type (text-to-speech).
	Speak(ctx context.Context, text string) (audio []byte, mime string, err error)
	// HasSTT / HasTTS report which halves are configured.
	HasSTT() bool
	HasTTS() bool
}

// ImageGen is the seam the `image_generate` tool drives.
//
// Its method set uses only stdlib types so a provider plugin satisfies it
// structurally without importing the kernel.
//
// The tool that drives it lives in kernel/imagetool, which keeps this name as
// an alias.
type ImageGen interface {
	// GenerateImage returns the raw bytes of each generated image plus their
	// shared MIME type. size/quality are passed through when non-empty; n<=0
	// means one image.
	GenerateImage(ctx context.Context, prompt, size, quality string, n int) ([][]byte, string, error)
	// HasImage reports whether image generation is configured.
	HasImage() bool
}

// Reranker is the seam the `rerank` tool drives.
//
// Its method set uses only stdlib types so a provider plugin satisfies it
// structurally without importing the kernel.
//
// The tool that drives it lives in kernel/reranktool, which keeps this name
// as an alias.
type Reranker interface {
	// Rerank returns, in descending relevance order, the original index of each
	// document and its relevance score (parallel slices). topN > 0 caps the
	// number returned.
	Rerank(ctx context.Context, query string, documents []string, topN int) ([]int, []float64, error)
	// HasRerank reports whether reranking is configured.
	HasRerank() bool
}
