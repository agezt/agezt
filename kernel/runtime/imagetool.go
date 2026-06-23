// SPDX-License-Identifier: MIT

package runtime

// Agent-facing image tool (M997): lets a running agent GENERATE images from a
// text prompt. It drives the daemon-injected image adapter (runtime.Config.
// ImageGenerator — typically the OpenAI-compatible image plugin). Generated
// images are saved as artifacts (never returned inline to the model), so a
// channel or the operator can view them — mirroring the voice tool's speak path.
// The kernel never imports the plugin — only this interface.

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"

	"github.com/agezt/agezt/kernel/agent"
)

// ImageGen is the seam the `image_generate` tool drives. Its method set uses
// only stdlib types so a provider plugin satisfies it structurally without
// importing the kernel.
type ImageGen interface {
	// GenerateImage returns the raw bytes of each generated image plus their
	// shared MIME type. size/quality are passed through when non-empty; n<=0
	// means one image.
	GenerateImage(ctx context.Context, prompt, size, quality string, n int) ([][]byte, string, error)
	// HasImage reports whether image generation is configured.
	HasImage() bool
}

// imageTool implements agent.Tool over an ImageGen adapter. saveArtifact is
// bound to the kernel's artifact store after Open.
type imageTool struct {
	gen          ImageGen
	saveArtifact func(data []byte) (string, error)
}

func newImageTool(g ImageGen) *imageTool { return &imageTool{gen: g} }

func (t *imageTool) Definition() agent.ToolDef {
	return agent.ToolDef{
		Name: "image_generate",
		Description: "Generate one or more images from a text prompt. The images are saved as artifacts you can attach " +
			"to a message — they are not returned inline. Use a vivid, specific prompt. Optional: size (e.g. 1024x1024), " +
			"quality (standard|hd), n (number of images, default 1).",
		InputSchema: json.RawMessage(`{
  "type": "object",
  "properties": {
    "prompt": {"type": "string", "description": "What to depict — be specific about subject, style, composition"},
    "size": {"type": "string", "description": "Optional pixel size, e.g. 1024x1024 or 1792x1024"},
    "quality": {"type": "string", "description": "Optional quality hint, e.g. standard or hd"},
    "n": {"type": "integer", "description": "How many images to generate (default 1)"}
  },
  "required": ["prompt"]
}`),
		Effect: agent.ToolEffect{Class: agent.EffectReadOnly},
	}
}

type imageToolInput struct {
	Prompt  string `json:"prompt"`
	Size    string `json:"size"`
	Quality string `json:"quality"`
	N       int    `json:"n"`
}

func (t *imageTool) Invoke(ctx context.Context, input json.RawMessage) (agent.Result, error) {
	if t.gen == nil || !t.gen.HasImage() {
		return agent.Result{Output: "image generation is not configured (set AGEZT_IMAGE_URL + AGEZT_IMAGE_MODEL)", IsError: true}, nil
	}
	var in imageToolInput
	if err := json.Unmarshal(input, &in); err != nil {
		return agent.Result{Output: "invalid input: " + err.Error(), IsError: true}, nil
	}
	if strings.TrimSpace(in.Prompt) == "" {
		return agent.Result{Output: "image_generate needs a prompt", IsError: true}, nil
	}
	images, mime, err := t.gen.GenerateImage(ctx, in.Prompt, in.Size, in.Quality, in.N)
	if err != nil {
		return agent.Result{Output: "image generation failed: " + err.Error(), IsError: true}, nil
	}
	if len(images) == 0 {
		return agent.Result{Output: "no images were generated", IsError: true}, nil
	}
	if t.saveArtifact == nil {
		return agent.Result{Output: fmt.Sprintf("generated %d image(s) of %s, but artifact storage is unavailable to persist them", len(images), mime), IsError: true}, nil
	}
	refs := make([]string, 0, len(images))
	for i, img := range images {
		ref, err := t.saveArtifact(img)
		if err != nil {
			return agent.Result{Output: fmt.Sprintf("saving image %d failed: %v", i, err), IsError: true}, nil
		}
		refs = append(refs, ref)
	}
	return agent.Result{Output: fmt.Sprintf("generated %d image(s) of %s, saved as artifact(s): %s", len(images), mime, strings.Join(refs, ", "))}, nil
}
