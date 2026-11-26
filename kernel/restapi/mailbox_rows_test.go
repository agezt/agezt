// SPDX-License-Identifier: MIT

package restapi

// The mailbox surface spells no row by hand.
//
// The mailbox's rows live in the contract (kernel/contract/boardapi): the
// message row, the draft the client sends, the ack it sends, and the
// envelopes around them. The surface serves those rows — it does not write a
// `map[string]any` per endpoint and spell the field names a second time.
//
// A hand-written map is how the row got two homes in the first place: the
// contract named the fields, the surface named them again, and nothing said
// the two lists were the same list. So the surface is measured: a map literal
// that spells the field names of a mailbox row is a violation, and a declared
// exception that no longer matches is a violation too (a stale exception is a
// lie about the code).
//
// The declared exception is the watch stream's fallback frame: when a message
// was evicted from the store between the write and the publish, the stream
// sends the EVENT's metadata view (id, topic, from, to, help) — the body
// lives in the store, so that frame is deliberately not the mail row.

import (
	"os"
	"path/filepath"
	"regexp"
	"runtime"
	"sort"
	"strings"
	"testing"

	"github.com/agezt/agezt/kernel/contract/restapiapi"
)

// mailboxSurfaceFiles are the files that serve the mailbox's rows.
var mailboxSurfaceFiles = []string{
	"mailbox.go",
	"mailbox_handlers.go",
	"mailbox_watch.go",
}

// mailboxRowExceptions are the hand-written views of a mailbox row that are
// declared, with the reason. The key is the file and the literal's own field
// names — so a literal that stops looking like the exception is no longer
// exempt, and the exception itself must be deleted.
var mailboxRowExceptions = map[string]map[string]string{
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

// mailboxRows are the rows the mailbox surface sends or receives, by the
// contract's row name.
func mailboxRows() map[string][]string {
	out := map[string][]string{}
	for _, row := range restapiapi.Rows() {
		switch row.Name {
		case restapiapi.RowMail, restapiapi.RowMailDraft, restapiapi.RowMailAck,
			restapiapi.RowMailAcked, restapiapi.RowMailboxList, restapiapi.RowMailboxSent,
			restapiapi.RowMailboxReplies, restapiapi.RowMailboxInbox, restapiapi.RowMailboxTopics:
			names := make([]string, 0, len(row.Fields))
			for _, f := range row.Fields {
				names = append(names, f.Name)
			}
			out[row.Name] = names
		}
	}
	return out
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

// TestTheMailboxSurfaceSpellsNoRowByHand is the ratchet: no map literal in the
// mailbox surface may spell the field names of a mailbox row.
func TestTheMailboxSurfaceSpellsNoRowByHand(t *testing.T) {
	dir := packageDir(t)
	rows := mailboxRows()
	seen := map[string]bool{}
	for _, name := range mailboxSurfaceFiles {
		source, err := os.ReadFile(filepath.Join(dir, name))
		if err != nil {
			t.Fatalf("the mailbox surface's %s is unreadable: %v", name, err)
		}
		for _, match := range mapLiteral.FindAllStringSubmatch(string(source), -1) {
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
			for row, fields := range rows {
				shared := 0
				for _, field := range fields {
					if keys[field] {
						shared++
					}
				}
				if shared < 2 {
					continue
				}
				if reason, ok := mailboxRowExceptions[name][literal]; ok {
					_ = reason
					seen[name+"|"+literal] = true
					continue
				}
				t.Errorf("%s writes a hand-written view of the %q row: it spells %s",
					name, row, literal)
			}
		}
	}
	for file, literals := range mailboxRowExceptions {
		for literal, reason := range literals {
			if !seen[file+"|"+literal] {
				t.Errorf("%s declares an exception for %q that no longer spells a row by hand — "+
					"the exception is stale and must be deleted in the same change (%s)",
					file, literal, reason)
			}
		}
	}
}
