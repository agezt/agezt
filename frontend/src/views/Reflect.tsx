import { Panel } from "@/components/Panel";
import { JsonView, Muted } from "@/components/JsonView";

export function Reflect() {
  return (
    <Panel<Record<string, any>> title="Reflection" path="/api/reflect">
      {(d) => (d && d.error ? <Muted>{d.error}</Muted> : <JsonView value={d} />)}
    </Panel>
  );
}
