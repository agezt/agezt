// SPDX-License-Identifier: MIT

package runtime

// Aliases for the daemon-injected tool adapter seams whose tools live in their
// own packages (Phase 3.1 Stage D). Config fields and cmd/agezt name these as
// runtime.<Seam>; the aliases keep that exported surface stable while the tool
// implementations live beside their seams.
//
// The seams themselves live in the CONTRACT layer (kernel/contract/mediaapi —
// W3.44). The kernel names the contract's row, so it reaches no tool package:
// the daemon builds the voice, image and rerank tools and hands them over as
// ordinary tools (W3.44).

import (
	"github.com/agezt/agezt/kernel/contract/mediaapi"
)

// Reranker is the rerank adapter seam. See kernel/contract/mediaapi.
type Reranker = mediaapi.Reranker

// ImageGen is the image-generation adapter seam. See kernel/contract/mediaapi.
type ImageGen = mediaapi.ImageGen

// Voice is the STT/TTS adapter seam. See kernel/contract/mediaapi.
type Voice = mediaapi.Voice
