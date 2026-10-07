package controlplane

import (
	"bufio"
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	appmarket "github.com/agezt/agezt/kernel/app/market"
	"github.com/agezt/agezt/kernel/contract/opapi"
	"github.com/agezt/agezt/kernel/market"
	"github.com/agezt/agezt/kernel/mcp"
	"github.com/agezt/agezt/kernel/runtime"
	"github.com/agezt/agezt/kernel/skill"
	"github.com/agezt/agezt/plugins/providers/mock"
	"io"
	"io/fs"
	"net"
	"net/http"
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"testing"
	"time"
)

type marketWriterLibrary struct{ pack market.Pack }

func (l marketWriterLibrary) Marketplaces() []market.Marketplace { return nil }
func (l marketWriterLibrary) ResolvePack(_, name, _ string) (market.Pack, error) {
	if name != l.pack.Name {
		return market.Pack{}, fmt.Errorf("owned missing pack: %s", name)
	}
	return l.pack, nil
}

type marketMaterializer struct {
	calls []string
	fail  bool
}

func (p *marketMaterializer) Create(corr string, spec skill.CreateSpec) (skill.Skill, bool, error) {
	p.calls = append(p.calls, "create:"+corr+":"+spec.Name)
	if p.fail {
		return skill.Skill{}, false, errors.New("owned create failure")
	}
	return skill.Skill{ID: "owned-skill", Status: skill.StatusDraft}, true, nil
}
func (p *marketMaterializer) Promote(corr, id string) (skill.Status, error) {
	p.calls = append(p.calls, "promote:"+corr+":"+id)
	return skill.StatusActive, nil
}
func (p *marketMaterializer) Quarantine(corr, id, reason string) error {
	p.calls = append(p.calls, "quarantine:"+corr+":"+id+":"+reason)
	if p.fail {
		return errors.New("owned quarantine failure")
	}
	return nil
}
func (p *marketMaterializer) AddMCPServer(corr string, srv mcp.Server) (mcp.Server, error) {
	p.calls = append(p.calls, "add-mcp:"+corr+":"+srv.Name)
	if p.fail {
		return mcp.Server{}, errors.New("owned mcp failure")
	}
	return srv, nil
}
func (p *marketMaterializer) RemoveMCPServer(corr, name string) (bool, error) {
	p.calls = append(p.calls, "remove-mcp:"+corr+":"+name)
	if p.fail {
		return false, errors.New("owned remove failure")
	}
	return true, nil
}

type marketMemoryTransport struct {
	index, pack []byte
	calls       []string
	fail        bool
}

func (p *marketMemoryTransport) RoundTrip(req *http.Request) (*http.Response, error) {
	p.calls = append(p.calls, req.URL.String())
	if err := req.Context().Err(); err != nil {
		return nil, err
	}
	if p.fail || req.URL.Host == "broken.owned" {
		return nil, errors.New("owned fetch failure")
	}
	raw := p.index
	if strings.HasSuffix(req.URL.Path, "pack.json") {
		raw = p.pack
	}
	return &http.Response{StatusCode: 200, Header: make(http.Header), Body: io.NopCloser(bytes.NewReader(raw)), Request: req}, nil
}
func marketStateBytes(t *testing.T, path string) map[string][]byte {
	t.Helper()
	out := map[string][]byte{}
	err := filepath.WalkDir(path, func(p string, d fs.DirEntry, err error) error {
		if os.IsNotExist(err) {
			return nil
		}
		if err != nil {
			return err
		}
		if !d.IsDir() {
			raw, err := os.ReadFile(p)
			if err != nil {
				return err
			}
			relative, _ := filepath.Rel(path, p)
			out[relative] = raw
		}
		return nil
	})
	if err != nil && !os.IsNotExist(err) {
		t.Fatal(err)
	}
	return out
}

