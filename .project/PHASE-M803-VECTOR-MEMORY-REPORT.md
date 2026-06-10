# Phase M803 — vector memory (local hybrid recall)

**Date:** 2026-06-10 · **Status:** DONE · **Arc:** vision gap #5.

## What — typo/morphology-tolerant recall, zero marginal cost

DECISIONS C5 said "local embeddings by default (zero marginal cost);
provider embeddings opt-in". This ships the default.

**kernel/memory/vector.go**:
- `Embed(text)` — signed feature hashing of word tokens + per-token
  character 3-grams (boundary-marked, over RUNES so ç/ğ/ş hash
  correctly) into an L2-normalized 256-dim vector. Pure Go (stdlib FNV),
  deterministic, no model download, no network, microseconds per record.
- `Cosine(a,b)` — dot of normalized vectors.
- `SearchSemantic` — cosine × the SAME confidence/recency weights as
  keyword Search, noise floor 0.2. Similarity is the better of the
  whole-query cosine and the best **damped per-token cosine** (×0.85,
  tokens ≥4 runes) — a salient term buried in a chatty question ("what
  do you remember about kubenetes?") isn't diluted below the floor by
  function words.
- `SearchHybrid` — keyword + semantic blended by record: both-signal
  records sum both scores; a record only one signal sees still surfaces.
  This is what `Manager.Recall*` and `Manager.Search` now use — the
  agent's pre-run recall, the memory tool, `agt memory search`, and the
  console all get it for free.
- Embeddings are recomputed on demand (pure functions, no cache, no
  schema change, nothing new in memory.json) — at recall scale
  (hundreds of records, one recall per run) hashing cost is noise.
- `searchText(r)` extracted so keyword overlap and the embedder always
  look at the same record surface.

What it gives: typo tolerance ("kubenetes" finds the kubernetes record
with ZERO keyword overlap) and morphology tolerance (Turkish
inflections: "servisinin yeniden başlatılması" finds "servisi …
yeniden başlatılıyor"). What it doesn't: true synonym semantics — that
is exactly the provider-embeddings opt-in, left as the documented
follow-up; hybrid keeps the keyword signal first-class for that reason.

## Tests (6 new suites; memory + full battery green)

- Embed determinism + L2 norm + nil for empty/noise text
- Cosine: self=1, related (typo/morph incl. Turkish) ≥ floor, unrelated
  < floor, nil→0
- SearchSemantic: typo query finds the record keyword Search misses;
  tombstoned/unrelated stay out
- ChattyQueryNotDiluted: per-token rescue finds it, unrelated chatty
  query stays empty (the probe that motivated the fix, regression-pinned)
- SearchHybrid: superset of keyword hits, both-signal outranks
  single-signal, limit after merge
- Determinism: identical calls → identical ordering (LastSeenMS/ID ties)

## Smoke (isolated AGEZT_HOME, real daemon)

Seeded EN + TR memories. `agt memory search "kubenetes clstr"` → found
the kubernetes record (0.53, pure semantic — keyword finds nothing);
`"ödeme servisinin yeniden başlatılması"` → found "ödeme servisi her
gece 03:00'te yeniden başlatılıyor" (3.82, hybrid); negative control
("uzay roketi yakıt formülü") → no records. Run-time path: `agt run
"what do you remember about kubenetes?"` → memory.retrieved journaled,
the model answered FROM MEMORY ("the cluster runs in Frankfurt").

Mid-smoke find: the first daemon run journaled NO memory.retrieved for
the chatty query — whole-query cosine was 0.143, under the floor. A
go-run probe confirmed the dilution; the per-token rescue fixed it and
the regression test pins it.

## Gate

Full `GOMAXPROCS=3 go test -p 2 ./...` green; vet + staticcheck clean;
linux cross-build OK; frontend untouched (recall shape unchanged);
go.mod unchanged (stdlib FNV only); no new env vars.

## Next

Vision gap #6: brain distiller standing surface. Memory polish backlog:
provider embeddings opt-in (C5's second half), HNSW if corpora outgrow
the exact scan, IDF weighting if tag noise ever shows up in ranking.
