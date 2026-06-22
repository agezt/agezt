// Pure aggregation of the runs list into chart-ready series for the Insights
// view. No React, no fetch — unit-tested directly. The kernel stays the source
// of truth (these are derived views over /api/runs).

export interface RunRow {
  correlation_id?: string;
  status?: string;
  model?: string;
  spent_mc?: number;
  started_unix_ms?: number;
  duration_ms?: number;
  iters?: number;
  parent_correlation?: string;
}

export interface ModelStat {
  model: string;
  runs: number;
  spentMc: number;
  avgSpentMc: number; // spentMc / runs — cost per run
  avgIters: number; // mean iters across that model's runs (0 if unknown)
}

// Internal accumulator; promoted to ModelStat once averages are computed.
interface ModelAcc {
  model: string;
  runs: number;
  spentMc: number;
  iters: number;
}
export interface SpendPoint {
  t: number; // started_unix_ms
  spentMc: number; // this run's spend
  cum: number; // cumulative spend through this run
}

export interface Insights {
  total: number;
  completed: number;
  failed: number;
  running: number;
  totalSpentMc: number;
  successRate: number; // 0..1 over finished runs
  avgDurationMs: number;
  avgIters: number;
  byModel: ModelStat[]; // sorted desc by spend
  spend: SpendPoint[]; // chronological, cumulative
}

function n(v: unknown): number {
  const x = Number(v);
  return Number.isFinite(x) ? x : 0;
}

export function computeInsights(runs: RunRow[]): Insights {
  let completed = 0;
  let failed = 0;
  let running = 0;
  let totalSpentMc = 0;
  let durSum = 0;
  let durCount = 0;
  let iterSum = 0;
  let iterCount = 0;
  const models = new Map<string, ModelAcc>();

  for (const r of runs) {
    const spent = n(r.spent_mc);
    totalSpentMc += spent;
    switch (r.status) {
      case "completed":
        completed++;
        break;
      case "failed":
        failed++;
        break;
      case "running":
        running++;
        break;
    }
    if (r.duration_ms != null && n(r.duration_ms) > 0) {
      durSum += n(r.duration_ms);
      durCount++;
    }
    if (n(r.iters) > 0) {
      iterSum += n(r.iters);
      iterCount++;
    }
    const m = r.model || "—";
    const ms = models.get(m) || { model: m, runs: 0, spentMc: 0, iters: 0 };
    ms.runs++;
    ms.spentMc += spent;
    ms.iters += n(r.iters);
    models.set(m, ms);
  }

  // Cumulative spend over time, oldest→newest.
  const spend: SpendPoint[] = [];
  let cum = 0;
  for (const r of [...runs].sort((a, b) => n(a.started_unix_ms) - n(b.started_unix_ms))) {
    const spent = n(r.spent_mc);
    cum += spent;
    spend.push({ t: n(r.started_unix_ms), spentMc: spent, cum });
  }

  const finished = completed + failed;
  return {
    total: runs.length,
    completed,
    failed,
    running,
    totalSpentMc,
    successRate: finished ? completed / finished : 0,
    avgDurationMs: durCount ? durSum / durCount : 0,
    avgIters: iterCount ? iterSum / iterCount : 0,
    byModel: [...models.values()]
      .map((ms): ModelStat => ({
        model: ms.model,
        runs: ms.runs,
        spentMc: ms.spentMc,
        avgSpentMc: ms.runs > 0 ? ms.spentMc / ms.runs : 0,
        avgIters: ms.runs > 0 ? ms.iters / ms.runs : 0,
      }))
      .sort((a, b) => b.spentMc - a.spentMc),
    spend,
  };
}
