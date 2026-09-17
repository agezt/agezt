// SPDX-License-Identifier: MIT

// config_helpers.go: configScope + register/unregister + errf helpers split off
// from config.go during the Day 211 god-file refactor (#134). Public API unchanged.
package config

import (
	"encoding/json"
	"fmt"
	"strings"

	"github.com/agezt/agezt/kernel/agent"
	"github.com/agezt/agezt/kernel/settings"
)

func configScope(raw, def string) (string, error) {
	scope := strings.TrimSpace(strings.ToLower(raw))
	if scope == "" {
		return def, nil
	}
	switch scope {
	case "effective", "global", "agent":
		return scope, nil
	default:
		return "", fmt.Errorf("scope must be effective, global, or agent")
	}
}

func (t *Tool) doRegister(in input) (agent.Result, error) {
	if len(in.Section) == 0 {
		return errf("section required"), nil
	}
	var sec settings.Section
	if err := json.Unmarshal(in.Section, &sec); err != nil {
		return errf("decode section: %v", err), nil
	}
	if err := t.registry().Register(sec); err != nil {
		return errf("%s", err.Error()), nil
	}
	return agent.Result{Output: fmt.Sprintf("registered schema section %q (restart to apply its values)", sec.ID)}, nil
}

func (t *Tool) doUnregister(in input) (agent.Result, error) {
	id := strings.TrimSpace(in.ID)
	if id == "" {
		return errf("id required"), nil
	}
	removed, err := t.registry().Unregister(id, in.Force)
	if err != nil {
		return errf("%s", err.Error()), nil
	}
	if removed {
		return agent.Result{Output: "unregistered " + id}, nil
	}
	return agent.Result{Output: id + " was not registered"}, nil
}

func errf(format string, a ...any) agent.Result {
	return agent.Result{Output: fmt.Sprintf(format, a...), IsError: true}
}
