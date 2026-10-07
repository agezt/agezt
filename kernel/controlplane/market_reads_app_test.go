// SPDX-License-Identifier: MIT
package controlplane

import (
	"bufio"
	"context"
	"encoding/json"
	"fmt"
	appmarket "github.com/agezt/agezt/kernel/app/market"
	"github.com/agezt/agezt/kernel/contract/opapi"
	"github.com/agezt/agezt/kernel/market"
	"github.com/agezt/agezt/kernel/mcp"
	"github.com/agezt/agezt/kernel/runtime"
	"github.com/agezt/agezt/plugins/providers/mock"
	"net"
	"reflect"
	"strings"
	"testing"
	"time"
)

type marketTypedLibrary struct {
	pack  market.Pack
	entry market.MarketplaceEntry
}

func (l marketTypedLibrary) Marketplaces() []market.Marketplace {
	return []market.Marketplace{{Name: "owned", Builtin: true, Packs: []market.MarketplaceEntry{l.entry}}}
}
func (l marketTypedLibrary) ResolvePack(_, name, _ string) (market.Pack, error) {
	if name != l.pack.Name {
		return market.Pack{}, fmt.Errorf("owned pack not found: %s", name)
	}
	return l.pack, nil
}
func marketTypedFixture(t *testing.T) (*runtime.Kernel, *Server, *mock.Provider) {
	t.Helper()
	dir := t.TempDir()
	p := mock.New()
	k, err := runtime.Open(runtime.Config{BaseDir: dir, Provider: p})
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { k.Close() })
	store := market.NewStore(dir)
	if err := store.AddSource(market.Source{Name: "owned", URL: "http://127.0.0.1:1/owned", AddedMS: 9223372036854775807}); err != nil {
		t.Fatal(err)
	}
	if err := store.RecordInstall(market.InstalledPack{Name: "owned", Version: "1.0.0", InstalledMS: 9223372036854775807}); err != nil {
		t.Fatal(err)
	}
	pack := market.Pack{Name: "owned", Version: "1.0.0", Signature: &market.Signature{SignedAt: 9007199254740993}, MCPServers: []mcp.Server{{Name: "owned", Command: "never-executed", CreatedMS: 9007199254740993, UpdatedMS: 9223372036854775807}}}
	entry := pack.Entry("")
	entry.Downloads = 9007199254740993
	k.SetMarket(market.NewManager(market.Config{Library: marketTypedLibrary{pack: pack, entry: entry}, Store: store}))
	s := NewServer(k, dir)
	s.token = "primary"
	return k, s, p
}
func marketTypedReply(t *testing.T, ctx context.Context, s *Server, cmd string) map[string]any {
	t.Helper()
	a, b := net.Pipe()
	defer a.Close()
	defer b.Close()
	a.SetDeadline(time.Now().Add(3 * time.Second))
	done := make(chan struct{})
	go func() { defer close(done); s.handleConn(ctx, b) }()
	raw, _ := json.Marshal(Request{ID: "owned", Cmd: cmd, Token: "primary", Args: map[string]any{"name": "owned", "unknown": true, "tenant": "spoof"}})
	if _, err := a.Write(append(raw, 10)); err != nil {
		t.Fatal(err)
	}
	line, err := bufio.NewReader(a).ReadBytes(10)
	if err != nil {
		t.Fatal(err)
	}
	a.Close()
	<-done
	decoder := json.NewDecoder(strings.NewReader(string(line)))
	decoder.UseNumber()
	var reply map[string]any
	if err := decoder.Decode(&reply); err != nil {
		t.Fatal(err)
	}
	return reply
}
func TestMarketTypedNativeLargeIntegersRemainExact(t *testing.T) {
	k, s, p := marketTypedFixture(t)
	head, hash := k.Journal().Head()
	for _, tc := range []struct{ name, cmd, want string }{{"downloads", CmdMarketList, "9007199254740993"}, {"added_ms", CmdMarketSources, "9223372036854775807"}, {"signed_at", CmdMarketShow, "9007199254740993"}, {"created_ms", CmdMarketShow, "9007199254740993"}, {"updated_ms", CmdMarketShow, "9223372036854775807"}, {"installed_at", CmdMarketShow, "9223372036854775807"}} {
		t.Run(tc.name, func(t *testing.T) {
			reply := marketTypedReply(t, context.Background(), s, tc.cmd)
			if reply["type"] != RespResult {
				t.Fatal(reply)
			}
			result := reply["result"].(map[string]any)
			var value any
			switch tc.name {
			case "downloads":
				value = result["packs"].([]any)[0].(map[string]any)["downloads"]
			case "added_ms":
				value = result["sources"].([]any)[0].(map[string]any)["added_ms"]
			case "signed_at":
				value = result["pack"].(map[string]any)["signature"].(map[string]any)["signed_at"]
			case "created_ms", "updated_ms":
				value = result["pack"].(map[string]any)["mcp_servers"].([]any)[0].(map[string]any)[tc.name]
			case "installed_at":
				value = result[tc.name]
			}
			actual, ok := value.(json.Number)
			if !ok || actual.String() != tc.want {
				t.Fatalf("EXPECTED:exact %s=%s ACTUAL:%v", tc.name, tc.want, value)
			}
		})
	}
	after, afterHash := k.Journal().Head()
	if head != after || hash != afterHash || p.CallCount() != 0 || len(k.MCPAttached()) != 0 {
		t.Fatal(head, after, p.CallCount())
	}
}
func TestMarketTypedNativeCanceledReadAdmission(t *testing.T) {
	for _, cmd := range []string{CmdMarketList, CmdMarketShow, CmdMarketSources} {
		t.Run(cmd, func(t *testing.T) {
			k, s, p := marketTypedFixture(t)
			head, hash := k.Journal().Head()
			ctx, cancel := context.WithCancel(context.Background())
			cancel()
			reply := marketTypedReply(t, ctx, s, cmd)
			message, _ := reply["error"].(string)
			after, afterHash := k.Journal().Head()
			if reply["type"] != RespError || !strings.Contains(message, "context canceled") || head != after || hash != afterHash || p.CallCount() != 0 {
				t.Fatalf("EXPECTED:canceled admission before reads ACTUAL:type=%v error=%q", reply["type"], message)
			}
		})
	}
}

