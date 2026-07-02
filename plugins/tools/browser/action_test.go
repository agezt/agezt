// SPDX-License-Identifier: MIT

package browser

import (
	"context"
	"encoding/json"
	"errors"
	"net"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/agezt/agezt/kernel/artifact"
)

func TestActionTool_InvokeRunsDriverAndNormalizesOutput(t *testing.T) {
	tool := NewAction("node", "/tmp/browse.mjs")
	tool.AllowAll = true
	tool.lookupIP = fakeLookup("93.184.216.34")
	var gotSpec actionRunSpec
	tool.run = func(_ context.Context, spec actionRunSpec) (actionRunOutput, error) {
		gotSpec = spec
		return actionRunOutput{Stdout: `{"ok":true,"url":"https://example.com/done","title":"Done","text":"hello"}`}, nil
	}

	res, err := tool.Invoke(context.Background(), mustRaw(t, map[string]any{
		"url": "https://example.com",
		"actions": []map[string]any{
			{"type": "click", "selector": "text=More"},
		},
	}))
	if err != nil {
		t.Fatalf("Invoke: %v", err)
	}
	if res.IsError {
		t.Fatalf("unexpected tool error: %s", res.Output)
	}
	if gotSpec.NodePath != "node" || gotSpec.DriverPath != "/tmp/browse.mjs" {
		t.Fatalf("driver spec not populated: %+v", gotSpec)
	}
	if !strings.Contains(res.Output, `"title": "Done"`) || !strings.Contains(res.Output, `"text": "hello"`) {
		t.Fatalf("normalized output missing fields:\n%s", res.Output)
	}
	if res.ObservationSource != "https://example.com/done" {
		t.Fatalf("ObservationSource=%q", res.ObservationSource)
	}
}

func TestActionTool_PassesParityOptionsToDriver(t *testing.T) {
	tool := NewAction("node", "/tmp/browse.mjs")
	tool.AllowAll = true
	tool.lookupIP = fakeLookup("93.184.216.34")
	var got actionInput
	tool.run = func(_ context.Context, spec actionRunSpec) (actionRunOutput, error) {
		if err := json.Unmarshal(spec.Spec, &got); err != nil {
			t.Fatalf("driver spec JSON: %v", err)
		}
		return actionRunOutput{Stdout: `{"ok":true,"url":"https://example.com","title":"Done","text":"ok","snapshot":[{"ref":"e1","selector":"#go"}],"events":{"console":[],"network":[]},"downloads":[{"path":"/tmp/file.txt"}]}`}, nil
	}

	res, err := tool.Invoke(context.Background(), mustRaw(t, map[string]any{
		"url":            "https://example.com",
		"screenshot":     true,
		"full_page":      true,
		"snapshot":       true,
		"snapshot_limit": 25,
		"events":         false,
		"event_limit":    7,
		"downloads":      false,
		"cookies":        true,
		"viewport":       map[string]any{"width": 1440, "height": 900},
		"actions": []map[string]any{
			{"type": "type", "selector": "#q", "value": "agezt", "delay_ms": 1},
			{"type": "select", "selector": "#kind", "value": "docs"},
			{"type": "hover", "selector": "#menu"},
			{"type": "scroll", "y": 400},
			{"type": "check", "selector": "#agree"},
			{"type": "uncheck", "selector": "#later"},
		},
	}))
	if err != nil {
		t.Fatalf("Invoke: %v", err)
	}
	if res.IsError {
		t.Fatalf("unexpected tool error: %s", res.Output)
	}
	if !got.FullPage || got.Snapshot == nil || !*got.Snapshot || got.SnapshotLimit != 25 {
		t.Fatalf("snapshot/full_page options not preserved: %+v", got)
	}
	if got.Events == nil || *got.Events || got.EventLimit != 7 || got.Downloads == nil || *got.Downloads {
		t.Fatalf("events/download options not preserved: %+v", got)
	}
	if !got.Cookies {
		t.Fatalf("cookies option not preserved: %+v", got)
	}
	if got.Viewport == nil || got.Viewport.Width != 1440 || got.Viewport.Height != 900 {
		t.Fatalf("viewport not preserved: %+v", got.Viewport)
	}
	if len(got.Actions) != 6 || got.Actions[0].Type != "type" || got.Actions[1].Type != "select" || got.Actions[3].Y != 400 {
		t.Fatalf("actions not preserved: %+v", got.Actions)
	}
	if !strings.Contains(res.Output, `"snapshot"`) || !strings.Contains(res.Output, `"downloads"`) {
		t.Fatalf("normalized output missing new driver fields:\n%s", res.Output)
	}
}

