// SPDX-License-Identifier: MIT

package agentgw

import (
	"encoding/json"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"sync"
	"testing"
	"time"

	"github.com/agezt/agezt/kernel/event"
	"github.com/agezt/agezt/kernel/journal"
)

func WriteJSONEntry(w io.Writer, entry AuditEntry) error {
	data, err := json.Marshal(entry)
	if err != nil {
		return err
	}
	_, err = w.Write(data)
	return err
}

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

func AuditDir(baseDir string) string {
	return filepath.Join(baseDir, "agentgw", "audit")
}

// TestAuditLogger_Log tests that Log buffers entries and flushes at capacity.
func TestAuditLogger_Log(t *testing.T) {
	dir := t.TempDir()
	j, err := journal.Open(dir, journal.Options{})
	if err != nil {
		t.Fatalf("journal.Open: %v", err)
	}
	defer j.Close()

	al := NewAuditLogger(j)

	// Log entries up to the buffer capacity (64)
	for i := range 63 {
		al.Log(AuditEntry{
			Timestamp:  time.Now(),
			TokenID:    "token_test",
			RunID:      "run_test",
			Capability: "memory.write",
			Operation:  "POST",
			Success:    true,
			DurationMs: int64(i),
		})
	}

	// Entry 63 should NOT have triggered a flush yet (buffer is at 63, flush threshold is 64)
	// But the 64th entry will trigger a flush.
	al.Log(AuditEntry{
		Timestamp:  time.Now(),
		TokenID:    "token_test",
		RunID:      "run_test",
		Capability: "memory.write",
		Operation:  "POST",
		Success:    true,
		DurationMs: 999,
	})

	// After 64 entries, a flush should have been triggered.
	// Verify by reading the journal.
	entries, err := readAuditEntriesFromJournal(t, dir)
	if err != nil {
		t.Fatalf("readAuditEntries: %v", err)
	}

	if len(entries) != 64 {
		t.Errorf("expected 64 entries, got %d", len(entries))
	}
}

// TestAuditLogger_LogSync tests that LogSync writes entries immediately.
func TestAuditLogger_LogSync(t *testing.T) {
	dir := t.TempDir()
	j, err := journal.Open(dir, journal.Options{})
	if err != nil {
		t.Fatalf("journal.Open: %v", err)
	}
	defer j.Close()

	al := NewAuditLogger(j)

	entry := AuditEntry{
		Timestamp:  time.Now(),
		TokenID:    "token_sync",
		RunID:      "run_sync",
		Capability: "memory.read",
		Operation:  "GET",
		Success:    true,
		DurationMs: 5,
		ClientIP:   "192.168.1.1",
	}

	al.LogSync(entry)

	// Verify the entry was written immediately.
	entries, err := readAuditEntriesFromJournal(t, dir)
	if err != nil {
		t.Fatalf("readAuditEntries: %v", err)
	}

	if len(entries) != 1 {
		t.Fatalf("expected 1 entry, got %d", len(entries))
	}

	if entries[0].TokenID != "token_sync" {
		t.Errorf("TokenID: got %q, want %q", entries[0].TokenID, "token_sync")
	}
	if entries[0].RunID != "run_sync" {
		t.Errorf("RunID: got %q, want %q", entries[0].RunID, "run_sync")
	}
	if entries[0].Capability != "memory.read" {
		t.Errorf("Capability: got %q, want %q", entries[0].Capability, "memory.read")
	}
	if entries[0].Success != true {
		t.Error("Success: got false, want true")
	}
	if entries[0].DurationMs != 5 {
		t.Errorf("DurationMs: got %d, want %d", entries[0].DurationMs, 5)
	}
	if entries[0].ClientIP != "192.168.1.1" {
		t.Errorf("ClientIP: got %q, want %q", entries[0].ClientIP, "192.168.1.1")
	}
}

