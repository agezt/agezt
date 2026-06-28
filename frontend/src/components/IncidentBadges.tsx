import {
  doctorIncidentPhase,
  doctorIncidentSourceLabel,
  type AutonomyItem,
  type DoctorIncidentPhaseTone,
} from "@/lib/autonomy";

type IncidentBadgeItem = Pick<AutonomyItem, "subject" | "phase" | "mode">;

function incidentSourceBadgeClass(
  source: string,
  mono = false,
): string {
  const weight = mono ? "font-mono" : "font-medium";
  return source === "operator"
    ? `rounded bg-accent/10 px-1.5 py-0.5 text-xs ${weight} text-accent`
    : `rounded bg-warn/10 px-1.5 py-0.5 text-xs ${weight} text-warn`;
}

export function incidentPhaseBadgeClass(
  tone: DoctorIncidentPhaseTone,
  mono = false,
): string {
  const weight = mono ? "font-mono" : "font-medium";
  switch (tone) {
    case "accent":
      return `rounded bg-accent/10 px-1.5 py-0.5 text-xs ${weight} text-accent`;
    case "warn":
      return `rounded bg-warn/10 px-1.5 py-0.5 text-xs ${weight} text-warn`;
    case "good":
      return `rounded bg-good/10 px-1.5 py-0.5 text-xs ${weight} text-good`;
    case "bad":
      return `rounded bg-bad/10 px-1.5 py-0.5 text-xs ${weight} text-bad`;
    default:
      return `rounded bg-panel px-1.5 py-0.5 text-xs ${weight} text-muted`;
  }
}

export function IncidentBadges({
  item,
  mono = false,
}: {
  item: IncidentBadgeItem | null | undefined;
  mono?: boolean;
}) {
  const source = doctorIncidentSourceLabel(item);
  const phase = doctorIncidentPhase(item);
  if (!source && !phase) return null;
  return (
    <>
      {source && (
        <span className={incidentSourceBadgeClass(source, mono)}>{source}</span>
      )}
      {phase && (
        <span className={incidentPhaseBadgeClass(phase.tone, mono)}>
          {phase.label}
        </span>
      )}
    </>
  );
}
