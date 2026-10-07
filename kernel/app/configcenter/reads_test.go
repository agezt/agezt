// SPDX-License-Identifier: MIT
package configcenter

import (
	"context"
	"errors"
	core "github.com/agezt/agezt/kernel/configcenter"
	"github.com/agezt/agezt/kernel/creds"
	"reflect"
	"strings"
	"testing"
	"time"
)

type configCenterReadProbe struct {
	entry      *core.ConfigEntry
	entries    []*core.ConfigEntry
	logs       []*core.AuditEntry
	stats      map[string]any
	err        error
	calls      []string
	rating     core.Rating
	key, agent string
	since      time.Duration
}

func (p *configCenterReadProbe) GetEntry(key string) (*core.ConfigEntry, error) {
	p.calls = append(p.calls, "get")
	p.key = key
	return p.entry, p.err
}
func (p *configCenterReadProbe) ListEntries() []*core.ConfigEntry {
	p.calls = append(p.calls, "list")
	return p.entries
}
func (p *configCenterReadProbe) ListByRating(rating core.Rating) []*core.ConfigEntry {
	p.calls = append(p.calls, "rating")
	p.rating = rating
	return p.entries
}
func (p *configCenterReadProbe) AccessLog(key, agent string, since time.Duration) []*core.AccessLogEntry {
	p.calls = append(p.calls, "access")
	p.key = key
	p.agent = agent
	p.since = since
	return p.logs
}
func (p *configCenterReadProbe) AuditLog(since time.Duration) []*core.AuditEntry {
	p.calls = append(p.calls, "audit")
	p.since = since
	return p.logs
}
func (p *configCenterReadProbe) Stats() map[string]any {
	p.calls = append(p.calls, "stats")
	return p.stats
}
func TestConfigCenterReadEntryRequiredOptionalMaskAndACLArrayOwnership(t *testing.T) {
	for _, rating := range []core.Rating{"", core.RatingPublic, core.RatingInternal, core.RatingRestricted, core.RatingSecret} {
		e := &core.ConfigEntry{Key: " raw key ", Value: "owned-sensitive-middle-value", Rating: rating, CreatedAt: 9007199254740993, UpdatedAt: 0, Version: -2}
		out := testCenterEntryMap(e)
		if len(out) != 6+cfgBoolInt(rating == core.RatingSecret) || out["key"] != e.Key || out["created_at"] != e.CreatedAt || out["updated_at"] != int64(0) || out["version"] != -2 || out["rating"] != string(rating) {
			t.Fatal(out)
		}
		expected := e.Value
		if rating == core.RatingSecret {
			expected = creds.MaskValue(e.Value)
			if out["masked"] != true || strings.Contains(out["value"].(string), "sensitive-middle") {
				t.Fatal("unmasked secret")
			}
		}
		if out["value"] != expected {
			t.Fatal(out)
		}
		e.Description = " raw description "
		e.Tags = []string{"second", "first"}
		e.AccessPolicy = core.PolicyDeny
		e.AllowedAgents = []string{" Raw Agent ", "first"}
		e.ExcludedAgents = []string{" Raw Denied "}
		rich := testCenterEntryMap(e)
		if len(rich) != 11+cfgBoolInt(rating == core.RatingSecret) || rich["description"] != e.Description || rich["access_policy"] != string(e.AccessPolicy) || !reflect.DeepEqual(rich["tags"], e.Tags) || !reflect.DeepEqual(rich["allowed_agents"], e.AllowedAgents) {
			t.Fatal(rich)
		}
		rich["allowed_agents"].([]string)[0] = "changed"
		rich["excluded_agents"].([]string)[0] = "changed"
		if e.AllowedAgents[0] != " Raw Agent " || e.ExcludedAgents[0] != " Raw Denied " {
			t.Fatal("ACL arrays aliased")
		}
	}
}
func TestConfigCenterReadsAdmissionOrderRawFiltersFreshnessAndLegacyCancel(t *testing.T) {
	svc := testCenterReads(nil)
	if _, err := svc.Get(context.Background(), GetInput{Key: "owned"}); err == nil || err.Error() != "config center not available" {
		t.Fatal(err)
	}
	if _, err := svc.List(context.Background(), ListInput{Rating: " PUBLIC "}); err == nil || err.Error() != "invalid rating:  PUBLIC " {
		t.Fatal(err)
	}
	if _, err := svc.AccessLog(context.Background(), AccessLogInput{Since: "bad"}); err == nil || err.Error() != "invalid duration: bad" {
		t.Fatal(err)
	}
	if _, err := svc.Audit(context.Background(), AuditInput{Since: "bad"}); err == nil || err.Error() != "invalid duration: bad" {
		t.Fatal(err)
	}
	health, err := svc.Health(context.Background(), HealthInput{})
	if err != nil || len(health) != 2 || health["status"] != "unavailable" || !reflect.DeepEqual(health["checks"], map[string]string{"config_center": "not configured"}) {
		t.Fatal(health, err)
	}
	p := &configCenterReadProbe{entry: &core.ConfigEntry{Key: "owned", Value: "first"}, err: errors.New("private underlying cause")}
	svc = testCenterReads(p)
	if _, err := svc.Get(context.Background(), GetInput{Key: " raw "}); err == nil || err.Error() != "key not found:  raw " || p.key != " raw " {
		t.Fatal(err, p)
	}
	p.err = nil
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	out, err := svc.Get(ctx, GetInput{Key: " raw "})
	if err != nil || out["entry"].(map[string]any)["value"] != "first" {
		t.Fatal(out, err)
	}
	p.entry.Value = "fresh"
	second, _ := svc.Get(ctx, GetInput{Key: " raw "})
	if out["entry"].(map[string]any)["value"] != "first" || second["entry"].(map[string]any)["value"] != "fresh" {
		t.Fatal(out, second)
	}
	for _, rating := range []string{"", "PUBLIC", "internal", "restricted", "secret"} {
		p.calls = nil
		result, err := svc.List(ctx, ListInput{Rating: rating})
		if err != nil || result["entries"] == nil || result["count"] != 0 {
			t.Fatal(result, err)
		}
		want := []string{"list"}
		if rating != "" {
			want = []string{"rating"}
		}
		if !reflect.DeepEqual(p.calls, want) || rating != "" && p.rating != core.Rating(strings.ToLower(rating)) {
			t.Fatal(p)
		}
	}
	for _, duration := range []string{"", "0", "1h", "-1h"} {
		want := time.Duration(0)
		if duration != "" {
			want, _ = time.ParseDuration(duration)
		}
		_, err := svc.AccessLog(ctx, AccessLogInput{Key: " raw ", AgentID: " Agent ", Since: duration})
		if err != nil || p.key != " raw " || p.agent != " Agent " || p.since != want {
			t.Fatal(err, p)
		}
		_, err = svc.Audit(ctx, AuditInput{Since: duration})
		if err != nil || p.since != want {
			t.Fatal(err, p)
		}
	}
	p.stats = map[string]any{"total_entries": 0, "by_rating": map[string]int{}}
	health, err = svc.Health(ctx, HealthInput{})
	if err != nil || len(health) != 3 || health["status"] != "healthy" || !reflect.DeepEqual(health["checks"], map[string]string{"config_center": "ok", "store": "ok"}) || !reflect.DeepEqual(health["stats"], p.stats) {
		t.Fatal(health, err)
	}
}
func TestConfigCenterReadLogsRequiredZeroRawOrderAndEmptyCollections(t *testing.T) {
	p := &configCenterReadProbe{logs: []*core.AuditEntry{{Timestamp: 9007199254740993, Event: " raw event ", Key: " raw key ", AgentID: " raw agent ", RunID: " raw run ", Rating: core.RatingSecret, Decision: core.AccessDenied, Reason: " raw reason ", ValueLog: "REDACTED", Policy: " raw policy "}, {}}}
	svc := testCenterReads(p)
	out, err := svc.AccessLog(context.Background(), AccessLogInput{})
	if err != nil || len(out) != 2 || out["count"] != 2 {
		t.Fatal(out, err)
	}
	rows := out["logs"].([]map[string]any)
	if len(rows[0]) != 8 || len(rows[1]) != 8 || rows[0]["timestamp"] != p.logs[0].Timestamp || rows[0]["value_log"] != "REDACTED" || rows[1]["timestamp"] != int64(0) || rows[1]["reason"] != "" {
		t.Fatal(rows)
	}
	out, err = svc.Audit(context.Background(), AuditInput{})
	if err != nil || len(out) != 2 || out["count"] != 2 {
		t.Fatal(out, err)
	}
	rows = out["entries"].([]map[string]any)
	if len(rows[0]) != 9 || len(rows[1]) != 9 || rows[0]["event"] != " raw event " || rows[0]["policy"] != " raw policy " || rows[1]["rating"] != "" {
		t.Fatal(rows)
	}
	for _, logs := range [][]*core.AuditEntry{nil, {}} {
		p.logs = logs
		out, _ := svc.AccessLog(context.Background(), AccessLogInput{})
		if out["logs"] == nil || len(out["logs"].([]map[string]any)) != 0 {
			t.Fatal(out)
		}
		out, _ = svc.Audit(context.Background(), AuditInput{})
		if out["entries"] == nil || len(out["entries"].([]map[string]any)) != 0 {
			t.Fatal(out)
		}
	}
}
func cfgBoolInt(value bool) int {
	if value {
		return 1
	}
	return 0
}
