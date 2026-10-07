// SPDX-License-Identifier: MIT
package channels

import (
	"context"
	"encoding/json"
	"reflect"
	"strings"
	"testing"

	"github.com/agezt/agezt/kernel/channel"
	"github.com/agezt/agezt/kernel/settings"
)

type inventoryProbe struct {
	sections                        []settings.Section
	manifests                       []channel.Manifest
	env, stored                     map[string]string
	secret, pinned, live, instances map[string]bool
	names                           []string
	calls                           []string
}

func (p *inventoryProbe) Prepare() InventoryValues { p.calls = append(p.calls, "prepare"); return p }
func (p *inventoryProbe) Sections() []settings.Section {
	p.calls = append(p.calls, "sections")
	return p.sections
}
func (p *inventoryProbe) EnvValue(key string) string {
	p.calls = append(p.calls, "env:"+key)
	return p.env[key]
}
func (p *inventoryProbe) SecretSet(key string) bool {
	p.calls = append(p.calls, "secret:"+key)
	return p.secret[key]
}
func (p *inventoryProbe) StoredValue(key string) (string, bool) {
	p.calls = append(p.calls, "store:"+key)
	value, ok := p.stored[key]
	return value, ok
}
func (p *inventoryProbe) EnvPinned(key string) bool { return p.pinned[key] }
func (p *inventoryProbe) Names() []string           { p.calls = append(p.calls, "names"); return p.names }
func (p *inventoryProbe) Manifests() []channel.Manifest {
	p.calls = append(p.calls, "manifests")
	return p.manifests
}
func (p *inventoryProbe) IsLive(key string) bool         { return p.live[key] }
func (p *inventoryProbe) IsLiveInstance(key string) bool { return p.instances[key] }
func inventoryField(t *testing.T, fields []map[string]any, name string) map[string]any {
	t.Helper()
	for _, field := range fields {
		if field["env"] == name {
			return field
		}
	}
	t.Fatal("field missing", name)
	return nil
}

func TestChannelInventoryPresencePrecedenceAccountsProbeAndSecretShape(t *testing.T) {
	p := &inventoryProbe{
		sections:  []settings.Section{{ID: "owned", Fields: []settings.Field{{Env: "PUB", Label: " raw label ", Required: true, Help: " raw help "}, {Env: "SEC", Label: "Secret", Secret: true}, {Env: "EMPTY", Label: "Empty"}}}},
		manifests: []channel.Manifest{{Kind: "owned-kind", Display: " raw display ", Description: " raw description ", Transport: " raw transport ", Duplex: true, Media: channel.MediaCaps{ImageIn: true, VoiceOut: true}, ConfigSection: "owned", RequiredEnv: []string{"PUB", "SEC"}, DocsURL: " raw docs ", SetupSteps: []string{}, ConnectMethod: " raw method "}},
		env:       map[string]string{"PUB": " live ", "PUB#work": " labelled env ", "SEC#env-only": "present-only"}, stored: map[string]string{"PUB": "stored shadow", "SEC": "never-display-this", "PUB#work": "shadow", "EMPTY": ""},
		secret: map[string]bool{"SEC#work": true}, pinned: map[string]bool{"PUB": true, "SEC": true}, names: []string{"PUB#work", "SEC#work", "PUB#alpha", "PUB#Bad Label", "UNRELATED#ignored", "PUB#work"},
		live: map[string]bool{"owned-kind": true}, instances: map[string]bool{"owned-kind#work": true},
	}
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	typed, err := NewInventory(p).List(ctx, ListInput{})
	out := inventoryJSONView(t, typed)
	if err != nil || len(out) != 4 || out["count"] != 1 {
		t.Fatal(out, err)
	}
	rows := out["channels"].([]map[string]any)
	row := rows[0]
	if len(row) != 15 || row["kind"] != "owned-kind" || row["display"] != " raw display " || row["description"] != " raw description " || row["transport"] != " raw transport " || row["docs_url"] != " raw docs " || row["duplex"] != true || row["connect_method"] != " raw method " || row["configured"] != true || row["live"] != true {
		t.Fatal(row)
	}
	fields := row["fields"].([]map[string]any)
	public := inventoryField(t, fields, "PUB")
	secret := inventoryField(t, fields, "SEC")
	empty := inventoryField(t, fields, "EMPTY")
	if len(public) != 8 || public["value"] != " live " || public["set"] != true || public["required"] != true || public["env_pinned"] != true || public["label"] != " raw label " || public["help"] != " raw help " {
		t.Fatal(public)
	}
	if len(secret) != 7 || secret["set"] != false || secret["env_pinned"] != true {
		t.Fatal("stored secret must configure without disclosing presence/value", secret)
	}
	if _, ok := secret["value"]; ok {
		t.Fatal("secret value member emitted")
	}
	if empty["value"] != "" || empty["set"] != false || empty["env_pinned"] != false {
		t.Fatal(empty)
	}
	accounts := row["accounts"].([]map[string]any)
	if len(accounts) != 3 || accounts[0]["label"] != "" || accounts[1]["label"] != "alpha" || accounts[2]["label"] != "work" {
		t.Fatal(accounts)
	}
	work := accounts[2]
	workFields := work["fields"].([]map[string]any)
	if len(work) != 5 || work["configured"] != true || work["live"] != true || inventoryField(t, workFields, "PUB")["value"] != " labelled env " || inventoryField(t, workFields, "PUB")["env_pinned"] != false || inventoryField(t, workFields, "SEC")["set"] != true {
		t.Fatal(work)
	}
	probe := row["probe"].(map[string]any)
	if len(probe) != 6 || probe["accounts"] != 3 || probe["configured_accounts"] != 2 || probe["live_accounts"] != 1 || probe["roundtrip_status"] != "ready" || probe["roundtrip_ready"] != true || probe["mode"] != "two_way" {
		t.Fatal(probe)
	}
	if !reflect.DeepEqual(out["probe_matrix"], map[string]any{"total": 1, "configured": 1, "live": 1, "roundtrip_ready": 1, "restart_needed": 0, "needs_setup": 0}) || !reflect.DeepEqual(out["media_matrix"], map[string]any{"image_in": 1, "image_out": 0, "voice_in": 0, "voice_out": 1}) {
		t.Fatal(out)
	}
	raw, _ := json.Marshal(out)
	if strings.Contains(string(raw), "never-display-this") || strings.Contains(string(raw), "present-only") || strings.Contains(string(raw), "env-only") {
		t.Fatal("inventory leaked/discovered env-only secret", string(raw))
	}
	if p.calls[0] != "prepare" || p.calls[1] != "sections" || p.calls[2] != "names" || p.calls[3] != "manifests" || p.calls[4] != "manifests" {
		t.Fatal("preparation/registry order", p.calls)
	}
	for _, call := range p.calls {
		if call == "store:PUB" {
			t.Fatal("live value did not shadow store")
		}
	}
	*typed.Channels[0].Fields[0].Value = "caller changed"
	p.env["PUB"] = "fresh"
	freshTyped, _ := NewInventory(p).List(context.Background(), ListInput{})
	fresh := inventoryJSONView(t, freshTyped)
	if fresh["channels"].([]map[string]any)[0]["fields"].([]map[string]any)[0]["value"] != "fresh" {
		t.Fatal("stale inventory")
	}
}

