import { RefreshCw, Activity } from "lucide-react";
import { usePanel } from "@/lib/usePanel";
import { useEvents } from "@/lib/events";
import { Card, CardHeader, CardTitle, CardBody } from "@/components/ui/card";
import { Button } from "@/components/ui/button";
import { Badge } from "@/components/ui/badge";
import { KeyValue, JsonView, Muted, ErrorText } from "@/components/JsonView";

function isPrimitive(v: unknown) {
  return v === null || ["string", "number", "boolean"].includes(typeof v);
}

// Status: the daemon's self-report. Primitive fields render as a key/value grid;
// nested objects fall to a JSON block so nothing is hidden.
export function Status() {
  const { connected } = useEvents();
  const { data, error, loading, reload } = usePanel<Record<string, any>>("/api/status");

  const primitives: [string, React.ReactNode][] = data
    ? Object.entries(data)
        .filter(([, v]) => isPrimitive(v))
        .map(([k, v]) => [k, String(v)])
    : [];
  const nested = data ? Object.entries(data).filter(([, v]) => !isPrimitive(v)) : [];

  return (
    <Card className="h-full">
      <CardHeader>
        <CardTitle>Status</CardTitle>
        <Badge variant={connected ? "good" : "bad"} className="ml-1">
          <Activity className="mr-1 size-3" />
          {connected ? "live" : "offline"}
        </Badge>
        <Button variant="ghost" size="icon" className="ml-auto" onClick={reload} title="Refresh">
          <RefreshCw className={loading ? "animate-spin" : ""} />
        </Button>
      </CardHeader>
      <CardBody className="space-y-4">
        {error ? (
          <ErrorText>{error}</ErrorText>
        ) : data ? (
          <>
            <KeyValue pairs={primitives} />
            {nested.map(([k, v]) => (
              <div key={k}>
                <div className="mb-1 text-xs uppercase tracking-wider text-muted">{k}</div>
                <JsonView value={v} />
              </div>
            ))}
          </>
        ) : (
          <Muted>loading…</Muted>
        )}
      </CardBody>
    </Card>
  );
}
