// SPDX-License-Identifier: MIT

// Package mediaapi is the contract layer's home for the adapter seams the
// daemon injects for the agent's own media and relevance tools: speech
// (STT/TTS), image generation and reranking.
//
// These three seams used to live beside their tools — in kernel/voicetool,
// kernel/imagetool and kernel/reranktool — and the kernel named them there.
// That was the last thing the kernel reached for: the kernel BUILDS the tools
// that drive these seams, so it had to name the tool packages, and naming the
// tool packages meant naming the seams with them.
//
// The tools moved to the composition root (W3.44): the daemon builds them, the
// daemon binds their artifact sink, and the daemon hands them to the kernel
// as ordinary tools. What the kernel still needs is the SEAM itself — it
// answers "is speech configured?" on the daemon's behalf — and a seam is a
// row, so it lives here.
//
// The seams are structural on purpose: every method set uses only standard
// library types, so a provider plugin satisfies the seam without importing
// anything from the kernel. That is what lets the plugin stay a plugin.
//
// The rules of the road:
//
//   - Rows and interfaces only. No store, no file, no clock.
//   - Pure. It may import the standard library and other contract packages
//     only (archcheck's principle P4).
package mediaapi
