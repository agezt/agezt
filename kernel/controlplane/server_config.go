// SPDX-License-Identifier: MIT

package controlplane

import (
	"context"
	"net"
	"sync"

	"github.com/agezt/agezt/kernel/board"
	"github.com/agezt/agezt/kernel/runtime"
	"github.com/agezt/agezt/kernel/tenant"
	"github.com/agezt/agezt/kernel/update"
)

// RuntimeConfig holds the core kernel and base directory for the server.
// Field names match Server's direct fields for transparent embedding.
type RuntimeConfig struct {
	Kernel  *runtime.Kernel
	BaseDir string
}

// LifecycleConfig holds the server's lifecycle state: listener, auth token,
// done channel, and shutdown coordination primitives.
//
// These fields are protected by Server.mu and should only be accessed
// through the Server's exported methods.
type LifecycleConfig struct {
	Listener     net.Listener
	Token        string
	Done         chan struct{}
	ServeCancel  context.CancelFunc
	ShutdownCh   chan struct{}
	mu           sync.Mutex
	wg           sync.WaitGroup
	stopOnce     sync.Once
	shutdownOnce sync.Once
}

// ProactiveConfig holds the optional proactive engine (pulse) and its
// injected watchers for standing orders, disk-space monitoring, and
// command probes.
type ProactiveConfig struct {
	// Pulse is the resident proactive engine. Nil when Pulse is disabled.
	Pulse PulseController

	// StandingFire fires a standing order on demand (M765). Nil until wired.
	StandingFire func(id string) bool

	// DiskWatch adds a pulse disk-space observer at runtime (M767). Nil when
	// pulse is disabled.
	DiskWatch func(path string, minPct float64) (string, bool)

	// ProbeWatch adds a pulse command-probe observer at runtime (M768). Nil
	// when pulse is disabled.
	ProbeWatch func(name string, argv []string) (string, bool)
}

// TenantConfig holds multi-tenant settings: the tenant registry, pinned
// config environment variables, and the per-connection cancel behaviour.
type TenantConfig struct {
	// Registry is the optional multi-tenant registry. Nil when multi-tenancy
	// is disabled.
	Registry *tenant.Registry

	// ConfigEnvPinned marks config env vars set in the real process
	// environment at startup (before the config-store injection).
	ConfigEnvPinned map[string]bool

	// CancelOnDisconnect, when true, cancels a streaming CmdRun if the
	// client connection drops before the run finishes (M35).
	CancelOnDisconnect bool
}

// HealthConfig holds system-health probe functions.
type HealthConfig struct {
	// DiskFree returns (free, total) bytes for the filesystem at path.
	// Nil when not wired; the disk handler reports it as unavailable.
	DiskFree DiskFreeFunc
}

// NetworkConfig holds the daemon's network-exposed HTTP server bindings.
type NetworkConfig struct {
	// Bindings lists HTTP servers (web UI, REST API, OpenAI API) with their
	// bind addresses and loopback status.
	Bindings []HTTPBinding
}

// ChannelConfig holds the messaging channel integrations.
type ChannelConfig struct {
	// Channels lists the configured messaging channels (Telegram, Slack,
	// Discord) for `agt status`.
	Channels []ChannelInfo

	// Send delivers an operator-initiated outbound message through a named
	// channel (M142). Nil when no channel is configured.
	Send ChannelSender
}

// CredentialConfig holds AWS credential chain description.
type CredentialConfig struct {
	// Chain is a human-readable description of the resolved AWS credential
	// chain (which keyless/ambient layers engaged). Empty when AWS
	// credentials aren't in play.
	Chain string
}

// BoardConfig holds the shared message-board instance and its post
// notifier for the M937 mailbox.
type BoardConfig struct {
	// Store is the daemon's ONE shared kernel/board instance. Nil for
	// fresh read-only access (tests, older daemons).
	Store *board.Store

	// Notify publishes board.posted for board writes so SDK sends wake
	// standing orders like agent sends. Nil-safe.
	Notify func(m board.Message, corr string)
}

// UpdateConfig holds the self-update engine (M860).
type UpdateConfig struct {
	// Service is the self-update engine. Nil when update is disabled.
	Service *update.Service
}

