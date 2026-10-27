// SPDX-License-Identifier: MIT

package runtime_test

import (
	"testing"

	"github.com/agezt/agezt/kernel/contract/journalapi"
	"github.com/agezt/agezt/kernel/event"
	"github.com/agezt/agezt/kernel/modules/artifacts/artifact"
	"github.com/agezt/agezt/kernel/runtime"
	"github.com/agezt/agezt/plugins/providers/mock"
)

// TestTheSystemTaskPortsHoldTheKernelsCapabilities: the kernel hands the
// cadence module the capabilities its system tasks run on.
//
// The ports are the CONTRACT's, so the module that holds them never names
// the kernel — which is the edge this slice closes.
func TestTheSystemTaskPortsHoldTheKernelsCapabilities(t *testing.T) {
	k, err := runtime.Open(runtime.Config{BaseDir: t.TempDir(), Provider: mock.New()})
	if err != nil {
		t.Fatalf("Open: %v", err)
	}
	t.Cleanup(func() { _ = k.Close() })

	ports := k.SystemTaskPorts()
	if ports.Publish == nil {
		t.Error("the kernel always has a bus — the publish port must not be nil")
	}
	if ports.ScanJournal == nil {
		t.Error("the kernel always has a journal — the scan port must not be nil")
	}
	if ports.DistillProfile == nil {
		t.Error("the distill path is the kernel's own — the port must not be nil")
	}
	if ports.RosterList == nil {
		t.Error("the roster list is the kernel's own — the port must not be nil")
	}
	if ports.Reload == nil {
		t.Error("the catalog reload is the kernel's own — the port must not be nil")
	}
}

// TestTheSystemTaskPortsStayNilWithoutTheCapability: a capability the daemon
// does not have is a NIL port, and a nil port is a refusal.
//
// The check is on the CONCRETE value: a nil *journal.Journal or a nil
// *artifact.Index handed to an interface parameter is already a non-nil
// interface, and the task would then hold a capability that is not there.
func TestTheSystemTaskPortsStayNilWithoutTheCapability(t *testing.T) {
	k, err := runtime.Open(runtime.Config{BaseDir: t.TempDir(), Provider: mock.New()})
	if err != nil {
		t.Fatalf("Open: %v", err)
	}
	t.Cleanup(func() { _ = k.Close() })

	ports := k.SystemTaskPorts()
	if ports.Memory != nil {
		t.Errorf("a kernel with no memory must yield a nil memory port, got %v", ports.Memory)
	}
	if ports.CollectArtifacts != nil {
		t.Error("a kernel with no artifact index must yield a nil collect port")
	}
	if ports.CatalogSyncer != nil {
		t.Errorf("a kernel with no catalog library must yield a nil syncer port, got %v", ports.CatalogSyncer)
	}
	if ports.CatalogStore != nil {
		t.Errorf("a kernel with no catalog library must yield a nil store port, got %v", ports.CatalogStore)
	}
}

// TestTheSystemTaskPortsCarryTheArtifactIndex: the artifact index is the
// HOST's file, and the host opens it — so the collect port is present only
// when the host opened one.
//
// The port is ONE verb and not the index's ten: two of them delete bytes and
// one switches off the journal's ref guard.
func TestTheSystemTaskPortsCarryTheArtifactIndex(t *testing.T) {
	store, err := artifact.Open(t.TempDir())
	if err != nil {
		t.Fatalf("artifact.Open: %v", err)
	}
	index, err := artifact.OpenIndex(store, t.TempDir())
	if err != nil {
		t.Fatalf("artifact.OpenIndex: %v", err)
	}
	k, err := runtime.Open(runtime.Config{
		BaseDir:       t.TempDir(),
		Provider:      mock.New(),
		Artifacts:     store,
		ArtifactIndex: index,
	})
	if err != nil {
		t.Fatalf("Open: %v", err)
	}
	t.Cleanup(func() { _ = k.Close() })

	ports := k.SystemTaskPorts()
	if ports.CollectArtifacts == nil {
		t.Fatal("a kernel with an artifact index must yield a collect port")
	}
	collected, bytes := ports.CollectArtifacts(0)
	if collected < 0 || bytes < 0 {
		t.Errorf("the collect port answered collected=%d bytes=%d", collected, bytes)
	}
}

// TestTheSystemTaskScannerProjectsTheJournalsRecords: the scan port hands
// back the contract's PROJECTION of the durable event — the kind, the
// correlation, the timestamp and the payload.
//
// The envelope's own fields (the hash chain, the sequence) belong to the
// journal and stay there.
func TestTheSystemTaskScannerProjectsTheJournalsRecords(t *testing.T) {
	k, err := runtime.Open(runtime.Config{BaseDir: t.TempDir(), Provider: mock.New()})
	if err != nil {
		t.Fatalf("Open: %v", err)
	}
	t.Cleanup(func() { _ = k.Close() })

	if _, err := k.Bus().Publish(event.Spec{
		Subject:       "test.scanned",
		Kind:          event.KindInfo,
		Actor:         "test",
		CorrelationID: "corr-scanned",
		Payload:       map[string]any{"n": 1},
	}); err != nil {
		t.Fatalf("Publish: %v", err)
	}

	ports := k.SystemTaskPorts()
	var seen int
	if err := ports.ScanJournal.Scan(func(r journalapi.Record) error {
		if r.CorrelationID != "corr-scanned" {
			return nil
		}
		seen++
		if r.Kind != event.KindInfo {
			t.Errorf("the projected kind = %v, want %v", r.Kind, event.KindInfo)
		}
		if r.TSUnixMS <= 0 {
			t.Errorf("the projected timestamp = %v, want a stamped one", r.TSUnixMS)
		}
		if len(r.Payload) == 0 {
			t.Error("the projected payload must carry the event's body")
		}
		return nil
	}); err != nil {
		t.Fatalf("Scan: %v", err)
	}
	if seen != 1 {
		t.Errorf("the scan saw the event %d times, want 1", seen)
	}
}

// TestTheSystemTaskPublisherWritesToTheBus: the publish port writes the
// event the task handed over — nothing is translated on the way in.
func TestTheSystemTaskPublisherWritesToTheBus(t *testing.T) {
	k, err := runtime.Open(runtime.Config{BaseDir: t.TempDir(), Provider: mock.New()})
	if err != nil {
		t.Fatalf("Open: %v", err)
	}
	t.Cleanup(func() { _ = k.Close() })

	ports := k.SystemTaskPorts()
	if err := ports.Publish.Publish(event.Spec{
		Subject:       "test.published",
		Kind:          event.KindInfo,
		Actor:         "test",
		CorrelationID: "corr-published",
	}); err != nil {
		t.Fatalf("Publish: %v", err)
	}

	found := false
	_ = k.Journal().Range(func(e *event.Event) error {
		if e.Subject == "test.published" && e.CorrelationID == "corr-published" {
			found = true
		}
		return nil
	})
	if !found {
		t.Fatal("the published event did not land in the journal")
	}
}
