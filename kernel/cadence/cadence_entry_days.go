// SPDX-License-Identifier: MIT

// Cadence Entry: FormatDays + ParseDays (the days-bitmask helpers).
// Code extracted from cadence_entry.go during the Day-128 god-file split.
// Public API unchanged.
package cadence


import (
	"fmt"
	"strings"
)

func FormatDays(days int) string {
	if days == 0 || days == AllDays {
		return ""
	}
	switch days {
	case maskWeekdays:
		return "Mon-Fri"
	case maskWeekends:
		return "Sat,Sun"
	}
	var names []string
	for wd := 0; wd < 7; wd++ {
		if days&(1<<uint(wd)) != 0 {
			names = append(names, dayAbbr[wd])
		}
	}
	return strings.Join(names, ",")
}

// dayTokens maps the accepted weekday spellings to their time.Weekday index.
var dayTokens = map[string]int{
	"sun": 0, "sunday": 0,
	"mon": 1, "monday": 1,
	"tue": 2, "tues": 2, "tuesday": 2,
	"wed": 3, "weds": 3, "wednesday": 3,
	"thu": 4, "thur": 4, "thurs": 4, "thursday": 4,
	"fri": 5, "friday": 5,
	"sat": 6, "saturday": 6,
}

// ParseDays parses a day specification into a weekday bitmask. It accepts the
// shortcuts "daily"/"everyday"/"all" (every day), "weekdays", "weekends", a
// comma-separated list ("mon,wed,fri"), and inclusive ranges ("mon-fri",
// wrapping like "fri-mon"). Day names are case-insensitive. An empty/"daily"
// spec yields 0 (every day).
func ParseDays(spec string) (int, error) {
	spec = strings.ToLower(strings.TrimSpace(spec))
	switch spec {
	case "", "daily", "everyday", "every-day", "all":
		return 0, nil
	case "weekdays", "weekday":
		return maskWeekdays, nil
	case "weekends", "weekend":
		return maskWeekends, nil
	}
	mask := 0
	for _, part := range strings.Split(spec, ",") {
		part = strings.TrimSpace(part)
		if part == "" {
			continue
		}
		if lo, hi, ok := strings.Cut(part, "-"); ok {
			loIdx, ok1 := dayTokens[strings.TrimSpace(lo)]
			hiIdx, ok2 := dayTokens[strings.TrimSpace(hi)]
			if !ok1 || !ok2 {
				return 0, fmt.Errorf("cadence: bad day range %q", part)
			}
			// Inclusive, wrapping (e.g. fri-mon = Fri,Sat,Sun,Mon).
			for d := loIdx; ; d = (d + 1) % 7 {
				mask |= 1 << uint(d)
				if d == hiIdx {
					break
				}
			}
			continue
		}
		idx, ok := dayTokens[part]
		if !ok {
			return 0, fmt.Errorf("cadence: unknown day %q", part)
		}
		mask |= 1 << uint(idx)
	}
	if mask == 0 {
		return 0, fmt.Errorf("cadence: no valid days in %q", spec)
	}
	return mask, nil
}

// Job is an interval+intent pair parsed from AGEZT_SCHEDULE (see ParseJobs),
// used to seed env-sourced entries into the store.
