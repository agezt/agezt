// SPDX-License-Identifier: MIT

package systemtasks

// Provenance: the system-task tests. The executors used to run against a live
//             kernel; they now run against the CONTRACT's ports, so a test
//             builds the answers it needs and hands them over (W5.4e).

import (
	"context"
	"errors"
	"strings"
	"testing"
	"time"

	"github.com/agezt/agezt/kernel/cadence"
	"github.com/agezt/agezt/kernel/contract/agentapi"
	"github.com/agezt/agezt/kernel/contract/cadenceapi"
	"github.com/agezt/agezt/kernel/contract/catalogapi"
	"github.com/agezt/agezt/kernel/contract/eventapi"
	"github.com/agezt/agezt/kernel/contract/journalapi"
	"github.com/agezt/agezt/kernel/contract/memoryapi"
)

// fakePublisher records what a task published.
//
// The tasks journal their summary through the contract's publish port, and a
// test reads the recorded specs: the same facts the journal would have
// carried, without a journal.
type fakePublisher struct {
	specs []eventapi.Spec
}

func (p *fakePublisher) Publish(spec eventapi.Spec) error {
	p.specs = append(p.specs, spec)
	return nil
}

// last returns the spec the task published for the given subject.
func (p *fakePublisher) last(subject string) (eventapi.Spec, bool) {
	for i := len(p.specs) - 1; i >= 0; i-- {
		if p.specs[i].Subject == subject {
			return p.specs[i], true
		}
	}
	return eventapi.Spec{}, false
}

// fakeMaintainer is the memory half the clean and tidy tasks drive.
type fakeMaintainer struct {
	cleanReport memoryapi.CleanReport
	cleanErr    error
	cleanCorr   string
	cleanDryRun bool
	deduped     int
	dedupErr    error
	dedupCorr   string
}

func (m *fakeMaintainer) Hygiene(int64) (memoryapi.HygieneStats, error) {
	return memoryapi.HygieneStats{}, nil
}

func (m *fakeMaintainer) Prune(string, int64, bool) (int, error) { return 0, nil }

func (m *fakeMaintainer) CleanLowValue(corr string, dryRun bool) (memoryapi.CleanReport, error) {
	m.cleanCorr = corr
	m.cleanDryRun = dryRun
	return m.cleanReport, m.cleanErr
}

func (m *fakeMaintainer) Audit() (memoryapi.AuditReport, error) {
	return memoryapi.AuditReport{}, nil
}

func (m *fakeMaintainer) DedupeDistilled(corr string, dryRun bool) (int, error) {
	m.dedupCorr = corr
	return m.deduped, m.dedupErr
}

// fakeScanner walks a fixed list of records and answers a fixed head.
type fakeScanner struct {
	records []journalapi.Record
	headSeq int64
	head    string
	scanned int
}

func (s *fakeScanner) Scan(visit func(journalapi.Record) error) error {
	for _, r := range s.records {
		s.scanned++
		if err := visit(r); err != nil {
			return err
		}
	}
	return nil
}

func (s *fakeScanner) Head() (int64, string) { return s.headSeq, s.head }

// fakeSyncer answers one sync with the given bytes and counts.
type fakeSyncer struct {
	raw       []byte
	catalog   *catalogapi.Catalog
	result    catalogapi.SyncResult
	err       error
	syncCalls int
}

func (s *fakeSyncer) Sync(context.Context, string, time.Duration) ([]byte, *catalogapi.Catalog, catalogapi.SyncResult, error) {
	s.syncCalls++
	return s.raw, s.catalog, s.result, s.err
}

// fakeStore records what was saved.
type fakeStore struct {
	saved    [][]byte
	saveErr  error
	saveCall int
}

func (s *fakeStore) Load() (*catalogapi.Catalog, error) { return &catalogapi.Catalog{}, nil }

func (s *fakeStore) SaveAPI(raw []byte, _ string) error {
	s.saveCall++
	if s.saveErr != nil {
		return s.saveErr
	}
	s.saved = append(s.saved, raw)
	return nil
}

