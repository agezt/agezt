import { useEffect, useMemo, useState } from "react";
import { X, ShieldCheck, Clock, Flame, AlertTriangle, Wrench, CheckCheck, Megaphone, Trash2, IdCard, HardDrive, AlertCircle, XCircle } from "lucide-react";
import { postJSON } from "@/lib/api";
import { cn } from "@/lib/utils";
import { Button } from "@/components/ui/button";
import { Badge } from "@/components/ui/badge";
import { SkeletonList } from "@/components/ui/skeleton";
import { useUI } from "@/components/ui/feedback";
import { type AgentProfile } from "@/views/Roster";
import { highImpactToolNames } from "@/lib/fleet";
import { AgentConfigPermissionRow, AgentPermissionsSnapshot, DetailOptionPicker, ToolCatalogRow } from "@/components/agentdetail/shared";
import { HIGH_IMPACT_LOCKDOWN_TOOLS, WorkflowToolAccessCard, addCsvItem, addCsvItems, addListItem, configAccessDetail, configAccessLabel, configOverridesText, effectiveToolPermissions, mcToUsdInput, normalizeNoiseToolPolicy, parseConfigOverridesText, permissionRowFromSnapshot, removeCsvItem, removeCsvItems, removeListItem, splitCsv, toolPolicyOverlap, usdToMcInput, workflowToolAccessSummary } from "@/components/agentdetail/capability";

