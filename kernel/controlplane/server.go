// SPDX-License-Identifier: MIT

package controlplane

import (
	"bufio"
	"context"
	"crypto/rand"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"net"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"time"

	"github.com/agezt/agezt/internal/brand"
	"github.com/agezt/agezt/kernel/approval"
	"github.com/agezt/agezt/kernel/runtime"
	"github.com/agezt/agezt/kernel/scheduler"
	"github.com/agezt/agezt/kernel/tenant"
)

// Server hosts the control plane for a running Kernel.
type Server struct {
	k       *runtime.Kernel
	baseDir string

	mu       sync.Mutex
	listener net.Listener
	token    string
	done     chan struct{}
	stopOnce sync.Once
	wg       sync.WaitGroup

	// shutdownCh fires (close) when a client sends CmdShutdown. The
	// daemon's main loop selects on this alongside SIGINT/SIGTERM so
	// programmatic shutdown shares the same orderly exit path as the
	// signal-driven one. Closed at most once (guarded by shutdownOnce).
	shutdownCh   chan struct{}
	shutdownOnce sync.Once

	// pulse is the optional resident proactive engine, injected by the
	// daemon via SetPulse. Nil when Pulse is disabled (AGEZT_PULSE=off);
	// the pulse handlers report "disabled" rather than dereferencing it.
	pulse PulseController

	// tenants is the optional multi-tenant registry, injected by the daemon
	// via SetTenants. Nil unless multi-tenancy is enabled; the tenant handlers
	// report "disabled" rather than dereferencing it.
	tenants *tenant.Registry

	// cancelOnDisconnect, when true, makes a streaming CmdRun cancel its run
	// if the client connection drops before the run finishes (M35). Off by
	// default so a backgrounded `agt run &` (whose client stays alive) is
	// unaffected; only a genuinely-gone client (Ctrl-C / killed) cancels.
	// Set once at startup via SetCancelOnDisconnect.
	cancelOnDisconnect bool
}

// NewServer constructs a Server that will manage runtime files under
// <baseDir>/runtime/ when Start is called.
func NewServer(k *runtime.Kernel, baseDir string) *Server {
	return &Server{
		k:          k,
		baseDir:    baseDir,
		shutdownCh: make(chan struct{}),
	}
}

// Shutdown returns a channel that closes when a client has issued
// CmdShutdown. The daemon's main loop should select on it next to
// the OS-signal channel so `agt shutdown` reaches the same orderly
// exit path as Ctrl+C. The channel never re-opens; the daemon must
// treat a close as terminal.
func (s *Server) Shutdown() <-chan struct{} { return s.shutdownCh }

// signalShutdown closes shutdownCh exactly once. Used by
// handleShutdown after the OK response has been written to the
// client, so the client read completes before the daemon starts
// tearing the process down.
func (s *Server) signalShutdown() {
	s.shutdownOnce.Do(func() { close(s.shutdownCh) })
}

// Start binds to localhost on an ephemeral port, writes the addr+token
// files, and serves connections until ctx is cancelled or Stop is called.
// Returns once the listener is ready; the accept loop runs in a goroutine.
func (s *Server) Start(ctx context.Context) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.listener != nil {
		return errors.New("controlplane: already started")
	}

	ln, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		return fmt.Errorf("controlplane: listen: %w", err)
	}
	tokBytes := make([]byte, 32)
	if _, err := rand.Read(tokBytes); err != nil {
		ln.Close()
		return fmt.Errorf("controlplane: rand: %w", err)
	}
	s.token = hex.EncodeToString(tokBytes)
	s.listener = ln
	s.done = make(chan struct{})

	if err := s.writeRuntimeFiles(ln.Addr().String()); err != nil {
		ln.Close()
		s.listener = nil
		return err
	}

	s.wg.Add(1)
	go func() {
		defer s.wg.Done()
		s.acceptLoop(ctx)
	}()
	// React to ctx cancellation by initiating shutdown. This goroutine
	// also exits when Stop is called directly (via s.done).
	s.wg.Add(1)
	go func() {
		defer s.wg.Done()
		select {
		case <-ctx.Done():
		case <-s.done:
			return
		}
		s.initiateShutdown()
	}()
	return nil
}

// Addr returns the server's bound TCP address (host:port). Empty before Start.
func (s *Server) Addr() string {
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.listener == nil {
		return ""
	}
	return s.listener.Addr().String()
}

