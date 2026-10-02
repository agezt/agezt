// SPDX-License-Identifier: MIT

// Package agentgw provides a secure gateway for AI agent subprocess code
// to communicate with the AGEZT kernel. It exposes scoped HTTP/WebSocket
// endpoints validated by JWT capability tokens.
//
// Capability namespaces exposed to agent code:
//   - eventbus: publish, subscribe
//   - channel: send, read, list
//   - memory: read, write, delete, search, list
//   - log: read, write
//   - agent: list, query
//   - db: query, read, write
package agentgw
