// SPDX-License-Identifier: MIT

// Package pluginhost answers the plugin contract's ports from the plugin
// host module.
//
// The built-in tools plugin hosts out-of-process plugins, and it is a
// PLUGIN: it may not name the module that owns the process (the binary
// pinning, the frame caps, the callback bridge, the teardown). So the spawn
// goes through the contract's PORT (kernel/contract/pluginapi) and this
// app-layer adapter answers it from the module.
//
// The adapter is a thin forwarding layer on purpose: the rules that matter
// — a pinned binary that does not match its digest is refused, an oversized
// frame tears the plugin down, a callback the operator did not wire is
// rejected — stay with the module that enforces them. What moves here is
// only the ANSWERING.
package pluginhost

import (
	"context"

	"github.com/agezt/agezt/kernel/contract/pluginapi"
	"github.com/agezt/agezt/kernel/modules/extensions/plugin"
)

// Spawner answers the contract's spawn port.
type Spawner struct{}

// NewSpawner returns the adapter that answers the spawn port.
func NewSpawner() *Spawner { return &Spawner{} }

// Spawn launches the plugin process the request describes and returns the
// ready plugin. The module's own plugin satisfies the contract's Spawned —
// its tools, its declared capabilities and its close are the three verbs
// the contract exposes.
func (Spawner) Spawn(ctx context.Context, req pluginapi.SpawnRequest) (pluginapi.Spawned, error) {
	p, err := plugin.Spawn(ctx, plugin.Config{
		Path:                   req.Path,
		Args:                   req.Args,
		Env:                    req.Env,
		Dir:                    req.Dir,
		InitTimeout:            req.InitTimeout,
		InvokeTimeout:          req.InvokeTimeout,
		MaxFrameBytes:          req.MaxFrameBytes,
		MaxConcurrentCallbacks: req.MaxConcurrentCallbacks,
		MaxAdvertisedTools:     req.MaxAdvertisedTools,
		Logger:                 req.Logger,
		PinnedHash:             req.PinnedHash,
		AllowedTools:           req.AllowedTools,
		HostTools:              req.HostTools,
	})
	if err != nil {
		return nil, err
	}
	return p, nil
}

// The adapter answers the contract's port — that is the whole of its job.
var _ pluginapi.Spawner = Spawner{}
