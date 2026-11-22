// index.ts — barrel for the shared API-key primitives. Importing from
// `@/components/apikeys` keeps callers decoupled from the file layout.
//
// These primitives are used by five views (Channels, Connections, Models,
// Setup and Voice), so they are not a feature of their own: a feature's
// siblings may not reach into another feature, and the shared layer is where
// something five views share belongs (W5.3).

export { ApiKeyField } from "./ApiKeyField";
export { KeyListItem, type KeyInfo } from "./KeyListItem";
export { ChatGPTSignInCard } from "./ChatGPTSignInCard";
export { useApiKeySubmit } from "./useApiKeySubmit";
