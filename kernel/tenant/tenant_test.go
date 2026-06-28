// SPDX-License-Identifier: MIT

package tenant_test

import (
	"context"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/agezt/agezt/kernel/agent"
	"github.com/agezt/agezt/kernel/runtime"
	"github.com/agezt/agezt/kernel/tenant"
	"github.com/agezt/agezt/plugins/providers/mock"
)

// fakeKernel is a minimal io.Closer standing in for a real kernel.
type fakeKernel struct {
	id, baseDir string
	closed      bool
}

func (f *fakeKernel) Close() error { f.closed = true; return nil }

// fakeOpener records every open call and returns a fakeKernel.
type fakeOpener struct {
	mu      sync.Mutex
	opens   []string // ids opened, in order (one entry per actual open)
	kernels map[string]*fakeKernel
}

func newFakeOpener() *fakeOpener { return &fakeOpener{kernels: map[string]*fakeKernel{}} }

func (o *fakeOpener) open(id, baseDir string) (io.Closer, error) {
	o.mu.Lock()
	defer o.mu.Unlock()
	o.opens = append(o.opens, id)
	k := &fakeKernel{id: id, baseDir: baseDir}
	o.kernels[id] = k
	return k, nil
}

func TestValidID(t *testing.T) {
	good := []string{"alpha", "team-1", "acme_corp", "a", "x0", strings.Repeat("a", 64)}
	for _, id := range good {
		if !tenant.ValidID(id) {
			t.Errorf("ValidID(%q) = false, want true", id)
		}
	}
	bad := []string{"", "..", "../evil", "a/b", "A", "_lead", "-lead", "white space", "dot.dot", strings.Repeat("a", 65), "café"}
	for _, id := range bad {
		if tenant.ValidID(id) {
			t.Errorf("ValidID(%q) = true, want false", id)
		}
	}
}

func TestRegistry_AcquireIsIdempotentAndIsolated(t *testing.T) {
	o := newFakeOpener()
	root := t.TempDir()
	reg, err := tenant.New(root, o.open)
	if err != nil {
		t.Fatal(err)
	}
	now := time.Unix(1780000000, 0)

	a1, err := reg.Acquire("alpha", now)
	if err != nil {
		t.Fatal(err)
	}
	a2, err := reg.Acquire("alpha", now) // second acquire must reuse
	if err != nil {
		t.Fatal(err)
	}
	if a1 != a2 {
		t.Error("Acquire should be idempotent (same *Tenant)")
	}
	b, err := reg.Acquire("beta", now)
	if err != nil {
		t.Fatal(err)
	}

	// Opened exactly once each, despite two Acquire("alpha") calls.
	if len(o.opens) != 2 {
		t.Errorf("opens = %v, want one each for alpha+beta", o.opens)
	}
	// Distinct, contained base dirs → isolation.
	if a1.BaseDir == b.BaseDir {
		t.Error("tenants must have distinct base dirs")
	}
	if filepath.Dir(a1.BaseDir) != root || filepath.Base(a1.BaseDir) != "alpha" {
		t.Errorf("alpha base dir %q not <root>/alpha", a1.BaseDir)
	}
	if reg.Count() != 2 {
		t.Errorf("Count = %d, want 2", reg.Count())
	}
	// Base dirs exist on disk.
	for _, dir := range []string{a1.BaseDir, b.BaseDir} {
		if info, err := os.Stat(dir); err != nil || !info.IsDir() {
			t.Errorf("base dir %q not created", dir)
		}
	}
}

func TestRegistry_RejectsTraversalBeforeAnySideEffect(t *testing.T) {
	o := newFakeOpener()
	root := t.TempDir()
	reg, _ := tenant.New(root, o.open)
	for _, bad := range []string{"../evil", "a/b", ".."} {
		if _, err := reg.Acquire(bad, time.Now()); err == nil {
			t.Errorf("Acquire(%q) should be rejected", bad)
		}
	}
	if len(o.opens) != 0 {
		t.Errorf("no kernel should open for invalid ids, got %v", o.opens)
	}
	// Nothing leaked outside root.
	if _, err := os.Stat(filepath.Join(filepath.Dir(root), "evil")); err == nil {
		t.Error("a traversal id created a dir outside root")
	}
}

