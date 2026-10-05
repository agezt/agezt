// SPDX-License-Identifier: MIT

package providers_test

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/agezt/agezt/kernel/app"
	"github.com/agezt/agezt/kernel/app/providers"
	"github.com/agezt/agezt/kernel/contract/opapi"
)

func TestProbeDispatchCancellationStopsOwnedHTTPWait(t *testing.T) {
	started, release := make(chan struct{}), make(chan struct{})
	endpoint := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		close(started)
		select {
		case <-r.Context().Done():
		case <-release:
		}
	}))
	defer endpoint.Close()
	defer close(release)
	ops, err := providers.ProbeOperations(func(context.Context) *providers.Probe { return providers.NewProbe(nil) })
	if err != nil {
		t.Fatal(err)
	}
	d, err := app.NewDispatcher(ops, app.Dependencies{Auth: observationAuth{}, Router: observationRouter{}})
	if err != nil {
		t.Fatal(err)
	}
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	raw, _ := json.Marshal(map[string]any{"url": endpoint.URL})
	done := make(chan struct{}, 1)
	go func() {
		_, _ = d.Dispatch(ctx, opapi.Caller{}, "provider_probe", raw, nil)
		done <- struct{}{}
	}()
	select {
	case <-started:
	case <-time.After(time.Second):
		t.Fatal("owned HTTP probe did not start")
	}
	cancel()
	select {
	case <-done:
	case <-time.After(250 * time.Millisecond):
		t.Fatal("EXPECTED: caller cancellation releases probe HTTP wait; ACTUAL: request continues on background timeout")
	}
}

func TestProbeContextCancellationDuringBodyDoesNotReportSuccess(t *testing.T) {
	started, release := make(chan struct{}), make(chan struct{})
	endpoint := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
		w.(http.Flusher).Flush()
		close(started)
		select {
		case <-r.Context().Done():
		case <-release:
		}
	}))
	defer endpoint.Close()
	defer close(release)
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	done := make(chan providers.ProbeOutput, 1)
	go func() {
		out, _ := providers.NewProbe(nil).CheckContext(ctx, providers.ProbeInput{URL: endpoint.URL})
		done <- out
	}()
	<-started
	// Let the owned response headers reach the client before canceling its body.
	time.Sleep(20 * time.Millisecond)
	cancel()
	select {
	case out := <-done:
		if out.OK || out.Error == "" {
			t.Fatalf("canceled response body reported success: %+v", out)
		}
	case <-time.After(time.Second):
		t.Fatal("canceled body wait did not stop")
	}
}

func TestProbeContextPortPreservesCallerAndLegacyEntryPoints(t *testing.T) {
	type key struct{}
	parent, cancel := context.WithTimeout(context.WithValue(context.Background(), key{}, "marker"), time.Second)
	defer cancel()
	seen := false
	probe := providers.NewProbeWithContext(func(ctx context.Context, endpoint, header, token string, max int64) ([]byte, int, string, error) {
		seen = true
		if ctx != parent || ctx.Value(key{}) != "marker" || endpoint != "https://fixture.invalid/models" || header != "Authorization" || token != "Bearer fixture" || max != 1<<20 {
			t.Fatal("context port lost caller or request fields")
		}
		return []byte(`{"data":[]}`), 200, "application/json", nil
	})
	if out, err := probe.CheckContext(parent, providers.ProbeInput{URL: "https://fixture.invalid", Key: "fixture"}); err != nil || !out.OK || !seen {
		t.Fatalf("context port output = %+v, %v", out, err)
	}
	legacy := providers.NewProbe(func(_, _, _ string, _ int64) ([]byte, int, string, error) { return []byte(`{"data":[]}`), 200, "", nil })
	if out, err := legacy.Check(providers.ProbeInput{URL: "https://fixture.invalid"}); err != nil || !out.OK {
		t.Fatalf("legacy injected port changed: %+v %v", out, err)
	}
}
