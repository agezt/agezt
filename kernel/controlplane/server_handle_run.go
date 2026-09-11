// SPDX-License-Identifier: MIT

// Server handleRun dispatcher: the Run command's main path that resolves kernel/agent/profile/overrides and streams events back to the client.
// Code extracted from server_handle_run.go during the Day-52 god-file split. Public API unchanged.
package controlplane


import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net"
	"strings"
	"time"

	"github.com/agezt/agezt/kernel/agent"
	"github.com/agezt/agezt/kernel/event"
	"github.com/agezt/agezt/kernel/executionprofile"
	"github.com/agezt/agezt/kernel/memory"
	"github.com/agezt/agezt/kernel/roster"
	"github.com/agezt/agezt/kernel/runtime"
	"github.com/agezt/agezt/kernel/warden"
)



func (s *Server) handleRun(ctx context.Context, conn net.Conn, req Request) {
	intentAny := req.Args["intent"]
	intent, _ := intentAny.(string)
	intent = strings.TrimSpace(intent)
	if intent == "" {
		s.writeResp(conn, Response{ID: req.ID, Type: RespError, Error: "args.intent required"})
		return
	}

	// Optional tenant routing: an empty tenant runs on the primary kernel
	// (unchanged single-tenant path); a named tenant routes to its isolated
	// kernel via the registry.
	tenantID, _, err := argString(req.Args, "tenant")
	if err != nil {
		s.fail(conn, req, err)
		return
	}
	k, err := s.kernelFor(tenantID)
	if err != nil {
		s.fail(conn, req, err)
		return
	}

	// Per-run model override (M148): `agt run --model <id>` routes THIS run to a
	// different model (a cheaper/bigger one) without restarting the daemon — the
	// same per-request routing the OpenAI-compatible API uses. Empty = the kernel
	// default. effModel is the model this run will actually use; the capability
	// gate below must judge it, not the daemon default.
	modelRaw, _, merr := argString(req.Args, "model")
	if merr != nil {
		s.writeResp(conn, Response{ID: req.ID, Type: RespError, Error: merr.Error()})
		return
	}
	modelOverride := strings.TrimSpace(modelRaw)

	// Run AS a named agent (M783): `agt run --agent <slug>` resolves a roster
	// profile and applies its soul / model / per-run cost ceiling as this run's
	// DEFAULTS. Explicit per-run overrides still win; the profile fills the
	// gaps. Resolved before the vision gate so the gate judges the model the
	// run will actually use. An unknown or paused agent is a usage error —
	// silently running as the default identity would be worse than failing.
	agentRaw, _, agerr := argString(req.Args, "agent")
	if agerr != nil {
		s.writeResp(conn, Response{ID: req.ID, Type: RespError, Error: agerr.Error()})
		return
	}
	var agentProf *roster.Profile
	if agentRef := strings.TrimSpace(agentRaw); agentRef != "" {
		p, found := k.Roster().Get(agentRef)
		if !found {
			s.writeResp(conn, Response{ID: req.ID, Type: RespError, Error: "unknown agent: " + agentRef})
			return
		}
		if p.Retired {
			s.writeResp(conn, Response{ID: req.ID, Type: RespError, Error: "agent " + p.Slug + " is retired — revive it first (agt agent revive " + p.Slug + ")"})
			return
		}
		if !p.Enabled {
			s.writeResp(conn, Response{ID: req.ID, Type: RespError, Error: "agent " + p.Slug + " is paused (agt agent resume " + p.Slug + ")"})
			return
		}
		if !p.AllowsDirectCall() {
			s.writeResp(conn, Response{ID: req.ID, Type: RespError, Error: managedSubagentDirectCallError(p, "called")})
			return
		}
		agentProf = &p
		if modelOverride == "" {
			modelOverride = strings.TrimSpace(p.Model)
		}
		// The agent's memory follows it (M786): recalls — context injection
		// and the memory tool — default to its scope (private notes + shared).
		scope := strings.TrimSpace(p.MemoryScope)
		if scope == "" {
			scope = p.Slug
		}
		ctx = memory.WithScope(ctx, scope)
		// And its working directory (M792): file/shell tools operate inside
		// the profile's workspace subdirectory.
		ctx = agent.WithWorkdir(ctx, p.Workdir)
		// And its identity + daily ceiling for the Governor's ledger (M793).
		ctx = runtime.WithAgentIdent(ctx, p.Slug, p.MaxDailyMc)
		// Its own model fallback chain too (M787): primary (the resolved
		// model — an explicit --model still wins the front slot) followed by
		// the profile's ordered fallbacks; the Governor walks it in order.
		if len(p.Fallbacks) > 0 {
			primary := modelOverride
			if primary == "" {
				primary = k.Model()
			}
			ctx = runtime.WithModelChain(ctx, agentModelChain(primary, p.Fallbacks))
		}
	}
	effModel := k.Model()
	if modelOverride != "" {
		effModel = modelOverride
	}

	// Pre-generate the correlation id here (it was minted just before Subscribe
	// below) so the vision sidecar's journaled event links to this run.
	corr := k.NewCorrelation()

	// Vision capability gate (M91): a run carrying image attachments requires a
	// model confirmed to accept image input. Unlike the M25 tool gate (which
	// allows unknown models because many tolerate tool schemas), an image sent to
	// a non-vision model is a guaranteed hard failure — so this denies unless the
	// active model is confirmed vision-capable (confirmed-or-reject), pre-flight,
	// before any provider call. Enforced here at the submission boundary so the
	// agent loop and message type stay untouched.
	imageRefs, _, ierr := argStringList(req.Args, "images")
	if ierr != nil {
		s.writeResp(conn, Response{ID: req.ID, Type: RespError, Error: ierr.Error()})
		return
	}
	if len(imageRefs) > 0 {
		var visionOK bool
		if cat := k.Catalog(); cat != nil {
			if _, m := cat.FindModel(effModel); m != nil {
				visionOK = m.SupportsVision()
			}
		}
		if !visionOK {
			// Vision SIDECAR (M821): rather than rejecting, ask a keyed vision model
			// to describe the image(s) and inject that text into the intent, so a
			// non-vision active model still "reads" the photo. Fall back to the hard
			// rejection only when no vision model is configured.
			caption, derr := k.DescribeImages(ctx, corr, imageRefs, "")
			if derr == nil && strings.TrimSpace(caption) != "" {
				intent += "\n\n[Image description (analyzed by a vision model):\n" + caption + "\n]"
				imageRefs = nil // consumed by the sidecar; don't send raw images downstream
			} else {
				_, _ = k.Bus().Publish(event.Spec{
					Subject: "governor.capability",
					Kind:    event.KindCapabilityRejected,
					Actor:   "controlplane",
					Payload: map[string]any{
						"model":            effModel,
						"capability":       "vision",
						"images_requested": len(imageRefs),
					},
				})
				s.writeResp(conn, Response{ID: req.ID, Type: RespError, Error: fmt.Sprintf(
					"model %q does not support vision (image input); add a vision-capable provider key or attach images only to a vision-capable model (see `agt provider check --caps`)",
					effModel)})
				return
			}
		}
		// Gate passed or sidecar consumed the images (M93/M821): carry any remaining
		// image refs into the run so they reach the initial user message.
		if len(imageRefs) > 0 {
			ctx = runtime.WithImages(ctx, imageRefs)
		}
	}
	// Route this run to the override model when given (M148); the loop reads it
	// via modelFromCtx, the same path the OpenAI API uses.
	if modelOverride != "" {
		ctx = runtime.WithModel(ctx, modelOverride)
	}
	// Per-run system-prompt override (M149): replace the base system prompt for
	// this run only; memory/world/skill injection still layers on top. Stored
	// trimmed so the run, the operator, and the dry-run plan all agree.
	sysRaw, _, serr := argString(req.Args, "system")
	if serr != nil {
		s.writeResp(conn, Response{ID: req.ID, Type: RespError, Error: serr.Error()})
		return
	}
	systemOverride := strings.TrimSpace(sysRaw)
	if systemOverride == "" && agentProf != nil {
		systemOverride = strings.TrimSpace(agentProf.Soul) // the agent's soul IS its system prompt
	}
	if systemOverride != "" {
		ctx = runtime.WithSystem(ctx, systemOverride)
	}
	// Per-run wall-clock timeout override (M154): bound THIS run without a
	// daemon-wide cap. Parsed as a Go duration; a malformed value is a usage error.
	timeoutRaw, _, terr := argString(req.Args, "timeout")
	if terr != nil {
		s.writeResp(conn, Response{ID: req.ID, Type: RespError, Error: terr.Error()})
		return
	}
	timeoutRaw = strings.TrimSpace(timeoutRaw)
	if timeoutRaw != "" {
		d, perr := time.ParseDuration(timeoutRaw)
		if perr != nil || d <= 0 {
			s.writeResp(conn, Response{ID: req.ID, Type: RespError, Error: fmt.Sprintf("invalid timeout %q (want a positive Go duration like 30s, 2m)", timeoutRaw)})
			return
		}
		ctx = runtime.WithRunTimeout(ctx, d)
	}
	// Per-run tool restriction (M158): a "tools" arg (present, possibly empty)
	// scopes THIS run to the named tools only — an empty list = no tools at all
	// (a safe, pure-reasoning run). Absent = unrestricted (all tools). A present
	// but non-array value is a usage error, NOT silently a zero-tool run.
	toolsAllow, toolsSet, toerr := argStringList(req.Args, "tools")
	if toerr != nil {
		s.writeResp(conn, Response{ID: req.ID, Type: RespError, Error: toerr.Error()})
		return
	}
	if toolsSet {
		ctx = runtime.WithTools(ctx, toolsAllow)
	}
	// Per-run cost cap (M166): bound THIS run's cumulative provider spend (in
	// USD-microcents) without a daemon-wide ceiling. A malformed (non-numeric)
	// value is a usage error; a non-positive value is treated as uncapped.
	maxCost, _, mcerr := argInt64(req.Args, "max_cost")
	if mcerr != nil {
		s.writeResp(conn, Response{ID: req.ID, Type: RespError, Error: mcerr.Error()})
		return
	}
	if maxCost <= 0 && agentProf != nil {
		maxCost = agentProf.MaxCostMc // the agent's own per-run spend ceiling
	}
	if maxCost > 0 {
		ctx = runtime.WithMaxCost(ctx, maxCost)
	}

	// Per-run execution profile (Hermes parity): selects the requested warden
	// profile for shell/code_exec. Conditional profiles such as docker are only
	// accepted when the active backend can satisfy them; otherwise they are
	// rejected instead of silently falling back.
	execProfileRaw, _, eperr := argString(req.Args, "execution_profile")
	if eperr != nil {
		s.writeResp(conn, Response{ID: req.ID, Type: RespError, Error: eperr.Error()})
		return
	}
	execProfile := strings.TrimSpace(execProfileRaw)
	// No explicit profile? Fall back to the assigned agent's default isolation
	// surface (roster.Profile.ExecutionProfile), so an agent configured to run
	// sandboxed does so on direct runs too — an explicit arg still wins.
	if execProfile == "" && agentProf != nil {
		execProfile = strings.TrimSpace(agentProf.ExecutionProfile)
	}
	execProfileLabel := ""
	remoteExecutionProfile := false
	if execProfile != "" {
		if ok, reason := executionprofile.PolicyFromEnv().Allows(execProfile); !ok {
			s.writeResp(conn, Response{ID: req.ID, Type: RespError, Error: fmt.Sprintf(
				"execution profile %q is blocked by policy: %s", execProfile, reason,
			)})
			return
		}
		if strings.EqualFold(execProfile, "ssh") {
			sshCfg := executionprofile.SSHConfigFromEnv()
			if !sshCfg.Active() {
				s.writeResp(conn, Response{ID: req.ID, Type: RespError, Error: `execution profile "ssh" requires an active SSH backend (set AGEZT_EXEC_SSH=1 and AGEZT_EXEC_SSH_TARGET=user@host)`})
				return
			}
			execProfileLabel = "ssh"
			ctx = executionprofile.WithSSHOverride(ctx, sshCfg)
			goto executionProfileDone
		}
		if strings.EqualFold(execProfile, "k8s") {
			k8sCfg := executionprofile.K8sConfigFromEnv()
			if !k8sCfg.Active() {
				s.writeResp(conn, Response{ID: req.ID, Type: RespError, Error: `execution profile "k8s" requires an active Kubernetes backend (set AGEZT_EXEC_K8S=1 and AGEZT_EXEC_K8S_POD=<pod>; optional AGEZT_EXEC_K8S_NAMESPACE/CONTEXT/CONTAINER/WORKDIR)`})
				return
			}
			if _, ok := k.Tools()["shell"]; !ok {
				if _, ok := k.Tools()["code_exec"]; !ok {
					s.writeResp(conn, Response{ID: req.ID, Type: RespError, Error: `execution profile "k8s" requires the shell or code_exec tool to be registered`})
					return
				}
			}
			execProfileLabel = "k8s"
			ctx = executionprofile.WithK8sOverride(ctx, k8sCfg)
			goto executionProfileDone
		}
		if strings.EqualFold(execProfile, "modal") {
			modalCfg := executionprofile.ModalConfigFromEnv()
			if !modalCfg.Active() {
				s.writeResp(conn, Response{ID: req.ID, Type: RespError, Error: `execution profile "modal" requires an active Modal backend (set AGEZT_EXEC_MODAL=1; optional AGEZT_EXEC_MODAL_REF or AGEZT_EXEC_MODAL_IMAGE)`})
				return
			}
			if _, ok := k.Tools()["shell"]; !ok {
				if _, ok := k.Tools()["code_exec"]; !ok {
					s.writeResp(conn, Response{ID: req.ID, Type: RespError, Error: `execution profile "modal" requires the shell or code_exec tool to be registered`})
					return
				}
			}
			execProfileLabel = "modal"
			ctx = executionprofile.WithModalOverride(ctx, modalCfg)
			goto executionProfileDone
		}
		if strings.EqualFold(execProfile, "daytona") {
			daytonaCfg := executionprofile.DaytonaConfigFromEnv()
			if !daytonaCfg.Active() {
				s.writeResp(conn, Response{ID: req.ID, Type: RespError, Error: `execution profile "daytona" requires an active Daytona backend (set AGEZT_EXEC_DAYTONA=1 and AGEZT_EXEC_DAYTONA_SANDBOX=<id-or-name>)`})
				return
			}
			if _, ok := k.Tools()["shell"]; !ok {
				if _, ok := k.Tools()["code_exec"]; !ok {
					s.writeResp(conn, Response{ID: req.ID, Type: RespError, Error: `execution profile "daytona" requires the shell or code_exec tool to be registered`})
					return
				}
			}
			execProfileLabel = "daytona"
			ctx = executionprofile.WithDaytonaOverride(ctx, daytonaCfg)
			goto executionProfileDone
		}
		if strings.EqualFold(execProfile, "remote-agezt") {
			if _, ok := k.Tools()["remote_run"]; !ok {
				s.writeResp(conn, Response{ID: req.ID, Type: RespError, Error: `execution profile "remote-agezt" requires configured AGEZT peers (set AGEZT_PEERS=name=https://host|token so the remote_run tool is registered)`})
				return
			}
			execProfileLabel = "remote-agezt"
			remoteExecutionProfile = true
			goto executionProfileDone
		}
		p, ok := executionprofile.WardenProfileForRun(execProfile)
		if !ok {
			s.writeResp(conn, Response{ID: req.ID, Type: RespError, Error: fmt.Sprintf(
				"execution profile %q is not routable for run tools yet (supported: %s)",
				execProfile, strings.Join(executionprofile.RoutableRunProfileIDsFor(executionprofile.Build(executionprofile.Options{
					Tools:   toolNames(k.Tools()),
					Warden:  k.Warden(),
					SSH:     executionprofile.SSHConfigFromEnv(),
					K8s:     executionprofile.K8sConfigFromEnv(),
					Modal:   executionprofile.ModalConfigFromEnv(),
					Daytona: executionprofile.DaytonaConfigFromEnv(),
				})), ", ")),
			})
			return
		}
		if p == warden.ProfileContainer && k.Warden().EffectiveProfile(p) != warden.ProfileContainer {
			s.writeResp(conn, Response{ID: req.ID, Type: RespError, Error: fmt.Sprintf(
				"execution profile %q requires an active container backend (set AGEZT_WARDEN_DOCKER=1 and configure AGEZT_WARDEN_DOCKER_IMAGE/runtime)",
				execProfile),
			})
			return
		}
		execProfileLabel = string(p)
		ctx = warden.WithProfileOverride(ctx, p)
	}
executionProfileDone:

	remotePeerRaw, _, rperr := argString(req.Args, "remote_peer")
	if rperr != nil {
		s.writeResp(conn, Response{ID: req.ID, Type: RespError, Error: rperr.Error()})
		return
	}
	remotePeer := strings.TrimSpace(remotePeerRaw)
	if remotePeer != "" && !remoteExecutionProfile {
		s.writeResp(conn, Response{ID: req.ID, Type: RespError, Error: `remote_peer requires execution_profile "remote-agezt"`})
		return
	}

	// Session-scoped auto-approve grant (chat "auto-approve Tool Forge this
	// session"): a comma/space-separated list of edict capabilities to auto-grant
	// when policy would otherwise prompt for HITL approval, for this run and every
	// sub-agent it spawns. Never overrides a hard-deny. Used so standing up an
	// agent army doesn't prompt for each tool-forge approval.
	if caps := parseCapList(req.Args["auto_approve_caps"]); len(caps) > 0 {
		ctx = runtime.WithAutoApproveCapabilities(ctx, caps)
	}

	// Session-scoped "trust this run's web/file content" grant (chat toggle): the
	// operator is deliberately driving an agentic task and accepts the untrusted
	// observations it reads, so the prompt-injection guard downgrades from
	// blocking to warn FOR THIS RUN (and its sub-agents). Never overrides a
	// hard-deny or any other guard. Lets a chat-driven research+act loop run
	// without an approval prompt on every step.
	if argTruthy(req.Args["prompt_injection_trust"]) {
		ctx = runtime.WithTrustedObservations(ctx)
	}

	// Assured run (M651): when assure > 0, run the "do-it-for-sure" loop — run,
	// verify completion, retry with the gap fed back — up to that many attempts,
	// instead of a single pass. A malformed value is a usage error.
	assureN, _, acerr := argInt64(req.Args, "assure")
	if acerr != nil {
		s.writeResp(conn, Response{ID: req.ID, Type: RespError, Error: acerr.Error()})
		return
	}

	// Dry-run (M159): resolve exactly what this run WOULD do — effective model
	// (and its catalog capabilities), the system-prompt source, the effective
	// wall-clock timeout, and the precise tool set the agent loop would see after
	// the per-run filter — then return that plan WITHOUT starting the run or
	// spending a token. Parsed from the SAME locals the real run uses (no
	// re-reading req.Args), so the plan can never drift from what would execute.
	// A mistyped dry_run is rejected here rather than silently executing the run.
	dryRun, _, berr := argBool(req.Args, "dry_run")
	if berr != nil {
		s.writeResp(conn, Response{ID: req.ID, Type: RespError, Error: berr.Error()})
		return
	}
	if dryRun {
		in := runPlanInput{
			Intent:           intent,
			Tenant:           tenantID,
			Model:            effModel,
			ModelOverridden:  modelOverride != "",
			SystemSet:        strings.TrimSpace(k.System()) != "",
			SystemOverride:   systemOverride != "",
			Timeout:          timeoutRaw,
			DaemonTimeout:    k.MaxDuration(),
			AllowSet:         toolsSet,
			Allow:            toolsAllow,
			MaxCostMC:        maxCost,
			ExecutionProfile: execProfile,
			WardenProfile:    execProfileLabel,
			RemotePeer:       remotePeer,
			ModelPriced:      modelPriced(effModel), // authoritative (catalog → fallback table)
		}
		in.StrictPricing, in.ModelHasPrice = strictPricingPlan(k.Provider(), effModel)
		if cat := k.Catalog(); cat != nil {
			if _, m := cat.FindModel(effModel); m != nil {
				in.ModelKnown = true
				in.SupportsVision = m.SupportsVision()
				in.SupportsTools = m.ToolCall
				in.ContextLimit = m.Limit.Context
			}
		}
		for name := range k.Tools() {
			in.AllToolNames = append(in.AllToolNames, name)
		}
		s.writeResp(conn, Response{ID: req.ID, Type: RespResult, Result: buildRunPlan(in)})
		return
	}
	if remoteExecutionProfile && assureN > 0 {
		s.writeResp(conn, Response{ID: req.ID, Type: RespError, Error: `execution profile "remote-agezt" cannot combine with assure yet; delegate once or run assurance on the peer`})
		return
	}

	// corr was pre-generated above (before the vision gate) so we can subscribe to
	// this run's subject *before* starting it. No race; no missed events.
	sub, err := k.Bus().Subscribe(k.SubjectForRun(corr), 1024)
	if err != nil {
		s.fail(conn, req, err)
		return
	}
	defer sub.Cancel()

	// Cost-cap inert advisory (M169): a per-run cost cap can only trip if the run
	// accrues PRICED spend. That is now narrower than it was: since BIZ-001 an
	// unrecognised model bills at the unpriced fallback rate, so only a KNOWN-free
	// or local model still computes as $0 and leaves the cap unable to bind.
	// Journal an advisory tied to this run's
	// correlation so `agt why <run>` shows the guardrail was inert — the run-time
	// counterpart to the dry-run "will not bind" warning. Best-effort.
	if maxCost > 0 && !modelPriced(effModel) {
		_, _ = k.Bus().Publish(event.Spec{
			Subject:       "governor.budget",
			Kind:          event.KindBudgetCapInert,
			Actor:         "controlplane",
			CorrelationID: corr,
			Payload:       map[string]any{"model": effModel, "cap_microcents": maxCost},
		})
	}

	type runResult struct {
		answer string
		err    error
	}
	resultCh := make(chan runResult, 1)
	go func() {
		if remoteExecutionProfile {
			receivedPayload := map[string]any{
				"intent":            intent,
				"execution_profile": "remote-agezt",
				"remote_tool":       "remote_run",
			}
			if remotePeer != "" {
				receivedPayload["remote_peer"] = remotePeer
			}
			if err := publishRemoteExecutionProfileRunEvent(k, corr, event.KindTaskReceived, "task", receivedPayload); err != nil {
				resultCh <- runResult{"", err}
				return
			}
			delegatingInfo := map[string]any{
				"profile": "remote-agezt",
				"phase":   "delegating",
				"tool":    "remote_run",
			}
			if remotePeer != "" {
				delegatingInfo["remote_peer"] = remotePeer
			}
			if err := publishRemoteExecutionProfileRunEvent(k, corr, event.KindInfo, "execution_profile", delegatingInfo); err != nil {
				resultCh <- runResult{"", err}
				return
			}
			payload := map[string]string{"task": intent}
			if remotePeer != "" {
				payload["peer"] = remotePeer
			}
			if strings.TrimSpace(modelOverride) != "" {
				payload["model"] = effModel
			}
			raw, err := json.Marshal(payload)
			if err != nil {
				_ = publishRemoteExecutionProfileRunEvent(k, corr, event.KindTaskFailed, "task", map[string]any{
					"error":             err.Error(),
					"reason":            "remote-agezt",
					"execution_profile": "remote-agezt",
				})
				resultCh <- runResult{"", err}
				return
			}
			res, err := k.RunTool(ctx, corr, "execution-profile-remote-agezt", "remote_run", raw)
			if err != nil {
				_ = publishRemoteExecutionProfileRunEvent(k, corr, event.KindTaskFailed, "task", map[string]any{
					"error":             err.Error(),
					"reason":            "remote-agezt",
					"execution_profile": "remote-agezt",
				})
				resultCh <- runResult{"", err}
				return
			}
			if res.IsError {
				msg := strings.TrimSpace(res.Output)
				if msg == "" {
					msg = "remote-agezt execution profile failed"
				}
				err := errors.New(msg)
				_ = publishRemoteExecutionProfileRunEvent(k, corr, event.KindTaskFailed, "task", map[string]any{
					"error":             err.Error(),
					"reason":            "remote-agezt",
					"execution_profile": "remote-agezt",
				})
				resultCh <- runResult{"", err}
				return
			}
			peerMeta := remoteExecutionProfilePeerMetadata(res.Output)
			s.mirrorRemoteExecutionProfileEvents(ctx, k, corr, peerMeta)
			completedInfo := map[string]any{
				"profile": "remote-agezt",
				"phase":   "completed",
				"tool":    "remote_run",
				"chars":   len(res.Output),
			}
			addRemoteExecutionProfilePeerMetadata(completedInfo, peerMeta)
			if err := publishRemoteExecutionProfileRunEvent(k, corr, event.KindInfo, "execution_profile", completedInfo); err != nil {
				resultCh <- runResult{"", err}
				return
			}
			completedTask := map[string]any{
				"iters":             0,
				"chars":             len(res.Output),
				"stopped":           "remote-agezt",
				"answer":            remoteExecutionProfileAnswerPreview(res.Output),
				"execution_profile": "remote-agezt",
			}
			addRemoteExecutionProfilePeerMetadata(completedTask, peerMeta)
			if err := publishRemoteExecutionProfileRunEvent(k, corr, event.KindTaskCompleted, "task", completedTask); err != nil {
				resultCh <- runResult{"", err}
				return
			}
			resultCh <- runResult{res.Output, nil}
			return
		}
		if assureN > 0 {
			ans, _, err := k.RunAssured(ctx, corr, intent, int(assureN))
			resultCh <- runResult{ans, err}
			return
		}
		if agentProf != nil && agentProf.RetryPolicy != nil && agentProf.RetryPolicy.MaxAttempts > 1 {
			ans, err := k.RunWithRetry(ctx, corr, intent, *agentProf.RetryPolicy)
			resultCh <- runResult{ans, err}
			return
		}
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
			result := map[string]any{
				"answer":         r.answer,
				"correlation_id": corr,
			}
			if agentProf != nil {
				result["agent"] = agentProf.Slug // who the run executed AS (M789)
			}
			// Enrich with this run's cost/iters/model (M146) so `agt run` can report
			// what the run cost without a second round-trip. Reuses the same journal
			// fold as `agt runs` (so the numbers agree); best-effort — a fold error or
			// an unpriced run (mock) just omits the fields.
			if runs, ferr := s.collectRuns(k); ferr == nil {
				if e := runs[corr]; e != nil {
					result["iters"] = e.Iters
					result["spent_mc"] = e.SpentMicrocents
					if e.Model != "" {
						result["model"] = e.Model
					}
				}
			}
			s.writeResp(conn, Response{ID: req.ID, Type: RespResult, Result: result})
			return
		case <-ctx.Done():
			return
		}
	}
}