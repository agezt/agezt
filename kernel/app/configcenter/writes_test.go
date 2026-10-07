// SPDX-License-Identifier: MIT
package configcenter

import (
	"context"
	"errors"
	"reflect"
	"testing"
	"time"

	core "github.com/agezt/agezt/kernel/configcenter"
	"github.com/agezt/agezt/kernel/creds"
)

type centerWriteProbe struct {
	entry, saved, echoed      *core.ConfigEntry
	getErr, setErr, deleteErr error
	getErrors                 []error
	calls                     []string
	keys                      []string
	auto                      core.Rating
	classifyValue             string
}

func (p *centerWriteProbe) Set(e *core.ConfigEntry) error {
	p.calls = append(p.calls, "set")
	p.saved = e
	return p.setErr
}
func (p *centerWriteProbe) GetEntry(key string) (*core.ConfigEntry, error) {
	p.calls = append(p.calls, "get")
	p.keys = append(p.keys, key)
	err := p.getErr
	if len(p.getErrors) > 0 {
		err = p.getErrors[0]
		p.getErrors = p.getErrors[1:]
	}
	if p.echoed != nil && p.saved != nil {
		return p.echoed, err
	}
	return p.entry, err
}
func (p *centerWriteProbe) Delete(key string) error {
	p.calls = append(p.calls, "delete")
	p.keys = append(p.keys, key)
	return p.deleteErr
}
func (p *centerWriteProbe) GetAutoRating(key, value string) core.Rating {
	p.calls = append(p.calls, "classify")
	p.keys = append(p.keys, key)
	p.classifyValue = value
	return p.auto
}
func assertCenterWriteCalls(t *testing.T, p *centerWriteProbe, want ...string) {
	t.Helper()
	if !reflect.DeepEqual(p.calls, want) {
		t.Fatalf("calls %v want %v", p.calls, want)
	}
}
func TestConfigCenterWritesRatingBeforeAvailability(t *testing.T) {
	s := testCenterWrites(nil)
	for _, raw := range []string{" PUBLIC ", "invalid"} {
		if _, err := s.Set(context.Background(), SetInput{Rating: raw}); err == nil || err.Error() != "invalid rating: "+raw {
			t.Fatal(err)
		}
		if _, err := s.SetRating(context.Background(), SetRatingInput{Rating: raw}); err == nil || err.Error() != "invalid rating: "+raw+" (expected: public, internal, restricted, secret)" {
			t.Fatal(err)
		}
	}
	for _, call := range []func() error{
		func() error { _, e := s.Set(context.Background(), SetInput{}); return e },
		func() error { _, e := s.Delete(context.Background(), DeleteInput{}); return e },
		func() error { _, e := s.SetRating(context.Background(), SetRatingInput{Rating: "PUBLIC"}); return e },
		func() error { _, e := s.SetAccess(context.Background(), SetAccessInput{}); return e },
	} {
		if err := call(); err == nil || err.Error() != "config center not available" {
			t.Fatal(err)
		}
	}
}
func TestConfigCenterWritesSetRawDefaultsRereadMaskAndLegacyCancel(t *testing.T) {
	for _, raw := range []string{"", "PUBLIC", "internal", "ReStRiCtEd", "secret"} {
		ctx, cancel := context.WithCancel(context.Background())
		cancel()
		echo := &core.ConfigEntry{Key: " raw key ", Value: "owned-sensitive-middle-value", Rating: core.RatingSecret, Version: 9, CreatedAt: 11, UpdatedAt: 12, Description: "computed", AllowedAgents: []string{"echo"}}
		p := &centerWriteProbe{echoed: echo}
		s := testCenterWrites(p)
		in := SetInput{Key: " raw key ", Value: " raw value ", Rating: raw, Description: " raw description ", AllowedAgents: []string{" Raw Agent "}, ExcludedAgents: []string{}}
		before := time.Now().Unix()
		out, err := s.Set(ctx, in)
		after := time.Now().Unix()
		if err != nil {
			t.Fatal(err)
		}
		assertCenterWriteCalls(t, p, "set", "get")
		rating := core.RatingInternal
		if raw != "" {
			rating, _ = core.ParseRating(raw)
		}
		e := p.saved
		if e.Key != in.Key || e.Value != in.Value || e.Rating != rating || e.Description != in.Description || e.Version != 1 || e.CreatedAt < before || e.CreatedAt > after || e.UpdatedAt != e.CreatedAt || e.Tags == nil || e.Metadata == nil || !reflect.DeepEqual(e.AllowedAgents, in.AllowedAgents) || !reflect.DeepEqual(e.ExcludedAgents, in.ExcludedAgents) || !reflect.DeepEqual(p.keys, []string{in.Key}) {
			t.Fatal(e, p.keys)
		}
		if len(out) != 1 || !reflect.DeepEqual(out["entry"], testCenterEntryMap(echo)) || out["entry"].(map[string]any)["value"] != creds.MaskValue(echo.Value) {
			t.Fatal(out)
		}
	}
}
func TestConfigCenterWritesErrorsPreserveCauseAndEffects(t *testing.T) {
	sentinel := errors.New("owned cause")
	for _, phase := range []string{"set", "reread"} {
		p := &centerWriteProbe{entry: &core.ConfigEntry{}}
		if phase == "set" {
			p.setErr = sentinel
		} else {
			p.getErr = sentinel
		}
		out, err := testCenterWrites(p).Set(context.Background(), SetInput{Key: "owned"})
		if !errors.Is(err, sentinel) || out != nil || p.saved == nil {
			t.Fatal(out, err)
		}
		if phase == "set" {
			assertCenterWriteCalls(t, p, "set")
		} else {
			assertCenterWriteCalls(t, p, "set", "get")
		}
	}
	p := &centerWriteProbe{deleteErr: sentinel}
	out, err := testCenterWrites(p).Delete(context.Background(), DeleteInput{Key: " raw "})
	if out != nil || !errors.Is(err, sentinel) || !reflect.DeepEqual(p.keys, []string{" raw "}) {
		t.Fatal(out, err, p.keys)
	}
	assertCenterWriteCalls(t, p, "delete")
	p = &centerWriteProbe{}
	out, err = testCenterWrites(p).Delete(context.Background(), DeleteInput{Key: " raw "})
	if err != nil || !reflect.DeepEqual(out, map[string]any{"deleted": true}) {
		t.Fatal(out, err)
	}
}
func TestConfigCenterWritesSetRatingLookupClassifierOverrideAndErrors(t *testing.T) {
	for _, auto := range []core.Rating{core.RatingPublic, core.RatingSecret} {
		ctx, cancel := context.WithCancel(context.Background())
		cancel()
		e := &core.ConfigEntry{Key: "stored", Value: " raw value ", Rating: core.RatingInternal, Description: "retained", AllowedAgents: []string{"retained"}}
		p := &centerWriteProbe{entry: e, auto: auto}
		out, err := testCenterWrites(p).SetRating(ctx, SetRatingInput{Key: " raw key ", Rating: "PUBLIC"})
		if err != nil || !reflect.DeepEqual(out, map[string]any{"override": auto != core.RatingPublic}) || p.saved != e || e.Rating != core.RatingPublic || e.Description != "retained" || p.classifyValue != " raw value " || !reflect.DeepEqual(p.keys, []string{" raw key ", " raw key "}) {
			t.Fatal(out, err, p)
		}
		assertCenterWriteCalls(t, p, "get", "classify", "set")
	}
	sentinel := errors.New("lookup cause")
	p := &centerWriteProbe{getErr: sentinel}
	out, err := testCenterWrites(p).SetRating(context.Background(), SetRatingInput{Key: " raw ", Rating: "public"})
	if out != nil || err == nil || err.Error() != "key not found:  raw " || errors.Is(err, sentinel) {
		t.Fatal(out, err)
	}
	assertCenterWriteCalls(t, p, "get")
	p = &centerWriteProbe{entry: &core.ConfigEntry{}, setErr: sentinel}
	out, err = testCenterWrites(p).SetRating(context.Background(), SetRatingInput{Rating: "public"})
	if out != nil || !errors.Is(err, sentinel) || p.entry.Rating != core.RatingPublic {
		t.Fatal(out, err)
	}
	assertCenterWriteCalls(t, p, "get", "classify", "set")
}
func TestConfigCenterWritesSetAccessReplaceRereadAndErrorPhases(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	for _, allow := range [][]string{nil, {}, {" Raw Agent ", "second"}} {
		e := &core.ConfigEntry{Key: "stored", Value: "value", Rating: core.RatingPublic, Description: "retained", AllowedAgents: []string{"old"}, ExcludedAgents: []string{"old"}}
		echo := &core.ConfigEntry{Key: "computed", Value: "owned-sensitive-middle", Rating: core.RatingSecret, Version: 9}
		p := &centerWriteProbe{entry: e, echoed: echo}
		out, err := testCenterWrites(p).SetAccess(ctx, SetAccessInput{Key: " raw ", AllowedAgents: allow, ExcludedAgents: []string{"denied"}})
		if err != nil || p.saved != e || !reflect.DeepEqual(e.AllowedAgents, allow) || !reflect.DeepEqual(e.ExcludedAgents, []string{"denied"}) || e.Description != "retained" || e.Value != "value" || e.Rating != core.RatingPublic || !reflect.DeepEqual(out, map[string]any{"entry": testCenterEntryMap(echo)}) || !reflect.DeepEqual(p.keys, []string{" raw ", " raw "}) {
			t.Fatal(out, err, p)
		}
		assertCenterWriteCalls(t, p, "get", "set", "get")
	}
	sentinel := errors.New("owned access cause")
	for _, phase := range []string{"lookup", "set", "reread"} {
		p := &centerWriteProbe{entry: &core.ConfigEntry{AllowedAgents: []string{"old"}}}
		switch phase {
		case "lookup":
			p.getErr = sentinel
		case "set":
			p.setErr = sentinel
		case "reread":
			p.getErrors = []error{nil, sentinel}
		}
		out, err := testCenterWrites(p).SetAccess(context.Background(), SetAccessInput{Key: "owned"})
		if out != nil || err == nil {
			t.Fatal(out, err)
		}
		if phase == "lookup" {
			if err.Error() != "key not found: owned" || errors.Is(err, sentinel) || p.entry.AllowedAgents == nil {
				t.Fatal(err, p)
			}
			assertCenterWriteCalls(t, p, "get")
		} else {
			if !errors.Is(err, sentinel) || p.entry.AllowedAgents != nil {
				t.Fatal(err, p)
			}
			if phase == "set" {
				assertCenterWriteCalls(t, p, "get", "set")
			} else {
				assertCenterWriteCalls(t, p, "get", "set", "get")
			}
		}
	}
}
