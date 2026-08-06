// SPDX-License-Identifier: MIT

// The kernel-to-HTTP-API adapter (Phase 2.6 file split out of main.go): one
// small struct serves BOTH kernel/restapi.Engine and kernel/openaiapi.Engine
// (+ UsageReporter), so the two HTTP surfaces share identical run/journal/
// artifact semantics. Deliberately NOT moved into kernel/restapi: that package
// is interface-only by design, and importing kernel/runtime there would invert
// its posture and couple it to openaiapi.

package main

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"sort"
	"time"

	"github.com/agezt/agezt/kernel/artifact"
	"github.com/agezt/agezt/kernel/bus"
	"github.com/agezt/agezt/kernel/event"
	"github.com/agezt/agezt/kernel/restapi"
	kernelruntime "github.com/agezt/agezt/kernel/runtime"
	"github.com/agezt/agezt/kernel/tenant"
)

// tenantAPIEngine resolves an X-Agezt-Tenant id to that tenant's isolated
// kernel adapter + bus (opened on demand). Shared by the OpenAI API and REST
// API tenant resolvers, which were verbatim copies before the Phase 2.6
// dedupe — kernelAPIEngine satisfies both surfaces' Engine interfaces.
func tenantAPIEngine(reg *tenant.Registry, id string) (kernelAPIEngine, *bus.Bus, error) {
	t, err := reg.Acquire(id, time.Now())
	if err != nil {
		return kernelAPIEngine{}, nil, err
	}
	tk, ok := t.Kernel.(*kernelruntime.Kernel)
	if !ok {
		return kernelAPIEngine{}, nil, fmt.Errorf("tenant %q: unexpected kernel type", id)
	}
	return kernelAPIEngine{tk}, tk.Bus(), nil
}

// kernelAPIEngine adapts *kernelruntime.Kernel to openaiapi.Engine: it adds
// DefaultModel/ModelIDs (drawn from the configured model + synced catalog) on
// top of the run/correlation methods the kernel already exposes.
type kernelAPIEngine struct{ k *kernelruntime.Kernel }

func (e kernelAPIEngine) NewCorrelation() string        { return e.k.NewCorrelation() }
func (e kernelAPIEngine) SubjectForRun(c string) string { return e.k.SubjectForRun(c) }
func (e kernelAPIEngine) RunModel(ctx context.Context, corr, intent, model string, images []string, jsonMode bool) (string, error) {
	// Honour the requested model for this run (empty → kernel default).
	ctx = kernelruntime.WithModel(ctx, model)
	// Structured-output request (M314): a client's response_format flows to the
	// provider's CompletionRequest.JSONMode. No-op when false.
	ctx = kernelruntime.WithJSONMode(ctx, jsonMode)
	// Carry any multimodal attachments (M246) the same way the control plane
	// does, so a vision request to the OpenAI-compatible API reaches the model.
	if len(images) > 0 {
		// Pre-gate vision capability (M255): the API path bypasses the control
		// plane's M91 gate, so reject a non-vision model here with a clear error
		// rather than wasting a provider call.
		if err := visionGate(e.k, model, images); err != nil {
			return "", err
		}
		ctx = kernelruntime.WithImages(ctx, images)
	}
	return e.k.RunWith(ctx, corr, intent)
}

