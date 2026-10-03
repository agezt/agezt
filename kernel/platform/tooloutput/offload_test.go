// SPDX-License-Identifier: MIT

package tooloutput_test

import (
	"errors"
	"strings"
	"testing"

	"github.com/agezt/agezt/kernel/platform/tooloutput"
)

type recordingStore struct {
	bytes []byte
	calls int
	mode  string
}

func (s *recordingStore) Put(b []byte) (string, error) {
	s.calls++
	s.bytes = append([]byte(nil), b...)
	if s.mode == "fail" {
		return "", errors.New("unavailable")
	}
	if s.mode == "empty" {
		return "", nil
	}
	return "ref", nil
}

func TestOffloadPreservesRepresentation(t *testing.T) {
	for _, tc := range []struct {
		name, mode      string
		size, threshold int
		want            bool
	}{
		{"default-large", "", 9000, 0, true}, {"default-boundary", "", 8192, 0, false},
		{"small", "", 30, 0, false}, {"custom", "", 800, 16, true}, {"custom-boundary", "", 800, 800, false},
		{"negative-default", "", 9000, -1, true}, {"store-failure", "fail", 9000, 0, false}, {"empty-ref", "empty", 9000, 0, false},
	} {
		t.Run(tc.name, func(t *testing.T) {
			full := strings.Repeat("x", tc.size)
			store := &recordingStore{mode: tc.mode}
			out, ref, n, off := tooloutput.Offload(store, tc.threshold, full)
			if n != len(full) || off != tc.want {
				t.Fatalf("bytes=%d offload=%v", n, off)
			}
			if tc.want {
				if ref != "ref" || len(out) >= len(full) || !strings.Contains(out, "artifact ref") || store.calls != 1 || string(store.bytes) != full {
					t.Errorf("output=%d ref=%q calls=%d bytes=%d", len(out), ref, store.calls, len(store.bytes))
				}
			}
			if !tc.want && (out != full || ref != "") {
				t.Error("inline fallback changed")
			}
			if tc.threshold == tc.size || tc.size <= tooloutput.DefaultArtifactThreshold && tc.threshold == 0 {
				if store.calls != 0 {
					t.Error("inline output attempted artifact write")
				}
			}
		})
	}
	full := strings.Repeat("x", 9000)
	if out, ref, n, off := tooloutput.Offload(nil, 0, full); out != full || ref != "" || n != len(full) || off {
		t.Error("nil-store fallback changed")
	}
}
