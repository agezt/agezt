import {
  createContext,
  useContext,
  useEffect,
  useRef,
  useState,
  type ReactNode,
} from "react";
import { eventsURL } from "@/lib/api";

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
  connected: boolean;
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
  const listeners = useRef<Set<(e: AgentEvent) => void>>(new Set());

  useEffect(() => {
    const src = new EventSource(eventsURL);
    src.onopen = () => setConnected(true);
    src.onerror = () => setConnected(false);
    src.onmessage = (m) => {
      let ev: AgentEvent;
      try {
        ev = JSON.parse(m.data) as AgentEvent;
      } catch {
        return;
      }
      setEvents((prev) => {
        const next = [ev, ...prev];
        return next.length > MAX_FEED ? next.slice(0, MAX_FEED) : next;
      });
      listeners.current.forEach((fn) => fn(ev));
    };
    return () => src.close();
  }, []);

  const subscribe = (fn: (e: AgentEvent) => void) => {
    listeners.current.add(fn);
    return () => {
      listeners.current.delete(fn);
    };
  };

  return <Ctx.Provider value={{ events, connected, subscribe }}>{children}</Ctx.Provider>;
}

export function useEvents(): EventsCtx {
  const c = useContext(Ctx);
  if (!c) throw new Error("useEvents must be used within EventsProvider");
  return c;
}
