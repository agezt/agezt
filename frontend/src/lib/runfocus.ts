import { useSyncExternalStore } from "react";

// Cross-component signal: "open this run in the Runs view". Set by the ⌘K palette
// (and anything else that wants to deep-link a run); consumed by the Runs view,
// which expands and scrolls to the matching row, then clears the signal so a later
// manual collapse sticks. Module-level store + subscribers — like lib/theme — so the
// setter works from anywhere, not just inside the Runs subtree.
let focused: string | null = null;
const listeners = new Set<() => void>();

function emit() {
  for (const l of listeners) l();
}

// focusRun requests that the Runs view open the run with this correlation id.
export function focusRun(correlationId: string): void {
  focused = correlationId;
  emit();
}

// clearRunFocus drops the pending request (called by the Runs view once it has
// applied it, so re-renders don't re-open a row the user has since collapsed).
export function clearRunFocus(): void {
  if (focused === null) return;
  focused = null;
  emit();
}

export function getRunFocus(): string | null {
  return focused;
}

export function useRunFocus(): string | null {
  return useSyncExternalStore(
    (cb) => {
      listeners.add(cb);
      return () => listeners.delete(cb);
    },
    getRunFocus,
    getRunFocus,
  );
}