func (s *fakeStore) SaveLocal(*catalogapi.Catalog, string) error { return nil }

func (s *fakeStore) UpsertCustomProvider(*catalogapi.Provider) (bool, error) { return false, nil }

func (s *fakeStore) LoadMeta() (catalogapi.Meta, error) { return catalogapi.Meta{}, nil }

// testPorts builds the port set a test hands the tasks.
//
// Every capability is present unless the test takes it away: a missing
// capability is a nil field, and the refusal is the point of the port.
func testPorts() (cadenceapi.SystemTaskPorts, *fakePublisher) {
	pub := &fakePublisher{}
	return cadenceapi.SystemTaskPorts{
		Publish:          pub,
		Memory:           &fakeMaintainer{},
		DistillProfile:   func(context.Context, string) (memoryapi.ProfileReport, error) { return memoryapi.ProfileReport{}, nil },
		RosterList:       func() []agentapi.Profile { return nil },
		CollectArtifacts: func(int64) (int, int64) { return 0, 0 },
		ScanJournal:      &fakeScanner{},
		CatalogSyncer:    &fakeSyncer{},
		CatalogStore:     &fakeStore{},
		Reload:           func() (*catalogapi.Catalog, bool, error) { return &catalogapi.Catalog{}, false, nil },
	}, pub
}

func TestRunScheduledMemoryCleanPublishesSummary(t *testing.T) {
	ports, pub := testPorts()
	mem := &fakeMaintainer{cleanReport: memoryapi.CleanReport{Scanned: 11, Rejected: 2, Removed: 3}}
	ports.Memory = mem

	if err := runScheduledMemoryClean(context.Background(), ports, "corr-memory-clean", "sched-memory-clean"); err != nil {
		t.Fatalf("runScheduledMemoryClean: %v", err)
	}
	if mem.cleanCorr != "corr-memory-clean" {
		t.Errorf("the clean must run under the caller's correlation, got %q", mem.cleanCorr)
	}
	if mem.cleanDryRun {
		t.Error("the scheduled clean must really clean — a dry run reports a store it did not touch")
	}
	spec, ok := pub.last("schedule.system_task.memory_clean")
	if !ok {
		t.Fatal("the memory_clean summary event was not published")
	}
	if spec.CorrelationID != "corr-memory-clean" {
		t.Errorf("summary correlation = %q, want corr-memory-clean", spec.CorrelationID)
	}
	payload, _ := spec.Payload.(map[string]any)
	if payload["scanned"] != 11 || payload["rejected"] != 2 || payload["removed"] != 3 {
		t.Errorf("summary payload = %v, want the clean report's numbers", payload)
	}
	if payload["schedule_id"] != "sched-memory-clean" {
		t.Errorf("summary payload schedule_id = %v", payload["schedule_id"])
	}
}

func TestRunScheduledMemoryTidyPublishesSummary(t *testing.T) {
	ports, pub := testPorts()
	mem := &fakeMaintainer{deduped: 4}
	ports.Memory = mem

	if err := runScheduledMemoryTidy(context.Background(), ports, "corr-memory-tidy", "sched-memory-tidy"); err != nil {
		t.Fatalf("runScheduledMemoryTidy: %v", err)
	}
	if mem.dedupCorr != "corr-memory-tidy" {
		t.Errorf("the tidy must run under the caller's correlation, got %q", mem.dedupCorr)
	}
	spec, ok := pub.last("schedule.system_task.memory_tidy")
	if !ok {
		t.Fatal("the memory_tidy summary event was not published")
	}
	payload, _ := spec.Payload.(map[string]any)
	if payload["collapsed"] != 4 {
		t.Errorf("summary payload collapsed = %v, want 4", payload["collapsed"])
	}
}

