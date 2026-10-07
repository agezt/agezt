// SPDX-License-Identifier: MIT
package controlplane

import (
	"bufio"
	"bytes"
	"context"
	"encoding/json"
	"net"
	"testing"
	"time"

	core "github.com/agezt/agezt/kernel/configcenter"
	"github.com/agezt/agezt/kernel/runtime"
	"github.com/agezt/agezt/plugins/providers/mock"
)

func TestConfigCenterTypedNativeLargeIntegerLexemesAndZeroFields(t *testing.T) {
	dir := t.TempDir()
	p := mock.New()
	k, err := runtime.Open(runtime.Config{BaseDir: dir, Provider: p})
	if err != nil {
		t.Fatal(err)
	}
	defer k.Close()
	e := core.NewConfigEntry("owned", "raw")
	if err := k.ConfigCenter().Set(e); err != nil {
		t.Fatal(err)
	}
	// Admin GetEntry exposes the stored pointer, allowing an exact wide-integer fixture.
	e.CreatedAt = 9007199254740993
	e.UpdatedAt = 0
	e.Version = -2
	head, hash := k.Journal().Head()
	s := NewServer(k, dir)
	s.token = "primary"
	for _, command := range []string{CmdConfigCenterGet, CmdConfigCenterList} {
		a, b := net.Pipe()
		a.SetDeadline(time.Now().Add(3 * time.Second))
		done := make(chan struct{})
		go func() { defer close(done); defer b.Close(); s.handleConn(context.Background(), b) }()
		raw, _ := json.Marshal(Request{ID: "owned", Cmd: command, Token: "primary", Args: map[string]any{"key": "owned"}})
		if _, err := a.Write(append(raw, 10)); err != nil {
			t.Fatal(err)
		}
		line, err := bufio.NewReader(a).ReadBytes(10)
		a.Close()
		<-done
		if err != nil {
			t.Fatal(err)
		}
		var wire map[string]any
		decoder := json.NewDecoder(bytes.NewReader(line))
		decoder.UseNumber()
		if err := decoder.Decode(&wire); err != nil {
			t.Fatal(err)
		}
		result, ok := wire["result"].(map[string]any)
		if !ok {
			t.Fatal(wire)
		}
		var entry map[string]any
		if command == CmdConfigCenterGet {
			entry, _ = result["entry"].(map[string]any)
		} else {
			rows, ok := result["entries"].([]any)
			if !ok || len(rows) != 1 {
				t.Fatal(result)
			}
			entry, _ = rows[0].(map[string]any)
		}
		for key, want := range map[string]string{"created_at": "9007199254740993", "updated_at": "0", "version": "-2"} {
			value, ok := entry[key].(json.Number)
			if !ok || value.String() != want {
				t.Fatal(command, key, entry)
			}
		}
	}
	after, afterHash := k.Journal().Head()
	if head != after || hash != afterHash || p.CallCount() != 0 {
		t.Fatal("read numeric codec audited/called provider")
	}
}
