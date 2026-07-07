// SPDX-License-Identifier: MIT

package okr

import (
	"testing"
	"time"
)

func TestCreateAddKeyResultLinkAndPersist(t *testing.T) {
	dir := t.TempDir()
	st, err := OpenStore(dir)
	if err != nil {
		t.Fatal(err)
	}
	obj, err := st.Create(CreateSpec{Title: "Ship proof loop", Owner: "founder"}, time.UnixMilli(1000))
	if err != nil {
		t.Fatalf("Create: %v", err)
	}
	if obj.Status != StatusActive || obj.Title != "Ship proof loop" {
		t.Fatalf("objective = %+v", obj)
	}
	obj, err = st.AddKeyResult(obj.ID, "2 gated tasks proven", 2, time.UnixMilli(2000))
	if err != nil {
		t.Fatalf("AddKeyResult: %v", err)
	}
	if len(obj.KeyResults) != 1 || obj.KeyResults[0].Target != 2 {
		t.Fatalf("key results = %+v", obj.KeyResults)
	}
	krID := obj.KeyResults[0].ID
	obj, err = st.LinkTask(obj.ID, krID, "task-a", time.UnixMilli(3000))
	if err != nil {
		t.Fatalf("LinkTask: %v", err)
	}
	// Idempotent re-link.
	obj, err = st.LinkTask(obj.ID, krID, "task-a", time.UnixMilli(3100))
	if err != nil {
		t.Fatalf("LinkTask idempotent: %v", err)
	}
	obj, err = st.LinkTask(obj.ID, krID, "task-b", time.UnixMilli(3200))
	if err != nil {
		t.Fatalf("LinkTask b: %v", err)
	}
	if got := obj.KeyResults[0].TaskIDs; len(got) != 2 {
		t.Fatalf("task ids = %v, want 2 unique", got)
	}

	// Reopen: state persists.
	re, err := OpenStore(dir)
	if err != nil {
		t.Fatalf("reopen: %v", err)
	}
	got, ok := re.Get(obj.ID)
	if !ok || len(got.KeyResults) != 1 || len(got.KeyResults[0].TaskIDs) != 2 {
		t.Fatalf("reopened objective = %+v ok=%v", got, ok)
	}
}

