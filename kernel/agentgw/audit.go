// SPDX-License-Identifier: MIT

package agentgw

import (
	"encoding/json"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"sync"

	"github.com/agezt/agezt/kernel/event"
	"github.com/agezt/agezt/kernel/journal"
)

// AuditLogger records capability access events to a journal.
type AuditLogger struct {
	j   *journal.Journal
	mu  sync.Mutex
	buf []AuditEntry
}

// NewAuditLogger creates a new audit logger backed by a journal.
func NewAuditLogger(j *journal.Journal) *AuditLogger {
	return &AuditLogger{
		j:   j,
		buf: make([]AuditEntry, 0, 64),
	}
}

// Log records an audit entry. It buffers entries and flushes periodically.
func (a *AuditLogger) Log(entry AuditEntry) {
	a.mu.Lock()
	a.buf = append(a.buf, entry)
	shouldFlush := len(a.buf) >= 64
	a.mu.Unlock()

	if shouldFlush {
		a.Flush()
	}
}

// LogSync records an entry and flushes immediately.
func (a *AuditLogger) LogSync(entry AuditEntry) {
	a.mu.Lock()
	a.buf = append(a.buf, entry)
	entries := a.buf
	// Hand the backing array off to writeEntries and start a fresh buffer.
	// Reusing it via a.buf[:0] would let a concurrent Log append into the same
	// array elements that writeEntries is reading unlocked — a data race.
	a.buf = make([]AuditEntry, 0, 64)
	a.mu.Unlock()

	a.writeEntries(entries)
}

// Flush writes all buffered entries to the journal.
func (a *AuditLogger) Flush() {
	a.mu.Lock()
	if len(a.buf) == 0 {
		a.mu.Unlock()
		return
	}
	entries := a.buf
	// Fresh backing array, not a.buf[:0]: entries is read unlocked by
	// writeEntries, so it must not share storage with future appends.
	a.buf = make([]AuditEntry, 0, 64)
	a.mu.Unlock()

	a.writeEntries(entries)
}

// writeEntries writes entries to the journal.
func (a *AuditLogger) writeEntries(entries []AuditEntry) {
	if len(entries) == 0 || a.j == nil {
		return
	}

	for _, e := range entries {
		data, err := json.Marshal(e)
		if err != nil {
			fmt.Fprintf(os.Stderr, "agentgw: audit: marshal: %v\n", err)
			continue
		}

		// Write as an audit event using the journal's Append method
		spec := event.Spec{
			Subject:       "agentgw.audit",
			Kind:          event.KindInfo,
			Actor:         e.TokenID,
			CorrelationID: e.RunID,
			Payload:       json.RawMessage(data),
			Tags: map[string]string{
				"run_id": e.RunID,
				"cap":    e.Capability,
			},
		}
		if _, err := a.j.Append(spec); err != nil {
			fmt.Fprintf(os.Stderr, "agentgw: audit: append: %v\n", err)
		}
	}
}

// WriteJSONEntry writes a single audit entry as JSON to w.
func WriteJSONEntry(w io.Writer, entry AuditEntry) error {
	data, err := json.Marshal(entry)
	if err != nil {
		return err
	}
	_, err = w.Write(data)
	return err
}

// ReadAuditLog reads audit entries from a journal file.
func ReadAuditLog(path string) ([]AuditEntry, error) {
	f, err := os.Open(path)
	if err != nil {
		return nil, fmt.Errorf("agentgw: audit: open: %w", err)
	}
	defer f.Close()

	var entries []AuditEntry
	dec := json.NewDecoder(f)
	for {
		var entry AuditEntry
		if err := dec.Decode(&entry); err == io.EOF {
			break
		} else if err != nil {
			return nil, fmt.Errorf("agentgw: audit: decode: %w", err)
		}
		entries = append(entries, entry)
	}

	return entries, nil
}

// AuditDir returns the directory where audit logs are stored.
func AuditDir(baseDir string) string {
	return filepath.Join(baseDir, "agentgw", "audit")
}
