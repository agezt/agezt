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

// bytes renders a byte count the way an operator reads it. Three views had each
// written this (Sandbox and Storage as `fmtBytes`, the file workspace and the
// artifact domain as `humanSize`) and they disagreed on the zero case — "—" in
// one, "0 B" in another — so the same empty file read differently per page.
export function bytes(n?: number): string {
  // Unknown and zero are different facts. `undefined` is "we were not told the
  // size" and reads as a dash; 0 is "this file is empty" and reads as 0 B. The
  // old copies collapsed both into "—", so an empty file looked like a missing
  // one — which is exactly the sort of quiet lie a shared helper should not tell.
  if (n === undefined || n === null || Number.isNaN(n) || n < 0) return "—";
  if (n === 0) return "0 B";
  if (n < 1024) return `${n} B`;
  if (n < 1024 ** 2) return `${(n / 1024).toFixed(1)} KB`;
  if (n < 1024 ** 3) return `${(n / 1024 ** 2).toFixed(1)} MB`;
  // Disk-scale numbers get a second decimal: the difference between 2.1 GB and
  // 2.15 GB matters when you are deciding what to reclaim. The artifact-side
  // copy stopped at MB and printed "2048.0 MB" for 2 GB.
  if (n < 1024 ** 4) return `${(n / 1024 ** 3).toFixed(2)} GB`;
  return `${(n / 1024 ** 4).toFixed(2)} TB`;
}