func TestRegistry_ReleaseAndRemove(t *testing.T) {
	o := newFakeOpener()
	reg, _ := tenant.New(t.TempDir(), o.open)
	now := time.Now()
	a, _ := reg.Acquire("alpha", now)
	reg.Acquire("beta", now)

	// Release closes alpha's kernel but keeps its dir; beta stays open.
	released, err := reg.Release("alpha")
	if !released || err != nil {
		t.Fatalf("Release: %v %v", released, err)
	}
	if !o.kernels["alpha"].closed {
		t.Error("released kernel should be closed")
	}
	if reg.Count() != 1 {
		t.Errorf("Count after release = %d, want 1", reg.Count())
	}
	if !reg.Exists("alpha") {
		t.Error("released tenant dir should remain on disk")
	}
	// Re-acquire reopens (a fresh open call).
	if _, err := reg.Acquire("alpha", now); err != nil {
		t.Fatal(err)
	}
	if len(o.opens) != 3 {
		t.Errorf("re-acquire should reopen; opens=%v", o.opens)
	}

	// Remove deletes the dir.
	removed, err := reg.Remove("beta")
	if !removed || err != nil {
		t.Fatalf("Remove: %v %v", removed, err)
	}
	if reg.Exists("beta") {
		t.Error("removed tenant dir should be gone")
	}
	if _, err := os.Stat(a.BaseDir); err != nil {
		t.Error("removing beta must not touch alpha")
	}
}

func TestRegistry_TokenMintedPersistedAndStable(t *testing.T) {
	o := newFakeOpener()
	root := t.TempDir()
	reg, _ := tenant.New(root, o.open)
	now := time.Now()

	a, err := reg.Acquire("alpha", now)
	if err != nil {
		t.Fatal(err)
	}
	// Acquire mints a non-empty token and persists it inside the tenant's dir.
	if len(a.Token) < 32 {
		t.Fatalf("token looks unminted: %q", a.Token)
	}
	onDisk, err := os.ReadFile(filepath.Join(a.BaseDir, ".tenant-token"))
	if err != nil {
		t.Fatalf("token file: %v", err)
	}
	if strings.TrimSpace(string(onDisk)) != a.Token {
		t.Errorf("on-disk token %q != acquired %q", strings.TrimSpace(string(onDisk)), a.Token)
	}

	// Token(id) returns the same value without the kernel open (after Release).
	if _, err := reg.Release("alpha"); err != nil {
		t.Fatal(err)
	}
	got, err := reg.Token("alpha")
	if err != nil {
		t.Fatal(err)
	}
	if got != a.Token {
		t.Errorf("Token after release = %q, want stable %q", got, a.Token)
	}
	// Re-acquire keeps the SAME token (read, not re-minted).
	a2, _ := reg.Acquire("alpha", now)
	if a2.Token != a.Token {
		t.Errorf("re-acquired token = %q, want stable %q", a2.Token, a.Token)
	}

	// Distinct tenants get distinct tokens.
	b, _ := reg.Acquire("beta", now)
	if b.Token == a.Token {
		t.Error("alpha and beta share a token")
	}

	// Token of an unknown tenant errors and does NOT create it.
	if _, err := reg.Token("ghost"); err == nil {
		t.Error("Token(ghost) should error")
	}
	if reg.Exists("ghost") {
		t.Error("Token(ghost) must not materialise the tenant")
	}
}

func TestRegistry_Authorize(t *testing.T) {
	o := newFakeOpener()
	reg, _ := tenant.New(t.TempDir(), o.open)
	now := time.Now()
	a, _ := reg.Acquire("alpha", now)
	reg.Acquire("beta", now)

	if !reg.Authorize("alpha", a.Token) {
		t.Error("alpha's own token must authorize alpha")
	}
	bTok, _ := reg.Token("beta")
	if reg.Authorize("alpha", bTok) {
		t.Error("beta's token must NOT authorize alpha (cross-tenant)")
	}
	if reg.Authorize("alpha", "") {
		t.Error("blank token must never authorize")
	}
	if reg.Authorize("alpha", "deadbeef") {
		t.Error("a wrong token must not authorize")
	}
	if reg.Authorize("ghost", "anything") {
		t.Error("unknown tenant must not authorize")
	}
	// Removing a tenant destroys its token (no auth against a gone tenant).
	reg.Remove("beta")
	if reg.Authorize("beta", bTok) {
		t.Error("removed tenant's token must no longer authorize")
	}
}

func TestRegistry_ListReflectsDiskAndOpenState(t *testing.T) {
	o := newFakeOpener()
	reg, _ := tenant.New(t.TempDir(), o.open)
	now := time.Now()
	reg.Acquire("alpha", now)
	reg.Acquire("beta", now)
	reg.Release("beta") // on disk, not open

	list, err := reg.List()
	if err != nil {
		t.Fatal(err)
	}
	if len(list) != 2 {
		t.Fatalf("list = %d, want 2", len(list))
	}
	byID := map[string]tenant.Info{}
	for _, i := range list {
		byID[i.ID] = i
	}
	if !byID["alpha"].Open {
		t.Error("alpha should be open")
	}
	if byID["beta"].Open {
		t.Error("beta should be closed (released)")
	}
}

func TestRegistry_OpenErrorPropagates(t *testing.T) {
	reg, _ := tenant.New(t.TempDir(), func(id, baseDir string) (io.Closer, error) {
		return nil, fmt.Errorf("boom")
	})
	if _, err := reg.Acquire("alpha", time.Now()); err == nil {
		t.Error("Acquire should surface the opener error")
	}
	if reg.Count() != 0 {
		t.Error("a failed open must not register the tenant")
	}
}

