# M344 — Memory distillation edge-case coverage (SPEC-05)

## Why
Priority-A coverage on SPEC-05 memory. `Manager.Distill` turns a run transcript
into durable memory records via an LLM, then parses the model's JSON with
`parseDistill` (a brace-scan tolerant of surrounding prose/fences). Reading the
code against `manager_test.go`, the happy path (pure + prose-wrapped JSON), the
blank-content skip, and the no-braces non-JSON no-op were already covered — but two
genuine behaviours were not:

1. **Invalid type coercion** (manager.go:454-457): a fact whose `type` is not a
   canonical `memory.Type` must be **stored as `SUMMARY`**, not dropped. A model's
   type vocabulary drifts; losing the fact would silently shrink memory.
2. **Braces-with-malformed-JSON no-op** (parseDistill): a response that *has*
   braces but whose contents don't parse must fail closed (store nothing) —
   distinct from the no-braces case, and the path that protects the brace-scan
   from feeding garbage into `json.Unmarshal`.

## What
Test-only. Added to `kernel/memory/manager_test.go`:
- **`TestDistillInvalidTypeCoercedToSummary`** — a fact with `"type":"BOGUS_TYPE"`
  is stored (1 id) and the persisted record's `Type` is `TypeSummary`, proving the
  `ValidType` fallback rather than a drop or a stored-invalid-type.
- **`TestDistillMalformedJSONObjectIsNoOp`** — a response `here you go: {facts:
  not, valid json,,}` (braces, invalid contents) yields 0 ids, no error, and
  `Count()==0` — a clean fail-closed no-op.

## Verification
- `go test ./kernel/memory -run Distill -v` — all four distill tests pass (2
  pre-existing + 2 new).
- `gofmt -l` clean; `go vet ./kernel/memory/` clean; `GOOS=linux go build ./...`
  exit 0. Full suite **2068** passing (was 2066; +2), `go test ./...` exit 0.
  `go.mod` / `go.sum` unchanged.

## Scope notes
- No production change — both behaviours already worked; this pins them. The rest
  of SPEC-05 memory (Remember/dedup/Forget/Recall/Supersede/validation, the
  memory tool, FileStore persistence, Search ranking/filters/limit/empty) was
  already well-covered, so this milestone targets only the two genuinely-uncovered
  distillation edges.
