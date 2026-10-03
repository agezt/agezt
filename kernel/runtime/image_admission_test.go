// SPDX-License-Identifier: MIT

package runtime_test

import (
	"context"
	"errors"
	"reflect"
	"strings"
	"testing"

	"github.com/agezt/agezt/kernel/catalog"
	"github.com/agezt/agezt/kernel/runtime"
	"github.com/agezt/agezt/plugins/providers/mock"
)

func TestAdmitImages_ModelResolution(t *testing.T) {
	cat := &catalog.Catalog{Providers: map[string]*catalog.Provider{
		"p": {ID: "p", Models: map[string]*catalog.Model{
			"vision": {ID: "vision", Modalities: catalog.Modalities{Input: []string{"text", "image"}}},
			"text":   {ID: "text", Modalities: catalog.Modalities{Input: []string{"text"}}},
		}},
	}}
	for _, tc := range []struct {
		name, defaultModel, ctxModel, model string
		noImages, noCatalog, wantErr        bool
	}{
		{name: "no images", defaultModel: "text", noImages: true},
		{name: "vision override", defaultModel: "text", ctxModel: "text", model: "vision"},
		{name: "text override", defaultModel: "vision", ctxModel: "vision", model: "text", wantErr: true},
		{name: "vision context", defaultModel: "text", ctxModel: "vision"},
		{name: "text context", defaultModel: "vision", ctxModel: "text", wantErr: true},
		{name: "vision default", defaultModel: "vision"},
		{name: "text default", defaultModel: "text", wantErr: true},
		{name: "unknown", defaultModel: "vision", model: "ghost", wantErr: true},
		{name: "nil catalog", defaultModel: "vision", noCatalog: true, wantErr: true},
	} {
		t.Run(tc.name, func(t *testing.T) {
			cfg := runtime.Config{BaseDir: t.TempDir(), Provider: mock.New(), Model: tc.defaultModel, Catalog: cat}
			if tc.noCatalog {
				cfg.Catalog = nil
			}
			k, err := runtime.Open(cfg)
			if err != nil {
				t.Fatal(err)
			}
			t.Cleanup(func() { k.Close() })
			images := []string{"photo"}
			if tc.noImages {
				images = nil
			}
			ctx := runtime.WithModel(context.Background(), tc.ctxModel)
			got, err := k.AdmitImages(ctx, "test", tc.model, " ", images)
			if (err != nil) != tc.wantErr {
				t.Fatalf("error=%v wantErr=%v", err, tc.wantErr)
			}
			if err == nil {
				if !reflect.DeepEqual(got.Images, images) || got.Caption != "" {
					t.Errorf("admission=%+v", got)
				}
				if tc.noImages && got.Intent != " " {
					t.Error("plain intent changed")
				}
				if !tc.noImages && !strings.Contains(got.Intent, "Describe the attached") {
					t.Error("image-only intent missing")
				}
			}
		})
	}
}

func TestAdmitImages_Cancellation(t *testing.T) {
	k, err := runtime.Open(runtime.Config{BaseDir: t.TempDir(), Provider: mock.New(), VisionModel: func() (string, bool) { return "vision", true }})
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { k.Close() })
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	if _, err := k.AdmitImages(ctx, "cancelled", "text", "describe", []string{"photo"}); !errors.Is(err, context.Canceled) {
		t.Fatalf("error=%v", err)
	}
}
