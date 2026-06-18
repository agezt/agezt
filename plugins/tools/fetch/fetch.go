// SPDX-License-Identifier: MIT

// Package fetch is the in-process `fetch` tool: it downloads a URL's bytes and
// SAVES them as a browsable artifact (M831), returning the artifact id + mime +
// size. It complements the `http` tool — which returns a page's text into the
// model's context — by capturing BINARY content (images, PDFs, archives, …) into
// the artifact store / file manager, where the operator can preview and download
// it. The capability is a plain network GET (edict CapHTTPGet).
//
// Like the http/web_search tools it goes through a netguard-protected client
// that refuses internal/metadata addresses (SSRF guard), relaxed only by the
// explicit AllowLoopback/AllowPrivate flags.
package fetch

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	stdhttp "net/http"
	"net/url"
	"path"
	"strings"
	"time"

	"github.com/agezt/agezt/kernel/agent"
	"github.com/agezt/agezt/kernel/artifact"
	"github.com/agezt/agezt/kernel/netguard"
)

// DefaultTimeout caps a single download.
const DefaultTimeout = 60 * time.Second

// MaxBytes caps a single download so one fetch can't exhaust disk/memory.
const MaxBytes = 50 << 20 // 50 MiB

// Indexer is the slice of the artifact index the tool needs — satisfied by
// *artifact.Index. An interface keeps the tool decoupled and testable.
type Indexer interface {
	PutEntry(meta artifact.Entry, data []byte, createdMs int64) (artifact.Entry, error)
}

// Tool is the `fetch` implementation of agent.Tool.
type Tool struct {
	// HTTP overrides the default client; when nil a netguard-protected client is
	// built (default-deny to internal/metadata addresses).
	HTTP *stdhttp.Client
	// AllowLoopback / AllowPrivate relax the egress guard for the default client.
	AllowLoopback bool
	AllowPrivate  bool
	// UserAgent is sent on every request.
	UserAgent string
	// OnBlock is called when the egress guard refuses a dial (journaled by the daemon).
	OnBlock func(ip, reason string)
	// Now returns the wall-clock millis stamped on the stored entry; injectable for tests.
	Now func() int64

	index Indexer
}

// DefaultUserAgent identifies the fetcher.
const DefaultUserAgent = "agezt-fetch/1.0"

// New returns a Tool with safe defaults.
func New() *Tool {
	return &Tool{UserAgent: DefaultUserAgent, Now: func() int64 { return time.Now().UnixMilli() }}
}

// SetIndex injects the artifact index (done by the daemon after the kernel opens,
// since the index lives on the kernel). Without it, the tool reports unavailable.
func (t *Tool) SetIndex(idx Indexer) { t.index = idx }

func (t *Tool) client() *stdhttp.Client {
	if t.HTTP != nil {
		return t.HTTP
	}
	var opts []netguard.Option
	if t.AllowLoopback {
		opts = append(opts, netguard.AllowLoopback())
	}
	if t.AllowPrivate {
		opts = append(opts, netguard.AllowPrivate())
	}
	if t.OnBlock != nil {
		opts = append(opts, netguard.OnBlock(t.OnBlock))
	}
	return netguard.New(opts...).HTTPClient(DefaultTimeout)
}

// Definition implements agent.Tool.
func (t *Tool) Definition() agent.ToolDef {
	return agent.ToolDef{
		Name: "fetch",
		Description: "Download a URL and SAVE its bytes as an artifact (file) — use this to keep " +
			"an image, PDF, or other file from the web (it appears in the Files view and can be " +
			"downloaded). Returns the artifact {id, mime, size, name}. For reading a page's TEXT " +
			"into your context, use the http tool instead.",
		InputSchema: json.RawMessage(`{
  "type": "object",
  "required": ["url"],
  "properties": {
    "url":  {"type":"string", "description":"Absolute http/https URL to download."},
    "name": {"type":"string", "description":"Optional file name for the saved artifact."}
  }
}`),
		Effect: agent.ToolEffect{
			Class: agent.EffectReversible,
			PredictedEffects: []string{
				"Download bytes from an HTTP(S) URL with GET.",
				"Persist the downloaded content into the artifact store for later viewing or reuse.",
			},
			AffectedResources: []string{"remote HTTP(S) endpoint", "local artifact store"},
			RollbackNotes:     "Delete the saved artifact id from the artifact store; the outbound GET itself cannot be unsent.",
			Confidence:        0.8,
		},
	}
}