func TestRegistry_CloseAll(t *testing.T) {
	o := newFakeOpener()
	reg, _ := tenant.New(t.TempDir(), o.open)
	now := time.Now()
	reg.Acquire("alpha", now)
	reg.Acquire("beta", now)
	if err := reg.CloseAll(); err != nil {
		t.Fatal(err)
	}
	if reg.Count() != 0 {
		t.Error("CloseAll should empty the live set")
	}
	for id, k := range o.kernels {
		if !k.closed {
			t.Errorf("kernel %q not closed by CloseAll", id)
		}
	}
}

// TestRegistry_RealKernelsAreIsolated proves the foundation end-to-end with real
// kernels: two tenants each run an intent through their own governed loop, and
// each tenant's journal contains only its own run — no cross-tenant bleed.
func TestRegistry_RealKernelsAreIsolated(t *testing.T) {
	kernels := map[string]*runtime.Kernel{}
	reg, err := tenant.New(t.TempDir(), func(id, baseDir string) (io.Closer, error) {
		k, err := runtime.Open(runtime.Config{
			BaseDir:  baseDir,
			Provider: mock.New(mock.FinalText("done-" + id)),
			Tools:    map[string]agent.Tool{},
		})
		if err == nil {
			kernels[id] = k
		}
		return k, err
	})
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { reg.CloseAll() })

	now := time.Now()
	if _, err := reg.Acquire("alpha", now); err != nil {
		t.Fatal(err)
	}
	if _, err := reg.Acquire("beta", now); err != nil {
		t.Fatal(err)
	}

	ctx := context.Background()
	if _, _, err := kernels["alpha"].Run(ctx, "alpha-secret-intent"); err != nil {
		t.Fatalf("alpha run: %v", err)
	}
	if _, _, err := kernels["beta"].Run(ctx, "beta-secret-intent"); err != nil {
		t.Fatalf("beta run: %v", err)
	}

	// Each tenant's journal is a separate file under its own base dir, and holds
	// only its own intent.
	alphaJournal := readAll(t, filepath.Join(kernels["alpha"].BaseDir(), "journal"))
	betaJournal := readAll(t, filepath.Join(kernels["beta"].BaseDir(), "journal"))
	if !strings.Contains(alphaJournal, "alpha-secret-intent") || strings.Contains(alphaJournal, "beta-secret-intent") {
		t.Error("alpha journal must contain only alpha's intent")
	}
	if !strings.Contains(betaJournal, "beta-secret-intent") || strings.Contains(betaJournal, "alpha-secret-intent") {
		t.Error("beta journal must contain only beta's intent")
	}
}

// readAll concatenates every regular file under dir (recursively) into one
// string — enough to assert which intents a tenant's journal recorded.
func readAll(t *testing.T, dir string) string {
	t.Helper()
	var b strings.Builder
	err := filepath.Walk(dir, func(path string, info os.FileInfo, err error) error {
		if err != nil {
			return err
		}
		if info.IsDir() {
			return nil
		}
		data, err := os.ReadFile(path)
		if err != nil {
			return err
		}
		b.Write(data)
		return nil
	})
	if err != nil {
		t.Fatalf("read journal dir %s: %v", dir, err)
	}
	return b.String()
}

// TestRegistry_HealsBlankTokenFile pins M474: a zero-length token file (left by a
// crash between the O_EXCL create and the write) must NOT wedge the tenant. Before
// the fix, every Token() re-read "" and the O_EXCL re-mint failed with IsExist, so
// the tenant returned an empty token forever.
func TestRegistry_HealsBlankTokenFile(t *testing.T) {
	o := newFakeOpener()
	reg, err := tenant.New(t.TempDir(), o.open)
	if err != nil {
		t.Fatal(err)
	}
	now := time.Unix(1780000000, 0)

	tn, err := reg.Acquire("alpha", now)
	if err != nil {
		t.Fatal(err)
	}
	if tn.Token == "" {
		t.Fatal("acquired token is empty")
	}

	// Simulate the crash artifact: truncate the token file to zero length.
	tokPath := filepath.Join(tn.BaseDir, ".tenant-token")
	if err := os.WriteFile(tokPath, nil, 0o600); err != nil {
		t.Fatal(err)
	}

	got, err := reg.Token("alpha")
	if err != nil {
		t.Fatalf("Token after blank file: %v", err)
	}
	if got == "" {
		t.Fatal("Token returned empty after a blank token file — tenant wedged (no self-heal)")
	}
	// The healed token is stable on subsequent reads.
	if again, _ := reg.Token("alpha"); again != got {
		t.Errorf("healed token not stable: %q vs %q", got, again)
	}
}
