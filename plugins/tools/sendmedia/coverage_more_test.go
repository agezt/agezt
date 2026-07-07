// SPDX-License-Identifier: MIT

package sendmedia

import (
	"context"
	"encoding/json"
	"errors"
	"reflect"
	"strings"
	"testing"

	"github.com/agezt/agezt/kernel/agent"
	"github.com/agezt/agezt/kernel/channel"
)

func TestSendmediaCoverageDefinition(t *testing.T) {
	tl := New()
	def := tl.Definition()
	if def.Name != "send_media" {
		t.Fatalf("Name = %q", def.Name)
	}
	if len(def.InputSchema) == 0 {
		t.Fatal("InputSchema should not be empty")
	}
	if !strings.Contains(def.Description, "(none configured yet)") {
		t.Fatalf("description should reflect empty targets, got %q", def.Description)
	}

	tl.Bind(
		func(context.Context, string, string, string, []channel.Attachment) error { return nil },
		map[string][]string{"telegram": {"1"}, "discord": {"2"}},
		func(string) ([]byte, error) { return nil, nil },
	)
	def = tl.Definition()
	if !strings.Contains(def.Description, "discord, telegram") {
		t.Fatalf("description should list sorted kinds, got %q", def.Description)
	}
	if def.Effect.Class != agent.EffectCompensable {
		t.Fatalf("Effect.Class = %v, want %v", def.Effect.Class, agent.EffectCompensable)
	}
}

func TestSendmediaCoverageExtForMIME(t *testing.T) {
	cases := map[string]string{
		"image/png":             ".png",
		"image/jpeg":            ".jpg",
		"image/jpg":             ".jpg",
		"image/gif":             ".gif",
		"image/webp":            ".webp",
		"audio/ogg;codecs=opus": ".ogg",
		"audio/opus":            ".ogg",
		"audio/mpeg":            ".mp3",
		"audio/mp3":             ".mp3",
		"audio/wav":             ".wav",
		"application/pdf":       ".pdf",
		"unknown/type":          ".bin",
	}
	for mime, want := range cases {
		if got := extForMIME(mime, ""); got != want {
			t.Fatalf("extForMIME(%q) = %q, want %q", mime, got, want)
		}
	}
	if got := extForMIME("unknown/type", "image"); got != ".png" {
		t.Fatalf("image kind fallback = %q", got)
	}
	if got := extForMIME("unknown/type", "audio"); got != ".ogg" {
		t.Fatalf("audio kind fallback = %q", got)
	}
}

func TestSendmediaCoverageBindPruning(t *testing.T) {
	tl := New()
	tl.Bind(
		func(context.Context, string, string, string, []channel.Attachment) error { return nil },
		map[string][]string{"telegram": {"1"}, "discord": {}},
		func(string) ([]byte, error) { return nil, nil },
	)
	// snapshot reflects pruned targets.
	_, _, targets := tl.snapshot()
	if got := targets["discord"]; got != nil {
		t.Fatalf("discord should be pruned, got %v", got)
	}
	if got := targets["telegram"]; len(got) != 1 || got[0] != "1" {
		t.Fatalf("telegram should be kept, got %v", got)
	}
}

func TestSendmediaCoverageKinds(t *testing.T) {
	got := kinds(map[string][]string{"discord": {"1"}, "telegram": {"2"}, "empty": {}})
	want := []string{"discord", "empty", "telegram"}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("kinds = %v, want %v", got, want)
	}
}

func TestSendmediaCoverageInvokeBranches(t *testing.T) {
	// Channel-restricted delivery + send error aggregation.
	tl := New()
	var calls []string
	tl.Bind(
		func(_ context.Context, kind, id, _ string, _ []channel.Attachment) error {
			calls = append(calls, kind+"/"+id)
			return errors.New("boom")
		},
		map[string][]string{"telegram": {"1", "2"}, "discord": {"3"}},
		func(string) ([]byte, error) { return pngBytes, nil },
	)

	// Empty artifact data → error.
	tlEmpty := New()
	tlEmpty.Bind(
		func(context.Context, string, string, string, []channel.Attachment) error { return nil },
		map[string][]string{"telegram": {"1"}},
		func(string) ([]byte, error) { return []byte{}, nil },
	)
	res, _ := tlEmpty.Invoke(context.Background(), json.RawMessage(`{"artifact":"x"}`))
	if !res.IsError || !strings.Contains(res.Output, "empty") {
		t.Fatalf("empty artifact = %+v", res)
	}

	// Malformed JSON.
	res, _ = tl.Invoke(context.Background(), json.RawMessage(`{`))
	if !res.IsError || !strings.Contains(res.Output, "invalid input") {
		t.Fatalf("malformed = %+v", res)
	}

	// All sends fail: when no deliveries succeed, the output is "send_media
	// failed: <errors>" without the partial-success "FAILED" suffix.
	res, _ = tl.Invoke(context.Background(), json.RawMessage(`{"artifact":"a"}`))
	if !res.IsError || !strings.Contains(res.Output, "send_media failed") || !strings.Contains(res.Output, "telegram/1") {
		t.Fatalf("all-fail = %q (calls=%v)", res.Output, calls)
	}
	if len(calls) != 3 {
		t.Fatalf("expected 3 delivery attempts, got %v", calls)
	}
}
