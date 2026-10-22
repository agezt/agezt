# web-research recipes

The helper (`scripts/extract.py`) covers fetch → clean-text. The judgment —
what to search, which sources to trust, how to cite — is the skill. Patterns:

## Search strategy

- Turn the question into **specific queries**, not the whole question. "fastest
  JSON parser Go 2025 benchmark" beats "what is the best way to parse JSON".
- Use the available search/fetch tools to find candidate pages, then `extract.py`
  to pull their text.
- Prefer **primary sources**: the project's own docs/repo, the standards body,
  the vendor — over blogs and aggregators that summarize them.
- Cross-check a surprising claim against a second independent source before you
  rely on it.

## Batch extract several candidates

```sh
python scripts/extract.py '{"urls":[
  "https://a.example/post",
  "https://b.example/docs",
  "https://c.example/spec"
], "max_chars":4000}'
```
Each result carries `url`, `status`, `title`, and clean `text` you can quote.

## Citation format

Keep a running list while you read, then end the answer with:

```
Sources:
- [Title](https://url-1)
- [Title](https://url-2)
```
Every non-obvious claim in the answer should map to one of these.

## When fetch returns almost nothing

A page that returns a few hundred chars of boilerplate is usually JS-rendered or
gated. Don't retry raw fetch — switch to the **browser-use** skill, which drives a
real browser and sees the rendered DOM. For infinite-scroll/lazy content, scroll
in the browser before reading.

## PDFs and docs

`extract.py` handles HTML. For a PDF link, download it and read it with a PDF
tool/pandas/pdfminer in `code_exec`:
```sh
curl -L -o paper.pdf "https://example.com/paper.pdf"
python -c "import pdfminer.high_level as h; print(h.extract_text('paper.pdf')[:4000])"
```

## Long sources — summarize before synthesizing

For a long article, extract it, then summarize *that text* into the 3–5 points
relevant to your sub-question before combining sources. Synthesize from your
notes, not from raw page dumps — it keeps the final answer tight and traceable.

## Be honest about gaps

If the sources don't answer the question, say what you found and what's still
open. A sourced "couldn't confirm X; closest is Y" is more useful than a
confident guess.
