// SPDX-License-Identifier: MIT
package controlplane

import (
	"bufio"
	"context"
	"encoding/json"
	"net"
	"reflect"
	"strings"
	"testing"
	"time"

	"github.com/agezt/agezt/kernel/app"
	appchannels "github.com/agezt/agezt/kernel/app/channels"
	"github.com/agezt/agezt/kernel/channel"
	"github.com/agezt/agezt/kernel/contract/opapi"
	"github.com/agezt/agezt/kernel/runtime"
	"github.com/agezt/agezt/kernel/settings"
	"github.com/agezt/agezt/plugins/providers/mock"
)

func TestChannelInventoryNativeTypedMetadata(t *testing.T) {
	if len(channelInventoryOperations) != 1 {
		t.Fatal(channelInventoryOperations)
	}
	spec := channelInventoryOperations[0].Spec()
	wire := commandRegistry[CmdChannelList]
	if !wire.AppOwned || !wire.ReadOnly || wire.TenantAllowed || wire.TenantRouted || wire.Streaming != StreamNone || spec.Input != reflect.TypeFor[appchannels.ListInput]() || spec.Output != reflect.TypeFor[appchannels.ListOutput]() || spec.Authz != opapi.PrimaryOnly || spec.Tenancy != opapi.Primary || spec.HTTP.Method != "GET" || spec.HTTP.Path != "/api/channels" {
		t.Fatal(spec, wire)
	}
}

type ownedMediaInventoryReader struct{}

func (r ownedMediaInventoryReader) Prepare() appchannels.InventoryValues { return r }
func (ownedMediaInventoryReader) Manifests() []channel.Manifest {
	return []channel.Manifest{{Kind: "owned", Media: channel.MediaCaps{ImageIn: true, VoiceIn: true, ImageOut: true, VoiceOut: true}}}
}
func (ownedMediaInventoryReader) IsLive(string) bool                { return false }
func (ownedMediaInventoryReader) IsLiveInstance(string) bool        { return false }
func (ownedMediaInventoryReader) Sections() []settings.Section      { return nil }
func (ownedMediaInventoryReader) EnvValue(string) string            { return "" }
func (ownedMediaInventoryReader) SecretSet(string) bool             { return false }
func (ownedMediaInventoryReader) StoredValue(string) (string, bool) { return "", false }
func (ownedMediaInventoryReader) EnvPinned(string) bool             { return false }
func (ownedMediaInventoryReader) Names() []string                   { return nil }

func TestChannelInventoryNativeMediaMemberOrder(t *testing.T) {
	dir := t.TempDir()
	k, err := runtime.Open(runtime.Config{BaseDir: dir, Provider: mock.New()})
	if err != nil {
		t.Fatal(err)
	}
	defer k.Close()
	s := NewServer(k, dir)
	s.token = "primary"
	ops, err := appchannels.InventoryOperations(func(context.Context) *appchannels.Inventory {
		return appchannels.NewInventory(ownedMediaInventoryReader{})
	})
	if err != nil {
		t.Fatal(err)
	}
	s.operationOnce.Do(func() {
		s.operations, s.operationErr = app.NewDispatcher(ops, app.Dependencies{Auth: appAuthenticator{s}, Router: appTenantRouter{s}})
	})
	a, b := net.Pipe()
	a.SetDeadline(time.Now().Add(3 * time.Second))
	done := make(chan struct{})
	go func() { defer close(done); s.handleConn(context.Background(), b) }()
	raw, _ := json.Marshal(Request{ID: "owned", Cmd: CmdChannelList, Token: "primary"})
	if _, err := a.Write(append(raw, 10)); err != nil {
		t.Fatal(err)
	}
	line, err := bufio.NewReader(a).ReadBytes(10)
	a.Close()
	<-done
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(string(line), `"media":{"image_in":true,"voice_in":true,"image_out":true,"voice_out":true}`) {
		t.Fatal("legacy MediaCaps declaration order lost", string(line))
	}
}

type ownedInventoryGateReader struct{ calls int }

func (r *ownedInventoryGateReader) Prepare() appchannels.InventoryValues {
	r.calls++
	panic("canceled request reached owned reader")
}
func (*ownedInventoryGateReader) Manifests() []channel.Manifest { return nil }
func (*ownedInventoryGateReader) IsLive(string) bool            { return false }
func (*ownedInventoryGateReader) IsLiveInstance(string) bool    { return false }

func TestChannelInventoryNativeCanceledAdmissionBeforeSelectedReader(t *testing.T) {
	p := mock.New()
	dir := t.TempDir()
	k, err := runtime.Open(runtime.Config{BaseDir: dir, Provider: p})
	if err != nil {
		t.Fatal(err)
	}
	defer k.Close()
	s := NewServer(k, dir)
	s.token = "primary"
	reader := &ownedInventoryGateReader{}
	providerCalls := 0
	ops, err := appchannels.InventoryOperations(func(context.Context) *appchannels.Inventory { providerCalls++; return appchannels.NewInventory(reader) })
	if err != nil {
		t.Fatal(err)
	}
	s.operationOnce.Do(func() {
		s.operations, s.operationErr = app.NewDispatcher(ops, app.Dependencies{Auth: appAuthenticator{s}, Router: appTenantRouter{s}})
	})
	head, hash := k.Journal().Head()
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	a, b := net.Pipe()
	a.SetDeadline(time.Now().Add(3 * time.Second))
	done := make(chan struct{})
	go func() { defer close(done); s.handleConn(ctx, b) }()
	raw, _ := json.Marshal(Request{ID: "owned", Cmd: CmdChannelList, Token: "primary", Args: map[string]any{"unknown": true}})
	if _, err := a.Write(append(raw, 10)); err != nil {
		t.Fatal(err)
	}
	line, err := bufio.NewReader(a).ReadBytes(10)
	a.Close()
	<-done
	if err != nil {
		t.Fatal(err)
	}
	var reply Response
	if json.Unmarshal(line, &reply) != nil || reply.Type != RespError || reply.Error != "context canceled" || providerCalls != 0 || reader.calls != 0 {
		t.Fatal(string(line), providerCalls, reader.calls)
	}
	after, afterHash := k.Journal().Head()
	if head != after || hash != afterHash || p.CallCount() != 0 {
		t.Fatal("canceled inventory wrote audit/provider")
	}
}
