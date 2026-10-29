// SPDX-License-Identifier: MIT

package controlplane

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"github.com/agezt/agezt/kernel/contract/boardapi"
	"net"
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"time"

	"github.com/agezt/agezt/internal/atomicfile"
	"github.com/agezt/agezt/kernel/app"
	appapprovals "github.com/agezt/agezt/kernel/app/approvals"
	appartifacts "github.com/agezt/agezt/kernel/app/artifacts"
	appaudit "github.com/agezt/agezt/kernel/app/audit"
	appautonomy "github.com/agezt/agezt/kernel/app/autonomy"
	appboard "github.com/agezt/agezt/kernel/app/board"
	appbudget "github.com/agezt/agezt/kernel/app/budget"
	appcatalog "github.com/agezt/agezt/kernel/app/catalog"
	appchannels "github.com/agezt/agezt/kernel/app/channels"
	appchat "github.com/agezt/agezt/kernel/app/chat"
	appconfig "github.com/agezt/agezt/kernel/app/config"
	appconfigcenter "github.com/agezt/agezt/kernel/app/configcenter"
	appcorrelation "github.com/agezt/agezt/kernel/app/correlation"
	appcouncil "github.com/agezt/agezt/kernel/app/council"
	appdata "github.com/agezt/agezt/kernel/app/data"
	appedict "github.com/agezt/agezt/kernel/app/edict"
	appexecprofile "github.com/agezt/agezt/kernel/app/execprofile"
	appfiles "github.com/agezt/agezt/kernel/app/files"
	appjournal "github.com/agezt/agezt/kernel/app/journal"
	appmarket "github.com/agezt/agezt/kernel/app/market"
	appmemory "github.com/agezt/agezt/kernel/app/memory"
	appmissioncontrol "github.com/agezt/agezt/kernel/app/missioncontrol"
	appnodes "github.com/agezt/agezt/kernel/app/nodes"
	appokr "github.com/agezt/agezt/kernel/app/okr"
	apppersona "github.com/agezt/agezt/kernel/app/persona"
	appplugins "github.com/agezt/agezt/kernel/app/plugins"
	appproviders "github.com/agezt/agezt/kernel/app/providers"
	apppulse "github.com/agezt/agezt/kernel/app/pulse"
	appreaper "github.com/agezt/agezt/kernel/app/reaper"
	appredaction "github.com/agezt/agezt/kernel/app/redaction"
	appreflection "github.com/agezt/agezt/kernel/app/reflection"
	appresearch "github.com/agezt/agezt/kernel/app/research"
	approster "github.com/agezt/agezt/kernel/app/roster"
	approuting "github.com/agezt/agezt/kernel/app/routing"
	appruns "github.com/agezt/agezt/kernel/app/runs"
	appsandbox "github.com/agezt/agezt/kernel/app/sandbox"
	appschedule "github.com/agezt/agezt/kernel/app/schedule"
	appseats "github.com/agezt/agezt/kernel/app/seats"
	appsettings "github.com/agezt/agezt/kernel/app/settings"
	appskill "github.com/agezt/agezt/kernel/app/skill"
	appstanding "github.com/agezt/agezt/kernel/app/standing"
	appstate "github.com/agezt/agezt/kernel/app/state"
	appsteer "github.com/agezt/agezt/kernel/app/steer"
	appstorage "github.com/agezt/agezt/kernel/app/storage"
	"github.com/agezt/agezt/kernel/app/system"
	apptaste "github.com/agezt/agezt/kernel/app/taste"
	apptenants "github.com/agezt/agezt/kernel/app/tenants"
	apptools "github.com/agezt/agezt/kernel/app/tools"
	appupdate "github.com/agezt/agezt/kernel/app/update"
	appwebhook "github.com/agezt/agezt/kernel/app/webhook"
	appworkboard "github.com/agezt/agezt/kernel/app/workboard"
	appworkflow "github.com/agezt/agezt/kernel/app/workflow"
	appworld "github.com/agezt/agezt/kernel/app/world"
	"github.com/agezt/agezt/kernel/approval"
	"github.com/agezt/agezt/kernel/configcenter"
	"github.com/agezt/agezt/kernel/contract/agentapi"
	"github.com/agezt/agezt/kernel/contract/opapi"
	"github.com/agezt/agezt/kernel/edict"
	"github.com/agezt/agezt/kernel/event"
	"github.com/agezt/agezt/kernel/executionprofile"
	"github.com/agezt/agezt/kernel/platform/rollbackstore"
	"github.com/agezt/agezt/kernel/redact"
	"github.com/agezt/agezt/kernel/settings"
	"github.com/agezt/agezt/kernel/ulid"
)

type systemHostKey struct{}

var rosterListOperations = func() []app.Operation {
	ops, err := approster.ListOperations(func(ctx context.Context) *approster.ListService {
		return ctx.Value(systemHostKey{}).(*Server).rosterListService()
	})
	if err != nil {
		panic(err)
	}
	return ops
}()

var rosterProfileWriteOperations = func() []app.Operation {
	ops, err := approster.ProfileWriteOperations(func(ctx context.Context) *approster.ProfileWriteService {
		s := ctx.Value(systemHostKey{}).(*Server)
		ports := s.rosterPorts()
		return approster.NewProfileWrite(ports.Get, ports.Add, ports.Update, s.invalidateAgentListCache)
	})
	if err != nil {
		panic(err)
	}
	return ops
}()

var sandboxOperations = func() []app.Operation {
	ops, err := appsandbox.Operations(func(ctx context.Context) *appsandbox.Service {
		server := ctx.Value(systemHostKey{}).(*Server)
		return appsandbox.New(filepath.Join(server.baseDirPorts().Dir(), "sandbox", "projects"))
	})
	if err != nil {
		panic(err)
	}
	return ops
}()

var budgetOperations = func() []app.Operation {
	ops, err := appbudget.Operations(func(ctx context.Context) *appbudget.Service {
		s := ctx.Value(systemHostKey{}).(*Server)
		return appbudget.New(appbudget.GovernorFrom(s.providerPorts().Budget()))
	})
	if err != nil {
		panic(err)
	}
	return ops
}()

var routingOperations = func() []app.Operation {
	ops, err := approuting.Operations(func(ctx context.Context) *approuting.Service {
		s := ctx.Value(systemHostKey{}).(*Server)
		provider := s.providerPorts()
		return approuting.New(approuting.Ports{
			TaskChains:    provider.TaskChains,
			SetTaskChains: provider.SetTaskChains,
			Named:         provider.Named,
			SetNamed:      provider.SetNamed,
			Agents: func() []approuting.Agent {
				return approuting.Agents(s.rosterPorts().List())
			},
			Journal: func(visit func(*event.Event)) {
				_ = s.journalPorts().Events.Range(func(e *event.Event) error {
					visit(e)
					return nil
				})
			},
			Store: func() approuting.Store { return settings.NewStore(s.baseDir) },
			Models: func() func(string) bool {
				cat := s.catalogPorts().Snapshot()
				return func(model string) bool {
					_, m := cat.FindModel(model)
					return m != nil
				}
			},
		})
	})
	if err != nil {
		panic(err)
	}
	return ops
}()

var executionProfileOperations = func() []app.Operation {
	ops, err := appexecprofile.Operations(func(ctx context.Context) *appexecprofile.Service {
		k := ctx.Value(appHostKey{}).(appHost).kernel
		return appexecprofile.New(appexecprofile.Ports{
			Inventory: func() executionprofile.Inventory { return appexecprofile.Inventory(k) },
			Policy:    executionprofile.PolicyFromEnv,
		})
	})
	if err != nil {
		panic(err)
	}
	return ops
}()

