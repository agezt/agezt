// SPDX-License-Identifier: MIT

package webui

import (
	"encoding/json"
	"net/http"
	"path/filepath"
	"reflect"
	"strings"
	"testing"

	"github.com/agezt/agezt/kernel/contract/opapi"
)

// The console answers each native route with the ROW the route names.
//
// The native routes declare their request side (the body schema) and their
// answer side (the row) in one place — `opapi.NativeRoute`. This ratchet says
// the two are the same answer: the console must not send a member the row
// does not name, and it must not drop a member the row always carries.
//
// It is two-way on purpose. A member the console invents is a second spelling
// of the row; a member the row declares and the console never sends is a row
// that promises more than the wire keeps.
func TestTheConsoleAnswersEachNativeRouteWithTheRowItNames(t *testing.T) {
	drivers := map[opapi.NativeRouteName]func(t *testing.T) []string{
		opapi.NativeAuthMeta: func(t *testing.T) []string {
			s, _ := newServer(t, &fakeCaller{}, "secret")
			return []string{httpJSON(t, s.Handler(), http.MethodGet, "/api/authmeta", "").Body.String()}
		},
		opapi.NativeLogin: func(t *testing.T) []string {
			s, _ := newServer(t, &fakeCaller{}, "secret")
			return []string{httpJSON(t, s.Handler(), http.MethodPost, "/api/login", "").Body.String()}
		},
		opapi.NativeLogout: func(t *testing.T) []string {
			s, _ := newServer(t, &fakeCaller{}, "secret")
			return []string{httpJSON(t, s.Handler(), http.MethodPost, "/api/logout", "").Body.String()}
		},
		opapi.NativeSSEToken: func(t *testing.T) []string {
			s, _ := newServer(t, &fakeCaller{}, "secret")
			return []string{httpJSON(t, s.Handler(), http.MethodGet, "/api/sse-token?token=secret", "").Body.String()}
		},
		opapi.NativeVoiceStatus: func(t *testing.T) []string {
			s, _ := newServer(t, &fakeCaller{}, "secret")
			return []string{httpJSON(t, s.Handler(), http.MethodGet, "/api/voice/status?token=secret", "").Body.String()}
		},
		opapi.NativeWorkflowHook: func(t *testing.T) []string {
			// The hook answers in one of two shapes: the run finished while the
			// caller waited, or the run was accepted and finishes in the
			// background. Both are the row the route names.
			reply := &fakeCaller{result: map[string]any{
				"workflow": "echoer", "correlation_id": "run-2",
				"executed": []any{"start"}, "outputs": map[string]any{"shape": "pong"},
			}}
			s, _ := newServer(t, reply, "secret")
			accepted := &fakeCaller{result: map[string]any{
				"accepted": true, "correlation_id": "run-1", "workflow": "hooked",
			}}
			sAccepted, _ := newServer(t, accepted, "secret")
			return []string{
				httpJSON(t, s.Handler(), http.MethodPost, "/hooks/echoer", `{}`).Body.String(),
				httpJSON(t, sAccepted.Handler(), http.MethodPost, "/hooks/hooked", `{}`).Body.String(),
			}
		},
		opapi.NativeFilesTree: func(t *testing.T) []string {
			// The file manager answers with the console's own row: the listing
			// it found under the configured root.
			withFileRoot(t, t.TempDir())
			s, _ := newServer(t, &fakeCaller{}, "secret")
			return []string{httpJSON(t, s.Handler(), http.MethodGet,
				"/api/files/tree?path=&token=secret", "").Body.String()}
		},
		opapi.NativeRollbackList: func(t *testing.T) []string {
			home := t.TempDir()
			t.Setenv("AGEZT_HOME", home)
			path := filepath.Join(home, filepath.FromSlash(rollbackCatalogRelativePath))
			if err := writeRollbackCatalogAt(path, rollbackCatalog{Checkpoints: []rollbackCheckpoint{
				{ID: "rb-a", Kind: rollbackCheckpointKindSkill, Action: "skill.set", RunID: "run-a",
					SubjectID: "s-1", SubjectName: "ops", BeforeStatus: "paused", CreatedMS: 100},
			}}); err != nil {
				t.Fatalf("write catalog: %v", err)
			}
			s, _ := newServer(t, &fakeCaller{}, "secret")
			return []string{httpJSON(t, s.Handler(), http.MethodGet,
				"/api/rollback/checkpoints?token=secret", "").Body.String()}
		},
		opapi.NativeRollbackApply: func(t *testing.T) []string {
			home := t.TempDir()
			t.Setenv("AGEZT_HOME", home)
			path := filepath.Join(home, filepath.FromSlash(rollbackCatalogRelativePath))
			if err := writeRollbackCatalogAt(path, rollbackCatalog{Checkpoints: []rollbackCheckpoint{
				{ID: "rb-a", Kind: rollbackCheckpointKindSkill, Action: "skill.set", RunID: "run-a",
					SubjectID: "s-1", SubjectName: "ops", BeforeStatus: "paused", CreatedMS: 100, AppliedMS: 500},
			}}); err != nil {
				t.Fatalf("write catalog: %v", err)
			}
			s, _ := newServer(t, &fakeCaller{}, "secret")
			return []string{httpJSON(t, s.Handler(), http.MethodPost,
				"/api/rollback/apply?token=secret", `{"id":"rb-a"}`).Body.String()}
		},
	}

	for name, driver := range drivers {
		var route *opapi.NativeRoute
		for _, candidate := range opapi.ConsoleNativeRoutes() {
			if candidate.Name == name {
				route = &candidate
				break
			}
		}
		if route == nil {
			t.Errorf("the native route %q is driven by this ratchet but the contract does not list it", name)
			continue
		}
		if route.Answer == nil {
			t.Errorf("the native route %q answers with the console's own row, but it names none — "+
				"the answer side of the route is declared in the contract", name)
			continue
		}
		row := reflect.TypeOf(route.Answer)
		for row.Kind() == reflect.Pointer {
			row = row.Elem()
		}
		always := map[string]bool{}
		optional := map[string]bool{}
		for i := 0; i < row.NumField(); i++ {
			name, opts, _ := strings.Cut(row.Field(i).Tag.Get("json"), ",")
			if name == "" || name == "-" {
				continue
			}
			if strings.Contains(opts, "omitempty") {
				optional[name] = true
				continue
			}
			always[name] = true
		}

		served := map[string]bool{}
		for _, body := range driver(t) {
			var answer map[string]any
			if err := json.Unmarshal([]byte(body), &answer); err != nil {
				t.Errorf("the native route %q answered a body that is not an object: %v", name, err)
				continue
			}
			for member := range answer {
				served[member] = true
				if always[member] || optional[member] {
					continue
				}
				t.Errorf("the native route %q answers the member %q, which the row it names does not carry",
					name, member)
			}
		}
		for member := range always {
			if served[member] {
				continue
			}
			t.Errorf("the native route %q does not answer the member %q, which the row it names always carries",
				name, member)
		}
	}

	for _, route := range opapi.ConsoleNativeRoutes() {
		if route.Answer == nil {
			continue
		}
		if _, ok := drivers[route.Name]; ok {
			continue
		}
		t.Errorf("the native route %q names the row it answers with, but nothing checks "+
			"that the console answers it — the route is unverified", route.Name)
	}
}
