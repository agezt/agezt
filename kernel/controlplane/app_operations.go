// SPDX-License-Identifier: MIT

package controlplane

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"github.com/agezt/agezt/kernel/board"
	"net"
	"path/filepath"
	"reflect"
	"strings"
	"time"

	"github.com/agezt/agezt/kernel/app"
	appapprovals "github.com/agezt/agezt/kernel/app/approvals"
	appartifacts "github.com/agezt/agezt/kernel/app/artifacts"
	appaudit "github.com/agezt/agezt/kernel/app/audit"
	appautonomy "github.com/agezt/agezt/kernel/app/autonomy"
	appboard "github.com/agezt/agezt/kernel/app/board"
	appcatalog "github.com/agezt/agezt/kernel/app/catalog"
	appchannels "github.com/agezt/agezt/kernel/app/channels"
	appconfig "github.com/agezt/agezt/kernel/app/config"
	appconfigcenter "github.com/agezt/agezt/kernel/app/configcenter"
	appedict "github.com/agezt/agezt/kernel/app/edict"
	appjournal "github.com/agezt/agezt/kernel/app/journal"
	appmarket "github.com/agezt/agezt/kernel/app/market"
	appmemory "github.com/agezt/agezt/kernel/app/memory"
	appmissioncontrol "github.com/agezt/agezt/kernel/app/missioncontrol"
	appokr "github.com/agezt/agezt/kernel/app/okr"
	appplugins "github.com/agezt/agezt/kernel/app/plugins"
	appproviders "github.com/agezt/agezt/kernel/app/providers"
	apppulse "github.com/agezt/agezt/kernel/app/pulse"
	appreaper "github.com/agezt/agezt/kernel/app/reaper"
	appredaction "github.com/agezt/agezt/kernel/app/redaction"
	approster "github.com/agezt/agezt/kernel/app/roster"
	appruns "github.com/agezt/agezt/kernel/app/runs"
	appsandbox "github.com/agezt/agezt/kernel/app/sandbox"
	appschedule "github.com/agezt/agezt/kernel/app/schedule"
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
	"github.com/agezt/agezt/kernel/contract/opapi"
	"github.com/agezt/agezt/kernel/edict"
	"github.com/agezt/agezt/kernel/event"
	"github.com/agezt/agezt/kernel/governor"
	"github.com/agezt/agezt/kernel/redact"
	"github.com/agezt/agezt/kernel/roster"
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
		return approster.NewProfileWrite(s.k.Roster().Get, s.k.AddProfile, s.k.UpdateProfile, s.invalidateAgentListCache)
	})
	if err != nil {
		panic(err)
	}
	return ops
}()

