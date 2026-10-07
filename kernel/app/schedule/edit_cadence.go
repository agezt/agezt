// SPDX-License-Identifier: MIT

package schedule

import (
	"fmt"
	"github.com/agezt/agezt/kernel/cadence"
	"strings"
	"time"
)

// CadenceNumber retains transport parse results until domain validation selects it.
type CadenceNumber struct {
	Value   float64
	Present bool
	Err     error
}

func (n CadenceNumber) values() (float64, bool, error) { return n.Value, n.Present, n.Err }

// EditCadence carries the preflight values; TZ preserves the legacy lenient string cast.
type EditCadence struct {
	TZ                                                                  string
	OnceAt, Cooldown, WindowStart, WindowEnd, Interval, Days, AtMinutes CadenceNumber
}

func ValidateEditCadence(in EditCadence, now time.Time) error {
	tz := in.TZ
	if strings.TrimSpace(tz) != "" {
		if _, err := time.LoadLocation(strings.TrimSpace(tz)); err != nil {
			return fmt.Errorf("cadence: unknown timezone %q: %w", strings.TrimSpace(tz), err)
		}
	}
	if at, ok, err := in.OnceAt.values(); err != nil {
		return err
	} else if ok {
		if !time.Unix(int64(at), 0).After(now) {
			return fmt.Errorf("cadence: one-shot time must be in the future")
		}
		return nil
	}
	if sec, ok, err := in.Cooldown.values(); err != nil {
		return err
	} else if ok {
		if time.Duration(sec)*time.Second < cadence.MinInterval {
			return fmt.Errorf("cadence: cooldown %s is below the %s minimum", time.Duration(sec)*time.Second, cadence.MinInterval)
		}
		return nil
	}
	if start, ok, err := in.WindowStart.values(); err != nil {
		return err
	} else if ok {
		end, _, err := in.WindowEnd.values()
		if err != nil {
			return err
		}
		sec, _, err := in.Interval.values()
		if err != nil {
			return err
		}
		days, _, err := in.Days.values()
		if err != nil {
			return err
		}
		interval := time.Duration(sec) * time.Second
		if interval < cadence.MinInterval {
			return fmt.Errorf("cadence: interval %s is below the %s minimum", interval, cadence.MinInterval)
		}
		if int(start) < 0 || int(start) > 1439 || int(end) < 0 || int(end) > 1439 {
			return fmt.Errorf("cadence: window bounds must be 00:00..23:59")
		}
		if int(end) <= int(start) {
			return fmt.Errorf("cadence: window end must be after its start")
		}
		if int(days) < 0 || int(days) > cadence.AllDays {
			return fmt.Errorf("cadence: day-mask must be 0..%d", cadence.AllDays)
		}
		return nil
	}
	if at, ok, err := in.AtMinutes.values(); err != nil {
		return err
	} else if ok {
		days, _, err := in.Days.values()
		if err != nil {
			return err
		}
		if int(at) < 0 || int(at) > 1439 {
			return fmt.Errorf("cadence: time-of-day must be 00:00..23:59")
		}
		if int(days) < 0 || int(days) > cadence.AllDays {
			return fmt.Errorf("cadence: day-mask must be 0..%d", cadence.AllDays)
		}
		return nil
	}
	if sec, ok, err := in.Interval.values(); err != nil {
		return err
	} else if ok && time.Duration(sec)*time.Second < cadence.MinInterval {
		return fmt.Errorf("cadence: interval %s is below the %s minimum", time.Duration(sec)*time.Second, cadence.MinInterval)
	}
	return nil
}