// Token returns the server's auth token. Empty before Start.
func (s *Server) Token() string {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.token
}

// Stop closes the listener and removes the runtime files. Idempotent;
// safe to call from cleanup hooks even when Start was driven by ctx.
func (s *Server) Stop() error {
	err := s.initiateShutdown()
	s.wg.Wait()
	return err
}

// initiateShutdown closes the listener and signals the ctx-watcher goroutine
// to exit. Idempotent.
func (s *Server) initiateShutdown() error {
	var firstErr error
	s.stopOnce.Do(func() {
		s.mu.Lock()
		ln := s.listener
		s.listener = nil
		done := s.done
		s.mu.Unlock()

		if done != nil {
			close(done)
		}
		if ln != nil {
			if err := ln.Close(); err != nil {
				firstErr = err
			}
		}
		_ = os.Remove(filepath.Join(s.baseDir, "runtime", addrFile))
		_ = os.Remove(filepath.Join(s.baseDir, "runtime", tokenFile))
	})
	return firstErr
}

func (s *Server) writeRuntimeFiles(addr string) error {
	dir := filepath.Join(s.baseDir, "runtime")
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return fmt.Errorf("controlplane: mkdir runtime: %w", err)
	}
	if err := os.WriteFile(filepath.Join(dir, addrFile), []byte(addr+"\n"), 0o600); err != nil {
		return fmt.Errorf("controlplane: write addr file: %w", err)
	}
	if err := os.WriteFile(filepath.Join(dir, tokenFile), []byte(s.token+"\n"), 0o600); err != nil {
		return fmt.Errorf("controlplane: write token file: %w", err)
	}
	return nil
}

func (s *Server) acceptLoop(ctx context.Context) {
	for {
		s.mu.Lock()
		ln := s.listener
		s.mu.Unlock()
		if ln == nil {
			return
		}
		conn, err := ln.Accept()
		if err != nil {
			// Listener closed → exit cleanly.
			return
		}
		s.wg.Add(1)
		go func() {
			defer s.wg.Done()
			s.handleConn(ctx, conn)
		}()
	}
}

