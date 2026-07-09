import { useEffect, useState } from "react";
import { FolderOpen, ChevronRight } from "lucide-react";
import { getJSON } from "@/lib/api";
import { cn } from "@/lib/utils";
import { SkeletonList } from "@/components/ui/skeleton";
import { ErrorText } from "@/components/JsonView";
import { type SkillLite } from "@/lib/agentdetail";
import { Row } from "@/components/agentdetail/shared";

interface SkillFile {
  path?: string;
  size?: number;
}
export function FilesTab({
  workdir,
  skills,
}: {
  workdir?: string;
  skills: SkillLite[] | null;
}) {
  const [openId, setOpenId] = useState<string | null>(null);
  return (
    <div className="space-y-3">
      <Row
        label="workdir"
        value={
          workdir ? (
            <span className="font-mono">{workdir}</span>
          ) : (
            "(workspace root — no dedicated subdir)"
          )
        }
      />
      <div>
        <div className="mb-1 text-xs uppercase tracking-normal text-muted">
          skill bundle files
        </div>
        {!skills ? (
          <SkeletonList count={2} lines={1} />
        ) : skills.length === 0 ? (
          <div className="text-[11px] text-muted">
            this agent owns no private skill bundles
          </div>
        ) : (
          <ul className="space-y-1.5">
            {skills.map((s) => (
              <li
                key={s.id}
                className="rounded-md border border-border bg-panel/30 p-2"
              >
                <button
                  onClick={() =>
                    setOpenId(openId === s.id ? null : (s.id ?? null))
                  }
                  className="flex w-full items-center gap-2 text-[11px]"
                >
                  <FolderOpen className="size-3.5 text-muted" />
                  <span className="font-medium">{s.name}</span>
                  <ChevronRight
                    className={cn(
                      "ml-auto size-3.5 text-muted transition-transform",
                      openId === s.id && "rotate-90",
                    )}
                  />
                </button>
                {openId === s.id && s.id && <SkillFiles id={s.id} />}
              </li>
            ))}
          </ul>
        )}
      </div>
    </div>
  );
}

function SkillFiles({ id }: { id: string }) {
  const [files, setFiles] = useState<SkillFile[] | null>(null);
  const [err, setErr] = useState<string | null>(null);
  useEffect(() => {
    let alive = true;
    getJSON<{ files?: SkillFile[] }>("/api/skill/files", { id })
      .then((d) => alive && setFiles(d.files || []))
      .catch((e) => alive && setErr((e as Error).message));
    return () => {
      alive = false;
    };
  }, [id]);
  if (err) return <ErrorText>{err}</ErrorText>;
  if (!files) return <SkeletonList count={2} lines={1} />;
  if (files.length === 0)
    return (
      <div className="mt-1 pl-5 text-[11px] text-muted">no bundled files</div>
    );
  return (
    <ul className="mt-1 space-y-0.5 pl-5">
      {files.map((f, i) => (
        <li
          key={i}
          className="flex items-center gap-2 font-mono text-xs text-muted"
        >
          <span className="truncate">{f.path}</span>
          {typeof f.size === "number" && (
            <span className="ml-auto shrink-0">{f.size}B</span>
          )}
        </li>
      ))}
    </ul>
  );
}

