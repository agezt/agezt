// SPDX-License-Identifier: MIT

package http

import (
	"context"
	"encoding/json"
	"io"
	stdhttp "net/http"
	"net/http/httptest"
	"net/url"
	"strings"
	"testing"
)

func invoke(t *testing.T, tool *Tool, in httpInput) (string, bool) {
	t.Helper()
	raw, _ := json.Marshal(in)
	r, err := tool.Invoke(context.Background(), raw)
	if err != nil {
		t.Fatalf("Invoke: %v", err)
	}
	return r.Output, r.IsError
}

func TestGet_Happy(t *testing.T) {
	srv := httptest.NewServer(stdhttp.HandlerFunc(func(w stdhttp.ResponseWriter, r *stdhttp.Request) {
		w.Header().Set("Content-Type", "text/plain")
		w.WriteHeader(200)
		w.Write([]byte("pong"))
	}))
	defer srv.Close()

	u, _ := url.Parse(srv.URL)
	tool := New()
	tool.AllowedHosts = []string{u.Hostname()}

	out, isErr := invoke(t, tool, httpInput{Method: "GET", URL: srv.URL + "/ping"})
	if isErr {
		t.Fatalf("expected ok; out=%s", out)
	}
	if !strings.Contains(out, `"status": 200`) {
		t.Errorf("status missing: %s", out)
	}
	if !strings.Contains(out, `"body": "pong"`) {
		t.Errorf("body missing: %s", out)
	}
}

func TestPost_SendsBodyAndHeaders(t *testing.T) {
	var (
		gotMethod, gotCT, gotBody string
		gotHeader                 string
	)
	srv := httptest.NewServer(stdhttp.HandlerFunc(func(w stdhttp.ResponseWriter, r *stdhttp.Request) {
		gotMethod = r.Method
		gotCT = r.Header.Get("Content-Type")
		gotHeader = r.Header.Get("X-Custom")
		b, _ := io.ReadAll(r.Body)
		gotBody = string(b)
		w.WriteHeader(201)
		w.Write([]byte(`{"ok":true}`))
	}))
	defer srv.Close()

	u, _ := url.Parse(srv.URL)
	tool := New()
	tool.AllowedHosts = []string{u.Hostname()}

	out, isErr := invoke(t, tool, httpInput{
		Method:      "POST",
		URL:         srv.URL + "/api",
		Body:        `{"x":1}`,
		ContentType: "application/json",
		Headers:     map[string]string{"X-Custom": "abc"},
	})
	if isErr {
		t.Errorf("got IsError; out=%s", out)
	}
	if gotMethod != "POST" {
		t.Errorf("method=%q", gotMethod)
	}
	if gotCT != "application/json" {
		t.Errorf("content-type=%q", gotCT)
	}
	if gotBody != `{"x":1}` {
		t.Errorf("body=%q", gotBody)
	}
	if gotHeader != "abc" {
		t.Errorf("X-Custom=%q", gotHeader)
	}
	if !strings.Contains(out, `"status": 201`) {
		t.Errorf("response status missing: %s", out)
	}
}

func TestHostDenied_DefaultDeny(t *testing.T) {
	tool := New() // no allowlist
	out, isErr := invoke(t, tool, httpInput{Method: "GET", URL: "https://example.com"})
	if !isErr {
		t.Errorf("expected IsError on default-deny; out=%s", out)
	}
	if !strings.Contains(out, "not in allowlist") {
		t.Errorf("output missing allowlist note: %s", out)
	}
}

func TestHostDenied_AllowList(t *testing.T) {
	tool := New()
	tool.AllowedHosts = []string{"good.com"}
	out, isErr := invoke(t, tool, httpInput{Method: "GET", URL: "https://bad.com"})
	if !isErr || !strings.Contains(out, "not in allowlist") {
		t.Errorf("bad host should be denied; out=%s isErr=%v", out, isErr)
	}
}