func (s *Server) handleConn(ctx context.Context, conn net.Conn) {
	defer conn.Close()
	// Generous read deadline per request — runs can take minutes.
	_ = conn.SetReadDeadline(time.Now().Add(10 * time.Minute))

	reader := bufio.NewReader(conn)
	line, err := reader.ReadBytes('\n')
	if err != nil {
		return
	}
	var req Request
	if err := json.Unmarshal(line, &req); err != nil {
		s.writeResp(conn, Response{ID: req.ID, Type: RespError, Error: "bad request: " + err.Error()})
		return
	}
	// Authentication + authorization (M38). The primary (admin) token
	// authorizes everything, on any tenant. Otherwise the request must name a
	// tenant AND present that tenant's own token: the principal is then that
	// tenant, restricted to an allowlist of tenant-routed commands and pinned
	// to its own tenant. This completes M14 tenant isolation on the control
	// side — a tenant manages its own runs/policy without the primary token,
	// and cannot touch another tenant or daemon-global state.
	if req.Token != s.Token() {
		reqTenant, _ := req.Args["tenant"].(string)
		reqTenant = strings.TrimSpace(reqTenant)
		if s.tenants == nil || reqTenant == "" || !s.tenants.Authorize(reqTenant, req.Token) {
			s.writeResp(conn, Response{ID: req.ID, Type: RespError, Error: "unauthorized"})
			return
		}
		// Authorized as the named tenant. Restrict to tenant-safe commands.
		if !tenantTokenAllows(req.Cmd) {
			s.writeResp(conn, Response{ID: req.ID, Type: RespError,
				Error: fmt.Sprintf("forbidden: a tenant token cannot run %q (primary token required)", req.Cmd)})
			return
		}
		// Pin the tenant arg to the authorized tenant (defense in depth — it
		// already matched, but no handler should ever see a different value).
		if req.Args == nil {
			req.Args = map[string]any{}
		}
		req.Args["tenant"] = reqTenant
	}

	switch req.Cmd {
	case CmdVersion:
		s.handleVersion(conn, req)
	case CmdRun:
		s.handleRun(ctx, conn, req)
	case CmdHalt:
		s.handleHalt(conn, req)
	case CmdResume:
		s.handleResume(conn, req)
	case CmdWhy:
		s.handleWhy(conn, req)
	case CmdWhoami:
		s.handleWhoami(conn, req)
	case CmdJournalVerify:
		s.handleVerify(conn, req)
	case CmdApprovals:
		s.handleApprovals(conn, req)
	case CmdApprovalsLog:
		s.handleApprovalsLog(conn, req)
	case CmdApprovalsStats:
		s.handleApprovalsStats(conn, req)
	case CmdDecide:
		s.handleDecide(conn, req)
	case CmdPlan:
		s.handlePlan(ctx, conn, req)
	case CmdPlanHistory:
		s.handlePlanHistory(conn, req)
	case CmdPlanStats:
		s.handlePlanStats(conn, req)
	case CmdCatalogSync:
		s.handleCatalogSync(ctx, conn, req)
	case CmdCatalogList:
		s.handleCatalogList(conn, req)
	case CmdCatalogDiscover:
		s.handleCatalogDiscover(ctx, conn, req)
	case CmdProviderReload:
		s.handleProviderReload(conn, req)
	case CmdProviderLog:
		s.handleProviderLog(conn, req)
	case CmdPulseSubscribe:
		s.handlePulseSubscribe(ctx, conn, req)
	case CmdPlanGenerate:
		s.handlePlanGenerate(ctx, conn, req)
	case CmdPlanRefine:
		s.handlePlanRefine(ctx, conn, req)
	case CmdBudget:
		s.handleBudget(conn, req)
	case CmdToolList:
		s.handleToolList(conn, req)
	case CmdToolLog:
		s.handleToolLog(conn, req)
	case CmdToolStats:
		s.handleToolStats(conn, req)
	case CmdStatus:
		s.handleStatus(conn, req)
	case CmdPluginList:
		s.handlePluginList(conn, req)
	case CmdShutdown:
		s.handleShutdown(conn, req)
	case CmdJournalTail:
		s.handleJournalTail(conn, req)
	case CmdEdictLog:
		s.handleEdictLog(conn, req)
	case CmdEdictStats:
		s.handleEdictStats(conn, req)
	case CmdEdictShow:
		s.handleEdictShow(conn, req)
	case CmdEdictTest:
		s.handleEdictTest(conn, req)
	case CmdEdictDenyList:
		s.handleEdictDenyList(conn, req)
	case CmdEdictDenyAdd:
		s.handleEdictDenyAdd(conn, req)
	case CmdEdictDenyRemove:
		s.handleEdictDenyRemove(conn, req)
	case CmdEdictSetLevel:
		s.handleEdictSetLevel(conn, req)
	case CmdEdictSetMode:
		s.handleEdictSetMode(conn, req)
	case CmdStateList:
		s.handleStateList(conn, req)
	case CmdStateGet:
		s.handleStateGet(conn, req)
	case CmdRunsList:
		s.handleRunsList(conn, req)
	case CmdRunsStats:
		s.handleRunsStats(conn, req)
	case CmdCancelRun:
		s.handleCancelRun(conn, req)
	case CmdConfig:
		s.handleConfig(conn, req)
	case CmdJournalGrep:
		s.handleJournalGrep(conn, req)
	case CmdJournalHead:
		s.handleJournalHead(conn, req)
	case CmdMemoryAdd:
		s.handleMemoryAdd(conn, req)
	case CmdMemoryList:
		s.handleMemoryList(conn, req)
	case CmdMemoryLog:
		s.handleMemoryLog(conn, req)
	case CmdMemoryGet:
		s.handleMemoryGet(conn, req)
	case CmdMemorySearch:
		s.handleMemorySearch(conn, req)
	case CmdMemoryForget:
		s.handleMemoryForget(conn, req)
	case CmdScheduleAdd:
		s.handleScheduleAdd(conn, req)
	case CmdScheduleList:
		s.handleScheduleList(conn, req)
	case CmdScheduleRemove:
		s.handleScheduleRemove(conn, req)
	case CmdScheduleRun:
		s.handleScheduleRun(conn, req)
	case CmdScheduleEnable:
		s.handleScheduleEnable(conn, req)
	case CmdScheduleEdit:
		s.handleScheduleEdit(conn, req)
	case CmdScheduleFires:
		s.handleScheduleFires(conn, req)
	case CmdScheduleStats:
		s.handleScheduleStats(conn, req)
	case CmdTenantCreate:
		s.handleTenantCreate(conn, req)
	case CmdTenantList:
		s.handleTenantList(conn, req)
	case CmdTenantRelease:
		s.handleTenantRelease(conn, req)
	case CmdTenantRemove:
		s.handleTenantRemove(conn, req)
	case CmdTenantToken:
		s.handleTenantToken(conn, req)
	case CmdWorldAdd:
		s.handleWorldAdd(conn, req)
	case CmdWorldRelate:
		s.handleWorldRelate(conn, req)
	case CmdWorldResolve:
		s.handleWorldResolve(conn, req)
	case CmdWorldNeighbors:
		s.handleWorldNeighbors(conn, req)
	case CmdWorldLog:
		s.handleWorldLog(conn, req)
	case CmdWorldList:
		s.handleWorldList(conn, req)
	case CmdWorldGet:
		s.handleWorldGet(conn, req)
	case CmdWorldForget:
		s.handleWorldForget(conn, req)
	case CmdSkillList:
		s.handleSkillList(conn, req)
	case CmdSkillGet:
		s.handleSkillGet(conn, req)
	case CmdSkillHistory:
		s.handleSkillHistory(conn, req)
	case CmdSkillPromote:
		s.handleSkillPromote(conn, req)
	case CmdSkillQuarantine:
		s.handleSkillQuarantine(conn, req)
	case CmdSkillRevert:
		s.handleSkillRevert(conn, req)
	case CmdReflectRun:
		s.handleReflectRun(conn, req)
	case CmdReflectShow:
		s.handleReflectShow(conn, req)
	case CmdPulseStatus:
		s.handlePulseStatus(conn, req)
	case CmdPulsePause:
		s.handlePulsePause(conn, req)
	case CmdPulseResume:
		s.handlePulseResume(conn, req)
	case CmdInbox:
		s.handleInbox(conn, req)
	default:
		s.writeResp(conn, Response{ID: req.ID, Type: RespError, Error: "unknown command: " + req.Cmd})
	}
}

