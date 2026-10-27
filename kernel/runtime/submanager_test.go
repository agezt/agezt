// SPDX-License-Identifier: MIT

package runtime_test

import (
	"reflect"
	"sort"
	"strings"
	"testing"

	"github.com/agezt/agezt/kernel/runtime"
)

// TestTheKernelHoldsOnlyTheSubManagersItCalls: the kernel holds exactly the
// two sub-managers it actually calls.
//
// W3.40 deleted the third — the accessors sub-manager — which was wired into
// the kernel (k.accessors = accessors.New(k)) and then read by nobody. It
// duplicated the kernel's own accessor methods, so it was a second,
// unreachable copy of the read surface — and it was the reason the kernel's
// module reach was counted TWICE in the archcheck ratchet: eleven allowlisted
// edges that bought nothing at all.
//
// A third sub-manager that nobody calls is the same defect wearing a new
// name, so the set is pinned here. A new sub-manager has to be wired AND
// called, and this test says so the moment it is wired and not called.
func TestTheKernelHoldsOnlyTheSubManagersItCalls(t *testing.T) {
	const runtimePkg = "github.com/agezt/agezt/kernel/runtime/"

	kernelType := reflect.TypeOf(runtime.Kernel{})
	var subs []string
	for i := 0; i < kernelType.NumField(); i++ {
		field := kernelType.Field(i)
		if field.Type.Kind() != reflect.Ptr || field.Type.Elem().Kind() != reflect.Struct {
			continue
		}
		pkg := field.Type.Elem().PkgPath()
		if !strings.HasPrefix(pkg, runtimePkg) {
			continue
		}
		subs = append(subs, strings.TrimPrefix(pkg, runtimePkg))
	}
	sort.Strings(subs)

	want := []string{"lifecycle", "runexec"}
	if len(subs) != len(want) {
		t.Fatalf("the kernel holds the sub-managers %v, want exactly %v — a sub-manager that nobody calls is a second copy of the surface, and it is counted twice in the ratchet", subs, want)
	}
	for i := range want {
		if subs[i] != want[i] {
			t.Errorf("the kernel holds the sub-managers %v, want exactly %v", subs, want)
			break
		}
	}
}

// TestTheKernelStillAnswersItsAccessorSurface pins the accessor surface by
// name.
//
// The deletion removed a COMPILE-TIME check: the deleted sub-package declared
// a KernelAPI interface of ~45 methods, and handing *Kernel to it proved the
// kernel answered every one of them. That check protected an unreachable
// copy, so it protected nothing — but the surface it guarded IS the surface
// callers use, so the list is pinned here by name. A getter that disappears
// now fails HERE, instead of failing nowhere.
//
// One of the 45 went with the package: ConfigMu() had the deleted
// sub-package as its only caller, so it was deleted too — a getter nobody
// calls is the same defect as a copy nobody calls.
func TestTheKernelStillAnswersItsAccessorSurface(t *testing.T) {
	kernelType := reflect.TypeOf(&runtime.Kernel{})
	for _, name := range []string{
		// Day 13 — the store getters the deleted sub-package mirrored.
		"Journal", "Bus", "State", "Edict", "Warden", "Approvals", "Scheduler",
		"Provider", "Memory", "Schedules",
		// Day 14 — the live reads.
		"Tools", "World", "Forge", "StartTime", "ConfigCenter", "MaxDuration",
		"Reflect",
		// Day 14 — the configMu-guarded live mutators.
		"ScheduleEngine", "SetScheduleEngine",
		// Day 15 — the standing / roster stores and the bus hook.
		"Standing", "Roster", "PublishBusEvent",
		// Day 16 — the store + configMu-free reads / writes.
		"Market", "SetMarket", "BaseDir", "ArtifactIndex", "DataLake",
		// Day 18a — the configMu-guarded live mutators and the read helpers.
		"Model", "SetModel", "System", "SetSystem", "SetCouncilMembers",
		"CouncilMembers", "SubAgentLimits", "Plugins",
		// Day 19 — catalog + reload + loop + plan.
		"Catalog", "CatalogStore", "Reload", "ReloadCatalog", "LoopRunner",
		"RunPlan",
	} {
		if _, ok := kernelType.MethodByName(name); !ok {
			t.Errorf("*Kernel has no method %s — the accessor surface is pinned by name, because the deleted sub-package's compile-time check is gone", name)
		}
	}

	// ConfigMu had the deleted sub-package as its only caller, so it went with
	// it. A getter nobody calls is the same defect as a copy nobody calls, so
	// its ABSENCE is pinned too: if it comes back, it comes back dead.
	if _, ok := kernelType.MethodByName("ConfigMu"); ok {
		t.Error("*Kernel has a ConfigMu method again — it had the deleted sub-package as its only caller, and a getter nobody calls is dead code")
	}

	// AgentGateway went the same way (W3.67): the agent gateway is an
	// ADAPTER (L5), so the kernel (L3) may not hold one, and the accessor
	// had no caller at all — the daemon owns the gateway now. Its ABSENCE
	// is pinned, because an accessor that comes back comes back with the
	// upward import edge that the ratchet just closed.
	if _, ok := kernelType.MethodByName("AgentGateway"); ok {
		t.Error("*Kernel has an AgentGateway method again — the gateway is the daemon's adapter (L5), the kernel is a module (L3), and the accessor had no caller")
	}
}
