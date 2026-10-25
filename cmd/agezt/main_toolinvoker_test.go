// SPDX-License-Identifier: MIT

package main

import (
	"context"
	"encoding/json"
	"sort"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	"github.com/agezt/agezt/kernel/contract/artifactapi"
	"github.com/agezt/agezt/kernel/contract/toolapi"
	"github.com/agezt/agezt/kernel/edict"
	"github.com/agezt/agezt/kernel/event"
	"github.com/agezt/agezt/kernel/modules/agents/roster"
	kernelruntime "github.com/agezt/agezt/kernel/runtime"
	"github.com/agezt/agezt/kernel/toolexec"
	"github.com/agezt/agezt/plugins/providers/mock"
)

type appBootTool struct{ calls atomic.Int32 }

func (*appBootTool) Definition() toolapi.ToolDef {
	return toolapi.ToolDef{Name: "probe", InputSchema: json.RawMessage(`{"type":"object"}`), Capability: toolapi.ToolCapability{Name: "introspect"}}
}
func (t *appBootTool) Invoke(context.Context, json.RawMessage) (toolapi.Result, error) {
	t.calls.Add(1)
	return toolapi.Result{Output: "ok"}, nil
}

func TestOpenAppKernelBindsPrimaryAndTenant(t *testing.T) {
	var unusedFactory atomic.Int32
	tool := &appBootTool{}
	cfg := kernelruntime.Config{BaseDir: t.TempDir(), Provider: mock.New(), Tools: map[string]toolapi.Tool{"probe": tool}, NewToolInvoker: func(toolexec.Dependencies) toolapi.Invoker { unusedFactory.Add(1); return nil }}
	primary, err := openAppKernel(cfg)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { primary.Close() })
	cfg.BaseDir = t.TempDir()
	cfg.TenantID = "tenant"
	tenant, err := openAppKernel(cfg)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { tenant.Close() })
	tenant.Edict().SetLevel(edict.CapIntrospect, edict.LevelDeny)
	if res, err := primary.RunTool(context.Background(), "same", "call", "probe", json.RawMessage(`{}`)); err != nil || res.Output != "ok" {
		t.Fatalf("primary result=%+v error=%v", res, err)
	}
	if _, err := tenant.RunTool(context.Background(), "same", "call", "probe", json.RawMessage(`{}`)); err == nil {
		t.Fatal("tenant borrowed primary allow")
	}
	if unusedFactory.Load() != 0 || tool.calls.Load() != 1 {
		t.Fatalf("factory=%d backend=%d", unusedFactory.Load(), tool.calls.Load())
	}
	for name, k := range map[string]*kernelruntime.Kernel{"primary": primary, "tenant": tenant} {
		found := 0
		if err := k.Journal().Range(func(e *event.Event) error {
			if e.Kind != event.KindPolicyDecision || e.CorrelationID != "same" {
				return nil
			}
			found++
			var p struct{ Allow bool }
			if err := json.Unmarshal(e.Payload, &p); err != nil {
				return err
			}
			if p.Allow != (name == "primary") {
				t.Errorf("%s policy=%s", name, e.Payload)
			}
			return nil
		}); err != nil {
			t.Fatal(err)
		}
		if found != 1 {
			t.Errorf("%s decisions=%d", name, found)
		}
	}
}

// TestOpenAppKernelWiresTheSeatCatalog: the daemon IS the composition root —
// it opens the seat catalog (the file is the daemon's business) and hands the
// kernel the contract's port, so the daemon's seat operations are a real
// catalog and not an unavailable one.
//
// The other end of the rule is the host's: a kernel that was given no catalog
// says "not available" instead of calling into nothing.
func TestOpenAppKernelWiresTheSeatCatalog(t *testing.T) {
	k, err := openAppKernel(kernelruntime.Config{BaseDir: t.TempDir(), Provider: mock.New(), Tools: map[string]toolapi.Tool{}})
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { k.Close() })
	store := k.Seats()
	if store == nil {
		t.Fatal("the daemon did not wire the seat catalog — the kernel holds no port")
	}
	if !store.Valid("reader") {
		t.Error("the daemon's seat catalog does not know the built-in seats")
	}
	if store.Valid("no-such-seat") {
		t.Error("the daemon's seat catalog accepts a seat that does not exist")
	}
}

// TestOpenAppKernelWiresTheDataLake: the daemon IS the composition root —
// it opens the data lake, seeds its built-in collections and hands the kernel
// the contract's port.
//
// The seeding lives HERE (W3.42): it is not part of the port the host is
// given, and it is the daemon's file that is seeded. A daemon that stopped
// seeding would leave every live box without the built-in collections —
// which is why the assertion is on the composition root's lake.
func TestOpenAppKernelWiresTheDataLake(t *testing.T) {
	k, err := openAppKernel(kernelruntime.Config{BaseDir: t.TempDir(), Provider: mock.New(), Tools: map[string]toolapi.Tool{}})
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { k.Close() })
	lake := k.DataLake()
	if lake == nil {
		t.Fatal("the daemon did not wire the data lake — the kernel holds no port")
	}
	all := lake.ListCollections()
	if len(all) == 0 {
		t.Fatal("the daemon's data lake carries no built-in collections")
	}
	names := make([]string, 0, len(all))
	for _, c := range all {
		names = append(names, c.Name)
	}
	sort.Strings(names)
	if names[0] != "bookmarks" {
		t.Fatalf("the daemon did not seed the built-in collections, got %v", names)
	}
}

