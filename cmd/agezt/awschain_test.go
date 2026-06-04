// SPDX-License-Identifier: MIT

package main

import (
	"strings"
	"testing"
)

// TestBuildAWSCredChain_WebIdentityAutoActivates: the standard EKS-injected
// env vars (AWS_WEB_IDENTITY_TOKEN_FILE + AWS_ROLE_ARN) turn on the IRSA layer
// with NO agezt-specific opt-in, and the boot-banner description reports it.
// Construction is network-free (the lookup is lazy), so no STS call happens.
func TestBuildAWSCredChain_WebIdentityAutoActivates(t *testing.T) {
	t.Setenv("AWS_WEB_IDENTITY_TOKEN_FILE", "/var/run/secrets/eks.amazonaws.com/serviceaccount/token")
	t.Setenv("AWS_ROLE_ARN", "arn:aws:iam::123456789012:role/EksPodRole")
	// Keep other opt-ins from bleeding in from the host env.
	t.Setenv("AGEZT_AWS_SSO_PROFILE", "")
	t.Setenv("AGEZT_AWS_ASSUME_ROLE_ARN", "")

	_, desc := buildAWSCredChain(func(string) string { return "" })
	if !strings.Contains(desc, "web_identity=EksPodRole") {
		t.Errorf("desc should report the web_identity layer; got %q", desc)
	}
}

// TestBuildAWSCredChain_NoWebIdentityWithoutRoleArn: a token-file path alone
// (no AWS_ROLE_ARN) must NOT activate the layer — we can't assume a role
// without knowing which one.
func TestBuildAWSCredChain_NoWebIdentityWithoutRoleArn(t *testing.T) {
	t.Setenv("AWS_WEB_IDENTITY_TOKEN_FILE", "/some/token")
	t.Setenv("AWS_ROLE_ARN", "")
	t.Setenv("AGEZT_AWS_SSO_PROFILE", "")
	t.Setenv("AGEZT_AWS_ASSUME_ROLE_ARN", "")

	_, desc := buildAWSCredChain(func(string) string { return "" })
	if strings.Contains(desc, "web_identity") {
		t.Errorf("web_identity must not activate without AWS_ROLE_ARN; got %q", desc)
	}
}
