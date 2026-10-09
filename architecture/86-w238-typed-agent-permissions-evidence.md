# W2.38a typed agent permissions

The last two agent-adjacent native commands now run on the shared dispatcher in
`kernel/app/roster`, beside the other roster operations:

- `agent_permissions` explains which tools and config entries an agent may use,
  how it can be woken, and its governance summary;
- `agent_capabilities` patches the agent-local capability surface (trust
  ceiling, tool allow and deny lists, noise policy, config overrides, memory
  scope, workdir and cost caps), then reports the same picture with the patched
  profile.

`PermissionService` works over `PermissionPorts` bound to the primary kernel:

- the roster's `Get` and the journaled `UpdateProfile`;
- the registered tools;
- the policy engine's `DecideWithCeiling`;
- the config center's entries, or none when the daemon has no config center.

`PermissionOperations` declares two primary-only, primary-tenancy specs with
unknown input allowed:

- `agent_permissions`, read-only, on `GET /api/agents/permissions`;
- `agent_capabilities`, audited, on `POST /api/agents/capabilities`.

These are the native registration's flags, and the routes match the Web UI.

The config-ownership predicate moves with the permission picture and is
exported as `ConfigEntryBelongsToAgent`. Agent teardown, which prunes the same
entries, now calls it there.

Removed with the move:

- `tool_agents.go`, `tool_views.go` and `tool_views_governance.go`, which held
  the native handlers, the patch decoder and the view helpers;
- `registerMiscSmallCommands`, whose last two entries these were;
- the native `validateAgentHierarchyRefs` and `profileView` wrappers, whose
  last users these handlers were.

## Preserved behavior

- `ref` is strict: a non-string or `null` is an error, and a blank one is
  required. The ref passes untrimmed, so the roster decides what matches.
- Tool rows follow registration name order. The agent denylist wins, then a
  non-empty allowlist hides the rest. Lists match trimmed and case-insensitive.
  Every other tool is decided by the policy engine under the agent's trust
  ceiling; an empty or unparsable ceiling decides at allow.
- Only policy-decided rows carry `hard_denied`, and `requires_approval` appears
  only when set. A row asks when the engine would ask or requires approval. Its
  status is `denied`, the ask level, or `allowed`.
- Config rows follow key order:
  - exclusion wins;
  - an entry with no allowed agents is global;
  - otherwise the agent must be listed;
  - agent names match trimmed and case-insensitive;
  - `owned` and `description` appear only when set.
- `config_entries` is `null` without a config center. `tool_allow` and
  `tool_deny` are `null` when empty. The tool, delegation and config-key lists
  are always arrays.
- Wake access:
  - retired wins over paused, and paused over managed;
  - the manager is the parent, else the owner;
  - delegation sources are the owner, then a distinct parent.
- Governance:
  - the trust default is L4;
  - risk is open, governed, restricted or `system_guardian`;
  - the tool, memory and noise policies, and the summary and permission
    passport strings, are unchanged;
  - a system guardian is held quiet: silent on success, no memory writes, at
    least warning severity, at most one notification per eight hours.
- Capability patches keep their order:
  1. the ref;
  2. the agent lookup;
  3. each sent field's decode, in declared order (`null` sends the zero value
     and still counts);
  4. `args capability field required` when nothing was sent;
  5. hierarchy validation against the live roster;
  6. the journaled update, with roster errors passed through, and an agent
     removed meanwhile reported as unknown.

  An empty override map clears the overrides. Lists and maps are copied.
- Any `tenant` argument is ignored, and tenant tokens are refused.

The only intended difference is the shared already-canceled admission. It
rejects an authorized call before any audit or change.

## Runnable comparison and regression evidence

The harness clones a fixture kernel and opens it with:

- ten registered tools, one with an advertised name apart from its
  registration name;
- four policy levels (allow, ask, deny, ask-scoped) beside the default;
- six config entries covering every visibility and ownership rule;
- ten profiles: plain, guarded (allow and deny lists, an L2 ceiling, a memory
  scope, a noise policy and a 2^53+1 cost cap), managed with distinct and with
  equal owner and parent, paused, denylist-only, one whose owner is retired,
  retired, and two system guardians.

It runs the pre-slice handlers on one copy and the registered operations on
another.

294 steps repeated twenty times cover eight sequences under primary, wrong and
tenant tokens, in normal and canceled contexts:

- every profile's picture;
- every ref error, an unknown agent, padded and upper-case refs, a named tenant
  and secret-named arguments;
- trust and list patches with re-reads, an unparsable ceiling and `null`
  fields;
- a full patch, then clearing overrides, then `null` resets;
- every decode error, an empty patch and unrelated keys;
- type errors and roster validation (bad override value, unsafe workdir);
- hierarchy, retired, system and managed targets, and a padded ref;
- tenant and secret arguments on a patch.

The comparison checks three things:

- responses are byte-exact, with only wall-clock times and ids hidden;
- the roster state is equal;
- both journals, grouped by correlation, are equal: the operation audit and
  the profile updates.

The harness asserts that the fixtures reach:

- restricted, `system_guardian` and denylist governance;
- agent-deny rows, `hard_denied`, the alias tool, owned and described config
  rows;
- managed and deduplicated delegation;
- paused and retired wake access;
- the guardian severity clamp;
- a landed patch.

The exceptions are 49 canceled primary-token steps. These return the admission
error and leave no audit record or roster change.

Permanent tests cover:

- ref errors before any lookup, for both operations, and the untrimmed ref;
- row order, deny over allow, hidden rows, and the ceiling, ask, status and
  optional-field rules;
- config rows with no center, an empty center, order, exclusion, matching and
  ownership;
- every ownership rule of `ConfigEntryBelongsToAgent`;
- every wake-access state;
- every governance risk, policy and string, with the guardian clamps;
- the capability patch's order, decode errors, empty patch, hierarchy check,
  update error and miss, `null` fields, clearing and copying;
- the specs and output schemas.

Native tests cover the registry flags. A native round trip checks that both
operations act on the primary roster whatever tenant is named, that the patch
is audited, and that a tenant token is refused. The existing permission and
capability suites pass unchanged through the typed path. So do the `agt doctor`
guardian tests, which build the quiet patch that doctor sends as
`agent_capabilities`.

Sixty-two independent mutations fail tests, among them the native registration,
the ceiling binding and the journaled update binding. One case is recorded and
not mutated: the native binding's guard for a daemon without a config center.
A runtime kernel always opens one, so only the app tests' fake port reaches
that branch.

Sources are restored byte-for-byte. Fixtures use isolated temporary kernels
only.
