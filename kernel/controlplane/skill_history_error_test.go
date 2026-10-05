// SPDX-License-Identifier: MIT

package controlplane

import (
	"github.com/agezt/agezt/kernel/event"
	"github.com/agezt/agezt/kernel/runtime"
	"github.com/agezt/agezt/kernel/skill"
	"github.com/agezt/agezt/plugins/providers/mock"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestSkillNativeHistoryRejectsCorruptJournalWithoutSuccess(t *testing.T) {
	for _, partial := range []bool{false, true} {
		t.Run(map[bool]string{false: "empty", true: "partial"}[partial], func(t *testing.T) {
			dir := t.TempDir()
			k, err := runtime.Open(runtime.Config{BaseDir: dir, Provider: mock.New()})
			if err != nil {
				t.Fatal(err)
			}
			defer k.Close()
			id := "owned"
			if partial {
				sk, _, err := k.Forge().Create("seed", skill.CreateSpec{Name: "fixture", Body: "body"})
				if err != nil {
					t.Fatal(err)
				}
				id = sk.ID
			}
			server := NewServer(k, dir)
			server.token = "primary"
			if err := k.Journal().Close(); err != nil {
				t.Fatal(err)
			}
			paths, err := filepath.Glob(filepath.Join(dir, "journal", "*.jsonl"))
			if err != nil || len(paths) != 1 {
				t.Fatalf("owned journal paths=%v err=%v", paths, err)
			}
			f, err := os.OpenFile(paths[0], os.O_WRONLY|os.O_APPEND, 0600)
			if err != nil {
				t.Fatal(err)
			}
			if _, err := f.WriteString("{broken\n"); err != nil {
				f.Close()
				t.Fatal(err)
			}
			if err := f.Close(); err != nil {
				t.Fatal(err)
			}
			if err := k.Journal().Range(func(*event.Event) error { return nil }); err == nil {
				t.Fatal("fixture did not induce a real journal decode failure")
			}
			response := callAppHost(t, server, Request{ID: "history", Cmd: CmdSkillHistory, Token: "primary", Args: map[string]any{"id": id}})[0]
			if response.Type != RespError || !strings.Contains(response.Error, "journal: decode") || response.Result != nil {
				t.Fatalf("EXPECTED: native error without completed partial history; ACTUAL: partial=%v response=%+v", partial, response)
			}
		})
	}
}
