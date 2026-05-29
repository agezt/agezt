# Phase Report — Milestone 1.ee (Vault passphrase rotation)

> Status: **shipped** · Date: 2026-05-29
> Closes the M1.w "passphrase rotation" deferral.
> Continues [PHASE-M1.dd-REPORT.md](PHASE-M1.dd-REPORT.md).

## Scope

M1.w shipped at-rest vault encryption (AES-256-GCM, iterated
HMAC-SHA256 KDF, 200k rounds). The deferral list noted:

> Vault: OS-keychain integration, passphrase rotation, argon2.

Of those three, **passphrase rotation** is the one operators
actually hit. The current behaviour: an operator who wants to
change their vault passphrase has no first-class command — they
have to `agt vault decrypt`, swap the env var, then `agt vault
encrypt`. That works but leaves a window where the vault is
plaintext on disk. M1.ee ships the proper atomic operation.

```
export AGEZT_VAULT_PASSPHRASE=old-pass
export AGEZT_VAULT_PASSPHRASE_NEW=new-pass
agt vault rotate
# vault is now encrypted under new-pass; no plaintext window
```

The on-disk file is replaced atomically (write-tmp + rename) so
even a crash mid-rotation leaves either the old vault intact OR
the new vault intact — never a half-written file. Every entry
survives; the only thing that changes is the encryption envelope's
salt + nonce + ciphertext (and therefore the key the ciphertext
opens under).

| Concern | Status |
|---|---|
| `Store.Rotate(newPassphrase string)` — atomic re-encrypt | ✅ |
| Refuses empty new passphrase (force operator through `decrypt`) | ✅ tested |
| Holds write lock for the whole operation (no concurrent inconsistency) | ✅ |
| Failed Rotate leaves the on-disk file byte-identical | ✅ tested |
| Successful Rotate updates the in-memory passphrase fn (no env-var follow-up needed) | ✅ tested |
| Successive Rotates produce fresh salt + nonce | ✅ tested |
| New passphrase decrypts; old passphrase returns `ErrWrongPassphrase` | ✅ tested |
| `agt vault rotate` subcommand wired with two-env-var protocol | ✅ |
| Refuses rotation on plaintext vaults (clear "use `vault encrypt`" hint) | ✅ |
| Refuses identical current=new passphrase (catches operator typo) | ✅ |
| Refuses missing either env var (clear which one) | ✅ |
| Help text + status line updated | ✅ |

## Why a two-env-var protocol

`agt vault rotate` requires BOTH `AGEZT_VAULT_PASSPHRASE` (current)
and `AGEZT_VAULT_PASSPHRASE_NEW` (target). Three alternatives
considered + rejected:

- **Read the new passphrase from stdin / a TTY prompt.** Tempting
  for a "no env-var-with-secret" story, but it'd be the only `agt`
  subcommand that ever prompts interactively. The rest of the
  toolchain assumes batch / scripted execution. Operators using
  1Password / Vault / `pass` already have a clean shell pattern
  for piping a secret into an env var (`export X=$(op item get ...)`),
  so the env-var path doesn't sacrifice anything.
- **Same env var, swap before and after.** Operators would do
  `AGEZT_VAULT_PASSPHRASE=old agt vault rotate-prepare` then
  `AGEZT_VAULT_PASSPHRASE=new agt vault rotate-commit`. Too easy
  to forget step 2 and end up locked out.
- **Pass new passphrase as a `--new-passphrase` flag.** Visible in
  `ps auxf` / shell history; defeats the whole point of putting
  secrets in env vars in the first place.

The two-env-var protocol mirrors how operators already manage the
single passphrase: shell rc / systemd unit / 1Password CLI sets
them, the command consumes them, the operator unsets the temporary
one. No new mental model.

## Files

### `kernel/creds/creds.go` — added `Rotate` + new env var const

```go
const NewPassphraseEnvVar = "AGEZT_VAULT_PASSPHRASE_NEW"

func (s *Store) Rotate(newPassphrase string) error
```

`Rotate` holds `s.mu` for the whole operation: validate → encrypt
in-memory data under the new passphrase → atomic write → swap the
in-memory passphrase function. A failed rotation leaves the
on-disk file untouched AND the in-memory Store still usable under
the old passphrase — the only invariant we promise is "either the
operation succeeded fully or no observable state changed."

Crucially, the in-memory `passphraseFn` is updated **after** the
atomic rename. If we'd updated it first, a rename failure would
leave the in-memory state pointing at a passphrase the on-disk file
doesn't open under, so the next Save would re-encrypt under one
passphrase while the operator's env var still says another. The
ordering here is the small-but-important invariant.

### `kernel/creds/rotate_test.go` — new (~190 LoC, 5 tests)

| Test | Locks in |
|---|---|
| `TestStore_RotateRoundTrip` | After rotate: new passphrase loads; old returns `ErrWrongPassphrase` |
| `TestStore_RotateRejectsEmptyPassphrase` | Won't silently downgrade to plaintext; force operator through `vault decrypt` if that's truly the intent |
| `TestStore_RotateLeavesFileUnchangedOnPreFlightError` | Rejected rotation → byte-identical file on disk (no truncate, no temp leak) |
| `TestStore_RotateUpdatesInMemoryPassphrase` | After Rotate, subsequent `Save()` calls work without updating the env var |
| `TestStore_RotateAlwaysProducesFreshSalt` | Two consecutive Rotates → byte-different files (fresh salt + nonce per save) |

