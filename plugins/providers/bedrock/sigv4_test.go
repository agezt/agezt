// SPDX-License-Identifier: MIT

package bedrock

// SigV4 tests. We test the package internals (signRequest,
// canonicalQuery, sha256Hex) rather than re-exporting them, so
// the file lives in package bedrock (not bedrock_test) to keep
// the symbols reachable.

import (
	"bytes"
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/ersinkoc/agezt/kernel/agent"
)

// Test helpers — keep below so they live near where they're used.

func testCtx() context.Context { return context.Background() }

func agentReq(model string) agent.CompletionRequest {
	return agent.CompletionRequest{
		Model:    model,
		Messages: []agent.Message{{Role: agent.RoleUser, Content: "hi"}},
	}
}

// TestSignRequest_HappyPath verifies signRequest produces an
// Authorization header in the expected AWS shape and includes
// the timestamp + content-hash headers. We don't reproduce a
// full AWS published test vector because Bedrock has no public
// test vectors with payload (general SigV4 vectors are for
// other services). Instead we lock in the *structure* of the
// signed request and verify deterministic re-execution.
func TestSignRequest_HappyPath(t *testing.T) {
	req, _ := http.NewRequest("POST",
		"https://bedrock-runtime.us-east-1.amazonaws.com/model/anthropic.claude-opus-4-7/invoke",
		bytes.NewReader([]byte(`{"x":1}`)))
	req.Header.Set("Content-Type", "application/json")

	body := []byte(`{"x":1}`)
	now := time.Date(2026, 5, 29, 12, 34, 56, 0, time.UTC)
	creds := SigV4Creds{
		AccessKeyID:     "AKIDEXAMPLE",
		SecretAccessKey: "wJalrXUtnFEMI/K7MDENG+bPxRfiCYEXAMPLEKEY",
	}
	if err := signRequest(req, "us-east-1", body, creds, now); err != nil {
		t.Fatalf("signRequest: %v", err)
	}

	// Time + content headers were injected:
	if got := req.Header.Get("X-Amz-Date"); got != "20260529T123456Z" {
		t.Errorf("X-Amz-Date = %q, want 20260529T123456Z", got)
	}
	wantContentHash := sha256Hex(body)
	if got := req.Header.Get("X-Amz-Content-Sha256"); got != wantContentHash {
		t.Errorf("X-Amz-Content-Sha256 = %q, want %q", got, wantContentHash)
	}

	auth := req.Header.Get("Authorization")
	if !strings.HasPrefix(auth, "AWS4-HMAC-SHA256 ") {
		t.Fatalf("Authorization missing AWS4 prefix: %q", auth)
	}
	// Required parts: Credential, SignedHeaders, Signature.
	for _, sub := range []string{
		"Credential=AKIDEXAMPLE/20260529/us-east-1/bedrock/aws4_request",
		"SignedHeaders=content-type;host;x-amz-content-sha256;x-amz-date",
		"Signature=",
	} {
		if !strings.Contains(auth, sub) {
			t.Errorf("Authorization missing %q\n  got: %q", sub, auth)
		}
	}

	// Re-sign should be byte-identical (deterministic algorithm).
	req2, _ := http.NewRequest("POST",
		"https://bedrock-runtime.us-east-1.amazonaws.com/model/anthropic.claude-opus-4-7/invoke",
		bytes.NewReader(body))
	req2.Header.Set("Content-Type", "application/json")
	if err := signRequest(req2, "us-east-1", body, creds, now); err != nil {
		t.Fatalf("signRequest #2: %v", err)
	}
	if a, b := req.Header.Get("Authorization"), req2.Header.Get("Authorization"); a != b {
		t.Errorf("signature non-deterministic:\n  #1: %s\n  #2: %s", a, b)
	}
}

func TestSignRequest_IncludesSessionTokenWhenSet(t *testing.T) {
	req, _ := http.NewRequest("POST", "https://bedrock-runtime.us-east-1.amazonaws.com/m/i/invoke", nil)
	req.Header.Set("Content-Type", "application/json")
	creds := SigV4Creds{
		AccessKeyID:     "AKID",
		SecretAccessKey: "SK",
		SessionToken:    "sts-temp-token",
	}
	if err := signRequest(req, "us-east-1", nil, creds, time.Now()); err != nil {
		t.Fatalf("signRequest: %v", err)
	}
	if got := req.Header.Get("X-Amz-Security-Token"); got != "sts-temp-token" {
		t.Errorf("X-Amz-Security-Token = %q, want sts-temp-token", got)
	}
	// Security-token must be in the signed headers too.
	auth := req.Header.Get("Authorization")
	if !strings.Contains(auth, "x-amz-security-token") {
		t.Errorf("SignedHeaders missing x-amz-security-token: %q", auth)
	}
}

func TestSignRequest_RejectsMissingCreds(t *testing.T) {
	req, _ := http.NewRequest("POST", "https://x/", nil)
	if err := signRequest(req, "us-east-1", nil, SigV4Creds{}, time.Now()); err == nil {
		t.Error("expected error for empty creds")
	}
	if err := signRequest(req, "us-east-1", nil, SigV4Creds{AccessKeyID: "AKID"}, time.Now()); err == nil {
		t.Error("expected error for missing SecretAccessKey")
	}
	if err := signRequest(req, "", nil, SigV4Creds{AccessKeyID: "AKID", SecretAccessKey: "SK"}, time.Now()); err == nil {
		t.Error("expected error for missing region")
	}
}