var sandboxOperations = func() []app.Operation {
	ops, err := appsandbox.Operations(func(ctx context.Context) *appsandbox.Service {
		return appsandbox.New(filepath.Join(ctx.Value(systemHostKey{}).(*Server).k.BaseDir(), "sandbox", "projects"))
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
			Spend: func() (int64, bool) {
				gov, ok := s.k.Provider().(*governor.Governor)
				if !ok {
					return 0, false
				}
				return gov.Snapshot().SpentMicrocents, true
			},
			Pending: func() []approval.Request {
				if registry := s.k.Approvals(); registry != nil {
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
		k := ctx.Value(systemHostKey{}).(*Server).k
		return appredaction.New(func() appredaction.Redactor { return k.Bus().Redactor() }, redact.MatchedCategories)
	})
	if err != nil {
		panic(err)
	}
	return ops
}()

var stateOperations = func() []app.Operation {
	ops, err := appstate.Operations(func(ctx context.Context) *appstate.Service {
		return appstate.New(ctx.Value(systemHostKey{}).(*Server).k.State())
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
			Get:    s.k.Roster().Get,
			Update: s.k.UpdateProfile,
			Tools:  s.k.Tools,
			Decide: func(capability edict.Capability, ceiling edict.TrustLevel) edict.Outcome {
				return s.k.Edict().DecideWithCeiling(capability, "", ceiling)
			},
			Config: func() ([]*configcenter.ConfigEntry, bool) {
				center := s.k.ConfigCenter()
				if center == nil {
					return nil, false
				}
				return center.ListEntries(), true
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
		return appedict.New(ctx.Value(appHostKey{}).(appHost).kernel.Edict())
	})
	if err != nil {
		panic(err)
	}
	return ops
}()

var edictDecisionOperations = func() []app.Operation {
	ops, err := appedict.DecisionOperations(func(ctx context.Context) *appedict.Decisions {
		return appedict.NewDecisions(ctx.Value(appHostKey{}).(appHost).kernel.Journal(), time.Now)
	})
	if err != nil {
		panic(err)
	}
	return ops
}()

var edictOverlayOperations = func() []app.Operation {
	ops, err := appedict.OverlayOperations(func(ctx context.Context) *appedict.Overlay {
		k := ctx.Value(appHostKey{}).(appHost).kernel
		save := func(snap *edict.OverlaySnapshot) error {
			return edict.SaveOverlaySnapshot(filepath.Join(k.BaseDir(), "runtime", edict.OverlaySnapshotFile), snap)
		}
		return appedict.NewOverlay(k.Journal(), save, func(spec event.Spec) { _, _ = k.Bus().Publish(spec) })
	})
	if err != nil {
		panic(err)
	}
	return ops
}()

var edictWriteOperations = func() []app.Operation {
	ops, err := appedict.WriteOperations(func(ctx context.Context) *appedict.Writes {
		k := ctx.Value(appHostKey{}).(appHost).kernel
		return appedict.NewWrites(k.Edict(), func(spec event.Spec) { _, _ = k.Bus().Publish(spec) })
	})
	if err != nil {
		panic(err)
	}
	return ops
}()

var approvalLiveOperations = func() []app.Operation {
	ops, err := appapprovals.LiveOperations(func(ctx context.Context) *appapprovals.Live {
		return appapprovals.NewLive(ctx.Value(appHostKey{}).(appHost).kernel.Approvals())
	})
	if err != nil {
		panic(err)
	}
	return ops
}()

var approvalHistoryOperations = func() []app.Operation {
	ops, err := appapprovals.Operations(func(ctx context.Context) *appapprovals.History {
		return appapprovals.New(ctx.Value(appHostKey{}).(appHost).kernel.Journal(), time.Now)
	})
	if err != nil {
		panic(err)
	}
	return ops
}()

var auditOperations = func() []app.Operation {
	ops, err := appaudit.Operations(func(ctx context.Context) *appaudit.Service {
		return appaudit.New(ctx.Value(appHostKey{}).(appHost).kernel.Journal(), time.Now)
	})
	if err != nil {
		panic(err)
	}
	return ops
}()

var traceOperations = func() []app.Operation {
	ops, err := appjournal.TraceOperations(func(ctx context.Context) *appjournal.Trace {
		return appjournal.NewTrace(ctx.Value(appHostKey{}).(appHost).kernel)
	})
	if err != nil {
		panic(err)
	}
	return ops
}()

var journalOperations = func() []app.Operation {
	ops, err := appjournal.Operations(func(ctx context.Context) *appjournal.Service {
		return journalReads(ctx.Value(appHostKey{}).(appHost).kernel)
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

var planOperations = func() []app.Operation {
	ops, err := appruns.PlanOperations(func(ctx context.Context) *appruns.Plans {
		return appruns.NewPlans(ctx.Value(appHostKey{}).(appHost).kernel.Journal())
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
			Get:       s.k.Roster().Get,
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
			Remove:    s.k.RemoveProfile,
			NewCorr:   s.k.NewCorrelation,
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
			Get:    s.k.Roster().Get,
			Impact: s.impactService().Summary,
			SetRetired: func(ref string, retired bool, reason string) (roster.Profile, error) {
				return s.k.SetProfileRetired(ref, retired, reason)
			},
			PauseStanding:        s.pauseAgentStanding,
			PauseSchedules:       s.pauseAgentSchedules,
			CountPausedStanding:  s.countAgentPausedStanding,
			CountPausedSchedules: s.countAgentPausedSchedules,
			NewCorrelation:       s.k.NewCorrelation,
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
			Get:            s.k.Roster().Get,
			NewCorrelation: s.k.NewCorrelation,
			Publish: func(subject, corr string, payload map[string]any) {
				publishOperatorAction(s.k, subject, corr, payload)
			},
			Pause: func(slug string) error {
				_, err := s.k.SetProfileEnabled(slug, false)
				return err
			},
			Retire: func(slug, reason string) error {
				_, err := s.k.SetProfileRetired(slug, true, reason)
				return err
			},
			HelpRequest: s.postOperatorHelp,
			ExhaustedChain: func(slug string, l approster.IncidentLineage, taskType string) []string {
				return latestExhaustedRoutingChain(s.k, slug, operatorWakeLineage{incidentID: l.IncidentID, rootIncidentID: l.RootIncidentID, parentIncidentID: l.ParentIncidentID}, taskType)
			},
			ForceGeneration: func(slug, taskType string) int { return latestOperatorForceGeneration(s.k, slug, taskType) },
			ApplyChain:      s.applyRoutingChain,
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
		return approster.NewRepair(s.k.Roster().Get, s.k.NewCorrelation, func(subject, corr string, payload map[string]any) {
			publishOperatorAction(s.k, subject, corr, payload)
		}, func(corr string, p roster.Profile, reason string, l approster.IncidentLineage) {
			go s.runAgentRepair(corr, p, reason, operatorWakeLineage{incidentID: l.IncidentID, rootIncidentID: l.RootIncidentID, parentIncidentID: l.ParentIncidentID})
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
		return approster.NewWake(s.k.Roster().Get, s.k.NewCorrelation, func(subject, corr string, payload map[string]any) {
			publishOperatorAction(s.k, subject, corr, payload)
		}, func(corr string, p roster.Profile, intent, reason string, l approster.IncidentLineage) {
			go s.runAgentWake(corr, p, intent, reason, operatorWakeLineage{incidentID: l.IncidentID, rootIncidentID: l.RootIncidentID, parentIncidentID: l.ParentIncidentID})
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
		return approster.NewTaskUpdate(s.k.UpdateProfile)
	})
	if err != nil {
		panic(err)
	}
	return ops
}()

var rosterSetEnabledOperations = func() []app.Operation {
	ops, err := approster.SetEnabledOperations(func(ctx context.Context) *approster.SetEnabledService {
		s := ctx.Value(systemHostKey{}).(*Server)
		return approster.NewSetEnabled(s.k.SetProfileEnabled, s.countAgentPausedStanding, s.countAgentPausedSchedules, s.invalidateAgentListCache)
	})
	if err != nil {
		panic(err)
	}
	return ops
}()

// nativeEscalationBoard reads the help topic and replies under the native
// board read cap for the application escalation fold.
type nativeEscalationBoard struct{ st *board.Store }

func (b nativeEscalationBoard) Help() []board.Message { return b.st.Read("help", boardReadMaxLimit) }
func (b nativeEscalationBoard) Replies(id string) []board.Message {
	return b.st.Replies(id, boardReadMaxLimit)
}

var rosterEscalationOperations = func() []app.Operation {
	ops, err := approster.EscalationOperations(func(ctx context.Context) *approster.EscalationService {
		s := ctx.Value(systemHostKey{}).(*Server)
		return approster.NewEscalations(s.k.Roster().Get, func() (approster.EscalationBoard, error) {
			st, err := s.boardReader()
			if err != nil {
				return nil, err
			}
			return nativeEscalationBoard{st}, nil
		}, s.k.Journal().Range)
	})
	if err != nil {
		panic(err)
	}
	return ops
}()

var rosterRepairStatusOperations = func() []app.Operation {
	ops, err := approster.RepairStatusOperations(func(ctx context.Context) *approster.RepairStatusService {
		s := ctx.Value(systemHostKey{}).(*Server)
		return approster.NewRepairStatus(s.k.Roster().Get, s.k.Journal().Range, agentAutoRepairCooldown, nil)
	})
	if err != nil {
		panic(err)
	}
	return ops
}()

var rosterActivityOperations = func() []app.Operation {
	ops, err := approster.ActivityOperations(func(ctx context.Context) *approster.ActivityService {
		s := ctx.Value(systemHostKey{}).(*Server)
		return approster.NewActivity(s.k.Roster().Get, s.k.Journal().Range)
	})
	if err != nil {
		panic(err)
	}
	return ops
}()

var rosterGraveyardOperations = func() []app.Operation {
	ops, err := approster.GraveyardOperations(func(ctx context.Context) *approster.GraveyardService {
		s := ctx.Value(systemHostKey{}).(*Server)
		return approster.NewGraveyard(func() []roster.Profile { return s.k.Roster().List() }, nil)
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
		return appwebhook.NewObservability(ctx.Value(appHostKey{}).(appHost).kernel.Journal(), nil)
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
		host := ctx.Value(appHostKey{}).(appHost)
		server := ctx.Value(systemHostKey{}).(*Server)
		return appcatalog.New(host.kernel, server.baseDir)
	})
	if err != nil {
		panic(err)
	}
	return ops
}()

var providerOperations = func() []app.Operation {
	ops, err := appproviders.Operations(func(ctx context.Context) *appproviders.Service {
		host := ctx.Value(appHostKey{}).(appHost)
		server := ctx.Value(systemHostKey{}).(*Server)
		return appproviders.New(host.kernel, server.baseDir)
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
		return appproviders.NewObservations(ctx.Value(appHostKey{}).(appHost).kernel.Journal())
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
			return appmemory.New(ctx.Value(appHostKey{}).(appHost).kernel.Memory())
		},
		func(ctx context.Context) *appmemory.Distillation {
			return appmemory.NewDistillation(ctx.Value(appHostKey{}).(appHost).kernel)
		},
		func(ctx context.Context) *appmemory.LogService {
			return appmemory.NewLog(ctx.Value(appHostKey{}).(appHost).kernel.Journal())
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
			return appworld.New(ctx.Value(appHostKey{}).(appHost).kernel.World())
		},
		func(ctx context.Context) *appworld.LogService {
			return appworld.NewLog(ctx.Value(appHostKey{}).(appHost).kernel.Journal())
		},
	)
	if err != nil {
		panic(err)
	}
	return operations
}()

var tasteOperations = func() []app.Operation {
	operations, err := apptaste.Operations(func(ctx context.Context) *apptaste.Service {
		return apptaste.New(ctx.Value(appHostKey{}).(appHost).kernel.Taste())
	})
	if err != nil {
		panic(err)
	}
	return operations
}()

var skillOperations = func() []app.Operation {
	operations, err := appskill.Operations(
		func(ctx context.Context) *appskill.Service {
			return appskill.New(ctx.Value(appHostKey{}).(appHost).kernel.Forge())
		},
		func(ctx context.Context) *appskill.Lifecycle {
			return appskill.NewLifecycle(ctx.Value(appHostKey{}).(appHost).kernel.Forge())
		},
		func(ctx context.Context) *appskill.Curation {
			host := ctx.Value(appHostKey{}).(appHost)
			return appskill.NewCuration(host.kernel.Forge(), func(slug string) bool { _, ok := host.kernel.Roster().Get(slug); return ok })
		},
		func(ctx context.Context) *appskill.Observations {
			host := ctx.Value(appHostKey{}).(appHost)
			return appskill.NewObservations(host.kernel.Forge(), host.kernel.Journal())
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
		host := ctx.Value(appHostKey{}).(appHost)
		k := host.kernel
		return appworkboard.NativeServices{Reads: appworkboard.New(k.Workboard()), Lifecycle: appworkboard.NewLifecycle(k, k.Workboard()), Relations: appworkboard.NewRelations(k), Watch: appworkboard.NewWatch(k.Workboard(), k.Journal()), Dispatch: server.workboardDispatcher(), ValidSeat: k.Seats().Valid}
	})
	if err != nil {
		panic(err)
	}
	return ops
}()

var okrOperations = func() []app.Operation {
	ops, err := appokr.Operations(func(ctx context.Context) *appokr.Service {
		k := ctx.Value(appHostKey{}).(appHost).kernel
		return appokr.New(k.OKR(), k)
	}, func(ctx context.Context) *appokr.Lifecycle {
		k := ctx.Value(appHostKey{}).(appHost).kernel
		return appokr.NewLifecycle(k, appokr.New(k.OKR(), k))
	})
	if err != nil {
		panic(err)
	}
	return ops
}()

var storageOperations = func() []app.Operation {
	ops, err := appstorage.Operations(func(ctx context.Context) *appstorage.Service {
		server := ctx.Value(systemHostKey{}).(*Server)
		k := ctx.Value(appHostKey{}).(appHost).kernel
		return appstorage.New(k.BaseDir(), storageFilesystem{}, server.diskFree)
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
			return appschedule.NewCreation(ctx.Value(appHostKey{}).(appHost).kernel.Schedules(), nil)
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
			return appstanding.NewObservations(ctx.Value(appHostKey{}).(appHost).kernel.Journal())
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
		return appautonomy.NewFeed(ctx.Value(appHostKey{}).(appHost).kernel.Journal())
	})
	if err != nil {
		panic(err)
	}
	return operations
}()

var toolInventoryOperations = func() []app.Operation {
	operations, err := apptools.InventoryOperations(func(ctx context.Context) *apptools.Inventory {
		return apptools.NewInventory(ctx.Value(appHostKey{}).(appHost).kernel)
	})
	if err != nil {
		panic(err)
	}
	return operations
}()

var toolObservationOperations = func() []app.Operation {
	operations, err := apptools.ObservationOperations(func(ctx context.Context) *apptools.Observations {
		return apptools.NewObservations(ctx.Value(appHostKey{}).(appHost).kernel.Journal())
	}, nil)
	if err != nil {
		panic(err)
	}
	return operations
}()

var forgeReadOperations = func() []app.Operation {
	operations, err := apptools.ForgeReadOperations(func(ctx context.Context) *apptools.ForgeCatalog {
		return apptools.NewForgeCatalog(ctx.Value(appHostKey{}).(appHost).kernel.ToolForge())
	})
	if err != nil {
		panic(err)
	}
	return operations
}()

var forgeLifecycleOperations = func() []app.Operation {
	operations, err := apptools.ForgeLifecycleOperations(func(ctx context.Context) *apptools.ForgeLifecycle {
		return apptools.NewForgeLifecycle(ctx.Value(appHostKey{}).(appHost).kernel)
	})
	if err != nil {
		panic(err)
	}
	return operations
}()

var toolboxReadOperations = func() []app.Operation {
	operations, err := apptools.ToolboxReadOperations(func(context.Context) *apptools.ToolboxReads { return apptools.NewToolboxReads(nativeToolboxReader{}) })
	if err != nil {
		panic(err)
	}
	return operations
}()

var toolboxInstallOperations = func() []app.Operation {
	operations, err := apptools.ToolboxInstallOperations(func(ctx context.Context) *apptools.ToolboxInstall {
		host := ctx.Value(appHostKey{}).(appHost)
		return apptools.NewToolboxInstall(nativeToolboxInstaller{}, func(kind event.Kind, payload map[string]any) error {
			_, err := host.kernel.Bus().Publish(event.Spec{Subject: "toolbox", Kind: kind, Actor: "toolbox", CorrelationID: opapi.CorrelationFromContext(ctx), Payload: payload})
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
		host := ctx.Value(appHostKey{}).(appHost)
		return apptools.NewMCPCatalog(host.kernel.MCPStore(), host.kernel)
	})
	if err != nil {
		panic(err)
	}
	return operations
}()

var mcpLifecycleOperations = func() []app.Operation {
	operations, err := apptools.MCPLifecycleOperations(func(ctx context.Context) *apptools.MCPLifecycle {
		host := ctx.Value(appHostKey{}).(appHost)
		return apptools.NewMCPLifecycle(host.kernel, host.kernel)
	})
	if err != nil {
		panic(err)
	}
	return operations
}()

var marketReadOperations = func() []app.Operation {
	operations, err := appmarket.ReadOperations(func(ctx context.Context) *appmarket.Reads {
		manager := ctx.Value(appHostKey{}).(appHost).kernel.Market()
		if manager == nil {
			return appmarket.NewReads(nil)
		}
		return appmarket.NewReads(manager)
	})
	if err != nil {
		panic(err)
	}
	return operations
}()

var marketWriteOperations = func() []app.Operation {
	operations, err := appmarket.WriteOperations(func(ctx context.Context) *appmarket.Writes {
		host := ctx.Value(appHostKey{}).(appHost)
		publish := func(kind event.Kind, payload map[string]any) error {
			_, err := host.kernel.Bus().Publish(event.Spec{Subject: "market", Kind: kind, Actor: "market", CorrelationID: opapi.CorrelationFromContext(ctx), Payload: payload})
			return err
		}
		manager := host.kernel.Market()
		if manager == nil {
			return appmarket.NewWrites(nil, publish)
		}
		return appmarket.NewWrites(manager, publish)
	})
	if err != nil {
		panic(err)
	}
	return operations
}()

var pluginInventoryOperations = func() []app.Operation {
	operations, err := appplugins.Operations(func(ctx context.Context) *appplugins.Service {
		return appplugins.New(nativePluginReader{ctx.Value(appHostKey{}).(appHost).kernel})
	})
	if err != nil {
		panic(err)
	}
	return operations
}()

var configReadOperations = func() []app.Operation {
	operations, err := appconfig.Operations(func(ctx context.Context) *appconfig.Service {
		return appconfig.New(nativeConfigReader{ctx.Value(appHostKey{}).(appHost).kernel}, configEnvVars, nativeConfigEnvPresent)
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
	operations := make([]app.Operation, 0, len(systemOperations)+len(lifecycleOperations)+len(rosterListOperations)+len(rosterGraveyardOperations)+len(rosterActivityOperations)+len(rosterRepairStatusOperations)+len(rosterEscalationOperations)+len(rosterSetEnabledOperations)+len(rosterProfileWriteOperations)+len(rosterPermissionOperations)+len(stateOperations)+len(redactionOperations)+len(reaperOperations)+len(missionControlOperations)+len(sandboxOperations)+len(rosterTaskUpdateOperations)+len(rosterWakeOperations)+len(rosterRepairOperations)+len(rosterResolveOperations)+len(rosterImpactOperations)+len(rosterSetRetiredOperations)+len(rosterRemoveOperations)+len(steerOperations)+len(runsOperations)+len(planOperations)+len(tenantOperations)+len(journalOperations)+len(traceOperations)+len(auditOperations)+len(approvalHistoryOperations)+len(approvalLiveOperations)+len(edictReadOperations)+len(edictWriteOperations)+len(edictDecisionOperations)+len(edictOverlayOperations)+len(updateOperations)+len(webhookOperations)+len(catalogOperations)+len(providerOperations)+len(oauthOperations)+len(observationOperations)+len(probeOperations)+len(acpInventoryOperations)+len(channelInventoryOperations)+len(channelAccountOperations)+len(channelOAuthOperations)+len(channelGatewayOperations)+len(channelInboxOperations)+len(channelSendOperations)+len(memoryOperations)+len(worldOperations)+len(tasteOperations)+len(skillOperations)+len(boardOperations)+len(workboardOperations)+len(okrOperations)+len(storageOperations)+len(artifactOperations)+len(scheduleOperations)+len(standingOperations)+len(workflowOperations)+len(pulseControlOperations)+len(pulseSubscribeOperations)+len(autonomyOperations)+len(toolInventoryOperations)+len(toolObservationOperations)+len(forgeReadOperations)+len(forgeLifecycleOperations)+len(toolboxReadOperations)+len(toolboxInstallOperations)+len(mcpCatalogOperations)+len(mcpLifecycleOperations)+len(marketReadOperations)+len(marketWriteOperations)+len(pluginInventoryOperations)+len(configReadOperations)+len(settingsOperations)+len(configCenterOperations))
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
	operations = append(operations, planOperations...)
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
	output, err := dispatchAppOperation(dc, ctx, appEmitter{dc.Conn, dc.Req.ID})
	if err != nil {
		dc.S.fail(dc.Conn, dc.Req, err)
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
		host.correlation = k.NewCorrelation()
		ctx = k.WithActorCorrelation(ctx, string(principal.Kind), host.correlation)
		ctx = opapi.WithCorrelation(ctx, host.correlation)
	}
	ctx = context.WithValue(ctx, appHostKey{}, host)
	return context.WithValue(ctx, systemHostKey{}, r.server), nil
}
