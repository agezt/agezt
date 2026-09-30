// SPDX-License-Identifier: MIT

// Package sigv4 implements the AWS Signature Version 4 algorithm.
//
// Extracted from plugins/providers/bedrock during M1.SigV4 so that
// non-Bedrock AWS services (sts:AssumeRole, sso:GetRoleCredentials,
// future S3/DynamoDB/etc.) can sign their own requests without
// duplicating the (subtle and easy-to-get-wrong) canonicalisation.
//
// References:
//
//	https://docs.aws.amazon.com/general/latest/gr/sigv4-create-canonical-request.html
//	https://docs.aws.amazon.com/general/latest/gr/sigv4-create-string-to-sign.html
//	https://docs.aws.amazon.com/general/latest/gr/sigv4-calculate-signature.html
//
// Scope is unchanged from the bedrock-internal version:
//   - POST/GET with a single signed request (no event-stream chunked
//     signing — that's only needed for upload streaming).
//   - Static credentials (AKID + secret + optional STS session token).
//     Higher-level providers (the AWS credential chain in
//     kernel/creds/aws.go) handle discovery and rotation; this
//     package only signs.
//
// Concurrency: SignRequest is pure — it mutates the passed *http.Request
// in place but holds no shared state. Safe to call concurrently with
// different requests.
package sigv4
