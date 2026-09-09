// incidentnav — deep-link addressing for a doctor/escalation incident tree.
// Like agentnav, this reserves one hash prefix, `#incident/<id>`, so a doctor
// incident is bookmarkable and survives reloads.

export const INCIDENT_HASH_PREFIX = "incident/";

export function openIncident(id: string): void {
  if (!id) return;
  location.hash = INCIDENT_HASH_PREFIX + encodeURIComponent(id);
}

export function incidentIdFromHash(
  hash: string = typeof location === "undefined" ? "" : location.hash,
): string | null {
  const raw = hash.replace(/^#\/?/, "");
  if (!raw.startsWith(INCIDENT_HASH_PREFIX)) return null;
  const encoded = raw.slice(INCIDENT_HASH_PREFIX.length);
  if (!encoded) return null;
  try {
    return decodeURIComponent(encoded) || null;
  } catch {
    return encoded || null;
  }
}