// Getter methods that return the current config values. These provide
// a structured API without migrating the internal storage.

// RuntimeConfig returns the kernel and base directory.
func (s *Server) RuntimeConfig() RuntimeConfig {
	return RuntimeConfig{
		Kernel:  s.k,
		BaseDir: s.baseDir,
	}
}

// LifecycleConfig returns the server's lifecycle state.
// Note: the embedded sync.Mutex is not accessible through this config.
func (s *Server) LifecycleConfig() LifecycleConfig {
	s.mu.Lock()
	defer s.mu.Unlock()
	return LifecycleConfig{
		Listener:     s.listener,
		Token:        s.token,
		Done:         s.done,
		ServeCancel:  s.serveCancel,
		ShutdownCh:   s.shutdownCh,
		mu:           sync.Mutex{}, // zero value; not a copy of the real mutex
		wg:           sync.WaitGroup{},
		stopOnce:     sync.Once{},
		shutdownOnce: sync.Once{},
	}
}

// ProactiveConfig returns the proactive engine configuration.
func (s *Server) ProactiveConfig() ProactiveConfig {
	return ProactiveConfig{
		Pulse:        s.pulse,
		StandingFire: s.standingFire,
		DiskWatch:    s.diskWatch,
		ProbeWatch:   s.probeWatch,
	}
}

// TenantConfig returns the multi-tenant configuration.
func (s *Server) TenantConfig() TenantConfig {
	return TenantConfig{
		Registry:           s.tenants,
		ConfigEnvPinned:    s.configEnvPinned,
		CancelOnDisconnect: s.cancelOnDisconnect,
	}
}

// HealthConfig returns the system health probe configuration.
func (s *Server) HealthConfig() HealthConfig {
	return HealthConfig{
		DiskFree: s.diskFree,
	}
}

// NetworkConfig returns the HTTP server bindings.
func (s *Server) NetworkConfig() NetworkConfig {
	return NetworkConfig{
		Bindings: s.httpBindings,
	}
}

// ChannelConfig returns the messaging channel configuration.
func (s *Server) ChannelConfig() ChannelConfig {
	return ChannelConfig{
		Channels: s.channels,
		Send:     s.channelSend,
	}
}

// CredentialConfig returns the AWS credential chain description.
func (s *Server) CredentialConfig() CredentialConfig {
	return CredentialConfig{
		Chain: s.credChain,
	}
}

// BoardConfig returns the message board configuration.
func (s *Server) BoardConfig() BoardConfig {
	return BoardConfig{
		Store:  s.boardStore,
		Notify: s.boardNotify,
	}
}

// UpdateConfig returns the self-update service configuration.
func (s *Server) UpdateConfig() UpdateConfig {
	return UpdateConfig{
		Service: s.updateSvc,
	}
}

// WithRuntimeConfig applies a RuntimeConfig to the server and returns s
// for chaining. This applies the config fields individually; the Server
// must not be running when called.
func (s *Server) WithRuntimeConfig(rc RuntimeConfig) *Server {
	s.k = rc.Kernel
	s.baseDir = rc.BaseDir
	return s
}

// WithTenantConfig applies a TenantConfig to the server.
func (s *Server) WithTenantConfig(tc TenantConfig) *Server {
	s.tenants = tc.Registry
	s.configEnvPinned = tc.ConfigEnvPinned
	s.cancelOnDisconnect = tc.CancelOnDisconnect
	return s
}

// WithProactiveConfig applies a ProactiveConfig to the server.
func (s *Server) WithProactiveConfig(pc ProactiveConfig) *Server {
	s.pulse = pc.Pulse
	s.standingFire = pc.StandingFire
	s.diskWatch = pc.DiskWatch
	s.probeWatch = pc.ProbeWatch
	return s
}

// WithHealthConfig applies a HealthConfig to the server.
func (s *Server) WithHealthConfig(hc HealthConfig) *Server {
	s.diskFree = hc.DiskFree
	return s
}

// WithNetworkConfig applies a NetworkConfig to the server.
func (s *Server) WithNetworkConfig(nc NetworkConfig) *Server {
	s.httpBindings = nc.Bindings
	return s
}

