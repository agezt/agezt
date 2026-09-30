// SPDX-License-Identifier: MIT

// Package builtinchannels registers the manifests for AGEZT's built-in
// communication channels (Telegram, WhatsApp, Slack, …) into the channel
// registry. It's the single place that describes every shipped channel for the
// Channels wizard — kept out of the kernel (which must not import plugins) and
// out of the per-channel packages (which stay transport-only). Adding a new
// built-in channel = one entry here plus its Config Center section; a future
// out-of-tree channel can call channel.RegisterManifest itself.
package builtinchannels
