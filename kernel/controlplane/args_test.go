// SPDX-License-Identifier: MIT

package controlplane

import "testing"

func TestArgString(t *testing.T) {
	args := map[string]any{"s": "hello", "n": float64(5), "b": true}

	if v, ok, err := argString(args, "s"); v != "hello" || !ok || err != nil {
		t.Errorf("present string: got (%q,%v,%v)", v, ok, err)
	}
	if v, ok, err := argString(args, "missing"); v != "" || ok || err != nil {
		t.Errorf("absent: got (%q,%v,%v), want ('',false,nil)", v, ok, err)
	}
	// Present but wrong type → ok=true (present) AND err!=nil (so the caller errors
	// instead of using the zero value).
	if _, ok, err := argString(args, "n"); !ok || err == nil {
		t.Errorf("number-as-string: got (ok=%v,err=%v), want (true, non-nil)", ok, err)
	}
	if _, _, err := argString(args, "b"); err == nil {
		t.Error("bool-as-string: want error")
	}
}