func TestProgressRollupFromDoneTasks(t *testing.T) {
	st, err := OpenStore(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	obj, _ := st.Create(CreateSpec{Title: "Q3"}, time.UnixMilli(1000))
	obj, _ = st.AddKeyResult(obj.ID, "KR1 target=2", 2, time.UnixMilli(1100))
	obj, _ = st.AddKeyResult(obj.ID, "KR2 all-linked", 0, time.UnixMilli(1200))
	kr1, kr2 := obj.KeyResults[0].ID, obj.KeyResults[1].ID
	obj, _ = st.LinkTask(obj.ID, kr1, "a", time.UnixMilli(1300))
	obj, _ = st.LinkTask(obj.ID, kr1, "b", time.UnixMilli(1310))
	obj, _ = st.LinkTask(obj.ID, kr2, "c", time.UnixMilli(1320))

	// Only "a" and "c" are done.
	done := map[string]bool{"a": true, "c": true}
	pr := obj.Progress(func(id string) bool { return done[id] })

	if len(pr.KeyResults) != 2 {
		t.Fatalf("kr progress len = %d", len(pr.KeyResults))
	}
	// KR1: 1 of target 2 done → 50%, not achieved.
	if pr.KeyResults[0].Done != 1 || pr.KeyResults[0].Percent != 50 || pr.KeyResults[0].Achieved {
		t.Fatalf("KR1 = %+v", pr.KeyResults[0])
	}
	// KR2: target defaults to 1 linked, "c" done → 100%, achieved.
	if pr.KeyResults[1].Done != 1 || pr.KeyResults[1].Percent != 100 || !pr.KeyResults[1].Achieved {
		t.Fatalf("KR2 = %+v", pr.KeyResults[1])
	}
	// Objective: (50 + 100)/2 = 75%, not achieved (KR1 unmet).
	if pr.Percent != 75 || pr.Achieved {
		t.Fatalf("objective progress = %+v", pr)
	}

	// Complete "b" → KR1 achieved → objective achieved.
	done["b"] = true
	pr = obj.Progress(func(id string) bool { return done[id] })
	if !pr.KeyResults[0].Achieved || !pr.Achieved || pr.Percent != 100 {
		t.Fatalf("after b done, progress = %+v", pr)
	}
}

func TestObjectivesForTaskAndSetStatus(t *testing.T) {
	st, err := OpenStore(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	o1, _ := st.Create(CreateSpec{Title: "O1"}, time.UnixMilli(1000))
	o1, _ = st.AddKeyResult(o1.ID, "kr", 0, time.UnixMilli(1100))
	o1, _ = st.LinkTask(o1.ID, o1.KeyResults[0].ID, "shared", time.UnixMilli(1200))
	o2, _ := st.Create(CreateSpec{Title: "O2"}, time.UnixMilli(1300))
	o2, _ = st.AddKeyResult(o2.ID, "kr", 0, time.UnixMilli(1400))
	o2, _ = st.LinkTask(o2.ID, o2.KeyResults[0].ID, "shared", time.UnixMilli(1500))

	ids := st.ObjectivesForTask("shared")
	if len(ids) != 2 {
		t.Fatalf("ObjectivesForTask = %v, want 2", ids)
	}

	got, err := st.SetStatus(o1.ID, StatusAchieved, time.UnixMilli(2000))
	if err != nil {
		t.Fatalf("SetStatus: %v", err)
	}
	if got.Status != StatusAchieved || got.AchievedMS != 2000 {
		t.Fatalf("achieved = %+v", got)
	}
	// Archived objectives drop out of ObjectivesForTask.
	if _, err := st.Archive(o2.ID, time.UnixMilli(2100)); err != nil {
		t.Fatalf("Archive: %v", err)
	}
	if ids := st.ObjectivesForTask("shared"); len(ids) != 1 || ids[0] != o1.ID {
		t.Fatalf("after archive, ObjectivesForTask = %v", ids)
	}
}

func TestOpenStore_InvalidDir(t *testing.T) {
	_, err := OpenStore("")
	if err == nil {
		t.Error("OpenStore with empty path should error")
	}
}

func TestCreate_EmptyTitle(t *testing.T) {
	st, err := OpenStore(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	_, err = st.Create(CreateSpec{Title: ""}, time.Now())
	if err == nil {
		t.Error("Create with empty title should error")
	}
}

func TestGet_NotFound(t *testing.T) {
	st, err := OpenStore(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	_, found := st.Get("nonexistent")
	if found {
		t.Error("Get nonexistent should return false")
	}
}

func TestList_Filters(t *testing.T) {
	st, err := OpenStore(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	now := time.UnixMilli(1000)
	o1, _ := st.Create(CreateSpec{Title: "Active", Tenant: "team-a"}, now)
	st.AddKeyResult(o1.ID, "KR 1", 1, now)
	st.Create(CreateSpec{Title: "Archived", Tenant: "team-b"}, now)

	// List all (no filter).
	if all := st.List(Filter{}); len(all) != 2 {
		t.Errorf("List() = %d, want 2", len(all))
	}
	// Filter by tenant.
	if filtered := st.List(Filter{Tenant: "team-a"}); len(filtered) != 1 {
		t.Errorf("List(team-a) = %d, want 1", len(filtered))
	}
	// Filter by status (no match).
	if filtered := st.List(Filter{Status: StatusArchived}); len(filtered) != 0 {
		t.Errorf("List(status=archived) = %d, want 0 (active only by default)", len(filtered))
	}
	// Limit.
	if limited := st.List(Filter{Limit: 1}); len(limited) != 1 {
		t.Errorf("List(limit=1) = %d, want 1", len(limited))
	}
	// Include archived.
	st.Archive(o1.ID, now)
	if archived := st.List(Filter{IncludeArchived: true}); len(archived) != 2 {
		t.Errorf("List(includeArchived=true) = %d, want 2", len(archived))
	}
}

func TestAddKeyResult_Validation(t *testing.T) {
	st, err := OpenStore(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	obj, _ := st.Create(CreateSpec{Title: "Test"}, time.UnixMilli(1000))

	// Empty title.
	if _, err := st.AddKeyResult(obj.ID, "", 1, time.Now()); err == nil {
		t.Error("AddKeyResult with empty title should error")
	}
	// Negative target.
	if _, err := st.AddKeyResult(obj.ID, "KR", -1, time.Now()); err == nil {
		t.Error("AddKeyResult with negative target should error")
	}
}

func TestLinkTask_Validation(t *testing.T) {
	st, err := OpenStore(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	obj, _ := st.Create(CreateSpec{Title: "Test"}, time.UnixMilli(1000))
	obj, _ = st.AddKeyResult(obj.ID, "KR 1", 1, time.UnixMilli(2000))
	krID := obj.KeyResults[0].ID

	// Empty kr id.
	if _, err := st.LinkTask(obj.ID, "", "t-1", time.Now()); err == nil {
		t.Error("LinkTask with empty krID should error")
	}
	// Non-existent kr.
	if _, err := st.LinkTask(obj.ID, "no-such-kr", "t-1", time.Now()); err != ErrKRNotFound {
		t.Errorf("LinkTask unknown kr: err=%v, want ErrKRNotFound", err)
	}
	// Non-existent obj.
	if _, err := st.LinkTask("no-such-obj", krID, "t-1", time.Now()); err == nil {
		t.Error("LinkTask unknown obj should error")
	}
}

func TestUnlinkTask(t *testing.T) {
	st, err := OpenStore(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	obj, _ := st.Create(CreateSpec{Title: "Test"}, time.UnixMilli(1000))
	st.AddKeyResult(obj.ID, "KR 1", 1, time.UnixMilli(2000))
	obj, _ = st.Get(obj.ID)
	krID := obj.KeyResults[0].ID

	// Link then unlink.
	st.LinkTask(obj.ID, krID, "t-1", time.Now())
	obj, _ = st.UnlinkTask(obj.ID, krID, "t-1", time.Now())
	kr := obj.KeyResults[0]
	if len(kr.TaskIDs) != 0 {
		t.Errorf("after unlink TaskIDs = %v, want empty", kr.TaskIDs)
	}
	// Unlink non-existent kr.
	if _, err := st.UnlinkTask(obj.ID, "no-such-kr", "t-1", time.Now()); err != ErrKRNotFound {
		t.Errorf("UnlinkTask unknown kr: err=%v, want ErrKRNotFound", err)
	}
	// Unlink non-existent obj.
	if _, err := st.UnlinkTask("no-such-obj", krID, "t-1", time.Now()); err == nil {
		t.Error("UnlinkTask unknown obj should error")
	}
}

func TestProgress_Empty(t *testing.T) {
	o := Objective{}
	p := o.Progress(nil)
	if p.Percent != 0 || p.Achieved {
		t.Errorf("Progress(empty) = %+v, want Percent=0 Achieved=false", p)
	}
	if len(p.KeyResults) != 0 {
		t.Errorf("Progress(empty) KeyResults = %d, want 0", len(p.KeyResults))
	}
}

func TestSetStatus_Errors(t *testing.T) {
	st, err := OpenStore(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	if _, err := st.SetStatus("nonexistent", StatusAchieved, time.Now()); err == nil {
		t.Error("SetStatus nonexistent should error")
	}
}

func TestFindKR_NotFound(t *testing.T) {
	kr := findKR(&Objective{}, "nonexistent")
	if kr != nil {
		t.Error("findKR nonexistent should return nil")
	}
}

func TestMutate_NotFound(t *testing.T) {
	_, err := (&Store{}).mutate("nonexistent", func(o *Objective, _ int64) error { return nil }, time.Now())
	if err == nil {
		t.Error("mutate nonexistent should error")
	}
}

func TestSaveLocked_InvalidPath(t *testing.T) {
	s := &Store{path: t.TempDir() + string(rune(0))}
	s.mu.Lock()
	err := s.saveLocked()
	s.mu.Unlock()
	if err == nil {
		t.Error("saveLocked with invalid path should error")
	}
}
