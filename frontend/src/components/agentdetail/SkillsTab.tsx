import { useState } from "react";
import { Sparkles, ArrowUpRight, Share2 } from "lucide-react";
import { clip } from "@/lib/utils";
import { Button } from "@/components/ui/button";
import { Badge, statusVariant } from "@/components/ui/badge";
import { SkeletonList } from "@/components/ui/skeleton";
import { EmptyState } from "@/components/ui/empty";
import { LoadMoreFooter } from "@/components/ui/load-more-footer";
import { type SkillLite } from "@/lib/agentdetail";

// SKILL_ROW_WINDOW is how many private-skill rows render at once — the skills
// prop is filtered per-agent upstream (/api/skills has no cursor); the window
// keeps a prolific author from ballooning the DOM, grown via Load-more.
const SKILL_ROW_WINDOW = 60;

export function SkillsTab({
  skills,
  busy,
  onAction,
  onManage,
}: {
  skills: SkillLite[] | null;
  busy: boolean;
  onAction: (
    path: string,
    params: Record<string, string>,
    success: string,
  ) => void;
  onManage: (view: string) => void;
}) {
  const [win, setWin] = useState(SKILL_ROW_WINDOW);
  if (!skills) return <SkeletonList count={3} lines={2} />;
  if (skills.length === 0)
    return (
      <EmptyState
        icon={Sparkles}
        title="No private skills"
        hint="Skills authored privately for this agent appear here. Share one to make it available fleet-wide."
      />
    );
  return (
    <div className="space-y-2">
      <ul className="space-y-2">
        {skills.slice(0, win).map((s) => (
          <li
            key={s.id}
            className="rounded-lg border border-border bg-panel/30 p-2.5"
          >
            <div className="flex flex-wrap items-center gap-2">
              {s.status && (
                <Badge variant={statusVariant(s.status)}>{s.status}</Badge>
              )}
              <span className="text-xs font-medium">{s.name}</span>
              <Button
                size="sm"
                variant="ghost"
                disabled={busy}
                className="ml-auto"
                title="Share with the whole fleet"
                onClick={() =>
                  s.id &&
                  onAction("/api/skill/share", { id: s.id }, `shared ${s.name}`)
                }
              >
                <Share2 className="size-3.5" /> Share
              </Button>
            </div>
            {s.description && (
              <div className="mt-1 text-[11px] text-muted">
                {clip(s.description, 200)}
              </div>
            )}
            {(s.triggers || []).length > 0 && (
              <div className="mt-1 flex flex-wrap gap-1">
                {(s.triggers || []).map((t, i) => (
                  <span
                    key={i}
                    className="rounded bg-card px-1.5 py-0.5 font-mono text-xs text-muted"
                  >
                    {t}
                  </span>
                ))}
              </div>
            )}
          </li>
        ))}
      </ul>
      {skills.length > SKILL_ROW_WINDOW && (
        <LoadMoreFooter
          hasMore={win < skills.length}
          loadingMore={false}
          onLoadMore={() => setWin((w) => w + SKILL_ROW_WINDOW)}
          pageSize={Math.min(SKILL_ROW_WINDOW, Math.max(1, skills.length - win))}
          label="skills"
        />
      )}
      <Button variant="ghost" size="sm" onClick={() => onManage("skills")}>
        Open Skills <ArrowUpRight className="size-3.5" />
      </Button>
    </div>
  );
}

