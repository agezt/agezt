// SPDX-License-Identifier: MIT

package restapi

// The REST surface spells no CONTRACT-OWNED row by hand.
//
// The daemon's rows live in the contracts: the board's message row and the
// envelopes around it (kernel/contract/boardapi), the artifact index's rows
// (kernel/contract/remoteapi), the update surface's rows
// (kernel/contract/updateapi). The surface serves those rows — it does not
// write a `map[string]any` per endpoint and spell the field names a second
// time.
//
// A hand-written map is how a row got two homes in the first place: the
// contract named the fields, the surface named them again, and nothing said
// the two lists were the same list. So the surface is measured: a map literal
// that spells the field names of a contract-owned row is a violation, and a
// declared exception that no longer matches is a violation too (a stale
// exception is a lie about the code).
//
// The declared exception is the watch stream's fallback frame: when a message
// was evicted from the store between the write and the publish, the stream
// sends the EVENT's metadata view (id, topic, from, to, help) — the body
// lives in the store, so that frame is deliberately not the mail row.

import (
	"os"
	"path/filepath"
	"reflect"
	"regexp"
	"runtime"
	"sort"
	"strings"
	"testing"

	"github.com/agezt/agezt/kernel/contract/boardapi"
	"github.com/agezt/agezt/kernel/contract/remoteapi"
	"github.com/agezt/agezt/kernel/contract/updateapi"
)

// contractOwnedRows are the rows the contracts own, by name. Every one of them
// is a row the surface serves or reads — so none of them may be written by
// hand.
var contractOwnedRows = []struct {
	Name string
	Row  reflect.Type
}{
	{"mail", reflect.TypeOf(boardapi.Mail{})},
	{"mailbox_list", reflect.TypeOf(boardapi.MailboxList{})},
	{"mailbox_sent", reflect.TypeOf(boardapi.MailboxSent{})},
	{"mailbox_replies", reflect.TypeOf(boardapi.MailboxReplies{})},
	{"mailbox_inbox", reflect.TypeOf(boardapi.MailboxInbox{})},
	{"mailbox_topics", reflect.TypeOf(boardapi.MailboxTopics{})},
	{"mail_acked", reflect.TypeOf(boardapi.MailAcked{})},
	{"artifact", reflect.TypeOf(remoteapi.Artifact{})},
	{"artifact_list", reflect.TypeOf(remoteapi.ArtifactList{})},
	{"update_release", reflect.TypeOf(updateapi.Release{})},
	{"update_check", reflect.TypeOf(updateapi.Check{})},
	{"update_applied", reflect.TypeOf(updateapi.Applied{})},
}

// rowExceptions are the hand-written views of a contract-owned row that are
// declared, with the reason. The key is the file and the literal's own field
// names — so a literal that stops looking like the exception is no longer
// exempt, and the exception itself must be deleted.
var rowExceptions = map[string]map[string]string{
	"mailbox_watch.go": {
		"from,help,id,to,topic": "the watch stream's fallback frame: the event's metadata view " +
			"for a message that was evicted from the store, not the mail row (the body lives " +
			"in the store)",
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
		t.Fatalf("the surface's sources are unreadable: %v", err)
	}
	out := map[string]string{}
	for _, entry := range entries {
		name := entry.Name()
		if entry.IsDir() || !strings.HasSuffix(name, ".go") || strings.HasSuffix(name, "_test.go") {
			continue
		}
		source, err := os.ReadFile(filepath.Join(dir, name))
		if err != nil {
			t.Fatalf("the surface's %s is unreadable: %v", name, err)
		}
		out[name] = string(source)
	}
	return out
}

// TestTheSurfaceSpellsNoContractOwnedRowByHand is the ratchet: no map literal
// in the surface may spell the field names of a contract-owned row.
func TestTheSurfaceSpellsNoContractOwnedRowByHand(t *testing.T) {
	sources := surfaceSources(t)
	seen := map[string]bool{}
	for name, source := range sources {
		for _, match := range mapLiteral.FindAllStringSubmatch(source, -1) {
			keys := map[string]bool{}
			for _, key := range literalKey.FindAllStringSubmatch(match[1], -1) {
				keys[key[1]] = true
			}
			if len(keys) < 2 {
				continue
			}
			names := make([]string, 0, len(keys))
			for key := range keys {
				names = append(names, key)
			}
			sort.Strings(names)
			literal := strings.Join(names, ",")
			for _, row := range contractOwnedRows {
				shared := 0
				for _, field := range rowFields(row.Row) {
					if keys[field] {
						shared++
					}
				}
				if shared < 2 {
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
