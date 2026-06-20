// SPDX-License-Identifier: MIT

package sendmedia

import (
	"context"
	"encoding/json"
	"strings"
	"testing"

	"github.com/agezt/agezt/kernel/channel"
)

// pngBytes is a minimal valid PNG header so http.DetectContentType sniffs image/png.
var pngBytes = []byte{0x89, 'P', 'N', 'G', '\r', '\n', 0x1a, '\n', 0, 0, 0, 0}

func TestUnboundIsCleanError(t *testing.T) {
	tl := New()
	res, err := tl.Invoke(context.Background(), json.RawMessage(`{"artifact":"abc"}`))
	if err != nil {
		t.Fatalf("unexpected err: %v", err)
	}
	if !res.IsError || !strings.Contains(res.Output, "not configured") {
		t.Fatalf("want not-configured error, got %+v", res)
	}
}

func TestSendPinsToAllowlistAndAttaches(t *testing.T) {
	var gotKind, gotID, gotText string
	var gotAtts []channel.Attachment
	tl := New()
	tl.Bind(
		func(_ context.Context, kind, id, text string, atts []channel.Attachment) error {
			gotKind, gotID, gotText, gotAtts = kind, id, text, atts
			return nil
		},
		map[string][]string{"telegram": {"123"}},
		func(ref string) ([]byte, error) { return pngBytes, nil },
	)

	res, err := tl.Invoke(context.Background(), json.RawMessage(`{"artifact":"ref1","caption":"look"}`))
	if err != nil {
		t.Fatal(err)
	}
	if res.IsError {
		t.Fatalf("unexpected error result: %+v", res)
	}
	if gotKind != "telegram" || gotID != "123" || gotText != "look" {
		t.Fatalf("delivery = %q/%q text=%q", gotKind, gotID, gotText)
	}
	if len(gotAtts) != 1 || gotAtts[0].Kind != "image" || gotAtts[0].MIME != "image/png" {
		t.Fatalf("attachment = %+v", gotAtts)
	}
	if string(gotAtts[0].Data) != string(pngBytes) {
		t.Fatal("attachment data mismatch")
	}
}

func TestArtifactRequired(t *testing.T) {
	tl := New()
	tl.Bind(
		func(_ context.Context, _, _, _ string, _ []channel.Attachment) error { return nil },
		map[string][]string{"slack": {"C1"}},
		func(string) ([]byte, error) { return pngBytes, nil },
	)
	res, _ := tl.Invoke(context.Background(), json.RawMessage(`{"caption":"hi"}`))
	if !res.IsError || !strings.Contains(res.Output, "artifact is required") {
		t.Fatalf("want artifact-required error, got %+v", res)
	}
}

func TestResolveErrorSurfaces(t *testing.T) {
	tl := New()
	tl.Bind(
		func(_ context.Context, _, _, _ string, _ []channel.Attachment) error { return nil },
		map[string][]string{"slack": {"C1"}},
		func(string) ([]byte, error) { return nil, context.DeadlineExceeded },
	)
	res, _ := tl.Invoke(context.Background(), json.RawMessage(`{"artifact":"missing"}`))
	if !res.IsError || !strings.Contains(res.Output, "could not resolve artifact") {
		t.Fatalf("want resolve error, got %+v", res)
	}
}

func TestUnknownChannelRejected(t *testing.T) {
	tl := New()
	tl.Bind(
		func(_ context.Context, _, _, _ string, _ []channel.Attachment) error { return nil },
		map[string][]string{"slack": {"C1"}},
		func(string) ([]byte, error) { return pngBytes, nil },
	)
	res, _ := tl.Invoke(context.Background(), json.RawMessage(`{"artifact":"r","channel":"telegram"}`))
	if !res.IsError || !strings.Contains(res.Output, "not configured") {
		t.Fatalf("want unknown-channel error, got %+v", res)
	}
}

func TestBuildAttachmentClassification(t *testing.T) {
	// PNG sniffs to image.
	if a := buildAttachment(pngBytes, ""); a.Kind != "image" || a.MIME != "image/png" || a.Filename != "media.png" {
		t.Fatalf("png = %+v", a)
	}
	// Unsniffable bytes with an audio hint fall back to an audio MIME.
	if a := buildAttachment([]byte{0, 1, 2, 3, 4, 5}, "audio"); a.Kind != "audio" || a.MIME != "audio/ogg" || a.Filename != "media.ogg" {
		t.Fatalf("audio hint = %+v", a)
	}
	// No hint + unsniffable → file.
	if a := buildAttachment([]byte{0, 1, 2, 3, 4, 5}, ""); a.Kind != "file" {
		t.Fatalf("file = %+v", a)
	}
}
