// SPDX-License-Identifier: MIT

package runtime

import (
	"context"
	"encoding/json"
	"errors"
	"strings"
	"testing"
)

// fakeImageGen is a controllable ImageGen for exercising imageTool.Invoke.
type fakeImageGen struct {
	has    bool
	images [][]byte
	mime   string
	err    error
}

func (f *fakeImageGen) GenerateImage(_ context.Context, _, _, _ string, _ int) ([][]byte, string, error) {
	return f.images, f.mime, f.err
}
func (f *fakeImageGen) HasImage() bool { return f.has }

// TestImageTool_Definition covers newImageTool + Definition.
func TestImageTool_Definition(t *testing.T) {
	tool := newImageTool(&fakeImageGen{has: true})
	def := tool.Definition()
	if def.Name != "image_generate" {
		t.Errorf("Definition name = %q, want image_generate", def.Name)
	}
	if def.Description == "" {
		t.Error("Definition description is empty")
	}
}

// TestImageTool_Invoke walks every branch of imageTool.Invoke.
func TestImageTool_Invoke(t *testing.T) {
	ctx := context.Background()

	// Not configured (nil gen) → error result.
	tool := newImageTool(nil)
	res, err := tool.Invoke(ctx, json.RawMessage(`{"prompt":"cat"}`))
	if err != nil || !res.IsError || !strings.Contains(res.Output, "not configured") {
		t.Errorf("nil gen: res=%+v err=%v", res, err)
	}

	// HasImage() false → not configured.
	tool = newImageTool(&fakeImageGen{has: false})
	res, _ = tool.Invoke(ctx, json.RawMessage(`{"prompt":"cat"}`))
	if !res.IsError {
		t.Error("HasImage=false should be an error result")
	}

	// Invalid JSON.
	tool = newImageTool(&fakeImageGen{has: true})
	res, _ = tool.Invoke(ctx, json.RawMessage(`{bad json`))
	if !res.IsError || !strings.Contains(res.Output, "invalid input") {
		t.Errorf("invalid json: res=%+v", res)
	}

	// Empty prompt.
	res, _ = tool.Invoke(ctx, json.RawMessage(`{"prompt":"  "}`))
	if !res.IsError || !strings.Contains(res.Output, "needs a prompt") {
		t.Errorf("empty prompt: res=%+v", res)
	}

	// GenerateImage error.
	tool = newImageTool(&fakeImageGen{has: true, err: errors.New("boom")})
	res, _ = tool.Invoke(ctx, json.RawMessage(`{"prompt":"cat"}`))
	if !res.IsError || !strings.Contains(res.Output, "failed") {
		t.Errorf("gen error: res=%+v", res)
	}

	// No images returned.
	tool = newImageTool(&fakeImageGen{has: true, images: nil, mime: "image/png"})
	res, _ = tool.Invoke(ctx, json.RawMessage(`{"prompt":"cat"}`))
	if !res.IsError || !strings.Contains(res.Output, "no images") {
		t.Errorf("no images: res=%+v", res)
	}

	// Images generated but no artifact store bound.
	tool = newImageTool(&fakeImageGen{has: true, images: [][]byte{[]byte("img")}, mime: "image/png"})
	res, _ = tool.Invoke(ctx, json.RawMessage(`{"prompt":"cat"}`))
	if !res.IsError || !strings.Contains(res.Output, "artifact storage is unavailable") {
		t.Errorf("no store: res=%+v", res)
	}

	// Success: artifact store bound, one image.
	tool = newImageTool(&fakeImageGen{has: true, images: [][]byte{[]byte("img1"), []byte("img2")}, mime: "image/png"})
	tool.saveArtifact = func(data []byte) (string, error) { return "ref-" + string(data), nil }
	res, _ = tool.Invoke(ctx, json.RawMessage(`{"prompt":"cat","n":2}`))
	if res.IsError || !strings.Contains(res.Output, "saved as artifact") {
		t.Errorf("success: res=%+v", res)
	}

	// saveArtifact error path.
	tool.saveArtifact = func(data []byte) (string, error) { return "", errors.New("disk full") }
	res, _ = tool.Invoke(ctx, json.RawMessage(`{"prompt":"cat"}`))
	if !res.IsError || !strings.Contains(res.Output, "saving image") {
		t.Errorf("save error: res=%+v", res)
	}
}
