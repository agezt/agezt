// SPDX-License-Identifier: MIT

package execprofile

import (
	"context"
	"encoding/json"
	"os"
	"path/filepath"
	"reflect"
	goruntime "runtime"
	"strings"
	"testing"

	"github.com/agezt/agezt/kernel/contract/opapi"
	"github.com/agezt/agezt/kernel/executionprofile"
	"github.com/agezt/agezt/kernel/platform/schema"
)

func encode(t *testing.T, v any) string {
	t.Helper()
	raw, err := json.Marshal(v)
	if err != nil {
		t.Fatal(err)
	}
	return string(raw)
}

func inventory() executionprofile.Inventory {
	return executionprofile.Inventory{
		HostOS: "plan9", HostArch: "mips",
		Profiles: []executionprofile.Profile{
			{
				ID: "remote", Name: "Remote", Summary: "s", Status: executionprofile.Status("supported"), Routed: true,
				RequestedIsolation: "vm", EffectiveIsolation: "process", Degraded: true, DegradeReason: "why",
				Tools: []string{"shell"}, Backends: []string{"ssh"}, FileSystem: "fs", Network: "net", Environment: "env", Secrets: "sec",
				SecretPolicy: &executionprofile.SecretPolicy{Mode: "deny", Scope: "remote/cloud", ValuesForwarded: true, MetadataForwarded: true, Valid: true, Detail: "d"},
				Limits:       []string{"l"}, BrowserAccess: "none", Cleanup: "c", PolicyCapability: "cap", Notes: []string{"n"},
			},
			{ID: "bare", Tools: []string{}, Notes: []string{}},
		},
		Count: 2, RoutedCount: 1, SupportedCount: 1, DegradedCount: 1,
	}
}

func TestList(t *testing.T) {
	builds := 0
	svc := New(Ports{Inventory: func() executionprofile.Inventory { builds++; return inventory() }})
	out, err := svc.List(context.Background(), ListRequest{})
	want := `{"host_os":"plan9","host_arch":"mips","profiles":[` +
		`{"id":"remote","name":"Remote","summary":"s","status":"supported","routed":true,"requested_isolation":"vm","effective_isolation":"process","degraded":true,"degrade_reason":"why","tools":["shell"],"backends":["ssh"],"filesystem":"fs","network":"net","environment":"env","secrets":"sec","limits":["l"],"browser_access":"none","cleanup":"c","policy_capability":"cap","notes":["n"],"secret_policy":{"mode":"deny","scope":"remote/cloud","values_forwarded":true,"metadata_forwarded":true,"valid":true,"detail":"d"}},` +
		`{"id":"bare","name":"","summary":"","status":"","routed":false,"requested_isolation":"","effective_isolation":"","degraded":false,"degrade_reason":"","tools":null,"backends":null,"filesystem":"","network":"","environment":"","secrets":"","limits":null,"browser_access":"","cleanup":"","policy_capability":"","notes":null}` +
		`],"count":2,"routed_count":1,"supported_count":1,"degraded_count":1}`
	if got := encode(t, out); err != nil || got != want || builds != 1 {
		t.Fatalf("every field is present, empty lists are null:\n%s\n%s", got, want)
	}
	empty := New(Ports{Inventory: func() executionprofile.Inventory { return executionprofile.Inventory{} }})
	if out, _ := empty.List(context.Background(), ListRequest{}); encode(t, out.Profiles) != `[]` {
		t.Fatal("no profiles is an empty array")
	}
}

func TestShow(t *testing.T) {
	builds := 0
	svc := New(Ports{Inventory: func() executionprofile.Inventory { builds++; return inventory() }})
	for in, want := range map[string]string{
		`{}`:             "args.id required",
		`{"id":"  "}`:    "args.id required",
		`{"id":null}`:    "args.id must be a string",
		`{"id":7}`:       "args.id must be a string",
		`{"id":["x"]}`:   "args.id must be a string",
		`{"id":" nope"}`: "unknown execution profile: nope",
	} {
		var req ShowRequest
		if err := json.Unmarshal([]byte(in), &req); err != nil {
			t.Fatal(err)
		}
		if _, err := svc.Show(context.Background(), req); err == nil || err.Error() != want {
			t.Fatalf("%s: %v, want %q", in, err, want)
		}
	}
	if builds != 1 {
		t.Fatal("an argument refusal never builds the inventory", builds)
	}
	out, err := svc.Show(context.Background(), ShowRequest{ID: json.RawMessage(`" bare "`)})
	if got := encode(t, out); err != nil || !strings.HasPrefix(got, `{"profile":{"id":"bare",`) || !strings.HasSuffix(got, `"notes":null},"host_os":"plan9","host_arch":"mips"}`) {
		t.Fatal("a trimmed id finds its profile", got, err)
	}
}

