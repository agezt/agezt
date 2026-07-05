import {
  createContext,
  useCallback,
  useContext,
  useEffect,
  useRef,
  useState,
  type ReactNode,
} from "react";
import { eventsURLAsync } from "@/lib/api";

export interface AgentEvent {
  id?: string;
  seq?: number;
  ts_unix_ms?: number;
  subject?: string;
  actor?: string;
  kind?: string;
  correlation_id?: string;
  payload?: any;
}

const MAX_FEED = 300;

interface EventsCtx {
  events: AgentEvent[];
  /** Whether the SSE socket is currently open. Clears on `onerror`. */
  connected: boolean;
  /**
   * Wall-clock time (ms) of the most recent event received. `null` until
   * the first event arrives after a (re)connect, and `null` while the socket
   * is closed. Consumers (e.g. the ConnectionChip) use this together with
   * `connected` to decide whether to show a "stale" indicator: a long gap
   * between events while `connected` is true is the signal that the link
   * looks alive but isn't actually pushing data.
   */
  lastEventAt: number | null;
  /** subscribe to the live stream; returns an unsubscribe fn. */
  subscribe: (fn: (e: AgentEvent) => void) => () => void;
}

const Ctx = createContext<EventsCtx | null>(null);

// EventsProvider owns the single EventSource for the whole app. Panels read the
// rolling feed via `events`; interactive views (Flow Studio) hook the raw
// stream via `subscribe` for low-latency per-event reactions. The UI holds no
// authoritative state — it subscribes to the journal and renders.
export function EventsProvider({ children }: { children: ReactNode }) {
  const [events, setEvents] = useState<AgentEvent[]>([]);
  const [connected, setConnected] = useState(false);
  const [lastEventAt, setLastEventAt] = useState<number | null>(null);
  const listeners = useRef<Set<(e: AgentEvent) => void>>(new Set());

  useEffect(() => {
    let src: EventSource | null = null;
    let cancelled = false;

    (async () => {
      const url = await eventsURLAsync();
      if (cancelled) return;
      src = new EventSource(url);
      src.onopen = () => {
        setConnected(true);
        // A fresh open starts the staleness clock from zero — a brand-new
        // socket that hasn't seen its first event yet counts as "stale
        // with no signal yet" rather than "live".
        setLastEventAt(null);
      };
      src.onerror = () => {
        setConnected(false);
        // Don't clear lastEventAt here: the chip uses the gap between an
        // old lastEventAt and the moment we noticed the disconnect to
        // display e.g. "last event 4s ago, link just dropped". Clearing
        // makes the chip flip straight to "disconnected" which is less
        // informative.
      };
      src.onmessage = (m) => {
        let ev: AgentEvent;
        try {
          ev = JSON.parse(m.data) as AgentEvent;
        } catch {
          return;
        }
        setLastEventAt(Date.now());
        setEvents((prev) => {
          const next = [ev, ...prev];
          return next.length > MAX_FEED ? next.slice(0, MAX_FEED) : next;
        });
        listeners.current.forEach((fn) => fn(ev));
      };
    })();

    return () => {
      cancelled = true;
      if (src) src.close();
    };
  }, []);

  // Stable across renders (the listener set is a ref), so consumers can use it
  // as a useEffect dependency without re-subscribing on every incoming event.
  const subscribe = useCallback((fn: (e: AgentEvent) => void) => {
    listeners.current.add(fn);
    return () => {
      listeners.current.delete(fn);
    };
  }, []);

  return <Ctx.Provider value={{ events, connected, lastEventAt, subscribe }}>{children}</Ctx.Provider>;
}

export function useEvents(): EventsCtx {
  const c = useContext(Ctx);
  if (!c) throw new Error("useEvents must be used within EventsProvider");
  return c;
}

/**
 * connectionState classifies the SSE link into one of three states from
 * the (connected, lastEventAt) snapshot the provider exposes:
 *
 *   - 'disconnected' — the socket is closed; the chip should warn and let
 *     the operator know they've lost live visibility.
 *   - 'live' — the socket is open AND we've seen at least one event in the
 *     last STALE_MS window. Normal operation.
 *   - 'stale' — the socket claims to be open, but events have stopped (no
 *     message in the last STALE_MS window, or no event ever arrived). A
 *     stale state suggests the SSE link looks alive but the daemon isn't
 *     actually pushing data — usually caused by a half-open connection,
 *     a stalled publish loop, or a daemon-side backpressure stall.
 *
 * Exported as a pure function so the chip and tests can share the same
 * classification logic without depending on a real-time clock.
 */
export const STALE_MS = 15_000;

/** @public Union of connection-state display names. */
export type ConnectionStateName = "live" | "disconnected" | "stale";

export interface ConnectionState {
  state: ConnectionStateName;
  /** ageMs is null when no event has ever arrived (post-connect); otherwise the gap since the last event. */
  ageMs: number | null;
  /** label is a short human-readable summary suitable for a tooltip or aria-label. */
  label: string;
}

export function connectionState(
  snapshot: { connected: boolean; lastEventAt: number | null },
  nowMs: number = Date.now(),
): ConnectionState {
  if (!snapshot.connected) {
    return {
      state: "disconnected",
      ageMs: snapshot.lastEventAt ? Math.max(0, nowMs - snapshot.lastEventAt) : null,
      label: snapshot.lastEventAt
        ? `disconnected (last event ${Math.round((nowMs - snapshot.lastEventAt) / 1000)}s ago)`
        : "disconnected",
    };
  }
  if (snapshot.lastEventAt === null) {
    // Connected but no event yet — we're waiting for the first packet. The
    // UI shows this as "stale" so the operator notices the gap rather than
    // seeing a green light that may stop showing data any second.
    return { state: "stale", ageMs: null, label: "live, no events yet" };
  }
  const age = Math.max(0, nowMs - snapshot.lastEventAt);
  if (age > STALE_MS) {
    return { state: "stale", ageMs: age, label: `stale (no event for ${Math.round(age / 1000)}s)` };
  }
  return { state: "live", ageMs: age, label: `live (last event ${Math.round(age / 1000)}s ago)` };
}