func TestRunScheduledLogCleanPublishesSummary(t *testing.T) {
	ports, pub := testPorts()
	scanner := &fakeScanner{
		records: []journalapi.Record{
			{TSUnixMS: 300},
			{TSUnixMS: 100},
			{TSUnixMS: 200},
		},
		headSeq: 42,
		head:    "abc123",
	}
	ports.ScanJournal = scanner

	if err := runScheduledLogClean(context.Background(), ports, "corr-log-clean", "sched-log-clean"); err != nil {
		t.Fatalf("runScheduledLogClean: %v", err)
	}
	if scanner.scanned != 3 {
		t.Errorf("the scan visited %d records, want 3", scanner.scanned)
	}
	spec, ok := pub.last("schedule.system_task.log_clean")
	if !ok {
		t.Fatal("the log_clean summary event was not published")
	}
	payload, _ := spec.Payload.(map[string]any)
	if payload["events_scanned"] != int64(3) {
		t.Errorf("events_scanned = %v, want 3", payload["events_scanned"])
	}
	if payload["oldest_unix_ms"] != int64(100) {
		t.Errorf("oldest_unix_ms = %v, want 100", payload["oldest_unix_ms"])
	}
	if payload["latest_unix_ms"] != int64(300) {
		t.Errorf("latest_unix_ms = %v, want 300", payload["latest_unix_ms"])
	}
	if payload["head_seq"] != int64(42) {
		t.Errorf("head_seq = %v, want 42", payload["head_seq"])
	}
	if payload["head_hash"] != "abc123" {
		t.Errorf("head_hash = %v, want abc123", payload["head_hash"])
	}
	if payload["effect_class"] != "log_maintenance" || payload["physical_deletion"] != false {
		t.Errorf("the summary must say it only inspects: %v", payload)
	}
}

func TestRunScheduledGraveyardScanReportsOnly(t *testing.T) {
	ports, pub := testPorts()
	ports.RosterList = func() []agentapi.Profile {
		return []agentapi.Profile{
			{Slug: "live-one"},
			{Slug: "live-two"},
			{Slug: "dead", Retired: true, RetiredMS: 100},
		}
	}
	// Retention disabled (default keep-forever): reports graveyard size, flags none.
	t.Setenv("AGEZT_GRAVEYARD_RETENTION_DAYS", "")

	if err := runScheduledGraveyardScan(context.Background(), ports, "corr-grave", "sched-grave"); err != nil {
		t.Fatalf("runScheduledGraveyardScan: %v", err)
	}
	spec, ok := pub.last("schedule.system_task.graveyard_scan")
	if !ok {
		t.Fatal("the graveyard_scan summary event was not published")
	}
	payload, _ := spec.Payload.(map[string]any)
	if payload["graveyard_count"] != 1 {
		t.Errorf("graveyard_count = %v, want 1", payload["graveyard_count"])
	}
	if payload["eligible_count"] != 0 {
		t.Errorf("eligible_count = %v, want 0 with retention disabled", payload["eligible_count"])
	}
	if payload["action"] != "report_only" {
		t.Errorf("the scan must report only, got action = %v", payload["action"])
	}
}

func TestRunScheduledGraveyardScanFlagsTheEligible(t *testing.T) {
	ports, pub := testPorts()
	ports.RosterList = func() []agentapi.Profile {
		return []agentapi.Profile{
			{Slug: "zeta", Retired: true, RetiredMS: 1},
			{Slug: "alpha", Retired: true, RetiredMS: 1},
		}
	}
	t.Setenv("AGEZT_GRAVEYARD_RETENTION_DAYS", "1")

	if err := runScheduledGraveyardScan(context.Background(), ports, "corr-grave", "sched-grave"); err != nil {
		t.Fatalf("runScheduledGraveyardScan: %v", err)
	}
	spec, _ := pub.last("schedule.system_task.graveyard_scan")
	payload, _ := spec.Payload.(map[string]any)
	if payload["eligible_count"] != 2 {
		t.Errorf("eligible_count = %v, want 2", payload["eligible_count"])
	}
	eligible, _ := payload["eligible"].([]string)
	if len(eligible) != 2 || eligible[0] != "alpha" || eligible[1] != "zeta" {
		t.Errorf("eligible = %v, want the sorted [alpha zeta]", payload["eligible"])
	}
}

