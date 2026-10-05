// SPDX-License-Identifier: MIT

package providers

// Optional row fields preserve presence, including present empty strings.
type ObservationLogRow struct {
	TSUnixMS int64   `json:"ts_unix_ms"`
	Seq      int64   `json:"seq"`
	Kind     string  `json:"kind"`
	Primary  *string `json:"primary,omitempty"`
	Chain    *string `json:"chain,omitempty"`
	TaskType *string `json:"task_type,omitempty"`
	Failed   *string `json:"failed,omitempty"`
	Next     *string `json:"next,omitempty"`
	Reason   *string `json:"reason,omitempty"`
	Scope    *string `json:"scope,omitempty"`
}
type ObservationLogOutput struct {
	Events     []ObservationLogRow `json:"events"`
	Count      int                 `json:"count"`
	NextCursor string              `json:"next_cursor"`
}
type ObservationStatsOutput struct {
	Routed             int            `json:"routed"`
	Fallbacks          int            `json:"fallbacks"`
	FallbackRate       float64        `json:"fallback_rate"`
	ByPrimary          map[string]int `json:"by_primary"`
	FallbacksByPrimary map[string]int `json:"fallbacks_by_primary"`
	WindowMS           int64          `json:"window_ms"`
}
type ObservationRejectionRow struct {
	TSUnixMS   int64   `json:"ts_unix_ms"`
	Kind       string  `json:"kind"`
	Capability string  `json:"capability"`
	Model      *string `json:"model,omitempty"`
	FromModel  *string `json:"from_model,omitempty"`
	ToModel    *string `json:"to_model,omitempty"`
}
type ObservationRejectionsOutput struct {
	Rejections []ObservationRejectionRow `json:"rejections"`
	Count      int                       `json:"count"`
}

func observationString(row map[string]any, key string) *string {
	raw, exists := row[key]
	if !exists {
		return nil
	}
	value := raw.(string)
	return &value
}
