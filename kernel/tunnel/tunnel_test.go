// SPDX-License-Identifier: MIT

package tunnel

import (
	"context"
	"reflect"
	"sync"
	"sync/atomic"
	"testing"
	"time"
)

func TestBuildCommand(t *testing.T) {
	cases := []struct {
		name string
		cfg  Config
		want []string
		err  bool
	}{
		{"cloudflared preset", Config{Provider: "cloudflared", TargetURL: "http://127.0.0.1:8787"},
			[]string{"cloudflared", "tunnel", "--url", "http://127.0.0.1:8787"}, false},
		{"ngrok preset strips scheme", Config{Provider: "ngrok", TargetURL: "http://127.0.0.1:8800/api"},
			[]string{"ngrok", "http", "127.0.0.1:8800", "--log=stdout", "--log-format=logfmt"}, false},
		{"explicit command wins", Config{Command: []string{"mytunnel", "--port", "9000"}, Provider: "cloudflared"},
			[]string{"mytunnel", "--port", "9000"}, false},
		{"preset needs target", Config{Provider: "cloudflared"}, nil, true},
		{"unknown provider", Config{Provider: "frpc", TargetURL: "http://x"}, nil, true},
		{"empty provider, no command", Config{TargetURL: "http://x"}, nil, true},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			got, err := buildCommand(c.cfg)
			if c.err {
				if err == nil {
					t.Fatalf("want error, got cmd %v", got)
				}
				return
			}
			if err != nil {
				t.Fatalf("unexpected error: %v", err)
			}
			if !reflect.DeepEqual(got, c.want) {
				t.Errorf("got %v, want %v", got, c.want)
			}
		})
	}
}

func TestExtractURL(t *testing.T) {
	cases := []struct {
		line, want string
	}{
		{"|  https://happy-tree-1234.trycloudflare.com  |", "https://happy-tree-1234.trycloudflare.com"},
		{`t=2026-06-08 lvl=info msg="started tunnel" url=https://abcd.ngrok-free.app`, "https://abcd.ngrok-free.app"},
		// Prefer the tunnel domain over a docs/banner link on the same line.
		{"visit https://developers.cloudflare.com/ or use https://x.trycloudflare.com now", "https://x.trycloudflare.com"},
		// Custom binary: no known domain → first https wins, trailing punctuation trimmed.
		{"forwarding to https://my.custom.example/path).", "https://my.custom.example/path"},
		{"no url here", ""},
		{"http://insecure.example only http is ignored", ""},
	}
	for _, c := range cases {
		if got := extractURL(c.line); got != c.want {
			t.Errorf("extractURL(%q) = %q, want %q", c.line, got, c.want)
		}
	}
}

func TestStripScheme(t *testing.T) {
	for in, want := range map[string]string{
		"http://127.0.0.1:8787":      "127.0.0.1:8787",
		"https://host:9000/path?q=1": "host:9000",
		"127.0.0.1:8800":             "127.0.0.1:8800",
	} {
		if got := stripScheme(in); got != want {
			t.Errorf("stripScheme(%q) = %q, want %q", in, got, want)
		}
	}
}

func TestNew_ErrorsWithoutConfig(t *testing.T) {
	if _, err := New(Config{}); err == nil {
		t.Error("New with empty config must error")
	}
	tu, err := New(Config{Provider: "cloudflared", TargetURL: "http://127.0.0.1:8787"})
	if err != nil {
		t.Fatal(err)
	}
	if tu.Name() != "cloudflared" {
		t.Errorf("Name() = %q, want cloudflared", tu.Name())
	}
}

// The supervisor captures the URL the process advertises.
func TestStart_CapturesURL(t *testing.T) {
	tu := &Tunnel{cmd: []string{"fake"}, run: func(ctx context.Context, _ string, _ []string, onLine func(string)) error {
		onLine("starting…")
		onLine("|  https://demo.trycloudflare.com  |")
		<-ctx.Done() // stay "running" until shutdown
		return nil
	}}
	ctx, cancel := context.WithCancel(context.Background())
	done := make(chan struct{})
	go func() { tu.Start(ctx); close(done) }()

	waitFor(t, func() bool { return tu.URL() == "https://demo.trycloudflare.com" })
	cancel()
	<-done
	if tu.URL() != "" {
		t.Errorf("URL should clear after shutdown, got %q", tu.URL())
	}
}

// The supervisor restarts the binary when it exits unexpectedly, and the backoff
// sleep is injected so the test doesn't actually wait.
func TestStart_RestartsOnExit(t *testing.T) {
	defer func() { afterFunc = time.After }()
	afterFunc = func(time.Duration) <-chan time.Time {
		ch := make(chan time.Time, 1)
		ch <- time.Time{} // fire immediately — no real sleep
		return ch
	}

	var calls int32
	ctx, cancel := context.WithCancel(context.Background())
	tu := &Tunnel{cmd: []string{"fake"}, run: func(ctx context.Context, _ string, _ []string, onLine func(string)) error {
		n := atomic.AddInt32(&calls, 1)
		if n < 3 {
			return nil // exit immediately → supervisor should restart
		}
		onLine("url=https://stable.ngrok-free.app")
		<-ctx.Done()
		return nil
	}}
	done := make(chan struct{})
	go func() { tu.Start(ctx); close(done) }()

	waitFor(t, func() bool { return atomic.LoadInt32(&calls) >= 3 && tu.URL() == "https://stable.ngrok-free.app" })
	cancel()
	<-done
}

func waitFor(t *testing.T, cond func() bool) {
	t.Helper()
	deadline := time.Now().Add(2 * time.Second)
	for time.Now().Before(deadline) {
		if cond() {
			return
		}
		time.Sleep(5 * time.Millisecond)
	}
	t.Fatal("condition not met within deadline")
}

// Guard against a data race on url across the supervisor goroutine and URL().
func TestURL_ConcurrentAccess(t *testing.T) {
	tu := &Tunnel{}
	var wg sync.WaitGroup
	wg.Add(2)
	go func() {
		defer wg.Done()
		for i := 0; i < 1000; i++ {
			tu.setURL("https://x")
		}
	}()
	go func() {
		defer wg.Done()
		for i := 0; i < 1000; i++ {
			_ = tu.URL()
		}
	}()
	wg.Wait()
}