func TestMarketTypedNativeThreeReadPoliciesAndTypes(t *testing.T) {
	type signature struct{ input, output reflect.Type }
	expected := map[string]signature{CmdMarketList: {reflect.TypeFor[appmarket.ListRequest](), reflect.TypeFor[appmarket.ListOutput]()}, CmdMarketShow: {reflect.TypeFor[appmarket.ShowRequest](), reflect.TypeFor[appmarket.ShowOutput]()}, CmdMarketSources: {reflect.TypeFor[appmarket.SourcesInput](), reflect.TypeFor[appmarket.SourcesOutput]()}}
	seen := map[string]int{}
	for _, operation := range registeredAppOperations() {
		spec := operation.Spec()
		sig, ok := expected[spec.Name]
		if !ok {
			continue
		}
		wire, ok := commandRegistry[spec.Name]
		if !ok || !wire.AppOwned || !wire.ReadOnly || wire.Streaming != StreamNone || wire.TenantAllowed || wire.TenantRouted || !spec.ReadOnly || spec.Authz != opapi.PrimaryOnly || spec.Tenancy != opapi.Primary || spec.Stream != opapi.StreamNone || !spec.AllowUnknownInput || spec.Input != sig.input || spec.Output != sig.output {
			t.Fatal(spec, wire)
		}
		seen[spec.Name]++
	}
	if len(seen) != 3 || len(marketReadOperations) != 3 {
		t.Fatal(seen)
	}
	for _, count := range seen {
		if count != 1 {
			t.Fatal(seen)
		}
	}
}
