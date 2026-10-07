// SPDX-License-Identifier: MIT
package configcenter

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"strings"

	"github.com/agezt/agezt/kernel/app"
	"github.com/agezt/agezt/kernel/contract/opapi"
)

type KeyRequest struct {
	Key json.RawMessage `json:"key,omitempty"`
}
type ListRequest struct {
	Rating json.RawMessage `json:"rating,omitempty"`
}
type SinceRequest struct {
	Since json.RawMessage `json:"since,omitempty"`
}
type AccessLogRequest struct {
	Key     json.RawMessage `json:"key,omitempty"`
	AgentID json.RawMessage `json:"agent_id,omitempty"`
	Since   json.RawMessage `json:"since,omitempty"`
}
type SetRequest struct {
	Key            json.RawMessage `json:"key,omitempty"`
	Value          json.RawMessage `json:"value,omitempty"`
	Rating         json.RawMessage `json:"rating,omitempty"`
	Description    json.RawMessage `json:"description,omitempty"`
	AllowedAgents  json.RawMessage `json:"allowed_agents,omitempty"`
	ExcludedAgents json.RawMessage `json:"excluded_agents,omitempty"`
}
type SetRatingRequest struct {
	Key    json.RawMessage `json:"key,omitempty"`
	Rating json.RawMessage `json:"rating,omitempty"`
}
type SetAccessRequest struct {
	Key            json.RawMessage `json:"key,omitempty"`
	AllowedAgents  json.RawMessage `json:"allowed_agents,omitempty"`
	ExcludedAgents json.RawMessage `json:"excluded_agents,omitempty"`
}

// Raw fields defer legacy null/type/blank validation to each command's codec.
var inputSchema = json.RawMessage(`{"type":"object","additionalProperties":true,"properties":{"key":{},"value":{},"rating":{},"description":{},"allowed_agents":{},"excluded_agents":{},"agent_id":{},"since":{}}}`)

func textArg(raw json.RawMessage, key string, required bool) (string, error) {
	if len(raw) == 0 {
		if required {
			return "", fmt.Errorf("args.%s required", key)
		}
		return "", nil
	}
	var value any
	if err := json.Unmarshal(raw, &value); err != nil {
		return "", err
	}
	text, ok := value.(string)
	if !ok {
		return "", fmt.Errorf("args.%s must be a string", key)
	}
	if required && strings.TrimSpace(text) == "" {
		return "", fmt.Errorf("args.%s required", key)
	}
	return text, nil
}
func stringList(raw json.RawMessage) []string {
	var value any
	if len(raw) == 0 || json.Unmarshal(raw, &value) != nil {
		return nil
	}
	var values []string
	switch value := value.(type) {
	case string:
		if strings.TrimSpace(value) == "" {
			return nil
		}
		values = strings.FieldsFunc(value, func(r rune) bool { return r == ',' || r == ';' || r == ' ' || r == '\n' || r == '\t' })
	case []any:
		values = make([]string, 0, len(value))
		for _, item := range value {
			if text, ok := item.(string); ok {
				values = append(values, text)
			}
		}
	default:
		return nil
	}
	out := make([]string, 0, len(values))
	seen := map[string]bool{}
	for _, value := range values {
		value = strings.TrimSpace(value)
		if value == "" {
			continue
		}
		key := strings.ToLower(value)
		if seen[key] {
			continue
		}
		seen[key] = true
		out = append(out, value)
	}
	return out
}

