// SPDX-License-Identifier: MIT

package controlplane

import (
	"encoding/json"
	"testing"
	"time"
)

func TestScheduleArgNumberAcceptsCommonNumericTypes(t *testing.T) {
	tests := map[string]any{
		"float64":     float64(3600),
		"int":         int(3600),
		"int64":       int64(3600),
		"json.Number": json.Number("3600"),
	}
	for name, value := range tests {
		t.Run(name, func(t *testing.T) {
			got, ok, err := scheduleArgNumber(map[string]any{"interval_sec": value}, "interval_sec")
			if err != nil {
				t.Fatalf("scheduleArgNumber returned error: %v", err)
			}
			if !ok {
				t.Fatal("scheduleArgNumber ok=false, want true")
			}
			if got != 3600 {
				t.Fatalf("scheduleArgNumber = %v, want 3600", got)
			}
		})
	}
}

func TestScheduleArgNumberRejectsNonNumericValues(t *testing.T) {
	if _, _, err := scheduleArgNumber(map[string]any{"interval_sec": "3600"}, "interval_sec"); err == nil {
		t.Fatal("scheduleArgNumber accepted a string")
	}
	if _, _, err := scheduleArgNumber(map[string]any{"interval_sec": json.Number("nope")}, "interval_sec"); err == nil {
		t.Fatal("scheduleArgNumber accepted a bad json.Number")
	}
}

func TestValidateScheduleEditCadenceArgsAcceptsIntegerTypes(t *testing.T) {
	now := time.Now()
	tests := []map[string]any{
		{"interval_sec": int(3600)},
		{"cooldown_sec": int64(60)},
		{"at_minutes": int(540), "days": int64(62)},
		{"window_start": int(540), "window_end": int64(1020), "interval_sec": json.Number("900"), "days": int(62)},
		{"once_at_unix": int64(now.Add(time.Hour).Unix())},
	}
	for _, args := range tests {
		if err := validateScheduleEditCadenceArgs(args, now); err != nil {
			t.Fatalf("validateScheduleEditCadenceArgs(%v): %v", args, err)
		}
	}
}
