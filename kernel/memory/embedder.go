// SPDX-License-Identifier: MIT

package memory

// Provider embeddings opt-in (M884) — the second half of DECISIONS C5
// ("local embeddings by default — zero marginal cost; provider embeddings
// opt-in"). The local feature-hash embedder (vector.go) gives typo and
// morphology tolerance but not true synonym semantics; a real embedding
// model does. The kernel stays decoupled: it defines the Embedder seam and
// the daemon injects an implementation (runtime.Config.MemoryEmbedder),
// typically backed by a provider plugin.
//
// Vectors are cached in memory keyed by record ID. Records are
// content-addressed (ID = BLAKE3 of type+subject+content), so a record's
// text can never change under its ID — the cache needs no invalidation and
// nothing new is persisted in memory.json. A recall embeds only the query
// plus whichever records the cache hasn't seen yet, batched in one call.
//
// Failure posture: recall NEVER fails because the embedder did. Any error
// (network, auth, shape mismatch) falls back to the local hybrid for that
// recall; the memory.retrieved event records which engine actually ranked.

import (
	"context"
	"fmt"
	"time"
)

// Embedder turns texts into semantic vectors. Implementations MUST return
// exactly one L2-normalized vector per input text, all of one consistent
// dimensionality, in input order. Batch + ctx + error fit a remote API's
// reality (one round trip per recall, cancellable, fallible).
type Embedder interface {
	EmbedBatch(ctx context.Context, texts []string) ([][]float32, error)
}

// embedTimeout bounds one recall's embedding round trip: long enough for a
// cold remote call over a few hundred uncached records, short enough that a
// wedged endpoint degrades recall to local instead of stalling the run.
const embedTimeout = 10 * time.Second

// SetEmbedder installs (or, with nil, removes) the provider embedder. Safe
// to call at runtime — the live-config pattern — so a daemon can hot-enable
// provider embeddings when a key arrives without restarting.
func (m *Manager) SetEmbedder(e Embedder) {
	m.embMu.Lock()
	defer m.embMu.Unlock()
	m.embedder = e
	m.embCache = nil // a different embedder's vectors are not comparable
}

func (m *Manager) getEmbedder() Embedder {
	m.embMu.Lock()
	defer m.embMu.Unlock()
	return m.embedder
}

// semanticProvider ranks active records by provider-embedding cosine against
// the query — the provider-grade analogue of SearchSemantic. It embeds the
// query plus any cache-miss records in ONE batch, fills the cache, and scores
// with the same confidence/recency weighting as every other Search variant.
func (m *Manager) semanticProvider(ctx context.Context, emb Embedder, recs []Record, query string, nowMS int64) ([]Scored, error) {
	active := make([]Record, 0, len(recs))
	for _, r := range recs {
		if r.Active() {
			active = append(active, r)
		}
	}

	// Assemble the batch: query first, then cache misses in record order.
	m.embMu.Lock()
	if m.embCache == nil {
		m.embCache = make(map[string][]float32)
	}
	texts := []string{query}
	missIDs := make([]string, 0, len(active))
	for _, r := range active {
		if _, ok := m.embCache[r.ID]; !ok {
			texts = append(texts, searchText(r))
			missIDs = append(missIDs, r.ID)
		}
	}
	m.embMu.Unlock()

	vecs, err := emb.EmbedBatch(ctx, texts)
	if err != nil {
		return nil, err
	}
	if len(vecs) != len(texts) {
		return nil, fmt.Errorf("memory: embedder returned %d vectors for %d texts", len(vecs), len(texts))
	}
	qv := vecs[0]

	m.embMu.Lock()
	for i, id := range missIDs {
		m.embCache[id] = vecs[i+1]
	}
	m.embMu.Unlock()

	out := make([]Scored, 0, len(active))
	for _, r := range active {
		cos := Cosine(qv, m.embCache[r.ID])
		if cos < minSemanticCosine {
			continue
		}
		conf := r.Confidence
		if conf <= 0 {
			conf = 0.5
		}
		out = append(out, Scored{Record: r, Score: cos * (0.5 + conf) * recencyFactor(r.LastSeenMS, nowMS)})
	}
	sortScored(out)
	return out, nil
}
