// queue.ts — the pure operations behind the chat message queue (M962). While a
// run is streaming, the user can line up follow-up messages; they show on screen
// and the next one auto-sends when the current run finishes. These helpers keep
// the reorder/remove/normalize logic pure so it's unit-tested without React.

export interface QueuedMsg {
  id: string;
  text: string;
}

// addQueued appends a message to the end of the queue (skips blank text).
export function addQueued(queue: QueuedMsg[], text: string, id: string): QueuedMsg[] {
  const t = text.trim();
  if (!t) return queue;
  return [...queue, { id, text: t }];
}

// removeQueued drops the message with the given id.
export function removeQueued(queue: QueuedMsg[], id: string): QueuedMsg[] {
  return queue.filter((m) => m.id !== id);
}

// moveQueued reorders one message up (-1) or down (+1) by one slot. Out-of-range
// moves (first up / last down / unknown id) return the queue unchanged.
export function moveQueued(queue: QueuedMsg[], id: string, dir: -1 | 1): QueuedMsg[] {
  const i = queue.findIndex((m) => m.id === id);
  if (i < 0) return queue;
  const j = i + dir;
  if (j < 0 || j >= queue.length) return queue;
  const next = queue.slice();
  [next[i], next[j]] = [next[j], next[i]];
  return next;
}

// dequeueFront splits the queue into its first message and the rest, for
// auto-send when a run completes. Returns {front: null, rest} on an empty queue.
export function dequeueFront(queue: QueuedMsg[]): { front: QueuedMsg | null; rest: QueuedMsg[] } {
  if (queue.length === 0) return { front: null, rest: queue };
  return { front: queue[0], rest: queue.slice(1) };
}
