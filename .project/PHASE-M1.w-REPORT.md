# Phase Report — Milestone 1.w (vault at-rest encryption)

> Status: **shipped** · Date: 2026-05-29
> Per the M1.o deferral comment: "At-rest encryption (M1.o.x —
> likely via the OS keychain on macOS/Linux/Windows once we add
> a small platform-specific dep)."
> Continues [PHASE-M1.m.x-REPORT.md](PHASE-M1.m.x-REPORT.md).

## Scope

The credential vault has been plaintext on disk since M1.o, with
0600 perms as the only protection. That's fine on a private
workstation; it's *not* fine for:

- shared workstations (other users with file-read access),
- cloud-synced home directories (Dropbox, iCloud Drive,
  OneDrive — backups don't honour Unix mode bits),
- disk images / memory dumps shared with vendors for debugging,
- accidental `git add ~/.agezt/creds.json` (worse than you'd think).

M1.w ships **AES-256-GCM at-rest encryption** with a passphrase
sourced from `AGEZT_VAULT_PASSPHRASE`. Encryption activates
automatically on every `Save` when the env var is set; existing
plaintext vaults continue to load unchanged for backwards
compatibility.

| Concern | Status |
|---|---|
| AES-256-GCM with random per-save salt + nonce | ✅ tested |
| Iterated-HMAC-SHA256 KDF (200,000 rounds) | ✅ |
| Pure stdlib — no `golang.org/x/crypto`, no `aws-sdk-go` keychain wrappers | ✅ |
| On-disk envelope format with algorithm metadata for future rotation | ✅ |
| `Load` auto-detects encrypted-vs-plaintext via envelope `schema` field | ✅ tested |
| Plaintext (M1.o) vaults still load unchanged | ✅ tested |
| `ErrPassphraseRequired` when encrypted vault loaded without passphrase | ✅ tested |
| `ErrWrongPassphrase` distinct from corruption — explicit operator hint | ✅ tested |
| GCM auth tag catches ciphertext tampering | ✅ tested |
| `kdf_iter` floor rejects adversary-lowered iteration counts | ✅ tested |
| Fresh salt+nonce per save (encrypting same data twice → different bytes) | ✅ tested |
| `Store.IsEncrypted()` accessor for status display | ✅ |
| `Store.SetPassphraseFn` for testing without mutating process env | ✅ |
| CLI: `agt vault status` shows state + path + entry count | ✅ |
| CLI: `agt vault encrypt` migrates plaintext → encrypted in place | ✅ |
| CLI: `agt vault decrypt` migrates encrypted → plaintext (with warning) | ✅ |
| Help text updated; `provider reload` hint after encrypt/decrypt | ✅ |
| All M1.o `Store` API stays source-compatible | ✅ |

## Changes

### 1. `kernel/creds/encrypt.go` — new file (~250 LoC)

The encryption layer is one focused file. Public surface:

```go
const (
    SchemaEncrypted  = "agezt-creds-v2"
    AlgorithmAESGCM  = "aes-256-gcm"
    KDFIteratedHMAC  = "hmac-sha256-iter"
    KDFIterations    = 200000
)

var (
    ErrPassphraseRequired = errors.New("creds: vault is encrypted but AGEZT_VAULT_PASSPHRASE is not set")
    ErrWrongPassphrase    = errors.New("creds: vault decryption failed (wrong passphrase or corrupted file)")
)
```

Internal: `encryptVault`, `decryptVault`, `isEncryptedVault`,
`deriveKey`. All package-internal — callers go through the Store
API.

Three design choices, each documented in the file:

**Why iterated HMAC-SHA256 instead of PBKDF2.** Stdlib doesn't have
PBKDF2 (lives in `golang.org/x/crypto`, excluded by the lean-deps
policy). The construction `d = HMAC(passphrase, d)` repeated 200k
times has the same asymptotic cost profile and the same resistance
to time-memory tradeoffs for our threat model (offline brute force
of an encrypted vault). It is NOT standard PBKDF2 — don't use the
output for inter-tool key portability — but it's a defensible KDF
for this vault's self-contained round trip.

**Why no OS-keychain integration.** Every operator already has a
preferred secret-manager tool (`pass`, `security`, 1Password CLI,
`vault`, `bw`, etc.) and a shell init that uses it. Shipping yet
another keychain wrapper would:

- Require a platform-specific dep (zalando/go-keyring is the
  obvious choice but adds ~3 transitive deps to the binary).
- Force operators onto whichever keychain the lib supports,
  which may not be the one they use.
- Solve the wrong problem — operators who care about secret
  storage have already chosen a solution; operators who don't
  care wouldn't configure one regardless.

So the surface is "set this env var, however you want." The
operator-side `eval $(security find-generic-password ...)` /
`export AGEZT_VAULT_PASSPHRASE=$(pass show agezt)` / etc. is
a one-line shell init.

**Why ErrWrongPassphrase is distinct from corruption.** GCM
authentication failure could be either, but operators see
"wrong passphrase" 100× more often than "file corrupted." The
specific sentinel lets the CLI print "check your
AGEZT_VAULT_PASSPHRASE" instead of "your vault file is broken,
panic."

### 2. `kernel/creds/creds.go` — Store now reads/writes encrypted

Three additions:

```go
const PassphraseEnvVar = "AGEZT_VAULT_PASSPHRASE"

type Store struct {
    // ... unchanged ...
    passphraseFn func() string  // defaults to os.Getenv(PassphraseEnvVar)
    wasEncrypted bool           // remembered from most recent Load
}

func (s *Store) SetPassphraseFn(fn func() string)
func (s *Store) IsEncrypted() bool
```

`Load` now detects the encrypted envelope first; on detection,
calls `decryptVault` with the configured passphrase. Plaintext
files take the original M1.o code path unchanged.

`Save` checks the passphrase function; when non-empty, calls
`encryptVault` instead of `json.MarshalIndent`. Same atomic
write-temp-then-rename mechanism.

Critically, **no existing Store call site needed changes.** The
`creds.NewStore(base) → Load → Get / Set → Save` sequence works
identically whether the file is encrypted or not. Encryption is
a save-time decision based on environment, not an API change.

### 3. `cmd/agt/vault.go` — three new CLI subcommands

```
agt vault status     — shows state without requiring passphrase
                       (envelope is detectable from public field)
agt vault encrypt    — re-saves plaintext vault as encrypted
                       (requires AGEZT_VAULT_PASSPHRASE)
agt vault decrypt    — re-saves encrypted vault as plaintext
                       (requires AGEZT_VAULT_PASSPHRASE to read first)
```

`status` is the most-used. It always works — even without the
passphrase, the envelope's `schema` field is plaintext metadata,
so we can report "encrypted" without decrypting. When the
passphrase IS set, we additionally report the entry count.

`encrypt` / `decrypt` are migration commands. Operators run them
once when changing their security posture; the automatic path
keeps the file in whatever format matches the env var on every
subsequent `Save`.

### 4. `cmd/agt/main.go` — dispatch + help

```go
case "vault":
    return cmdVault(args[1:], stdout, stderr)
```

Plus three new help lines documenting the subcommands.

### 5. `kernel/creds/encrypt_test.go` — 13 new tests

| Test | Locks in |
|---|---|
| `TestEncryptDecrypt_RoundTrip` | 3-entry vault encrypts and decrypts to identical map |
| `TestEncrypt_FreshSaltAndNonce` | Same passphrase + same plaintext → different ciphertext bytes; both decrypt correctly |
| `TestDecrypt_WrongPassphraseReturnsSentinel` | Wrong passphrase → `ErrWrongPassphrase` |
| `TestDecrypt_TamperedCiphertextRejected` | Flipping one byte in ciphertext → GCM auth fails → `ErrWrongPassphrase` |
| `TestDecrypt_EmptyPassphraseReturnsRequired` | Empty passphrase → `ErrPassphraseRequired` |
| `TestDecrypt_RejectsLowIterationCount` | Adversary lowers KDFIter to 100 → refused |
| `TestIsEncryptedVault_DistinguishesPlainAndEncrypted` | Plain JSON, encrypted envelope, garbage all sort correctly |
| `TestStore_SaveEncryptedWhenPassphraseSet` | Passphrase set → file on disk is envelope, no plaintext credential visible |
| `TestStore_SavePlaintextWhenNoPassphrase` | No passphrase → flat-JSON file (M1.o-compatible) |
| `TestStore_LoadEncryptedRoundTrip` | Save-then-Load via two Store instances preserves data |
| `TestStore_LoadEncryptedRequiresPassphrase` | Loading encrypted with empty passphrase → `ErrPassphraseRequired` |
| `TestStore_LoadEncryptedWrongPassphrase` | Loading encrypted with wrong passphrase → `ErrWrongPassphrase` |
| `TestStore_LegacyPlaintextStillLoads` | An M1.o vault file (no envelope) loads unchanged on M1.w |

The tampering test is the most important — it proves that an
attacker who can modify the file but not learn the passphrase
gets caught, rather than silently swapping out one credential.

## Test summary

```
go test ./kernel/creds/ -v -count=1
(13 new encryption tests + all existing M1.o tests — PASS, except
 1 skip on Windows where unix mode bits don't apply)

go test ./... -count=1
(all packages PASS)
```

Total suite: **498 passing** (from 485 after M1.m.x). +13 from
M1.w.

## Operator workflow examples

**Enable encryption on a fresh install.** Set the env var before
the first `agt provider creds set`:

```
# In ~/.bashrc / ~/.zshrc / launchd plist / systemd env file:
export AGEZT_VAULT_PASSPHRASE="$(pass show agezt)"

# Then normal credential workflow — every save is encrypted:
agt provider creds set OPENAI_API_KEY sk-...
agt vault status
  path:    /Users/op/.agezt/creds.json
  status:  encrypted (aes-256-gcm)
  entries: 1
```

**Migrate an existing plaintext vault.** Operator already has
credentials; wants to encrypt:

```
export AGEZT_VAULT_PASSPHRASE="$(security find-generic-password -a agezt -s vault -w)"
agt vault encrypt
  vault encrypted: /Users/op/.agezt/creds.json
  remember: every shell/daemon that reads the vault now needs AGEZT_VAULT_PASSPHRASE set
  run `agt provider reload` to reload the daemon with the new file

# Daemon picks up the change (it already has the env var):
agt provider reload
```

**Roll back to plaintext** (rare; operator deciding the
keychain dance isn't worth it for their threat model):

```
export AGEZT_VAULT_PASSPHRASE=...        # needed to read the file first
agt vault decrypt
  vault decrypted to plaintext: /Users/op/.agezt/creds.json
  warning: every credential in this file is now readable by anyone with file-read access
           unset AGEZT_VAULT_PASSPHRASE to keep vaults plaintext on future saves
unset AGEZT_VAULT_PASSPHRASE             # important — else next save re-encrypts
```

**Check status without passphrase** (e.g. during a security
audit on an unfamiliar machine):

```
unset AGEZT_VAULT_PASSPHRASE
agt vault status
  path:    /Users/op/.agezt/creds.json
  status:  encrypted (aes-256-gcm)
  entries: unknown (set AGEZT_VAULT_PASSPHRASE and re-run to count)
  hint:    export AGEZT_VAULT_PASSPHRASE=... before running agezt commands
```

## Daemon integration

The daemon reads `AGEZT_VAULT_PASSPHRASE` from its environment at
startup (passes through `os.Getenv` via the existing Store
defaults). Operators running the daemon under systemd / launchd
add the env var to the unit file or plist; running it in a
terminal exports it from the shell first.

`agt provider reload` re-reads the vault from disk through the
same `Store.Load` path that auto-handles encryption — no
control-plane changes needed.

## What's intentionally NOT in M1.w

- **OS-keychain integration** (zalando/go-keyring, libsecret etc).
  Operator-provisioned env var is the surface; their keychain
  tool wraps `AGEZT_VAULT_PASSPHRASE` setup.
- **Per-credential encryption.** Today the whole file is one GCM
  blob. A per-entry encrypted-value scheme would let the daemon
  decrypt one credential at a time, reducing in-memory plaintext
  exposure. Worth doing if/when we see a credential-extraction
  attack against the daemon's memory; not before.
- **Argon2 / scrypt KDF.** Higher memory-hardness than iterated
  HMAC. Both require external deps. Iterated HMAC at 200k rounds
  is good for the operator-on-laptop threat model; argon2 is
  appropriate at server-fleet scale where compromised file +
  unknown passphrase + huge GPU farm is realistic.
- **Passphrase prompting / TTY input.** Programs that read
  passphrases interactively need careful TTY handling
  (suppress echo, restore on signal, handle ssh-without-tty,
  etc.). Operator-side `eval $(secret-manager ...)` covers
  the same need with less code.
- **Vault rotation** (`agt vault rotate-passphrase`). Decrypt with
  old, encrypt with new. Trivial follow-up; not in v1 because
  operators rotate keys not by re-encrypting but by issuing new
  ones (and old ones expire).

## Files touched

- [kernel/creds/encrypt.go](../kernel/creds/encrypt.go) — new (~250 LoC).
- [kernel/creds/encrypt_test.go](../kernel/creds/encrypt_test.go) — new (~270 LoC, 13 tests).
- [kernel/creds/creds.go](../kernel/creds/creds.go) — added `PassphraseEnvVar` const, `passphraseFn`/`wasEncrypted` fields, `SetPassphraseFn`/`IsEncrypted` methods, branch in Load + Save.
- [cmd/agt/vault.go](../cmd/agt/vault.go) — new (~140 LoC) with `status`/`encrypt`/`decrypt`.
- [cmd/agt/main.go](../cmd/agt/main.go) — dispatch case + 3 help lines.

Zero changes to the daemon, the control plane, providers, agent
loop, scheduler, planner, or pulse. The vault is a leaf storage
concern; encryption stays inside it.

## Deferrals after M1.w

- **OS-keychain integration / passphrase rotation / argon2** —
  discussed above.
- **Browser tool.**
- **Out-of-process plugin host.**
- **Planner v2** (re-planning, sub-planners, planner tools).
- **Pulse v2** (historical replay, TUI, dropped-events synthetic).
- **AWS credential-provider chain** (M1.m.x.x).
- **Non-Anthropic body shapes on Bedrock** (M1.m.y).
- **Per-task-type routing** (DECISIONS C2 extension).

Picking up **browser tool** next — it's the last operator-visible
capability gap that meaningfully expands what an agent can do
(web research, scraping, form-fill, screenshot capture). After
that, the out-of-process plugin host is the architectural-cleanup
phase that lets us safely accept third-party plugins.