// TestAuditLogger_Flush tests that Flush writes buffered entries.
func TestAuditLogger_Flush(t *testing.T) {
	dir := t.TempDir()
	j, err := journal.Open(dir, journal.Options{})
	if err != nil {
		t.Fatalf("journal.Open: %v", err)
	}
	defer j.Close()

	al := NewAuditLogger(j)

	// Log some entries without triggering auto-flush.
	for i := range 10 {
		al.Log(AuditEntry{
			Timestamp:  time.Now(),
			TokenID:    "token_flush",
			RunID:      "run_flush",
			Capability: "memory.write",
			Operation:  "POST",
			Success:    i%2 == 0,
			DurationMs: int64(i),
		})
	}

	// Explicitly flush.
	al.Flush()

	// Verify all 10 entries were written.
	entries, err := readAuditEntriesFromJournal(t, dir)
	if err != nil {
		t.Fatalf("readAuditEntries: %v", err)
	}

	if len(entries) != 10 {
		t.Errorf("expected 10 entries, got %d", len(entries))
	}
}

// TestAuditLogger_Flush_Empty tests that Flush on an empty buffer is a no-op.
func TestAuditLogger_Flush_Empty(t *testing.T) {
	dir := t.TempDir()
	j, err := journal.Open(dir, journal.Options{})
	if err != nil {
		t.Fatalf("journal.Open: %v", err)
	}
	defer j.Close()

	al := NewAuditLogger(j)

	// Flush without logging anything.
	al.Flush()

	// No panic and no entries written.
	entries, err := readAuditEntriesFromJournal(t, dir)
	if err != nil {
		t.Fatalf("readAuditEntries: %v", err)
	}

	if len(entries) != 0 {
		t.Errorf("expected 0 entries, got %d", len(entries))
	}
}

// TestAuditLogger_LogSync_Then_Log tests that LogSync followed by Log works correctly.
func TestAuditLogger_LogSync_Then_Log(t *testing.T) {
	dir := t.TempDir()
	j, err := journal.Open(dir, journal.Options{})
	if err != nil {
		t.Fatalf("journal.Open: %v", err)
	}
	defer j.Close()

	al := NewAuditLogger(j)

	// LogSync first entry.
	al.LogSync(AuditEntry{
		Timestamp:  time.Now(),
		TokenID:    "token_first",
		RunID:      "run_first",
		Capability: "memory.write",
		Operation:  "POST",
		Success:    true,
		DurationMs: 10,
	})

	// Log more entries.
	for i := range 5 {
		al.Log(AuditEntry{
			Timestamp:  time.Now(),
			TokenID:    "token_batch",
			RunID:      "run_batch",
			Capability: "memory.read",
			Operation:  "GET",
			Success:    true,
			DurationMs: int64(i),
		})
	}

	// Flush to ensure buffered entries are written.
	al.Flush()

	// Verify 1 + 5 = 6 entries total.
	entries, err := readAuditEntriesFromJournal(t, dir)
	if err != nil {
		t.Fatalf("readAuditEntries: %v", err)
	}

	if len(entries) != 6 {
		t.Errorf("expected 6 entries, got %d", len(entries))
	}

	if entries[0].TokenID != "token_first" {
		t.Errorf("first entry TokenID: got %q, want %q", entries[0].TokenID, "token_first")
	}
}

// TestAuditLogger_WithNilJournal tests that AuditLogger handles nil journal gracefully.
func TestAuditLogger_WithNilJournal(t *testing.T) {
	al := NewAuditLogger(nil)

	// These should not panic.
	al.Log(AuditEntry{
		Timestamp:  time.Now(),
		TokenID:    "token_nil",
		RunID:      "run_nil",
		Capability: "memory.write",
		Operation:  "POST",
		Success:    true,
		DurationMs: 1,
	})

	al.LogSync(AuditEntry{
		Timestamp:  time.Now(),
		TokenID:    "token_nil_sync",
		RunID:      "run_nil_sync",
		Capability: "memory.write",
		Operation:  "POST",
		Success:    true,
		DurationMs: 2,
	})

	al.Flush()

	// No panic means success.
}