func (s *Server) writeResp(conn net.Conn, resp Response) {
	_ = writeResp(conn, resp)
}

// writeResp is the underlying writer that *returns* its error.
// Used by long-lived handlers (pulse) where a broken pipe is the
// client-disconnect signal that should stop the goroutine; the
// method form above keeps the fire-and-forget shape every other
// handler relies on.
func writeResp(conn net.Conn, resp Response) error {
	enc, err := json.Marshal(resp)
	if err != nil {
		return err
	}
	enc = append(enc, '\n')
	_ = conn.SetWriteDeadline(time.Now().Add(30 * time.Second))
	_, err = conn.Write(enc)
	return err
}

// ----- command handlers -----

func (s *Server) handleVersion(conn net.Conn, req Request) {
	s.writeResp(conn, Response{
		ID:   req.ID,
		Type: RespResult,
		Result: map[string]any{
			brand.Binary:       brand.Version,
			"protocol_version": brand.ProtocolVersion,
		},
	})
}

func (s *Server) handleHalt(conn net.Conn, req Request) {
	reason, _ := req.Args["reason"].(string)
	s.k.HaltWith(reason)
	s.writeResp(conn, Response{ID: req.ID, Type: RespResult, Result: map[string]any{
		"ok":     true,
		"halted": true,
		"reason": reason,
	}})
}

// handleCancelRun cancels a single in-flight run by correlation id (M32),
// leaving the kernel un-halted and other runs untouched — the targeted
// alternative to the global halt. Routes to the tenant kernel when a
// tenant is named (empty → primary), mirroring handleRun.
func (s *Server) handleCancelRun(conn net.Conn, req Request) {
	corr, _ := req.Args["correlation"].(string)
	if corr == "" {
		s.writeResp(conn, Response{ID: req.ID, Type: RespError, Error: "args.correlation required"})
		return
	}
	tenantID, _ := req.Args["tenant"].(string)
	k, err := s.kernelFor(tenantID)
	if err != nil {
		s.writeResp(conn, Response{ID: req.ID, Type: RespError, Error: err.Error()})
		return
	}
	cancelled := k.CancelRun(corr)
	s.writeResp(conn, Response{ID: req.ID, Type: RespResult, Result: map[string]any{
		"correlation": corr,
		"cancelled":   cancelled,
	}})
}

func (s *Server) handleResume(conn net.Conn, req Request) {
	reason, _ := req.Args["reason"].(string)
	s.k.ResumeWith(reason)
	s.writeResp(conn, Response{ID: req.ID, Type: RespResult, Result: map[string]any{
		"ok":     true,
		"halted": false,
		"reason": reason,
	}})
}