// runOperations binds the run operation to the routed kernel. The run is
// tenant-routed (a tenant's run stays inside its own kernel) and cancels
// itself when the operator's client goes away, so the host supplies the
// connection's disconnect signal.
var runOperations = func() []app.Operation {
	ops, err := appruns.RunOperations(func(ctx context.Context) *appruns.RunService {
		k := ctx.Value(appHostKey{}).(appHost).kernel
		s := ctx.Value(systemHostKey{}).(*Server)
		conn, _ := ctx.Value(appConnKey{}).(net.Conn)
		return appruns.NewRunService(appruns.RunPorts{
			Kernel: k,
			Run: func(corr string) (appruns.Run, bool, error) {
				rows, err := s.runFold(k)
				if err != nil {
					return appruns.Run{}, false, err
				}
				row, ok := rows[corr]
				return row, ok, nil
			},
			CancelOnDisconnect: s.cancelOnDisconnect,
			Disconnect:         func() <-chan struct{} { return disconnectSignal(conn) },
			MirrorRemote: func(ctx context.Context, corr string, meta map[string]string) {
				s.mirrorRemoteExecutionProfileEvents(ctx, k, corr, meta)
			},
		})
	})
	if err != nil {
		panic(err)
	}
	return ops
}()

var dataOperations = func() []app.Operation {
	ops, err := appdata.Operations(func(ctx context.Context) *appdata.Service {
		if lake := ctx.Value(systemHostKey{}).(*Server).k.DataLake(); lake != nil {
			return appdata.New(lake)
		}
		return appdata.New(nil)
	})
	if err != nil {
		panic(err)
	}
	return ops
}()

var personaOperations = func() []app.Operation {
	ops, err := apppersona.Operations(func(ctx context.Context) *apppersona.Service {
		s := ctx.Value(systemHostKey{}).(*Server)
		prompts := filepath.Join(s.baseDir, "chat_prompts.json")
		ports := s.personaPorts()
		ports.Store = func() apppersona.Store { return settings.NewStore(s.baseDir) }
		ports.ReadPrompts = func() ([]byte, error) { return os.ReadFile(prompts) }
		// The prompt library is written through the platform's atomic
		// writer, so a reader never sees a half-written library. The
		// library is read back on every Chat view, and a torn file
		// would read as an EMPTY library rather than as an error — the
		// operator would see their prompts gone and no reason why.
		ports.SavePrompts = func(data []byte) error { return atomicfile.WriteFile(prompts, data, 0o600) }
		return apppersona.New(ports)
	})
	if err != nil {
		panic(err)
	}
	return ops
}()

var seatOperations = func() []app.Operation {
	ops, err := appseats.Operations(func(ctx context.Context) *appseats.Service {
		return appseats.New(ctx.Value(systemHostKey{}).(*Server).seatPorts().Store())
	})
	if err != nil {
		panic(err)
	}
	return ops
}()

var councilOperations = func() []app.Operation {
	ops, err := appcouncil.Operations(func(ctx context.Context) *appcouncil.Service {
		s := ctx.Value(systemHostKey{}).(*Server)
		return appcouncil.New(appcouncil.PortsFromKernel(s.k, s.baseDir))
	})
	if err != nil {
		panic(err)
	}
	return ops
}()

var councilAskOperations = func() []app.Operation {
	ops, err := appcouncil.AskOperations(func(ctx context.Context) appcouncil.Engines {
		k := ctx.Value(systemHostKey{}).(*Server).k
		return appcouncil.EnginesFromKernel(k)
	})
	if err != nil {
		panic(err)
	}
	return ops
}()

var researchOperations = func() []app.Operation {
	ops, err := appresearch.AskOperations(func(ctx context.Context) appresearch.Engines {
		k := ctx.Value(systemHostKey{}).(*Server).k
		return appresearch.Engines{
			Research:       k.Research,
			NewCorrelation: ctx.Value(systemHostKey{}).(*Server).correlationPorts().Mint,
		}
	})
	if err != nil {
		panic(err)
	}
	return ops
}()

var reflectionOperations = func() []app.Operation {
	ops, err := appreflection.Operations(func(ctx context.Context) *appreflection.Service {
		return appreflection.New(ctx.Value(systemHostKey{}).(*Server).k.Reflect(), ulid.New)
	})
	if err != nil {
		panic(err)
	}
	return ops
}()

var chatSummaryOperations = func() []app.Operation {
	ops, err := appchat.SummaryOperations(func(ctx context.Context) *appchat.Summarizer {
		s := ctx.Value(systemHostKey{}).(*Server)
		// The default model is the IDENTITY family's answer: the summarizer
		// asks the persona family for it rather than reaching for the kernel.
		return appchat.NewSummarizer(appchat.SummaryPorts{Provider: s.providerPorts().Live, Model: s.personaPorts().Identity.DefaultModel})
	})
	if err != nil {
		panic(err)
	}
	return ops
}()

var chatOperations = func() []app.Operation {
	ops, err := appchat.Operations(func(ctx context.Context) *appchat.Service {
		var ports appchat.Ports
		ports.ActiveMemory = ctx.Value(systemHostKey{}).(*Server).memoryPorts().Active
		return appchat.New(ports)
	})
	if err != nil {
		panic(err)
	}
	return ops
}()

var nodeOperations = func() []app.Operation {
	ops, err := appnodes.Operations(func(ctx context.Context) *appnodes.Service {
		s := ctx.Value(systemHostKey{}).(*Server)
		return appnodes.New(appnodes.Ports{
			Model:     s.personaPorts().Identity.DefaultModel,
			RemoteRun: s.toolsPorts().RemoteRun,
			PeerSpec:  s.nodePeerSpec,
			Client:    nodeProbeClient,
		})
	})
	if err != nil {
		panic(err)
	}
	return ops
}()

var fileOperations = func() []app.Operation {
	ops, err := appfiles.Operations(func(ctx context.Context) *appfiles.Service {
		s := ctx.Value(systemHostKey{}).(*Server)
		host := ctx.Value(appHostKey{}).(appHost)
		callID := app.RequestID(ctx)
		return appfiles.New(appfiles.Ports{Runner: host.kernel, Correlation: host.correlation, CallID: callID, CatalogPath: filepath.Join(s.baseDir, filepath.FromSlash(rollbackstore.RelativePath))})
	})
	if err != nil {
		panic(err)
	}
	return ops
}()

var missionControlOperations = func() []app.Operation {
	ops, err := appmissioncontrol.Operations(func(ctx context.Context) *appmissioncontrol.Service {
		s := ctx.Value(systemHostKey{}).(*Server)
		return appmissioncontrol.New(appmissioncontrol.Ports{
			Spend: s.providerPorts().SpentMicrocents,
			Pending: func() []approval.Request {
				if registry := s.approvalPorts().Registry(); registry != nil {
					return registry.Pending()
				}
				return nil
			},
			Asks: func() []map[string]any {
				if s.pulse != nil {
					return s.pulse.PendingAsks()
				}
				return nil
			},
			Now: time.Now,
		})
	})
	if err != nil {
		panic(err)
	}
	return ops
}()

var reaperOperations = func() []app.Operation {
	ops, err := appreaper.Operations(func(ctx context.Context) *appreaper.Service {
		return appreaper.New(ctx.Value(systemHostKey{}).(*Server).k.ReaperScan, time.Now)
	})
	if err != nil {
		panic(err)
	}
	return ops
}()

var redactionOperations = func() []app.Operation {
	ops, err := appredaction.Operations(func(ctx context.Context) *appredaction.Service {
		ports := ctx.Value(systemHostKey{}).(*Server).busPorts()
		return appredaction.New(func() appredaction.Redactor { return ports.Redactor() }, redact.MatchedCategories)
	})
	if err != nil {
		panic(err)
	}
	return ops
}()

var stateOperations = func() []app.Operation {
	ops, err := appstate.Operations(func(ctx context.Context) *appstate.Service {
		return appstate.New(ctx.Value(systemHostKey{}).(*Server).statePorts().Store())
	})
	if err != nil {
		panic(err)
	}
	return ops
}()

