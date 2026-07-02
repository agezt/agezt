// SPDX-License-Identifier: MIT

package runtime

import (
	"reflect"
	"strings"
	"testing"
)

func TestParseSubQuestions_JSONArray(t *testing.T) {
	got := parseSubQuestions(`["what is X?", "how does X work?"]`, "explain X", 3)
	// Original question is always first, then the parsed sub-questions.
	want := []string{"explain X", "what is X?", "how does X work?"}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("got %#v, want %#v", got, want)
	}
}

func TestParseSubQuestions_FencedJSON(t *testing.T) {
	raw := "Here you go:\n```json\n[\"a question here\", \"b question here\"]\n```\n"
	got := parseSubQuestions(raw, "root question", 5)
	if len(got) != 3 || got[0] != "root question" {
		t.Fatalf("got %#v", got)
	}
}

func TestParseSubQuestions_NumberedList(t *testing.T) {
	raw := "1. first sub-question\n2) second sub-question\n- third sub-question"
	got := parseSubQuestions(raw, "top question", 10)
	if len(got) != 4 {
		t.Fatalf("expected question + 3 items, got %#v", got)
	}
	if got[1] != "first sub-question" {
		t.Fatalf("bullet/number prefix not stripped: %q", got[1])
	}
}

func TestParseSubQuestions_CapAndDedup(t *testing.T) {
	// Duplicate of the question (case-insensitive) must not repeat; cap honored.
	got := parseSubQuestions(`["Root Q", "extra one", "extra two"]`, "root q", 2)
	if len(got) != 2 {
		t.Fatalf("cap not honored: %#v", got)
	}
	if got[0] != "root q" {
		t.Fatalf("question not first: %#v", got)
	}
}

func TestParseSubQuestions_EmptyFallsBackToQuestion(t *testing.T) {
	got := parseSubQuestions("   ", "only question", 3)
	if len(got) != 1 || got[0] != "only question" {
		t.Fatalf("got %#v", got)
	}
}

func TestParseSearchHits_FiltersNonHTTP(t *testing.T) {
	out := `[
      {"title":"A","url":"https://a.example/x","snippet":"sa"},
      {"title":"B","url":"ftp://b.example","snippet":"sb"},
      {"title":"C","url":"http://c.example","snippet":"sc"}
    ]`
	got := parseSearchHits(out)
	if len(got) != 2 {
		t.Fatalf("expected 2 http(s) hits, got %d: %#v", len(got), got)
	}
	if got[0].URL != "https://a.example/x" || got[1].URL != "http://c.example" {
		t.Fatalf("wrong hits: %#v", got)
	}
}

func TestParseSearchHits_MalformedIsEmpty(t *testing.T) {
	if got := parseSearchHits("not json at all"); len(got) != 0 {
		t.Fatalf("expected empty, got %#v", got)
	}
	if got := parseSearchHits(""); len(got) != 0 {
		t.Fatalf("expected empty for blank, got %#v", got)
	}
}

func TestExtractCitedSources(t *testing.T) {
	md := "Claim one [S1] and claim two [S3]. Repeat [S1]. Bogus [S9] beyond range."
	got := extractCitedSources(md, 3) // only S1..S3 valid
	want := []int{1, 3}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("got %#v, want %#v", got, want)
	}
}

func TestExtractCitedSources_None(t *testing.T) {
	if got := extractCitedSources("no citations here", 5); len(got) != 0 {
		t.Fatalf("expected none, got %#v", got)
	}
}

func TestResearchConfidence(t *testing.T) {
	cases := []struct {
		cited, total int
		want         float64
	}{
		{0, 0, 0},
		{0, 4, 0},
		{2, 4, 0.5},
		{4, 4, 1},
		{5, 4, 1}, // clamp
	}
	for _, c := range cases {
		if got := researchConfidence(c.cited, c.total); got != c.want {
			t.Errorf("researchConfidence(%d,%d)=%v want %v", c.cited, c.total, got, c.want)
		}
	}
}

func TestBuildResearchSynthPrompt_IncludesSourcesAndRules(t *testing.T) {
	src := []ResearchSource{
		{ID: "S1", URL: "https://a.example", Title: "Alpha", Text: "alpha body"},
		{ID: "S2", URL: "https://b.example", Title: "", Text: "beta body"},
	}
	p := buildResearchSynthPrompt("what?", src)
	for _, want := range []string{"[S1]", "Alpha", "https://a.example", "https://b.example", "[S#] citation"} {
		if !strings.Contains(p, want) {
			t.Errorf("synth prompt missing %q", want)
		}
	}
}

func TestResearchOptions_Defaults(t *testing.T) {
	o := ResearchOptions{}.withDefaults()
	if o.MaxSubQuestions != 3 || o.ResultsPerQuery != 4 || o.MaxSources != 8 || o.MaxVerifyClaims != 6 {
		t.Fatalf("bad defaults: %#v", o)
	}
	capped := ResearchOptions{MaxSubQuestions: 99, MaxSources: 99, MaxVerifyClaims: 99}.withDefaults()
	if capped.MaxSubQuestions != 8 || capped.MaxSources != 20 || capped.MaxVerifyClaims != 12 {
		t.Fatalf("caps not applied: %#v", capped)
	}
}

func TestExtractClaims(t *testing.T) {
	md := "The sky is blue [S1]. Water is wet [S2]! This has no citation. Confidence: high [S1]."
	got := extractClaims(md, 2)
	if len(got) != 2 {
		t.Fatalf("expected 2 cited claims (skip uncited + Confidence line), got %d: %#v", len(got), got)
	}
	if got[0].SourceIDs[0] != "S1" || got[1].SourceIDs[0] != "S2" {
		t.Fatalf("wrong source ids: %#v", got)
	}
	if !strings.HasSuffix(got[0].Text, ".") {
		t.Fatalf("claim text should end with period: %q", got[0].Text)
	}
}

func TestExtractClaims_OutOfRangeCitationDropped(t *testing.T) {
	// [S9] is out of range for n=1, so the sentence has no valid citation.
	md := "Bold assertion [S9]. Grounded one [S1]."
	got := extractClaims(md, 1)
	if len(got) != 1 || got[0].SourceIDs[0] != "S1" {
		t.Fatalf("expected only the S1 claim, got %#v", got)
	}
}

func TestParseResearchVerdict(t *testing.T) {
	cases := []struct {
		reply       string
		wantVerdict string
	}{
		{"SUPPORTED\nThe source states this directly.", "supported"},
		{"REFUTED - the source says the opposite", "refuted"},
		{"UNSUPPORTED by the cited text", "refuted"}, // contains SUPPORTED but must be refuted
		{"NOT SUPPORTED anywhere", "refuted"},
		{"The source CONTRADICTS the claim", "refuted"},
		{"UNCERTAIN\nsource is insufficient", "uncertain"},
		{"I cannot tell from this", "uncertain"},
	}
	for _, c := range cases {
		gotV, _ := parseResearchVerdict(c.reply)
		if gotV != c.wantVerdict {
			t.Errorf("parseResearchVerdict(%q) verdict=%q want %q", c.reply, gotV, c.wantVerdict)
		}
	}
	// Reason is the first non-verdict line.
	_, note := parseResearchVerdict("SUPPORTED\nBecause the abstract says so.")
	if note != "Because the abstract says so." {
		t.Fatalf("note = %q", note)
	}
}
