// features/artifacts — Day 17 twentieth feature carve-out.
//
// Owns:
//   - Artifacts page (Artifacts.tsx, 617 satır; carries the
//     artifact explorer UI: the artifact list + per-artifact
//     detail + BlobArtifact component for blob rendering)
//   - lib/artifacts.tsx (174 satır; the artifact domain: isImage +
//     rawURL + isPdf + textKind + isRunInternal + BlobArtifact
//     component + categoryOf + ArtifactEntry type + the looks-like-
//     path test for the path heuristic)
//
// Public surface (re-exported here so the rest of the codebase reads
// @/features/artifacts and never needs to know about the sub-paths):
//   - components/Artifacts.tsx — the page view
//   - BlobArtifact (re-exported from lib) — the React component
//     for rendering blob artifacts; used by views/Inbox.tsx +
//     views/ChannelSessions.tsx for inline artifact rendering
//   - The 5 helpers (isImage, rawURL, isPdf, textKind,
//     isRunInternal) — directly tested in lib/artifacts.test.tsx
//     and used by FileManagerWorkspace
//   - categoryOf (re-exported) — used by tests
//   - types.ts — ArtifactEntry (the cross-feature type)
//
// The 1 lib file (artifacts) is the feature's domain business
// logic; external code reaches it via
// @/features/artifacts/lib/artifacts (allowed but not advertised).
//
// Cross-feature deps:
//   - @/app/api, @/app/events, @/app/utils, @/components/ui/*
//     (shared UI primitives — same pattern as the rest of the
//     features)
//
// The 617-line Artifacts.tsx is small; no immediate split needed
// beyond the existing lib/components split.
//
// See docs/FRONTEND-REFACTOR-PLAN.md for the carve-out rationale.
export { Artifacts } from "./components/Artifacts";
export { BlobArtifact } from "./lib/artifacts";
export {
  isImage,
  rawURL,
  isPdf,
  textKind,
  isRunInternal,
  categoryOf,
} from "./lib/artifacts";
export * from "./types";