func (s *Server) handleWhy(conn net.Conn, req Request) {
	idAny, _ := req.Args["event_id"]
	id, _ := idAny.(string)
	if id == "" {
		s.writeResp(conn, Response{ID: req.ID, Type: RespError, Error: "args.event_id required"})
		return
	}
	// Tenant-scoped (M53): an empty tenant traces the primary journal; a named
	// tenant traces its own isolated journal, so a tenant walks only its own
	// events — completing tenant isolation on the observability surface (M39
	// did runs list/stats; this does why).
	k, err := s.kernelFor(tenantOf(req))
	if err != nil {
		s.writeResp(conn, Response{ID: req.ID, Type: RespError, Error: err.Error()})
		return
	}
	events, err := k.Why(id)
	if err != nil {
		s.writeResp(conn, Response{ID: req.ID, Type: RespError, Error: err.Error()})
		return
	}
	out := make([]any, 0, len(events))
	for _, e := range events {
		out = append(out, e)
	}
	// Parent backlink (M42): if this chain belongs to a sub-agent run,
	// surface its lead's correlation so an operator can walk child→parent
	// (only parent→child was visible before). correlation is the chain's
	// shared id; parent_correlation is "" for top-level runs.
	corr := ""
	if len(events) > 0 {
		corr = events[0].CorrelationID
	}
	parent := ""
	if corr != "" {
		parent = k.ParentOf(corr)
	}
	s.writeResp(conn, Response{
		ID:   req.ID,
		Type: RespResult,
		Result: map[string]any{
			"events":             out,
			"correlation":        corr,
			"parent_correlation": parent,
		},
	})
}

// handleWhoami reports the authenticated principal (M62). By the time a request
// reaches a handler, handleConn has already verified the token: the primary
// token equals s.Token(); any other token that got here authenticated as the
// tenant named in (and pinned to) req.Args["tenant"]. So identity is a pure
// read of req.Token vs the primary token — no new auth state.
func (s *Server) handleWhoami(conn net.Conn, req Request) {
	if req.Token == s.Token() {
		s.writeResp(conn, Response{
			ID:   req.ID,
			Type: RespResult,
			Result: map[string]any{
				"identity": "primary",
				"primary":  true,
				"tenant":   "",
			},
		})
		return
	}
	tenant, _ := req.Args["tenant"].(string)
	s.writeResp(conn, Response{
		ID:   req.ID,
		Type: RespResult,
		Result: map[string]any{
			"identity": "tenant",
			"primary":  false,
			"tenant":   tenant,
		},
	})
}

func (s *Server) handleVerify(conn net.Conn, req Request) {
	if err := s.k.Verify(); err != nil {
		s.writeResp(conn, Response{ID: req.ID, Type: RespError, Error: err.Error()})
		return
	}
	s.writeResp(conn, Response{ID: req.ID, Type: RespResult, Result: map[string]any{"ok": true}})
}

func (s *Server) handleApprovals(conn net.Conn, req Request) {
	pending := s.k.Approvals().Pending()
	out := make([]map[string]any, 0, len(pending))
	for _, p := range pending {
		out = append(out, map[string]any{
			"id":             p.ID,
			"capability":     p.Capability,
			"tool_name":      p.ToolName,
			"input":          p.Input,
			"reason":         p.Reason,
			"actor":          p.Actor,
			"correlation_id": p.CorrelationID,
			"created_unix":   p.CreatedAt.Unix(),
			"timeout_unix":   p.Timeout.Unix(),
		})
	}
	s.writeResp(conn, Response{
		ID:     req.ID,
		Type:   RespResult,
		Result: map[string]any{"pending": out, "count": len(out)},
	})
}

// planSpec is the JSON shape a `agt plan` client submits. It's a thin
// wire shape that the server reifies into scheduler.Plan with the
// kernel's wired LoopRunner + Approvals registry.
type planSpec struct {
	Name        string         `json:"name"`
	MaxParallel int            `json:"max_parallel"`
	Nodes       []planNodeSpec `json:"nodes"`
}

type planNodeSpec struct {
	ID   string   `json:"id"`
	Kind string   `json:"kind"`
	Deps []string `json:"deps,omitempty"`
	// Loop fields.
	Intent string `json:"intent,omitempty"`
	// Gate fields.
	Capability  string `json:"capability,omitempty"`
	Description string `json:"description,omitempty"`
}