func Operations(reads func(context.Context) *Reads, writes func(context.Context) *Writes) ([]app.Operation, error) {
	if reads == nil || writes == nil {
		return nil, errors.New("configcenter read/write providers required")
	}
	spec := func(name string, read bool) opapi.Spec {
		return opapi.Spec{Name: name, ReadOnly: read, Authz: opapi.PrimaryOnly, Tenancy: opapi.Primary, AllowUnknownInput: true, InputSchema: inputSchema}
	}
	get, err := app.NewOperation(spec("configcenter.get", true), func(ctx context.Context, in KeyRequest) (GetOutput, error) {
		key, err := textArg(in.Key, "key", true)
		if err != nil {
			return GetOutput{}, err
		}
		return reads(ctx).Get(ctx, GetInput{Key: key})
	})
	if err != nil {
		return nil, err
	}
	list, err := app.NewOperation(spec("configcenter.list", true), func(ctx context.Context, in ListRequest) (ListOutput, error) {
		rating, err := textArg(in.Rating, "rating", false)
		if err != nil {
			return ListOutput{}, err
		}
		return reads(ctx).List(ctx, ListInput{Rating: rating})
	})
	if err != nil {
		return nil, err
	}
	accessLog, err := app.NewOperation(spec("configcenter.access-log", true), func(ctx context.Context, in AccessLogRequest) (AccessLogOutput, error) {
		key, err := textArg(in.Key, "key", false)
		if err != nil {
			return AccessLogOutput{}, err
		}
		agent, err := textArg(in.AgentID, "agent_id", false)
		if err != nil {
			return AccessLogOutput{}, err
		}
		since, err := textArg(in.Since, "since", false)
		if err != nil {
			return AccessLogOutput{}, err
		}
		return reads(ctx).AccessLog(ctx, AccessLogInput{Key: key, AgentID: agent, Since: since})
	})
	if err != nil {
		return nil, err
	}
	audit, err := app.NewOperation(spec("configcenter.audit", true), func(ctx context.Context, in SinceRequest) (AuditOutput, error) {
		since, err := textArg(in.Since, "since", false)
		if err != nil {
			return AuditOutput{}, err
		}
		return reads(ctx).Audit(ctx, AuditInput{Since: since})
	})
	if err != nil {
		return nil, err
	}
	health, err := app.NewOperation(spec("configcenter.health", true), func(ctx context.Context, in HealthInput) (HealthOutput, error) { return reads(ctx).Health(ctx, in) })
	if err != nil {
		return nil, err
	}
	set, err := app.NewOperation(spec("configcenter.set", false), func(ctx context.Context, in SetRequest) (SetOutput, error) {
		key, err := textArg(in.Key, "key", true)
		if err != nil {
			return SetOutput{}, err
		}
		value, err := textArg(in.Value, "value", true)
		if err != nil {
			return SetOutput{}, err
		}
		rating, err := textArg(in.Rating, "rating", false)
		if err != nil {
			return SetOutput{}, err
		}
		description, err := textArg(in.Description, "description", false)
		if err != nil {
			return SetOutput{}, err
		}
		return writes(ctx).Set(ctx, SetInput{Key: key, Value: value, Rating: rating, Description: description, AllowedAgents: stringList(in.AllowedAgents), ExcludedAgents: stringList(in.ExcludedAgents)})
	})
	if err != nil {
		return nil, err
	}
	deleteOp, err := app.NewOperation(spec("configcenter.delete", false), func(ctx context.Context, in KeyRequest) (DeleteOutput, error) {
		key, err := textArg(in.Key, "key", true)
		if err != nil {
			return DeleteOutput{}, err
		}
		return writes(ctx).Delete(ctx, DeleteInput{Key: key})
	})
	if err != nil {
		return nil, err
	}
	rating, err := app.NewOperation(spec("configcenter.set-rating", false), func(ctx context.Context, in SetRatingRequest) (SetRatingOutput, error) {
		key, err := textArg(in.Key, "key", true)
		if err != nil {
			return SetRatingOutput{}, err
		}
		rating, err := textArg(in.Rating, "rating", true)
		if err != nil {
			return SetRatingOutput{}, err
		}
		return writes(ctx).SetRating(ctx, SetRatingInput{Key: key, Rating: rating})
	})
	if err != nil {
		return nil, err
	}
	access, err := app.NewOperation(spec("configcenter.access", false), func(ctx context.Context, in SetAccessRequest) (SetAccessOutput, error) {
		key, err := textArg(in.Key, "key", true)
		if err != nil {
			return SetAccessOutput{}, err
		}
		return writes(ctx).SetAccess(ctx, SetAccessInput{Key: key, AllowedAgents: stringList(in.AllowedAgents), ExcludedAgents: stringList(in.ExcludedAgents)})
	})
	if err != nil {
		return nil, err
	}
	return []app.Operation{get, list, accessLog, audit, health, set, deleteOp, rating, access}, nil
}