// TestOpenAppKernelWiresTheArtifactStore: the daemon IS the composition root
// — it opens the content-addressed artifact store and the metadata index over
// it, and hands the kernel the contract's ports (W3.43).
//
// The two are a PAIR, and the assertion is on the pair: an entry the index
// holds must point at bytes the blob store holds. A daemon that wired two
// unrelated stores would pass a "both ports are non-nil" check and still lose
// every artifact it recorded.
func TestOpenAppKernelWiresTheArtifactStore(t *testing.T) {
	k, err := openAppKernel(kernelruntime.Config{BaseDir: t.TempDir(), Provider: mock.New(), Tools: map[string]toolapi.Tool{}})
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { k.Close() })

	store := k.Artifacts()
	if store == nil {
		t.Fatal("the daemon did not wire the artifact store — the kernel holds no port")
	}
	idx := k.ArtifactIndex()
	if idx == nil {
		t.Fatal("the daemon did not wire the artifact index — the kernel holds no port")
	}

	data := []byte("the daemon's own artifact")
	entry, err := idx.PutEntry(artifactapi.Entry{Kind: "file", Source: "run", Name: "note.txt"}, data, 1)
	if err != nil {
		t.Fatalf("PutEntry: %v", err)
	}
	got, err := store.Get(entry.Ref)
	if err != nil {
		t.Fatalf("the index and the blob store are not a pair: %v", err)
	}
	if string(got) != string(data) {
		t.Fatalf("the blob came back as %q, want %q", got, data)
	}
}

// TestOpenAppKernelWiresTheMediaTools: the voice, image and rerank tools are
// the DAEMON's (W3.44) — the kernel is a module and may not name a tool
// package, so the composition root builds them over the daemon's own adapter
// seams and hands them over as ordinary tools.
//
// The daemon end: the three tools are live in the kernel's tool set, and the
// one that persists bytes writes into the DAEMON's artifact store — the same
// store the kernel holds, so a synthesized reply lands where the kernel's own
// offload path would put it.
func TestOpenAppKernelWiresTheMediaTools(t *testing.T) {
	callerTools := map[string]toolapi.Tool{}
	k, err := openAppKernel(kernelruntime.Config{
		BaseDir:        t.TempDir(),
		Provider:       mock.New(),
		Tools:          callerTools,
		Voice:          daemonStubVoice{},
		ImageGenerator: daemonStubImageGen{},
		Reranker:       daemonStubReranker{},
	})
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { k.Close() })

	// The caller's map must come back UNTOUCHED: the primary kernel and every
	// tenant share one map, and each kernel must see its own tools bound to its
	// own artifact store. A map written in place would hand the last kernel's
	// tools to all of them.
	if _, ok := callerTools["voice"]; ok {
		t.Fatal("the daemon wrote into the caller's tool map")
	}

	tools := k.Tools()
	for _, name := range []string{"voice", "image_generate", "rerank"} {
		if _, ok := tools[name]; !ok {
			t.Fatalf("the daemon did not wire the %q tool", name)
		}
	}

	// The voice tool's artifact sink is the daemon's store: the ref it reports
	// must resolve through the kernel's own blob port.
	res, err := tools["voice"].Invoke(context.Background(), json.RawMessage(`{"op":"speak","text":"hello"}`))
	if err != nil {
		t.Fatalf("Invoke: %v", err)
	}
	if res.IsError {
		t.Fatalf("the voice tool errored: %s", res.Output)
	}
	fields := strings.Fields(res.Output)
	ref := fields[len(fields)-1]
	got, err := k.Artifacts().Get(ref)
	if err != nil {
		t.Fatalf("the voice tool wrote somewhere else than the daemon's store: %v", err)
	}
	if string(got) != "AUDIO" {
		t.Fatalf("the artifact holds %q, want %q", got, "AUDIO")
	}
}

// daemonStubVoice / daemonStubImageGen / daemonStubReranker are the daemon's
// adapter seams, stubbed: the tools are what the daemon builds over them, and
// the stubs answer enough for the tools to be real.
type daemonStubVoice struct{}

func (daemonStubVoice) Transcribe(context.Context, []byte, string) (string, error) {
	return "transcribed", nil
}
func (daemonStubVoice) Speak(context.Context, string) ([]byte, string, error) {
	return []byte("AUDIO"), "audio/ogg", nil
}
func (daemonStubVoice) HasSTT() bool { return true }
func (daemonStubVoice) HasTTS() bool { return true }

type daemonStubImageGen struct{}

func (daemonStubImageGen) GenerateImage(context.Context, string, string, string, int) ([][]byte, string, error) {
	return [][]byte{[]byte("PNG")}, "image/png", nil
}
func (daemonStubImageGen) HasImage() bool { return true }

type daemonStubReranker struct{}

func (daemonStubReranker) Rerank(context.Context, string, []string, int) ([]int, []float64, error) {
	return []int{0}, []float64{1}, nil
}
func (daemonStubReranker) HasRerank() bool { return true }

// TestOpenAppKernelWiresTheReaper: the daemon IS the composition root — it
// builds the agents module's reaper over the kernel's own ports and injects
// it, so the daemon's scan is the module's scan and not an empty report.
func TestOpenAppKernelWiresTheReaper(t *testing.T) {
	k, err := openAppKernel(kernelruntime.Config{BaseDir: t.TempDir(), Provider: mock.New(), Tools: map[string]toolapi.Tool{}})
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { k.Close() })
	if _, err := k.Roster().Add(roster.Profile{Slug: "dormant"}); err != nil {
		t.Fatal(err)
	}
	future := time.Now().Add(time.Hour).UnixMilli()
	rep := k.ReaperScan(future, future)
	if len(rep.DeadAgents) != 1 || rep.DeadAgents[0].Slug != "dormant" {
		t.Fatalf("the daemon's reaper did not answer: %+v", rep.DeadAgents)
	}
}
