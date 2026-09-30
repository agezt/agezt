// SPDX-License-Identifier: MIT

// Package bedrock is the in-process AWS Bedrock Provider.
//
// **Scope (M1.m):** bearer-token auth + Anthropic body shape only.
//
//   - Auth: AWS_BEARER_TOKEN_BEDROCK (long-lived, no SigV4 needed).
//     SigV4-signed requests (AWS_ACCESS_KEY_ID / AWS_SECRET_ACCESS_KEY)
//     land in M1.m.x.
//   - Body: Anthropic Messages API shape (the largest Bedrock use case
//     by usage). Other vendor body shapes — Mistral, Meta, Amazon Titan,
//     Cohere, AI21, DeepSeek — return ErrVendorUnsupported with a hint.
//
// Bedrock's HTTP wire:
//
//	POST https://bedrock-runtime.{region}.amazonaws.com/model/{modelID}/invoke
//	Authorization: Bearer {AWS_BEARER_TOKEN_BEDROCK}
//	Content-Type: application/json
//
// The model ID is interpolated into the URL path (not the request body),
// and the body carries `anthropic_version: "bedrock-2023-05-31"` instead
// of a `model` field. Otherwise the body is the same Messages-API shape
// the anthropic adapter speaks.
//
// Cross-region inference profiles (`us.anthropic.*`, `eu.anthropic.*`,
// `global.anthropic.*`, etc.) are recognised as Anthropic too — vendor
// detection looks for the `anthropic.` segment anywhere in the model id.
package bedrock