var rosterPermissionOperations = func() []app.Operation {
	ops, err := approster.PermissionOperations(func(ctx context.Context) *approster.PermissionService {
		s := ctx.Value(systemHostKey{}).(*Server)
		return approster.NewPermissions(approster.PermissionPorts{
			Get:    s.rosterPorts().Get,
			Update: s.rosterPorts().Update,
			Tools:  s.toolsPorts().Registry.Registered,
			Decide: func(capability edict.Capability, ceiling edict.TrustLevel) edict.Outcome {
				return s.edictPorts().DecideWithCeiling(capability, "", ceiling)
			},
			Config: func() ([]*configcenter.ConfigEntry, bool) {
				return s.configCenterPorts().Entries()
			},
		})
	})
	if err != nil {
		panic(err)
	}
	return ops
}()

var edictReadOperations = func() []app.Operation {
	ops, err := appedict.Operations(func(ctx context.Context) *appedict.Service {
		return appedict.New(requestEdictPorts(ctx).Engine())
	})
	if err != nil {
		panic(err)
	}
	return ops
}()

var edictDecisionOperations = func() []app.Operation {
	ops, err := appedict.DecisionOperations(func(ctx context.Context) *appedict.Decisions {
		return appedict.NewDecisions(requestJournalPorts(ctx).Events, time.Now)
	})
	if err != nil {
		panic(err)
	}
	return ops
}()

var edictOverlayOperations = func() []app.Operation {
	ops, err := appedict.OverlayOperations(func(ctx context.Context) *appedict.Overlay {
		save := func(snap *edict.OverlaySnapshot) error {
			return edict.SaveOverlaySnapshot(filepath.Join(requestBaseDirPorts(ctx).Dir(), "runtime", edict.OverlaySnapshotFile), snap)
		}
		return appedict.NewOverlay(requestJournalPorts(ctx).Events, save, func(spec event.Spec) { _, _ = requestBusPorts(ctx).Publish(spec) })
	})
	if err != nil {
		panic(err)
	}
	return ops
}()

var edictWriteOperations = func() []app.Operation {
	ops, err := appedict.WriteOperations(func(ctx context.Context) *appedict.Writes {
		return appedict.NewWrites(requestEdictPorts(ctx).WriteEngine(), func(spec event.Spec) { _, _ = requestBusPorts(ctx).Publish(spec) })
	})
	if err != nil {
		panic(err)
	}
	return ops
}()

var approvalLiveOperations = func() []app.Operation {
	ops, err := appapprovals.LiveOperations(func(ctx context.Context) *appapprovals.Live {
		return appapprovals.NewLive(requestApprovalPorts(ctx).Broker())
	})
	if err != nil {
		panic(err)
	}
	return ops
}()

var approvalHistoryOperations = func() []app.Operation {
	ops, err := appapprovals.Operations(func(ctx context.Context) *appapprovals.History {
		return appapprovals.New(requestJournalPorts(ctx).Events, time.Now)
	})
	if err != nil {
		panic(err)
	}
	return ops
}()

var auditOperations = func() []app.Operation {
	ops, err := appaudit.Operations(func(ctx context.Context) *appaudit.Service {
		return appaudit.New(requestJournalPorts(ctx).Events, time.Now)
	})
	if err != nil {
		panic(err)
	}
	return ops
}()

var traceOperations = func() []app.Operation {
	ops, err := appjournal.TraceOperations(func(ctx context.Context) *appjournal.Trace {
		return appjournal.NewTrace(requestJournalPorts(ctx).Events)
	})
	if err != nil {
		panic(err)
	}
	return ops
}()

var journalOperations = func() []app.Operation {
	ops, err := appjournal.Operations(func(ctx context.Context) *appjournal.Service {
		return journalReads(ctx)
	})
	if err != nil {
		panic(err)
	}
	return ops
}()

var tenantOperations = func() []app.Operation {
	ops, err := apptenants.Operations(func(ctx context.Context) *apptenants.Service {
		return ctx.Value(systemHostKey{}).(*Server).tenantService()
	})
	if err != nil {
		panic(err)
	}
	return ops
}()

var planAuthoringOperations = func() []app.Operation {
	ops, err := appruns.PlanAuthoringOperations(func(ctx context.Context) *appruns.PlanAuthoring {
		return appruns.NewPlanAuthoring(ctx.Value(systemHostKey{}).(*Server).providerPorts().Live)
	})
	if err != nil {
		panic(err)
	}
	return ops
}()

var planRunOperations = func() []app.Operation {
	ops, err := appruns.PlanRunOperations(func(ctx context.Context) *appruns.PlanRunner {
		s := ctx.Value(systemHostKey{}).(*Server)
		k := s.k
		return appruns.NewPlanRunner(k.LoopRunner, s.approvalPorts().Registry, k.RunPlan, func(pattern string, buffer int) (appruns.PlanSubscription, error) {
			sub, err := s.busPorts().Subscribe(pattern, buffer)
			if err != nil {
				return nil, err
			}
			return appruns.BusSubscription{Subscription: sub}, nil
		})
	})
	if err != nil {
		panic(err)
	}
	return ops
}()

var planOperations = func() []app.Operation {
	ops, err := appruns.PlanOperations(func(ctx context.Context) *appruns.Plans {
		return appruns.NewPlans(requestJournalPorts(ctx).Events)
	})
	if err != nil {
		panic(err)
	}
	return ops
}()

var runsOperations = func() []app.Operation {
	ops, err := appruns.Operations(func(ctx context.Context) *appruns.Service {
		return ctx.Value(systemHostKey{}).(*Server).runReads(ctx.Value(appHostKey{}).(appHost).kernel)
	})
	if err != nil {
		panic(err)
	}
	return ops
}()

var steerOperations = func() []app.Operation {
	ops, err := appsteer.Operations(func(ctx context.Context) *appsteer.Service {
		return appsteer.New(ctx.Value(appHostKey{}).(appHost).kernel)
	})
	if err != nil {
		panic(err)
	}
	return ops
}()

var rosterRemoveOperations = func() []app.Operation {
	ops, err := approster.RemoveOperations(func(ctx context.Context) *approster.RemoveService {
		s := ctx.Value(systemHostKey{}).(*Server)
		return approster.NewRemove(approster.RemovePorts{
			Get:       s.rosterPorts().Get,
			Subagents: s.agentSubagents,
			Retained:  s.agentRemovalMailboxImpact,
			Workflows: s.agentWorkflowImpact,
			Retire:    s.retireAgentSubagents,
			Standing:  s.removeAgentStanding,
			Schedules: s.removeAgentSchedules,
			Memory:    s.forgetAgentMemory,
			Authored:  s.forgetAgentAuthoredSharedMemory,
			Skills:    s.archiveAgentSkills,
			Config:    s.deleteAgentConfigEntries,
			Workspace: s.deleteAgentWorkspace,
			Remove:    s.rosterPorts().Remove,
			NewCorr:   s.correlationPorts().Mint,
			Publish: func(subject, corr string, payload map[string]any) {
				publishOperatorAction(s.k, subject, corr, payload)
			},
			Invalidate: s.invalidateAgentListCache,
		})
	})
	if err != nil {
		panic(err)
	}
	return ops
}()

var rosterSetRetiredOperations = func() []app.Operation {
	ops, err := approster.SetRetiredOperations(func(ctx context.Context) *approster.SetRetiredService {
		s := ctx.Value(systemHostKey{}).(*Server)
		return approster.NewSetRetired(approster.SetRetiredPorts{
			Get:    s.rosterPorts().Get,
			Impact: s.impactService().Summary,
			SetRetired: func(ref string, retired bool, reason string) (agentapi.Profile, error) {
				return s.rosterPorts().SetRetired(ref, retired, reason)
			},
			PauseStanding:        s.pauseAgentStanding,
			PauseSchedules:       s.pauseAgentSchedules,
			CountPausedStanding:  s.countAgentPausedStanding,
			CountPausedSchedules: s.countAgentPausedSchedules,
			NewCorrelation:       s.correlationPorts().Mint,
			Publish: func(subject, corr string, payload map[string]any) {
				publishOperatorAction(s.k, subject, corr, payload)
			},
			Invalidate: s.invalidateAgentListCache,
		})
	})
	if err != nil {
		panic(err)
	}
	return ops
}()

var rosterImpactOperations = func() []app.Operation {
	ops, err := approster.ImpactOperations(func(ctx context.Context) *approster.ImpactService {
		return ctx.Value(systemHostKey{}).(*Server).impactService()
	})
	if err != nil {
		panic(err)
	}
	return ops
}()

