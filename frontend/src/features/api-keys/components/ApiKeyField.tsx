// ApiKeyField.tsx — the one inline-password input that every API key entry page
// uses. Owns the reveal toggle, "set" / "pinned" badges, "Get one" link, and the
// Enter-to-submit behaviour so callers can stay declarative.
//
// Visual contract (consistent across Connections / Setup / Voice / etc.):
//
//   ┌─ env chip (mono) ──────────[ set ● ]──[ Get one ↗ ]──┐
//   │  ┌─[ password •••• ] ──[ 👁 ] ─┐    [ Save ]           │
//   │  └────────────────────────────┘                       │
//   └────────────────────────────────────────────────────────┘
//
// When `pinned` is true the input is replaced by a "Set from the environment"
// notice — same affordance VoiceSetup already had, now shared.
// When `isSet` is true a green "set" badge appears (and the placeholder shows
// "•••••••• (set — type to replace)").
//
// Callers control the value (controlled input) so they can keep the trimmed
// string and the loading state next to their other form bits.

import { Check, Eye, EyeOff, ExternalLink, KeyRound, Lock } from "lucide-react";
import { useState, type KeyboardEvent } from "react";
import { Input } from "@/components/ui/input";
import { Button } from "@/components/ui/button";
import { cn } from "@/app/utils";

export interface ApiKeyFieldProps {
  /** Env var name — rendered as a mono chip beside the input. */
  env: string;
  /** Controlled value. */
  value: string;
  /** Receives the trimmed value (the field never submits empty/whitespace). */
  onChange: (value: string) => void;
  /** Called with the trimmed value when the user presses Enter or clicks Save. */
  onSubmit: (value: string) => void | Promise<void>;
  /** True when a value is already stored — shows "set" badge + masked placeholder. */
  isSet?: boolean;
  /** True when the key is locked from the environment — input is hidden, plain notice shown. */
  pinned?: boolean;
  /** Disables the input + Save button. */
  busy?: boolean;
  /** Optional placeholder hint shown when value is empty (and not set). */
  hint?: string;
  /** Optional "Get one" link (e.g. provider dashboard). */
  link?: string;
  /** Optional last-4 fingerprint shown beside the env chip (read-only). */
  fingerprint?: string;
  /** Optional className on the root container. */
  className?: string;
  /** Aria label override (defaults to `env`). */
  ariaLabel?: string;
}

/**
 * One inline API key entry. Trimmed submission, Enter-to-submit, reveal toggle,
 * and the "set" / "pinned" badges all live here so every page stays in sync.
 */
export function ApiKeyField({
  env,
  value,
  onChange,
  onSubmit,
  isSet = false,
  pinned = false,
  busy = false,
  hint,
  link,
  fingerprint,
  className,
  ariaLabel,
}: ApiKeyFieldProps) {
  const [reveal, setReveal] = useState(false);

  async function commit() {
    const v = value.trim();
    if (!v) return;
    await onSubmit(v);
  }

  function onKeyDown(e: KeyboardEvent<HTMLInputElement>) {
    if (e.key === "Enter") {
      e.preventDefault();
      void commit();
    }
  }

  if (pinned) {
    return (
      <div className={cn("space-y-1", className)}>
        <FieldHeader env={env} isSet={isSet} fingerprint={fingerprint} link={link} />
        <div className="flex items-center gap-1.5 rounded-md border border-dashed border-border bg-card/50 px-2.5 py-1.5 text-xs text-muted">
          <Lock className="size-3" />
          Set from the environment.
        </div>
      </div>
    );
  }

  const disabled = busy;
  const placeholder = isSet ? "•••••••• (set — type to replace)" : hint || "paste your key";

  return (
    <div className={cn("space-y-1", className)}>
      <FieldHeader env={env} isSet={isSet} fingerprint={fingerprint} link={link} />
      <div className="flex items-center gap-1.5">
        <div className="relative flex-1">
          <Input
            type={reveal ? "text" : "password"}
            value={value}
            disabled={disabled}
            onChange={(e) => onChange(e.target.value)}
            onKeyDown={onKeyDown}
            placeholder={placeholder}
            autoComplete="new-password"
            aria-label={ariaLabel || `${env} API key`}
            className="pr-7 font-mono"
          />
          <button
            type="button"
            onClick={() => setReveal((r) => !r)}
            disabled={disabled}
            aria-label={reveal ? "Hide key" : "Reveal key"}
            title={reveal ? "Hide key" : "Reveal key"}
            className="absolute right-1.5 top-1/2 -translate-y-1/2 text-muted transition-colors hover:text-foreground disabled:opacity-50"
          >
            {reveal ? <EyeOff className="size-3.5" /> : <Eye className="size-3.5" />}
          </button>
        </div>
        <Button
          size="sm"
          disabled={disabled || !value.trim()}
          onClick={() => void commit()}
          title={`Save ${env}`}
        >
          Save
        </Button>
      </div>
    </div>
  );
}

function FieldHeader({
  env,
  isSet,
  fingerprint,
  link,
}: {
  env: string;
  isSet?: boolean;
  fingerprint?: string;
  link?: string;
}) {
  return (
    <div className="flex flex-wrap items-center gap-1.5 text-xs">
      <KeyRound className="size-3 text-muted" />
      <span className="font-mono text-foreground/80">{env}</span>
      {fingerprint && <span className="font-mono text-muted">{fingerprint}</span>}
      {isSet && (
        <span className="inline-flex items-center gap-0.5 text-xs text-good">
          <Check className="size-3" /> set
        </span>
      )}
      {link && (
        <a
          href={link}
          target="_blank"
          rel="noreferrer"
          className="ml-auto inline-flex items-center gap-0.5 text-xs text-accent hover:underline"
        >
          Get one <ExternalLink className="size-3" />
        </a>
      )}
    </div>
  );
}
