// SPDX-License-Identifier: MIT

package main

import (
	"testing"

	"github.com/agezt/agezt/kernel/catalog"
)

// gateVisionWith is the pure core of the API/channel vision pre-gate (M255).
func TestGateVisionWith(t *testing.T) {
	cat := &catalog.Catalog{Providers: map[string]*catalog.Provider{
		"p": {ID: "p", Models: map[string]*catalog.Model{
			"vmodel": {ID: "vmodel", Modalities: catalog.Modalities{Input: []string{"text", "image"}}},
			"tmodel": {ID: "tmodel", Modalities: catalog.Modalities{Input: []string{"text"}}},
		}},
	}}
	imgs := []string{"data:image/png;base64,AA"}

	cases := []struct {
		name         string
		cat          *catalog.Catalog
		defaultModel string
		model        string
		images       []string
		wantErr      bool
	}{
		{"no images always passes", cat, "tmodel", "tmodel", nil, false},
		{"vision model passes", cat, "tmodel", "vmodel", imgs, false},
		{"non-vision model rejected", cat, "vmodel", "tmodel", imgs, true},
		{"empty model uses default (vision)", cat, "vmodel", "", imgs, false},
		{"empty model uses default (non-vision)", cat, "tmodel", "", imgs, true},
		{"unknown model rejected", cat, "tmodel", "ghost", imgs, true},
		{"nil catalog rejected", nil, "vmodel", "vmodel", imgs, true},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			err := gateVisionWith(c.cat, c.defaultModel, c.model, c.images)
			if (err != nil) != c.wantErr {
				t.Errorf("gateVisionWith err=%v, wantErr=%v", err, c.wantErr)
			}
		})
	}
}