var rosterResolveOperations = func() []app.Operation {
	ops, err := approster.ResolveOperations(func(ctx context.Context) *approster.ResolveService {
		s := ctx.Value(systemHostKey{}).(*Server)
		return approster.NewResolve(approster.ResolvePorts{
			Get:            s.rosterPorts().Get,
			NewCorrelation: s.correlationPorts().Mint,
			Publish: func(subject, corr string, payload map[string]any) {
				publishOperatorAction(s.k, subject, corr, payload)
			},
			Pause: func(slug string) error {
				_, err := s.rosterPorts().SetEnabled(slug, false)
				return err
			},
			Retire: func(slug, reason string) error {
				_, err := s.rosterPorts().SetRetired(slug, true, reason)
				return err
			},
			HelpRequest: s.postOperatorHelp,
			ExhaustedChain: func(slug string, l approster.IncidentLineage, taskType string) []string {
				return approster.LatestExhaustedRoutingChain(s.journalPorts().Events.Range, slug, l, taskType)
			},
			ForceGeneration: func(slug, taskType string) int {
				return approster.LatestOperatorForceGeneration(s.journalPorts().Events.Range, slug, taskType)
			},
			ApplyChain: s.applyRoutingChain,
		})
	})
	if err != nil {
		panic(err)
	}
	return ops
}()

var rosterRepairOperations = func() []app.Operation {
	ops, err := approster.RepairOperations(func(ctx context.Context) *approster.RepairService {
		s := ctx.Value(systemHostKey{}).(*Server)
		return approster.NewRepair(s.rosterPorts().Get, s.correlationPorts().Mint, func(subject, corr string, payload map[string]any) {
			publishOperatorAction(s.k, subject, corr, payload)
		}, func(corr string, p agentapi.Profile, reason string, l approster.IncidentLineage) {
			go s.launchOperatorRepair(corr, p, reason, l)
		})
	})
	if err != nil {
		panic(err)
	}
	return ops
}()

var rosterWakeOperations = func() []app.Operation {
	ops, err := approster.WakeOperations(func(ctx context.Context) *approster.WakeService {
		s := ctx.Value(systemHostKey{}).(*Server)
		return approster.NewWake(s.rosterPorts().Get, s.correlationPorts().Mint, func(subject, corr string, payload map[string]any) {
			publishOperatorAction(s.k, subject, corr, payload)
		}, func(corr string, p agentapi.Profile, intent, reason string, l approster.IncidentLineage) {
			go s.launchOperatorWake(corr, p, intent, reason, l)
		})
	})
	if err != nil {
		panic(err)
	}
	return ops
}()

var rosterTaskUpdateOperations = func() []app.Operation {
	ops, err := approster.TaskUpdateOperations(func(ctx context.Context) *approster.TaskUpdateService {
		s := ctx.Value(systemHostKey{}).(*Server)
		return approster.NewTaskUpdate(s.rosterPorts().Update)
	})
	if err != nil {
		panic(err)
	}
	return ops
}()

var rosterSetEnabledOperations = func() []app.Operation {
	ops, err := approster.SetEnabledOperations(func(ctx context.Context) *approster.SetEnabledService {
		s := ctx.Value(systemHostKey{}).(*Server)
		return approster.NewSetEnabled(s.rosterPorts().SetEnabled, s.countAgentPausedStanding, s.countAgentPausedSchedules, s.invalidateAgentListCache)
	})
	if err != nil {
		panic(err)
	}
	return ops
}()

// nativeEscalationBoard reads the help topic and replies under the native
// board read cap for the application escalation fold.
type nativeEscalationBoard struct{ st boardapi.Store }

func (b nativeEscalationBoard) Help() []boardapi.Message { return b.st.Read("help", boardReadMaxLimit) }
func (b nativeEscalationBoard) Replies(id string) []boardapi.Message {
	return b.st.Replies(id, boardReadMaxLimit)
}

var rosterEscalationOperations = func() []app.Operation {
	ops, err := approster.EscalationOperations(func(ctx context.Context) *approster.EscalationService {
		s := ctx.Value(systemHostKey{}).(*Server)
		return approster.NewEscalations(s.rosterPorts().Get, func() (approster.EscalationBoard, error) {
			st, err := s.boardReader()
			if err != nil {
				return nil, err
			}
			return nativeEscalationBoard{st}, nil
		}, s.journalPorts().Events.Range)
	})
	if err != nil {
		panic(err)
	}
	return ops
}()

var rosterRepairStatusOperations = func() []app.Operation {
	ops, err := approster.RepairStatusOperations(func(ctx context.Context) *approster.RepairStatusService {
		s := ctx.Value(systemHostKey{}).(*Server)
		return approster.NewRepairStatus(s.rosterPorts().Get, s.journalPorts().Events.Range, approster.AutoRepairCooldown, nil)
	})
	if err != nil {
		panic(err)
	}
	return ops
}()

var rosterActivityOperations = func() []app.Operation {
	ops, err := approster.ActivityOperations(func(ctx context.Context) *approster.ActivityService {
		s := ctx.Value(systemHostKey{}).(*Server)
		return approster.NewActivity(s.rosterPorts().Get, s.journalPorts().Events.Range)
	})
	if err != nil {
		panic(err)
	}
	return ops
}()

var rosterGraveyardOperations = func() []app.Operation {
	ops, err := approster.GraveyardOperations(func(ctx context.Context) *approster.GraveyardService {
		s := ctx.Value(systemHostKey{}).(*Server)
		return approster.NewGraveyard(func() []agentapi.Profile { return s.rosterPorts().List() }, nil)
	})
	if err != nil {
		panic(err)
	}
	return ops
}()

var updateOperations = func() []app.Operation {
	ops, err := appupdate.Operations(func(ctx context.Context) *appupdate.Service {
		return ctx.Value(systemHostKey{}).(*Server).operatorUpdate()
	})
	if err != nil {
		panic(err)
	}
	return ops
}()

var webhookOperations = func() []app.Operation {
	ops, err := appwebhook.Operations(func(ctx context.Context) *appwebhook.Observability {
		return appwebhook.NewObservability(requestJournalPorts(ctx).Events, nil)
	})
	if err != nil {
		panic(err)
	}
	return ops
}()

var lifecycleOperations = func() []app.Operation {
	ops, err := system.LifecycleOperations(func(ctx context.Context) *system.Lifecycle {
		s := ctx.Value(systemHostKey{}).(*Server)
		return system.NewLifecycle(s.k, s.scheduleShutdown)
	})
	if err != nil {
		panic(err)
	}
	return ops
}()

var systemOperations = func() []app.Operation {
	ops, err := system.Operations(func(ctx context.Context) *system.Service { return ctx.Value(systemHostKey{}).(*Server).systemService() })
	if err != nil {
		panic(err)
	}
	return ops
}()

var catalogOperations = func() []app.Operation {
	ops, err := appcatalog.Operations(func(ctx context.Context) *appcatalog.Service {
		server := ctx.Value(systemHostKey{}).(*Server)
		return appcatalog.New(requestCatalogPorts(ctx), requestBusPorts(ctx), server.baseDir)
	})
	if err != nil {
		panic(err)
	}
	return ops
}()

var providerOperations = func() []app.Operation {
	ops, err := appproviders.Operations(func(ctx context.Context) *appproviders.Service {
		server := ctx.Value(systemHostKey{}).(*Server)
		return appproviders.New(requestCatalogPorts(ctx), server.baseDir)
	})
	if err != nil {
		panic(err)
	}
	return ops
}()

var oauthOperations = func() []app.Operation {
	ops, err := appproviders.OAuthOperations(func(ctx context.Context) *appproviders.OAuth {
		return ctx.Value(systemHostKey{}).(*Server).providerOAuth()
	})
	if err != nil {
		panic(err)
	}
	return ops
}()

