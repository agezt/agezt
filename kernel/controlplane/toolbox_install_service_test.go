// SPDX-License-Identifier: MIT
package controlplane

import (
	"bufio"
	"context"
	"encoding/json"
	"github.com/agezt/agezt/kernel/event"
	"github.com/agezt/agezt/kernel/toolbox"
	"net"
	"testing"
	"time"
)

func TestToolboxInstallNativeUnknownNamesKeepProgressSummaryAndJournalOrder(t *testing.T) {
	name := "__agezt_owned_not_in_catalog__"
	for _, entry := range toolbox.Catalog {
		if entry.Name == name {
			t.Fatal("fixture would run a real installer")
		}
	}
	k, s, _, p := pulseAppFixture(t)
	head, _ := k.Journal().Head()
	client, conn := net.Pipe()
	defer client.Close()
	defer conn.Close()
	done := make(chan struct{})
	go func() { defer close(done); s.handleConn(context.Background(), conn) }()
	client.SetDeadline(time.Now().Add(3 * time.Second))
	raw, _ := json.Marshal(Request{ID: "owned", Cmd: CmdToolboxInstall, Token: "primary", Args: map[string]any{"names": []any{name, name}}})
	if _, err := client.Write(append(raw, 10)); err != nil {
		t.Fatal(err)
	}
	reader := bufio.NewReader(client)
	var frames []Response
	for {
		line, err := reader.ReadBytes(10)
		if err != nil {
			t.Fatal(err)
		}
		var r Response
		if err := json.Unmarshal(line, &r); err != nil {
			t.Fatal(err)
		}
		frames = append(frames, r)
		if r.Type != RespEvent {
			break
		}
	}
	client.Close()
	<-done
	if len(frames) != 3 || frames[2].Type != RespResult {
		t.Fatal(frames)
	}
	for _, r := range frames[:2] {
		if r.Type != RespEvent || r.Event == nil || r.Event.Kind != event.KindToolboxProgress || r.Event.Subject != "toolbox.install" || r.Event.Actor != "toolbox" || r.Event.Seq != 0 {
			t.Fatal(r)
		}
		var result toolbox.InstallResult
		_ = json.Unmarshal(r.Event.Payload, &result)
		if result.Tool != name || !result.Skipped || result.OK || result.Error != "unknown tool" {
			t.Fatal(result)
		}
	}
	summary := frames[2].Result
	if len(summary) != 3 || len(summary["installed"].([]any)) != 0 || len(summary["failed"].([]any)) != 0 || len(summary["skipped"].([]any)) != 2 {
		t.Fatal(summary)
	}
	rows, err := k.Journal().Tail(100)
	if err != nil {
		t.Fatal(err)
	}
	var kinds []event.Kind
	for _, e := range rows {
		if e.Seq > head {
			kinds = append(kinds, e.Kind)
			if e.Kind == event.KindToolboxInstalled {
				var payload map[string]any
				_ = json.Unmarshal(e.Payload, &payload)
				if payload["tool"] != name || payload["ok"] != false || payload["skipped"] != true {
					t.Fatal(payload)
				}
			}
		}
	}
	want := []event.Kind{event.KindOpInvoked, event.KindToolboxInstallRequested, event.KindToolboxInstalled, event.KindToolboxInstalled, event.KindOpCompleted}
	if len(kinds) != len(want) {
		t.Fatal(kinds)
	}
	for i, kind := range kinds {
		if kind != want[i] {
			t.Fatal(kinds, want)
		}
	}
	if p.CallCount() != 0 {
		t.Fatal(p.CallCount())
	}
}