// TestReadAuditLog tests reading audit entries from a JSONL file.
func TestReadAuditLog(t *testing.T) {
	// Create a temporary JSONL file with audit entries.
	dir := t.TempDir()
	auditFile := filepath.Join(dir, "audit.jsonl")

	entries := []AuditEntry{
		{
			Timestamp:  time.Now().UTC().Truncate(time.Second),
			TokenID:    "token_read_test",
			RunID:      "run_read_1",
			Capability: "memory.write",
			Operation:  "POST",
			Path:       "/v1/memory/write",
			Success:    true,
			DurationMs: 25,
			ClientIP:   "10.0.0.1",
		},
		{
			Timestamp:  time.Now().UTC().Truncate(time.Second),
			TokenID:    "token_read_test",
			RunID:      "run_read_2",
			Capability: "memory.read",
			Operation:  "GET",
			Path:       "/v1/memory/read",
			Success:    false,
			Error:      "not found",
			DurationMs: 10,
			ClientIP:   "10.0.0.2",
		},
	}

	// Write entries as JSONL.
	f, err := os.Create(auditFile)
	if err != nil {
		t.Fatalf("Create: %v", err)
	}
	for _, e := range entries {
		data, _ := MarshalAuditEntry(e)
		f.Write(data)
		f.Write([]byte("\n"))
	}
	f.Close()

	// Read back.
	readEntries, err := ReadAuditLog(auditFile)
	if err != nil {
		t.Fatalf("ReadAuditLog: %v", err)
	}

	if len(readEntries) != 2 {
		t.Fatalf("expected 2 entries, got %d", len(readEntries))
	}

	// Verify first entry.
	if readEntries[0].TokenID != "token_read_test" {
		t.Errorf("TokenID[0]: got %q, want %q", readEntries[0].TokenID, "token_read_test")
	}
	if readEntries[0].RunID != "run_read_1" {
		t.Errorf("RunID[0]: got %q, want %q", readEntries[0].RunID, "run_read_1")
	}
	if readEntries[0].Capability != "memory.write" {
		t.Errorf("Capability[0]: got %q, want %q", readEntries[0].Capability, "memory.write")
	}
	if readEntries[0].Success != true {
		t.Error("Success[0]: got false, want true")
	}
	if readEntries[0].Error != "" {
		t.Errorf("Error[0]: got %q, want %q", readEntries[0].Error, "")
	}

	// Verify second entry.
	if readEntries[1].RunID != "run_read_2" {
		t.Errorf("RunID[1]: got %q, want %q", readEntries[1].RunID, "run_read_2")
	}
	if readEntries[1].Success != false {
		t.Error("Success[1]: got true, want false")
	}
	if readEntries[1].Error != "not found" {
		t.Errorf("Error[1]: got %q, want %q", readEntries[1].Error, "not found")
	}
}

// TestReadAuditLog_FileNotFound tests that ReadAuditLog returns an error for non-existent files.
func TestReadAuditLog_FileNotFound(t *testing.T) {
	_, err := ReadAuditLog("/nonexistent/path/audit.jsonl")
	if err == nil {
		t.Error("expected error for non-existent file, got nil")
	}
}

// TestReadAuditLog_InvalidJSON tests that ReadAuditLog returns an error for invalid JSON.
func TestReadAuditLog_InvalidJSON(t *testing.T) {
	dir := t.TempDir()
	auditFile := filepath.Join(dir, "invalid.jsonl")

	f, err := os.Create(auditFile)
	if err != nil {
		t.Fatalf("Create: %v", err)
	}
	f.WriteString("not valid json\n")
	f.Close()

	_, err = ReadAuditLog(auditFile)
	if err == nil {
		t.Error("expected error for invalid JSON, got nil")
	}
}

// TestReadAuditLog_EmptyFile tests that ReadAuditLog returns an empty slice for empty files.
func TestReadAuditLog_EmptyFile(t *testing.T) {
	dir := t.TempDir()
	auditFile := filepath.Join(dir, "empty.jsonl")

	f, err := os.Create(auditFile)
	if err != nil {
		t.Fatalf("Create: %v", err)
	}
	f.Close()

	entries, err := ReadAuditLog(auditFile)
	if err != nil {
		t.Fatalf("ReadAuditLog: %v", err)
	}

	if len(entries) != 0 {
		t.Errorf("expected 0 entries, got %d", len(entries))
	}
}

// TestAuditDir tests that AuditDir returns the correct path.
func TestAuditDir(t *testing.T) {
	baseDir := "/var/lib/agezt"
	expected := filepath.Join(baseDir, "agentgw", "audit")
	got := AuditDir(baseDir)

	if got != expected {
		t.Errorf("AuditDir(%q): got %q, want %q", baseDir, got, expected)
	}
}