func TestCheck(t *testing.T) {
	policies := 0
	policy := executionprofile.ProfilePolicy{}
	svc := New(Ports{
		Inventory: inventory,
		Policy:    func() executionprofile.ProfilePolicy { policies++; return policy },
	})
	out, err := svc.Check(context.Background(), CheckRequest{})
	if err != nil || policies != 1 {
		t.Fatal(err, policies)
	}
	report := executionprofile.Diagnose(inventory(), executionprofile.HealthOptions{Policy: policy})
	if out.HostOS != "plan9" || out.HostArch != "mips" || out.Count != report.Count || out.OKCount != report.OKCount || out.WarningCount != report.WarningCount || out.FailCount != report.FailCount || len(out.Checks) != len(report.Checks) || len(out.Checks) == 0 {
		t.Fatalf("the report is copied: %+v", out)
	}
	if !reflect.DeepEqual(out.RoutableRunProfiles, append([]string(nil), report.RoutableRunProfiles...)) {
		t.Fatal(out.RoutableRunProfiles, report.RoutableRunProfiles)
	}
	for i, c := range report.Checks {
		if out.Checks[i] != (Check{ID: c.ID, ProfileID: c.ProfileID, Status: string(c.Status), Title: c.Title, Detail: c.Detail, Next: c.Next, Routed: c.Routed, Degraded: c.Degraded, BackendAvailable: c.BackendAvailable, Backend: c.Backend}) {
			t.Fatalf("check %d: %+v / %+v", i, out.Checks[i], c)
		}
	}
	// A backend found on PATH is named; a fake ssh makes that deterministic.
	bin := t.TempDir()
	name := "ssh"
	if goruntime.GOOS == "windows" {
		name = "ssh.exe"
	}
	if err := os.WriteFile(filepath.Join(bin, name), []byte("#!/bin/sh\n"), 0o755); err != nil {
		t.Fatal(err)
	}
	t.Setenv("PATH", bin)
	withSSH := New(Ports{
		Inventory: func() executionprofile.Inventory {
			inv := inventory()
			inv.Profiles = append(inv.Profiles, executionprofile.Profile{ID: "ssh"})
			return inv
		},
		Policy: func() executionprofile.ProfilePolicy { return policy },
	})
	found, _ := withSSH.Check(context.Background(), CheckRequest{})
	if !strings.Contains(encode(t, found), `{"id":"ssh.backend","profile_id":"ssh","status":"warning","title":"SSH remote profile","detail":"ssh is on PATH, but AGEZT does not route this execution profile yet","next":"wire this backend into the tool execution path before advertising it as selectable","routed":false,"degraded":false,"backend_available":true,"backend":"ssh"}`) {
		t.Fatal("a found backend is named", encode(t, found))
	}
	if got := encode(t, Check{}); got != `{"id":"","profile_id":"","status":"","title":"","detail":"","next":"","routed":false,"degraded":false,"backend_available":false,"backend":""}` {
		t.Fatal("every check field is present", got)
	}
	if got := encode(t, CheckOutput{}); !strings.HasSuffix(got, `"routable_run_profiles":null}`) {
		t.Fatal("no routable profiles is null", got)
	}
}

func TestOperations(t *testing.T) {
	if _, err := Operations(nil); err == nil {
		t.Fatal("provider is required")
	}
	ops, err := Operations(func(context.Context) *Service { return nil })
	if err != nil || len(ops) != 3 {
		t.Fatal(len(ops), err)
	}
	for i, want := range []struct {
		name, path    string
		input, output reflect.Type
	}{
		{"execution_profiles", "/api/execution_profiles", reflect.TypeFor[ListRequest](), reflect.TypeFor[ListOutput]()},
		{"execution_profile_show", "", reflect.TypeFor[ShowRequest](), reflect.TypeFor[ShowOutput]()},
		{"execution_profile_check", "/api/execution_profile_check", reflect.TypeFor[CheckRequest](), reflect.TypeFor[CheckOutput]()},
	} {
		s := ops[i].Spec()
		out, err := schema.FromType(want.output, false)
		if err != nil {
			t.Fatal(err)
		}
		method := ""
		if want.path != "" {
			method = "GET"
		}
		if s.Name != want.name || !s.ReadOnly || s.Authz != opapi.OwnTenant || s.Tenancy != opapi.CallerTenant || s.Stream != opapi.StreamNone || !s.AllowUnknownInput || s.HTTP.Method != method || s.HTTP.Path != want.path || s.Input != want.input || s.Output != want.output || string(s.OutputSchema) != string(out) {
			t.Fatalf("%s spec: %+v", want.name, s)
		}
		for _, in := range []string{`{}`, `{"id":null,"tenant":"acme","x":1}`} {
			if err := schema.ValidateJSON(s.InputSchema, json.RawMessage(in)); err != nil {
				t.Fatal(want.name, in, err)
			}
		}
	}
}
