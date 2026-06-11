// SPDX-License-Identifier: MIT

// Package artifacts is the in-process `artifacts` tool: it lets the agent
// enumerate, read back, and delete the files it has saved — the artifact store /
// Files view (M822–M823) seen from the agent's side (M832). `fetch` and the
// tool-output offloader PUT files in; this tool reads them back OUT, so a file
// saved in one run is usable in a later one (the model gets an id from `fetch`
// but, without this, had no way to list "what do I have" or pull a saved file's
// bytes into context).
//
// It is a read/list/delete view over the existing artifact index — no new store,
// no network. Listing and reading are the file-read axis; delete is file-delete.
package artifacts

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"strings"

	"github.com/agezt/agezt/kernel/agent"
	"github.com/agezt/agezt/kernel/artifact"
)

// MaxReadBytes caps how much of a text artifact `read` returns inline, so pulling
// a large file back can't blow the model's context. Bigger files are truncated
// with a note (the whole file is still downloadable from the Files view).
const MaxReadBytes = 256 << 10 // 256 KiB

// DefaultListLimit caps a `list` with no explicit limit.
const DefaultListLimit = 50

// Index is the slice of the artifact index the tool needs — satisfied by
// *artifact.Index. An interface keeps the tool decoupled and testable.
type Index interface {
	List(f artifact.Filter) []artifact.Entry
	Bytes(id string) ([]byte, artifact.Entry, error)
	Delete(id string) error
}

// Tool is the `artifacts` implementation of agent.Tool.
type Tool struct {
	index Index
}

// New returns an empty Tool; call SetIndex before use.
func New() *Tool { return &Tool{} }

// SetIndex injects the artifact index (done by the daemon after the kernel opens,
// since the index lives on the kernel). Without it, the tool reports unavailable.
func (t *Tool) SetIndex(idx Index) { t.index = idx }

// Definition implements agent.Tool.
func (t *Tool) Definition() agent.ToolDef {
	return agent.ToolDef{
		Name: "artifacts",
		Description: "List, read, or delete the files saved in the artifact store / Files view — " +
			"the images, downloads (from `fetch`), and offloaded tool outputs you've produced. " +
			"op=list returns metadata {id, name, mime, kind, source, size} (filter by kind/source/corr); " +
			"op=read returns a text file's content by id (binary files report their metadata instead — " +
			"download them from the Files view); op=delete removes one by id.",
		InputSchema: json.RawMessage(`{
  "type": "object",
  "required": ["op"],
  "properties": {
    "op":     {"type":"string", "enum":["list","read","delete"], "description":"What to do."},
    "id":     {"type":"string", "description":"Artifact id (required for read/delete)."},
    "kind":   {"type":"string", "description":"list filter: image | download | tool-output | …"},
    "source": {"type":"string", "description":"list filter: fetch | telegram | run | …"},
    "corr":   {"type":"string", "description":"list filter: correlation id of the run/message."},
    "limit":  {"type":"integer", "description":"list: max entries to return (default 50)."}
  }
}`),
	}
}

type input struct {
	Op     string `json:"op"`
	ID     string `json:"id,omitempty"`
	Kind   string `json:"kind,omitempty"`
	Source string `json:"source,omitempty"`
	Corr   string `json:"corr,omitempty"`
	Limit  int    `json:"limit,omitempty"`
}

// Invoke implements agent.Tool.
func (t *Tool) Invoke(ctx context.Context, raw json.RawMessage) (agent.Result, error) {
	var in input
	if err := json.Unmarshal(raw, &in); err != nil {
		return agent.Result{}, fmt.Errorf("artifacts: parse input: %w", err)
	}
	if t.index == nil {
		return errResult("artifact store unavailable"), nil
	}
	switch strings.ToLower(strings.TrimSpace(in.Op)) {
	case "list":
		return t.list(in), nil
	case "read":
		return t.read(in), nil
	case "delete":
		return t.del(in), nil
	default:
		return errResult("op must be one of list, read, delete"), nil
	}
}

func (t *Tool) list(in input) agent.Result {
	entries := t.index.List(artifact.Filter{Kind: in.Kind, Source: in.Source, Corr: in.Corr})
	limit := in.Limit
	if limit <= 0 {
		limit = DefaultListLimit
	}
	truncated := false
	if len(entries) > limit {
		entries = entries[:limit]
		truncated = true
	}
	rows := make([]map[string]any, 0, len(entries))
	for _, e := range entries {
		row := map[string]any{
			"id": e.ID, "name": e.Name, "mime": e.Mime, "kind": e.Kind,
			"source": e.Source, "size": e.Size,
		}
		if e.Caption != "" {
			row["caption"] = e.Caption
		}
		rows = append(rows, row)
	}
	out, _ := json.MarshalIndent(map[string]any{
		"count":     len(rows),
		"truncated": truncated,
		"artifacts": rows,
	}, "", "  ")
	return agent.Result{Output: string(out)}
}

func (t *Tool) read(in input) agent.Result {
	id := strings.TrimSpace(in.ID)
	if id == "" {
		return errResult("id required for read")
	}
	data, e, err := t.index.Bytes(id)
	if err != nil {
		return errResult("read " + id + ": " + err.Error())
	}
	if !isTextMime(e.Mime, data) {
		out, _ := json.MarshalIndent(map[string]any{
			"id": e.ID, "name": e.Name, "mime": e.Mime, "size": e.Size,
			"binary": true,
			"note":   "binary file — not shown inline; download it from the Files view",
		}, "", "  ")
		return agent.Result{Output: string(out)}
	}
	text := string(data)
	note := ""
	if len(data) > MaxReadBytes {
		text = string(data[:MaxReadBytes])
		note = fmt.Sprintf("\n\n[truncated: showing first %d of %d bytes]", MaxReadBytes, len(data))
	}
	header := fmt.Sprintf("%s (%s, %d bytes)\n\n", e.Name, e.Mime, e.Size)
	return agent.Result{Output: header + text + note}
}

func (t *Tool) del(in input) agent.Result {
	id := strings.TrimSpace(in.ID)
	if id == "" {
		return errResult("id required for delete")
	}
	if err := t.index.Delete(id); err != nil {
		return errResult("delete " + id + ": " + err.Error())
	}
	return agent.Result{Output: "deleted " + id}
}

// isTextMime reports whether an artifact is safe to return inline as text — either
// its mime is text-like, or (when the mime is missing/opaque) the bytes contain no
// NUL, the classic "looks binary" tell.
func isTextMime(mime string, data []byte) bool {
	m := strings.ToLower(strings.TrimSpace(mime))
	switch {
	case strings.HasPrefix(m, "text/"):
		return true
	case m == "application/json" || m == "application/xml" || m == "application/javascript" ||
		m == "application/x-yaml" || m == "application/yaml" || m == "image/svg+xml" ||
		strings.HasSuffix(m, "+json") || strings.HasSuffix(m, "+xml"):
		return true
	case m == "":
		// Unknown mime: treat as text only if the bytes look like text (no NUL).
		return !bytes.Contains(data, []byte{0})
	default:
		return false
	}
}

func errResult(msg string) agent.Result {
	return agent.Result{Output: "artifacts: " + msg, IsError: true}
}