// WithChannelConfig applies a ChannelConfig to the server.
func (s *Server) WithChannelConfig(cc ChannelConfig) *Server {
	s.channels = cc.Channels
	s.channelSend = cc.Send
	return s
}

// WithCredentialConfig applies a CredentialConfig to the server.
func (s *Server) WithCredentialConfig(cc CredentialConfig) *Server {
	s.credChain = cc.Chain
	return s
}

// WithBoardConfig applies a BoardConfig to the server.
func (s *Server) WithBoardConfig(bc BoardConfig) *Server {
	s.boardStore = bc.Store
	s.boardNotify = bc.Notify
	return s
}

// WithUpdateConfig applies an UpdateConfig to the server.
func (s *Server) WithUpdateConfig(uc UpdateConfig) *Server {
	s.updateSvc = uc.Service
	return s
}

// ApplyConfig applies all config structs to the server at once.
// This is an atomic operation that returns an error if any field is
// invalid. The Server must not be running when called.
//
// cfg is taken by pointer because ServerConfig transitively embeds
// LifecycleConfig (which holds sync primitives) — copying it would pass a
// lock by value (go vet copylocks).
func (s *Server) ApplyConfig(cfg *ServerConfig) *Server {
	s.k = cfg.Runtime.Kernel
	s.baseDir = cfg.Runtime.BaseDir
	s.tenants = cfg.Tenant.Registry
	s.configEnvPinned = cfg.Tenant.ConfigEnvPinned
	s.cancelOnDisconnect = cfg.Tenant.CancelOnDisconnect
	s.pulse = cfg.Proactive.Pulse
	s.standingFire = cfg.Proactive.StandingFire
	s.diskWatch = cfg.Proactive.DiskWatch
	s.probeWatch = cfg.Proactive.ProbeWatch
	s.diskFree = cfg.Health.DiskFree
	s.httpBindings = cfg.Network.Bindings
	s.channels = cfg.Channel.Channels
	s.channelSend = cfg.Channel.Send
	s.credChain = cfg.Credential.Chain
	s.boardStore = cfg.Board.Store
	s.boardNotify = cfg.Board.Notify
	s.updateSvc = cfg.Update.Service
	return s
}

// ServerConfig holds all server configuration in one struct.
// This allows for atomic configuration of the entire server.
type ServerConfig struct {
	Runtime    RuntimeConfig
	Lifecycle  LifecycleConfig
	Proactive  ProactiveConfig
	Tenant     TenantConfig
	Health     HealthConfig
	Network    NetworkConfig
	Channel    ChannelConfig
	Credential CredentialConfig
	Board      BoardConfig
	Update     UpdateConfig
}

// NewServerFromConfig creates a new Server with the given configuration.
// cfg is taken by pointer; see ApplyConfig for why ServerConfig must not be
// copied (it embeds sync primitives via LifecycleConfig).
func NewServerFromConfig(cfg *ServerConfig) *Server {
	s := &Server{
		k:          cfg.Runtime.Kernel,
		baseDir:    cfg.Runtime.BaseDir,
		shutdownCh: make(chan struct{}),
	}
	s.applyConfig(cfg)
	return s
}

// applyConfig is the internal helper that applies cfg to s.
func (s *Server) applyConfig(cfg *ServerConfig) {
	s.tenants = cfg.Tenant.Registry
	s.configEnvPinned = cfg.Tenant.ConfigEnvPinned
	s.cancelOnDisconnect = cfg.Tenant.CancelOnDisconnect
	s.pulse = cfg.Proactive.Pulse
	s.standingFire = cfg.Proactive.StandingFire
	s.diskWatch = cfg.Proactive.DiskWatch
	s.probeWatch = cfg.Proactive.ProbeWatch
	s.diskFree = cfg.Health.DiskFree
	s.httpBindings = cfg.Network.Bindings
	s.channels = cfg.Channel.Channels
	s.channelSend = cfg.Channel.Send
	s.credChain = cfg.Credential.Chain
	s.boardStore = cfg.Board.Store
	s.boardNotify = cfg.Board.Notify
	s.updateSvc = cfg.Update.Service
}
