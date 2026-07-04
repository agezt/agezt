import { useEffect, useState } from "react";
import { getJSON } from "@/lib/api";

export function useConversationRouting() {
  // The chat task's routing chain (M931): pinned to the top of the model picker
  // so the models this conversation will actually fall back through lead the
  // list — pick from your configured fallbacks first, keyed providers after.
  const [chatChain, setChatChain] = useState<string[]>([]);

  useEffect(() => {
    let live = true;
    getJSON<{ chains?: Record<string, string[]> }>("/api/routing")
      .then((r) => {
        if (live) setChatChain(r.chains?.chat || []);
      })
      .catch(() => {});
    return () => {
      live = false;
    };
  }, []);

  return { chatChain };
}
