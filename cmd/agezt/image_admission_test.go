// SPDX-License-Identifier: MIT

package main

import (
	"context"
	"encoding/json"
	"strings"
	"testing"

	"github.com/agezt/agezt/kernel/artifact"
	"github.com/agezt/agezt/kernel/catalog"
	"github.com/agezt/agezt/kernel/channel"
	"github.com/agezt/agezt/kernel/contract/llm"
	"github.com/agezt/agezt/kernel/event"
	kernelruntime "github.com/agezt/agezt/kernel/runtime"
	"github.com/agezt/agezt/plugins/providers/mock"
)

// Exercise the actual shared REST/OpenAI engine and channel handler, including
// the provider boundary and durable rejection event rather than a gate stub.
func TestImageIngress(t *testing.T) {
	for _, ingress := range []string{"api", "channel"} {
		for _, mode := range []string{"sidecar", "vision", "reject", "empty-caption"} {
			t.Run(ingress+"/"+mode, func(t *testing.T) {
				const corr = "image-ingress"
				images := []string{dataURL("image/png", "image")}
				var requests []llm.CompletionRequest
				prov := mock.New()
				prov.OnRequest = func(req llm.CompletionRequest) { requests = append(requests, req) }
				prov.Responder = func(req llm.CompletionRequest) llm.CompletionResponse {
					if req.TaskType == "vision" {
						if mode == "empty-caption" {
							return mock.FinalText("  ")
						}
						return mock.FinalText("a red square")
					}
					return mock.FinalText("image handled")
				}
				cfg := kernelruntime.Config{BaseDir: t.TempDir(), Provider: prov, Model: "text-model"}
				if mode != "reject" {
					cfg.VisionModel = func() (string, bool) { return "vision-model", true }
				}
				if mode == "vision" {
					cfg.Model = "vision-model"
					cfg.Catalog = &catalog.Catalog{Providers: map[string]*catalog.Provider{
						"p": {ID: "p", Models: map[string]*catalog.Model{
							"vision-model": {ID: "vision-model", Modalities: catalog.Modalities{Input: []string{"text", "image"}}},
						}},
					}}
				}
				k, err := kernelruntime.Open(cfg)
				if err != nil {
					t.Fatal(err)
				}
				t.Cleanup(func() { k.Close() })
				var answer string
				if ingress == "api" {
					answer, err = (kernelAPIEngine{k}).RunModel(context.Background(), corr, "describe this", "", images, false)
				} else {
					var reply channel.Reply
					reply, err = makeChannelHandler(k)(context.Background(), channel.UnifiedMessage{
						ChannelKind: "telegram", Sender: "test", Text: "describe this", Images: images,
					}, corr)
					answer = reply.Text
				}
				if ingress == "channel" {
					entries := k.ArtifactIndex().List(artifact.Filter{Kind: "image", Corr: corr})
					if len(entries) != 1 {
						t.Fatalf("archived %d images, want 1", len(entries))
					}
					if mode == "sidecar" && entries[0].Caption != "a red square" {
						t.Errorf("archive caption=%q", entries[0].Caption)
					}
				}
				if mode == "reject" || mode == "empty-caption" {
					if err == nil || !strings.Contains(err.Error(), "add a vision-capable provider key") {
						t.Fatalf("error = %v; want shared vision rejection", err)
					}
					var rejected int
					if jerr := k.Journal().Range(func(e *event.Event) error {
						if e.Kind == event.KindCapabilityRejected && e.CorrelationID == corr {
							var p struct {
								Model      string
								Capability string
								Images     int `json:"images_requested"`
							}
							if err := json.Unmarshal(e.Payload, &p); err != nil {
								return err
							}
							if p.Model != "text-model" || p.Capability != "vision" || p.Images != 1 {
								t.Errorf("rejection payload = %+v", p)
							}
							rejected++
						}
						return nil
					}); jerr != nil {
						t.Fatal(jerr)
					}
					if rejected != 1 {
						t.Fatalf("journaled %d correlated rejections, want 1", rejected)
					}
					for _, req := range requests {
						if req.TaskType != "vision" {
							t.Fatal("rejected images reached primary provider")
						}
					}
					return
				}
				if err != nil || answer != "image handled" {
					t.Fatalf("answer=%q err=%v", answer, err)
				}
				var sidecar, primary int
				for _, req := range requests {
					if req.TaskType == "vision" {
						sidecar++
						if req.Model != "vision-model" || req.CorrelationID != corr || len(req.Messages[0].Images) != 1 {
							t.Errorf("sidecar request = %+v", req)
						}
						continue
					}
					primary++
					var sawCaption, sawImages bool
					for _, msg := range req.Messages {
						sawCaption = sawCaption || strings.Contains(msg.Content, "a red square")
						sawImages = sawImages || len(msg.Images) > 0
					}
					if mode == "sidecar" && (!sawCaption || sawImages) {
						t.Errorf("primary caption=%v images=%v", sawCaption, sawImages)
					}
					if mode == "vision" && !sawImages {
						t.Error("vision primary lost images")
					}
				}
				wantSidecar := 0
				if mode == "sidecar" {
					wantSidecar = 1
				}
				if sidecar != wantSidecar || primary != 1 {
					t.Errorf("sidecar=%d primary=%d", sidecar, primary)
				}
			})
		}
	}
}