var observationOperations = func() []app.Operation {
	operations, err := appproviders.ObservationOperations(func(ctx context.Context) *appproviders.Observations {
		return appproviders.NewObservations(requestJournalPorts(ctx).Events)
	})
	if err != nil {
		panic(err)
	}
	return operations
}()

var probeOperations = func() []app.Operation {
	operations, err := appproviders.ProbeOperations(func(context.Context) *appproviders.Probe { return appproviders.NewProbe(nil) })
	if err != nil {
		panic(err)
	}
	return operations
}()

var acpInventoryOperations = func() []app.Operation {
	operations, err := appchannels.ACPInventoryOperations(func(context.Context) *appchannels.ACPInventory { return appchannels.NewACPInventory(nil, nil) })
	if err != nil {
		panic(err)
	}
	return operations
}()

var channelSendOperations = func() []app.Operation {
	operations, err := appchannels.SendOperations(func(ctx context.Context) *appchannels.Outbound {
		return ctx.Value(systemHostKey{}).(*Server).channelOutbound()
	})
	if err != nil {
		panic(err)
	}
	return operations
}()

var channelInboxOperations = func() []app.Operation {
	operations, err := appchannels.InboxOperations(func(ctx context.Context) *appchannels.Inbox {
		return ctx.Value(systemHostKey{}).(*Server).channelInbox()
	})
	if err != nil {
		panic(err)
	}
	return operations
}()

var channelGatewayOperations = func() []app.Operation {
	operations, err := appchannels.GatewayOperations(func(ctx context.Context) *appchannels.Gateway {
		return ctx.Value(systemHostKey{}).(*Server).channelGateway()
	})
	if err != nil {
		panic(err)
	}
	return operations
}()

var channelOAuthOperations = func() []app.Operation {
	operations, err := appchannels.OAuthOperations(func(ctx context.Context) *appchannels.OAuth {
		return ctx.Value(systemHostKey{}).(*Server).channelOAuth()
	})
	if err != nil {
		panic(err)
	}
	return operations
}()

var channelAccountOperations = func() []app.Operation {
	operations, err := appchannels.AccountOperations(func(ctx context.Context) *appchannels.Accounts {
		return ctx.Value(systemHostKey{}).(*Server).channelAccounts()
	})
	if err != nil {
		panic(err)
	}
	return operations
}()

var channelInventoryOperations = func() []app.Operation {
	operations, err := appchannels.InventoryOperations(func(ctx context.Context) *appchannels.Inventory {
		return ctx.Value(systemHostKey{}).(*Server).channelInventory()
	})
	if err != nil {
		panic(err)
	}
	return operations
}()

var memoryOperations = func() []app.Operation {
	operations, err := appmemory.Operations(
		func(ctx context.Context) *appmemory.Service {
			return appmemory.New(requestMemoryPorts(ctx).Manager())
		},
		func(ctx context.Context) *appmemory.Distillation {
			return appmemory.NewDistillation(requestMemoryPorts(ctx))
		},
		func(ctx context.Context) *appmemory.LogService {
			return appmemory.NewLog(requestJournalPorts(ctx).Events)
		},
	)
	if err != nil {
		panic(err)
	}
	return operations
}()

var worldOperations = func() []app.Operation {
	operations, err := appworld.Operations(
		func(ctx context.Context) *appworld.Service {
			return appworld.New(requestWorldPorts(ctx).Graph())
		},
		func(ctx context.Context) *appworld.LogService {
			return appworld.NewLog(requestJournalPorts(ctx).Events)
		},
	)
	if err != nil {
		panic(err)
	}
	return operations
}()

var tasteOperations = func() []app.Operation {
	operations, err := apptaste.Operations(func(ctx context.Context) *apptaste.Service {
		return apptaste.New(requestTastePorts(ctx).Store())
	})
	if err != nil {
		panic(err)
	}
	return operations
}()

var skillOperations = func() []app.Operation {
	operations, err := appskill.Operations(
		func(ctx context.Context) *appskill.Service {
			return appskill.New(requestSkillPorts(ctx).Reader())
		},
		func(ctx context.Context) *appskill.Lifecycle {
			return appskill.NewLifecycle(requestSkillPorts(ctx).Lifecycle())
		},
		func(ctx context.Context) *appskill.Curation {
			host := ctx.Value(appHostKey{}).(appHost)
			return appskill.NewCuration(requestSkillPorts(ctx).Curation(), func(slug string) bool {
				_, ok := approster.ProfilePortsFromKernel(host.kernel).Get(slug)
				return ok
			})
		},
		func(ctx context.Context) *appskill.Observations {
			return appskill.NewObservations(requestSkillPorts(ctx).Observation(), requestJournalPorts(ctx).Events)
		},
	)
	if err != nil {
		panic(err)
	}
	return operations
}()

var boardOperations = func() []app.Operation {
	operations, err := appboard.Operations(
		func(ctx context.Context) (*appboard.Service, error) {
			server := ctx.Value(systemHostKey{}).(*Server)
			store, err := server.boardReader()
			if err != nil {
				return nil, err
			}
			return appboard.New(store, nil), nil
		},
		func(ctx context.Context) (*appboard.Service, error) {
			server := ctx.Value(systemHostKey{}).(*Server)
			store, ok := server.boardWriter()
			if !ok {
				return nil, errors.New("the board is not available on this daemon")
			}
			return appboard.New(store, server.boardNotify), nil
		},
	)
	if err != nil {
		panic(err)
	}
	return operations
}()

var workboardOperations = func() []app.Operation {
	ops, err := appworkboard.Operations(func(ctx context.Context) appworkboard.NativeServices {
		server := ctx.Value(systemHostKey{}).(*Server)
		ports := requestWorkboardPorts(ctx)
		return appworkboard.NativeServices{Reads: appworkboard.New(ports.Reader), Lifecycle: appworkboard.NewLifecycle(ports.Lifecycle, ports.SeatSetter), Relations: appworkboard.NewRelations(ports.Relations), Watch: appworkboard.NewWatch(ports.Store, ports.Events), Dispatch: server.workboardDispatcher(), ValidSeat: seatValidator(ports.SeatStore)}
	})
	if err != nil {
		panic(err)
	}
	return ops
}()

var okrOperations = func() []app.Operation {
	ops, err := appokr.Operations(func(ctx context.Context) *appokr.Service {
		ports := requestOKRPorts(ctx)
		return appokr.New(ports.Reader, ports.Progress)
	}, func(ctx context.Context) *appokr.Lifecycle {
		ports := requestOKRPorts(ctx)
		return appokr.NewLifecycle(ports.Mutation, appokr.New(ports.Reader, ports.Progress))
	})
	if err != nil {
		panic(err)
	}
	return ops
}()

var storageOperations = func() []app.Operation {
	ops, err := appstorage.Operations(func(ctx context.Context) *appstorage.Service {
		server := ctx.Value(systemHostKey{}).(*Server)
		return appstorage.New(requestBaseDirPorts(ctx).Dir(), storageFilesystem{}, server.diskFree)
	})
	if err != nil {
		panic(err)
	}
	return ops
}()
var artifactOperations = func() []app.Operation {
	ops, err := appartifacts.Operations(func(ctx context.Context) *appartifacts.Service {
		return ctx.Value(systemHostKey{}).(*Server).artifactService()
	})
	if err != nil {
		panic(err)
	}
	return ops
}()

var scheduleOperations = func() []app.Operation {
	server := func(ctx context.Context) *Server { return ctx.Value(systemHostKey{}).(*Server) }
	operations, err := appschedule.Operations(appschedule.Providers{
		Reads:     func(ctx context.Context) *appschedule.Service { return server(ctx).scheduleReads() },
		Lifecycle: func(ctx context.Context) *appschedule.Lifecycle { return server(ctx).scheduleLifecycle(ctx) },
		Admission: func(ctx context.Context) *appschedule.Admission { return server(ctx).scheduleAdmission() },
		Creation: func(ctx context.Context) *appschedule.Creation {
			return appschedule.NewCreation(appschedule.PortsFromKernel(ctx.Value(appHostKey{}).(appHost).kernel).Store, nil)
		},
		Editing: func(ctx context.Context) *appschedule.Editing { return server(ctx).scheduleEditing() },
		Firings: func(ctx context.Context) *appschedule.FiringService {
			return server(ctx).scheduleFiringReads(ctx.Value(appHostKey{}).(appHost).kernel)
		},
	})
	if err != nil {
		panic(err)
	}
	return operations
}()

