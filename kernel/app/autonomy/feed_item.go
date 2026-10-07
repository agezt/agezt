// SPDX-License-Identifier: MIT
package autonomy

// FeedItem names the native row contract. Required identity fields retain zero
// values; numeric pointers distinguish absent doctor metadata from present zero.
type FeedItem struct {
	Seq                    int64    `json:"seq"`
	TSUnixMS               int64    `json:"ts_unix_ms"`
	Kind                   string   `json:"kind"`
	Subject                string   `json:"subject"`
	Category               string   `json:"category"`
	Title                  string   `json:"title"`
	CorrelationID          string   `json:"correlation_id"`
	Detail                 string   `json:"detail,omitempty"`
	Agent                  string   `json:"agent,omitempty"`
	TargetAgent            string   `json:"target_agent,omitempty"`
	DelegateTo             string   `json:"delegate_to,omitempty"`
	DelegatedBy            string   `json:"delegated_by,omitempty"`
	RootAgent              string   `json:"root_agent,omitempty"`
	IncidentID             string   `json:"incident_id,omitempty"`
	RootIncidentID         string   `json:"root_incident_id,omitempty"`
	ParentIncidentID       string   `json:"parent_incident_id,omitempty"`
	Phase                  string   `json:"phase,omitempty"`
	Mode                   string   `json:"mode,omitempty"`
	Resolution             string   `json:"resolution,omitempty"`
	RoutingTaskType        string   `json:"routing_task_type,omitempty"`
	RoutingTaskModelChain  []string `json:"routing_task_model_chain,omitempty"`
	RoutingForceGeneration *int     `json:"routing_force_generation,omitempty"`
	ChainDepth             *int     `json:"chain_depth,omitempty"`
}