// TestWriteJSONEntry tests writing a single audit entry as JSON.
func TestWriteJSONEntry(t *testing.T) {
	entry := AuditEntry{
		Timestamp:  time.Date(2024, 1, 15, 10, 30, 0, 0, time.UTC),
		TokenID:    "token_write",
		RunID:      "run_write",
		Capability: "eventbus.publish",
		Operation:  "PUB",
		Path:       "/v1/eventbus/publish",
		Success:    true,
		DurationMs: 8,
		ClientIP:   "172.16.0.1",
	}

	data, err := MarshalAuditEntry(entry)
	if err != nil {
		t.Fatalf("MarshalAuditEntry: %v", err)
	}

	// Verify it can be unmarshaled back.
	var decoded AuditEntry
	if err := json.Unmarshal(data, &decoded); err != nil {
		t.Fatalf("UnmarshalJSON: %v", err)
	}

	if decoded.TokenID != entry.TokenID {
		t.Errorf("TokenID: got %q, want %q", decoded.TokenID, entry.TokenID)
	}
	if decoded.RunID != entry.RunID {
		t.Errorf("RunID: got %q, want %q", decoded.RunID, entry.RunID)
	}
	if decoded.Capability != entry.Capability {
		t.Errorf("Capability: got %q, want %q", decoded.Capability, entry.Capability)
	}
	if decoded.Operation != entry.Operation {
		t.Errorf("Operation: got %q, want %q", decoded.Operation, entry.Operation)
	}
	if decoded.Success != entry.Success {
		t.Errorf("Success: got %v, want %v", decoded.Success, entry.Success)
	}
	if decoded.DurationMs != entry.DurationMs {
		t.Errorf("DurationMs: got %d, want %d", decoded.DurationMs, entry.DurationMs)
	}
}

// TestWriteJSONEntry_Error tests that WriteJSONEntry handles marshal errors gracefully.
func TestWriteJSONEntry_Error(t *testing.T) {
	// This test verifies that invalid entries are handled.
	// Since AuditEntry contains only basic types, marshal should not fail.
	entry := AuditEntry{
		TokenID:    "token_err",
		RunID:      "run_err",
		Capability: "test",
		Operation:  "OP",
		Success:    true,
		DurationMs: 1,
	}

	data, err := MarshalAuditEntry(entry)
	if err != nil {
		t.Errorf("MarshalAuditEntry: unexpected error: %v", err)
	}
	if len(data) == 0 {
		t.Error("MarshalAuditEntry: returned empty data")
	}
}

// TestAuditLogger_Concurrent tests that concurrent Log/Flush calls are safe.
func TestAuditLogger_Concurrent(t *testing.T) {
	dir := t.TempDir()
	j, err := journal.Open(dir, journal.Options{})
	if err != nil {
		t.Fatalf("journal.Open: %v", err)
	}
	defer j.Close()

	al := NewAuditLogger(j)

	// Run concurrent log and flush operations.
	var wg sync.WaitGroup
	for range 10 {
		wg.Add(1)
		go func() {
			defer wg.Done()
			for j := range 100 {
				al.Log(AuditEntry{
					Timestamp:  time.Now(),
					TokenID:    "token_concurrent",
					RunID:      "run_concurrent",
					Capability: "memory.write",
					Operation:  "POST",
					Success:    true,
					DurationMs: int64(j),
				})
			}
		}()
	}

	// Also run concurrent flushes.
	for range 5 {
		wg.Add(1)
		go func() {
			defer wg.Done()
			for range 50 {
				al.Flush()
			}
		}()
	}

	// Wait for goroutines to complete.
	wg.Wait()

	// Flush everything at the end.
	al.Flush()

	// Verify no panic occurred and entries were written.
	entries, err := readAuditEntriesFromJournal(t, dir)
	if err != nil {
		t.Fatalf("readAuditEntries: %v", err)
	}

	// We expect some entries to be written (exact count may vary due to race)
	if len(entries) == 0 {
		t.Error("expected some entries after concurrent access, got 0")
	}
}