func TestCanonicalQuery_SortsByKeyThenValue(t *testing.T) {
	q := map[string][]string{
		"b": {"2"},
		"a": {"3", "1"},
	}
	got := canonicalQuery(q)
	want := "a=1&a=3&b=2"
	if got != want {
		t.Errorf("canonicalQuery = %q, want %q", got, want)
	}
}

func TestAwsURIEncode_LeavesUnreservedAlone(t *testing.T) {
	const unreserved = "abcXYZ0123-_.~"
	if got := awsURIEncode(unreserved, true); got != unreserved {
		t.Errorf("encode(%q) = %q, want unchanged", unreserved, got)
	}
}

func TestAwsURIEncode_EncodesReservedAndUnicode(t *testing.T) {
	got := awsURIEncode("a b/c", false) // slash stays when encodeSlash=false
	want := "a%20b/c"
	if got != want {
		t.Errorf("encode = %q, want %q", got, want)
	}
	got2 := awsURIEncode("a b/c", true)
	want2 := "a%20b%2Fc"
	if got2 != want2 {
		t.Errorf("encode = %q, want %q", got2, want2)
	}
}

// TestProvider_CompleteWithSigV4 exercises end-to-end: SigV4
// credentials configured on the provider → request goes out with
// AWS4-HMAC-SHA256 Authorization → mock server accepts and replies.
// Verifies the bedrock.Provider.applyAuth wiring without
// re-validating the signature math itself.
func TestProvider_CompleteWithSigV4(t *testing.T) {
	var seenAuth string
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		seenAuth = r.Header.Get("Authorization")
		_ = json.NewEncoder(w).Encode(map[string]any{
			"id": "msg_1", "type": "message", "role": "assistant",
			"content":     []map[string]any{{"type": "text", "text": "sigv4 ok"}},
			"stop_reason": "end_turn",
			"usage":       map[string]any{"input_tokens": 1, "output_tokens": 2},
		})
	}))
	defer srv.Close()

	p := &Provider{
		Region:   "us-east-1",
		Endpoint: srv.URL + "/model/anthropic.claude-opus-4-7/invoke",
		HTTP:     srv.Client(),
		Now:      func() time.Time { return time.Date(2026, 5, 29, 12, 0, 0, 0, time.UTC) },
	}
	p.SetSigV4Creds(&SigV4Creds{
		AccessKeyID:     "AKIDEXAMPLE",
		SecretAccessKey: "secret",
	})
	// Verify the auth selector picked SigV4 (no bearer token set).
	if !p.hasAuth() {
		t.Fatal("hasAuth should be true with SigV4 creds")
	}

	resp, err := p.Complete(testCtx(), agentReq("anthropic.claude-opus-4-7"))
	if err != nil {
		t.Fatalf("Complete: %v", err)
	}
	if !strings.HasPrefix(seenAuth, "AWS4-HMAC-SHA256 ") {
		t.Errorf("Authorization = %q, want AWS4-HMAC-SHA256 prefix (SigV4 not applied)", seenAuth)
	}
	if resp.Message.Content != "sigv4 ok" {
		t.Errorf("content = %q", resp.Message.Content)
	}
}

func TestProvider_BearerStillWinsWhenBothSet(t *testing.T) {
	// If both bearer and SigV4 are configured (operator tried both at
	// some point), bearer wins — simpler wire-time decision.
	var seenAuth string
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		seenAuth = r.Header.Get("Authorization")
		_ = json.NewEncoder(w).Encode(map[string]any{
			"id": "x", "type": "message", "role": "assistant",
			"content":     []map[string]any{{"type": "text", "text": "bearer"}},
			"stop_reason": "end_turn",
			"usage":       map[string]any{"input_tokens": 1, "output_tokens": 1},
		})
	}))
	defer srv.Close()

	p := &Provider{
		BearerToken: "bearer-tok",
		Region:      "us-east-1",
		Endpoint:    srv.URL + "/x",
		HTTP:        srv.Client(),
	}
	p.SetSigV4Creds(&SigV4Creds{AccessKeyID: "AKID", SecretAccessKey: "SK"})

	_, err := p.Complete(testCtx(), agentReq("anthropic.claude-opus-4-7"))
	if err != nil {
		t.Fatalf("Complete: %v", err)
	}
	if seenAuth != "Bearer bearer-tok" {
		t.Errorf("Authorization = %q, want Bearer bearer-tok (bearer should win)", seenAuth)
	}
}

func TestProvider_NoAuthErrors(t *testing.T) {
	p := &Provider{Region: "us-east-1"}
	_, err := p.Complete(testCtx(), agentReq("anthropic.claude-opus-4-7"))
	if err != ErrNoBearerToken {
		t.Errorf("err = %v, want ErrNoBearerToken", err)
	}
}

func TestCollapseSpaces_LeavesSingleRunsAlone(t *testing.T) {
	cases := map[string]string{
		"a b c":     "a b c",
		"a  b":      "a b",
		"a   b  c":  "a b c",
		"single":    "single",
		"":          "",
	}
	for in, want := range cases {
		if got := collapseSpaces(in); got != want {
			t.Errorf("collapseSpaces(%q) = %q, want %q", in, got, want)
		}
	}
}