func TestRunScheduledArtifactCollectPublishesSummary(t *testing.T) {
	ports, pub := testPorts()
	var cutoffSeen int64
	ports.CollectArtifacts = func(olderThanMs int64) (int, int64) {
		cutoffSeen = olderThanMs
		return 7, 4096
	}

	if err := runScheduledArtifactCollect(context.Background(), ports, "corr-collect", "sched-collect"); err != nil {
		t.Fatalf("runScheduledArtifactCollect: %v", err)
	}
	if cutoffSeen == 0 {
		t.Error("the collect must pass a cutoff to the index")
	}
	spec, ok := pub.last("schedule.system_task.artifact_collect")
	if !ok {
		t.Fatal("the artifact_collect summary event was not published")
	}
	payload, _ := spec.Payload.(map[string]any)
	if payload["collected"] != 7 || payload["bytes"] != int64(4096) {
		t.Errorf("summary payload = %v, want the collect's numbers", payload)
	}
	if payload["older_than_days"] != 30 {
		t.Errorf("older_than_days = %v, want 30", payload["older_than_days"])
	}
}

func TestRunScheduledCatalogSyncPublishesSummaryAndReloads(t *testing.T) {
	ports, pub := testPorts()
	syncer := &fakeSyncer{
		raw:    []byte(`{"testprov":{}}`),
		result: catalogapi.SyncResult{Bytes: 128, ProviderCount: 1, ModelCount: 1},
	}
	store := &fakeStore{}
	reloads := 0
	ports.CatalogSyncer = syncer
	ports.CatalogStore = store
	ports.Reload = func() (*catalogapi.Catalog, bool, error) {
		reloads++
		return &catalogapi.Catalog{}, true, nil
	}

	if err := runScheduledCatalogSync(context.Background(), ports, "corr-catalog-sync", "sched-catalog-sync"); err != nil {
		t.Fatalf("runScheduledCatalogSync: %v", err)
	}
	if syncer.syncCalls != 1 {
		t.Errorf("the sync ran %d times, want 1", syncer.syncCalls)
	}
	if store.saveCall != 1 {
		t.Errorf("the catalog was saved %d times, want 1", store.saveCall)
	}
	if len(store.saved) != 1 || string(store.saved[0]) != string(syncer.raw) {
		t.Errorf("the saved catalog = %q, want the bytes the syncer fetched", store.saved)
	}
	if reloads != 1 {
		t.Errorf("reloads = %d, want 1", reloads)
	}
	spec, ok := pub.last("catalog.sync")
	if !ok {
		t.Fatal("the catalog.sync summary event was not published")
	}
	payload, _ := spec.Payload.(map[string]any)
	if payload["provider_count"] != 1 || payload["model_count"] != 1 {
		t.Errorf("summary payload counts = providers %v models %v, want 1/1", payload["provider_count"], payload["model_count"])
	}
	if payload["providers_reloaded"] != true {
		t.Errorf("providers_reloaded = %v, want true", payload["providers_reloaded"])
	}
	if payload["effect_class"] != "config_update" {
		t.Errorf("effect_class = %v, want config_update", payload["effect_class"])
	}
}

func TestRunScheduledProfileDistillPublishesSummary(t *testing.T) {
	ports, pub := testPorts()
	ports.DistillProfile = func(_ context.Context, corr string) (memoryapi.ProfileReport, error) {
		if corr != "corr-distill" {
			t.Errorf("the distill ran under %q, want corr-distill", corr)
		}
		return memoryapi.ProfileReport{InputRecords: 9, FacetsWritten: 2, Facets: []string{"style"}}, nil
	}

	if err := runScheduledProfileDistill(context.Background(), ports, "corr-distill", "sched-distill"); err != nil {
		t.Fatalf("runScheduledProfileDistill: %v", err)
	}
	spec, ok := pub.last("schedule.system_task.profile_distill")
	if !ok {
		t.Fatal("the profile_distill summary event was not published")
	}
	payload, _ := spec.Payload.(map[string]any)
	if payload["input_records"] != 9 || payload["facets_written"] != 2 {
		t.Errorf("summary payload = %v, want the distill report's numbers", payload)
	}
}

