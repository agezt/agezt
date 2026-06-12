import type { AgentEvent } from "@/lib/events";

// Proactive desktop notifications (M919): AGEZT reaches OUT to the operator for
// the few high-signal events that genuinely need a human — an approval waiting,
// a run that failed, a halt, a budget ceiling hit — so a problem surfaces even
// when the tab is in the background, not only when you happen to be looking. The
// classifier is pure + unit-tested; the firing hook lives in NotifyToggle.

const PREF_KEY = "agezt.notify.enabled";

// notifyEnabled reports the operator's opt-in (localStorage, per-device). Off by
// default — browser notifications require an explicit user gesture to enable, and
// we never nag without consent.
export function notifyEnabled(): boolean {
  try {
    return localStorage.getItem(PREF_KEY) === "1";
  } catch {
    return false;
  }
}

export function setNotifyEnabled(on: boolean): void {
  try {
    localStorage.setItem(PREF_KEY, on ? "1" : "0");
  } catch {
    /* private mode / storage disabled — the toggle just won't persist */
  }
}

// notifySupported / notifyPermission wrap the Notification API so the component
// (and tests) don't poke globals directly.
export function notifySupported(): boolean {
  return typeof Notification !== "undefined";
}

export function notifyPermission(): NotificationPermission {
  return notifySupported() ? Notification.permission : "denied";
}

// DesktopNotice is what we'd show for one event: a title, a one-line body, a tag
// (so repeats of the same thing COALESCE into one notification rather than
// stacking), and the view to open on click.
export interface DesktopNotice {
  title: string;
  body: string;
  tag: string;
  hash: string;
}

function str(v: unknown): string {
  return v == null ? "" : String(v);
}

// notifyEventClassify maps an event to a desktop notice, or null when the event
// isn't worth interrupting the operator for. Deliberately narrow — only the
// "you need to act / something broke" set, not the firehose. Pure + unit-tested.
export function notifyEventClassify(e: AgentEvent): DesktopNotice | null {
  const k = (e.kind || "").toLowerCase();
  const p: any = e.payload || {};
  switch (k) {
    case "approval.requested":
      return {
        title: "Approval needed",
        body: str(p.capability) || str(p.tool_name) || "A capability request is waiting.",
        tag: "approval-" + (str(p.id) || str(e.id)),
        hash: "approvals",
      };
    case "task.failed":
      return {
        title: "Run failed",
        body: str(p.reason) || str(p.error) || "A run failed.",
        tag: "fail-" + (str(e.correlation_id) || str(e.id)),
        hash: "runs",
      };
    case "halt":
      return {
        title: "Daemon halted",
        body: str(p.reason) || "The kernel was halted.",
        tag: "halt",
        hash: "alerts",
      };
    case "budget.exceeded":
      return {
        title: "Budget ceiling exceeded",
        body: "Spending hit the configured ceiling.",
        tag: "budget",
        hash: "budget",
      };
    default:
      return null;
  }
}
