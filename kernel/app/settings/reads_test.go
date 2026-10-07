// SPDX-License-Identifier: MIT
package settings

import (
	"context"
	"encoding/json"
	core "github.com/agezt/agezt/kernel/settings"
	"reflect"
	"testing"
)

type settingsReadProbe struct {
	sections       []Section
	calls          []string
	env, stored    map[string]string
	secret, pinned map[string]bool
}

func (p *settingsReadProbe) SchemaSections() []Section {
	p.calls = append(p.calls, "schema")
	return p.sections
}
func (p *settingsReadProbe) PrepareValues() ValuesReader {
	p.calls = append(p.calls, "prepare")
	return p
}
func (p *settingsReadProbe) Sections() []Section {
	p.calls = append(p.calls, "sections")
	return p.sections
}
func (p *settingsReadProbe) EnvPinned(name string) bool {
	p.calls = append(p.calls, "pin:"+name)
	return p.pinned[name]
}
func (p *settingsReadProbe) SecretSet(name string) bool {
	p.calls = append(p.calls, "secret:"+name)
	return p.secret[name]
}
func (p *settingsReadProbe) EnvValue(name string) string {
	p.calls = append(p.calls, "env:"+name)
	return p.env[name]
}
func (p *settingsReadProbe) StoredValue(name string) (string, bool) {
	p.calls = append(p.calls, "store:"+name)
	value, ok := p.stored[name]
	return value, ok
}
func TestSettingsReadsSchemaShapeBoundariesAndFreshLegacyCanceledCalls(t *testing.T) {
	for _, sections := range [][]Section{nil, {}, {{ID: " raw id ", Name: " raw name ", Help: " raw help ", Source: " raw source ", Locked: true, Fields: []core.Field{{Env: "z", Label: " raw label ", Type: core.TypeSelect, Secret: true, Required: false, Help: " raw field help ", Apply: core.ApplyLive, Options: []string{"second", "first"}, ReadOnly: true, Locked: true}, {Env: "a", Apply: ""}, {Env: "b", Apply: "z"}, {Env: "", Apply: core.ApplyLive}}}}} {
		p := &settingsReadProbe{sections: sections}
		svc := NewReads(p)
		out, err := svc.Schema(context.Background(), SchemaInput{})
		if err != nil || !reflect.DeepEqual(out.Sections, sections) || !reflect.DeepEqual(out.ReloadBoundaries, core.ReloadBoundaries(sections)) || !reflect.DeepEqual(p.calls, []string{"schema"}) {
			t.Fatal(out, err, p.calls)
		}
		raw, _ := json.Marshal(out)
		var root map[string]json.RawMessage
		json.Unmarshal(raw, &root)
		if sections == nil && string(root["sections"]) != "null" || sections != nil && len(sections) == 0 && string(root["sections"]) != "[]" {
			t.Fatal(string(raw))
		}
		if len(sections) == 0 && string(root["reload_boundaries"]) != "[]" {
			t.Fatal(string(raw))
		}
		if len(sections) > 0 {
			var rows []map[string]json.RawMessage
			json.Unmarshal(root["sections"], &rows)
			if len(rows[0]) != 6 {
				t.Fatal(string(raw))
			}
			var fields []map[string]json.RawMessage
			json.Unmarshal(rows[0]["fields"], &fields)
			if len(fields[0]) != 10 || string(fields[0]["required"]) != "false" || string(fields[0]["options"]) != `["second","first"]` {
				t.Fatal(string(raw))
			}
		}
		p.sections = []Section{{ID: "fresh", Fields: []core.Field{}}}
		ctx, cancel := context.WithCancel(context.Background())
		cancel()
		second, err := svc.Schema(ctx, SchemaInput{})
		if err != nil || second.Sections[0].ID != "fresh" {
			t.Fatal(second, err)
		}
	}
}
func TestSettingsReadsValuesPrivacyPriorityRawPresenceOrderAndFreshness(t *testing.T) {
	names := []string{" secret ", "missing-secret", "env", "fallback", "raw", "empty"}
	fields := []core.Field{{Env: names[0], Secret: true}, {Env: names[1], Secret: true}, {Env: names[2]}, {Env: names[3]}, {Env: names[4]}, {Env: names[5]}}
	p := &settingsReadProbe{sections: []Section{{Fields: fields}}, env: map[string]string{names[0]: "never-read", names[1]: "never-read", names[2]: " live raw ", names[4]: "   "}, stored: map[string]string{names[0]: "never-read", names[2]: "shadowed", names[3]: " stored raw "}, secret: map[string]bool{names[0]: true}, pinned: map[string]bool{names[0]: true, names[2]: true}}
	out, err := NewReads(p).Values(context.Background(), ValuesInput{})
	if err != nil || len(out.Fields) != 6 {
		t.Fatal(out, err)
	}
	rows := settingsRowsJSON(t, out.Fields)
	if len(rows) != 6 {
		t.Fatal(out)
	}
	for i, row := range rows {
		if row["env"] != names[i] || row["secret"] != (i < 2) || row["env_pinned"] != (i == 0 || i == 2) || len(row) != 4+boolInt(i >= 2) {
			t.Fatal(i, row)
		}
	}
	if rows[0]["set"] != true || rows[1]["set"] != false || rows[2]["value"] != " live raw " || rows[3]["value"] != " stored raw " || rows[4]["value"] != "   " || rows[4]["set"] != true || rows[5]["value"] != "" || rows[5]["set"] != false {
		t.Fatal(rows)
	}
	expected := []string{"prepare", "sections", "pin: secret ", "secret: secret ", "pin:missing-secret", "secret:missing-secret", "pin:env", "env:env", "pin:fallback", "env:fallback", "store:fallback", "pin:raw", "env:raw", "pin:empty", "env:empty", "store:empty"}
	if !reflect.DeepEqual(p.calls, expected) {
		t.Fatal(p.calls)
	}
	raw, _ := json.Marshal(out)
	if string(raw) == "" {
		t.Fatal("no output")
	}
	for _, row := range rows[:2] {
		if _, present := row["value"]; present {
			t.Fatal("secret value projected")
		}
	}
	rows[0]["env"] = "changed"
	p.env[names[2]] = "fresh"
	p.secret[names[0]] = false
	p.calls = nil
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	second, err := NewReads(p).Values(ctx, ValuesInput{})
	if err != nil || settingsRowsJSON(t, second.Fields)[0]["set"] != false || settingsRowsJSON(t, second.Fields)[2]["value"] != "fresh" || p.sections[0].Fields[0].Env != names[0] {
		t.Fatal(second, err)
	}
	for _, sections := range [][]Section{nil, {}, {{Fields: nil}}, {{Fields: []core.Field{}}}} {
		empty, err := NewReads(&settingsReadProbe{sections: sections}).Values(context.Background(), ValuesInput{})
		raw, _ := json.Marshal(empty)
		if err != nil || string(raw) != `{"fields":[]}` {
			t.Fatal(string(raw), err)
		}
	}
}
func boolInt(value bool) int {
	if value {
		return 1
	}
	return 0
}