type fetchInput struct {
	URL  string `json:"url"`
	Name string `json:"name,omitempty"`
}

// Invoke implements agent.Tool.
func (t *Tool) Invoke(ctx context.Context, raw json.RawMessage) (agent.Result, error) {
	var in fetchInput
	if err := json.Unmarshal(raw, &in); err != nil {
		return agent.Result{}, fmt.Errorf("fetch: parse input: %w", err)
	}
	u := strings.TrimSpace(in.URL)
	if u == "" {
		return errResult("url required"), nil
	}
	if !strings.HasPrefix(u, "http://") && !strings.HasPrefix(u, "https://") {
		return errResult("url must be an absolute http/https URL"), nil
	}
	if t.index == nil {
		return errResult("artifact store unavailable"), nil
	}

	req, err := stdhttp.NewRequestWithContext(ctx, stdhttp.MethodGet, u, nil)
	if err != nil {
		return errResult("build request: " + err.Error()), nil
	}
	ua := t.UserAgent
	if ua == "" {
		ua = DefaultUserAgent
	}
	req.Header.Set("User-Agent", ua)

	resp, err := t.client().Do(req)
	if err != nil {
		return errResult("download failed: " + err.Error()), nil
	}
	defer resp.Body.Close()
	if resp.StatusCode >= 400 {
		return errResult(fmt.Sprintf("server returned HTTP %d", resp.StatusCode)), nil
	}
	data, err := io.ReadAll(io.LimitReader(resp.Body, MaxBytes+1))
	if err != nil {
		return errResult("read body: " + err.Error()), nil
	}
	if len(data) > MaxBytes {
		return errResult(fmt.Sprintf("download exceeds the %d MiB limit", MaxBytes>>20)), nil
	}
	if len(data) == 0 {
		return errResult("downloaded 0 bytes"), nil
	}

	mime := cleanMime(resp.Header.Get("Content-Type"))
	if mime == "" {
		mime = stdhttp.DetectContentType(data)
	}
	name := strings.TrimSpace(in.Name)
	if name == "" {
		name = nameFromURL(u)
	}

	e, err := t.index.PutEntry(artifact.Entry{
		Kind:   kindForMime(mime),
		Source: "fetch",
		Name:   name,
		Mime:   mime,
	}, data, t.Now())
	if err != nil {
		return errResult("save: " + err.Error()), nil
	}

	out, _ := json.MarshalIndent(map[string]any{
		"id":    e.ID,
		"ref":   e.Ref,
		"name":  e.Name,
		"mime":  e.Mime,
		"size":  e.Size,
		"saved": true,
		"note":  "saved to the Files view; reference it by id",
	}, "", "  ")
	return agent.Result{
		Output:            string(out),
		ObservationTrust:  agent.ObservationUntrusted,
		ObservationSource: u,
	}, nil
}

// cleanMime strips any "; charset=…" parameter from a Content-Type.
func cleanMime(ct string) string {
	ct = strings.TrimSpace(ct)
	if i := strings.IndexByte(ct, ';'); i >= 0 {
		ct = ct[:i]
	}
	return strings.ToLower(strings.TrimSpace(ct))
}

// kindForMime buckets a mime into the artifact-index Kind the file manager groups by.
func kindForMime(mime string) string {
	if strings.HasPrefix(mime, "image/") {
		return "image"
	}
	return "download"
}

// nameFromURL derives a file name from the URL's last path segment, or a default.
func nameFromURL(raw string) string {
	if pu, err := url.Parse(raw); err == nil {
		if base := path.Base(pu.Path); base != "" && base != "/" && base != "." {
			return base
		}
		if pu.Host != "" {
			return pu.Host
		}
	}
	return "download"
}

func errResult(msg string) agent.Result {
	return agent.Result{Output: "fetch: " + msg, IsError: true}
}