var standingOperations = func() []app.Operation {
	server := func(ctx context.Context) *Server { return ctx.Value(systemHostKey{}).(*Server) }
	operations, err := appstanding.Operations(appstanding.Providers{
		Service: func(ctx context.Context) *appstanding.Service { return server(ctx).standingService() },
		Observations: func(ctx context.Context) *appstanding.Observations {
			return appstanding.NewObservations(requestJournalPorts(ctx).Events)
		},
		Firing: func(ctx context.Context) *appstanding.Firing {
			s := server(ctx)
			return appstanding.NewFiring(s.standingService(), s.standingFire)
		},
	})
	if err != nil {
		panic(err)
	}
	return operations
}()

var workflowOperations = func() []app.Operation {
	server := func(ctx context.Context) *Server { return ctx.Value(systemHostKey{}).(*Server) }
	operations, err := appworkflow.Operations(appworkflow.Providers{Reads: func(ctx context.Context) *appworkflow.Reads { return server(ctx).workflowReads() }, Lifecycle: func(ctx context.Context) *appworkflow.Lifecycle { return server(ctx).workflowLifecycle() }, Copilot: func(ctx context.Context) *appworkflow.Copilot { return server(ctx).workflowCopilot() }, Execution: func(ctx context.Context) *appworkflow.Execution { return server(ctx).workflowExecution() }})
	if err != nil {
		panic(err)
	}
	return operations
}()

var pulseControlOperations = func() []app.Operation {
	operations, err := apppulse.ControlOperations(func(ctx context.Context) *apppulse.Controls {
		return ctx.Value(systemHostKey{}).(*Server).pulseControls()
	})
	if err != nil {
		panic(err)
	}
	return operations
}()

var pulseSubscribeOperations = func() []app.Operation {
	operations, err := apppulse.SubscribeOperations(func(ctx context.Context) apppulse.SubscribeHost {
		server := ctx.Value(systemHostKey{}).(*Server)
		conn, _ := ctx.Value(pulseNativeConnKey{}).(net.Conn)
		host := apppulse.SubscribeHost{Stream: server.pulseStream()}
		if conn != nil {
			host.Prepare = func() { _ = conn.SetReadDeadline(time.Time{}) }
			host.ClientGone = func() <-chan struct{} { return pulseClientGone(ctx, conn) }
		}
		return host
	})
	if err != nil {
		panic(err)
	}
	return operations
}()

var autonomyOperations = func() []app.Operation {
	operations, err := appautonomy.Operations(func(ctx context.Context) *appautonomy.Feed {
		return appautonomy.NewFeed(requestJournalPorts(ctx).Events)
	})
	if err != nil {
		panic(err)
	}
	return operations
}()

var toolInventoryOperations = func() []app.Operation {
	operations, err := apptools.InventoryOperations(func(ctx context.Context) *apptools.Inventory {
		return apptools.NewInventory(requestToolsPorts(ctx).Registry)
	})
	if err != nil {
		panic(err)
	}
	return operations
}()

var toolObservationOperations = func() []app.Operation {
	operations, err := apptools.ObservationOperations(func(ctx context.Context) *apptools.Observations {
		return apptools.NewObservations(requestJournalPorts(ctx).Events)
	}, nil)
	if err != nil {
		panic(err)
	}
	return operations
}()

var forgeReadOperations = func() []app.Operation {
	operations, err := apptools.ForgeReadOperations(func(ctx context.Context) *apptools.ForgeCatalog {
		return apptools.NewForgeCatalog(requestToolsPorts(ctx).ScriptForge)
	})
	if err != nil {
		panic(err)
	}
	return operations
}()

var forgeLifecycleOperations = func() []app.Operation {
	operations, err := apptools.ForgeLifecycleOperations(func(ctx context.Context) *apptools.ForgeLifecycle {
		return apptools.NewForgeLifecycle(requestToolsPorts(ctx).ForgeWriter)
	})
	if err != nil {
		panic(err)
	}
	return operations
}()

var toolboxReadOperations = func() []app.Operation {
	operations, err := apptools.ToolboxReadOperations(func(ctx context.Context) *apptools.ToolboxReads {
		return apptools.NewToolboxReads(ctx.Value(systemHostKey{}).(*Server).toolboxHost)
	})
	if err != nil {
		panic(err)
	}
	return operations
}()

var toolboxInstallOperations = func() []app.Operation {
	operations, err := apptools.ToolboxInstallOperations(func(ctx context.Context) *apptools.ToolboxInstall {
		return apptools.NewToolboxInstall(ctx.Value(systemHostKey{}).(*Server).toolboxHost, func(kind event.Kind, payload map[string]any) error {
			_, err := requestBusPorts(ctx).Publish(event.Spec{Subject: "toolbox", Kind: kind, Actor: "toolbox", CorrelationID: opapi.CorrelationFromContext(ctx), Payload: payload})
			return err
		})
	})
	if err != nil {
		panic(err)
	}
	return operations
}()

var mcpCatalogOperations = func() []app.Operation {
	operations, err := apptools.MCPCatalogOperations(func(ctx context.Context) *apptools.MCPCatalog {
		ports := requestToolsPorts(ctx)
		return apptools.NewMCPCatalog(ports.MCP, ports.Attachment)
	})
	if err != nil {
		panic(err)
	}
	return operations
}()

var mcpLifecycleOperations = func() []app.Operation {
	operations, err := apptools.MCPLifecycleOperations(func(ctx context.Context) *apptools.MCPLifecycle {
		ports := requestToolsPorts(ctx)
		return apptools.NewMCPLifecycle(ports.MCPLifecycle, ports.Attachment)
	})
	if err != nil {
		panic(err)
	}
	return operations
}()

var marketReadOperations = func() []app.Operation {
	operations, err := appmarket.ReadOperations(func(ctx context.Context) *appmarket.Reads {
		return appmarket.NewReads(requestMarketPorts(ctx).Manager())
	})
	if err != nil {
		panic(err)
	}
	return operations
}()

var marketWriteOperations = func() []app.Operation {
	operations, err := appmarket.WriteOperations(func(ctx context.Context) *appmarket.Writes {
		publish := func(kind event.Kind, payload map[string]any) error {
			_, err := requestBusPorts(ctx).Publish(event.Spec{Subject: "market", Kind: kind, Actor: "market", CorrelationID: opapi.CorrelationFromContext(ctx), Payload: payload})
			return err
		}
		return appmarket.NewWrites(requestMarketPorts(ctx).Manager(), publish)
	})
	if err != nil {
		panic(err)
	}
	return operations
}()

var pluginInventoryOperations = func() []app.Operation {
	operations, err := appplugins.Operations(func(ctx context.Context) *appplugins.Service {
		return appplugins.New(requestPluginPorts(ctx))
	})
	if err != nil {
		panic(err)
	}
	return operations
}()

var configReadOperations = func() []app.Operation {
	operations, err := appconfig.Operations(func(ctx context.Context) *appconfig.Service {
		return appconfig.New(nativeConfigReader{persona: requestPersonaPorts(ctx), tools: requestToolsPorts(ctx), edict: requestEdictPorts(ctx), provider: requestProviderPorts(ctx), basedir: requestBaseDirPorts(ctx), plugins: requestPluginPorts(ctx)}, configEnvVars, nativeConfigEnvPresent)
	})
	if err != nil {
		panic(err)
	}
	return operations
}()

var settingsOperations = func() []app.Operation {
	operations, err := appsettings.Operations(func(ctx context.Context) *appsettings.Reads {
		return ctx.Value(systemHostKey{}).(*Server).settingsReads()
	}, func(ctx context.Context) *appsettings.Writes {
		return ctx.Value(systemHostKey{}).(*Server).settingsWrites()
	})
	if err != nil {
		panic(err)
	}
	return operations
}()

