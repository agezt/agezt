// SPDX-License-Identifier: MIT

package agent

import (
	"errors"
	"strings"
	"testing"
)

// recordingPutter captures what was offloaded; failNext makes Put error once.
type recordingPutter struct {
	puts [][]byte
	fail bool
}

func (r *recordingPutter) Put(data []byte) (string, error) {
	if r.fail {
		return "", errors.New("store down")
	}
	r.puts = append(r.puts, data)
	return "ref-" + itoa(len(data)), nil
}

func itoa(n int) string {
	if n == 0 {
		return "0"
	}
	var b []byte
	for n > 0 {
		b = append([]byte{byte('0' + n%10)}, b...)
		n /= 10
	}
	return string(b)
}

func TestOffloadToolOutput(t *testing.T) {
	big := strings.Repeat("x", DefaultArtifactThreshold+100)
	small := "tiny output"

	t.Run("no store inlines", func(t *testing.T) {
		out, ref, n, off := offloadToolOutput(nil, 0, big)
		if off || ref != "" || out != big || n != len(big) {
			t.Errorf("no-store: out/ref/off = %d/%q/%v, want inline", len(out), ref, off)
		}
	})

	t.Run("small output inlines", func(t *testing.T) {
		p := &recordingPutter{}
		out, ref, _, off := offloadToolOutput(p, 0, small)
		if off || ref != "" || out != small || len(p.puts) != 0 {
			t.Errorf("small: off=%v ref=%q puts=%d, want inline + no Put", off, ref, len(p.puts))
		}
	})

	t.Run("large output offloads", func(t *testing.T) {
		p := &recordingPutter{}
		out, ref, n, off := offloadToolOutput(p, 0, big)
		if !off {
			t.Fatal("large output should offload")
		}
		if ref == "" || n != len(big) {
			t.Errorf("ref=%q n=%d, want ref + full byte count %d", ref, n, len(big))
		}
		if len(out) >= len(big) || !strings.Contains(out, "offloaded") || !strings.Contains(out, ref) {
			t.Errorf("event output should be a short preview naming the ref, got %d chars", len(out))
		}
		if len(p.puts) != 1 || string(p.puts[0]) != big {
			t.Errorf("store should have received the full bytes once, got %d puts", len(p.puts))
		}
	})

	t.Run("Put failure falls back to inline", func(t *testing.T) {
		p := &recordingPutter{fail: true}
		out, ref, _, off := offloadToolOutput(p, 0, big)
		if off || ref != "" || out != big {
			t.Errorf("on Put error the output must inline unchanged (off=%v ref=%q)", off, ref)
		}
	})

	t.Run("custom threshold honoured", func(t *testing.T) {
		p := &recordingPutter{}
		// A 100-byte output offloads when the threshold is 10.
		_, ref, _, off := offloadToolOutput(p, 10, strings.Repeat("y", 100))
		if !off || ref == "" {
			t.Errorf("custom threshold not honoured: off=%v ref=%q", off, ref)
		}
	})
}
