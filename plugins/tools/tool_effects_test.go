// SPDX-License-Identifier: MIT

package tools_test

import (
	"strings"
	"testing"

	"github.com/agezt/agezt/kernel/agent"
	"github.com/agezt/agezt/kernel/warden"
	"github.com/agezt/agezt/plugins/tools/acpagent"
	"github.com/agezt/agezt/plugins/tools/artifacts"
	"github.com/agezt/agezt/plugins/tools/boardtool"
	"github.com/agezt/agezt/plugins/tools/browser"
	"github.com/agezt/agezt/plugins/tools/codeexec"
	"github.com/agezt/agezt/plugins/tools/coding"
	"github.com/agezt/agezt/plugins/tools/config"
	"github.com/agezt/agezt/plugins/tools/council"
	"github.com/agezt/agezt/plugins/tools/db"
	"github.com/agezt/agezt/plugins/tools/fetch"
	"github.com/agezt/agezt/plugins/tools/file"
	"github.com/agezt/agezt/plugins/tools/forgetool"
	"github.com/agezt/agezt/plugins/tools/homeassistant"
	httptool "github.com/agezt/agezt/plugins/tools/http"
	"github.com/agezt/agezt/plugins/tools/introspecttool"
	"github.com/agezt/agezt/plugins/tools/mcptool"
	"github.com/agezt/agezt/plugins/tools/notify"
	"github.com/agezt/agezt/plugins/tools/overseertool"
	"github.com/agezt/agezt/plugins/tools/peer"
	"github.com/agezt/agezt/plugins/tools/runstool"
	"github.com/agezt/agezt/plugins/tools/schedule"
	"github.com/agezt/agezt/plugins/tools/shell"
	"github.com/agezt/agezt/plugins/tools/skilltool"
	"github.com/agezt/agezt/plugins/tools/standingtool"
	"github.com/agezt/agezt/plugins/tools/websearch"
	"github.com/agezt/agezt/plugins/tools/workflowtool"
)

func TestFirstPartyToolDefinitionsDeclareEffects(t *testing.T) {
	ft, err := file.New(t.TempDir())
	if err != nil {
		t.Fatalf("file.New: %v", err)
	}

	tools := []agent.Tool{
		acpagent.New("agent", t.TempDir()),
		artifacts.New(),
		boardtool.New(),
		browser.New(),
		codeexec.NewWithWarden(warden.New(nil), t.TempDir(), map[string]string{"python": "python"}, true),
		coding.New("agent", t.TempDir()),
		config.New(t.TempDir()),
		council.New(),
		db.New(),
		fetch.New(),
		ft,
		forgetool.New(),
		homeassistant.New(),
		httptool.New(),
		introspecttool.New(),
		mcptool.New(),
		notify.New(),
		overseertool.New(),
		peer.NewWithTenants(map[string]peer.Peer{"main": {Name: "main", URL: "https://peer.example"}}, nil),
		runstool.New(),
		schedule.New(),
		shell.NewWithWarden(warden.New(nil)),
		skilltool.New(),
		standingtool.New(),
		websearch.New(),
		workflowtool.New(),
	}

	for _, tool := range tools {
		if tool == nil {
			t.Fatal("test fixture constructed a nil tool")
		}
		def := tool.Definition()
		t.Run(def.Name, func(t *testing.T) {
			if strings.TrimSpace(def.Name) == "" {
				t.Fatal("tool name is empty")
			}
			eff := def.Effect
			if eff.Class == "" {
				t.Fatal("effect class is empty")
			}
			if len(eff.PredictedEffects) == 0 {
				t.Fatal("predicted effects are empty")
			}
			if len(eff.AffectedResources) == 0 {
				t.Fatal("affected resources are empty")
			}
			if strings.TrimSpace(eff.RollbackNotes) == "" {
				t.Fatal("rollback notes are empty")
			}
			if eff.Confidence <= 0 || eff.Confidence > 1 {
				t.Fatalf("confidence=%v, want 0 < confidence <= 1", eff.Confidence)
			}
		})
	}
}