func TestActionVerbTools_TransformToActionInput(t *testing.T) {
	base := NewAction("node", "/tmp/browse.mjs")
	base.AllowAll = true
	base.SessionRoot = t.TempDir()
	base.lookupIP = fakeLookup("93.184.216.34")
	var got []actionInput
	base.run = func(_ context.Context, spec actionRunSpec) (actionRunOutput, error) {
		var in actionInput
		if err := json.Unmarshal(spec.Spec, &in); err != nil {
			t.Fatalf("driver spec JSON: %v", err)
		}
		got = append(got, in)
		return actionRunOutput{Stdout: `{"ok":true,"url":"https://example.com","title":"Done","text":"ok","snapshot":[{"ref":"e1","selector":"#go","role":"button","name":"Go"}]}`}, nil
	}

	tools := map[string]*ActionVerbTool{}
	for _, tool := range NewActionVerbTools(base) {
		vt := tool.(*ActionVerbTool)
		tools[vt.Name] = vt
	}

	if _, err := tools[ActionVerbSnapshot].Invoke(context.Background(), mustRaw(t, map[string]any{"url": "https://example.com"})); err != nil {
		t.Fatalf("snapshot Invoke: %v", err)
	}
	if got[0].Snapshot == nil || !*got[0].Snapshot || got[0].Screenshot == nil || *got[0].Screenshot {
		t.Fatalf("snapshot defaults = %+v", got[0])
	}

	if _, err := tools[ActionVerbClick].Invoke(context.Background(), mustRaw(t, map[string]any{
		"url":           "https://example.com",
		"selector":      "#go",
		"wait_selector": "#done",
	})); err != nil {
		t.Fatalf("click Invoke: %v", err)
	}
	if len(got[1].Actions) != 2 || got[1].Actions[0].Type != "click" || got[1].Actions[1].Type != "wait" {
		t.Fatalf("click actions = %+v", got[1].Actions)
	}

	if _, err := tools[ActionVerbType].Invoke(context.Background(), mustRaw(t, map[string]any{
		"url":      "https://example.com",
		"selector": "#q",
		"value":    "hello",
		"submit":   true,
	})); err != nil {
		t.Fatalf("type Invoke: %v", err)
	}
	if len(got[2].Actions) != 2 || got[2].Actions[0].Type != "type" || got[2].Actions[1].Type != "press" || got[2].Actions[1].Key != "Enter" {
		t.Fatalf("type actions = %+v", got[2].Actions)
	}

	if _, err := tools[ActionVerbOpen].Invoke(context.Background(), mustRaw(t, map[string]any{
		"url":        "https://example.com",
		"profile":    "session",
		"session_id": "work",
		"tab_id":     "main",
	})); err != nil {
		t.Fatalf("open Invoke with session: %v", err)
	}
	if got[3].Profile != "session" || got[3].SessionID != "work" || got[3].TabID != "main" || got[3].UserDataDir == "" {
		t.Fatalf("session profile not passed through wrapper: %+v", got[3])
	}

	if _, err := tools[ActionVerbClick].Invoke(context.Background(), mustRaw(t, map[string]any{
		"session_id": "work",
		"tab_id":     "main",
		"ref":        "e1",
	})); err != nil {
		t.Fatalf("click Invoke with ref: %v", err)
	}
	if got[4].URL != "https://example.com" || len(got[4].Actions) != 1 || got[4].Actions[0].Selector != "#go" {
		t.Fatalf("ref was not resolved into selector action: %+v", got[4])
	}

	if _, err := tools[ActionVerbCookies].Invoke(context.Background(), mustRaw(t, map[string]any{
		"url": "https://example.com",
	})); err != nil {
		t.Fatalf("cookies Invoke: %v", err)
	}
	if !got[5].Cookies || got[5].Screenshot == nil || *got[5].Screenshot || got[5].Snapshot == nil || *got[5].Snapshot {
		t.Fatalf("cookies wrapper not converted correctly: %+v", got[5])
	}
}

func TestActionVerbTools_ValidateRequiredFields(t *testing.T) {
	base := NewAction("node", "/tmp/browse.mjs")
	base.AllowAll = true
	base.lookupIP = fakeLookup("93.184.216.34")
	base.run = func(context.Context, actionRunSpec) (actionRunOutput, error) {
		t.Fatal("driver must not run for invalid wrapper input")
		return actionRunOutput{}, nil
	}
	cases := []struct {
		name string
		in   map[string]any
	}{
		{ActionVerbClick, map[string]any{"url": "https://example.com"}},
		{ActionVerbType, map[string]any{"url": "https://example.com", "selector": "#q"}},
	}
	for _, tc := range cases {
		tool := &ActionVerbTool{Name: tc.name, Base: base}
		res, err := tool.Invoke(context.Background(), mustRaw(t, tc.in))
		if err != nil {
			t.Fatalf("%s Invoke: %v", tc.name, err)
		}
		if !res.IsError {
			t.Fatalf("%s should be rejected, got %q", tc.name, res.Output)
		}
	}
}