// TestTheTasksRefuseWhenTheDaemonHasNoCapability: a missing capability is a
// refusal, and the refusal says WHICH capability is missing.
//
// "This daemon has no memory" and "the memory is empty" are different facts,
// and a task that reported the second one would be reporting a store it never
// looked at.
func TestTheTasksRefuseWhenTheDaemonHasNoCapability(t *testing.T) {
	cases := []struct {
		name    string
		prepare func(ports *cadenceapi.SystemTaskPorts)
		run     func(ports cadenceapi.SystemTaskPorts) error
		want    string
	}{
		{
			name:    "no memory",
			prepare: func(p *cadenceapi.SystemTaskPorts) { p.Memory = nil },
			run: func(p cadenceapi.SystemTaskPorts) error {
				return runScheduledMemoryClean(context.Background(), p, "c", "s")
			},
			want: "memory unavailable",
		},
		{
			name:    "no memory (tidy)",
			prepare: func(p *cadenceapi.SystemTaskPorts) { p.Memory = nil },
			run: func(p cadenceapi.SystemTaskPorts) error {
				return runScheduledMemoryTidy(context.Background(), p, "c", "s")
			},
			want: "memory unavailable",
		},
		{
			name:    "no journal",
			prepare: func(p *cadenceapi.SystemTaskPorts) { p.ScanJournal = nil },
			run: func(p cadenceapi.SystemTaskPorts) error {
				return runScheduledLogClean(context.Background(), p, "c", "s")
			},
			want: "journal unavailable",
		},
		{
			name:    "no artifact index",
			prepare: func(p *cadenceapi.SystemTaskPorts) { p.CollectArtifacts = nil },
			run: func(p cadenceapi.SystemTaskPorts) error {
				return runScheduledArtifactCollect(context.Background(), p, "c", "s")
			},
			want: "artifact index unavailable",
		},
		{
			name:    "no catalog library",
			prepare: func(p *cadenceapi.SystemTaskPorts) { p.CatalogSyncer = nil },
			run: func(p cadenceapi.SystemTaskPorts) error {
				return runScheduledCatalogSync(context.Background(), p, "c", "s")
			},
			want: "the catalog library is not available on this daemon",
		},
		{
			name:    "no catalog store",
			prepare: func(p *cadenceapi.SystemTaskPorts) { p.CatalogStore = nil },
			run: func(p cadenceapi.SystemTaskPorts) error {
				return runScheduledCatalogSync(context.Background(), p, "c", "s")
			},
			want: "the catalog library is not available on this daemon",
		},
		{
			name:    "no reload path",
			prepare: func(p *cadenceapi.SystemTaskPorts) { p.Reload = nil },
			run: func(p cadenceapi.SystemTaskPorts) error {
				return runScheduledCatalogSync(context.Background(), p, "c", "s")
			},
			want: "catalog reload unavailable",
		},
		{
			name:    "no publisher",
			prepare: func(p *cadenceapi.SystemTaskPorts) { p.Publish = nil },
			run: func(p cadenceapi.SystemTaskPorts) error {
				return runScheduledMemoryTidy(context.Background(), p, "c", "s")
			},
			want: "the event bus is unavailable",
		},
		{
			name:    "no roster",
			prepare: func(p *cadenceapi.SystemTaskPorts) { p.RosterList = nil },
			run: func(p cadenceapi.SystemTaskPorts) error {
				return runScheduledGraveyardScan(context.Background(), p, "c", "s")
			},
			want: "roster unavailable",
		},
		{
			name:    "no distill path",
			prepare: func(p *cadenceapi.SystemTaskPorts) { p.DistillProfile = nil },
			run: func(p cadenceapi.SystemTaskPorts) error {
				return runScheduledProfileDistill(context.Background(), p, "c", "s")
			},
			want: "profile distillation unavailable",
		},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			ports, _ := testPorts()
			tc.prepare(&ports)
			err := tc.run(ports)
			if err == nil {
				t.Fatalf("the task must refuse without %s", tc.name)
			}
			if !strings.Contains(err.Error(), tc.want) {
				t.Fatalf("the refusal = %q, want it to name %q", err.Error(), tc.want)
			}
		})
	}
}

