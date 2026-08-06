// SPDX-License-Identifier: MIT

package runtime

// Aliases for the daemon-injected tool adapter seams whose tools live in their
// own packages (Phase 3.1 Stage D). Config fields and cmd/agezt name these as
// runtime.<Seam>; the aliases keep that exported surface stable while the tool
// implementations live beside their seams.

import (
	"github.com/agezt/agezt/kernel/imagetool"
	"github.com/agezt/agezt/kernel/reranktool"
	"github.com/agezt/agezt/kernel/voicetool"
)

// Reranker is the rerank adapter seam. See kernel/reranktool.
type Reranker = reranktool.Reranker

// ImageGen is the image-generation adapter seam. See kernel/imagetool.
type ImageGen = imagetool.ImageGen

// Voice is the STT/TTS adapter seam. See kernel/voicetool.
type Voice = voicetool.Voice