Tests live in `package creds` (not `creds_test`) — same convention
as `encrypt_test.go` — so they can read the encryption internals
directly without an extra export-just-for-tests surface.

### `cmd/agt/vault.go` — added `agt vault rotate`

`cmdVaultRotate`:

1. Reads both env vars, errors clearly if either is missing.
2. Refuses identical current/new (catches typos).
3. Loads vault under current passphrase. If the load fails with
   `ErrWrongPassphrase`, the error message already names it; the
   command returns 1 and the operator sees what's wrong.
4. Refuses rotation on plaintext vaults — directs them at
   `agt vault encrypt` instead.
5. Calls `Store.Rotate(new)`.
6. Prints a three-step "what to do next" checklist so the operator
   doesn't forget to update their shell rc / unset the temp env /
   reload the daemon.

The dispatcher (`cmdVault`) and help text were updated to include
the new subcommand.

## Operator workflow examples

**Routine rotation:**

```bash
# Step 1: rotate the file
export AGEZT_VAULT_PASSPHRASE=old-pass
export AGEZT_VAULT_PASSPHRASE_NEW=new-pass
agt vault rotate
# vault re-encrypted under new passphrase: /home/me/.agezt/creds.json
# entries preserved: 7

# Step 2: update your shell rc / 1Password / systemd unit
# (replace AGEZT_VAULT_PASSPHRASE=old-pass with =new-pass)

# Step 3: unset the temporary
unset AGEZT_VAULT_PASSPHRASE_NEW

# Step 4: reload the daemon (it still has the old key in memory)
agt provider reload
```

**Suspected compromise — fast rotation:**

```bash
# Generate a fresh passphrase
AGEZT_VAULT_PASSPHRASE_NEW=$(openssl rand -base64 32) \
AGEZT_VAULT_PASSPHRASE=oldpass \
agt vault rotate
# Then immediately rotate every credential inside the vault, since
# whoever had the old vault passphrase also had its contents.
```

(The second half — rotating each API key with its respective
provider — is outside agezt's scope. The vault rotation buys
fresh at-rest protection; it doesn't undo whatever the attacker
already exfiltrated.)

## Test summary

```
go test ./kernel/creds/ -v -count=1 -run TestStore_Rotate
(5 tests — all PASS)

go test ./... -count=1
(35 packages — all PASS, no regressions)
```

+5 from M1.ee.

## What's intentionally NOT in M1.ee

- **OS-keychain integration.** Still on the M1.w deferral list.
  Every operator already has their preferred secret manager
  (`security`, `pass`, `cmdkey`, 1Password CLI, etc.); shipping
  yet another integration would invite "why doesn't agezt pick
  mine?" without adding capability the operator can't already
  script into their shell init. Decision unchanged.
- **Argon2 KDF.** The current iterated-HMAC-SHA256 KDF is sound
  for the offline-brute-force threat model. Argon2 would be more
  memory-hard (better against GPU/ASIC attackers) but isn't in
  stdlib — pulling in `golang.org/x/crypto` for one function
  violates the lean-deps policy. If a real demonstrated attack
  motivates it, that's a different conversation; today the KDF
  isn't the weakest link.
- **Rotation without env vars (TTY prompt).** Discussed above
  under "Why a two-env-var protocol." Rejected.
- **Schedule-based rotation reminders.** Some teams have policies
  ("rotate every 90 days"). Could be a `agt vault rotate-by` cron
  hint or a daemon banner warning when the salt is old. Not
  shipped; operator's policy engine + their existing scheduling
  story (cron, systemd timers) handles it better than agezt would.
- **Multi-key envelopes.** Could let operators decrypt under any
  of N keys for transition periods. Not shipped — the rotation
  story above (rotate + reload + verify) is short enough that a
  multi-key window adds complexity for negligible time savings.
- **Rotate-and-rekey-individual-secrets.** Out of scope; that's a
  per-provider concern (call OpenAI's key-rotation API, etc.)
  that the kernel has no business orchestrating.

## Files touched

- [kernel/creds/creds.go](../kernel/creds/creds.go) — added `NewPassphraseEnvVar` constant + `Store.Rotate` method.
- [kernel/creds/rotate_test.go](../kernel/creds/rotate_test.go) — new, 5 rotation tests.
- [cmd/agt/vault.go](../cmd/agt/vault.go) — added `agt vault rotate` subcommand + help.

Zero changes to encrypt.go, daemon, governor, providers, or any
other package. Rotation sits cleanly on top of M1.w's existing
encryption primitives.

## Deferrals after M1.ee

Unchanged from M1.dd, minus the passphrase-rotation item just
shipped:

- Pulse v3+ (TUI dashboard, until/last flags, replay rate limit,
  subject indexing).
- Planner v2 (re-planning, sub-planners, planner-side tools).
- Plugin sandboxing, signing, hot-reload, streaming, callbacks.
- Browser: JS rendering, screenshots, search, cookies.
- Non-Anthropic body shapes on Bedrock (multi-vendor per-vendor
  scope — Mistral chat / Cohere R+ / Llama prompt-template, each
  with distinct tool-use semantics; deferred until concrete
  operator demand surfaces).
- Vault: OS-keychain integration, argon2 (unchanged from above).
- MCP bridge v2 (resources/sampling/progress/cancellation/SSE/image).
- Routing extensions (TaskRouteRequire, per-task-type budgets,
  per-task-type model overrides) — wait for demand.
- AWS extensions (SSO, assume-role, web identity, process creds,
  IMDSv1) — wait for demand.