func settingsRowsJSON(t *testing.T, rows []ValueRow) []map[string]any {
	t.Helper()
	raw, err := json.Marshal(rows)
	if err != nil {
		t.Fatal(err)
	}
	var out []map[string]any
	if err := json.Unmarshal(raw, &out); err != nil {
		t.Fatal(err)
	}
	return out
}
func TestSettingsTypedSchemaOwnsNestedCollectionsAndPreservesNilEmpty(t *testing.T) {
	p := &settingsReadProbe{sections: []Section{{ID: "owned", Fields: []core.Field{{Env: "owned", Options: []string{"second", "first"}}}}}}
	out, err := NewReads(p).Schema(context.Background(), SchemaInput{})
	if err != nil {
		t.Fatal(err)
	}
	out.Sections[0].ID = "changed"
	out.Sections[0].Fields[0].Env = "changed"
	out.Sections[0].Fields[0].Options[0] = "changed"
	if p.sections[0].ID != "owned" || p.sections[0].Fields[0].Env != "owned" || p.sections[0].Fields[0].Options[0] != "second" {
		t.Fatal("nested schema alias")
	}
	for _, fields := range [][]core.Field{nil, {}, {{Options: nil}}, {{Options: []string{}}}} {
		p.sections = []Section{{Fields: fields}}
		out, _ := NewReads(p).Schema(context.Background(), SchemaInput{})
		if !reflect.DeepEqual(out.Sections, p.sections) {
			t.Fatal(out, p.sections)
		}
	}
}
