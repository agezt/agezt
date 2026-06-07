import { RefreshCw } from "lucide-react";
import { usePanel } from "@/lib/usePanel";
import { Card, CardHeader, CardTitle, CardBody } from "@/components/ui/card";
import { Button } from "@/components/ui/button";
import { JsonView, Muted, ErrorText } from "@/components/JsonView";

// GenericPanel renders any read-only /api route as pretty JSON. It's the
// fallback for surfaces that don't yet have a bespoke view — nothing is lost
// during the incremental port, the data is just shown raw.
export function GenericPanel({ title, path }: { title: string; path: string }) {
  const { data, error, loading, reload } = usePanel(path);
  return (
    <Card className="h-full">
      <CardHeader>
        <CardTitle>{title}</CardTitle>
        <Button variant="ghost" size="icon" className="ml-auto" onClick={reload} title="Refresh">
          <RefreshCw className={loading ? "animate-spin" : ""} />
        </Button>
      </CardHeader>
      <CardBody>
        {error ? <ErrorText>{error}</ErrorText> : data ? <JsonView value={data} /> : <Muted>loading…</Muted>}
      </CardBody>
    </Card>
  );
}