func marketWriteCommands() []string {
	return []string{CmdMarketInstall, CmdMarketUninstall, CmdMarketAddSource, CmdMarketRemoveSource, CmdMarketSync}
}
func marketWriteFixture(t *testing.T, cmd string) (*runtime.Kernel, *Server, *marketMaterializer, *marketMemoryTransport, *mock.Provider, map[string]any, string) {
	t.Helper()
	dir := t.TempDir()
	p := mock.New()
	k, err := runtime.Open(runtime.Config{BaseDir: dir, Provider: p})
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { k.Close() })
	store := market.NewStore(dir)
	backend := &marketMaterializer{}
	good := "---\nname: owned\ndescription: owned fixture\n---\nRead the fixture.\n"
	pack := market.Pack{Name: "owned", Version: "1.0.0", Skills: []market.PackSkill{{SkillMD: good}}, MCPServers: []mcp.Server{{Name: "owned", Command: "never-executed"}}, ToolRequirements: []string{"owned-tool"}}
	raw, _ := json.Marshal(pack)
	index, _ := json.Marshal(market.Marketplace{Name: "owned", FormatVersion: 1, Packs: []market.MarketplaceEntry{pack.Entry("pack.json")}})
	transport := &marketMemoryTransport{index: index, pack: raw}
	if err := store.AddSource(market.Source{Name: "owned", URL: "http://remote.owned/index.json"}); err != nil {
		t.Fatal(err)
	}
	if cmd == CmdMarketUninstall {
		if err := store.RecordInstall(market.InstalledPack{Name: "owned", Version: "1.0.0", SkillIDs: []string{"owned-skill"}, MCPServers: []string{"owned"}}); err != nil {
			t.Fatal(err)
		}
	}
	k.SetMarket(market.NewManager(market.Config{Library: marketWriterLibrary{pack}, Store: store, Skills: backend, MCP: backend, Now: func() int64 { return 9007199254740993 }, Syncer: &market.Syncer{HTTP: &http.Client{Transport: transport}}}))
	s := NewServer(k, dir)
	s.token = "primary"
	args := map[string]any{"name": "owned", "url": "http://remote.owned/new.json", "correlation_id": "caller-poison", "unknown": true}
	if cmd == CmdMarketAddSource {
		args["name"] = "new"
	}
	return k, s, backend, transport, p, args, filepath.Join(dir, "market")
}
func marketWriteReply(t *testing.T, ctx context.Context, s *Server, cmd string, args map[string]any) Response {
	t.Helper()
	a, b := net.Pipe()
	defer a.Close()
	defer b.Close()
	a.SetDeadline(time.Now().Add(3 * time.Second))
	done := make(chan struct{})
	go func() { defer close(done); s.handleConn(ctx, b) }()
	raw, _ := json.Marshal(Request{ID: "owned", Cmd: cmd, Token: "primary", Args: args})
	if _, err := a.Write(append(raw, 10)); err != nil {
		t.Fatal(err)
	}
	reader := bufio.NewReader(a)
	var reply Response
	for {
		line, err := reader.ReadBytes(10)
		if err != nil {
			t.Fatal(err)
		}
		if err := json.Unmarshal(line, &reply); err != nil {
			t.Fatal(err)
		}
		if reply.Type == RespResult || reply.Type == RespError {
			break
		}
	}
	a.Close()
	<-done
	return reply
}
func TestMarketWriterNativeAuditAdmissionBlocksAllFiveEffects(t *testing.T) {
	for _, cmd := range marketWriteCommands() {
		t.Run(cmd, func(t *testing.T) {
			k, s, b, h, p, args, path := marketWriteFixture(t, cmd)
			before := marketStateBytes(t, path)
			if err := k.Journal().Close(); err != nil {
				t.Fatal(err)
			}
			reply := marketWriteReply(t, context.Background(), s, cmd, args)
			if reply.Type != RespError || !strings.Contains(reply.Error, "journal") || !reflect.DeepEqual(before, marketStateBytes(t, path)) || len(b.calls) != 0 || len(h.calls) != 0 || p.CallCount() != 0 {
				t.Fatalf("EXPECTED:audit admission blocks effects ACTUAL:reply=%+v effects=%v fetch=%v", reply, b.calls, h.calls)
			}
		})
	}
}
func TestMarketWriterNativeCanceledAdmissionBlocksAllFiveEffects(t *testing.T) {
	for _, cmd := range marketWriteCommands() {
		t.Run(cmd, func(t *testing.T) {
			k, s, b, h, p, args, path := marketWriteFixture(t, cmd)
			before := marketStateBytes(t, path)
			head, hash := k.Journal().Head()
			ctx, cancel := context.WithCancel(context.Background())
			cancel()
			reply := marketWriteReply(t, ctx, s, cmd, args)
			after, afterHash := k.Journal().Head()
			if reply.Type != RespError || !strings.Contains(reply.Error, "context canceled") || !reflect.DeepEqual(before, marketStateBytes(t, path)) || len(b.calls) != 0 || len(h.calls) != 0 || p.CallCount() != 0 || head != after || hash != afterHash {
				t.Fatalf("EXPECTED:canceled admission before all effects/audit ACTUAL:reply=%+v effects=%v fetch=%v seq=%d/%d", reply, b.calls, h.calls, head, after)
			}
		})
	}
}
func TestMarketWriterNativeOneOwnedDomainCorrelation(t *testing.T) {
	for _, cmd := range marketWriteCommands() {
		t.Run(cmd, func(t *testing.T) {
			k, s, b, _, p, args, _ := marketWriteFixture(t, cmd)
			head, _ := k.Journal().Head()
			reply := marketWriteReply(t, context.Background(), s, cmd, args)
			if reply.Type != RespResult {
				t.Fatal(reply)
			}
			rows, _ := k.Journal().Tail(100)
			corr := ""
			domain := 0
			for _, e := range rows {
				if e.Seq <= head {
					continue
				}
				if corr == "" {
					corr = e.CorrelationID
				}
				if e.CorrelationID == "" || e.CorrelationID == "caller-poison" || e.CorrelationID != corr {
					t.Fatalf("EXPECTED:one owned audit/domain identity ACTUAL:kind=%s corr=%q owned=%q", e.Kind, e.CorrelationID, corr)
				}
				if strings.HasPrefix(string(e.Kind), "market.") {
					domain++
				}
			}
			if domain != 1 || p.CallCount() != 0 {
				t.Fatal(domain, p.CallCount())
			}
			for _, call := range b.calls {
				if strings.Contains(call, "caller-poison") {
					t.Fatal("caller correlation reached materializer", call)
				}
			}
		})
	}
}

