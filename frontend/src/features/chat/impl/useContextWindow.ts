import { useEffect, useRef, useState } from "react";
import type { Msg } from "@/lib/conversations";

interface UseContextWindowParams {
  messages: Msg[];
}

export function useContextWindow({ messages }: UseContextWindowParams) {
  // pinned = the thread is stuck to the bottom (auto-scrolls on new content).
  // It flips to false when you scroll up to read scrollback, so a live stream
  // never yanks you back down mid-read; a "jump to latest" button restores it.
  const [pinned, setPinned] = useState(true);
  const scrollRef = useRef<HTMLDivElement>(null);

  // Pin the thread to the bottom as content streams in — but only while pinned,
  // so scrolling up to read history during a live stream isn't disrupted.
  useEffect(() => {
    const el = scrollRef.current;
    if (el && pinned) el.scrollTop = el.scrollHeight;
  }, [messages, pinned]);

  // Track whether the user is near the bottom; that's what keeps the thread
  // pinned. A ~80px slack means "close enough" counts as pinned.
  function onScroll() {
    const el = scrollRef.current;
    if (!el) return;
    setPinned(el.scrollHeight - el.scrollTop - el.clientHeight < 80);
  }

  function jumpToLatest() {
    const el = scrollRef.current;
    if (el) el.scrollTop = el.scrollHeight;
    setPinned(true);
  }

  return {
    pinned,
    setPinned,
    scrollRef,
    onScroll,
    jumpToLatest,
  };
}