export function CapabilityControlPanel({
  slug,
  profile,
  toolCatalog,
  edictLevels,
  agentPermissions,
  busy,
  onChanged,
}: {
  slug: string;
  profile: AgentProfile;
  toolCatalog: ToolCatalogRow[] | null;
  edictLevels: Record<string, string>;
  agentPermissions: AgentPermissionsSnapshot | null;
  busy: boolean;
  onChanged: () => void;
}) {
  const ui = useUI();
  const [trust, setTrust] = useState<AgentProfile["trust_ceiling"]>(profile.trust_ceiling || "L4");
  const [allow, setAllow] = useState((profile.tool_allow || []).join(", "));
  const [deny, setDeny] = useState((profile.tool_deny || []).join(", "));
  const [config, setConfig] = useState(configOverridesText(profile.config_overrides));
  const [memoryScope, setMemoryScope] = useState(profile.memory_scope || "");
  const [workdir, setWorkdir] = useState(profile.workdir || "");
  const [maxCost, setMaxCost] = useState(mcToUsdInput(profile.max_cost_mc));
  const [maxDaily, setMaxDaily] = useState(mcToUsdInput(profile.max_daily_mc));
  const [silentOnSuccess, setSilentOnSuccess] = useState(!!profile.noise_policy?.silent_on_success);
  const [disableMemoryWrites, setDisableMemoryWrites] = useState(!!profile.noise_policy?.disable_memory_writes);
  const [notifySeverity, setNotifySeverity] = useState<NonNullable<NonNullable<AgentProfile["noise_policy"]>["min_notify_severity"]>>(profile.noise_policy?.min_notify_severity || "info");
  const [notifyCooldown, setNotifyCooldown] = useState(String(profile.noise_policy?.min_notify_interval_sec || 0));
  const [saving, setSaving] = useState(false);
  const draftAllow = splitCsv(allow);
  const draftDeny = splitCsv(deny);
  const draftProfile = useMemo(
    () => ({
      ...profile,
      trust_ceiling: trust,
      tool_allow: draftAllow,
      tool_deny: draftDeny,
      memory_scope: memoryScope,
      workdir,
    }),
    [draftAllow, draftDeny, memoryScope, profile, trust, workdir],
  );
  const effective = useMemo(
    () =>
      toolCatalog && toolCatalog.length > 0
        ? effectiveToolPermissions(toolCatalog, draftProfile, edictLevels)
        : agentPermissions?.permissions
        ? agentPermissions.permissions.map(permissionRowFromSnapshot)
        : [],
    [agentPermissions, draftProfile, edictLevels, toolCatalog],
  );
  const configEntries = useMemo(
    () => [...(agentPermissions?.config_entries || [])].sort((a, b) => a.key.localeCompare(b.key)),
    [agentPermissions],
  );
  const highImpactLockdownTools = useMemo(() => {
    const names = effective.length > 0
      ? [...HIGH_IMPACT_LOCKDOWN_TOOLS, ...effective.map((row) => row.name)]
      : HIGH_IMPACT_LOCKDOWN_TOOLS;
    return highImpactToolNames(names, HIGH_IMPACT_LOCKDOWN_TOOLS.length);
  }, [effective]);

  useEffect(() => {
    setTrust(profile.trust_ceiling || "L4");
    setAllow((profile.tool_allow || []).join(", "));
    setDeny((profile.tool_deny || []).join(", "));
    setConfig(configOverridesText(profile.config_overrides));
    setMemoryScope(profile.memory_scope || "");
    setWorkdir(profile.workdir || "");
    setMaxCost(mcToUsdInput(profile.max_cost_mc));
    setMaxDaily(mcToUsdInput(profile.max_daily_mc));
    setSilentOnSuccess(!!profile.noise_policy?.silent_on_success);
    setDisableMemoryWrites(!!profile.noise_policy?.disable_memory_writes);
    setNotifySeverity(profile.noise_policy?.min_notify_severity || "info");
    setNotifyCooldown(String(profile.noise_policy?.min_notify_interval_sec || 0));
  }, [profile]);

  async function save() {
    const policyTools = normalizeNoiseToolPolicy(draftAllow, draftDeny, disableMemoryWrites);
    const overlap = toolPolicyOverlap(policyTools.allow, policyTools.deny);
    if (overlap) {
      ui.toast(`Tool ${overlap} cannot be both allowed and denied`, "error");
      return;
    }
    const parsed = parseConfigOverridesText(config);
    if (typeof parsed === "string") {
      ui.toast(parsed, "error");
      return;
    }
    const maxCostMc = usdToMcInput(maxCost);
    if (maxCostMc === null) {
      ui.toast("Max/run must be a dollar amount like 0.05", "error");
      return;
    }
    const maxDailyMc = usdToMcInput(maxDaily);
    if (maxDailyMc === null) {
      ui.toast("Max/day must be a dollar amount like 0.50", "error");
      return;
    }
    setSaving(true);
    try {
      await postJSON("/api/agents/capabilities", {
        ref: slug,
        trust_ceiling: trust,
        tool_allow: policyTools.allow,
        tool_deny: policyTools.deny,
        memory_scope: memoryScope.trim(),
        workdir: workdir.trim(),
        max_cost_mc: maxCostMc,
        max_daily_mc: maxDailyMc,
        noise_policy: {
          silent_on_success: silentOnSuccess,
          disable_memory_writes: disableMemoryWrites,
          min_notify_severity: notifySeverity,
          min_notify_interval_sec: Math.max(0, Number.parseInt(notifyCooldown || "0", 10) || 0),
        },
        config_overrides: parsed,
      });
      ui.toast(`${slug} capability policy updated`, "success");
      onChanged();
    } catch (e) {
      ui.toast((e as Error).message, "error");
    } finally {
      setSaving(false);
    }
  }

  function allowTool(name: string) {
    setAllow(addCsvItem(allow, name));
    setDeny(removeCsvItem(deny, name));
  }

  function denyTool(name: string) {
    setDeny(addCsvItem(deny, name));
    setAllow(removeCsvItem(allow, name));
  }

  function clearTool(name: string) {
    setAllow(removeCsvItem(allow, name));
    setDeny(removeCsvItem(deny, name));
  }

  function applyQuietSystemPreset() {
    setTrust((current) => (current === "L4" ? "L2" : current));
    setMemoryScope((current) => current.trim() || `system/${slug}`);
    setWorkdir((current) => current.trim() || `system/${slug}`);
    setMaxCost("0.05");
    setMaxDaily("0.05");
    setSilentOnSuccess(true);
    setDisableMemoryWrites(true);
    setNotifySeverity("warning");
    setNotifyCooldown("28800");
  }

  function applyRestrictedWorkerPreset() {
    setTrust("L2");
    setAllow("memory, notify");
    setDeny("");
    setWorkdir((current) => current.trim() || `agents/${slug}`);
    setMaxCost("0.25");
    setMaxDaily("1");
    setSilentOnSuccess(true);
    setDisableMemoryWrites(true);
    setNotifySeverity("warning");
    setNotifyCooldown("3600");
  }

  function applyHighImpactLockdownPreset() {
    setTrust((current) => (current === "L4" ? "L2" : current));
    setAllow(removeCsvItems(allow, highImpactLockdownTools));
    setDeny(addCsvItems(deny, highImpactLockdownTools));
    setSilentOnSuccess(true);
    setNotifySeverity("warning");
    setNotifyCooldown((current) => {
      const seconds = Number.parseInt(current || "0", 10) || 0;
      return String(Math.max(seconds, 3600));
    });
  }

  function applyOpenLabPreset() {
    setTrust("L4");
    setAllow("");
    setDeny("");
    setMaxCost("");
    setMaxDaily("");
    setSilentOnSuccess(false);
    setDisableMemoryWrites(false);
    setNotifySeverity("info");
    setNotifyCooldown("0");
  }

  async function updateConfigAccess(row: AgentConfigPermissionRow, mode: "allow" | "exclude" | "clear") {
    const allowed = row.allowed_agents || [];
    const excluded = row.excluded_agents || [];
    const nextAllowed =
      mode === "allow"
        ? addListItem(allowed, slug)
        : mode === "clear"
          ? removeListItem(allowed, slug)
          : removeListItem(allowed, slug);
    const nextExcluded =
      mode === "exclude"
        ? addListItem(excluded, slug)
        : mode === "clear"
          ? removeListItem(excluded, slug)
          : removeListItem(excluded, slug);
    setSaving(true);
    try {
      await postJSON("/api/configcenter/access", {
        key: row.key,
        allowed_agents: nextAllowed,
        excluded_agents: nextExcluded,
      });
      ui.toast(`${row.key} access updated for ${slug}`, "success");
      onChanged();
    } catch (e) {
      ui.toast((e as Error).message, "error");
    } finally {
      setSaving(false);
    }
  }

  return (
    <div className="rounded-lg border border-accent/30 bg-accent/5 p-2.5">
      <div className="mb-2 flex flex-wrap items-center gap-2">
        <div className="min-w-0 flex-1">
          <div className="flex items-center gap-1.5 text-xs font-semibold uppercase tracking-normal text-accent">
            <ShieldCheck className="size-3" /> Capability control
          </div>
          <div className="mt-1 text-[11px] text-muted">
            Tool access, trust ceiling, spend caps, memory scope, and runtime config for this identity.
          </div>
        </div>
        <Button size="sm" disabled={busy || saving} onClick={save}>
          <Wrench className="size-3.5" /> Save
        </Button>
      </div>
      <div className="grid gap-2 md:grid-cols-3">
        <div className="md:col-span-1">
          <DetailOptionPicker
            label="Trust ceiling"
            value={trust || "L4"}
            onChange={(level) => setTrust(level as AgentProfile["trust_ceiling"])}
            columns="grid-cols-2"
            options={[
              { value: "L4", label: "L4", detail: "allow", icon: <ShieldCheck className="size-3.5" /> },
              { value: "L3", label: "L3", detail: "ask scoped", icon: <ShieldCheck className="size-3.5" /> },
              { value: "L2", label: "L2", detail: "ask first", icon: <AlertTriangle className="size-3.5" /> },
              { value: "L1", label: "L1", detail: "ask always", icon: <AlertCircle className="size-3.5" /> },
              { value: "L0", label: "L0", detail: "deny", icon: <XCircle className="size-3.5" /> },
            ]}
          />
        </div>
        <label className="flex flex-col gap-1 text-[11px] text-muted">
          Tool allow
          <input
            value={allow}
            onChange={(e) => setAllow(e.target.value)}
            placeholder="shell, memory, mcp_fake_greet"
            className="rounded-md border border-border bg-card px-2 py-1 text-sm text-foreground outline-none focus-visible:border-accent"
          />
        </label>
        <label className="flex flex-col gap-1 text-[11px] text-muted">
          Tool deny
          <input
            value={deny}
            onChange={(e) => setDeny(e.target.value)}
            placeholder="notify, shell"
            className="rounded-md border border-border bg-card px-2 py-1 text-sm text-foreground outline-none focus-visible:border-accent"
          />
        </label>
      </div>
      <div className="mt-2 flex flex-wrap items-center gap-1.5 rounded-md border border-border bg-card/55 px-2 py-1.5">
        <span className="mr-1 text-xs font-semibold uppercase tracking-normal text-muted">Policy presets</span>
        <Button type="button" variant="ghost" size="sm" onClick={applyQuietSystemPreset} title="Apply quiet guardian defaults">
          <Megaphone className="size-3.5" /> Quiet system preset
        </Button>
        <Button type="button" variant="ghost" size="sm" onClick={applyRestrictedWorkerPreset} title="Allow only memory/notify with quiet output">
          <ShieldCheck className="size-3.5" /> Restricted worker preset
        </Button>
        <Button type="button" variant="ghost" size="sm" onClick={applyHighImpactLockdownPreset} title={`Deny high-impact tools: ${highImpactLockdownTools.join(", ")}`}>
          <ShieldCheck className="size-3.5" /> High-impact lockdown
        </Button>
        <Button type="button" variant="ghost" size="sm" onClick={applyOpenLabPreset} title="Clear agent-specific tool restrictions">
          <Wrench className="size-3.5" /> Open lab preset
        </Button>
      </div>
      <div className="mt-2 grid gap-2 md:grid-cols-[1fr_1fr_12rem_12rem]">
        <button
          type="button"
          onClick={() => setSilentOnSuccess(!silentOnSuccess)}
          className={cn(
            "flex items-center gap-2 rounded-lg border px-3 py-2 transition-all",
            silentOnSuccess
              ? "border-good/50 bg-good/15 shadow-sm shadow-good/20"
              : "border-border/50 bg-panel/30 hover:border-border"
          )}
        >
          <div className={cn(
            "flex h-8 w-8 items-center justify-center rounded-lg",
            silentOnSuccess ? "bg-good/25" : "bg-panel/60"
          )}>
            <ShieldCheck className={cn("size-4", silentOnSuccess ? "text-good" : "text-muted/50")} />
          </div>
          <div className="flex flex-col text-left">
            <span className={cn("text-xs font-medium", silentOnSuccess ? "text-foreground" : "text-muted")}>
              Silent on success
            </span>
            <span className="text-xs text-muted">suppress OK logs</span>
          </div>
          <div className={cn(
            "ml-auto flex h-5 w-10 items-center rounded-full border px-0.5 transition-colors",
            silentOnSuccess ? "border-good/50 bg-good/30" : "border-border bg-panel"
          )}>
            <div className={cn(
              "h-3.5 w-3.5 rounded-full transition-transform",
              silentOnSuccess ? "translate-x-5 bg-good shadow-sm shadow-good/50" : "translate-x-0 bg-muted/40"
            )} />
          </div>
        </button>
        <button
          type="button"
          onClick={() => setDisableMemoryWrites(!disableMemoryWrites)}
          className={cn(
            "flex items-center gap-2 rounded-lg border px-3 py-2 transition-all",
            disableMemoryWrites
              ? "border-warn/50 bg-warn/15 shadow-sm shadow-warn/20"
              : "border-border/50 bg-panel/30 hover:border-border"
          )}
        >
          <div className={cn(
            "flex h-8 w-8 items-center justify-center rounded-lg",
            disableMemoryWrites ? "bg-warn/25" : "bg-panel/60"
          )}>
            <HardDrive className={cn("size-4", disableMemoryWrites ? "text-warn" : "text-muted/50")} />
          </div>
          <div className="flex flex-col text-left">
            <span className={cn("text-xs font-medium", disableMemoryWrites ? "text-foreground" : "text-muted")}>
              Memory writes
            </span>
            <span className="text-xs text-muted">
              {disableMemoryWrites ? "blocked" : "allowed"}
            </span>
          </div>
          <div className={cn(
            "ml-auto flex h-5 w-10 items-center rounded-full border px-0.5 transition-colors",
            disableMemoryWrites ? "border-warn/50 bg-warn/30" : "border-border bg-panel"
          )}>
            <div className={cn(
              "h-3.5 w-3.5 rounded-full transition-transform",
              disableMemoryWrites ? "translate-x-5 bg-warn shadow-sm shadow-warn/50" : "translate-x-0 bg-muted/40"
            )} />
          </div>
        </button>
        <DetailOptionPicker
          label="Notify level"
          value={notifySeverity}
          onChange={setNotifySeverity}
          columns="grid-cols-1"
          options={[
            { value: "info", label: "Info", detail: "all notifications", icon: <Megaphone className="size-3.5" /> },
            { value: "warning", label: "Warning", detail: "warn and critical", icon: <AlertTriangle className="size-3.5" /> },
            { value: "critical", label: "Critical", detail: "critical only", icon: <Flame className="size-3.5" /> },
          ]}
        />
        <label className="flex flex-col gap-1 text-[11px] text-muted">
          <span className="flex items-center gap-1"><Clock className="size-3" /> Cooldown (sec)</span>
          <input
            type="number"
            min={0}
            value={notifyCooldown}
            onChange={(e) => setNotifyCooldown(e.target.value)}
            className="rounded-md border border-border bg-card px-2 py-1.5 font-mono text-sm text-foreground outline-none focus-visible:border-accent"
          />
        </label>
      </div>
      <div className="mt-2 grid gap-2 md:grid-cols-4">
        <label className="flex flex-col gap-1 text-[11px] text-muted">
          Memory scope
          <input
            value={memoryScope}
            onChange={(e) => setMemoryScope(e.target.value)}
            placeholder={`agent/${slug}`}
            className="rounded-md border border-border bg-card px-2 py-1 font-mono text-sm text-foreground outline-none focus-visible:border-accent"
          />
        </label>
        <label className="flex flex-col gap-1 text-[11px] text-muted">
          Workspace subdir
          <input
            value={workdir}
            onChange={(e) => setWorkdir(e.target.value)}
            placeholder={`agents/${slug}`}
            className="rounded-md border border-border bg-card px-2 py-1 font-mono text-sm text-foreground outline-none focus-visible:border-accent"
          />
        </label>
        <label className="flex flex-col gap-1 text-[11px] text-muted">
          Max/run ($)
          <input
            type="text"
            inputMode="decimal"
            value={maxCost}
            onChange={(e) => setMaxCost(e.target.value)}
            placeholder="0.05"
            className="rounded-md border border-border bg-card px-2 py-1 text-sm text-foreground outline-none focus-visible:border-accent"
          />
        </label>
        <label className="flex flex-col gap-1 text-[11px] text-muted">
          Max/day ($)
          <input
            type="text"
            inputMode="decimal"
            value={maxDaily}
            onChange={(e) => setMaxDaily(e.target.value)}
            placeholder="0.50"
            className="rounded-md border border-border bg-card px-2 py-1 text-sm text-foreground outline-none focus-visible:border-accent"
          />
        </label>
      </div>
      <label className="mt-2 flex flex-col gap-1 text-[11px] text-muted">
        Config overrides
        <textarea
          value={config}
          onChange={(e) => setConfig(e.target.value)}
          rows={3}
          placeholder="AGEZT_PROVIDER=openai"
          className="rounded-md border border-border bg-card px-2 py-1 font-mono text-xs text-foreground outline-none focus-visible:border-accent"
        />
      </label>
      <div className="mt-3">
        <div className="mb-1 flex items-center gap-2 text-xs font-semibold uppercase tracking-normal text-muted">
          Config center access
          <span className="rounded bg-card px-1.5 py-0.5 font-mono text-[9px]">
            {configEntries.filter((row) => row.visible).length}/{configEntries.length}
          </span>
        </div>
        {!agentPermissions ? (
          <SkeletonList count={2} lines={1} />
        ) : configEntries.length === 0 ? (
          <div className="text-[11px] text-muted">no config center entries are visible to this daemon snapshot</div>
        ) : (
          <div className="max-h-44 overflow-auto rounded-lg border border-border bg-card/55">
            <table className="w-full text-left text-[11px]">
              <thead className="sticky top-0 bg-card text-xs uppercase tracking-normal text-muted">
                <tr>
                  <th className="px-2 py-1.5 font-medium">Key</th>
                  <th className="px-2 py-1.5 font-medium">Rating</th>
                  <th className="px-2 py-1.5 font-medium">Access</th>
                  <th className="px-2 py-1.5 font-medium">Scope</th>
                  <th className="px-2 py-1.5 font-medium">Policy</th>
                </tr>
              </thead>
              <tbody>
                {configEntries.map((row) => (
                  <tr key={row.key} className="border-t border-border/70">
                    <td className="max-w-52 truncate px-2 py-1.5 font-mono text-foreground/85" title={row.description || row.key}>
                      {row.key}
                    </td>
                    <td className="px-2 py-1.5 text-muted">{row.rating || "-"}</td>
                    <td className="px-2 py-1.5">
                      <Badge variant={row.visible ? "good" : "bad"}>
                        {row.visible ? "visible" : "blocked"}
                      </Badge>
                    </td>
                    <td className="max-w-56 truncate px-2 py-1.5 text-muted" title={configAccessDetail(row)}>
                      {configAccessLabel(row)}
                    </td>
                    <td className="px-2 py-1.5">
                      <div className="flex items-center gap-1">
                        <Button
                          type="button"
                          variant="ghost"
                          size="icon"
                          className="h-6 w-6 text-good"
                          title={`Allow ${slug} for ${row.key}`}
                          aria-label={`Allow ${slug} for ${row.key}`}
                          disabled={saving}
                          onClick={() => updateConfigAccess(row, "allow")}
                        >
                          <CheckCheck className="size-3.5" />
                        </Button>
                        <Button
                          type="button"
                          variant="ghost"
                          size="icon"
                          className="h-6 w-6 text-bad"
                          title={`Exclude ${slug} from ${row.key}`}
                          aria-label={`Exclude ${slug} from ${row.key}`}
                          disabled={saving}
                          onClick={() => updateConfigAccess(row, "exclude")}
                        >
                          <X className="size-3.5" />
                        </Button>
                        <Button
                          type="button"
                          variant="ghost"
                          size="icon"
                          className="h-6 w-6 text-muted"
                          title={`Clear ${slug} config policy for ${row.key}`}
                          aria-label={`Clear ${slug} config policy for ${row.key}`}
                          disabled={saving}
                          onClick={() => updateConfigAccess(row, "clear")}
                        >
                          <Trash2 className="size-3.5" />
                        </Button>
                      </div>
                    </td>
                  </tr>
                ))}
              </tbody>
            </table>
          </div>
        )}
      </div>
      <div className="mt-3">
        <div className="mb-1 flex items-center gap-2 text-xs font-semibold uppercase tracking-normal text-muted">
          Effective tool access
          <span className="rounded bg-card px-1.5 py-0.5 font-mono text-[9px]">
            {effective.filter((row) => row.allowed).length}/{effective.length}
          </span>
        </div>
        <WorkflowToolAccessCard summary={workflowToolAccessSummary(effective)} />
        {!toolCatalog && !agentPermissions ? (
          <SkeletonList count={2} lines={1} />
        ) : effective.length === 0 ? (
          <div className="text-[11px] text-muted">no registered tools are visible to this daemon</div>
        ) : (
          <div className="max-h-56 overflow-auto rounded-lg border border-border bg-card/55">
            <table className="w-full text-left text-[11px]">
              <thead className="sticky top-0 bg-card text-xs uppercase tracking-normal text-muted">
                <tr>
                  <th className="px-2 py-1.5 font-medium">Tool</th>
                  <th className="px-2 py-1.5 font-medium">Capability</th>
                  <th className="px-2 py-1.5 font-medium">Effective</th>
                  <th className="px-2 py-1.5 font-medium">Source</th>
                  <th className="px-2 py-1.5 font-medium">Policy</th>
                </tr>
              </thead>
              <tbody>
                {effective.map((row) => (
                  <tr key={row.name} className="border-t border-border/70">
                    <td className="max-w-40 truncate px-2 py-1.5 font-mono text-foreground/85" title={row.description || row.name}>
                      {row.name}
                    </td>
                    <td className="px-2 py-1.5 font-mono text-muted">{row.capability || "-"}</td>
                    <td className="px-2 py-1.5">
                      <Badge variant={row.allowed ? "good" : row.ask ? "warn" : "bad"}>
                        {row.label}
                      </Badge>
                    </td>
                    <td className="px-2 py-1.5 text-muted">{row.reason}</td>
                    <td className="px-2 py-1.5">
                      <div className="flex items-center gap-1">
                        <Button
                          type="button"
                          variant="ghost"
                          size="icon"
                          className="h-6 w-6 text-good"
                          title={`Allow ${row.name}`}
                          aria-label={`Allow ${row.name}`}
                          onClick={() => allowTool(row.name)}
                        >
                          <CheckCheck className="size-3.5" />
                        </Button>
                        <Button
                          type="button"
                          variant="ghost"
                          size="icon"
                          className="h-6 w-6 text-bad"
                          title={`Deny ${row.name}`}
                          aria-label={`Deny ${row.name}`}
                          onClick={() => denyTool(row.name)}
                        >
                          <X className="size-3.5" />
                        </Button>
                        <Button
                          type="button"
                          variant="ghost"
                          size="icon"
                          className="h-6 w-6 text-muted"
                          title={`Clear ${row.name} policy`}
                          aria-label={`Clear ${row.name} policy`}
                          onClick={() => clearTool(row.name)}
                        >
                          <Trash2 className="size-3.5" />
                        </Button>
                      </div>
                    </td>
                  </tr>
                ))}
              </tbody>
            </table>
          </div>
        )}
      </div>
    </div>
  );
}

