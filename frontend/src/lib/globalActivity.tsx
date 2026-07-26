import {
  createContext,
  useCallback,
  useContext,
  useEffect,
  useMemo,
  useRef,
  useState,
  type ReactNode,
} from "react";
import { getJSON } from "@/lib/api";
import {
  foldActivityEvent,
  seedFromRuns,
  summarize,
  type ActivityState,
  type ActivitySummary,
} from "@/lib/activity";
import { useEvents } from "@/lib/events";

interface GlobalActivityContext {
  state: ActivityState;
  summary: ActivitySummary;
  seeding: boolean;
  refresh: () => Promise<void>;
}

const Context = createContext<GlobalActivityContext | null>(null);

// GlobalActivityProvider is the app-shell source of truth for in-flight work.
// A daemon snapshot makes runs that predate the browser visible immediately;
// the single SSE stream then advances that state without polling.
export function GlobalActivityProvider({ children }: { children: ReactNode }) {
  const { connected, subscribe } = useEvents();
  const [state, setState] = useState<ActivityState>({});
  const [seeding, setSeeding] = useState(true);
  const eventRevision = useRef(0);
  const requestRevision = useRef(0);
  const seededOnce = useRef(false);

  const refresh = useCallback(async () => {
    const request = ++requestRevision.current;
    const eventsAtStart = eventRevision.current;
    setSeeding(true);
    try {
      const result = await getJSON<{ runs?: Record<string, unknown>[] }>("/api/runs");
      if (request !== requestRevision.current) return;
      const seeded = seedFromRuns(result.runs || []);
      setState((live) =>
        eventRevision.current === eventsAtStart
          ? seeded
          : { ...seeded, ...live },
      );
    } catch {
      // Keep the last evidence. SSE may still be healthy even when the snapshot
      // request fails transiently.
    } finally {
      if (request === requestRevision.current) setSeeding(false);
    }
  }, []);

  useEffect(() => subscribe((event) => {
    eventRevision.current++;
    setState((current) => foldActivityEvent(current, event));
  }), [subscribe]);

  // Seed immediately and reconcile again after every SSE reconnection. The
  // snapshot removes runs whose terminal event was missed while disconnected.
  useEffect(() => {
    if (!seededOnce.current) {
      seededOnce.current = true;
      void refresh();
      return;
    }
    if (connected) void refresh();
  }, [connected, refresh]);

  const summary = useMemo(() => summarize(state), [state]);
  const value = useMemo(
    () => ({ state, summary, seeding, refresh }),
    [state, summary, seeding, refresh],
  );
  return <Context.Provider value={value}>{children}</Context.Provider>;
}

export function useGlobalActivity(): GlobalActivityContext {
  const value = useContext(Context);
  if (!value) {
    throw new Error("useGlobalActivity must be used within GlobalActivityProvider");
  }
  return value;
}
