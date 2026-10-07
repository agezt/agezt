// SPDX-License-Identifier: MIT
package event

// WireSchema is the canonical JSON envelope declaration for kernel event frames.
// Domain payloads remain owned by their producers.
const WireSchema = `{"type":"object","additionalProperties":true,"properties":{"id":{"type":"string"},"seq":{"type":"integer"},"ts_unix_ms":{"type":"integer"},"prev_hash":{"type":"string"},"hash":{"type":"string"},"subject":{"type":"string"},"actor":{"type":"string"},"kind":{"type":"string"},"correlation_id":{"type":"string"},"causation_id":{"type":"string"},"payload":{},"tags":{"type":"object","additionalProperties":{"type":"string"}}},"required":["id","seq","ts_unix_ms","prev_hash","subject","actor","kind"]}`
