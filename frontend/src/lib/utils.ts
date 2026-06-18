import { clsx, type ClassValue } from "clsx";
import { twMerge } from "tailwind-merge";

export function cn(...inputs: ClassValue[]): string {
  return twMerge(clsx(inputs));
}

export function clip(s: unknown, n: number): string {
  const str = s == null ? "" : String(s);
  return str.length > n ? str.slice(0, n - 1) + "…" : str;
}

export function prettyJSON(s: string): string {
  try {
    return JSON.stringify(JSON.parse(s), null, 2);
  } catch {
    return s || "";
  }
}

export function fmtTime(ms?: number): string {
  if (!ms) return "";
  try {
    return new Date(ms).toLocaleTimeString();
  } catch {
    return "";
  }
}

export function fmtDateTime(ms?: number): string {
  if (!ms) return "";
  try {
    return new Date(ms).toLocaleString();
  } catch {
    return "";
  }
}

// fmtAgo renders a coarse relative time ("3m ago", "2d ago") — for "when was
// this last used / seen" labels where recency matters more than the exact clock.
export function fmtAgo(ms?: number): string {
  if (!ms) return "";
  const diff = Date.now() - ms;
  if (diff < 60_000) return "just now";
  const m = Math.floor(diff / 60_000);
  if (m < 60) return `${m}m ago`;
  const h = Math.floor(m / 60);
  if (h < 24) return `${h}h ago`;
  const d = Math.floor(h / 24);
  if (d < 30) return `${d}d ago`;
  const mo = Math.floor(d / 30);
  if (mo < 12) return `${mo}mo ago`;
  return `${Math.floor(mo / 12)}y ago`;
}

// fmtDue renders a coarse future/past deadline ("in 10m", "overdue 3m") for
// scheduled wakeups and other operational timers.
export function fmtDue(ms?: number, now = Date.now()): string {
  if (!ms || !Number.isFinite(ms) || ms <= 0) return "unknown";
  const delta = ms - now;
  const abs = Math.abs(delta);
  const minutes = Math.max(1, Math.round(abs / 60_000));
  const hours = Math.floor(minutes / 60);
  const days = Math.floor(hours / 24);
  let text: string;
  if (days >= 1) {
    text = `${days}d ${hours % 24}h`;
  } else if (hours >= 1) {
    text = `${hours}h ${minutes % 60}m`;
  } else {
    text = `${minutes}m`;
  }
  return delta < 0 ? `overdue ${text}` : `in ${text}`;
}
