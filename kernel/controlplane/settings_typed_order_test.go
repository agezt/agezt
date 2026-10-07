// SPDX-License-Identifier: MIT
package controlplane

import (
	"bufio"
	"bytes"
	"context"
	"encoding/json"
	"github.com/agezt/agezt/kernel/runtime"
	"github.com/agezt/agezt/kernel/settings"
	"github.com/agezt/agezt/plugins/providers/mock"
	"net"
	"testing"
	"time"
)

func TestSettingsTypedNativeSchemaPreservesLegacyStructMemberOrder(t *testing.T) {
	dir := t.TempDir()
	p := mock.New()
	k, err := runtime.Open(runtime.Config{BaseDir: dir, Provider: p})
	if err != nil {
		t.Fatal(err)
	}
	defer k.Close()
	s := NewServer(k, dir)
	s.token = "primary"
	sections := settings.NewRegistry(dir).Sections()
	want, _ := json.Marshal(Response{ID: "owned", Type: RespResult, Result: map[string]any{"sections": sections, "reload_boundaries": settings.ReloadBoundaries(sections)}})
	a, b := net.Pipe()
	defer a.Close()
	defer b.Close()
	a.SetDeadline(time.Now().Add(3 * time.Second))
	done := make(chan struct{})
	go func() { defer close(done); s.handleConn(context.Background(), b) }()
	raw, _ := json.Marshal(Request{ID: "owned", Cmd: CmdConfigSchema, Token: "primary"})
	a.Write(append(raw, 10))
	line, err := bufio.NewReader(a).ReadBytes(10)
	if err != nil {
		t.Fatal(err)
	}
	a.Close()
	<-done
	if !bytes.Equal(line, append(want, 10)) {
		t.Fatal("legacy section/field JSON member order changed")
	}
}
