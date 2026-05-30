// SPDX-License-Identifier: MIT

package peer

import (
	"context"
	"encoding/json"
	"strings"
	"testing"
)

func invoke(t *testing.T, tool *Tool, input map[string]string) (string, bool) {
	t.Helper()
	in, _ := json.Marshal(input)
	res, err := tool.Invoke(context.Background(), in)
	if err != nil {
		t.Fatalf("Invoke: %v", err)
	}
	return res.Output, res.IsError
}

// fakePost returns a poster that records the call and returns a scripted result.
func fakePost(status int, body string, gotEndpoint, gotToken, gotBody *string) poster {
	return func(_ context.Context, endpoint, token string, b []byte) (int, []byte, error) {
		if gotEndpoint != nil {
			*gotEndpoint = endpoint
		}
		if gotToken != nil {
			*gotToken = token
		}
		if gotBody != nil {
			*gotBody = string(b)
		}
		return status, []byte(body), nil
	}
}

func TestRemoteRun_HappyPath(t *testing.T) {
	var endpoint, token, body string
	tool := &Tool{
		Peers: map[string]Peer{"nodeB": {Name: "nodeB", URL: "http://host:8800", Token: "sekret"}},
		post:  fakePost(200, `{"correlation_id":"run-abc","status":"completed","answer":"42"}`, &endpoint, &token, &body),
	}
	out, isErr := invoke(t, tool, map[string]string{"peer": "nodeB", "task": "compute the answer"})
	if isErr {
		t.Fatalf("unexpected error: %s", out)
	}
	if !strings.Contains(out, "42") {
		t.Errorf("answer not relayed: %s", out)
	}
	if !strings.Contains(out, "peer=nodeB") || !strings.Contains(out, "correlation=run-abc") {
		t.Errorf("footer missing peer/correlation: %s", out)
	}
	// It drove the peer's native REST endpoint with the Bearer token and intent.
	if endpoint != "http://host:8800/api/v1/runs" {
		t.Errorf("endpoint = %q", endpoint)
	}
	if token != "sekret" {
		t.Errorf("token = %q", token)
	}
	if !strings.Contains(body, `"intent":"compute the answer"`) {
		t.Errorf("body = %q", body)
	}
}

func TestRemoteRun_SinglePeerDefault(t *testing.T) {
	// With exactly one peer, the peer name may be omitted.
	tool := &Tool{
		Peers: map[string]Peer{"only": {Name: "only", URL: "http://h:1"}},
		post:  fakePost(200, `{"status":"completed","answer":"ok","correlation_id":"c"}`, nil, nil, nil),
	}
	out, isErr := invoke(t, tool, map[string]string{"task": "go"})
	if isErr {
		t.Fatalf("unexpected error: %s", out)
	}
	if !strings.Contains(out, "peer=only") {
		t.Errorf("should default to the sole peer: %s", out)
	}
}

func TestRemoteRun_AmbiguousPeerRequiresName(t *testing.T) {
	tool := &Tool{
		Peers: map[string]Peer{"a": {Name: "a", URL: "http://h:1"}, "b": {Name: "b", URL: "http://h:2"}},
		post:  fakePost(200, `{}`, nil, nil, nil),
	}
	out, isErr := invoke(t, tool, map[string]string{"task": "go"}) // no peer named, two configured
	if !isErr || !strings.Contains(out, "peer name is required") {
		t.Errorf("ambiguous peer should error, got: %s", out)
	}
}

func TestRemoteRun_UnknownPeer(t *testing.T) {
	tool := &Tool{
		Peers: map[string]Peer{"a": {Name: "a", URL: "http://h:1"}},
		post:  fakePost(200, `{}`, nil, nil, nil),
	}
	out, isErr := invoke(t, tool, map[string]string{"peer": "nope", "task": "go"})
	if !isErr || !strings.Contains(out, "unknown peer") {
		t.Errorf("unknown peer should error, got: %s", out)
	}
}

func TestRemoteRun_PeerFailureSurfaced(t *testing.T) {
	tool := &Tool{
		Peers: map[string]Peer{"a": {Name: "a", URL: "http://h:1"}},
		post:  fakePost(502, `{"correlation_id":"run-x","status":"failed","error":"provider exhausted"}`, nil, nil, nil),
	}
	out, isErr := invoke(t, tool, map[string]string{"peer": "a", "task": "go"})
	if !isErr {
		t.Error("a failed remote run should be an error result")
	}
	if !strings.Contains(out, "provider exhausted") || !strings.Contains(out, "run-x") {
		t.Errorf("peer error + correlation should surface: %s", out)
	}
}

func TestRemoteRun_EmptyTask(t *testing.T) {
	tool := &Tool{Peers: map[string]Peer{"a": {Name: "a", URL: "http://h:1"}}, post: fakePost(200, `{}`, nil, nil, nil)}
	out, isErr := invoke(t, tool, map[string]string{"peer": "a", "task": "  "})
	if !isErr || !strings.Contains(out, "task is required") {
		t.Errorf("empty task should error: %s", out)
	}
}

func TestNew_DisabledWhenNoPeers(t *testing.T) {
	if New(nil) != nil {
		t.Error("New(nil) should return nil (disabled)")
	}
	if New(map[string]Peer{}) != nil {
		t.Error("New(empty) should return nil")
	}
	if New(map[string]Peer{"a": {Name: "a", URL: "http://h:1"}}) == nil {
		t.Error("New with a peer should return a tool")
	}
}

func TestParsePeers(t *testing.T) {
	peers, err := ParsePeers("nodeB=http://h:8800|tok, nodeC = https://h2:8801 ")
	if err != nil {
		t.Fatal(err)
	}
	if len(peers) != 2 {
		t.Fatalf("got %d peers", len(peers))
	}
	if peers["nodeB"].URL != "http://h:8800" || peers["nodeB"].Token != "tok" {
		t.Errorf("nodeB = %+v", peers["nodeB"])
	}
	if peers["nodeC"].URL != "https://h2:8801" || peers["nodeC"].Token != "" {
		t.Errorf("nodeC = %+v", peers["nodeC"])
	}

	if _, err := ParsePeers("noequals|x"); err == nil {
		t.Error("entry without name= should error")
	}
	if _, err := ParsePeers("n=not-a-url"); err == nil {
		t.Error("non-http URL should error")
	}
	if p, err := ParsePeers("  "); err != nil || p != nil {
		t.Errorf("empty spec = %v, %v", p, err)
	}
}

func TestDescribe_RedactsToken(t *testing.T) {
	out := Describe(map[string]Peer{"a": {Name: "a", URL: "http://h:1", Token: "supersecret"}})
	if strings.Contains(out, "supersecret") {
		t.Errorf("Describe leaked the token: %s", out)
	}
	if !strings.Contains(out, "(token)") {
		t.Errorf("token-authed peer should be marked: %s", out)
	}
}