var configCenterOperations = func() []app.Operation {
	operations, err := appconfigcenter.Operations(func(ctx context.Context) *appconfigcenter.Reads {
		return ctx.Value(systemHostKey{}).(*Server).configCenterReads()
	}, func(ctx context.Context) *appconfigcenter.Writes {
		return ctx.Value(systemHostKey{}).(*Server).configCenterWrites()
	})
	if err != nil {
		panic(err)
	}
	return operations
}()

func registeredAppOperations() []app.Operation {
	operations := make([]app.Operation, 0, len(systemOperations)+len(lifecycleOperations)+len(rosterListOperations)+len(rosterGraveyardOperations)+len(rosterActivityOperations)+len(rosterRepairStatusOperations)+len(rosterEscalationOperations)+len(rosterSetEnabledOperations)+len(rosterProfileWriteOperations)+len(rosterPermissionOperations)+len(stateOperations)+len(redactionOperations)+len(reaperOperations)+len(missionControlOperations)+len(budgetOperations)+len(routingOperations)+len(executionProfileOperations)+len(dataOperations)+len(personaOperations)+len(seatOperations)+len(councilOperations)+len(councilAskOperations)+len(researchOperations)+len(reflectionOperations)+len(chatOperations)+len(chatSummaryOperations)+len(nodeOperations)+len(fileOperations)+len(sandboxOperations)+len(rosterTaskUpdateOperations)+len(rosterWakeOperations)+len(rosterRepairOperations)+len(rosterResolveOperations)+len(rosterImpactOperations)+len(rosterSetRetiredOperations)+len(rosterRemoveOperations)+len(steerOperations)+len(runsOperations)+len(planOperations)+len(planRunOperations)+len(planAuthoringOperations)+len(tenantOperations)+len(journalOperations)+len(traceOperations)+len(auditOperations)+len(approvalHistoryOperations)+len(approvalLiveOperations)+len(edictReadOperations)+len(edictWriteOperations)+len(edictDecisionOperations)+len(edictOverlayOperations)+len(updateOperations)+len(webhookOperations)+len(catalogOperations)+len(providerOperations)+len(oauthOperations)+len(observationOperations)+len(probeOperations)+len(acpInventoryOperations)+len(channelInventoryOperations)+len(channelAccountOperations)+len(channelOAuthOperations)+len(channelGatewayOperations)+len(channelInboxOperations)+len(channelSendOperations)+len(memoryOperations)+len(worldOperations)+len(tasteOperations)+len(skillOperations)+len(boardOperations)+len(workboardOperations)+len(okrOperations)+len(storageOperations)+len(artifactOperations)+len(scheduleOperations)+len(standingOperations)+len(workflowOperations)+len(pulseControlOperations)+len(pulseSubscribeOperations)+len(autonomyOperations)+len(toolInventoryOperations)+len(toolObservationOperations)+len(forgeReadOperations)+len(forgeLifecycleOperations)+len(toolboxReadOperations)+len(toolboxInstallOperations)+len(mcpCatalogOperations)+len(mcpLifecycleOperations)+len(marketReadOperations)+len(marketWriteOperations)+len(pluginInventoryOperations)+len(configReadOperations)+len(settingsOperations)+len(configCenterOperations))
	operations = append(operations, systemOperations...)
	operations = append(operations, lifecycleOperations...)
	operations = append(operations, catalogOperations...)
	operations = append(operations, providerOperations...)
	operations = append(operations, oauthOperations...)
	operations = append(operations, observationOperations...)
	operations = append(operations, probeOperations...)
	operations = append(operations, acpInventoryOperations...)
	operations = append(operations, channelInventoryOperations...)
	operations = append(operations, channelAccountOperations...)
	operations = append(operations, channelOAuthOperations...)
	operations = append(operations, channelGatewayOperations...)
	operations = append(operations, channelInboxOperations...)
	operations = append(operations, channelSendOperations...)
	operations = append(operations, memoryOperations...)
	operations = append(operations, worldOperations...)
	operations = append(operations, tasteOperations...)
	operations = append(operations, skillOperations...)
	operations = append(operations, boardOperations...)
	operations = append(operations, workboardOperations...)
	operations = append(operations, okrOperations...)
	operations = append(operations, storageOperations...)
	operations = append(operations, artifactOperations...)
	operations = append(operations, scheduleOperations...)
	operations = append(operations, standingOperations...)
	operations = append(operations, workflowOperations...)
	operations = append(operations, pulseControlOperations...)
	operations = append(operations, pulseSubscribeOperations...)
	operations = append(operations, autonomyOperations...)
	operations = append(operations, toolInventoryOperations...)
	operations = append(operations, toolObservationOperations...)
	operations = append(operations, forgeReadOperations...)
	operations = append(operations, forgeLifecycleOperations...)
	operations = append(operations, toolboxReadOperations...)
	operations = append(operations, toolboxInstallOperations...)
	operations = append(operations, mcpCatalogOperations...)
	operations = append(operations, mcpLifecycleOperations...)
	operations = append(operations, marketReadOperations...)
	operations = append(operations, marketWriteOperations...)
	operations = append(operations, pluginInventoryOperations...)
	operations = append(operations, configReadOperations...)
	operations = append(operations, settingsOperations...)
	operations = append(operations, webhookOperations...)
	operations = append(operations, updateOperations...)
	operations = append(operations, rosterListOperations...)
	operations = append(operations, rosterGraveyardOperations...)
	operations = append(operations, rosterActivityOperations...)
	operations = append(operations, rosterRepairStatusOperations...)
	operations = append(operations, rosterEscalationOperations...)
	operations = append(operations, rosterSetEnabledOperations...)
	operations = append(operations, rosterProfileWriteOperations...)
	operations = append(operations, rosterPermissionOperations...)
	operations = append(operations, stateOperations...)
	operations = append(operations, redactionOperations...)
	operations = append(operations, reaperOperations...)
	operations = append(operations, missionControlOperations...)
	operations = append(operations, budgetOperations...)
	operations = append(operations, routingOperations...)
	operations = append(operations, executionProfileOperations...)
	operations = append(operations, dataOperations...)
	operations = append(operations, personaOperations...)
	operations = append(operations, seatOperations...)
	operations = append(operations, councilOperations...)
	operations = append(operations, councilAskOperations...)
	operations = append(operations, researchOperations...)
	operations = append(operations, reflectionOperations...)
	operations = append(operations, chatOperations...)
	operations = append(operations, chatSummaryOperations...)
	operations = append(operations, nodeOperations...)
	operations = append(operations, fileOperations...)
	operations = append(operations, sandboxOperations...)
	operations = append(operations, rosterTaskUpdateOperations...)
	operations = append(operations, rosterWakeOperations...)
	operations = append(operations, rosterRepairOperations...)
	operations = append(operations, rosterResolveOperations...)
	operations = append(operations, rosterImpactOperations...)
	operations = append(operations, rosterSetRetiredOperations...)
	operations = append(operations, rosterRemoveOperations...)
	operations = append(operations, steerOperations...)
	operations = append(operations, runsOperations...)
	operations = append(operations, runOperations...)
	operations = append(operations, planOperations...)
	operations = append(operations, planRunOperations...)
	operations = append(operations, planAuthoringOperations...)
	operations = append(operations, tenantOperations...)
	operations = append(operations, journalOperations...)
	operations = append(operations, traceOperations...)
	operations = append(operations, auditOperations...)
	operations = append(operations, approvalHistoryOperations...)
	operations = append(operations, approvalLiveOperations...)
	operations = append(operations, edictReadOperations...)
	operations = append(operations, edictWriteOperations...)
	operations = append(operations, edictDecisionOperations...)
	operations = append(operations, edictOverlayOperations...)
	return append(operations, configCenterOperations...)
}

func registerAppSystemCommands() {
	for _, operation := range registeredAppOperations() {
		spec, err := appCommandSpec(operation)
		if err != nil {
			panic(err)
		}
		register(spec)
	}
}

