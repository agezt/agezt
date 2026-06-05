// SPDX-License-Identifier: MIT

package main

// Composes the operator-facing AWS credential resolution chain for
// the daemon. Encapsulated here so the boot path and the
// post-reload path use byte-identical logic.
//
// Optional opt-ins (M1.rr + M1.vv), in priority order, all gated
// by env vars so they're zero-cost when unused:
//
//	AGEZT_AWS_SSO_PROFILE             — read SSO config from this
//	                                     ~/.aws/config profile and
//	                                     resolve role creds via the
//	                                     SSO portal (M1.rr).
//	AGEZT_AWS_ASSUME_ROLE_ARN         — call sts:AssumeRole using
//	                                     the chain below as signing
//	                                     creds (M1.vv).
//	AWS_WEB_IDENTITY_TOKEN_FILE +     — IRSA / EKS Pod Identity:
//	AWS_ROLE_ARN                        sts:AssumeRoleWithWebIdentity,
//	                                     keyless, auto-detected (M1.ww).
//
// Lookup order:
//   1. vault (agezt-managed; operator overrides everything)
//   2. process env (`export FOO=...`)
//   3. SSO portal      [opt-in via AGEZT_AWS_SSO_PROFILE]
//   4. STS AssumeRole  [opt-in via AGEZT_AWS_ASSUME_ROLE_ARN]
//   5. Web identity    [auto via AWS_WEB_IDENTITY_TOKEN_FILE + AWS_ROLE_ARN]
//   6. AWS default chain (env / ~/.aws files / IMDS)
//
// 3–5 are AWS-name-only lookups; they return "" for non-AWS_*
// names so unrelated lookups fall through cleanly to 6.

import (
	"os"
	"strconv"
	"strings"

	"github.com/agezt/agezt/internal/brand"
	"github.com/agezt/agezt/kernel/creds"
	"github.com/agezt/agezt/kernel/creds/sigv4"
)

// buildAWSCredChain returns the composed lookup plus a short
// human-readable description of which opt-ins fired. The
// description is appended to the boot banner so operators can
// confirm assume-role / SSO actually engaged.
func buildAWSCredChain(vaultLookup func(string) string) (func(string) string, string) {
	var (
		descParts []string
		layers    []func(string) string
	)

	// 1 + 2: vault + env. Always present.
	layers = append(layers, vaultLookup, os.Getenv)

	// 3: SSO opt-in. Refuse to engage silently — if the operator
	// asked for SSO but the profile is missing/malformed, surface
	// that in the boot banner so they notice.
	if profile := strings.TrimSpace(os.Getenv(brand.EnvPrefix + "AWS_SSO_PROFILE")); profile != "" {
		if params, ok := creds.LoadSSOParamsFromProfile(profile); ok {
			layers = append(layers, creds.AWSSSOLookup(params))
			descParts = append(descParts, "sso="+profile)
		} else {
			descParts = append(descParts, "sso="+profile+"(no-config)")
		}
	}

	// 4: AssumeRole opt-in. Signing creds for the STS call come
	// from the chain *below* this layer (so vault/env/file/IMDS
	// can all be the signing-cred source). We build a sub-chain
	// for that explicitly rather than recursing into our own
	// caller — recursion would deadlock the cache mutex if STS
	// itself probed AWS_ASSUME_ROLE_ARN.
	if roleArn := strings.TrimSpace(os.Getenv(brand.EnvPrefix + "AWS_ASSUME_ROLE_ARN")); roleArn != "" {
		baseChain := creds.ChainLookup(vaultLookup, os.Getenv, creds.AWSDefaultChain())
		signingCreds := sigv4.Creds{
			AccessKeyID:     baseChain("AWS_ACCESS_KEY_ID"),
			SecretAccessKey: baseChain("AWS_SECRET_ACCESS_KEY"),
			SessionToken:    baseChain("AWS_SESSION_TOKEN"),
		}
		region := baseChain("AWS_REGION")
		if region == "" {
			region = baseChain("AWS_DEFAULT_REGION")
		}
		duration := parseAssumeRoleDurationSeconds(os.Getenv(brand.EnvPrefix + "AWS_ASSUME_ROLE_DURATION_SECONDS"))
		params := creds.AssumeRoleParams{
			Region:          region,
			BaseCreds:       signingCreds,
			RoleArn:         roleArn,
			RoleSessionName: strings.TrimSpace(os.Getenv(brand.EnvPrefix + "AWS_ASSUME_ROLE_SESSION_NAME")),
			DurationSeconds: duration,
			ExternalID:      strings.TrimSpace(os.Getenv(brand.EnvPrefix + "AWS_ASSUME_ROLE_EXTERNAL_ID")),
		}
		layers = append(layers, creds.AWSAssumeRoleLookup(params))
		descParts = append(descParts, "assume_role="+shortArn(roleArn))
	}

	// 5: Web identity (IRSA / EKS Pod Identity). Auto-activates on the
	// standard AWS_WEB_IDENTITY_TOKEN_FILE + AWS_ROLE_ARN env vars that
	// EKS injects — no agezt-specific config, the SDK-native ambient path.
	// Placed BEFORE the default chain so a pod assumes its OWN role rather
	// than falling through to the node's IMDS instance-profile role.
	if tokenFile := strings.TrimSpace(os.Getenv("AWS_WEB_IDENTITY_TOKEN_FILE")); tokenFile != "" {
		if roleArn := strings.TrimSpace(os.Getenv("AWS_ROLE_ARN")); roleArn != "" {
			region := strings.TrimSpace(os.Getenv("AWS_REGION"))
			if region == "" {
				region = strings.TrimSpace(os.Getenv("AWS_DEFAULT_REGION"))
			}
			params := creds.WebIdentityParams{
				Region:          region,
				RoleArn:         roleArn,
				RoleSessionName: strings.TrimSpace(os.Getenv("AWS_ROLE_SESSION_NAME")),
				TokenFile:       tokenFile,
			}
			layers = append(layers, creds.AWSWebIdentityLookup(params))
			descParts = append(descParts, "web_identity="+shortArn(roleArn))
		}
	}

	// 6: default chain (env / ~/.aws files / IMDS).
	layers = append(layers, creds.AWSDefaultChain())

	desc := "AWS chain: vault → env → default(file+IMDS)"
	if len(descParts) > 0 {
		desc = "AWS chain: vault → env → " + strings.Join(descParts, " → ") + " → default(file+IMDS)"
	}
	return creds.ChainLookup(layers...), desc
}

// parseAssumeRoleDurationSeconds parses the optional STS AssumeRole session
// duration from its env string. A missing, malformed, zero, or NEGATIVE value
// returns 0, which kernel/creds maps to the AWS default (3600s). The >0 guard
// matters: kernel/creds/sts.go only substitutes the default for an exact 0, so
// a negative (e.g. a typo'd "-3600") would otherwise be sent to STS verbatim and
// rejected with a ValidationError at first credential resolution — a runtime
// failure of the whole AWS chain instead of a graceful fallback. Mirrors the >0
// guard every duration parse in main.go uses.
func parseAssumeRoleDurationSeconds(v string) int {
	if n, err := strconv.Atoi(strings.TrimSpace(v)); err == nil && n > 0 {
		return n
	}
	return 0
}

// shortArn truncates the role ARN to the last path component so the
// banner line stays readable. Full ARNs are >60 chars; the role
// name alone is enough for the operator to recognise.
func shortArn(arn string) string {
	if i := strings.LastIndex(arn, "/"); i >= 0 && i < len(arn)-1 {
		return arn[i+1:]
	}
	return arn
}
