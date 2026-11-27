// SPDX-License-Identifier: MIT

package webui

import (
	"os"
	"path/filepath"
	"reflect"
	"regexp"
	"runtime"
	"sort"
	"strings"
	"testing"

	"github.com/agezt/agezt/kernel/contract/consoleapi"
	"github.com/agezt/agezt/kernel/contract/rollbackapi"
)

// The console spells no CONTRACT-OWNED row by hand.
//
// The console's PROXIED answers are typed already — they come from the app
// layer through the operation registry. The console's OWN answers are the
// rows the contracts own: the rollback catalogue's checkpoints and the
// envelopes around them, and the console's own rows — the auth meta, the
// login, the voice status, the workflow hook's answers and the stream's
// frames.
//
// A hand-written map is how a row got two homes in the first place: the
// contract named the fields, the surface named them again, and nothing said
// the two lists were the same list. So the console is measured: a map literal
// that spells the field names of a contract-owned row is a violation, and a
// declared exception that no longer matches is a violation too (a stale
// exception is a lie about the code).
//
// The floor is TWO shared field names — the same floor the REST surface's
// guard uses. The console's operation ARGUMENTS share two names with the
// checkpoint row ("id", "reason"), and they are not answers: the one that
// does is declared below, with the reason.

// contractOwnedRows are the rows the contracts own, by name.
//
// Floor is the number of shared field names that makes a map literal a
// violation. It is TWO for every row — two names in common are a coincidence
// of English — and ONE for the error envelope, which carries a single member:
// a refusal that spelled its own map would be a second spelling of the row,
// and a one-member row would never be seen by a floor of two.
var contractOwnedRows = []struct {
	Name  string
	Row   reflect.Type
	Floor int
}{
	{"rollback_checkpoint", reflect.TypeOf(rollbackapi.Checkpoint{}), 0},
	{"rollback_list", reflect.TypeOf(rollbackapi.List{}), 0},
	{"rollback_applied", reflect.TypeOf(rollbackapi.Applied{}), 0},
	{"console_auth_meta", reflect.TypeOf(consoleapi.AuthMeta{}), 0},
	{"console_login", reflect.TypeOf(consoleapi.Login{}), 0},
	{"console_logout", reflect.TypeOf(consoleapi.Logout{}), 0},
	{"console_sse_token", reflect.TypeOf(consoleapi.SSEToken{}), 0},
	{"console_voice_status", reflect.TypeOf(consoleapi.VoiceStatus{}), 0},
	{"console_voice_half", reflect.TypeOf(consoleapi.VoiceHalf{}), 0},
	{"console_workflow_hook", reflect.TypeOf(consoleapi.WorkflowHook{}), 0},
	{"console_stream_open", reflect.TypeOf(consoleapi.StreamOpen{}), 0},
	{"console_stream_event", reflect.TypeOf(consoleapi.StreamEvent{}), 0},
	{"console_run_stream_event", reflect.TypeOf(consoleapi.RunStreamEvent{}), 0},
	{"console_stream_error", reflect.TypeOf(consoleapi.StreamError{}), 0},
	{"console_stream_result", reflect.TypeOf(consoleapi.StreamResult{}), 0},
	{"console_error", reflect.TypeOf(consoleapi.Error{}), 1},
}

// rowExceptions are the hand-written views of a contract-owned row that are
// declared, with the reason. The key is the file and the literal's own field
// names — so a literal that stops looking like the exception is no longer
// exempt, and the exception itself must be deleted.
var rowExceptions = map[string]map[string]string{
	"rollback_helpers.go": {
		"id,reason,status": "the skill restore's operation ARGUMENTS, not an answer: the " +
			"console forwards them to the daemon's skill_restore operation",
	},
}

// mapLiteral finds the hand-written map literals of a source file.
var mapLiteral = regexp.MustCompile(`map\[string\]any\{([^}]*)\}`)

// literalKey finds the keys of a map literal.
var literalKey = regexp.MustCompile(`"([a-z0-9_]+)"\s*:`)

// rowFields reads the wire names of a row's fields.
func rowFields(row reflect.Type) []string {
	names := make([]string, 0, row.NumField())
	for i := 0; i < row.NumField(); i++ {
		name, _, _ := strings.Cut(row.Field(i).Tag.Get("json"), ",")
		if name == "" || name == "-" {
			continue
		}
		names = append(names, name)
	}
	return names
}

// packageDir is the directory this package's sources live in.
func packageDir(t *testing.T) string {
	t.Helper()
	_, file, _, ok := runtime.Caller(0)
	if !ok {
		t.Fatal("the caller's path is unknown")
	}
	return filepath.Dir(file)
}

// surfaceSources reads the package's own sources — the surface the guard
// measures.
func surfaceSources(t *testing.T) map[string]string {
	t.Helper()
	dir := packageDir(t)
	entries, err := os.ReadDir(dir)
	if err != nil {
		t.Fatalf("the console's sources are unreadable: %v", err)
	}
	out := map[string]string{}
	for _, entry := range entries {
		name := entry.Name()
		if entry.IsDir() || !strings.HasSuffix(name, ".go") || strings.HasSuffix(name, "_test.go") {
			continue
		}
		source, err := os.ReadFile(filepath.Join(dir, name))
		if err != nil {
			t.Fatalf("the console's %s is unreadable: %v", name, err)
		}
		out[name] = string(source)
	}
	return out
}

// TestTheConsoleSpellsNoContractOwnedRowByHand is the ratchet: no map literal
// in the console may spell the field names of a contract-owned row.
func TestTheConsoleSpellsNoContractOwnedRowByHand(t *testing.T) {
	sources := surfaceSources(t)
	seen := map[string]bool{}
	for name, source := range sources {
		for _, match := range mapLiteral.FindAllStringSubmatch(source, -1) {
			keys := map[string]bool{}
			for _, key := range literalKey.FindAllStringSubmatch(match[1], -1) {
				keys[key[1]] = true
			}
			names := make([]string, 0, len(keys))
			for key := range keys {
				names = append(names, key)
			}
			sort.Strings(names)
			literal := strings.Join(names, ",")
			for _, row := range contractOwnedRows {
				floor := row.Floor
				if floor == 0 {
					floor = 2
				}
				if len(keys) < floor {
					continue
				}
				shared := 0
				for _, field := range rowFields(row.Row) {
					if keys[field] {
						shared++
					}
				}
				if shared < floor {
					continue
				}
				if _, ok := rowExceptions[name][literal]; ok {
					seen[name+"|"+literal] = true
					continue
				}
				t.Errorf("%s writes a hand-written view of the %q row: it spells %s",
					name, row.Name, literal)
			}
		}
	}
	for file, literals := range rowExceptions {
		for literal, reason := range literals {
			if !seen[file+"|"+literal] {
				t.Errorf("%s declares an exception for %q that no longer spells a row by hand — "+
					"the exception is stale and must be deleted in the same change (%s)",
					file, literal, reason)
			}
		}
	}
}