// appCommandSpec binds the native protocol's object result and Event envelopes
// before any operation can mutate state through this adapter.
func appCommandSpec(operation app.Operation) (commandSpec, error) {
	spec := operation.Spec()
	var declaration struct {
		Type any `json:"type"`
	}
	if err := json.Unmarshal(spec.OutputSchema, &declaration); err != nil {
		return commandSpec{}, err
	}
	var types []any
	switch declared := declaration.Type.(type) {
	case string:
		types = []any{declared}
	case []any:
		types = declared
	}
	object := false
	for _, typ := range types {
		if typ == "object" {
			object = true
		} else if typ != "null" {
			return commandSpec{}, errors.New("control-plane terminal output requires an object schema")
		}
	}
	if !object {
		return commandSpec{}, errors.New("control-plane terminal output requires an object schema")
	}
	if spec.Stream != opapi.StreamNone && spec.Emission != reflect.TypeFor[event.Event]() && spec.Emission != reflect.TypeFor[*event.Event]() {
		return commandSpec{}, errors.New("control-plane streams require kernel event frames")
	}
	wire := commandSpec{Cmd: spec.Name, ReadOnly: spec.ReadOnly, AppOwned: true,
		TenantAllowed: spec.Authz == opapi.OwnTenant, TenantRouted: spec.Tenancy == opapi.CallerTenant,
		Streaming: StreamMode(spec.Stream), Handler: handleAppOperation}
	// The historical pulse wire is event-only and owns its lazy disconnect reader.
	// Keep canonical StreamLive metadata; suppress the generic native reader/result.
	if spec.Name == CmdPulseSubscribe {
		if spec.Stream != opapi.StreamLive || !spec.ReadOnly || spec.Authz != opapi.PrimaryOnly || spec.Tenancy != opapi.Primary {
			return commandSpec{}, errors.New("pulse native stream requires primary read-only StreamLive metadata")
		}
		wire.Streaming = StreamNone
		wire.Handler = handlePulseAppStream
	}
	return wire, nil
}

func dispatchAppOperation(dc *DispatchCtx, ctx context.Context, emitter opapi.Emitter) (any, error) {
	dc.S.operationOnce.Do(func() {
		dc.S.operations, dc.S.operationErr = app.NewDispatcher(registeredAppOperations(), app.Dependencies{Auth: appAuthenticator{dc.S}, Router: appTenantRouter{dc.S}, Audit: appAuditor{}})
	})
	if dc.S.operationErr != nil {
		return nil, dc.S.operationErr
	}
	raw, err := json.Marshal(dc.Req.Args)
	if err != nil {
		return nil, err
	}
	if dc.Req.Args == nil {
		raw = json.RawMessage(`{}`)
	}
	// Routing trims the tenant, so the caller names the trimmed tenant too: an
	// operator-selected tenant write is audited under the kernel it reached.
	return dc.S.operations.Dispatch(ctx, opapi.Caller{Credential: dc.Req.Token, Tenant: strings.TrimSpace(tenantOf(dc.Req)), Source: "controlplane"}, dc.Req.Cmd, raw, emitter)
}

func handleAppOperation(dc *DispatchCtx) {
	terminal := &nativeTerminalCleanup{}
	defer terminal.release()
	write := &nativeTerminalWrite{}
	defer write.discard()
	ctx := opapi.WithTerminalWrite(opapi.WithTerminalCleanup(dc.Ctx, terminal), write)
	ctx = app.WithRequestID(ctx, dc.Req.ID)
	ctx = context.WithValue(ctx, appConnKey{}, dc.Conn)
	output, err := dispatchAppOperation(dc, ctx, appEmitter{dc.Conn, dc.Req.ID})
	if err != nil {
		// A domain error that classifies itself (app/files) keeps its code in
		// error_code beside the message.
		resp := Response{ID: dc.Req.ID, Type: RespError, Error: err.Error()}
		var coded interface{ ErrorCode() string }
		if errors.As(err, &coded) {
			resp.ErrorCode = coded.ErrorCode()
		}
		dc.S.writeResp(dc.Conn, resp)
		write.finishWrite()
		return
	}
	encoded, err := json.Marshal(output)
	if err != nil {
		dc.S.fail(dc.Conn, dc.Req, err)
		write.finishWrite()
		return
	}
	var result map[string]any
	// Preserve integer/decimal lexemes through the native object envelope. Decoding
	// typed output through float64 silently rounds identities above2^53.
	decoder := json.NewDecoder(bytes.NewReader(encoded))
	decoder.UseNumber()
	if err := decoder.Decode(&result); err != nil {
		dc.S.fail(dc.Conn, dc.Req, err)
		write.finishWrite()
		return
	}

	// Legacy config_schema writes core Section/Field structs directly. Preserve
	// their declared JSON member order inside the otherwise generic object codec.
	if snapshot, ok := output.(appsettings.SchemaOutput); ok {
		result["sections"] = snapshot.Sections
	}
	// Legacy channel_list embeds MediaCaps structs in map rows. Keep their
	// declared member order while inventory DTOs use the shared object envelope.
	// Legacy agent_task_update embeds the roster AgentTask struct directly.
	if out, ok := output.(approster.TaskUpdateOutput); ok {
		result["task"] = out.Task
	}
	// Legacy journal_tail/grep/export embed the journal's Event structs directly.
	if out, ok := output.(appjournal.EventsOutput); ok {
		result["events"] = out.Events
	}
	if out, ok := output.(appjournal.ExportOutput); ok {
		result["events"] = out.Events
	}
	// Legacy why embeds the correlation and causation chains' Event structs.
	if out, ok := output.(appjournal.WhyOutput); ok {
		result["events"], result["causation_chain"] = out.Events, out.CausationChain
	}
	if inventory, ok := output.(appchannels.ListOutput); ok {
		rows, _ := result["channels"].([]any)
		for i, value := range rows {
			row, _ := value.(map[string]any)
			row["media"] = inventory.Channels[i].Media
		}
	}

	// Preserve the legacy nested thread/message struct member order and int64s.
	if inbox, ok := output.(appchannels.InboxOutput); ok {
		result["threads"] = inbox.Threads
	}
	// Legacy file_restore embeds the rollback Checkpoint struct directly.
	if out, ok := output.(appfiles.RestoreOutput); ok {
		result["checkpoint"] = out.Checkpoint
	}
	// Legacy chat_suggestions embeds the Suggestion structs directly.
	if out, ok := output.(appchat.SuggestionsOutput); ok {
		result["suggestions"] = out.Suggestions
	}
	dc.S.writeResp(dc.Conn, Response{ID: dc.Req.ID, Type: RespResult, Result: result})
	write.finishWrite()
}

type appAuthenticator struct{ server *Server }

func (a appAuthenticator) Authenticate(_ context.Context, caller opapi.Caller) (opapi.Principal, error) {
	if a.server.tokenIsPrimary(caller.Credential) {
		return opapi.Principal{Kind: opapi.Operator}, nil
	}
	tenant := strings.TrimSpace(caller.Tenant)
	if tenant == "" || a.server.tenants == nil || !a.server.tenants.Authorize(tenant, caller.Credential) {
		return opapi.Principal{}, errors.New("unauthorized")
	}
	return opapi.Principal{Kind: opapi.Tenant, Tenant: tenant}, nil
}

type appTenantRouter struct{ server *Server }

func (r appTenantRouter) Route(ctx context.Context, principal opapi.Principal, spec opapi.Spec) (context.Context, error) {
	k := r.server.k
	if spec.Tenancy == opapi.CallerTenant {
		var err error
		k, err = r.server.kernelFor(principal.Tenant)
		if err != nil {
			return nil, err
		}
	}
	host := appHost{kernel: k}
	if !spec.ReadOnly {
		correlation := appcorrelation.PortsFromKernel(k)
		host.correlation = correlation.Mint()
		ctx = correlation.WithActor(ctx, string(principal.Kind), host.correlation)
		ctx = opapi.WithCorrelation(ctx, host.correlation)
	}
	ctx = context.WithValue(ctx, appHostKey{}, host)
	return context.WithValue(ctx, systemHostKey{}, r.server), nil
}
