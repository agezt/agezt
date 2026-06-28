import { fmtTime } from "@/lib/utils";
import {
  doctorIncidentLabel,
  doctorIncidentNodeTitle,
  doctorIncidentTreeOpsSummary,
  type DoctorIncidentNode,
  type DoctorIncidentTree,
} from "@/lib/autonomy";
import {
  IncidentBadges,
  incidentPhaseBadgeClass,
} from "@/components/IncidentBadges";

export function DoctorIncidentTrees({
  trees,
  compact = false,
  onOpenIncident,
}: {
  trees: DoctorIncidentTree[];
  compact?: boolean;
  onOpenIncident?: (incidentId: string) => void;
}) {
  if (!trees.length) return null;
  return (
    <ul className="space-y-2">
      {trees.map((tree) => {
        const ops = doctorIncidentTreeOpsSummary(tree);
        return (
          <li
            key={tree.id}
            className="rounded-lg border border-border bg-panel/40 px-2.5 py-2"
          >
            <div className="flex flex-wrap items-center gap-2 text-xs">
              <span className="rounded bg-panel px-1.5 py-0.5 font-mono text-xs text-warn">
                root
              </span>
              <span className="font-medium">{tree.rootAgent}</span>
              <span className={incidentPhaseBadgeClass(ops.tone, true)}>
                {ops.label}
              </span>
              <span className="text-muted">{ops.detail}</span>
              <span className="ml-auto font-mono text-xs text-muted opacity-70">
                {fmtTime(tree.latestTs)}
              </span>
              {onOpenIncident && (
                <button
                  onClick={() => onOpenIncident(tree.id)}
                  className="text-xs font-medium text-accent/80 transition-colors hover:text-accent"
                  title="Open this incident tree"
                >
                  open -&gt;
                </button>
              )}
            </div>
            <div className="mt-2 space-y-1.5">
              {tree.roots.map((node) => (
                <DoctorIncidentNodeView
                  key={node.id}
                  node={node}
                  compact={compact}
                />
              ))}
            </div>
          </li>
        );
      })}
    </ul>
  );
}

function DoctorIncidentNodeView({
  node,
  compact,
}: {
  node: DoctorIncidentNode;
  compact: boolean;
}) {
  const latest = node.latest;
  return (
    <div className={node.depth > 0 ? "ml-3 border-l border-border/70 pl-3" : ""}>
      <div className="flex items-center gap-2 text-xs">
        <span className="rounded bg-panel px-1.5 py-0.5 font-mono text-xs text-warn">
          {node.depth > 0 ? `hop ${node.depth}` : "incident"}
        </span>
        <IncidentBadges item={latest} mono />
        <span className="font-medium">{doctorIncidentNodeTitle(node)}</span>
        <span className="text-muted">
          {node.items.length} event{node.items.length === 1 ? "" : "s"}
        </span>
        <span className="ml-auto font-mono text-xs text-muted opacity-70">
          {fmtTime(node.latestTs)}
        </span>
      </div>
      {latest && (
        <>
          <div className="mt-1 text-xs text-foreground/90">{latest.title}</div>
          <div className={compact ? "truncate text-[11px] text-muted" : "text-[11px] text-muted"}>
            {[doctorIncidentLabel(latest), latest.detail].filter(Boolean).join(" · ")}
          </div>
        </>
      )}
      {node.children.length > 0 && (
        <div className="mt-2 space-y-1.5">
          {node.children.map((child) => (
            <DoctorIncidentNodeView key={child.id} node={child} compact={compact} />
          ))}
        </div>
      )}
    </div>
  );
}
