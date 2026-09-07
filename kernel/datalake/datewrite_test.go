// SPDX-License-Identifier: MIT

package datalake

import "testing"

// A `date`-typed field must be canonical `YYYY-MM-DD` in the stored record.
// The lake is the AUTHORITATIVE write seam: the console editor is not the only
// way in — `handleDataInsert` (kernel/controlplane/datalake.go) and the agent's
// `db` tool (plugins/tools/db) both call Insert/Update directly, so normalizing
// only the UI would leave agent-written rows non-canonical and silently missing
// from every month filter and chronological sort downstream.
func TestInsertCanonicalizesDateTypedFields(t *testing.T) {
	l := newLake(t)
	_, err := l.CreateCollection(Schema{Name: "exp", Fields: []Field{
		{Name: "date", Type: "date", Label: "Date"},
		{Name: "item", Type: "text", Label: "Item"},
	}}, "a")
	if err != nil {
		t.Fatalf("CreateCollection: %v", err)
	}

	in := map[string]any{"date": "2026-9-5", "item": "2026-9-5"}
	r, err := l.Insert("exp", in, "a")
	if err != nil {
		t.Fatalf("Insert: %v", err)
	}
	if got := r.Fields["date"]; got != "2026-09-05" {
		t.Errorf("stored date = %v, want 2026-09-05", got)
	}
	// Only declared `date` fields are touched. The schema is explicitly advisory
	// and records may carry extra keys, so a text field holding something that
	// merely looks like a date is the operator's own content, not a typo to fix.
	if got := r.Fields["item"]; got != "2026-9-5" {
		t.Errorf("text field was rewritten to %v, want untouched", got)
	}
	// Insert used to alias the caller's map straight into the Record. A
	// normalizer that rewrote it in place would silently mutate the caller's
	// data as a side effect — its own bug, so pin it.
	if in["date"] != "2026-9-5" {
		t.Errorf("caller's map mutated: in[date] = %v, want 2026-9-5", in["date"])
	}
	// And it must be the canonical form on disk, not just in the returned value.
	got, err := l.Get("exp", r.ID)
	if err != nil {
		t.Fatalf("Get: %v", err)
	}
	if got.Fields["date"] != "2026-09-05" {
		t.Errorf("re-read date = %v, want 2026-09-05", got.Fields["date"])
	}
}

// Normalization must never LOSE data or guess an instant. Anything that is not
// exactly one whole date is stored as written — notably a timestamp, which the
// read-side dayKey deliberately truncates to its day: reusing that helper on
// write would have destroyed the time component.
func TestInsertLeavesAnythingButAWholeDateAlone(t *testing.T) {
	l := newLake(t)
	_, err := l.CreateCollection(Schema{Name: "exp", Fields: []Field{
		{Name: "date", Type: "date"},
	}}, "a")
	if err != nil {
		t.Fatalf("CreateCollection: %v", err)
	}

	for _, tc := range []struct{ in, want string }{
		{"2026-09-05", "2026-09-05"},             // already canonical: idempotent
		{"2026-9-5", "2026-09-05"},               // the whole point
		{"2026/9/5", "2026-09-05"},               // slash separator is the same date
		{"  2026-9-5  ", "2026-09-05"},           // surrounding space is not content
		{"2026-09-05T10:30", "2026-09-05T10:30"}, // truncating this loses the time
		{"2026-9-5 extra", "2026-9-5 extra"},     // not a whole date
		{"2026-13-05", "2026-13-05"},             // month out of range: never guessed
		{"2026-00-05", "2026-00-05"},             // nor is a zero month
		{"2026-09-32", "2026-09-32"},             // nor a 32nd day
		{"tomorrow", "tomorrow"},
		{"", ""},
	} {
		r, err := l.Insert("exp", map[string]any{"date": tc.in}, "a")
		if err != nil {
			t.Fatalf("Insert(%q): %v", tc.in, err)
		}
		if got := r.Fields["date"]; got != tc.want {
			t.Errorf("Insert(%q) stored %v, want %v", tc.in, got, tc.want)
		}
	}

	// A non-string value in a date field is not a string to rewrite: it must
	// survive untouched rather than error or become "".
	r, err := l.Insert("exp", map[string]any{"date": float64(20260905)}, "a")
	if err != nil {
		t.Fatalf("Insert numeric: %v", err)
	}
	if got := r.Fields["date"]; got != float64(20260905) {
		t.Errorf("numeric date stored %v, want it left alone", got)
	}
}

// Update is the second write door, and it merges a PATCH. Only the keys being
// written are normalized — an unrelated edit must not quietly rewrite a stored
// value the operator never touched.
func TestUpdateCanonicalizesDateTypedPatch(t *testing.T) {
	l := newLake(t)
	_, _ = l.CreateCollection(Schema{Name: "exp", Fields: []Field{
		{Name: "date", Type: "date"},
		{Name: "item", Type: "text"},
	}}, "a")
	// An unparseable date is stored as written (see the test above), which gives
	// a genuinely non-canonical row to reason about.
	r, err := l.Insert("exp", map[string]any{"date": "2026-13-45", "item": "x"}, "a")
	if err != nil {
		t.Fatalf("Insert: %v", err)
	}

	// Editing an unrelated field must leave that stored value alone.
	after, err := l.Update("exp", r.ID, map[string]any{"item": "y"}, "a")
	if err != nil {
		t.Fatalf("Update(item): %v", err)
	}
	if after.Fields["date"] != "2026-13-45" {
		t.Errorf("untouched date rewritten to %v, want 2026-13-45 preserved", after.Fields["date"])
	}

	// Writing the field is what normalizes it.
	after, err = l.Update("exp", r.ID, map[string]any{"date": "2026-10-2"}, "a")
	if err != nil {
		t.Fatalf("Update(date): %v", err)
	}
	if after.Fields["date"] != "2026-10-02" {
		t.Errorf("patched date = %v, want 2026-10-02", after.Fields["date"])
	}

	// The nil-means-delete contract still holds.
	after, err = l.Update("exp", r.ID, map[string]any{"date": nil}, "a")
	if err != nil {
		t.Fatalf("Update(nil): %v", err)
	}
	if _, ok := after.Fields["date"]; ok {
		t.Error("nil patch value must delete the key")
	}
}

// A collection created without a schema (records may carry extra keys) must not
// be rewritten either — there is no declared date field to justify it.
func TestInsertWithoutSchemaFieldsStoresVerbatim(t *testing.T) {
	l := newLake(t)
	if _, err := l.CreateCollection(Schema{Name: "raw"}, "a"); err != nil {
		t.Fatalf("CreateCollection: %v", err)
	}
	r, err := l.Insert("raw", map[string]any{"date": "2026-9-5"}, "a")
	if err != nil {
		t.Fatalf("Insert: %v", err)
	}
	if got := r.Fields["date"]; got != "2026-9-5" {
		t.Errorf("unschema'd collection rewrote date to %v, want verbatim", got)
	}
}
