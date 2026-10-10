// SPDX-License-Identifier: MIT

package nodes

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"reflect"
	"strings"
	"testing"

	"github.com/agezt/agezt/internal/brand"
	"github.com/agezt/agezt/kernel/contract/opapi"
	"github.com/agezt/agezt/kernel/platform/schema"
)

func TestParsePeers(t *testing.T) {
	peers, err := ParsePeers(" , b = https://b.example/ | tok ,a=http://a:1 ,, ")
	if err != nil || !reflect.DeepEqual(peers, []Peer{{Name: "b", URL: "https://b.example", Token: "tok"}, {Name: "a", URL: "http://a:1"}}) {
		t.Fatal(peers, err)
	}
	if peers, err := ParsePeers("  "); peers != nil || err != nil {
		t.Fatal(peers, err)
	}
	for spec, want := range map[string]string{
		"nodeB":                   `peer "nodeB": expected name=url|token`,
		"a=":                      `peer "a=": name and url are required`,
		"=http://x":               `peer "=http://x": name and url are required`,
		"a=|tok":                  `peer "a=|tok": name and url are required`,
		"a=http://x,a=http://y":   `duplicate peer "a"`,
		"a=ftp://x":               `peer "a": url must be http(s)`,
		"a=http://":               `peer "a": url must be http(s)`,
		"a=http://x,b=:bad":       `peer "b": url must be http(s)`,
		"ok=http://x, broken one": `peer "broken one": expected name=url|token`,
	} {
		if _, err := ParsePeers(spec); err == nil || err.Error() != want {
			t.Fatalf("%s: %v, want %s", spec, err, want)
		}
	}
}

func registry(t *testing.T, ports Ports) string {
	t.Helper()
	out, err := New(ports).Registry(context.Background(), RegistryRequest{})
	if err != nil {
		t.Fatal(err)
	}
	raw, _ := json.Marshal(out)
	return string(raw)
}

func TestRegistry(t *testing.T) {
	health := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/api/v1/health" {
			w.WriteHeader(http.StatusNotFound)
			return
		}
		switch r.Header.Get("Authorization") {
		case "Bearer good":
			_, _ = w.Write([]byte(`{"status":"ok","version":"v2","model_count":3}`))
		case "Bearer degraded":
			_, _ = w.Write([]byte(`{"status":"degraded"}`))
		case "Bearer garbage":
			_, _ = w.Write([]byte(`not json`))
		case "Bearer broken":
			w.WriteHeader(http.StatusBadGateway)
		default:
			w.WriteHeader(http.StatusUnauthorized)
		}
	}))
	defer health.Close()
	closed := httptest.NewServer(http.NotFoundHandler())
	closedURL := closed.URL
	closed.Close()
	spec := "z=" + health.URL + "/|good,y=" + health.URL + "|degraded,x=" + health.URL + "|garbage,w=" + health.URL + "|broken,v=" + health.URL + ",u=" + closedURL + "|tok"
	calls := 0
	ports := Ports{
		Model:     func() string { return "m1" },
		RemoteRun: func() (bool, bool) { calls++; return true, false },
		PeerSpec:  func() string { return spec },
		Client:    func() *http.Client { return health.Client() },
	}
	got := registry(t, ports)
	local := `{"id":"local","name":"local","local":true,"reachable":true,"status":"ok","version":"` + brand.Version + `","model":"m1","capabilities":["controlplane","webui","agent-runtime","remote-run"]}`
	for _, want := range []string{
		`{"nodes":[` + local + `,{"id":"peer:u","name":"u","local":false,"url":"` + closedURL + `","auth":"token","reachable":false,"status":"unreachable","error":"Get \"` + closedURL + `/api/v1/health\": dial tcp`,
		`{"id":"peer:v","name":"v","local":false,"url":"` + health.URL + `","auth":"none","reachable":false,"status":"unreachable","error":"401 (token rejected)"}`,
		`{"id":"peer:w","name":"w","local":false,"url":"` + health.URL + `","auth":"token","reachable":false,"status":"unreachable","error":"status 502"}`,
		`{"id":"peer:x","name":"x","local":false,"url":"` + health.URL + `","auth":"token","reachable":false,"status":"unreachable","error":"bad health response: invalid character 'o' in literal null (expecting 'u')"}`,
		`{"id":"peer:y","name":"y","local":false,"url":"` + health.URL + `","auth":"token","reachable":false,"status":"degraded","version":"","model_count":0,"error":"status=degraded"}`,
		`{"id":"peer:z","name":"z","local":false,"url":"` + health.URL + `","auth":"token","reachable":true,"status":"ok","version":"v2","model_count":3}],"count":7,"peer_count":6,"remote_run_registered":false}`,
	} {
		if !strings.Contains(got, want) {
			t.Fatalf("missing %s in %s", want, got)
		}
	}
	if calls != 2 || strings.Contains(got, "good") || strings.Contains(got, "garbage") {
		t.Fatal(calls, got)
	}

	ports.RemoteRun = func() (bool, bool) { return false, false }
	ports.PeerSpec = func() string { return "a=http://x,a=http://y" }
	ports.Model = func() string { return "" }
	if got := registry(t, ports); got != `{"nodes":[{"id":"local","name":"local","local":true,"reachable":true,"status":"ok","version":"`+brand.Version+`","model":"","capabilities":["controlplane","webui","agent-runtime"]}],"count":1,"peer_count":0,"error":"duplicate peer \"a\""}` {
		t.Fatal(got)
	}
	ports.PeerSpec = func() string { return "" }
	ports.RemoteRun = func() (bool, bool) { return true, true }
	if got := registry(t, ports); !strings.HasSuffix(got, `"capabilities":["controlplane","webui","agent-runtime","remote-run"]}],"count":1,"peer_count":0,"remote_run_registered":true}`) {
		t.Fatal(got)
	}
}

func TestOperations(t *testing.T) {
	if _, err := Operations(nil); err == nil {
		t.Fatal("provider is required")
	}
	ops, err := Operations(func(context.Context) *Service { return nil })
	if err != nil || len(ops) != 1 {
		t.Fatal(len(ops), err)
	}
	s := ops[0].Spec()
	out, err := schema.FromType(reflect.TypeFor[RegistryOutput](), false)
	if err != nil {
		t.Fatal(err)
	}
	if s.Name != "node_registry" || !s.ReadOnly || s.Authz != opapi.PrimaryOnly || s.Tenancy != opapi.Primary || s.Stream != opapi.StreamNone || !s.AllowUnknownInput || s.HTTP.Method != "GET" || s.HTTP.Path != "/api/nodes" || s.Input != reflect.TypeFor[RegistryRequest]() || s.Output != reflect.TypeFor[RegistryOutput]() || string(s.OutputSchema) != string(out) {
		t.Fatalf("spec: %+v", s)
	}
	for _, in := range []string{`{}`, `{"tenant":"t","junk":[1]}`} {
		if err := schema.ValidateJSON(s.InputSchema, json.RawMessage(in)); err != nil {
			t.Fatal(in, err)
		}
	}
}
