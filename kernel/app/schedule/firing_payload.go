// SPDX-License-Identifier: MIT

package schedule

import (
	"encoding/json"
)

type scheduleFiredPayload struct {
	ScheduleID      string         `json:"schedule_id"`
	Intent          string         `json:"intent"`
	Model           string         `json:"model"`
	Target          string         `json:"target"`
	Agent           string         `json:"agent"`
	Workflow        string         `json:"workflow"`
	SystemTask      string         `json:"system_task"`
	Tool            string         `json:"tool"`
	Executor        string         `json:"executor"`
	Category        string         `json:"category"`
	EffectClass     string         `json:"effect_class"`
	UsesLLM         *bool          `json:"uses_llm"`
	AutonomyRunbook map[string]any `json:"autonomy_runbook"`
}

// extractScheduleFired pulls schedule_id + intent + model out of a
// schedule.fired payload (M54; schedule_id added M55). Returns zero values on
// parse failure so a malformed firing still lists with its correlation and
// outcome. schedule_id is "" for firings journaled before M55.
func extractScheduleFired(payload json.RawMessage) scheduleFiredPayload {
	if len(payload) == 0 {
		return scheduleFiredPayload{}
	}
	var p scheduleFiredPayload
	if err := json.Unmarshal(payload, &p); err != nil {
		return scheduleFiredPayload{}
	}
	return p
}