func TestActionTool_ProfilePolicyBlocksUnsafeModesByDefault(t *testing.T) {
	tool := NewAction("node", "/tmp/browse.mjs")
	tool.AllowAll = true
	tool.AllowLoopback = true
	tool.lookupIP = fakeLookup("127.0.0.1")
	tool.run = func(context.Context, actionRunSpec) (actionRunOutput, error) {
		t.Fatal("driver must not run for blocked profile modes")
		return actionRunOutput{}, nil
	}
	for _, profile := range []string{"user-attached", "remote-cdp"} {
		res, err := tool.Invoke(context.Background(), mustRaw(t, map[string]any{
			"url":     "http://localhost",
			"profile": profile,
		}))
		if err != nil {
			t.Fatalf("Invoke %s: %v", profile, err)
		}
		if !res.IsError || !strings.Contains(res.Output, "disabled") {
			t.Fatalf("profile %s should be disabled, got error=%v output=%q", profile, res.IsError, res.Output)
		}
	}
}

func TestActionTool_ProfilePolicyInjectsOperatorConfig(t *testing.T) {
	tool := NewAction("node", "/tmp/browse.mjs")
	tool.AllowAll = true
	tool.AllowLoopback = true
	tool.AllowUserProfile = true
	tool.UserDataDir = filepath.Join(t.TempDir(), "profile")
	tool.AllowRemoteCDP = true
	tool.RemoteCDPURL = "http://localhost:9222"
	tool.lookupIP = fakeLookup("127.0.0.1")
	var got []actionInput
	tool.run = func(_ context.Context, spec actionRunSpec) (actionRunOutput, error) {
		var in actionInput
		if err := json.Unmarshal(spec.Spec, &in); err != nil {
			t.Fatalf("driver spec JSON: %v", err)
		}
		got = append(got, in)
		return actionRunOutput{Stdout: `{"ok":true,"url":"http://localhost","text":"ok"}`}, nil
	}

	for _, profile := range []string{"user-attached", "remote-cdp"} {
		res, err := tool.Invoke(context.Background(), mustRaw(t, map[string]any{
			"url":     "http://localhost",
			"profile": profile,
		}))
		if err != nil {
			t.Fatalf("Invoke %s: %v", profile, err)
		}
		if res.IsError {
			t.Fatalf("profile %s should be allowed: %s", profile, res.Output)
		}
	}
	if got[0].Profile != "user-attached" || got[0].UserDataDir == "" || got[0].RemoteCDPURL != "" {
		t.Fatalf("user profile spec not injected correctly: %+v", got[0])
	}
	if got[1].Profile != "remote-cdp" || got[1].RemoteCDPURL != "http://localhost:9222" || got[1].UserDataDir != "" {
		t.Fatalf("remote-cdp spec not injected correctly: %+v", got[1])
	}
}

func TestActionTool_ProfileSessionInjectsManagedDir(t *testing.T) {
	tool := NewAction("node", "/tmp/browse.mjs")
	tool.AllowAll = true
	tool.SessionRoot = t.TempDir()
	tool.lookupIP = fakeLookup("93.184.216.34")
	var got actionInput
	tool.run = func(_ context.Context, spec actionRunSpec) (actionRunOutput, error) {
		if err := json.Unmarshal(spec.Spec, &got); err != nil {
			t.Fatalf("driver spec JSON: %v", err)
		}
		return actionRunOutput{Stdout: `{"ok":true,"url":"https://example.com","text":"ok"}`}, nil
	}

	res, err := tool.Invoke(context.Background(), mustRaw(t, map[string]any{
		"url":        "https://example.com",
		"profile":    "session",
		"session_id": "work-1",
	}))
	if err != nil {
		t.Fatalf("Invoke: %v", err)
	}
	if res.IsError {
		t.Fatalf("profile=session should be allowed: %s", res.Output)
	}
	rootAbs, err := filepath.Abs(tool.SessionRoot)
	if err != nil {
		t.Fatal(err)
	}
	if got.Profile != "session" || got.SessionID != "work-1" || got.RemoteCDPURL != "" {
		t.Fatalf("session profile spec not injected correctly: %+v", got)
	}
	if rel, err := filepath.Rel(rootAbs, got.UserDataDir); err != nil || rel != "work-1" {
		t.Fatalf("session user_data_dir = %q, rel=%q err=%v", got.UserDataDir, rel, err)
	}
}

