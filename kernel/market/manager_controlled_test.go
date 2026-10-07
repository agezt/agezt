// SPDX-License-Identifier: MIT
package market

import (
	"context"
	"errors"
	"github.com/agezt/agezt/kernel/skill"
	"testing"
)

func TestControlledInstallSinkFailureBeforeAnyMaterialization(t *testing.T) {
	m, f, mc := newTestManager(t)
	owned := errors.New("owned sink failure")
	rec, err := m.InstallContext(context.Background(), "corr", "", "web-research-pack", "", func(Event) error { return owned })
	installed, _ := m.store.Installed()
	if err != owned || len(f.created) != 0 || len(f.promoted) != 0 || len(mc.added) != 0 || len(installed) != 0 || rec.Name != "" {
		t.Fatal(rec, err, f, mc, installed)
	}
}
func TestControlledInstallStopsAfterFirstAcceptedEffectWithoutRollback(t *testing.T) {
	m, f, mc := newTestManager(t)
	owned := errors.New("owned sink failure")
	rec, err := m.InstallContext(context.Background(), "corr", "", "web-research-pack", "", func(e Event) error {
		if e.Stage == "skill" {
			return owned
		}
		return nil
	})
	installed, _ := m.store.Installed()
	if err != owned || len(f.created) != 1 || len(f.promoted) != 1 || len(mc.added) != 0 || len(installed) != 0 || len(rec.SkillIDs) != 1 {
		t.Fatal(rec, err, f, mc, installed)
	}
}
func TestControlledUninstallStopsBeforeLaterReverseAndRetainsProvenance(t *testing.T) {
	m, f, mc := newTestManager(t)
	if _, err := m.Install("corr", "", "web-research-pack", "", nil); err != nil {
		t.Fatal(err)
	}
	owned := errors.New("owned reverse progress failure")
	err := m.UninstallContext(context.Background(), "corr", "web-research-pack", func(Event) error { return owned })
	installed, _ := m.store.Installed()
	if err != owned || len(f.quarantine) != 1 || len(mc.removed) != 0 || len(installed) != 1 {
		t.Fatal(err, f, mc, installed)
	}
}
func TestControlledCorePreCanceledAndCanceledProgressStopNextEffect(t *testing.T) {
	m, f, mc := newTestManager(t)
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	if _, err := m.InstallContext(ctx, "corr", "", "web-research-pack", "", nil); err != context.Canceled {
		t.Fatal(err)
	}
	if err := m.UninstallContext(ctx, "corr", "web-research-pack", nil); err != context.Canceled {
		t.Fatal(err)
	}
	if len(f.created) != 0 || len(mc.added) != 0 {
		t.Fatal(f, mc)
	}
	ctx, cancel = context.WithCancel(context.Background())
	defer cancel()
	_, err := m.InstallContext(ctx, "corr", "", "web-research-pack", "", func(Event) error { cancel(); return nil })
	if err != context.Canceled || len(f.created) != 0 || len(mc.added) != 0 {
		t.Fatal(err, f, mc)
	}
}
func TestLegacyVoidCallbacksRetainBehaviorAndBestEffortUninstall(t *testing.T) {
	m, f, mc := newTestManager(t)
	frames := []Event{}
	rec, err := m.Install("corr", "", "web-research-pack", "", func(e Event) { frames = append(frames, e) })
	if err != nil || len(frames) != 6 || frames[0].Stage != "vet" || frames[len(frames)-1].Stage != "done" || len(f.created) != 1 || len(mc.added) != 1 || rec.Name != "web-research-pack" {
		t.Fatal(rec, err, frames, f, mc)
	}
	if err := m.Uninstall("corr", "web-research-pack", nil); err != nil || len(f.quarantine) != 1 || len(mc.removed) != 1 {
		t.Fatal(err, f, mc)
	}
}

type controlledFailForge struct {
	*fakeForge
	failure error
}

func (f controlledFailForge) Create(string, skill.CreateSpec) (skill.Skill, bool, error) {
	return skill.Skill{}, false, f.failure
}
func (f controlledFailForge) Quarantine(string, string, string) error { return f.failure }
func TestControlledDomainAndSinkFailuresRetainBothCauses(t *testing.T) {
	owned := errors.New("owned domain failure")
	sink := errors.New("owned sink failure")
	m, f, _ := newTestManager(t)
	m.skills = controlledFailForge{f, owned}
	_, err := m.InstallContext(context.Background(), "corr", "", "web-research-pack", "", func(e Event) error {
		if e.Stage == "skill" && !e.OK {
			return sink
		}
		return nil
	})
	if !errors.Is(err, owned) || !errors.Is(err, sink) {
		t.Fatalf("EXPECTED:domain and sink causes ACTUAL:%v", err)
	}
	if err := m.store.RecordInstall(InstalledPack{Name: "owned", SkillIDs: []string{"owned"}}); err != nil {
		t.Fatal(err)
	}
	err = m.UninstallContext(context.Background(), "corr", "owned", func(Event) error { return sink })
	if !errors.Is(err, owned) || !errors.Is(err, sink) {
		t.Fatalf("EXPECTED:reverse and sink causes ACTUAL:%v", err)
	}
}

func TestControlledCancellationAfterSkillAndReverseStopsNextSubsystem(t *testing.T) {
	m, f, mc := newTestManager(t)
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	_, err := m.InstallContext(ctx, "corr", "", "web-research-pack", "", func(e Event) error {
		if e.Stage == "skill" {
			cancel()
		}
		return nil
	})
	installed, _ := m.store.Installed()
	if err != context.Canceled || len(f.created) != 1 || len(f.promoted) != 1 || len(mc.added) != 0 || len(installed) != 0 {
		t.Fatal(err, f, mc, installed)
	}
	if err := m.store.RecordInstall(InstalledPack{Name: "owned", SkillIDs: []string{"owned"}, MCPServers: []string{"owned"}}); err != nil {
		t.Fatal(err)
	}
	ctx, cancel = context.WithCancel(context.Background())
	defer cancel()
	err = m.UninstallContext(ctx, "corr", "owned", func(Event) error { cancel(); return nil })
	installed, _ = m.store.Installed()
	if err != context.Canceled || len(f.quarantine) != 1 || len(mc.removed) != 0 || len(installed) != 1 {
		t.Fatal(err, f, mc, installed)
	}
}
