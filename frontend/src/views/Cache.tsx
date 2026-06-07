import { Panel, Stats } from "@/components/Panel";
import { Muted } from "@/components/JsonView";
import { money } from "@/lib/format";

export function Cache() {
  return (
    <Panel<Record<string, any>> title="Cache" path="/api/cache">
      {(d) => (
        <>
          <Stats
            pairs={[
              ["saved", money(d.saved_microcents)],
              ["cache reads", `${d.cached_input_tokens || 0} tok`],
              ["cache writes", `${d.cache_write_input_tokens || 0} tok`],
              ["priced calls", d.calls || 0],
            ]}
          />
          {!(d.cached_input_tokens || d.cache_write_input_tokens) && (
            <Muted>no prompt-cache usage recorded</Muted>
          )}
        </>
      )}
    </Panel>
  );
}