func TestActionTool_ProfileSessionRejectsInvalidSessionID(t *testing.T) {
	tool := NewAction("node", "/tmp/browse.mjs")
	tool.AllowAll = true
	tool.SessionRoot = t.TempDir()
	tool.lookupIP = fakeLookup("93.184.216.34")
	tool.run = func(context.Context, actionRunSpec) (actionRunOutput, error) {
		t.Fatal("driver must not run for invalid session id")
		return actionRunOutput{}, nil
	}
	for _, id := range []string{"", "..", "../x", ".hidden", "bad/slash", `bad\slash`, strings.Repeat("a", 81)} {
		res, err := tool.Invoke(context.Background(), mustRaw(t, map[string]any{
			"url":        "https://example.com",
			"profile":    "session",
			"session_id": id,
		}))
		if err != nil {
			t.Fatalf("Invoke %q: %v", id, err)
		}
		if !res.IsError || !strings.Contains(res.Output, "session_id") {
			t.Fatalf("session_id %q should be rejected, got error=%v output=%q", id, res.IsError, res.Output)
		}
	}
}

func TestActionTool_ProfileSessionRequiresRoot(t *testing.T) {
	tool := NewAction("node", "/tmp/browse.mjs")
	tool.AllowAll = true
	tool.lookupIP = fakeLookup("93.184.216.34")
	tool.run = func(context.Context, actionRunSpec) (actionRunOutput, error) {
		t.Fatal("driver must not run when session root is missing")
		return actionRunOutput{}, nil
	}

	res, err := tool.Invoke(context.Background(), mustRaw(t, map[string]any{
		"url":        "https://example.com",
		"profile":    "session",
		"session_id": "work",
	}))
	if err != nil {
		t.Fatalf("Invoke: %v", err)
	}
	if !res.IsError || !strings.Contains(res.Output, "session root") {
		t.Fatalf("expected missing session root error, got %q", res.Output)
	}
}

func TestActionTool_TabIDPersistsAndResolvesURL(t *testing.T) {
	tool := NewAction("node", "/tmp/browse.mjs")
	tool.AllowAll = true
	tool.SessionRoot = t.TempDir()
	tool.Now = func() int64 { return 42 }
	tool.lookupIP = fakeLookup("93.184.216.34")
	var got []actionInput
	tool.run = func(_ context.Context, spec actionRunSpec) (actionRunOutput, error) {
		var in actionInput
		if err := json.Unmarshal(spec.Spec, &in); err != nil {
			t.Fatalf("driver spec JSON: %v", err)
		}
		got = append(got, in)
		if len(got) == 1 {
			return actionRunOutput{Stdout: `{"ok":true,"url":"https://example.com/dashboard","title":"Dash","text":"opened","snapshot":[{"ref":"e1","selector":"#continue","role":"button","name":"Continue"}]}`}, nil
		}
		return actionRunOutput{Stdout: `{"ok":true,"url":"https://example.com/dashboard?clicked=1","title":"Dash","text":"clicked","snapshot":[{"ref":"e2","selector":"#done","role":"button","name":"Done"}]}`}, nil
	}

	res, err := tool.Invoke(context.Background(), mustRaw(t, map[string]any{
		"url":        "https://example.com/login",
		"profile":    "session",
		"session_id": "work",
		"tab_id":     "main",
	}))
	if err != nil {
		t.Fatalf("open Invoke: %v", err)
	}
	if res.IsError {
		t.Fatalf("open should succeed: %s", res.Output)
	}
	if !strings.Contains(res.Output, `"tab_id": "main"`) || !strings.Contains(res.Output, `"live": false`) {
		t.Fatalf("tab ref missing from output:\n%s", res.Output)
	}

	res, err = tool.Invoke(context.Background(), mustRaw(t, map[string]any{
		"session_id": "work",
		"tab_id":     "main",
		"actions":    []map[string]any{{"type": "click", "selector": "#continue"}},
	}))
	if err != nil {
		t.Fatalf("follow-up Invoke: %v", err)
	}
	if res.IsError {
		t.Fatalf("follow-up should succeed: %s", res.Output)
	}
	if len(got) != 2 {
		t.Fatalf("driver run count = %d, want 2", len(got))
	}
	if got[1].Profile != "session" || got[1].URL != "https://example.com/dashboard" || got[1].TabID != "main" {
		t.Fatalf("tab URL not resolved into follow-up spec: %+v", got[1])
	}
	statePath, err := tool.tabStatePath("work", "main")
	if err != nil {
		t.Fatal(err)
	}
	data, err := os.ReadFile(statePath)
	if err != nil {
		t.Fatalf("read tab state: %v", err)
	}
	if !strings.Contains(string(data), `https://example.com/dashboard?clicked=1`) || !strings.Contains(string(data), `"updated_ms": 42`) || !strings.Contains(string(data), `"ref": "e2"`) {
		t.Fatalf("tab state not updated:\n%s", data)
	}
}

