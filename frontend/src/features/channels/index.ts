// features/channels — Day 18 twenty-second feature carve-out.
//
// Owns:
//   - Channels page (Channels.tsx, 771 satır; carries the channel
//     management UI: ConnectForm + POPULAR_CHANNELS list +
//     ChannelRow type)
//   - ChannelSessions sub-page (ChannelSessions.tsx, 233 satır;
//     the per-channel sessions view; the React component used
//     by views/Chat/Chat.tsx for inline channel session rendering)
//   - lib/channelSessions.ts (92 satır; the channel session domain:
//     channelSessionStats + channelSessionHealthBadge + ChannelRow
//     type)
//
// Public surface (re-exported here so the rest of the codebase reads
// @/features/channels and never needs to know about the sub-paths):
//   - components/Channels.tsx — the page view
//   - ChannelSessions (re-exported) — used by views/Chat/Chat.tsx
//   - ConnectForm + POPULAR_CHANNELS (re-exported) — used by
//     views/Wizards.tsx as the create-channel wizard
//   - types.ts — ChannelRow
//
// The 1 lib file (channelSessions) is the feature's domain
// business logic; external code reaches it via
// @/features/channels/lib/channelSessions (allowed but not
// advertised).
//
// Cross-feature deps:
//   - @/app/api, @/app/events, @/app/utils, @/components/ui/*,
//     @/features/artifacts/lib/BlobArtifact (cross-feature) —
//     same pattern as the rest of the features
//
// The 771+233 = 1004-line page+sub-page pair is the biggest
// single risk in this feature; a future Day-26+ split should
// separate ConnectForm + formatters into components/Channels/form.tsx.
//
// See docs/FRONTEND-REFACTOR-PLAN.md for the carve-out rationale.
export { Channels } from "./components/Channels";
export { ChannelSessions } from "./components/ChannelSessions";
export {
  ConnectForm,
  POPULAR_CHANNELS,
} from "./components/Channels";
export * from "./types";
