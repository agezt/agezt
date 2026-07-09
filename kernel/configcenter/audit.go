// SPDX-License-Identifier: MIT

package configcenter

import (
	"encoding/json"
	"fmt"
	"log/slog"
	"os"
	"path/filepath"
	"sync"
	"time"
)

// AuditLogger handles audit logging for config access events.
type AuditLogger struct {
	mu     sync.Mutex
	dir    string
	config *Config
}

// NewAuditLogger creates a new audit logger.
func NewAuditLogger(dir string, config *Config) *AuditLogger {
	return &AuditLogger{
		mu:     sync.Mutex{},
		dir:    dir,
		config: config,
	}
}

// Close closes the audit logger (no-op for file-based logging).
func (a *AuditLogger) Close() error {
	return nil
}

// Log records an access attempt in the audit log.
func (a *AuditLogger) Log(req *ConfigAccessRequest, decision AccessDecision, policy, reasonCode, value string) {
	a.mu.Lock()
	defer a.mu.Unlock()

	entry := &AuditEntry{
		ID:         fmt.Sprintf("audit_%d_%s", time.Now().UnixNano(), req.AgentID),
		Event:      "config.access",
		Timestamp:  time.Now().Unix(),
		AgentID:    req.AgentID,
		RunID:      req.RunID,
		Key:        req.Key,
		Reason:     req.Reason,
		Decision:   decision,
		Policy:     policy,
		ReasonCode: reasonCode,
		Metadata:   make(map[string]string),
	}

	// Determine value logging based on rating
	classifier := NewSecretClassifier()
	rating := classifier.Classify(req.Key, value)

	if decision == AccessAllowed {
		if rating == RatingSecret {
			entry.ValueLog = "REDACTED"
		} else if a.config == nil || a.config.Audit.LogPublicValues || rating != RatingPublic {
			// Show preview for restricted/internal, full for public
			entry.ValueLog = a.previewValue(value, rating)
		} else {
			entry.ValueLog = HashValue(value)
		}
	} else {
		if rating == RatingSecret {
			entry.ValueLog = "REDACTED"
		} else {
			entry.ValueLog = HashValue(value)
		}
	}

	// Write to disk
	a.writeToFile(entry)
}

// previewValue returns a preview of a value appropriate for logging.
func (a *AuditLogger) previewValue(value string, rating Rating) string {
	if rating == RatingPublic || rating == "" {
		return value
	}

	// For restricted/internal, show first 8 chars + hash
	if len(value) <= 8 {
		return value
	}
	return value[:8] + "..." + HashValue(value)[:8]
}

// writeToFile writes an audit entry to disk. Failures are logged rather than
// silently dropped so a broken audit trail is at least visible in the daemon log.
func (a *AuditLogger) writeToFile(entry *AuditEntry) {
	if a.dir == "" {
		return
	}

	data, err := json.Marshal(entry)
	if err != nil {
		slog.Warn("config center: audit entry marshal failed", "error", err)
		return
	}

	filename := filepath.Join(a.dir, fmt.Sprintf("audit_%s.jsonl", time.Now().Format("2006-01-02")))

	if err := os.MkdirAll(a.dir, 0o755); err != nil {
		slog.Warn("config center: audit dir create failed", "dir", a.dir, "error", err)
		return
	}

	f, err := os.OpenFile(filename, os.O_APPEND|os.O_CREATE|os.O_WRONLY, 0o644)
	if err != nil {
		slog.Warn("config center: audit log open failed", "file", filename, "error", err)
		return
	}
	defer f.Close()

	// One Write for entry+newline: an interleaved second write can't corrupt
	// the JSONL line, and a partial-write error is surfaced.
	if _, err := f.Write(append(data, '\n')); err != nil {
		slog.Warn("config center: audit log write failed", "file", filename, "error", err)
	}
}

// Query searches audit logs with filters.
func (a *AuditLogger) Query(opts AuditQuery) []*AuditEntry {
	a.mu.Lock()
	defer a.mu.Unlock()

	var results []*AuditEntry

	if a.dir == "" {
		return results
	}

	entries, err := os.ReadDir(a.dir)
	if err != nil {
		return results
	}

	for _, entry := range entries {
		if entry.IsDir() || len(entry.Name()) < 7 || entry.Name()[:7] != "audit_" {
			continue
		}

		filepath := filepath.Join(a.dir, entry.Name())
		data, err := os.ReadFile(filepath)
		if err != nil {
			continue
		}

		// Parse each line as a JSON object
		lines := splitLines(string(data))
		for _, line := range lines {
			if line == "" {
				continue
			}

			var auditEntry AuditEntry
			if err := json.Unmarshal([]byte(line), &auditEntry); err != nil {
				continue
			}

			// Apply filters
			if opts.AgentID != nil && *opts.AgentID != "" && auditEntry.AgentID != *opts.AgentID {
				continue
			}
			if opts.Key != nil && *opts.Key != "" && auditEntry.Key != *opts.Key {
				continue
			}
			if opts.Decision != "" && auditEntry.Decision != opts.Decision {
				continue
			}
			if opts.Since > 0 && auditEntry.Timestamp < opts.Since {
				continue
			}
			if opts.Until > 0 && auditEntry.Timestamp > opts.Until {
				continue
			}

			results = append(results, &auditEntry)
		}
	}

	// Apply limit
	if opts.Limit > 0 && len(results) > opts.Limit {
		results = results[len(results)-opts.Limit:]
	}

	return results
}

// AuditQuery contains query filters for audit logs.
type AuditQuery struct {
	AgentID  *string // Pointer to detect if set
	Key      *string
	Decision AccessDecision
	Since    int64 // Unix timestamp
	Until    int64 // Unix timestamp
	Limit    int
}

// AccessLogEntry represents an access log entry (alias for AuditEntry for clarity).
type AccessLogEntry = AuditEntry

// QueryAccessLog queries access logs with filters.
// This is a convenience method that accepts string parameters.
func (a *AuditLogger) QueryAccessLog(opts AuditQuery) []*AccessLogEntry {
	return a.Query(opts)
}

func splitLines(s string) []string {
	var lines []string
	start := 0
	for i := 0; i < len(s); i++ {
		if s[i] == '\n' {
			lines = append(lines, s[start:i])
			start = i + 1
		}
	}
	if start < len(s) {
		lines = append(lines, s[start:])
	}
	return lines
}
