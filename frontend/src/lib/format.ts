// microcents → dollar string. The kernel tracks spend in microcents (1e-9 USD).
export function money(mc?: number): string {
  return "$" + ((mc || 0) / 1e9).toFixed(4);
}

// rate (0..1) → integer percent string, or "—" when there's nothing to rate.
export function pct(rate?: number, denom?: number): string {
  if (denom !== undefined && !denom) return "—";
  return Math.round((rate || 0) * 100) + "%";
}

// fmtCount renders a token/char count compactly, with one decimal below 10K so
// small contexts don't all round to the same label: 9_850 → "9.9K", 12_400 →
// "12K", 1_500_000 → "1.5M".
export function fmtCount(n: number): string {
  if (n >= 1_000_000) return (n / 1_000_000).toFixed(n % 1_000_000 ? 1 : 0) + "M";
  if (n >= 10_000) return Math.round(n / 1000) + "K";
  if (n >= 1000) return (n / 1000).toFixed(1) + "K";
  return String(Math.round(n));
}

// sort object keys by descending numeric value (for "top by count" lists).
export function byDescValue(obj: Record<string, number>): string[] {
  return Object.keys(obj).sort((a, b) => (obj[b] || 0) - (obj[a] || 0));
}
