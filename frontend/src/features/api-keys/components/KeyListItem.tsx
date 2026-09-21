// KeyListItem.tsx — the per-key row used inside Models & Keys (and re-usable
// wherever a provider env has "store many keys, pick active"). Shows label +
// fingerprint, an "active" badge (or "activate" button when not), and a delete
// affordance. Values are write-only — only the fingerprint is shown.

import { Check, Trash2 } from "lucide-react";
import { Badge } from "@/components/ui/badge";
import { Button } from "@/components/ui/button";

export interface KeyInfo {
  label: string;
  active: boolean;
  last4: string;
}

export interface KeyListItemProps {
  keyInfo: KeyInfo;
  busy?: boolean;
  onActivate?: () => void;
  onRemove?: () => void;
}

export function KeyListItem({ keyInfo, busy = false, onActivate, onRemove }: KeyListItemProps) {
  return (
    <li className="flex items-center gap-2 rounded-md border border-border/60 bg-panel/40 px-2 py-1 text-xs">
      {keyInfo.active ? (
        <Badge variant="good">
          <Check className="size-2.5 mr-1" /> active
        </Badge>
      ) : (
        <button
          type="button"
          onClick={onActivate}
          disabled={busy || !onActivate}
          className="rounded border border-border px-1.5 py-0.5 text-[9px] font-medium text-muted transition-colors hover:text-foreground disabled:opacity-50"
          title="Make this the active key"
          aria-label={`Activate ${keyInfo.label}`}
        >
          activate
        </button>
      )}
      <span className="font-medium text-foreground">{keyInfo.label}</span>
      <span className="font-mono text-xs text-muted">{keyInfo.last4}</span>
      <Button
        variant="ghost"
        size="icon"
        className="ml-auto size-6"
        onClick={onRemove}
        disabled={busy || !onRemove}
        title="Remove this key"
        aria-label={`Remove ${keyInfo.label}`}
      >
        <Trash2 className="size-3.5 text-muted hover:text-bad" />
      </Button>
    </li>
  );
}
