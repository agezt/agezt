// SPDX-License-Identifier: MIT

package schedule_test

import (
	"errors"
	"fmt"
	appschedule "github.com/agezt/agezt/kernel/app/schedule"
	"github.com/agezt/agezt/kernel/cadence"
	"strings"
	"testing"
	"time"
)

func cadenceNumber(v float64) appschedule.CadenceNumber {
	return appschedule.CadenceNumber{Value: v, Present: true}
}
func TestScheduleEditCadenceRetainsVariantPriorityAndOriginalSelectedParseCause(t *testing.T) {
	cause := errors.New("owned selected parse cause")
	ignored := errors.New("owned ignored parse cause")
	now := time.Unix(1000, 0)
	for _, mode := range []string{"once-error", "once-selected", "cooldown-error", "cooldown-selected", "window-start-error", "window-end-error", "window-interval-error", "window-days-error", "window-selected", "daily-at-error", "daily-days-error", "daily-selected", "interval-error", "unused-days-error"} {
		t.Run(mode, func(t *testing.T) {
			in := appschedule.EditCadence{}
			want := cause
			switch mode {
			case "once-error":
				in.OnceAt = appschedule.CadenceNumber{Present: true, Err: cause}
				in.Cooldown = appschedule.CadenceNumber{Present: true, Err: ignored}
			case "once-selected":
				in.OnceAt = cadenceNumber(1001.9)
				in.Cooldown = appschedule.CadenceNumber{Present: true, Err: ignored}
				in.Interval = appschedule.CadenceNumber{Present: true, Err: ignored}
				want = nil
			case "cooldown-error":
				in.Cooldown = appschedule.CadenceNumber{Present: true, Err: cause}
				in.WindowStart = appschedule.CadenceNumber{Present: true, Err: ignored}
			case "cooldown-selected":
				in.Cooldown = cadenceNumber(1.9)
				in.WindowStart = appschedule.CadenceNumber{Present: true, Err: ignored}
				in.Interval = appschedule.CadenceNumber{Present: true, Err: ignored}
				want = nil
			case "window-start-error":
				in.WindowStart = appschedule.CadenceNumber{Present: true, Err: cause}
				in.WindowEnd = appschedule.CadenceNumber{Present: true, Err: ignored}
			case "window-end-error":
				in.WindowStart = cadenceNumber(60)
				in.WindowEnd = appschedule.CadenceNumber{Present: true, Err: cause}
				in.Interval = appschedule.CadenceNumber{Present: true, Err: ignored}
			case "window-interval-error":
				in.WindowStart = cadenceNumber(60)
				in.WindowEnd = cadenceNumber(120)
				in.Interval = appschedule.CadenceNumber{Present: true, Err: cause}
				in.Days = appschedule.CadenceNumber{Present: true, Err: ignored}
			case "window-days-error":
				in.WindowStart = cadenceNumber(60)
				in.WindowEnd = cadenceNumber(120)
				in.Interval = cadenceNumber(0)
				in.Days = appschedule.CadenceNumber{Present: true, Err: cause}
			case "window-selected":
				in.WindowStart = cadenceNumber(60.9)
				in.WindowEnd = cadenceNumber(120.9)
				in.Interval = cadenceNumber(1.9)
				in.Days = cadenceNumber(0.9)
				in.AtMinutes = appschedule.CadenceNumber{Present: true, Err: ignored}
				want = nil
			case "daily-at-error":
				in.AtMinutes = appschedule.CadenceNumber{Present: true, Err: cause}
				in.Days = appschedule.CadenceNumber{Present: true, Err: ignored}
			case "daily-days-error":
				in.AtMinutes = cadenceNumber(1440)
				in.Days = appschedule.CadenceNumber{Present: true, Err: cause}
			case "daily-selected":
				in.AtMinutes = cadenceNumber(1439.9)
				in.Days = cadenceNumber(127.9)
				in.Interval = appschedule.CadenceNumber{Present: true, Err: ignored}
				want = nil
			case "interval-error":
				in.Interval = appschedule.CadenceNumber{Present: true, Err: cause}
			case "unused-days-error":
				in.Days = appschedule.CadenceNumber{Present: true, Err: ignored}
				in.WindowEnd = appschedule.CadenceNumber{Present: true, Err: ignored}
				want = nil
			}
			if err := appschedule.ValidateEditCadence(in, now); err != want {
				t.Fatal(mode, err, want)
			}
		})
	}
}
func TestScheduleEditCadenceRetainsTimezoneFirstStrictBoundsAndTruncation(t *testing.T) {
	now := time.Unix(1000, 0)
	cooldown := fmt.Sprintf("cadence: cooldown 0s is below the %s minimum", cadence.MinInterval)
	interval := fmt.Sprintf("cadence: interval 0s is below the %s minimum", cadence.MinInterval)
	cases := []struct {
		name string
		in   appschedule.EditCadence
		want string
	}{
		{name: "empty"}, {name: "trimmed timezone", in: appschedule.EditCadence{TZ: " UTC "}},
		{name: "timezone first", in: appschedule.EditCadence{TZ: "missing/zone", OnceAt: appschedule.CadenceNumber{Present: true, Err: errors.New("wrong later cause")}}, want: "cadence: unknown timezone \"missing/zone\":"},
		{name: "once current second", in: appschedule.EditCadence{OnceAt: cadenceNumber(1000.9)}, want: "cadence: one-shot time must be in the future"},
		{name: "once past", in: appschedule.EditCadence{OnceAt: cadenceNumber(999)}, want: "cadence: one-shot time must be in the future"},
		{name: "once future", in: appschedule.EditCadence{OnceAt: cadenceNumber(1001)}},
		{name: "cooldown floor", in: appschedule.EditCadence{Cooldown: cadenceNumber(0.99)}, want: cooldown},
		{name: "cooldown fraction", in: appschedule.EditCadence{Cooldown: cadenceNumber(1.99)}},
		{name: "interval floor", in: appschedule.EditCadence{Interval: cadenceNumber(0.99)}, want: interval},
		{name: "interval fraction", in: appschedule.EditCadence{Interval: cadenceNumber(1.99)}},
		{name: "absent interval", in: appschedule.EditCadence{Interval: appschedule.CadenceNumber{Value: 0.99}}},
		{name: "window interval before bounds", in: appschedule.EditCadence{WindowStart: cadenceNumber(1440), WindowEnd: cadenceNumber(-1), Interval: cadenceNumber(0), Days: cadenceNumber(128)}, want: interval},
		{name: "window bounds before order", in: appschedule.EditCadence{WindowStart: cadenceNumber(1440), WindowEnd: cadenceNumber(-1), Interval: cadenceNumber(1), Days: cadenceNumber(128)}, want: "cadence: window bounds must be 00:00..23:59"},
		{name: "window fractional order", in: appschedule.EditCadence{WindowStart: cadenceNumber(60.1), WindowEnd: cadenceNumber(60.9), Interval: cadenceNumber(1), Days: cadenceNumber(128)}, want: "cadence: window end must be after its start"},
		{name: "window day floor", in: appschedule.EditCadence{WindowStart: cadenceNumber(60), WindowEnd: cadenceNumber(120), Interval: cadenceNumber(1), Days: cadenceNumber(-1)}, want: fmt.Sprintf("cadence: day-mask must be 0..%d", cadence.AllDays)},
		{name: "window day cap", in: appschedule.EditCadence{WindowStart: cadenceNumber(60), WindowEnd: cadenceNumber(120), Interval: cadenceNumber(1), Days: cadenceNumber(128)}, want: fmt.Sprintf("cadence: day-mask must be 0..%d", cadence.AllDays)},
		{name: "daily minute floor", in: appschedule.EditCadence{AtMinutes: cadenceNumber(-1), Days: cadenceNumber(128)}, want: "cadence: time-of-day must be 00:00..23:59"},
		{name: "daily minute cap", in: appschedule.EditCadence{AtMinutes: cadenceNumber(1440)}, want: "cadence: time-of-day must be 00:00..23:59"},
		{name: "daily day floor", in: appschedule.EditCadence{AtMinutes: cadenceNumber(60), Days: cadenceNumber(-1)}, want: fmt.Sprintf("cadence: day-mask must be 0..%d", cadence.AllDays)},
		{name: "daily day cap", in: appschedule.EditCadence{AtMinutes: cadenceNumber(60), Days: cadenceNumber(128)}, want: fmt.Sprintf("cadence: day-mask must be 0..%d", cadence.AllDays)},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			err := appschedule.ValidateEditCadence(tc.in, now)
			got := ""
			if err != nil {
				got = err.Error()
			}
			if tc.name == "timezone first" {
				if !strings.HasPrefix(got, tc.want) {
					t.Fatal(got, tc.want)
				}
			} else if got != tc.want {
				t.Fatal(got, tc.want)
			}
		})
	}
}
