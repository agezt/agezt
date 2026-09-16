// SPDX-License-Identifier: MIT

package runtime

import (
	"time"
)

// The overrideX helpers turn a typed setter into an Apply: parse once, assign
// only on success, so a malformed value leaves the config at its inherited
// value rather than zeroing the knob.

func overrideString(set func(*Config, string)) func(*Config, string) bool {
	return func(c *Config, raw string) bool {
		v, ok := agentConfigStringValue(raw)
		if ok {
			set(c, v)
		}
		return ok
	}
}

func overrideInt(set func(*Config, int)) func(*Config, string) bool {
	return func(c *Config, raw string) bool {
		v, ok := agentConfigIntValue(raw)
		if ok {
			set(c, v)
		}
		return ok
	}
}

func overrideBool(set func(*Config, bool)) func(*Config, string) bool {
	return func(c *Config, raw string) bool {
		v, ok := agentConfigBoolValue(raw)
		if ok {
			set(c, v)
		}
		return ok
	}
}

func overrideDuration(set func(*Config, time.Duration)) func(*Config, string) bool {
	return func(c *Config, raw string) bool {
		v, ok := agentConfigDurationValue(raw)
		if ok {
			set(c, v)
		}
		return ok
	}
}
