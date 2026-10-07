// SPDX-License-Identifier: MIT
package controlplane

import (
	"context"
	"path/filepath"
	"reflect"
	"testing"

	appconfigcenter "github.com/agezt/agezt/kernel/app/configcenter"
	core "github.com/agezt/agezt/kernel/configcenter"
	"github.com/agezt/agezt/kernel/runtime"
	"github.com/agezt/agezt/plugins/providers/mock"
)

func TestConfigCenterWritesNativeSelectedManagerPersistenceClassifierAndNoDirectAudit(t *testing.T) {
	root := t.TempDir()
	p := mock.New()
	k, err := runtime.Open(runtime.Config{BaseDir: filepath.Join(root, "kernel"), Provider: p})
	if err != nil {
		t.Fatal(err)
	}
	defer k.Close()
	s := NewServer(k, filepath.Join(root, "server"))
	svc := s.configCenterWrites()
	head, hash := k.Journal().Head()
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	if _, err := svc.Set(ctx, appconfigcenter.SetInput{Key: "owned", Value: " raw value ", Description: "retained"}); err != nil {
		t.Fatal(err)
	}
	e, err := k.ConfigCenter().GetEntry("owned")
	if err != nil || e.Value != " raw value " || e.Rating != core.RatingInternal || e.Version != 1 {
		t.Fatal(e, err)
	}
	k.ConfigCenter().Classifier().SetOverride("owned", core.RatingPublic)
	out, err := svc.SetRating(ctx, appconfigcenter.SetRatingInput{Key: "owned", Rating: "PUBLIC"})
	if err != nil || !reflect.DeepEqual(out, appconfigcenter.SetRatingOutput{Override: false}) {
		t.Fatal(out, err)
	}
	out, err = svc.SetRating(ctx, appconfigcenter.SetRatingInput{Key: "owned", Rating: "restricted"})
	if err != nil || !reflect.DeepEqual(out, appconfigcenter.SetRatingOutput{Override: true}) {
		t.Fatal(out, err)
	}
	if _, err := svc.SetAccess(ctx, appconfigcenter.SetAccessInput{Key: "owned", AllowedAgents: []string{"allowed"}, ExcludedAgents: []string{"denied"}}); err != nil {
		t.Fatal(err)
	}
	e, err = k.ConfigCenter().GetEntry("owned")
	if err != nil || e.Version != 4 || e.Description != "retained" || e.Rating != core.RatingRestricted || !reflect.DeepEqual(e.AllowedAgents, []string{"allowed"}) || !reflect.DeepEqual(e.ExcludedAgents, []string{"denied"}) {
		t.Fatal(e, err)
	}
	reopened, err := core.New(core.DefaultConfig(filepath.Join(root, "kernel")))
	if err != nil {
		t.Fatal(err)
	}
	defer reopened.Close()
	// The existing loader routes through Store.Set, which resets loaded versions to 1.
	restored, err := reopened.GetEntry("owned")
	if err != nil || restored.Value != e.Value || restored.Version != 1 || !reflect.DeepEqual(restored.AllowedAgents, e.AllowedAgents) {
		t.Fatal(restored, err)
	}
	if _, err := svc.Delete(ctx, appconfigcenter.DeleteInput{Key: "owned"}); err != nil {
		t.Fatal(err)
	}
	if _, err := k.ConfigCenter().GetEntry("owned"); err == nil {
		t.Fatal("delete did not reach selected manager")
	}
	after, afterHash := k.Journal().Head()
	if head != after || hash != afterHash || p.CallCount() != 0 {
		t.Fatal("direct app calls changed journal/provider")
	}
}
