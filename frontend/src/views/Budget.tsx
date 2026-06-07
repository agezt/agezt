import { RefreshCw } from "lucide-react";
import { usePanel } from "@/lib/usePanel";
import { Card, CardHeader, CardTitle, CardBody } from "@/components/ui/card";
import { Button } from "@/components/ui/button";
import { KeyValue, JsonView, Muted, ErrorText } from "@/components/JsonView";

function isPrimitive(v: unknown) {
  return v === null || ["string", "number", "boolean"].includes(typeof v);
}

export function Budget() {
  const { data, error, loading, reload } = usePanel<Record<string, any>>("/api/budget");
  const primitives: [string, React.ReactNode][] = data
    ? Object.entries(data)
        .filter(([, v]) => isPrimitive(v))
        .map(([k, v]) => [k, String(v)])
    : [];
  const nested = data ? Object.entries(data).filter(([, v]) => !isPrimitive(v)) : [];

  return (
    <Card className="h-full">
      <CardHeader>
        <CardTitle>Budget</CardTitle>
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
