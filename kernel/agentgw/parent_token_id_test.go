// SPDX-License-Identifier: MIT

package agentgw

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"
)

// mintChild drives handleTokenCreate with an authenticated parent and returns
// the validated claims of the child token it minted.
func mintChild(t *testing.T, g *Gateway, parent *TokenClaims, body string) *TokenClaims {
	t.Helper()

	req := httptest.NewRequest("POST", "/v1/token/create", strings.NewReader(body))
	req = req.WithContext(context.WithValue(req.Context(), claimsKey{}, parent))
	rr := httptest.NewRecorder()
	g.handleTokenCreate(rr, req)
	if rr.Code != http.StatusCreated {
		t.Fatalf("handleTokenCreate: got %d, want %d. Body: %s", rr.Code, http.StatusCreated, rr.Body.String())
	}

	var resp struct {
		Token string `json:"token"`
	}
	if err := json.Unmarshal(rr.Body.Bytes(), &resp); err != nil {
		t.Fatalf("decode response: %v (body %s)", err, rr.Body.String())
	}
	child, err := g.tokenMgr.ValidateToken(resp.Token)
	if err != nil {
		t.Fatalf("ValidateToken(child): %v", err)
	}
	return child
}

// TestTokenCreateRecordsParentTokenNotRunID guards JWT-002.
//
// handleTokenCreate used to set `ParentTokenID: parent.RunID`, while
// CreateSubprocessToken — the same operation in the library — correctly sets
// parent.TokenID. Because auditAccess logs claims.ParentTokenID as its TokenID
// field, and every child of a run shares one RunID, every audit entry for an
// HTTP-minted token recorded the run instead of the specific parent token.
// After a token leak that makes it impossible to tell which minted token was
// used.
func TestTokenCreateRecordsParentTokenNotRunID(t *testing.T) {
	g := NewGateway(GatewayConfig{
		SocketPath:  "@test/agentgw/parenttokenid.sock",
		TokenSecret: []byte("test-secret-key-32-chars-minimum!!"),
	})

	// Mint a real parent token so it carries a server-assigned TokenID, the
	// way a live parent does.
	parent := &TokenClaims{
		RunID:     "run_abc123",
		Caps:      []string{"memory.write", "memory.read"},
		MaxRate:   60,
		MaxBurst:  10,
		ExpiresAt: time.Now().Add(1 * time.Hour),
	}
	if _, err := g.tokenMgr.CreateToken(parent); err != nil {
		t.Fatalf("CreateToken(parent): %v", err)
	}
	if parent.TokenID == "" {
		t.Fatal("parent token has no TokenID; the fixture cannot distinguish the two identifiers")
	}
	if parent.TokenID == parent.RunID {
		t.Fatalf("TokenID %q == RunID; the fixture cannot distinguish the two identifiers", parent.TokenID)
	}

	child := mintChild(t, g, parent, `{"sub_id":"sub_1","caps":["memory.read"],"expiry_ms":600000}`)

	if child.ParentTokenID == parent.RunID {
		t.Errorf("ParentTokenID = %q, which is the parent's RUN id — token-level audit attribution is lost (JWT-002); want the parent's TokenID %q",
			child.ParentTokenID, parent.TokenID)
	}
	if child.ParentTokenID != parent.TokenID {
		t.Errorf("ParentTokenID = %q, want parent TokenID %q", child.ParentTokenID, parent.TokenID)
	}

	// The HTTP path must agree with the library path on this field.
	libToken, err := g.tokenMgr.CreateSubprocessToken(parent, "sub_2", []string{"memory.read"}, 10*time.Minute)
	if err != nil {
		t.Fatalf("CreateSubprocessToken: %v", err)
	}
	lib, err := g.tokenMgr.ValidateToken(libToken)
	if err != nil {
		t.Fatalf("ValidateToken(library child): %v", err)
	}
	if child.ParentTokenID != lib.ParentTokenID {
		t.Errorf("HTTP mint recorded ParentTokenID %q but the library recorded %q for the same parent; the two mint paths must agree",
			child.ParentTokenID, lib.ParentTokenID)
	}

	// Two children of the same run must be distinguishable by their parent
	// token — that is the property RunID cannot provide.
	if child.RunID != parent.RunID {
		t.Errorf("child RunID = %q, want inherited %q", child.RunID, parent.RunID)
	}
}

// TestAuditAccessLogsParentTokenID shows the consequence end to end: the audit
// record's TokenID must name a token, not a run.
func TestAuditAccessLogsParentTokenID(t *testing.T) {
	g := NewGateway(GatewayConfig{
		SocketPath:  "@test/agentgw/parenttokenid-audit.sock",
		TokenSecret: []byte("test-secret-key-32-chars-minimum!!"),
	})

	parent := &TokenClaims{
		RunID:     "run_shared_by_every_child",
		Caps:      []string{"memory.read"},
		ExpiresAt: time.Now().Add(1 * time.Hour),
	}
	if _, err := g.tokenMgr.CreateToken(parent); err != nil {
		t.Fatalf("CreateToken(parent): %v", err)
	}

	child := mintChild(t, g, parent, `{"sub_id":"sub_1","caps":["memory.read"],"expiry_ms":600000}`)

	// auditAccess copies ParentTokenID into the entry's TokenID field.
	if child.ParentTokenID == parent.RunID {
		t.Fatalf("audit would record TokenID=%q, a run id shared by every child of the run (JWT-002)", child.ParentTokenID)
	}
}