func TestActionVerbRefRejectsStaleRef(t *testing.T) {
	root := t.TempDir()
	tool := &ActionVerbTool{Name: ActionVerbClick, Base: &ActionTool{SessionRoot: root}}
	stateDir := filepath.Join(root, "work", ".agezt-tabs")
	if err := os.MkdirAll(stateDir, 0o700); err != nil {
		t.Fatal(err)
	}
	state := actionTabState{
		URL:       "https://example.com",
		UpdatedMS: 1,
		Snapshot:  []actionSnapshotRef{{Ref: "e1", Selector: "#go"}},
	}
	data, err := json.Marshal(state)
	if err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(stateDir, "main.json"), data, 0o600); err != nil {
		t.Fatal(err)
	}

	res, err := tool.Invoke(context.Background(), mustRaw(t, map[string]any{
		"session_id": "work",
		"tab_id":     "main",
		"ref":        "e9",
	}))
	if err != nil {
		t.Fatalf("Invoke: %v", err)
	}
	if !res.IsError || !strings.Contains(res.Output, "fresh ref") {
		t.Fatalf("expected stale ref error, got error=%v output=%q", res.IsError, res.Output)
	}
}

func TestActionTool_TabIDRejectsMissingOrInvalidState(t *testing.T) {
	tool := NewAction("node", "/tmp/browse.mjs")
	tool.AllowAll = true
	tool.SessionRoot = t.TempDir()
	tool.lookupIP = fakeLookup("93.184.216.34")
	tool.run = func(context.Context, actionRunSpec) (actionRunOutput, error) {
		t.Fatal("driver must not run when tab state is invalid")
		return actionRunOutput{}, nil
	}
	cases := []struct {
		name string
		in   map[string]any
		want string
	}{
		{
			name: "missing state",
			in:   map[string]any{"profile": "session", "session_id": "work", "tab_id": "missing"},
			want: "has no saved URL",
		},
		{
			name: "invalid tab id",
			in:   map[string]any{"url": "https://example.com", "profile": "session", "session_id": "work", "tab_id": "../x"},
			want: "tab_id",
		},
		{
			name: "wrong profile",
			in:   map[string]any{"url": "https://example.com", "profile": "isolated", "tab_id": "main"},
			want: "profile=session",
		},
	}
	for _, tc := range cases {
		res, err := tool.Invoke(context.Background(), mustRaw(t, tc.in))
		if err != nil {
			t.Fatalf("%s Invoke: %v", tc.name, err)
		}
		if !res.IsError || !strings.Contains(res.Output, tc.want) {
			t.Fatalf("%s: expected %q error, got error=%v output=%q", tc.name, tc.want, res.IsError, res.Output)
		}
	}
}

func TestActionVerbCloseRemovesManagedSession(t *testing.T) {
	root := t.TempDir()
	sessionDir := filepath.Join(root, "work")
	if err := os.MkdirAll(sessionDir, 0o700); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(sessionDir, "Cookies"), []byte("cookie"), 0o600); err != nil {
		t.Fatal(err)
	}
	tool := &ActionVerbTool{Name: ActionVerbClose, Base: &ActionTool{SessionRoot: root}}

	res, err := tool.Invoke(context.Background(), mustRaw(t, map[string]any{"session_id": "work"}))
	if err != nil {
		t.Fatalf("Invoke: %v", err)
	}
	if res.IsError {
		t.Fatalf("close should succeed: %s", res.Output)
	}
	if _, err := os.Stat(sessionDir); !errors.Is(err, os.ErrNotExist) {
		t.Fatalf("session dir still exists or stat failed differently: %v", err)
	}
	if !strings.Contains(res.Output, `"closed": true`) || !strings.Contains(res.Output, `"session_id": "work"`) {
		t.Fatalf("close output missing fields: %s", res.Output)
	}
}