func TestMarketWriterNativeBrokenProgressStopsLaterEffects(t *testing.T) {
	for _, cmd := range []string{CmdMarketInstall, CmdMarketUninstall} {
		t.Run(cmd, func(t *testing.T) {
			k, s, b, _, p, args, path := marketWriteFixture(t, cmd)
			before := marketStateBytes(t, path)
			head, _ := k.Journal().Head()
			conn := newToolboxRecordingConn(Request{ID: "owned", Cmd: cmd, Token: "primary", Args: args})
			conn.failure = errors.New("owned market progress write failure")
			s.handleConn(context.Background(), conn)
			want := 0
			if cmd == CmdMarketUninstall {
				want = 1
			}
			rows, _ := k.Journal().Tail(100)
			failed, completed, domain := 0, 0, 0
			for _, e := range rows {
				if e.Seq <= head {
					continue
				}
				switch string(e.Kind) {
				case "op.failed":
					failed++
				case "op.completed":
					completed++
				}
				if strings.HasPrefix(string(e.Kind), "market.") {
					domain++
				}
			}
			if len(b.calls) != want || !reflect.DeepEqual(before, marketStateBytes(t, path)) || failed != 1 || completed != 0 || domain != 0 || p.CallCount() != 0 {
				t.Fatalf("EXPECTED:stream error stops subsequent effects ACTUAL:cmd=%s effects=%v failed=%d completed=%d domain=%d", cmd, b.calls, failed, completed, domain)
			}
		})
	}
}

func TestMarketWriterNativeFiveTypedPoliciesAndSignatures(t *testing.T) {
	inputs := map[string]reflect.Type{CmdMarketInstall: reflect.TypeFor[appmarket.InstallRequest](), CmdMarketUninstall: reflect.TypeFor[appmarket.WriteNameRequest](), CmdMarketAddSource: reflect.TypeFor[appmarket.AddSourceRequest](), CmdMarketRemoveSource: reflect.TypeFor[appmarket.WriteNameRequest](), CmdMarketSync: reflect.TypeFor[appmarket.WriteNameRequest]()}
	seen := map[string]int{}
	for _, op := range registeredAppOperations() {
		sp := op.Spec()
		input, ok := inputs[sp.Name]
		if !ok {
			continue
		}
		wire, ok := commandRegistry[sp.Name]
		stream := opapi.StreamNone
		if sp.Name == CmdMarketInstall || sp.Name == CmdMarketUninstall {
			stream = opapi.StreamEvents
		}
		if !ok || !wire.AppOwned || wire.ReadOnly || wire.TenantAllowed || wire.TenantRouted || wire.Streaming != StreamMode(stream) || sp.ReadOnly || sp.Authz != opapi.PrimaryOnly || sp.Tenancy != opapi.Primary || sp.Stream != stream || !sp.AllowUnknownInput || sp.Input != input {
			t.Fatal(sp, wire)
		}
		seen[sp.Name]++
	}
	if len(marketWriteOperations) != 5 || len(seen) != 5 {
		t.Fatal(seen)
	}
	for _, count := range seen {
		if count != 1 {
			t.Fatal(seen)
		}
	}
}
func TestMarketWriterNativeClosedPublicationAfterEffectsReturnsFailure(t *testing.T) {
	k, s, b, _, p, args, path := marketWriteFixture(t, CmdMarketInstall)
	before := marketStateBytes(t, path)
	conn := newToolboxRecordingConn(Request{ID: "owned", Cmd: CmdMarketInstall, Token: "primary", Args: args})
	closed := false
	conn.afterWrite = func() {
		if !closed {
			closed = true
			if err := k.Journal().Close(); err != nil {
				t.Fatal(err)
			}
		}
	}
	s.handleConn(context.Background(), conn)
	lines := bytes.Split(bytes.TrimSpace(conn.output.Bytes()), []byte{10})
	var reply Response
	if len(lines) == 0 {
		t.Fatal("missing response")
	}
	if err := json.Unmarshal(lines[len(lines)-1], &reply); err != nil {
		t.Fatal(err)
	}
	if reply.Type != RespError || !strings.Contains(reply.Error, "journal") || len(b.calls) != 3 || reflect.DeepEqual(before, marketStateBytes(t, path)) || p.CallCount() != 0 {
		t.Fatal(reply, b.calls, closed)
	}
}