func TestChannelInventoryEmptyManifestsSectionsNilAndProbeMatrixBranches(t *testing.T) {
	for _, manifests := range [][]channel.Manifest{nil, {}} {
		typed, err := NewInventory(&inventoryProbe{manifests: manifests}).List(context.Background(), ListInput{})
		out := inventoryJSONView(t, typed)
		if err != nil || out["count"] != 0 || out["channels"] == nil || len(out["channels"].([]map[string]any)) != 0 {
			t.Fatal(out, err)
		}
		for _, matrix := range []string{"probe_matrix", "media_matrix"} {
			for _, value := range out[matrix].(map[string]any) {
				if value != 0 {
					t.Fatal(out)
				}
			}
		}
	}
	for _, duplex := range []bool{false, true} {
		for _, configured := range []bool{false, true} {
			for _, live := range []bool{false, true} {
				m := channel.Manifest{Duplex: duplex}
				typedProbe := channelAccountProbe(m, configured, live)
				probe := inventoryJSONView(t, typedProbe)
				status, note := "needs_setup", "required channel settings are missing"
				if live {
					status = "ready"
					note = "live outbound account can send a test message"
					if duplex {
						note = "live two-way account can receive and send a test reply"
					}
				} else if configured {
					status = "restart_required"
					note = "configured but not live in this daemon process"
				}
				mode := "outbound"
				if duplex {
					mode = "two_way"
				}
				if len(probe) != 6 || probe["configured"] != configured || probe["live"] != live || probe["roundtrip_ready"] != live || probe["roundtrip_status"] != status || probe["mode"] != mode || probe["note"] != note {
					t.Fatal(probe)
				}
				combinedTyped := channelProbe(m, []ChannelAccount{{Configured: configured, Live: live}})
				combined := inventoryJSONView(t, combinedTyped)
				typedMatrix := ProbeMatrix{}
				addChannelProbeTotals(&typedMatrix, combinedTyped)
				matrix := inventoryJSONView(t, typedMatrix)
				if combined["roundtrip_status"] != status || combined["roundtrip_ready"] != live || matrix["total"] != 1 {
					t.Fatal(combined, matrix)
				}
				key := "needs_setup"
				if live {
					key = "roundtrip_ready"
				} else if configured {
					key = "restart_needed"
				}
				if matrix[key] != 1 {
					t.Fatal(matrix)
				}
			}
		}
	}
	p := &inventoryProbe{manifests: []channel.Manifest{{Kind: "empty", ConfigSection: "none", Media: channel.MediaCaps{ImageOut: true, VoiceIn: true}}}, live: map[string]bool{"empty": true}}
	typed, _ := NewInventory(p).List(context.Background(), ListInput{})
	out := inventoryJSONView(t, typed)
	row := out["channels"].([]map[string]any)[0]
	if row["configured"] != true || row["live"] != true || row["probe"].(map[string]any)["roundtrip_status"] != "restart_required" || row["fields"] == nil || len(row["fields"].([]map[string]any)) != 0 {
		t.Fatal(row)
	}
	if !reflect.DeepEqual(out["media_matrix"], map[string]any{"image_in": 0, "image_out": 1, "voice_in": 1, "voice_out": 0}) {
		t.Fatal(out)
	}
}
