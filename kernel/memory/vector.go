// SPDX-License-Identifier: MIT

package memory

// Vector retrieval (M803, DECISIONS C5: "local embeddings by default — zero
// marginal cost; provider embeddings opt-in"). The local embedder is a
// feature-hashed bag of word tokens + character n-grams: deterministic, pure
// Go (stdlib FNV), no model download, no network, no marginal cost — and
// language-agnostic, which matters for an owner who writes memories in
// Turkish and queries them in English-mixed shorthand. Character n-grams
// give what keyword overlap can't: typo tolerance ("kubenetes" still finds
// the kubernetes record) and morphology tolerance ("servisler" still finds
// "servis"). It does NOT give true synonym semantics — that is exactly the
// provider-embeddings opt-in (a future milestone), and the reason hybrid
// search keeps the keyword signal as a first-class term instead of replacing
// it.
//
// Embeddings are recomputed on demand: at recall scale (hundreds of records,
// one recall per run) hashing is microseconds per record, and recomputing
// keeps Search* pure functions of their inputs — no cache invalidation, no
// schema change, nothing new persisted in memory.json.

import (
	"hash/fnv"
	"math"
	"sort"
	"strings"
)

// EmbedDim is the embedding dimensionality. 256 signed-hash buckets keep
// collision noise low at this corpus size while staying cheap to dot.
const EmbedDim = 256

// minSemanticCosine is the noise floor: unrelated texts under feature
// hashing land well below it, morphology/typo neighbours well above.
const minSemanticCosine = 0.2

// tokenCosineDamp discounts per-token matches relative to whole-query
// cosine: a single salient token ("kubenetes" inside "what do you remember
// about kubenetes?") may carry the match, but never quite as strongly as a
// query that is ABOUT the record end to end.
const tokenCosineDamp = 0.85

// minTokenLen keeps per-token matching to salient words; the noise floor
// does the rest (a 4-letter function word rarely clears it against a whole
// record).
const minTokenLen = 4

// Embed maps text to an L2-normalized EmbedDim vector via signed feature
// hashing of its word tokens and per-token character 3-grams (with boundary
// markers, over runes so UTF-8 — ç/ğ/ş — hashes correctly). Deterministic;
// returns nil for text with no tokens.
func Embed(text string) []float32 {
	tokens := tokenize(text)
	if len(tokens) == 0 {
		return nil
	}
	vec := make([]float32, EmbedDim)
	for _, tok := range tokens {
		addFeature(vec, tok)
		runes := []rune("^" + tok + "$")
		for i := 0; i+3 <= len(runes); i++ {
			addFeature(vec, string(runes[i:i+3]))
		}
	}
	var norm float64
	for _, v := range vec {
		norm += float64(v) * float64(v)
	}
	if norm == 0 {
		return nil
	}
	scale := float32(1 / math.Sqrt(norm))
	for i := range vec {
		vec[i] *= scale
	}
	return vec
}

// addFeature hashes one feature into its bucket with a ±1 sign drawn from an
// independent hash bit — signed hashing keeps E[a·b]=0 for unrelated texts.
func addFeature(vec []float32, feature string) {
	h := fnv.New64a()
	_, _ = h.Write([]byte(feature))
	sum := h.Sum64()
	bucket := sum % EmbedDim
	if (sum>>32)&1 == 1 {
		vec[bucket]++
	} else {
		vec[bucket]--
	}
}

// Cosine returns the cosine similarity of two Embed outputs. Both are
// already L2-normalized, so this is a plain dot product; mismatched or nil
// vectors score 0.
func Cosine(a, b []float32) float64 {
	if len(a) == 0 || len(a) != len(b) {
		return 0
	}
	var dot float64
	for i := range a {
		dot += float64(a[i]) * float64(b[i])
	}
	return dot
}

// searchText is the record's retrieval surface — subject + content + tags —
// shared by keyword overlap and the embedder so the two signals always look
// at the same text.
func searchText(r Record) string {
	var b strings.Builder
	b.WriteString(r.Subject)
	b.WriteByte(' ')
	b.WriteString(r.Content)
	for k, v := range r.Tags {
		b.WriteByte(' ')
		b.WriteString(k)
		b.WriteByte(' ')
		b.WriteString(v)
	}
	return b.String()
}

// SearchSemantic ranks active records by embedding cosine against the query,
// weighted by the same confidence and recency factors as keyword Search.
// The similarity is the better of (a) the whole-query cosine and (b) the
// best damped per-token cosine — so a salient term buried in a chatty
// question ("what do you remember about kubenetes?") still finds its record
// instead of being diluted below the noise floor by function words.
// Records under the floor are excluded. Pure function; same tie rules as
// Search.
func SearchSemantic(rs []Record, query string, limit int, nowMS int64) []Scored {
	qv := Embed(query)
	out := make([]Scored, 0, len(rs))
	if qv == nil {
		return out
	}
	var tokVecs [][]float32
	for _, tok := range tokenize(query) {
		if len([]rune(tok)) >= minTokenLen {
			if tv := Embed(tok); tv != nil {
				tokVecs = append(tokVecs, tv)
			}
		}
	}
	for _, r := range rs {
		if !r.Active() {
			continue
		}
		rv := Embed(searchText(r))
		cos := Cosine(qv, rv)
		for _, tv := range tokVecs {
			if c := Cosine(tv, rv) * tokenCosineDamp; c > cos {
				cos = c
			}
		}
		if cos < minSemanticCosine {
			continue
		}
		conf := r.Confidence
		if conf <= 0 {
			conf = 0.5
		}
		score := cos * (0.5 + conf) * recencyFactor(r.LastSeenMS, nowMS)
		out = append(out, Scored{Record: r, Score: score})
	}
	sortScored(out)
	if limit > 0 && len(out) > limit {
		out = out[:limit]
	}
	return out
}

// SearchHybrid blends both signals: keyword overlap (exact-term precision)
// plus embedding cosine (typo/morphology recall). A record matching both
// sums both scores; a record only one signal sees still surfaces — which is
// the point: "kubenetes" has zero keyword overlap with the kubernetes
// record, and "izmir hava" should still rank the exact-keyword record first.
// Pure function; this is what Manager recall uses.
func SearchHybrid(rs []Record, query string, limit int, nowMS int64) []Scored {
	byID := make(map[string]Scored)
	for _, s := range Search(rs, query, 0, nowMS) {
		byID[s.Record.ID] = s
	}
	for _, s := range SearchSemantic(rs, query, 0, nowMS) {
		if prev, ok := byID[s.Record.ID]; ok {
			prev.Score += s.Score
			byID[s.Record.ID] = prev
		} else {
			byID[s.Record.ID] = s
		}
	}
	out := make([]Scored, 0, len(byID))
	for _, s := range byID {
		out = append(out, s)
	}
	sortScored(out)
	if limit > 0 && len(out) > limit {
		out = out[:limit]
	}
	return out
}

// sortScored applies the retrieval ordering shared by every Search variant:
// score desc, then LastSeenMS desc, then ID asc — stable, deterministic.
func sortScored(out []Scored) {
	sort.SliceStable(out, func(i, j int) bool {
		if out[i].Score != out[j].Score {
			return out[i].Score > out[j].Score
		}
		if out[i].Record.LastSeenMS != out[j].Record.LastSeenMS {
			return out[i].Record.LastSeenMS > out[j].Record.LastSeenMS
		}
		return out[i].Record.ID < out[j].Record.ID
	})
}
