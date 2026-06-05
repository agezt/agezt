// SPDX-License-Identifier: MIT

package standing

import (
	"context"
	"strconv"
	"strings"
	"time"
)

// cronTickInterval is how often the cron ticker wakes. Sub-minute so every
// matching minute is caught; per-order per-minute dedup makes the extra ticks
// harmless.
const cronTickInterval = 30 * time.Second

// minuteStamp identifies the minute t falls in, for once-per-minute dedup.
func minuteStamp(t time.Time) int64 { return t.Unix() / 60 }

// tickCron fires every enabled cron-triggered order whose schedule matches t, at
// most once per matching minute (tracked in lastFired). Returns the ids fired —
// used by tests; the daemon ignores the return. fire is dispatched on its own
// goroutine so a long run never stalls the ticker.
func tickCron(ctx context.Context, store *Store, t time.Time, lastFired map[string]int64, fire FireFunc) []string {
	stamp := minuteStamp(t)
	var fired []string
	orders := store.List()
	for _, o := range orders {
		if !o.Enabled {
			continue
		}
		for _, tr := range o.Triggers {
			if tr.Type != TriggerCron || !matchesCron(tr.Schedule, t) {
				continue
			}
			if lastFired[o.ID] == stamp {
				continue // already fired this minute
			}
			lastFired[o.ID] = stamp
			ord := o
			sched := tr.Schedule
			go fire(ctx, ord, "cron:"+sched)
			fired = append(fired, o.ID)
			break
		}
	}
	pruneToLive(lastFired, orders) // bound the dedup map to live orders
	return fired
}

// StartCron drives cron-triggered standing orders (SPEC-16 §4): a background
// ticker fires each enabled order whose cron schedule matches the current minute.
// now is injectable for tests (nil → time.Now). Returns false when it can't start
// (nil store/fire). The goroutine stops on ctx cancellation; a panic is recovered.
func StartCron(ctx context.Context, store *Store, now func() time.Time, fire FireFunc) bool {
	if store == nil || fire == nil {
		return false
	}
	if now == nil {
		now = time.Now
	}
	fire = safeFire(fire) // contain a panicking order to its own goroutine
	lastFired := map[string]int64{}
	go func() {
		defer func() { _ = recover() }()
		tk := time.NewTicker(cronTickInterval)
		defer tk.Stop()
		for {
			select {
			case <-ctx.Done():
				return
			case <-tk.C:
				tickCron(ctx, store, now(), lastFired, fire)
			}
		}
	}()
	return true
}

// matchesCron reports whether t satisfies a standard 5-field cron expression
// (minute hour day-of-month month day-of-week). It is a stdlib-only matcher — no
// dependency — supporting the usual grammar per field: `*`, a single value, a
// range `a-b`, a step `*/n` or `a-b/n`, and comma lists of those. Day-of-week
// accepts 0 or 7 for Sunday. Per cron convention, when BOTH day-of-month and
// day-of-week are restricted (neither is `*`) the day matches if EITHER does.
//
// A malformed expression never matches (returns false) rather than erroring, so
// a bad standing-order cron simply never fires instead of crashing the ticker.
func matchesCron(spec string, t time.Time) bool {
	fields := strings.Fields(spec)
	if len(fields) != 5 {
		return false
	}
	min, okMin := cronField(fields[0], 0, 59)
	hour, okHour := cronField(fields[1], 0, 23)
	dom, okDom := cronField(fields[2], 1, 31)
	mon, okMon := cronField(fields[3], 1, 12)
	dow, okDow := cronField(fields[4], 0, 7)
	if !okMin || !okHour || !okDom || !okMon || !okDow {
		return false
	}
	if !min[t.Minute()] || !hour[t.Hour()] || !mon[int(t.Month())] {
		return false
	}
	// Day match: cron's dom/dow OR-when-both-restricted rule.
	wd := int(t.Weekday()) // 0=Sunday
	domMatch := dom[t.Day()]
	dowMatch := dow[wd] || (wd == 0 && dow[7]) || (wd == 7 && dow[0])
	domRestricted := fields[2] != "*"
	dowRestricted := fields[4] != "*"
	switch {
	case domRestricted && dowRestricted:
		return domMatch || dowMatch
	case domRestricted:
		return domMatch
	case dowRestricted:
		return dowMatch
	default:
		return true // both wild
	}
}

// cronField parses one cron field into the set of values it allows within
// [lo,hi]. Returns false on any malformed token.
func cronField(field string, lo, hi int) (map[int]bool, bool) {
	out := make(map[int]bool)
	for _, part := range strings.Split(field, ",") {
		if part == "" {
			return nil, false
		}
		// Step: "<range>/<n>".
		stepRange := part
		step := 1
		if slash := strings.IndexByte(part, '/'); slash >= 0 {
			stepRange = part[:slash]
			n, err := strconv.Atoi(part[slash+1:])
			if err != nil || n <= 0 {
				return nil, false
			}
			step = n
		}
		start, end := lo, hi
		switch {
		case stepRange == "*":
			// full range
		case strings.IndexByte(stepRange, '-') >= 0:
			dash := strings.IndexByte(stepRange, '-')
			a, err1 := strconv.Atoi(stepRange[:dash])
			b, err2 := strconv.Atoi(stepRange[dash+1:])
			if err1 != nil || err2 != nil {
				return nil, false
			}
			start, end = a, b
		default:
			v, err := strconv.Atoi(stepRange)
			if err != nil {
				return nil, false
			}
			start, end = v, v
		}
		if start < lo || end > hi || start > end {
			return nil, false
		}
		for v := start; v <= end; v += step {
			out[v] = true
		}
	}
	if len(out) == 0 {
		return nil, false
	}
	return out, true
}