func (s *Server) handlePlan(ctx context.Context, conn net.Conn, req Request) {
	rawAny, ok := req.Args["plan_json"]
	if !ok {
		s.writeResp(conn, Response{ID: req.ID, Type: RespError, Error: "args.plan_json required (JSON string)"})
		return
	}
	rawStr, ok := rawAny.(string)
	if !ok {
		s.writeResp(conn, Response{ID: req.ID, Type: RespError, Error: "args.plan_json must be a JSON string"})
		return
	}
	var spec planSpec
	if err := json.Unmarshal([]byte(rawStr), &spec); err != nil {
		s.writeResp(conn, Response{ID: req.ID, Type: RespError, Error: "plan_json parse: " + err.Error()})
		return
	}
	if len(spec.Nodes) == 0 {
		s.writeResp(conn, Response{ID: req.ID, Type: RespError, Error: "plan has no nodes"})
		return
	}

	runner := s.k.LoopRunner()
	apr := s.k.Approvals()
	nodes := make([]scheduler.Node, 0, len(spec.Nodes))
	for _, ns := range spec.Nodes {
		switch ns.Kind {
		case "loop":
			nodes = append(nodes, &scheduler.LoopNode{
				NodeID: ns.ID, Deps: ns.Deps, Intent: ns.Intent, Runner: runner,
			})
		case "gate":
			nodes = append(nodes, &scheduler.GateNode{
				NodeID: ns.ID, Deps: ns.Deps, Approvals: apr,
				Capability: ns.Capability, Description: ns.Description,
			})
		default:
			s.writeResp(conn, Response{ID: req.ID, Type: RespError,
				Error: fmt.Sprintf("node %q: unknown kind %q (want loop|gate)", ns.ID, ns.Kind)})
			return
		}
	}
	plan := scheduler.Plan{
		Name:        spec.Name,
		MaxParallel: spec.MaxParallel,
		Nodes:       nodes,
	}

	planID := "plan-" + strings.TrimPrefix(req.ID, "q")
	if planID == "plan-" {
		planID = ""
	}

	// Subscribe to per-plan events before launching so the client
	// sees plan.started + every node.* event in order.
	subjectPrefix := "plan."
	sub, err := s.k.Bus().Subscribe(subjectPrefix+">", 1024)
	if err != nil {
		s.writeResp(conn, Response{ID: req.ID, Type: RespError, Error: err.Error()})
		return
	}
	defer sub.Cancel()

	type planResult struct {
		res *scheduler.PlanResult
		err error
	}
	resultCh := make(chan planResult, 1)
	go func() {
		r, err := s.k.RunPlan(ctx, plan, planID)
		resultCh <- planResult{r, err}
	}()

	for {
		select {
		case ev, ok := <-sub.C:
			if !ok {
				s.writeResp(conn, Response{ID: req.ID, Type: RespError, Error: "event subscription closed"})
				return
			}
			s.writeResp(conn, Response{ID: req.ID, Type: RespEvent, Event: ev})
		case r := <-resultCh:
			// Drain in-flight events.
			drain := true
			for drain {
				select {
				case ev := <-sub.C:
					if ev == nil {
						drain = false
					} else {
						s.writeResp(conn, Response{ID: req.ID, Type: RespEvent, Event: ev})
					}
				default:
					drain = false
				}
			}
			if r.err != nil {
				s.writeResp(conn, Response{ID: req.ID, Type: RespError, Error: r.err.Error()})
				return
			}
			outputs := map[string]any{}
			for id, nr := range r.res.NodeResults {
				outputs[id] = nr.Output
			}
			s.writeResp(conn, Response{
				ID: req.ID, Type: RespResult,
				Result: map[string]any{
					"plan_id":      r.res.PlanID,
					"node_outputs": outputs,
				},
			})
			return
		case <-ctx.Done():
			return
		}
	}
}