// UsageFor implements openaiapi.UsageReporter (M282): sum the REAL provider
// token usage for a run by folding its budget.consumed events (each LLM call the
// governor priced). Returns ok=false when nothing was consumed (a free/local/
// mock model) so the API falls back to its estimate instead of reporting 0/0.
func (e kernelAPIEngine) UsageFor(corr string) (int, int, bool) {
	// Fast path: the Governor keeps a bounded in-memory per-correlation usage
	// index, so usage for a just-completed run is O(1) instead of an O(journal)
	// scan per API response (which a client hammering the API could amplify into a
	// DoS). The journal scan below stays the authoritative fallback for any
	// correlation not in the bounded index, so the reported numbers are identical.
	if ur, ok := e.k.Provider().(interface {
		UsageFor(string) (int, int, bool)
	}); ok {
		if in, out, ok := ur.UsageFor(corr); ok && (in != 0 || out != 0) {
			return in, out, true
		}
	}
	in, out, found := 0, 0, false
	_ = e.k.Journal().Range(func(ev *event.Event) error {
		if ev.Kind != event.KindBudgetConsumed {
			return nil
		}
		var p struct {
			CorrelationID string `json:"correlation_id"`
			InputTokens   int    `json:"input_tokens"`
			OutputTokens  int    `json:"output_tokens"`
		}
		if json.Unmarshal(ev.Payload, &p) != nil || p.CorrelationID != corr {
			return nil
		}
		in += p.InputTokens
		out += p.OutputTokens
		found = true
		return nil
	})
	if !found || (in == 0 && out == 0) {
		return 0, 0, false
	}
	return in, out, true
}

func (e kernelAPIEngine) DefaultModel() string { return e.k.Model() }
func (e kernelAPIEngine) ModelIDs() []string {
	cat := e.k.Catalog()
	if cat == nil {
		return nil
	}
	var ids []string
	for _, p := range cat.ProviderList() {
		for id := range p.Models {
			ids = append(ids, id)
		}
	}
	sort.Strings(ids)
	return ids
}

// EventsForCorrelation returns the journaled events of a run, in order, by
// ranging the journal (the restapi run-inspection route, P7-API-02). Empty when
// the correlation is unknown.
func (e kernelAPIEngine) EventsForCorrelation(corr string) ([]*event.Event, error) {
	var out []*event.Event
	err := e.k.Journal().Range(func(ev *event.Event) error {
		if ev.CorrelationID == corr {
			out = append(out, ev)
		}
		return nil
	})
	return out, err
}

func (e kernelAPIEngine) ArtifactEntries(kind, source, corr string) ([]restapi.ArtifactEntry, error) {
	idx := e.k.ArtifactIndex()
	if idx == nil {
		return nil, fmt.Errorf("artifact index unavailable")
	}
	ents := idx.List(artifact.Filter{Kind: kind, Source: source, Corr: corr})
	out := make([]restapi.ArtifactEntry, 0, len(ents))
	for _, a := range ents {
		out = append(out, restArtifactEntry(a))
	}
	return out, nil
}

func (e kernelAPIEngine) ArtifactBytes(id string, maxBytes int64) ([]byte, restapi.ArtifactEntry, error) {
	idx := e.k.ArtifactIndex()
	if idx == nil {
		return nil, restapi.ArtifactEntry{}, fmt.Errorf("artifact index unavailable")
	}
	meta, ok := idx.Get(id)
	if !ok {
		return nil, restapi.ArtifactEntry{}, restapi.ErrArtifactNotFound
	}
	if maxBytes > 0 && meta.Size > maxBytes {
		return nil, restArtifactEntry(meta), restapi.ErrArtifactTooLarge
	}
	data, meta, err := idx.Bytes(id)
	if err != nil {
		if errors.Is(err, artifact.ErrNotFound) {
			return nil, restapi.ArtifactEntry{}, restapi.ErrArtifactNotFound
		}
		return nil, restapi.ArtifactEntry{}, err
	}
	if maxBytes > 0 && int64(len(data)) > maxBytes {
		return nil, restArtifactEntry(meta), restapi.ErrArtifactTooLarge
	}
	return data, restArtifactEntry(meta), nil
}

func restArtifactEntry(a artifact.Entry) restapi.ArtifactEntry {
	return restapi.ArtifactEntry{
		ID:        a.ID,
		Ref:       a.Ref,
		Name:      a.Name,
		Mime:      a.Mime,
		Kind:      a.Kind,
		Source:    a.Source,
		Sender:    a.Sender,
		Corr:      a.Corr,
		Size:      a.Size,
		CreatedMs: a.CreatedMs,
		Caption:   a.Caption,
	}
}
