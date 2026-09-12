// SPDX-License-Identifier: MIT

// Control-plane Client: self-update surface (UpdateCheck + UpdateApply + related types).
// Code extracted from client.go during the Day-115 god-file split.
// Public API unchanged.
package controlplane


import (
	"context"
	"strings"
)

type UpdateCheckResult struct {
	Current  string
	UpToDate bool
	Update   *UpdateInfo
	Disabled bool // true when the daemon has update disabled
}

// UpdateInfo describes an available update.
type UpdateInfo struct {
	Version string
	SHA256  string
	URL     string
	Notes   string
}

// UpdateCheck queries the daemon's update source and returns whether an update
// is available. Returns Update.Disabled=true when the daemon has no update
// source configured.
func (c *Client) UpdateCheck(ctx context.Context) (*UpdateCheckResult, error) {
	out, err := c.Call(ctx, CmdUpdateCheck, nil)
	if err != nil {
		return nil, err
	}
	result := &UpdateCheckResult{
		Current:  toString(out["current"]),
		UpToDate: toBool(out["up_to_date"]),
	}
	if update, ok := out["update"].(map[string]any); ok && update != nil {
		result.Update = &UpdateInfo{
			Version: toString(update["version"]),
			SHA256:  toString(update["sha256"]),
			URL:     toString(update["url"]),
			Notes:   toString(update["notes"]),
		}
	}
	if disabled, ok := out["status"].(string); ok && strings.Contains(disabled, "disabled") {
		result.Disabled = true
	}
	return result, nil
}

// UpdateApplyResult is the result of an update apply.
type UpdateApplyResult struct {
	Applied bool
	Version string
	Error   string
}

// UpdateApply downloads the specified update, validates it, drains the daemon,
// and atomically swaps the binary. The daemon then restarts with the new binary.
func (c *Client) UpdateApply(ctx context.Context, version, sha256, url, notes string) (*UpdateApplyResult, error) {
	args := map[string]any{
		"version": version,
		"sha256":  sha256,
		"url":     url,
	}
	if notes != "" {
		args["notes"] = notes
	}
	out, err := c.Call(ctx, CmdUpdateApply, args)
	if err != nil {
		return nil, err
	}
	return &UpdateApplyResult{
		Applied: toBool(out["applied"]),
		Version: toString(out["version"]),
		Error:   toString(out["error"]),
	}, nil
}

// Close releases the client. Connections are dialed per call (Call/Stream
// each open and close their own), so there is nothing persistent to tear
// down — Close exists so callers can `defer cl.Close()` uniformly and a
// future pooled transport has a hook.
func (c *Client) Close() error { return nil }
