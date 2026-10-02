// SPDX-License-Identifier: MIT

// Package market is AGEZT's capability marketplace: it packages skills, MCP
// servers, and CLI-tool requirements into installable "packs", catalogues them
// in "marketplaces" (a built-in Official one plus, later, synced remotes), and
// installs a pack by materializing its parts into the systems that already run
// them — skills into the Forge, MCP servers into the MCP registry, tool needs
// reported to the Toolbox. It deliberately reuses those subsystems rather than
// reimplementing capability execution; the marketplace is only discovery +
// packaging + install/sync on top.
//
// Manifest shapes mirror the Claude Code plugin.json / marketplace.json open
// standard (and agentskills.io bundles) so packs are portable across tools.
package market
