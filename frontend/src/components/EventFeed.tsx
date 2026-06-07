import { useMemo, useState } from "react";
import { useEvents } from "@/lib/events";
import { Card, CardHeader, CardTitle, CardBody } from "@/components/ui/card";
import { Input } from "@/components/ui/input";
import { Muted } from "@/components/JsonView";

// Live journal firehose. Filtering only toggles row visibility — the stream is
// never reconnected, so switching/clearing the filter is instant.
export function EventFeed() {
  const { events } = useEvents();
  const [filter, setFilter] = useState("");
  const f = filter.trim().toLowerCase();

  const shown = useMemo(
    () => (f ? events.filter((e) => (e.kind || "").toLowerCase().includes(f)) : events),
    [events, f],
  );

  return (
    <Card className="h-full">
      <CardHeader>
        <CardTitle>Event Feed</CardTitle>
        <Muted>{events.length}</Muted>
        <Input
          value={filter}
          onChange={(e) => setFilter(e.target.value)}
          placeholder="filter kind…"
          className="ml-auto h-6 w-36 text-xs"
        />
      </CardHeader>
      <CardBody className="p-0">
        {shown.length === 0 ? (
          <div className="p-3">
            <Muted>no events yet</Muted>
          </div>
        ) : (
          <ul>
            {shown.map((e, i) => (
              <li
                key={e.id || i}
                className="grid grid-cols-[56px_1fr] gap-2 border-b border-border/60 px-3 py-1 hover:bg-panel"
              >
                <span className="truncate text-right text-muted">{e.seq ?? ""}</span>
                <span className="truncate">
                  <span className="text-accent">{e.kind}</span>{" "}
                  <span>{e.subject}</span>
                  {e.actor ? <span className="text-muted"> · {e.actor}</span> : null}
                </span>
              </li>
            ))}
          </ul>
        )}
      </CardBody>
    </Card>
  );
}