// TestTheCatalogSyncSurvivesAReloadThatAnswersNoError: a reload that answers
// no catalog and NO error at all must not take the daemon down.
//
// The message is built from what the port actually answered, so a nil error
// cannot be dereferenced on the way to the summary event.
func TestTheCatalogSyncSurvivesAReloadThatAnswersNoError(t *testing.T) {
	ports, pub := testPorts()
	ports.Reload = func() (*catalogapi.Catalog, bool, error) { return nil, false, nil }

	err := runScheduledCatalogSync(context.Background(), ports, "corr", "sched")
	if err == nil {
		t.Fatal("a reload that answered no catalog must fail the task")
	}
	spec, ok := pub.last("catalog.sync")
	if !ok {
		t.Fatal("the failure must be journalled")
	}
	payload, _ := spec.Payload.(map[string]any)
	if payload["error"] == "" || payload["error"] == nil {
		t.Errorf("the failure summary must carry the error, got %v", payload)
	}
}

// TestTheCatalogSyncReportsTheReloadError: a reload that fails with an error
// publishes THAT error, not a generic one.
func TestTheCatalogSyncReportsTheReloadError(t *testing.T) {
	ports, pub := testPorts()
	ports.Reload = func() (*catalogapi.Catalog, bool, error) {
		return nil, false, errors.New("provider registry exploded")
	}

	err := runScheduledCatalogSync(context.Background(), ports, "corr", "sched")
	if err == nil {
		t.Fatal("a failed reload must fail the task")
	}
	if !strings.Contains(err.Error(), "provider registry exploded") {
		t.Errorf("the failure = %q, want it to name the reload's error", err.Error())
	}
	spec, _ := pub.last("catalog.sync")
	payload, _ := spec.Payload.(map[string]any)
	if payload["error"] != "provider registry exploded" {
		t.Errorf("the journalled error = %v, want the reload's own error", payload["error"])
	}
}

// TestRunDispatchesEverySystemTask: every task the catalogue names is
// reachable through the dispatch.
//
// The catalogue and the dispatch are two lists, and a task that is in one and
// not in the other is a task that exists and never runs.
func TestRunDispatchesEverySystemTask(t *testing.T) {
	ports, _ := testPorts()
	for _, name := range []string{
		cadence.SystemTaskCatalogSync,
		cadence.SystemTaskArtifactCollect,
		cadence.SystemTaskMemoryClean,
		cadence.SystemTaskMemoryTidy,
		cadence.SystemTaskLogClean,
		cadence.SystemTaskGraveyardScan,
		cadence.SystemTaskProfileDistill,
	} {
		if err := Run(context.Background(), ports, "corr", "sched", name); err != nil {
			t.Errorf("the dispatch of %s failed: %v", name, err)
		}
	}
	// The catalogue and the dispatch must agree on the whole set.
	for _, info := range cadence.SystemTaskInfos() {
		err := Run(context.Background(), ports, "corr", "sched", info.Name)
		if err != nil && strings.Contains(err.Error(), "unknown system task") {
			t.Errorf("the catalogue names %q but the dispatch does not know it", info.Name)
		}
	}
}

// TestRunRejectsAnUnknownTask: an unknown task name is an error, not a
// silent no-op.
func TestRunRejectsAnUnknownTask(t *testing.T) {
	ports, _ := testPorts()
	err := Run(context.Background(), ports, "corr", "sched", "not_a_task")
	if err == nil {
		t.Fatal("an unknown system task must be an error")
	}
	if !strings.Contains(err.Error(), "unknown system task") {
		t.Errorf("the error = %q, want it to name the unknown task", err.Error())
	}
}
