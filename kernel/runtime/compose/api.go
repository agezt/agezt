// SPDX-License-Identifier: MIT

package compose

import (
	"time"

	"github.com/agezt/agezt/kernel/agent"
	"github.com/agezt/agezt/kernel/edict"
	"github.com/agezt/agezt/kernel/imagetool"
	"github.com/agezt/agezt/kernel/mcp"
	"github.com/agezt/agezt/kernel/reranktool"
	"github.com/agezt/agezt/kernel/voicetool"
	"github.com/agezt/agezt/kernel/warden"
)

// OpenAPI is the surface the compose package needs from the host
// runtime to construct a *Kernel. Day-20a slice: the accessors
// the per-store openers (added in Day 20b) will need, plus a
// minimal Finalize hook that runs once every store is wired. The
// setter half is intentionally absent — the Day 20b commit will
// add SetJournal/SetBus/SetState/… as each per-store opener
// migrates across.
//
// *Kernel satisfies this interface implicitly via its public
// methods (the same pattern lifecycle.KernelAPI and
// accessors.KernelAPI use).
type OpenAPI interface {
	// Config access (read-only).
	BaseDir() string
	CatalogDir() string
	MemoryEmbedder() agent.Provider
	TenantID() string
	OnReload() func() error
	Model() string
	System() string
	SubAgentTool() bool
	MemoryTool() bool
	WorldTool() bool
	MarketTool() bool
	Voice() voicetool.Voice
	VisionModel() func() (string, bool)
	MemoryDistill() bool
	SkillForge() bool
	ShadowEval() bool
	EdictEngine() edict.Engine
	WardenEngine() warden.Engine
	ApprovalTimeout() time.Duration
	AutoApproveCapabilities() []string
	Provider() agent.Provider
	Tools() map[string]agent.Tool
	ImageGenerator() imagetool.ImageGen
	Reranker() reranktool.Reranker
	AutoPromoteScriptTools() bool
	MCPDialer() mcp.Dialer
	MCPHTTPDialer() mcp.HTTPDialer
	ScriptRunner() any
	ShutdownDrainTimeout() time.Duration
	ResumeEnabled() bool
	ResumeSnapshotMaxBytes() int64
}
