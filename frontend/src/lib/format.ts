// microcents → dollar string. The kernel tracks spend in microcents (1e-9 USD).
export function money(mc?: number): string {
  return "$" + ((mc || 0) / 1e9).toFixed(4);
}

// rate (0..1) → integer percent string, or "—" when there's nothing to rate.
export function pct(rate?: number, denom?: number): string {
  if (denom !== undefined && !denom) return "—";
  return Math.round((rate || 0) * 100) + "%";
}

// sort object keys by descending numeric value (for "top by count" lists).
export function byDescValue(obj: Record<string, number>): string[] {
  return Object.keys(obj).sort((a, b) => (obj[b] || 0) - (obj[a] || 0));
}
