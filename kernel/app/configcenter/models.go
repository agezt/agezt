// SPDX-License-Identifier: MIT
package configcenter

type EntryRow struct {
	Key            string   `json:"key"`
	Value          string   `json:"value"`
	Rating         string   `json:"rating"`
	CreatedAt      int64    `json:"created_at"`
	UpdatedAt      int64    `json:"updated_at"`
	Version        int      `json:"version"`
	Masked         bool     `json:"masked,omitempty"`
	Description    string   `json:"description,omitempty"`
	Tags           []string `json:"tags,omitempty"`
	AccessPolicy   string   `json:"access_policy,omitempty"`
	AllowedAgents  []string `json:"allowed_agents,omitempty"`
	ExcludedAgents []string `json:"excluded_agents,omitempty"`
}
type EntryOutput struct {
	Entry EntryRow `json:"entry"`
}
type GetOutput = EntryOutput
type SetOutput = EntryOutput
type SetAccessOutput = EntryOutput
type ListOutput struct {
	Entries []EntryRow `json:"entries"`
	Count   int        `json:"count"`
}
type AccessLogRow struct {
	Timestamp int64  `json:"timestamp"`
	Key       string `json:"key"`
	AgentID   string `json:"agent_id"`
	RunID     string `json:"run_id"`
	Rating    string `json:"rating"`
	Decision  string `json:"decision"`
	Reason    string `json:"reason"`
	ValueLog  string `json:"value_log"`
}
type AccessLogOutput struct {
	Logs  []AccessLogRow `json:"logs"`
	Count int            `json:"count"`
}
type AuditRow struct {
	Timestamp int64  `json:"timestamp"`
	Event     string `json:"event"`
	Key       string `json:"key"`
	AgentID   string `json:"agent_id"`
	RunID     string `json:"run_id"`
	Rating    string `json:"rating"`
	Reason    string `json:"reason"`
	Decision  string `json:"decision"`
	Policy    string `json:"policy"`
}
type AuditOutput struct {
	Entries []AuditRow `json:"entries"`
	Count   int        `json:"count"`
}
type HealthOutput struct {
	Status string            `json:"status"`
	Checks map[string]string `json:"checks"`
	// A healthy manager can return a nil stats map; the root must still be present.
	Stats *map[string]any `json:"stats,omitempty"`
}
type DeleteOutput struct {
	Deleted bool `json:"deleted"`
}
type SetRatingOutput struct {
	Override bool `json:"override"`
}
