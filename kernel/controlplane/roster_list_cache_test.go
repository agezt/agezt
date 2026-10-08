// SPDX-License-Identifier: MIT
package controlplane

import (
	"bufio"
	"encoding/json"
	approster "github.com/agezt/agezt/kernel/app/roster"
	"github.com/agezt/agezt/kernel/roster"
	"net"
	"testing"
	"time"
)

func TestAgentListInvalidationRejectsEmptyRosterCache(t *testing.T) {
	s := &Server{}
	profiles := []roster.Profile{{Slug: "owned", Enabled: true, UpdatedMS: 17}}
	s.rosterListOnce.Do(func() {
		s.rosterList = approster.NewList(func() []roster.Profile { return profiles }, func([]roster.Profile) map[string]map[string]any { return nil }, func() time.Time { return time.Unix(1, 0) })
	})
	read := func() Response {
		a, b := net.Pipe()
		a.SetDeadline(time.Now().Add(time.Second))
		done := make(chan struct{})
		go func() { defer close(done); s.handleAgentList(b, Request{ID: "owned"}) }()
		line, err := bufio.NewReader(a).ReadBytes(10)
		a.Close()
		b.Close()
		<-done
		if err != nil {
			t.Fatal(err)
		}
		var reply Response
		json.Unmarshal(line, &reply)
		return reply
	}
	first := read()
	if first.Type != RespResult || first.Result["total"] != float64(1) {
		t.Fatal(first)
	}
	profiles = nil
	s.invalidateAgentListCache()
	second := read()
	if second.Type != RespResult || second.Result["total"] != float64(0) || second.Result["enabled_count"] != float64(0) {
		t.Fatalf("EXPECTED: invalidated cache recomputes empty roster; ACTUAL: %+v", second)
	}
}