func TestWildcardSubdomain(t *testing.T) {
	tool := New()
	tool.AllowedHosts = []string{"*.example.com"}

	// Override the client so the test doesn't actually hit the network.
	tool.HTTP = &stdhttp.Client{Transport: rejectingTransport{}}

	// Match: "api.example.com" → ok host-wise (then the rejectingTransport
	// fails the dial — we only care that the allowlist accepted it).
	_, isErr := invoke(t, tool, httpInput{Method: "GET", URL: "https://api.example.com/x"})
	if !isErr {
		t.Error("expected the transport reject to bubble as IsError")
	}
	out, _ := invoke(t, tool, httpInput{Method: "GET", URL: "https://api.example.com/x"})
	if strings.Contains(out, "not in allowlist") {
		t.Errorf("host should have been allowed: %s", out)
	}

	// No match: bare "example.com" is NOT allowed by "*.example.com".
	out2, isErr2 := invoke(t, tool, httpInput{Method: "GET", URL: "https://example.com/x"})
	if !isErr2 || !strings.Contains(out2, "not in allowlist") {
		t.Errorf("bare apex should be denied by *.example.com; out=%s", out2)
	}
}

func TestAllowAllBypassesHostCheck(t *testing.T) {
	tool := New()
	tool.AllowAll = true
	tool.HTTP = &stdhttp.Client{Transport: rejectingTransport{}}
	out, isErr := invoke(t, tool, httpInput{Method: "GET", URL: "https://anything.example/x"})
	if !isErr {
		t.Error("transport reject should bubble")
	}
	if strings.Contains(out, "not in allowlist") {
		t.Error("AllowAll should bypass allowlist check")
	}
}

func TestRejectsNonHTTPSchemes(t *testing.T) {
	tool := New()
	tool.AllowAll = true
	for _, u := range []string{"file:///etc/passwd", "ftp://example.com", "gopher://example.com"} {
		out, isErr := invoke(t, tool, httpInput{Method: "GET", URL: u})
		if !isErr || !strings.Contains(out, "scheme") {
			t.Errorf("%s should be rejected; out=%s", u, out)
		}
	}
}

func TestRejectsBadMethod(t *testing.T) {
	tool := New()
	tool.AllowAll = true
	out, isErr := invoke(t, tool, httpInput{Method: "DELETE", URL: "https://x.example/"})
	if !isErr || !strings.Contains(out, "not allowed") {
		t.Errorf("DELETE should be rejected in M1; out=%s", out)
	}
}

func TestBodyTooLarge(t *testing.T) {
	tool := New()
	tool.AllowAll = true
	huge := strings.Repeat("x", MaxRequestBodyBytes+1)
	out, isErr := invoke(t, tool, httpInput{Method: "POST", URL: "https://x.example/", Body: huge})
	if !isErr || !strings.Contains(out, "too large") {
		t.Errorf("over-size body should be rejected; out=%s", out)
	}
}

func TestNon2xx_FlaggedIsError(t *testing.T) {
	srv := httptest.NewServer(stdhttp.HandlerFunc(func(w stdhttp.ResponseWriter, _ *stdhttp.Request) {
		w.WriteHeader(500)
		w.Write([]byte("boom"))
	}))
	defer srv.Close()
	u, _ := url.Parse(srv.URL)
	tool := New()
	tool.AllowedHosts = []string{u.Hostname()}
	out, isErr := invoke(t, tool, httpInput{Method: "GET", URL: srv.URL})
	if !isErr {
		t.Error("expected IsError for HTTP 500")
	}
	if !strings.Contains(out, `"status": 500`) {
		t.Errorf("status missing: %s", out)
	}
}

func TestResponseTruncation(t *testing.T) {
	big := strings.Repeat("y", MaxResponseBytes+1000)
	srv := httptest.NewServer(stdhttp.HandlerFunc(func(w stdhttp.ResponseWriter, _ *stdhttp.Request) {
		w.Write([]byte(big))
	}))
	defer srv.Close()
	u, _ := url.Parse(srv.URL)
	tool := New()
	tool.AllowedHosts = []string{u.Hostname()}
	out, _ := invoke(t, tool, httpInput{Method: "GET", URL: srv.URL})
	if !strings.Contains(out, `"truncated": true`) {
		t.Errorf("truncation flag missing: %s", out[:min(200, len(out))])
	}
}

func TestDefinitionSchema(t *testing.T) {
	def := New().Definition()
	if def.Name != "http" {
		t.Errorf("name=%q", def.Name)
	}
	if !strings.Contains(def.Description, "allowlist") {
		t.Errorf("description missing allowlist hint: %q", def.Description)
	}
}

// rejectingTransport refuses every request — used to confirm the
// allowlist gate runs BEFORE the network is touched.
type rejectingTransport struct{}

func (rejectingTransport) RoundTrip(*stdhttp.Request) (*stdhttp.Response, error) {
	return nil, errTransport
}

var errTransport = &transportErr{}

type transportErr struct{}

func (*transportErr) Error() string { return "rejected by test transport" }
