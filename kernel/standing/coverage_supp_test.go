// SPDX-License-Identifier: MIT

package standing

import (
	"os"
	"path/filepath"
	"testing"
)

func TestOpen_MalformedJSON(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "standing.json")
	if err := os.WriteFile(path, []byte(`not json`), 0o644); err != nil {
		t.Fatal(err)
	}
	_, err := Open(dir)
	if err == nil {
		t.Fatal("Open with malformed JSON should error")
	}
}

func TestList_Empty(t *testing.T) {
	s, err := Open(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	out := s.List()
	if len(out) != 0 {
		t.Fatalf("List on empty store = %d, want 0", len(out))
	}
}

func valSet(t *testing.T, field string, lo, hi int) map[int]bool {
	t.Helper()
	s, ok := cronField(field, lo, hi)
	if !ok {
		t.Fatalf("cronField(%q, %d, %d) returned ok=false", field, lo, hi)
	}
	return s
}

func TestCronField_AllStars(t *testing.T) {
	s := valSet(t, "*", 0, 59)
	if !s[0] || !s[30] || !s[59] {
		t.Error("cronField(*) should accept any value")
	}
}

func TestCronField_SingleValue(t *testing.T) {
	s := valSet(t, "5", 0, 59)
	if !s[5] {
		t.Error("cronField('5') should accept 5")
	}
	if s[4] || s[6] {
		t.Error("cronField('5') should reject neighbours")
	}
}

func TestCronField_StepRange(t *testing.T) {
	s := valSet(t, "*/15", 0, 59)
	if !s[0] || !s[15] || !s[30] || !s[45] {
		t.Error("cronField('*/15') should accept multiples of 15")
	}
	if s[7] || s[59] {
		t.Error("cronField('*/15') should reject non-multiples")
	}
}

func TestCronField_List(t *testing.T) {
	s := valSet(t, "1,3,5", 0, 59)
	if !s[1] || !s[3] || !s[5] {
		t.Error("cronField('1,3,5') should accept listed values")
	}
	if s[2] || s[4] {
		t.Error("cronField('1,3,5') should reject non-listed values")
	}
}

func TestCronField_Range(t *testing.T) {
	s := valSet(t, "10-20", 0, 59)
	if !s[10] || !s[15] || !s[20] {
		t.Error("cronField('10-20') should accept range values")
	}
	if s[9] || s[21] {
		t.Error("cronField('10-20') should reject out-of-range")
	}
}