func (s *Server) handleDecide(conn net.Conn, req Request) {
	idAny, _ := req.Args["id"]
	id, _ := idAny.(string)
	decAny, _ := req.Args["decision"]
	dec, _ := decAny.(string)
	reasonAny, _ := req.Args["reason"]
	reason, _ := reasonAny.(string)

	if id == "" {
		s.writeResp(conn, Response{ID: req.ID, Type: RespError, Error: "args.id required"})
		return
	}
	var decision approval.Decision
	switch dec {
	case "grant":
		decision = approval.DecisionGrant
	case "deny":
		decision = approval.DecisionDeny
	default:
		s.writeResp(conn, Response{ID: req.ID, Type: RespError, Error: `args.decision must be "grant" or "deny"`})
		return
	}
	if err := s.k.Approvals().Resolve(id, decision, reason, "operator"); err != nil {
		s.writeResp(conn, Response{ID: req.ID, Type: RespError, Error: err.Error()})
		return
	}
	s.writeResp(conn, Response{
		ID:     req.ID,
		Type:   RespResult,
		Result: map[string]any{"ok": true, "id": id, "decision": dec},
	})
}

// SetCancelOnDisconnect enables/disables cancelling a streaming run when its
// client connection drops (M35). Called once at startup by the daemon when
// AGEZT_CANCEL_ON_DISCONNECT=on. Off by default.
func (s *Server) SetCancelOnDisconnect(on bool) { s.cancelOnDisconnect = on }

func (s *Server) handleRun(ctx context.Context, conn net.Conn, req Request) {
	intentAny, _ := req.Args["intent"]
	intent, _ := intentAny.(string)
	intent = strings.TrimSpace(intent)
	if intent == "" {
		s.writeResp(conn, Response{ID: req.ID, Type: RespError, Error: "args.intent required"})
		return
	}

	// Optional tenant routing: an empty tenant runs on the primary kernel
	// (unchanged single-tenant path); a named tenant routes to its isolated
	// kernel via the registry.
	tenantID, _ := req.Args["tenant"].(string)
	k, err := s.kernelFor(tenantID)
	if err != nil {
		s.writeResp(conn, Response{ID: req.ID, Type: RespError, Error: err.Error()})
		return
	}

	// Pre-generate the correlation ID so we can subscribe to this run's
	// subject *before* starting it. No race; no missed events.
	corr := k.NewCorrelation()
	sub, err := k.Bus().Subscribe(k.SubjectForRun(corr), 1024)
	if err != nil {
		s.writeResp(conn, Response{ID: req.ID, Type: RespError, Error: err.Error()})
		return
	}
	defer sub.Cancel()

	type runResult struct {
		answer string
		err    error
	}
	resultCh := make(chan runResult, 1)
	go func() {
		ans, err := k.RunWith(ctx, corr, intent)
		resultCh <- runResult{ans, err}
	}()

	// Cancel-on-disconnect (M35): if enabled, watch the client connection and
	// cancel this run the moment the client goes away (Ctrl-C / killed),
	// instead of letting it run on headless. The client sends nothing after
	// its request, so a read unblocks only when the connection closes — at
	// which point we cancel via the same path as `agt runs cancel`. We clear
	// the read deadline first so a long run isn't mistaken for a disconnect
	// when the 10-minute handleConn deadline elapses. When the run finishes
	// normally, handleConn's defer closes the conn, the read returns, and
	// CancelRun is a harmless no-op (the run is already gone).
	if s.cancelOnDisconnect {
		go func() {
			_ = conn.SetReadDeadline(time.Time{})
			buf := make([]byte, 1)
			_, _ = conn.Read(buf) // blocks until the connection closes
			k.CancelRun(corr)
		}()
	}

	// Forward events to the client until the run finishes, then drain
	// the subscription one last time and send the final result.
	for {
		select {
		case ev, ok := <-sub.C:
			if !ok {
				// Subscription closed unexpectedly.
				s.writeResp(conn, Response{ID: req.ID, Type: RespError, Error: "event subscription closed"})
				return
			}
			s.writeResp(conn, Response{ID: req.ID, Type: RespEvent, Event: ev})
		case r := <-resultCh:
			// Drain any in-flight events for this run.
			drain := true
			for drain {
				select {
				case ev := <-sub.C:
					if ev == nil {
						drain = false
					} else {
						s.writeResp(conn, Response{ID: req.ID, Type: RespEvent, Event: ev})
					}
				default:
					drain = false
				}
			}
			if r.err != nil {
				s.writeResp(conn, Response{ID: req.ID, Type: RespError, Error: r.err.Error()})
				return
			}
			s.writeResp(conn, Response{
				ID:   req.ID,
				Type: RespResult,
				Result: map[string]any{
					"answer":         r.answer,
					"correlation_id": corr,
				},
			})
			return
		case <-ctx.Done():
			return
		}
	}
}