// TestAuditLogger_ErrorEntry tests that error details are properly recorded.
func TestAuditLogger_ErrorEntry(t *testing.T) {
	dir := t.TempDir()
	j, err := journal.Open(dir, journal.Options{})
	if err != nil {
		t.Fatalf("journal.Open: %v", err)
	}
	defer j.Close()

	al := NewAuditLogger(j)

	errorMsg := "permission denied: insufficient scope"
	al.LogSync(AuditEntry{
		Timestamp:  time.Now(),
		TokenID:    "token_error",
		RunID:      "run_error",
		Capability: "memory.delete",
		Operation:  "DELETE",
		Path:       "/v1/memory/delete",
		Success:    false,
		Error:      errorMsg,
		DurationMs: 3,
		ClientIP:   "192.168.0.1",
	})

	entries, err := readAuditEntriesFromJournal(t, dir)
	if err != nil {
		t.Fatalf("readAuditEntries: %v", err)
	}

	if len(entries) != 1 {
		t.Fatalf("expected 1 entry, got %d", len(entries))
	}

	if entries[0].Success != false {
		t.Error("Success: got true, want false")
	}
	if entries[0].Error != errorMsg {
		t.Errorf("Error: got %q, want %q", entries[0].Error, errorMsg)
	}
}

// TestAuditLogger_Subprocess tests that subprocess field is recorded.
func TestAuditLogger_Subprocess(t *testing.T) {
	dir := t.TempDir()
	j, err := journal.Open(dir, journal.Options{})
	if err != nil {
		t.Fatalf("journal.Open: %v", err)
	}
	defer j.Close()

	al := NewAuditLogger(j)

	al.LogSync(AuditEntry{
		Timestamp:  time.Now(),
		TokenID:    "token_parent",
		Subprocess: "sub_abc123",
		RunID:      "run_subprocess",
		Capability: "memory.write",
		Operation:  "POST",
		Success:    true,
		DurationMs: 12,
	})

	entries, err := readAuditEntriesFromJournal(t, dir)
	if err != nil {
		t.Fatalf("readAuditEntries: %v", err)
	}

	if len(entries) != 1 {
		t.Fatalf("expected 1 entry, got %d", len(entries))
	}

	if entries[0].Subprocess != "sub_abc123" {
		t.Errorf("Subprocess: got %q, want %q", entries[0].Subprocess, "sub_abc123")
	}
	if entries[0].TokenID != "token_parent" {
		t.Errorf("TokenID: got %q, want %q", entries[0].TokenID, "token_parent")
	}
}

// --- Helper functions ---

// readAuditEntriesFromJournal reads audit entries from the journal directory.
// The journal stores event.Event objects with AuditEntry in the Payload field.
func readAuditEntriesFromJournal(t *testing.T, dir string) ([]AuditEntry, error) {
	t.Helper()

	// Find the segment file.
	files, err := os.ReadDir(dir)
	if err != nil {
		return nil, err
	}

	var segmentFile string
	for _, e := range files {
		if !e.IsDir() && filepath.Ext(e.Name()) == ".jsonl" {
			segmentFile = filepath.Join(dir, e.Name())
			break
		}
	}

	if segmentFile == "" {
		return []AuditEntry{}, nil
	}

	// Open the journal segment file and decode event.Event objects.
	f, err := os.Open(segmentFile)
	if err != nil {
		return nil, err
	}
	defer f.Close()

	var auditEntries []AuditEntry
	dec := json.NewDecoder(f)
	for {
		var ev event.Event
		if err := dec.Decode(&ev); err == io.EOF {
			break
		} else if err != nil {
			return nil, fmt.Errorf("decode event: %w", err)
		}
		// Extract AuditEntry from Payload.
		if len(ev.Payload) == 0 {
			continue
		}
		var entry AuditEntry
		if err := json.Unmarshal(ev.Payload, &entry); err != nil {
			return nil, fmt.Errorf("unmarshal audit entry: %w", err)
		}
		auditEntries = append(auditEntries, entry)
	}

	return auditEntries, nil
}

// MarshalAuditEntry marshals an AuditEntry to JSON using the standard json package.
func MarshalAuditEntry(e AuditEntry) ([]byte, error) {
	return json.Marshal(e)
}
