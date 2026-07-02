import { useEffect, useState } from "react";
import { ShieldCheck, History, Check, X, Clock, RefreshCw } from "lucide-react";
import { Row, Count } from "@/components/Panel";
import { Badge } from "@/components/ui/badge";
import { Button } from "@/components/ui/button";
import { Card, CardBody } from "@/components/ui/card";
import { Page } from "@/components/ui/page";
import { EmptyState } from "@/components/ui/empty";
import { SkeletonList } from "@/components/ui/skeleton";
import { ActionButton } from "@/components/ActionButton";
import { usePanel } from "@/lib/usePanel";
import { getJSON } from "@/lib/api";
import { cn, fmtTime } from "@/lib/utils";
import { ErrorText, Muted } from "@/components/JsonView";

interface PendingApproval {
  id?: string;
  capability?: string;
  tool_name?: string;
  reason?: string;
}

export function Approvals() {
  const { data, error, loading, reload } = usePanel<{ pending?: PendingApproval[] }>("/api/approvals");
  const items = data?.pending || [];
  return (
    <Page
      icon={ShieldCheck}
      title="Approvals"
      description="Capabilities the agent is waiting on you to grant or deny"
      actions={
        <Button variant="ghost" size="sm" onClick={reload} disabled={loading} title="Refresh">
          <RefreshCw className={cn("size-3.5", loading && "animate-spin")} />
        </Button>
      }
    >
      <Card glass>
        <CardBody className="space-y-3">
          {error ? (
            <ErrorText>{error}</ErrorText>
          ) : !data ? (
            <SkeletonList count={3} lines={2} />
          ) : (
            <>
              <Count>{items.length} pending</Count>
              {items.length ? (
                items.map((a: PendingApproval, i: number) => (
                  <Row key={a.id || i}>
                    <Badge variant="warn">{a.capability || a.tool_name || "?"}</Badge>
                    <span>{a.reason || a.tool_name || a.id || ""}</span>
                    {a.id ? (
                      <span className="ml-auto flex gap-1">
                        <ActionButton label="approve" variant="good" path="/api/decide" params={{ id: a.id, decision: "grant" }} onDone={reload} success="Request approved" />
                        <ActionButton label="deny" variant="danger" path="/api/decide" params={{ id: a.id, decision: "deny" }} onDone={reload} success="Request denied" />
                      </span>
                    ) : null}
                  </Row>
                ))
              ) : (
                <EmptyState
                  icon={ShieldCheck}
                  title="Nothing awaiting approval"
                  hint="When the agent hits a capability gated by your ask policy, the request will appear here for you to approve or deny."
                />
              )}
            </>
          )}
        </CardBody>
      </Card>

      <ApprovalsHistory />
    </Page>
  );
}

interface ResolvedApproval {
  ts_unix_ms?: number;
  approval_id?: string;
  capability?: string;
  tool?: string;
  reason?: string;
  status?: string; // pending | granted | denied | timeout
  resolved_by?: string;
}

// ApprovalsHistory shows past HITL decisions (M773): every approval the agent has asked
// for, joined with what you (or a timeout) decided. The pending list above is the
// to-do; this is the record — the audit trail of the trust boundary, so you can review
// what was allowed, what was refused, and who/what resolved it. Read-only.
export function ApprovalsHistory() {
  const [rows, setRows] = useState<ResolvedApproval[]>([]);
  const [loaded, setLoaded] = useState(false);

  async function load() {
    try {
      const d = await getJSON<{ approvals?: ResolvedApproval[] }>("/api/approvals_log", { limit: "50" });
      // The log includes still-pending rows; those live in the panel above, so the
      // history shows only resolved decisions.
      setRows((d.approvals || []).filter((a) => a.status && a.status !== "pending"));
    } catch {
      setRows([]);
    } finally {
      setLoaded(true);
    }
  }
  useEffect(() => {
    load();
    const id = setInterval(load, 8000);
    return () => clearInterval(id);
  }, []);

  return (
    <div className="glass rounded-xl p-3">
      <div className="mb-2 flex items-center gap-1.5 text-xs font-semibold uppercase tracking-normal text-muted">
        <History className="size-3.5" /> Decision history
        {loaded && <span className="ml-1 normal-case text-muted/70">({rows.length})</span>}
      </div>
      {!loaded ? (
        <Muted>loading…</Muted>
      ) : rows.length === 0 ? (
        <Muted>no resolved approvals yet — decisions you make above will be recorded here</Muted>
      ) : (
        <ul className="space-y-1">
          {rows.map((a, i) => (
            <li key={a.approval_id || i} className="flex items-center gap-2 border-b border-border/40 py-1 text-xs last:border-0">
              <StatusBadge status={a.status} />
              <span className="shrink-0 font-mono text-[11px] text-foreground">{a.capability || a.tool || "?"}</span>
              <span className="min-w-0 flex-1 truncate text-muted">{a.reason || a.tool || ""}</span>
              {a.resolved_by && <span className="shrink-0 text-xs text-muted/80">by {a.resolved_by}</span>}
              <span className="w-14 shrink-0 text-right tabular-nums text-muted">{fmtTime(a.ts_unix_ms)}</span>
            </li>
          ))}
        </ul>
      )}
    </div>
  );
}

function StatusBadge({ status }: { status?: string }) {
  if (status === "granted")
    return (
      <span className="inline-flex w-16 shrink-0 items-center gap-1 text-good">
        <Check className="size-3" /> granted
      </span>
    );
  if (status === "denied")
    return (
      <span className="inline-flex w-16 shrink-0 items-center gap-1 text-bad">
        <X className="size-3" /> denied
      </span>
    );
  return (
    <span className="inline-flex w-16 shrink-0 items-center gap-1 text-muted">
      <Clock className="size-3" /> timeout
    </span>
  );
}
