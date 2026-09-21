# `features/api-keys/` — shared API-key entry primitives

Every place that accepts an API key, OAuth login, or env-pinned secret must go
through this module. Two reasons:

1. **One affordance, every page.** Reveal toggle, Enter-to-submit, "set" badge,
   pinned-notice, and "Get one ↗" link are the same across Setup wizard,
   Connections, Models & Keys, Voice Setup, and Channels. Operators only have
   to learn it once.
2. **One bug-fix surface.** Trim, fingerprint, set-state, and pinned-env
   handling live in one place. Adding a new key entry is now ~10 lines, not
   ~80.

## Public API

Import from `@/features/api-keys` — never from the inner files directly.

| Export            | When to use                                                                 |
| ----------------- | --------------------------------------------------------------------------- |
| `ApiKeyField`     | The default inline password input (label, reveal, Save). 99% of use cases.  |
| `KeyListItem`     | One row inside a "store many keys" list (label + last-4 + activate/remove). |
| `ChatGPTSignInCard` | OAuth-backed ChatGPT subscription login. Independent of any wizard.        |
| `useApiKeySubmit` | Hook wrapping `add` / `activate` / `remove` for the keyring store endpoint. |

## `ApiKeyField` rules

- Always controlled (`value` + `onChange`). `onSubmit` receives the trimmed
  value — never accept an empty/whitespace submission, the field already guards
  against that.
- Pass `isSet` whenever a key is already saved server-side. The placeholder
  becomes `•••••••• (set — type to replace)`.
- Pass `pinned` when the env is locked from outside (the daemon sees an
  `env_pinned` flag). The input is replaced by a "Set from the environment."
  notice and `onSubmit` is **not** invoked.
- Pass `link` whenever there's a public dashboard where the operator can
  generate the key. Convention: `link="https://platform.openai.com/api-keys"`.
- Pass `fingerprint` (last-4 or similar) when known — shown beside the env chip.
- Always pass `ariaLabel` if the field is not the only secret on the page —
  screen readers cannot disambiguate two `API key` inputs otherwise.

## `ChatGPTSignInCard` rules

- The card owns its own polling. Pass `onChanged` to react when the connected
  state flips (mount with an existing connection counts as a flip).
- `compact` drops the bottom status line — use it inside dense wizard steps.
- Do not duplicate this card inline. If you need a different OAuth provider,
  make a sibling card, not a forked copy.

## Adding a new API-key page

1. Import from `@/features/api-keys`.
2. If the page manages a list of keys, also import `useApiKeySubmit`.
3. Keep `onSubmit` async; `ApiKeyField` does not await it on purpose so the
   caller's loading state stays in sync with their own backend.
4. Do not add bespoke password inputs in the same page — refactor the page to
   use `ApiKeyField` instead.
