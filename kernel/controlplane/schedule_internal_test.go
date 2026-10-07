// SPDX-License-Identifier: MIT

package controlplane

import (
	"encoding/json"
	appschedule "github.com/agezt/agezt/kernel/app/schedule"
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
			raw, _ := json.Marshal(value)
			n := appschedule.NumberRequest(raw, "interval_sec")
			got, ok, err := n.Value, n.Present, n.Err
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
	if n := appschedule.NumberRequest(json.RawMessage(`"3600"`), "interval_sec"); n.Err == nil {
		t.Fatal("scheduleArgNumber accepted a string")
	}
	if n := appschedule.NumberRequest(json.RawMessage(`"nope"`), "interval_sec"); n.Err == nil {
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
		raw, _ := json.Marshal(args)
		var in appschedule.RequestInput
		if err := json.Unmarshal(raw, &in); err != nil {
			t.Fatal(err)
		}
		if err := appschedule.ValidateEditCadence(in.EditCadence(), now); err != nil {
			t.Fatalf("ValidateEditCadence(%v): %v", args, err)
		}
	}
}
