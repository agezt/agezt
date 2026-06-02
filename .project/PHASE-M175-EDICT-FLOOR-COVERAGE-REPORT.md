# M175 — Wider hard-deny floor: block-device writes, wipefs, poweroff

## Why
Follow-up to the M173 Edict review (MEDIUM-2): the default hard-deny floor
(`DefaultHardDeny`) advertised protection against disk-wipe / power operations but
missed several even in plain form. Most notably `dd-of-dev` keyed on `dd if=`
(a source operand), not on the actually-destructive `of=/dev/<disk>` target, so a
`dd of=/dev/sdb` with no `if=` slipped through; `wipefs` and `poweroff`/`halt`
families had no rule at all.

## What
Added to `DefaultHardDeny` (all scoped to `CapShell`):
- `dd-of-sd`, `dd-of-nvme`, `dd-of-vd`, `dd-of-xvd`, `dd-of-mmcblk` — substring
  `of=/dev/<family>`, catching a `dd` (or any command) that writes to a raw block
  device, including the no-`if=` shape the old rule missed. The common cloud/VM
  device families are covered (sd/nvme/vd, AWS Xen xvd, SD/eMMC mmcblk).
- `wipefs` — wiping filesystem signatures; no benign agent use.
- `poweroff` — joins the existing `shutdown -` / `reboot` power-operation rules.

### False-positive safety (the floor has no override)
The device rules key on the dangerous *families* and deliberately do NOT match the
safe pseudo-devices `/dev/null`, `/dev/zero`, `/dev/random`, so benign
`dd of=/dev/null` and `echo … > /dev/null` remain allowed. This matters because a
hard-deny is non-overridable — a false positive would permanently block a
legitimate command. Composes with M173: the candidates matched are the
decoded/whitespace-normalized forms, so `of=/dev/sd` is still caught when padded or
JSON-escaped.

Substring matching remains best-effort vs semantic rewrites (the M173 caveat
stands); these additions raise coverage of the catastrophic-and-unambiguous cases.

## Tests (+1)
`TestDefaultHardDeny_DeviceAndPower` — denies `dd if=/dev/zero of=/dev/sdb`,
`dd of=/dev/nvme0n1` (no if=), `dd of=/dev/vda`, `dd of=/dev/xvdf`, `wipefs -a
/dev/sda`, `sudo poweroff`; ALLOWS `dd of=/dev/null if=/dev/zero`,
`echo hello > /dev/null`, `cat /proc/cpuinfo` (no false denies).

## Verification
- `go.mod` / `go.sum` unchanged; no new protocol command, env var, or event kind.
- `go vet ./...` clean; `GOOS=linux go build ./...` ok; `gofmt -l` clean.
- `go test ./... -count=1` — FAIL 0, 1556 tests (was 1555; +1), 61 packages.

## Result
The non-overridable floor now covers raw block-device writes (the actual
destructive operand), filesystem-signature wiping, and power-off — without
introducing a false deny on the safe pseudo-devices.