func TestActionVerbTabsListAndCloseTab(t *testing.T) {
	root := t.TempDir()
	base := &ActionTool{SessionRoot: root}
	if err := base.saveTabState("work", "main", "https://example.com/main", []actionSnapshotRef{{Ref: "e1", Selector: "#main"}}); err != nil {
		t.Fatalf("save main tab: %v", err)
	}
	if err := base.saveTabState("work", "reports", "https://example.com/reports", nil); err != nil {
		t.Fatalf("save reports tab: %v", err)
	}
	tabs := &ActionVerbTool{Name: ActionVerbTabs, Base: base}
	res, err := tabs.Invoke(context.Background(), mustRaw(t, map[string]any{"session_id": "work"}))
	if err != nil {
		t.Fatalf("tabs Invoke: %v", err)
	}
	if res.IsError || !strings.Contains(res.Output, `"tab_id": "main"`) || !strings.Contains(res.Output, `"refs": 1`) || !strings.Contains(res.Output, `"tab_id": "reports"`) {
		t.Fatalf("tabs output unexpected error=%v:\n%s", res.IsError, res.Output)
	}

	closeTab := &ActionVerbTool{Name: ActionVerbClose, Base: base}
	res, err = closeTab.Invoke(context.Background(), mustRaw(t, map[string]any{"session_id": "work", "tab_id": "main"}))
	if err != nil {
		t.Fatalf("close tab Invoke: %v", err)
	}
	if res.IsError || !strings.Contains(res.Output, `"tab_id": "main"`) {
		t.Fatalf("close tab output unexpected error=%v:\n%s", res.IsError, res.Output)
	}
	if _, err := base.readTabState("work", "main"); err == nil {
		t.Fatal("main tab state should be removed")
	}
	if _, err := base.readTabState("work", "reports"); err != nil {
		t.Fatalf("reports tab state should remain: %v", err)
	}
}

func TestActionTool_ProfilePolicyValidatesRemoteCDPEgress(t *testing.T) {
	tool := NewAction("node", "/tmp/browse.mjs")
	tool.AllowAll = true
	tool.AllowRemoteCDP = true
	tool.RemoteCDPURL = "http://localhost:9222"
	tool.lookupIP = func(_ context.Context, _ string, host string) ([]net.IP, error) {
		if host == "example.com" {
			return []net.IP{net.ParseIP("93.184.216.34")}, nil
		}
		return []net.IP{net.ParseIP("127.0.0.1")}, nil
	}
	tool.run = func(context.Context, actionRunSpec) (actionRunOutput, error) {
		t.Fatal("driver must not run when remote CDP egress is blocked")
		return actionRunOutput{}, nil
	}

	res, err := tool.Invoke(context.Background(), mustRaw(t, map[string]any{
		"url":     "https://example.com",
		"profile": "remote-cdp",
	}))
	if err != nil {
		t.Fatalf("Invoke: %v", err)
	}
	if !res.IsError || !strings.Contains(res.Output, "remote-cdp egress blocked") {
		t.Fatalf("expected remote-cdp egress block, got %q", res.Output)
	}
}

func TestActionVerbTools_DefinitionsHaveValidSchemas(t *testing.T) {
	base := NewAction("node", "/tmp/browse.mjs")
	for _, tool := range NewActionVerbTools(base) {
		def := tool.Definition()
		var schema map[string]any
		if err := json.Unmarshal(def.InputSchema, &schema); err != nil {
			t.Fatalf("%s schema is invalid JSON: %v\n%s", def.Name, err, string(def.InputSchema))
		}
		if def.Effect.Class == "" || len(def.Effect.PredictedEffects) == 0 {
			t.Fatalf("%s missing effect metadata", def.Name)
		}
	}
}

func TestActionTool_AttachesScreenshotAndDownloadArtifacts(t *testing.T) {
	tool := NewAction("node", "/tmp/browse.mjs")
	tool.AllowAll = true
	tool.lookupIP = fakeLookup("93.184.216.34")
	idx := &fakeActionIndex{}
	tool.SetIndex(idx)
	tool.Now = func() int64 { return 1234 }

	shotDir, err := os.MkdirTemp("", "browseruse-")
	if err != nil {
		t.Fatalf("MkdirTemp screenshot: %v", err)
	}
	t.Cleanup(func() { _ = os.RemoveAll(shotDir) })
	downloadDir, err := os.MkdirTemp("", "browseruse-downloads-")
	if err != nil {
		t.Fatalf("MkdirTemp downloads: %v", err)
	}
	t.Cleanup(func() { _ = os.RemoveAll(downloadDir) })
	shot := filepath.Join(shotDir, "page.png")
	download := filepath.Join(downloadDir, "report.csv")
	if err := os.WriteFile(shot, []byte("\x89PNG\r\n\x1a\nshot"), 0o600); err != nil {
		t.Fatalf("write screenshot: %v", err)
	}
	if err := os.WriteFile(download, []byte("a,b\n1,2\n"), 0o600); err != nil {
		t.Fatalf("write download: %v", err)
	}

	tool.run = func(context.Context, actionRunSpec) (actionRunOutput, error) {
		stdout, _ := json.Marshal(map[string]any{
			"ok":         true,
			"url":        "https://example.com",
			"title":      "Done",
			"text":       "ok",
			"screenshot": shot,
			"downloads": []map[string]any{
				{"suggested_filename": "report.csv", "path": download},
			},
		})
		return actionRunOutput{Stdout: string(stdout)}, nil
	}

	res, err := tool.Invoke(context.Background(), mustRaw(t, map[string]any{"url": "https://example.com"}))
	if err != nil {
		t.Fatalf("Invoke: %v", err)
	}
	if res.IsError {
		t.Fatalf("unexpected tool error: %s", res.Output)
	}
	if len(idx.entries) != 2 {
		t.Fatalf("saved artifact count = %d, want 2", len(idx.entries))
	}
	if !strings.Contains(res.Output, `"screenshot_artifact"`) || !strings.Contains(res.Output, `"artifact"`) {
		t.Fatalf("artifact references missing from output:\n%s", res.Output)
	}
	if idx.entries[0].Kind != "image" || idx.entries[0].Source != "browser.action" {
		t.Fatalf("screenshot artifact metadata = %+v", idx.entries[0])
	}
	if idx.entries[1].Kind != "download" || idx.entries[1].Name != "report.csv" {
		t.Fatalf("download artifact metadata = %+v", idx.entries[1])
	}
}

