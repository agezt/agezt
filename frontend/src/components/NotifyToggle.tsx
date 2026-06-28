import { useEffect, useState } from "react";
import { Bell, BellOff, BellRing } from "lucide-react";
import { useEvents } from "@/lib/events";
import { useUI } from "@/components/ui/feedback";
import { cn } from "@/lib/utils";
import {
  notifyEnabled,
  setNotifyEnabled,
  notifySupported,
  notifyPermission,
  notifyEventClassify,
} from "@/lib/notify";

// useDesktopNotifications fires a browser notification for each high-signal event
// (approval/failure/halt/budget) while the operator has opted in AND granted
// permission. Subscribes to the LIVE stream only (no journal backfill), so it
// never replays old events on load. Mounted once near the app root.
function useDesktopNotifications(enabled: boolean) {
  const { subscribe } = useEvents();
  useEffect(() => {
    if (!enabled) return;
    return subscribe((e) => {
      if (!notifyEnabled() || notifyPermission() !== "granted") return;
      const n = notifyEventClassify(e);
      if (!n) return;
      try {
        const note = new Notification(n.title, { body: n.body, tag: n.tag });
        note.onclick = () => {
          window.focus();
          if (location.hash.replace(/^#\/?/, "") !== n.hash) location.hash = n.hash;
          note.close();
        };
      } catch {
        /* a browser that throws on construction — give up silently */
      }
    });
  }, [subscribe, enabled]);
}

// NotifyToggle is the header control to turn proactive desktop notifications on
// or off. Enabling requests Notification permission (a user gesture); the icon
// reflects on / off / blocked. It also drives the firing hook via shared state.
export function NotifyToggle() {
  const ui = useUI();
  const [on, setOn] = useState(false);
  const [perm, setPerm] = useState<NotificationPermission>("default");

  useEffect(() => {
    setOn(notifyEnabled());
    setPerm(notifyPermission());
  }, []);

  useDesktopNotifications(on && perm === "granted");

  if (!notifySupported()) return null;

  async function toggle() {
    if (on) {
      setNotifyEnabled(false);
      setOn(false);
      ui.toast("Desktop notifications off", "success");
      return;
    }
    let p = notifyPermission();
    if (p === "default") {
      p = await Notification.requestPermission();
      setPerm(p);
    }
    if (p !== "granted") {
      ui.toast("Notifications are blocked in your browser settings", "error");
      return;
    }
    setNotifyEnabled(true);
    setOn(true);
    ui.toast("Desktop notifications on — you'll be pinged for approvals, failures and halts", "success");
  }

  const blocked = perm === "denied";
  const Icon = blocked ? BellOff : on ? BellRing : Bell;
  return (
    <button
      onClick={toggle}
      disabled={blocked}
      title={
        blocked
          ? "Notifications blocked in browser settings"
          : on
            ? "Proactive desktop notifications: ON"
            : "Enable proactive desktop notifications (approvals, failures, halts)"
      }
      aria-label="Toggle desktop notifications"
      className={cn(
        "inline-flex size-8 items-center justify-center rounded-md border transition-colors",
        blocked
          ? "border-border text-muted opacity-60"
          : on
            ? "border-accent text-accent"
            : "border-border text-muted hover:text-foreground hover:border-accent",
      )}
    >
      <Icon className="size-4" />
    </button>
  );
}
