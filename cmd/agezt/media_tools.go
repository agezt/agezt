// SPDX-License-Identifier: MIT

package main

import (
	"maps"

	"github.com/agezt/agezt/kernel/contract/toolapi"
	"github.com/agezt/agezt/kernel/imagetool"
	"github.com/agezt/agezt/kernel/reranktool"
	kernelruntime "github.com/agezt/agezt/kernel/runtime"
	"github.com/agezt/agezt/kernel/voicetool"
)

// mediaTools builds the voice, image and rerank tools over the daemon's own
// adapter seams and returns them as ordinary tools.
//
// The kernel used to build these three itself: it reached into
// kernel/voicetool, kernel/imagetool and kernel/reranktool to do it, and that
// reach was the kernel's last word on those packages (W3.44). The kernel is a
// module and may not name a module's implementation — so the tools are the
// DAEMON's now: the daemon owns the adapters, the daemon owns the artifact
// store the two of them persist into, and the daemon hands them to the kernel
// as tools like any other.
//
// The two that persist bytes bind the daemon's OWN artifact store — the same
// one the kernel holds — so a synthesized reply and a generated image land in
// the same place they always did.
//
// The tool map is COPIED, never mutated: the primary kernel and every tenant
// share one map, and each kernel must see ITS OWN tools bound to ITS OWN
// store. A map written in place would hand the last kernel's tools to all of
// them.
func mediaTools(tools map[string]toolapi.Tool, cfg kernelruntime.Config) map[string]toolapi.Tool {
	out := make(map[string]toolapi.Tool, len(tools)+3)
	maps.Copy(out, tools)
	if cfg.Voice != nil {
		vt := voicetool.New(cfg.Voice)
		if cfg.Artifacts != nil {
			vt.SaveArtifact = cfg.Artifacts.Put
		}
		out["voice"] = vt
	}
	if cfg.ImageGenerator != nil {
		it := imagetool.New(cfg.ImageGenerator)
		if cfg.Artifacts != nil {
			it.SaveArtifact = cfg.Artifacts.Put
		}
		out["image_generate"] = it
	}
	if cfg.Reranker != nil {
		out["rerank"] = reranktool.New(cfg.Reranker)
	}
	return out
}