func TestActionTool_RefusesArtifactPathOutsideBrowserTemp(t *testing.T) {
	tool := NewAction("node", "/tmp/browse.mjs")
	tool.SetIndex(&fakeActionIndex{})
	path := filepath.Join(t.TempDir(), "page.png")
	if err := os.WriteFile(path, []byte("not a browser temp file"), 0o600); err != nil {
		t.Fatal(err)
	}
	out := tool.attachArtifacts(`{"ok":true,"url":"https://example.com","screenshot":"` + strings.ReplaceAll(path, `\`, `\\`) + `"}`)
	if !strings.Contains(out, "screenshot_artifact_error") {
		t.Fatalf("expected artifact path refusal, got:\n%s", out)
	}
}

func TestActionTool_BlocksLoopbackByDefault(t *testing.T) {
	tool := NewAction("node", "/tmp/browse.mjs")
	tool.AllowAll = true
	tool.lookupIP = fakeLookup("127.0.0.1")
	tool.run = func(context.Context, actionRunSpec) (actionRunOutput, error) {
		t.Fatal("driver must not run for blocked egress")
		return actionRunOutput{}, nil
	}

	res, err := tool.Invoke(context.Background(), mustRaw(t, map[string]any{"url": "http://localhost"}))
	if err != nil {
		t.Fatalf("Invoke: %v", err)
	}
	if !res.IsError || !strings.Contains(res.Output, "egress blocked") {
		t.Fatalf("expected egress block, got error=%v output=%q", res.IsError, res.Output)
	}
}

func TestActionTool_AllowsLoopbackWhenConfigured(t *testing.T) {
	tool := NewAction("node", "/tmp/browse.mjs")
	tool.AllowAll = true
	tool.AllowLoopback = true
	tool.lookupIP = fakeLookup("127.0.0.1")
	tool.run = func(context.Context, actionRunSpec) (actionRunOutput, error) {
		return actionRunOutput{Stdout: `{"ok":true,"url":"http://localhost","text":"ok"}`}, nil
	}

	res, err := tool.Invoke(context.Background(), mustRaw(t, map[string]any{"url": "http://localhost"}))
	if err != nil {
		t.Fatalf("Invoke: %v", err)
	}
	if res.IsError {
		t.Fatalf("loopback should be allowed with opt-in: %s", res.Output)
	}
}

func TestActionTool_RequiresAllowedHost(t *testing.T) {
	tool := NewAction("node", "/tmp/browse.mjs")
	tool.AllowedHosts = []string{"allowed.example"}
	tool.lookupIP = fakeLookup("93.184.216.34")

	res, err := tool.Invoke(context.Background(), mustRaw(t, map[string]any{"url": "https://blocked.example"}))
	if err != nil {
		t.Fatalf("Invoke: %v", err)
	}
	if !res.IsError || !strings.Contains(res.Output, "host not in allowlist") {
		t.Fatalf("expected allowlist denial, got %q", res.Output)
	}
}

func TestActionTool_ValidatesGotoActionURL(t *testing.T) {
	tool := NewAction("node", "/tmp/browse.mjs")
	tool.AllowAll = true
	tool.lookupIP = func(_ context.Context, _ string, host string) ([]net.IP, error) {
		if host == "example.com" {
			return []net.IP{net.ParseIP("93.184.216.34")}, nil
		}
		return []net.IP{net.ParseIP("127.0.0.1")}, nil
	}
	tool.run = func(context.Context, actionRunSpec) (actionRunOutput, error) {
		t.Fatal("driver must not run when goto points at loopback")
		return actionRunOutput{}, nil
	}

	res, err := tool.Invoke(context.Background(), mustRaw(t, map[string]any{
		"url":     "https://example.com",
		"actions": []map[string]any{{"type": "goto", "url": "http://internal.test"}},
	}))
	if err != nil {
		t.Fatalf("Invoke: %v", err)
	}
	if !res.IsError || !strings.Contains(res.Output, "action 0") || !strings.Contains(res.Output, "egress blocked") {
		t.Fatalf("expected action URL egress denial, got %q", res.Output)
	}
}

func TestActionTool_RejectsInvalidActions(t *testing.T) {
	tool := NewAction("node", "/tmp/browse.mjs")
	tool.AllowAll = true
	tool.lookupIP = fakeLookup("93.184.216.34")
	cases := []map[string]any{
		{"type": "click"},
		{"type": "wait"},
		{"type": "type"},
		{"type": "select", "selector": "#x"},
		{"type": "scroll"},
		{"type": "wat"},
	}
	for _, action := range cases {
		res, err := tool.Invoke(context.Background(), mustRaw(t, map[string]any{
			"url":     "https://example.com",
			"actions": []map[string]any{action},
		}))
		if err != nil {
			t.Fatalf("Invoke: %v", err)
		}
		if !res.IsError {
			t.Fatalf("action %+v should be rejected, got %q", action, res.Output)
		}
	}
}

func TestActionTool_RejectsInvalidOptions(t *testing.T) {
	tool := NewAction("node", "/tmp/browse.mjs")
	tool.AllowAll = true
	tool.lookupIP = fakeLookup("93.184.216.34")
	cases := []map[string]any{
		{"url": "https://example.com", "timeout_ms": -1},
		{"url": "https://example.com", "max_chars": -1},
		{"url": "https://example.com", "snapshot_limit": -1},
		{"url": "https://example.com", "event_limit": -1},
		{"url": "https://example.com", "viewport": map[string]any{"width": -1}},
		{"url": "https://example.com", "actions": []map[string]any{{"type": "wait", "ms": -1}}},
		{"url": "https://example.com", "actions": []map[string]any{{"type": "type", "selector": "#q", "delay_ms": -1}}},
	}
	for _, in := range cases {
		res, err := tool.Invoke(context.Background(), mustRaw(t, in))
		if err != nil {
			t.Fatalf("Invoke: %v", err)
		}
		if !res.IsError {
			t.Fatalf("input %+v should be rejected, got %q", in, res.Output)
		}
	}
}

func TestNormalizeActionOutput_TruncatesText(t *testing.T) {
	text := strings.Repeat("a", 20)
	out, _, err := normalizeActionOutput(`{"ok":true,"url":"https://example.com","text":"`+text+`"}`, 8)
	if err != nil {
		t.Fatalf("normalizeActionOutput: %v", err)
	}
	if !strings.Contains(out, `"truncated_text": true`) || !strings.Contains(out, `...[truncated]`) {
		t.Fatalf("expected truncation marker:\n%s", out)
	}
}

func TestActionTool_DriverFailureIsToolError(t *testing.T) {
	tool := NewAction("node", "/tmp/browse.mjs")
	tool.AllowAll = true
	tool.lookupIP = fakeLookup("93.184.216.34")
	tool.run = func(context.Context, actionRunSpec) (actionRunOutput, error) {
		return actionRunOutput{Stderr: "playwright missing"}, errors.New("exit status 1")
	}

	res, err := tool.Invoke(context.Background(), mustRaw(t, map[string]any{"url": "https://example.com"}))
	if err != nil {
		t.Fatalf("Invoke: %v", err)
	}
	if !res.IsError || !strings.Contains(res.Output, "playwright missing") {
		t.Fatalf("driver failure should be surfaced as tool error, got %q", res.Output)
	}
}

func TestNewAction_DisabledWithoutDriver(t *testing.T) {
	if NewAction("node", "") != nil {
		t.Fatal("blank driver path should disable browser.action")
	}
}

func fakeLookup(ip string) func(context.Context, string, string) ([]net.IP, error) {
	return func(context.Context, string, string) ([]net.IP, error) {
		return []net.IP{net.ParseIP(ip)}, nil
	}
}

func mustRaw(t *testing.T, v any) json.RawMessage {
	t.Helper()
	b, err := json.Marshal(v)
	if err != nil {
		t.Fatal(err)
	}
	return b
}

type fakeActionIndex struct {
	entries []artifact.Entry
	data    [][]byte
}

func (f *fakeActionIndex) PutEntry(meta artifact.Entry, data []byte, createdMs int64) (artifact.Entry, error) {
	meta.ID = "art-test-" + string(rune('a'+len(f.entries)))
	meta.Ref = "ref-" + meta.ID
	meta.Size = int64(len(data))
	meta.CreatedMs = createdMs
	f.entries = append(f.entries, meta)
	f.data = append(f.data, append([]byte(nil), data...))
	return meta, nil
}
