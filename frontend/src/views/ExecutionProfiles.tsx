import { useEffect, useMemo, useState } from "react";
import {
  AlertTriangle,
  Boxes,
  CheckCircle2,
  Globe,
  HardDrive,
  Lock,
  RefreshCw,
  Route,
  Save,
  Server,
  Shield,
  Terminal,
  Timer,
  Wrench,
  XOctagon,
  type LucideIcon,
} from "lucide-react";
import { getJSON, postJSON } from "@/lib/api";
import { cn } from "@/lib/utils";
import { Button } from "@/components/ui/button";
import { Badge } from "@/components/ui/badge";
import { Input } from "@/components/ui/input";
import { Page } from "@/components/ui/page";
import { Advanced, Disclosure } from "@/components/ui/disclosure";
import { SkeletonList } from "@/components/ui/skeleton";
import { ErrorText, Muted } from "@/components/JsonView";

export interface ExecutionProfile {
  id?: string;
  name?: string;
  summary?: string;
  status?: string;
  routed?: boolean;
  requested_isolation?: string;
  effective_isolation?: string;
  degraded?: boolean;
  degrade_reason?: string;
  tools?: string[];
  backends?: string[];
  filesystem?: string;
  network?: string;
  environment?: string;
  secrets?: string;
  secret_policy?: {
    mode?: string;
    scope?: string;
    values_forwarded?: boolean;
    metadata_forwarded?: boolean;
    valid?: boolean;
    detail?: string;
  };
  limits?: string[];
  browser_access?: string;
  cleanup?: string;
  policy_capability?: string;
  notes?: string[];
}

export interface ExecutionProfileInventory {
  host_os?: string;
  host_arch?: string;
  profiles?: ExecutionProfile[];
  count?: number;
  routed_count?: number;
  supported_count?: number;
  degraded_count?: number;
}

export interface ExecutionProfileCheck {
  id?: string;
  profile_id?: string;
  status?: "ok" | "warning" | "fail" | string;
  title?: string;
  detail?: string;
  next?: string;
  routed?: boolean;
  degraded?: boolean;
  backend_available?: boolean;
  backend?: string;
}

export interface ExecutionProfileHealthReport {
  host_os?: string;
  host_arch?: string;
  checks?: ExecutionProfileCheck[];
  count?: number;
  ok_count?: number;
  warning_count?: number;
  fail_count?: number;
  routable_run_profiles?: string[];
}

export interface ConfigValueEntry {
  env?: string;
  value?: string;
  set?: boolean;
  env_pinned?: boolean;
}

interface ConfigValuesResponse {
  fields?: ConfigValueEntry[];
}

export interface ExecutionProfilePolicyValues {
  allow: string;
  deny: string;
  allowPinned: boolean;
  denyPinned: boolean;
}

export interface ExecutionProfileBackendValues {
  sshEnabled: boolean;
  sshTarget: string;
  sshWorkDir: string;
  sshIdentity: string;
  sshPort: string;
  sshStrictHostKey: string;
  k8sEnabled: boolean;
  k8sContext: string;
  k8sNamespace: string;
  k8sPod: string;
  k8sContainer: string;
  k8sWorkDir: string;
  modalEnabled: boolean;
  modalRef: string;
  modalImage: string;
  modalEnvironment: string;
  modalAddPython: string;
  modalWorkDir: string;
  daytonaEnabled: boolean;
  daytonaSandbox: string;
  daytonaWorkDir: string;
  dockerEnabled: boolean;
  dockerRuntime: string;
  dockerImage: string;
  dockerNetwork: string;
  envLocal: string;
  envWarden: string;
  envDocker: string;
  secretEnvLocal: string;
  secretEnvWarden: string;
  secretEnvDocker: string;
  secretFilesLocal: string;
  secretFilesWarden: string;
  secretFilesDocker: string;
  remoteSecretPolicy: string;
  remoteEventMirror: string;
  remoteArtifactBytes: string;
  pinned: Record<string, boolean>;
}

type BadgeTone = "default" | "good" | "warn" | "bad" | "accent";
type MetricTone = "accent" | "good" | "warn" | "bad" | "muted";

const emptyBackendValues: ExecutionProfileBackendValues = {
  sshEnabled: false,
  sshTarget: "",
  sshWorkDir: "",
  sshIdentity: "",
  sshPort: "",
  sshStrictHostKey: "",
  k8sEnabled: false,
  k8sContext: "",
  k8sNamespace: "",
  k8sPod: "",
  k8sContainer: "",
  k8sWorkDir: "",
  modalEnabled: false,
  modalRef: "",
  modalImage: "",
  modalEnvironment: "",
  modalAddPython: "",
  modalWorkDir: "",
  daytonaEnabled: false,
  daytonaSandbox: "",
  daytonaWorkDir: "",
  dockerEnabled: false,
  dockerRuntime: "",
  dockerImage: "",
  dockerNetwork: "",
  envLocal: "",
  envWarden: "",
  envDocker: "",
  secretEnvLocal: "",
  secretEnvWarden: "",
  secretEnvDocker: "",
  secretFilesLocal: "",
  secretFilesWarden: "",
  secretFilesDocker: "",
  remoteSecretPolicy: "",
  remoteEventMirror: "",
  remoteArtifactBytes: "",
  pinned: {},
};

export function profileStatusTone(status?: string, degraded = false): BadgeTone {
  if (degraded) return "warn";
  switch ((status || "").toLowerCase()) {
    case "supported":
      return "good";
    case "degraded":
    case "partial":
      return "warn";
    case "planned":
      return "default";
    default:
      return "default";
  }
}

export function checkStatusTone(status?: string): BadgeTone {
  switch ((status || "").toLowerCase()) {
    case "ok":
      return "good";
    case "warning":
      return "warn";
    case "fail":
      return "bad";
    default:
      return "default";
  }
}

export function checksByProfileID(checks: ExecutionProfileCheck[] = []): Record<string, ExecutionProfileCheck[]> {
  const out: Record<string, ExecutionProfileCheck[]> = {};
  for (const check of checks) {
    const id = (check.profile_id || "").trim();
    if (!id) continue;
    out[id] = [...(out[id] || []), check];
  }
  return out;
}

export function executionProfileRollup(
  inventory: ExecutionProfileInventory | null,
  report: ExecutionProfileHealthReport | null,
) {
  const profiles = inventory?.profiles || [];
  const checks = report?.checks || [];
  return {
    total: inventory?.count ?? profiles.length,
    routed: inventory?.routed_count ?? profiles.filter((p) => p.routed).length,
    supported: inventory?.supported_count ?? profiles.filter((p) => p.status === "supported").length,
    degraded: inventory?.degraded_count ?? profiles.filter((p) => p.degraded).length,
    selectable: report?.routable_run_profiles?.length ?? 0,
    warnings: report?.warning_count ?? checks.filter((c) => c.status === "warning").length,
    failures: report?.fail_count ?? checks.filter((c) => c.status === "fail").length,
  };
}

export function executionProfilePolicyFromConfigValues(fields: ConfigValueEntry[] = []): ExecutionProfilePolicyValues {
  const byEnv = new Map(fields.map((field) => [field.env, field]));
  const allow = byEnv.get("AGEZT_EXEC_PROFILE_ALLOW");
  const deny = byEnv.get("AGEZT_EXEC_PROFILE_DENY");
  return {
    allow: allow?.value || "",
    deny: deny?.value || "",
    allowPinned: !!allow?.env_pinned,
    denyPinned: !!deny?.env_pinned,
  };
}

export function executionProfileBackendFromConfigValues(fields: ConfigValueEntry[] = []): ExecutionProfileBackendValues {
  const byEnv = new Map(fields.map((field) => [field.env, field]));
  const value = (env: string) => byEnv.get(env)?.value || "";
  const pinned: Record<string, boolean> = {};
  for (const field of fields) {
    if (field.env) pinned[field.env] = !!field.env_pinned;
  }
  return {
    sshEnabled: configTruthy(value("AGEZT_EXEC_SSH")),
    sshTarget: value("AGEZT_EXEC_SSH_TARGET"),
    sshWorkDir: value("AGEZT_EXEC_SSH_WORKDIR"),
    sshIdentity: value("AGEZT_EXEC_SSH_IDENTITY"),
    sshPort: value("AGEZT_EXEC_SSH_PORT"),
    sshStrictHostKey: value("AGEZT_EXEC_SSH_STRICT_HOST_KEY"),
    k8sEnabled: configTruthy(value("AGEZT_EXEC_K8S")),
    k8sContext: value("AGEZT_EXEC_K8S_CONTEXT"),
    k8sNamespace: value("AGEZT_EXEC_K8S_NAMESPACE"),
    k8sPod: value("AGEZT_EXEC_K8S_POD"),
    k8sContainer: value("AGEZT_EXEC_K8S_CONTAINER"),
    k8sWorkDir: value("AGEZT_EXEC_K8S_WORKDIR"),
    modalEnabled: configTruthy(value("AGEZT_EXEC_MODAL")),
    modalRef: value("AGEZT_EXEC_MODAL_REF"),
    modalImage: value("AGEZT_EXEC_MODAL_IMAGE"),
    modalEnvironment: value("AGEZT_EXEC_MODAL_ENVIRONMENT"),
    modalAddPython: value("AGEZT_EXEC_MODAL_ADD_PYTHON"),
    modalWorkDir: value("AGEZT_EXEC_MODAL_WORKDIR"),
    daytonaEnabled: configTruthy(value("AGEZT_EXEC_DAYTONA")),
    daytonaSandbox: value("AGEZT_EXEC_DAYTONA_SANDBOX"),
    daytonaWorkDir: value("AGEZT_EXEC_DAYTONA_WORKDIR"),
    dockerEnabled: configTruthy(value("AGEZT_WARDEN_DOCKER")),
    dockerRuntime: value("AGEZT_WARDEN_DOCKER_RUNTIME"),
    dockerImage: value("AGEZT_WARDEN_DOCKER_IMAGE"),
    dockerNetwork: value("AGEZT_WARDEN_DOCKER_NETWORK"),
    envLocal: value("AGEZT_EXEC_ENV_LOCAL"),
    envWarden: value("AGEZT_EXEC_ENV_WARDEN"),
    envDocker: value("AGEZT_EXEC_ENV_DOCKER"),
    secretEnvLocal: value("AGEZT_EXEC_SECRET_ENV_LOCAL"),
    secretEnvWarden: value("AGEZT_EXEC_SECRET_ENV_WARDEN"),
    secretEnvDocker: value("AGEZT_EXEC_SECRET_ENV_DOCKER"),
    secretFilesLocal: value("AGEZT_EXEC_SECRET_FILES_LOCAL"),
    secretFilesWarden: value("AGEZT_EXEC_SECRET_FILES_WARDEN"),
    secretFilesDocker: value("AGEZT_EXEC_SECRET_FILES_DOCKER"),
    remoteSecretPolicy: value("AGEZT_EXEC_REMOTE_SECRET_POLICY"),
    remoteEventMirror: value("AGEZT_REMOTE_EVENT_MIRROR"),
    remoteArtifactBytes: value("AGEZT_REMOTE_ARTIFACT_BYTES"),
    pinned,
  };
}

function configTruthy(value: string): boolean {
  return ["1", "true", "yes", "on"].includes(value.trim().toLowerCase());
}

function backendValuesEqual(a: ExecutionProfileBackendValues, b: ExecutionProfileBackendValues): boolean {
  return (
    a.sshEnabled === b.sshEnabled &&
    a.sshTarget === b.sshTarget &&
    a.sshWorkDir === b.sshWorkDir &&
    a.sshIdentity === b.sshIdentity &&
    a.sshPort === b.sshPort &&
    a.sshStrictHostKey === b.sshStrictHostKey &&
    a.k8sEnabled === b.k8sEnabled &&
    a.k8sContext === b.k8sContext &&
    a.k8sNamespace === b.k8sNamespace &&
    a.k8sPod === b.k8sPod &&
    a.k8sContainer === b.k8sContainer &&
    a.k8sWorkDir === b.k8sWorkDir &&
    a.modalEnabled === b.modalEnabled &&
    a.modalRef === b.modalRef &&
    a.modalImage === b.modalImage &&
    a.modalEnvironment === b.modalEnvironment &&
    a.modalAddPython === b.modalAddPython &&
    a.modalWorkDir === b.modalWorkDir &&
    a.daytonaEnabled === b.daytonaEnabled &&
    a.daytonaSandbox === b.daytonaSandbox &&
    a.daytonaWorkDir === b.daytonaWorkDir &&
    a.dockerEnabled === b.dockerEnabled &&
    a.dockerRuntime === b.dockerRuntime &&
    a.dockerImage === b.dockerImage &&
    a.dockerNetwork === b.dockerNetwork &&
    a.envLocal === b.envLocal &&
    a.envWarden === b.envWarden &&
    a.envDocker === b.envDocker &&
    a.secretEnvLocal === b.secretEnvLocal &&
    a.secretEnvWarden === b.secretEnvWarden &&
    a.secretEnvDocker === b.secretEnvDocker &&
    a.secretFilesLocal === b.secretFilesLocal &&
    a.secretFilesWarden === b.secretFilesWarden &&
    a.secretFilesDocker === b.secretFilesDocker &&
    a.remoteSecretPolicy === b.remoteSecretPolicy &&
    a.remoteEventMirror === b.remoteEventMirror &&
    a.remoteArtifactBytes === b.remoteArtifactBytes
  );
}

export function ExecutionProfiles() {
  const [inventory, setInventory] = useState<ExecutionProfileInventory | null>(null);
  const [report, setReport] = useState<ExecutionProfileHealthReport | null>(null);
  const [policy, setPolicy] = useState<ExecutionProfilePolicyValues>({
    allow: "",
    deny: "",
    allowPinned: false,
    denyPinned: false,
  });
  const [draftPolicy, setDraftPolicy] = useState({ allow: "", deny: "" });
  const [backend, setBackend] = useState<ExecutionProfileBackendValues>(emptyBackendValues);
  const [draftBackend, setDraftBackend] = useState<ExecutionProfileBackendValues>(emptyBackendValues);
  const [err, setErr] = useState<string | null>(null);
  const [loading, setLoading] = useState(false);
  const [savingPolicy, setSavingPolicy] = useState(false);
  const [savingBackend, setSavingBackend] = useState(false);

  async function reload() {
    setLoading(true);
    const [inv, health, configValues] = await Promise.allSettled([
      getJSON<ExecutionProfileInventory>("/api/execution_profiles"),
      getJSON<ExecutionProfileHealthReport>("/api/execution_profile_check"),
      getJSON<ConfigValuesResponse>("/api/config/values"),
    ]);
    if (inv.status === "fulfilled") setInventory(inv.value);
    if (health.status === "fulfilled") setReport(health.value);
    if (configValues.status === "fulfilled") {
      const nextPolicy = executionProfilePolicyFromConfigValues(configValues.value.fields || []);
      setPolicy((current) => {
        setDraftPolicy((draft) =>
          draft.allow === current.allow && draft.deny === current.deny ? { allow: nextPolicy.allow, deny: nextPolicy.deny } : draft,
        );
        return nextPolicy;
      });
      const nextBackend = executionProfileBackendFromConfigValues(configValues.value.fields || []);
      setBackend((current) => {
        setDraftBackend((draft) => (backendValuesEqual(draft, current) ? nextBackend : draft));
        return nextBackend;
      });
    }
    const errors = [inv, health, configValues]
      .filter((r): r is PromiseRejectedResult => r.status === "rejected")
      .map((r) => (r.reason as Error).message);
    setErr(errors.length ? errors.join("; ") : null);
    setLoading(false);
  }

  async function savePolicy() {
    setSavingPolicy(true);
    try {
      const allow = draftPolicy.allow.trim();
      const deny = draftPolicy.deny.trim();
      const writes: Promise<unknown>[] = [];
      if (!policy.allowPinned && allow !== policy.allow) {
        writes.push(postJSON("/api/config/set", { name: "AGEZT_EXEC_PROFILE_ALLOW", value: allow }));
      }
      if (!policy.denyPinned && deny !== policy.deny) {
        writes.push(postJSON("/api/config/set", { name: "AGEZT_EXEC_PROFILE_DENY", value: deny }));
      }
      await Promise.all(writes);
      await reload();
    } catch (e) {
      setErr((e as Error).message);
    } finally {
      setSavingPolicy(false);
    }
  }

  async function saveBackend() {
    setSavingBackend(true);
    try {
      const writes: Promise<unknown>[] = [];
      for (const [env, value] of backendConfigPayloads(draftBackend)) {
        if (backend.pinned[env]) continue;
        if (value !== backendConfigValue(backend, env)) {
          writes.push(postJSON("/api/config/set", { name: env, value }));
        }
      }
      await Promise.all(writes);
      await reload();
    } catch (e) {
      setErr((e as Error).message);
    } finally {
      setSavingBackend(false);
    }
  }

  useEffect(() => {
    reload();
    const id = setInterval(reload, 10000);
    return () => clearInterval(id);
  }, []);

  const checks = report?.checks || [];
  const byProfile = useMemo(() => checksByProfileID(checks), [checks]);
  const routable = new Set(report?.routable_run_profiles || []);
  const rollup = executionProfileRollup(inventory, report);
  const profiles = inventory?.profiles || [];
  const host = inventory?.host_os && inventory?.host_arch ? `${inventory.host_os}/${inventory.host_arch}` : undefined;

  return (
    <Page
      icon={Server}
      title="Execution Profiles"
      description={host}
      width="wide"
      actions={
        <Button variant="ghost" size="sm" onClick={reload} disabled={loading} title="Reload">
          <RefreshCw className={cn("size-3.5", loading && "animate-spin")} />
        </Button>
      }
    >
      {err && <ErrorText>{err}</ErrorText>}

      {!inventory || !report ? (
        <SkeletonList count={4} lines={2} />
      ) : (
        <>
          <section className="grid gap-2 md:grid-cols-6">
            <Metric label="Profiles" value={rollup.total} tone="muted" icon={Terminal} />
            <Metric label="Routed" value={rollup.routed} tone={rollup.routed > 0 ? "accent" : "muted"} icon={Route} />
            <Metric label="Selectable" value={rollup.selectable} tone={rollup.selectable > 0 ? "good" : "warn"} icon={CheckCircle2} />
            <Metric label="Supported" value={rollup.supported} tone="good" icon={Shield} />
            <Metric label="Warnings" value={rollup.warnings} tone={rollup.warnings > 0 ? "warn" : "muted"} icon={AlertTriangle} />
            <Metric label="Failures" value={rollup.failures} tone={rollup.failures > 0 ? "bad" : "muted"} icon={XOctagon} />
          </section>

          {/* Policy + backend editors are power-user configuration — folded away
              by default so the calm view leads with the rollup + inventory. */}
          <Advanced label="Run policy & backend configuration">
            <div className="space-y-3">
              <ExecutionPolicyPanel
                policy={policy}
                draft={draftPolicy}
                onDraft={setDraftPolicy}
                saving={savingPolicy}
                onSave={savePolicy}
                selectable={report.routable_run_profiles || []}
              />

              <ExecutionBackendPanel
                backend={backend}
                draft={draftBackend}
                onDraft={setDraftBackend}
                saving={savingBackend}
                onSave={saveBackend}
              />
            </div>
          </Advanced>

          <div className="grid min-h-[22rem] flex-1 gap-3 xl:grid-cols-[minmax(0,1.45fr)_minmax(320px,0.55fr)]">
            <section className="min-h-0 overflow-auto rounded-lg border border-border bg-card/75">
              <div className="sticky top-0 z-10 flex items-center gap-2 border-b border-border bg-card/95 px-3 py-2 backdrop-blur">
                <Terminal className="size-4 text-accent" />
                <div className="min-w-0 flex-1 text-xs font-semibold uppercase tracking-normal text-muted">Inventory</div>
                <Badge>{profiles.length}</Badge>
              </div>
              {profiles.length === 0 ? (
                <div className="p-3">
                  <Muted>no execution profiles</Muted>
                </div>
              ) : (
                <ul className="divide-y divide-border/70">
                  {profiles.map((profile) => (
                    <ProfileRow
                      key={profile.id || profile.name}
                      profile={profile}
                      checks={byProfile[profile.id || ""] || []}
                      selectable={routable.has(profile.id || "")}
                    />
                  ))}
                </ul>
              )}
            </section>

            <section className="min-h-0 overflow-auto rounded-lg border border-border bg-card/75">
              <div className="sticky top-0 z-10 flex items-center gap-2 border-b border-border bg-card/95 px-3 py-2 backdrop-blur">
                <Shield className="size-4 text-accent" />
                <div className="min-w-0 flex-1 text-xs font-semibold uppercase tracking-normal text-muted">Health Checks</div>
                <Badge variant={rollup.failures > 0 ? "bad" : rollup.warnings > 0 ? "warn" : "good"}>
                  {rollup.failures > 0 ? `${rollup.failures} fail` : rollup.warnings > 0 ? `${rollup.warnings} warn` : "ok"}
                </Badge>
              </div>
              {checks.length === 0 ? (
                <div className="p-3">
                  <Muted>no health checks</Muted>
                </div>
              ) : (
                (() => {
                  // Lead with the problems; fold the "everything's fine" checks
                  // under a single green line so the panel isn't a wall of oks.
                  const probs = checks.filter((c) => checkStatusTone(c.status) !== "good");
                  const oks = checks.filter((c) => checkStatusTone(c.status) === "good");
                  return (
                    <div className="space-y-2 p-2">
                      {probs.map((check) => (
                        <HealthCheckLine key={check.id || `${check.profile_id}-${check.title}`} check={check} />
                      ))}
                      {oks.length > 0 && (
                        <Disclosure
                          summary={
                            <span className="inline-flex items-center gap-1.5 text-xs font-medium text-good">
                              <CheckCircle2 className="size-3.5" />
                              {oks.length} passing
                            </span>
                          }
                        >
                          <div className="space-y-2 pt-1">
                            {oks.map((check) => (
                              <HealthCheckLine key={check.id || `${check.profile_id}-${check.title}`} check={check} />
                            ))}
                          </div>
                        </Disclosure>
                      )}
                    </div>
                  );
                })()
              )}
            </section>
          </div>
        </>
      )}
    </Page>
  );
}

// Shared tone → colour classes so metrics, chips and status accents read as one
// colour language across the page (green=ok, amber=warn, red=bad, blue=routed).
const TONE: Record<MetricTone, { text: string; border: string; bg: string }> = {
  good: { text: "text-good", border: "border-good/40", bg: "bg-good/10" },
  warn: { text: "text-warn", border: "border-warn/40", bg: "bg-warn/10" },
  bad: { text: "text-bad", border: "border-bad/40", bg: "bg-bad/10" },
  accent: { text: "text-accent", border: "border-accent/40", bg: "bg-accent/10" },
  muted: { text: "text-foreground", border: "border-border", bg: "bg-panel" },
};

function Metric({
  label,
  value,
  tone,
  icon: Icon,
}: {
  label: string;
  value: number;
  tone: MetricTone;
  icon: LucideIcon;
}) {
  const t = TONE[tone];
  return (
    <div className={cn("flex items-center gap-2.5 rounded-lg border bg-card/80 px-3 py-2", t.border)}>
      <span className={cn("grid size-9 shrink-0 place-items-center rounded-lg", t.bg, t.text)}>
        <Icon className="size-5" />
      </span>
      <div className="min-w-0">
        <div className={cn("text-2xl font-semibold leading-none tabular-nums", t.text)}>{value}</div>
        <div className="mt-1 truncate text-[11px] font-semibold uppercase tracking-normal text-muted">{label}</div>
      </div>
    </div>
  );
}

function ExecutionPolicyPanel({
  policy,
  draft,
  onDraft,
  saving,
  onSave,
  selectable,
}: {
  policy: ExecutionProfilePolicyValues;
  draft: { allow: string; deny: string };
  onDraft: (next: { allow: string; deny: string }) => void;
  saving: boolean;
  onSave: () => void;
  selectable: string[];
}) {
  const dirty = draft.allow.trim() !== policy.allow || draft.deny.trim() !== policy.deny;
  const locked = policy.allowPinned && policy.denyPinned;
  return (
    <section className="rounded-lg border border-border bg-card/80 px-3 py-2">
      <div className="flex flex-wrap items-center gap-2">
        <Shield className="size-4 text-accent" />
        <div className="min-w-0 flex-1 text-xs font-semibold uppercase tracking-normal text-muted">Run Policy</div>
        {selectable.length > 0 ? <Badge variant="good">{selectable.join(", ")}</Badge> : <Badge variant="warn">none selectable</Badge>}
        <Badge variant={locked ? "default" : "accent"}>{locked ? "external" : "live"}</Badge>
      </div>
      <div className="mt-2 grid gap-2 md:grid-cols-[minmax(0,1fr)_minmax(0,1fr)_auto]">
        <PolicyInput
          label="Allow"
          value={draft.allow}
          pinned={policy.allowPinned}
          onChange={(allow) => onDraft({ ...draft, allow })}
        />
        <PolicyInput
          label="Deny"
          value={draft.deny}
          pinned={policy.denyPinned}
          onChange={(deny) => onDraft({ ...draft, deny })}
        />
        <Button
          size="sm"
          className="self-end"
          onClick={onSave}
          disabled={!dirty || saving || locked}
          aria-label="Save run policy"
          title="Save execution profile policy"
        >
          {saving ? <RefreshCw className="size-3.5 animate-spin" /> : <Save className="size-3.5" />}
          Save
        </Button>
      </div>
    </section>
  );
}

function PolicyInput({
  label,
  value,
  pinned,
  onChange,
}: {
  label: string;
  value: string;
  pinned: boolean;
  onChange: (value: string) => void;
}) {
  return (
    <label className="min-w-0">
      <span className="mb-1 flex items-center gap-1.5 text-[11px] font-semibold uppercase tracking-normal text-muted">
        {label}
        {pinned && <Badge>external</Badge>}
      </span>
      <Input
        value={value}
        disabled={pinned}
        onChange={(e) => onChange(e.target.value)}
        placeholder="local, warden, docker, ssh, k8s, modal, daytona"
        className="h-8 font-mono text-xs"
      />
    </label>
  );
}

function backendConfigPayloads(values: ExecutionProfileBackendValues): Array<[string, string]> {
  return [
    ["AGEZT_EXEC_SSH", values.sshEnabled ? "on" : ""],
    ["AGEZT_EXEC_SSH_TARGET", values.sshTarget.trim()],
    ["AGEZT_EXEC_SSH_WORKDIR", values.sshWorkDir.trim()],
    ["AGEZT_EXEC_SSH_IDENTITY", values.sshIdentity.trim()],
    ["AGEZT_EXEC_SSH_PORT", values.sshPort.trim()],
    ["AGEZT_EXEC_SSH_STRICT_HOST_KEY", values.sshStrictHostKey.trim()],
    ["AGEZT_EXEC_K8S", values.k8sEnabled ? "on" : ""],
    ["AGEZT_EXEC_K8S_CONTEXT", values.k8sContext.trim()],
    ["AGEZT_EXEC_K8S_NAMESPACE", values.k8sNamespace.trim()],
    ["AGEZT_EXEC_K8S_POD", values.k8sPod.trim()],
    ["AGEZT_EXEC_K8S_CONTAINER", values.k8sContainer.trim()],
    ["AGEZT_EXEC_K8S_WORKDIR", values.k8sWorkDir.trim()],
    ["AGEZT_EXEC_MODAL", values.modalEnabled ? "on" : ""],
    ["AGEZT_EXEC_MODAL_REF", values.modalRef.trim()],
    ["AGEZT_EXEC_MODAL_IMAGE", values.modalImage.trim()],
    ["AGEZT_EXEC_MODAL_ENVIRONMENT", values.modalEnvironment.trim()],
    ["AGEZT_EXEC_MODAL_ADD_PYTHON", values.modalAddPython.trim()],
    ["AGEZT_EXEC_MODAL_WORKDIR", values.modalWorkDir.trim()],
    ["AGEZT_EXEC_DAYTONA", values.daytonaEnabled ? "on" : ""],
    ["AGEZT_EXEC_DAYTONA_SANDBOX", values.daytonaSandbox.trim()],
    ["AGEZT_EXEC_DAYTONA_WORKDIR", values.daytonaWorkDir.trim()],
    ["AGEZT_WARDEN_DOCKER", values.dockerEnabled ? "on" : ""],
    ["AGEZT_WARDEN_DOCKER_RUNTIME", values.dockerRuntime.trim()],
    ["AGEZT_WARDEN_DOCKER_IMAGE", values.dockerImage.trim()],
    ["AGEZT_WARDEN_DOCKER_NETWORK", values.dockerNetwork.trim()],
    ["AGEZT_EXEC_ENV_LOCAL", values.envLocal.trim()],
    ["AGEZT_EXEC_ENV_WARDEN", values.envWarden.trim()],
    ["AGEZT_EXEC_ENV_DOCKER", values.envDocker.trim()],
    ["AGEZT_EXEC_SECRET_ENV_LOCAL", values.secretEnvLocal.trim()],
    ["AGEZT_EXEC_SECRET_ENV_WARDEN", values.secretEnvWarden.trim()],
    ["AGEZT_EXEC_SECRET_ENV_DOCKER", values.secretEnvDocker.trim()],
    ["AGEZT_EXEC_SECRET_FILES_LOCAL", values.secretFilesLocal.trim()],
    ["AGEZT_EXEC_SECRET_FILES_WARDEN", values.secretFilesWarden.trim()],
    ["AGEZT_EXEC_SECRET_FILES_DOCKER", values.secretFilesDocker.trim()],
    ["AGEZT_EXEC_REMOTE_SECRET_POLICY", values.remoteSecretPolicy.trim()],
    ["AGEZT_REMOTE_EVENT_MIRROR", values.remoteEventMirror.trim()],
    ["AGEZT_REMOTE_ARTIFACT_BYTES", values.remoteArtifactBytes.trim()],
  ];
}

function backendConfigValue(values: ExecutionProfileBackendValues, env: string): string {
  return backendConfigPayloads(values).find(([name]) => name === env)?.[1] || "";
}

function ExecutionBackendPanel({
  backend,
  draft,
  onDraft,
  saving,
  onSave,
}: {
  backend: ExecutionProfileBackendValues;
  draft: ExecutionProfileBackendValues;
  onDraft: (next: ExecutionProfileBackendValues) => void;
  saving: boolean;
  onSave: () => void;
}) {
  const dirty = !backendValuesEqual(draft, backend);
  const pinned = (env: string) => !!backend.pinned[env];
  return (
    <section className="rounded-lg border border-border bg-card/80 px-3 py-2">
      <div className="flex flex-wrap items-center gap-2">
        <Wrench className="size-4 text-accent" />
        <div className="min-w-0 flex-1 text-xs font-semibold uppercase tracking-normal text-muted">Backend Config</div>
        <Badge variant={draft.sshEnabled && draft.sshTarget.trim() ? "good" : "default"}>ssh live</Badge>
        <Badge variant={draft.k8sEnabled && draft.k8sPod.trim() ? "good" : "default"}>k8s live</Badge>
        <Badge variant={draft.modalEnabled ? "good" : "default"}>modal live</Badge>
        <Badge variant={draft.daytonaEnabled && draft.daytonaSandbox.trim() ? "good" : "default"}>daytona live</Badge>
        <Badge variant={draft.dockerEnabled ? "warn" : "default"}>docker restart</Badge>
        <Badge variant="accent">env live</Badge>
        <Badge variant={draft.remoteSecretPolicy === "metadata" ? "warn" : "good"}>remote secrets {draft.remoteSecretPolicy || "deny"}</Badge>
      </div>
      <div className="mt-2 grid gap-3 xl:grid-cols-4">
        <div className="rounded-md border border-border/65 bg-panel/35 p-2">
          <div className="mb-2 flex items-center gap-2">
            <Terminal className="size-3.5 text-accent" />
            <span className="text-xs font-semibold text-foreground">SSH Remote</span>
            <Badge variant="accent">live</Badge>
          </div>
          <div className="grid gap-2 md:grid-cols-2">
            <ToggleField
              label="Enable SSH"
              checked={draft.sshEnabled}
              pinned={pinned("AGEZT_EXEC_SSH")}
              onChange={(sshEnabled) => onDraft({ ...draft, sshEnabled })}
            />
            <BackendInput
              label="SSH Target"
              value={draft.sshTarget}
              pinned={pinned("AGEZT_EXEC_SSH_TARGET")}
              onChange={(sshTarget) => onDraft({ ...draft, sshTarget })}
              placeholder="deploy@example.com"
            />
            <BackendInput
              label="SSH Workdir"
              value={draft.sshWorkDir}
              pinned={pinned("AGEZT_EXEC_SSH_WORKDIR")}
              onChange={(sshWorkDir) => onDraft({ ...draft, sshWorkDir })}
              placeholder="/srv/app"
            />
            <BackendInput
              label="SSH Identity"
              value={draft.sshIdentity}
              pinned={pinned("AGEZT_EXEC_SSH_IDENTITY")}
              onChange={(sshIdentity) => onDraft({ ...draft, sshIdentity })}
              placeholder="~/.ssh/id_ed25519"
            />
            <BackendInput
              label="SSH Port"
              value={draft.sshPort}
              pinned={pinned("AGEZT_EXEC_SSH_PORT")}
              onChange={(sshPort) => onDraft({ ...draft, sshPort })}
              placeholder="22"
            />
            <BackendInput
              label="Strict Host Key"
              value={draft.sshStrictHostKey}
              pinned={pinned("AGEZT_EXEC_SSH_STRICT_HOST_KEY")}
              onChange={(sshStrictHostKey) => onDraft({ ...draft, sshStrictHostKey })}
              placeholder="accept-new"
            />
          </div>
        </div>

        <div className="rounded-md border border-border/65 bg-panel/35 p-2">
          <div className="mb-2 flex items-center gap-2">
            <Server className="size-3.5 text-accent" />
            <span className="text-xs font-semibold text-foreground">Kubernetes Pod</span>
            <Badge variant="accent">live</Badge>
          </div>
          <div className="grid gap-2 md:grid-cols-2">
            <ToggleField
              label="Enable K8s"
              checked={draft.k8sEnabled}
              pinned={pinned("AGEZT_EXEC_K8S")}
              onChange={(k8sEnabled) => onDraft({ ...draft, k8sEnabled })}
            />
            <BackendInput
              label="K8s Pod"
              value={draft.k8sPod}
              pinned={pinned("AGEZT_EXEC_K8S_POD")}
              onChange={(k8sPod) => onDraft({ ...draft, k8sPod })}
              placeholder="runner-0"
            />
            <BackendInput
              label="K8s Context"
              value={draft.k8sContext}
              pinned={pinned("AGEZT_EXEC_K8S_CONTEXT")}
              onChange={(k8sContext) => onDraft({ ...draft, k8sContext })}
              placeholder="prod"
            />
            <BackendInput
              label="K8s Namespace"
              value={draft.k8sNamespace}
              pinned={pinned("AGEZT_EXEC_K8S_NAMESPACE")}
              onChange={(k8sNamespace) => onDraft({ ...draft, k8sNamespace })}
              placeholder="agents"
            />
            <BackendInput
              label="K8s Container"
              value={draft.k8sContainer}
              pinned={pinned("AGEZT_EXEC_K8S_CONTAINER")}
              onChange={(k8sContainer) => onDraft({ ...draft, k8sContainer })}
              placeholder="worker"
            />
            <BackendInput
              label="K8s Workdir"
              value={draft.k8sWorkDir}
              pinned={pinned("AGEZT_EXEC_K8S_WORKDIR")}
              onChange={(k8sWorkDir) => onDraft({ ...draft, k8sWorkDir })}
              placeholder="/workspace"
            />
          </div>
        </div>

        <div className="rounded-md border border-border/65 bg-panel/35 p-2">
          <div className="mb-2 flex items-center gap-2">
            <Globe className="size-3.5 text-accent" />
            <span className="text-xs font-semibold text-foreground">Modal Shell</span>
            <Badge variant="accent">live</Badge>
          </div>
          <div className="grid gap-2 md:grid-cols-2">
            <ToggleField
              label="Enable Modal"
              checked={draft.modalEnabled}
              pinned={pinned("AGEZT_EXEC_MODAL")}
              onChange={(modalEnabled) => onDraft({ ...draft, modalEnabled })}
            />
            <BackendInput
              label="Modal Ref"
              value={draft.modalRef}
              pinned={pinned("AGEZT_EXEC_MODAL_REF")}
              onChange={(modalRef) => onDraft({ ...draft, modalRef })}
              placeholder="app.py::main"
            />
            <BackendInput
              label="Modal Image"
              value={draft.modalImage}
              pinned={pinned("AGEZT_EXEC_MODAL_IMAGE")}
              onChange={(modalImage) => onDraft({ ...draft, modalImage })}
              placeholder="python:3.12"
            />
            <BackendInput
              label="Modal Env"
              value={draft.modalEnvironment}
              pinned={pinned("AGEZT_EXEC_MODAL_ENVIRONMENT")}
              onChange={(modalEnvironment) => onDraft({ ...draft, modalEnvironment })}
              placeholder="prod"
            />
            <BackendInput
              label="Add Python"
              value={draft.modalAddPython}
              pinned={pinned("AGEZT_EXEC_MODAL_ADD_PYTHON")}
              onChange={(modalAddPython) => onDraft({ ...draft, modalAddPython })}
              placeholder="3.12"
            />
            <BackendInput
              label="Modal Workdir"
              value={draft.modalWorkDir}
              pinned={pinned("AGEZT_EXEC_MODAL_WORKDIR")}
              onChange={(modalWorkDir) => onDraft({ ...draft, modalWorkDir })}
              placeholder="/workspace"
            />
          </div>
        </div>

        <div className="rounded-md border border-border/65 bg-panel/35 p-2">
          <div className="mb-2 flex items-center gap-2">
            <Server className="size-3.5 text-accent" />
            <span className="text-xs font-semibold text-foreground">Daytona Sandbox</span>
            <Badge variant="accent">live</Badge>
          </div>
          <div className="grid gap-2 md:grid-cols-2">
            <ToggleField
              label="Enable Daytona"
              checked={draft.daytonaEnabled}
              pinned={pinned("AGEZT_EXEC_DAYTONA")}
              onChange={(daytonaEnabled) => onDraft({ ...draft, daytonaEnabled })}
            />
            <BackendInput
              label="Daytona Sandbox"
              value={draft.daytonaSandbox}
              pinned={pinned("AGEZT_EXEC_DAYTONA_SANDBOX")}
              onChange={(daytonaSandbox) => onDraft({ ...draft, daytonaSandbox })}
              placeholder="sandbox-1"
            />
            <BackendInput
              label="Daytona Workdir"
              value={draft.daytonaWorkDir}
              pinned={pinned("AGEZT_EXEC_DAYTONA_WORKDIR")}
              onChange={(daytonaWorkDir) => onDraft({ ...draft, daytonaWorkDir })}
              placeholder="/workspace"
            />
          </div>
        </div>

        <div className="rounded-md border border-border/65 bg-panel/35 p-2">
          <div className="mb-2 flex items-center gap-2">
            <Boxes className="size-3.5 text-accent" />
            <span className="text-xs font-semibold text-foreground">Docker/OCI</span>
            <Badge variant="warn">restart</Badge>
          </div>
          <div className="grid gap-2 md:grid-cols-2">
            <ToggleField
              label="Enable Docker"
              checked={draft.dockerEnabled}
              pinned={pinned("AGEZT_WARDEN_DOCKER")}
              onChange={(dockerEnabled) => onDraft({ ...draft, dockerEnabled })}
            />
            <BackendInput
              label="Docker Runtime"
              value={draft.dockerRuntime}
              pinned={pinned("AGEZT_WARDEN_DOCKER_RUNTIME")}
              onChange={(dockerRuntime) => onDraft({ ...draft, dockerRuntime })}
              placeholder="docker"
            />
            <BackendInput
              label="Docker Image"
              value={draft.dockerImage}
              pinned={pinned("AGEZT_WARDEN_DOCKER_IMAGE")}
              onChange={(dockerImage) => onDraft({ ...draft, dockerImage })}
              placeholder="python:3.12-slim"
            />
            <BackendInput
              label="Docker Network"
              value={draft.dockerNetwork}
              pinned={pinned("AGEZT_WARDEN_DOCKER_NETWORK")}
              onChange={(dockerNetwork) => onDraft({ ...draft, dockerNetwork })}
              placeholder="none"
            />
          </div>
        </div>

        <div className="rounded-md border border-border/65 bg-panel/35 p-2">
          <div className="mb-2 flex items-center gap-2">
            <Globe className="size-3.5 text-accent" />
            <span className="text-xs font-semibold text-foreground">Env Passthrough</span>
            <Badge variant="accent">live</Badge>
          </div>
          <div className="grid gap-2 md:grid-cols-2">
            <BackendInput
              label="Local Env"
              value={draft.envLocal}
              pinned={pinned("AGEZT_EXEC_ENV_LOCAL")}
              onChange={(envLocal) => onDraft({ ...draft, envLocal })}
              placeholder="FOO, BAR"
            />
            <BackendInput
              label="Local Secret Env"
              value={draft.secretEnvLocal}
              pinned={pinned("AGEZT_EXEC_SECRET_ENV_LOCAL")}
              onChange={(secretEnvLocal) => onDraft({ ...draft, secretEnvLocal })}
              placeholder="OPENAI_API_KEY"
            />
            <BackendInput
              label="Warden Env"
              value={draft.envWarden}
              pinned={pinned("AGEZT_EXEC_ENV_WARDEN")}
              onChange={(envWarden) => onDraft({ ...draft, envWarden })}
              placeholder="FOO, BAR"
            />
            <BackendInput
              label="Warden Secret Env"
              value={draft.secretEnvWarden}
              pinned={pinned("AGEZT_EXEC_SECRET_ENV_WARDEN")}
              onChange={(secretEnvWarden) => onDraft({ ...draft, secretEnvWarden })}
              placeholder="OPENAI_API_KEY"
            />
            <BackendInput
              label="Docker Env"
              value={draft.envDocker}
              pinned={pinned("AGEZT_EXEC_ENV_DOCKER")}
              onChange={(envDocker) => onDraft({ ...draft, envDocker })}
              placeholder="FOO, BAR"
            />
            <BackendInput
              label="Docker Secret Env"
              value={draft.secretEnvDocker}
              pinned={pinned("AGEZT_EXEC_SECRET_ENV_DOCKER")}
              onChange={(secretEnvDocker) => onDraft({ ...draft, secretEnvDocker })}
              placeholder="OPENAI_API_KEY"
            />
            <BackendInput
              label="Local Secret Files"
              value={draft.secretFilesLocal}
              pinned={pinned("AGEZT_EXEC_SECRET_FILES_LOCAL")}
              onChange={(secretFilesLocal) => onDraft({ ...draft, secretFilesLocal })}
              placeholder="OPENAI_API_KEY:openai.key"
            />
            <BackendInput
              label="Warden Secret Files"
              value={draft.secretFilesWarden}
              pinned={pinned("AGEZT_EXEC_SECRET_FILES_WARDEN")}
              onChange={(secretFilesWarden) => onDraft({ ...draft, secretFilesWarden })}
              placeholder="OPENAI_API_KEY:openai.key"
            />
            <BackendInput
              label="Docker Secret Files"
              value={draft.secretFilesDocker}
              pinned={pinned("AGEZT_EXEC_SECRET_FILES_DOCKER")}
              onChange={(secretFilesDocker) => onDraft({ ...draft, secretFilesDocker })}
              placeholder="OPENAI_API_KEY:openai.key"
            />
          </div>
        </div>

        <div className="rounded-md border border-border/65 bg-panel/35 p-2">
          <div className="mb-2 flex items-center gap-2">
            <Server className="size-3.5 text-accent" />
            <span className="text-xs font-semibold text-foreground">Remote AGEZT</span>
            <Badge variant="accent">live</Badge>
          </div>
          <div className="grid gap-2">
            <BackendSelect
              label="Remote Secret Policy"
              value={draft.remoteSecretPolicy}
              pinned={pinned("AGEZT_EXEC_REMOTE_SECRET_POLICY")}
              options={["", "deny", "metadata"]}
              onChange={(remoteSecretPolicy) => onDraft({ ...draft, remoteSecretPolicy })}
            />
            <BackendSelect
              label="Remote Event Mirror"
              value={draft.remoteEventMirror}
              pinned={pinned("AGEZT_REMOTE_EVENT_MIRROR")}
              options={["", "off", "metadata", "redacted"]}
              onChange={(remoteEventMirror) => onDraft({ ...draft, remoteEventMirror })}
            />
            <BackendSelect
              label="Remote Artifact Bytes"
              value={draft.remoteArtifactBytes}
              pinned={pinned("AGEZT_REMOTE_ARTIFACT_BYTES")}
              options={["", "off", "allow"]}
              onChange={(remoteArtifactBytes) => onDraft({ ...draft, remoteArtifactBytes })}
            />
          </div>
        </div>
      </div>
      <div className="mt-2 flex justify-end">
        <Button
          size="sm"
          onClick={onSave}
          disabled={!dirty || saving}
          aria-label="Save backend config"
          title="Save execution profile backend config"
        >
          {saving ? <RefreshCw className="size-3.5 animate-spin" /> : <Save className="size-3.5" />}
          Save
        </Button>
      </div>
    </section>
  );
}

function ToggleField({
  label,
  checked,
  pinned,
  onChange,
}: {
  label: string;
  checked: boolean;
  pinned: boolean;
  onChange: (checked: boolean) => void;
}) {
  return (
    <label className="flex min-h-8 items-center gap-2 rounded-md border border-border/70 bg-background/40 px-2 text-xs">
      <input
        type="checkbox"
        checked={checked}
        disabled={pinned}
        onChange={(e) => onChange(e.target.checked)}
        className="size-3.5 accent-current"
      />
      <span className="min-w-0 flex-1 truncate">{label}</span>
      {pinned && <Badge>external</Badge>}
    </label>
  );
}

function BackendInput({
  label,
  value,
  pinned,
  onChange,
  placeholder,
}: {
  label: string;
  value: string;
  pinned: boolean;
  onChange: (value: string) => void;
  placeholder: string;
}) {
  return (
    <label className="min-w-0">
      <span className="mb-1 flex items-center gap-1.5 text-[11px] font-semibold uppercase tracking-normal text-muted">
        {label}
        {pinned && <Badge>external</Badge>}
      </span>
      <Input
        value={value}
        disabled={pinned}
        onChange={(e) => onChange(e.target.value)}
        placeholder={placeholder}
        className="h-8 font-mono text-xs"
      />
    </label>
  );
}

function BackendSelect({
  label,
  value,
  pinned,
  options,
  onChange,
}: {
  label: string;
  value: string;
  pinned: boolean;
  options: string[];
  onChange: (value: string) => void;
}) {
  const allOptions = options.includes(value) ? options : [...options, value];
  return (
    <label className="min-w-0">
      <span className="mb-1 flex items-center gap-1.5 text-[11px] font-semibold uppercase tracking-normal text-muted">
        {label}
        {pinned && <Badge>external</Badge>}
      </span>
      <select
        value={value}
        disabled={pinned}
        onChange={(e) => onChange(e.target.value)}
        className="h-8 w-full rounded-md border border-input bg-background px-2 font-mono text-xs text-foreground"
      >
        {allOptions.map((option) => (
          <option key={option || "blank"} value={option}>
            {option || "default (deny)"}
          </option>
        ))}
      </select>
    </label>
  );
}

function ProfileRow({
  profile,
  checks,
  selectable,
}: {
  profile: ExecutionProfile;
  checks: ExecutionProfileCheck[];
  selectable: boolean;
}) {
  const id = profile.id || "profile";
  const statusTone = profileStatusTone(profile.status, profile.degraded);
  const isolationChanged =
    (profile.requested_isolation || "") !== "" &&
    (profile.effective_isolation || "") !== "" &&
    profile.requested_isolation !== profile.effective_isolation;

  const rowTone = TONE[statusTone as MetricTone] ?? TONE.muted;
  const StatusIcon =
    statusTone === "good" ? CheckCircle2 : statusTone === "warn" ? AlertTriangle : statusTone === "bad" ? XOctagon : Terminal;

  return (
    <li className={cn("border-l-2 p-3", rowTone.border)}>
      <div className="flex min-w-0 flex-wrap items-start gap-2">
        <span className={cn("grid size-8 shrink-0 place-items-center rounded-lg", rowTone.bg, rowTone.text)}>
          <StatusIcon className="size-4" />
        </span>
        <div className="min-w-0 flex-1">
          <div className="flex min-w-0 flex-wrap items-center gap-1.5">
            <h3 className="truncate text-sm font-semibold">{profile.name || id}</h3>
            <span className="font-mono text-[11px] text-muted">{id}</span>
            <Badge variant={statusTone}>{profile.status || "unknown"}</Badge>
            <Badge variant={profile.routed ? "accent" : "default"}>
              <Route className="mr-1 size-3" />
              {profile.routed ? "routed" : "not routed"}
            </Badge>
            {selectable && (
              <Badge variant="good">
                <CheckCircle2 className="mr-1 size-3" />
                selectable
              </Badge>
            )}
            {profile.degraded && (
              <Badge variant="warn">
                <AlertTriangle className="mr-1 size-3" />
                degraded
              </Badge>
            )}
          </div>
          {profile.summary && <p className="mt-1 text-xs leading-snug text-muted">{profile.summary}</p>}
        </div>
      </div>

      {/* The full 8-field spec is a wall of text across ~10 profiles; fold it per
          row so the inventory reads as a scannable list (name + status + warnings)
          and the operator expands the one profile they care about. */}
      <Disclosure
        className="mt-2"
        summary={<span className="text-[11px] font-semibold uppercase tracking-normal text-muted">Details</span>}
      >
        <div className="grid gap-2 lg:grid-cols-2">
          <Fact icon={Shield} label="Isolation">
            <span className={cn("font-mono", isolationChanged && "text-warn")}>{profile.requested_isolation || "unknown"}</span>
            <span className="mx-1 text-muted">-&gt;</span>
            <span className={cn("font-mono", profile.degraded && "text-warn")}>{profile.effective_isolation || "unknown"}</span>
          </Fact>
          <Fact icon={Wrench} label="Tools">
            <ChipList items={profile.tools || []} empty="none" />
          </Fact>
          <Fact icon={Boxes} label="Backends">
            <ChipList items={profile.backends || []} empty="none" />
          </Fact>
          <Fact icon={Timer} label="Limits">
            <ChipList items={profile.limits || []} empty="tool defaults" />
          </Fact>
          <Fact icon={HardDrive} label="Filesystem">
            {profile.filesystem || "not declared"}
          </Fact>
          <Fact icon={Globe} label="Network">
            {profile.network || "not declared"}
          </Fact>
          <Fact icon={Lock} label="Secrets">
            {profile.secrets || "not declared"}
          </Fact>
          <Fact icon={Terminal} label="Cleanup">
            {profile.cleanup || "not declared"}
          </Fact>
        </div>
      </Disclosure>

      {(profile.degrade_reason || profile.policy_capability || (profile.notes || []).length > 0) && (
        <div className="mt-2 flex flex-wrap gap-1.5 text-xs">
          {profile.policy_capability && <Badge variant="accent">{profile.policy_capability}</Badge>}
          {profile.secret_policy && (
            <Badge variant={profile.secret_policy.valid === false ? "warn" : "accent"}>
              secret policy: {profile.secret_policy.mode || "deny"}
            </Badge>
          )}
          {profile.secret_policy && !profile.secret_policy.values_forwarded && <Badge>secret values denied</Badge>}
          {profile.degrade_reason && <Badge variant="warn">{profile.degrade_reason}</Badge>}
          {(profile.notes || []).map((note) => (
            <Badge key={note}>{note}</Badge>
          ))}
        </div>
      )}

      {checks.length > 0 && (
        <div className="mt-2 space-y-1.5">
          {checks.map((check) => (
            <HealthCheckLine key={check.id || check.title} check={check} compact />
          ))}
        </div>
      )}
    </li>
  );
}

function Fact({ icon: Icon, label, children }: { icon: typeof Shield; label: string; children: React.ReactNode }) {
  return (
    <div className="min-w-0 rounded-md border border-border/65 bg-panel/35 px-2.5 py-2 text-xs">
      <div className="mb-1 flex items-center gap-1.5 font-semibold uppercase tracking-normal text-muted">
        <Icon className="size-3" />
        {label}
      </div>
      <div className="min-w-0 break-words text-foreground/85">{children}</div>
    </div>
  );
}

function ChipList({ items, empty }: { items: string[]; empty: string }) {
  if (items.length === 0) return <span className="text-muted">{empty}</span>;
  return (
    <span className="flex flex-wrap gap-1">
      {items.map((item) => (
        <span key={item} className="rounded border border-border bg-background/60 px-1.5 py-0.5 font-mono text-[11px] text-foreground/85">
          {item}
        </span>
      ))}
    </span>
  );
}

function HealthCheckLine({ check, compact = false }: { check: ExecutionProfileCheck; compact?: boolean }) {
  const tone = checkStatusTone(check.status);
  const Icon = tone === "good" ? CheckCircle2 : tone === "bad" ? XOctagon : AlertTriangle;
  return (
    <div
      className={cn(
        "rounded-md border px-2.5 py-2 text-xs",
        tone === "good"
          ? "border-good/25 bg-good/5"
          : tone === "bad"
            ? "border-bad/25 bg-bad/5"
            : tone === "warn"
              ? "border-warn/25 bg-warn/5"
              : "border-border bg-panel/35",
      )}
    >
      <div className="flex min-w-0 items-start gap-2">
        <Icon
          className={cn(
            "mt-0.5 size-3.5 shrink-0",
            tone === "good" ? "text-good" : tone === "bad" ? "text-bad" : tone === "warn" ? "text-warn" : "text-muted",
          )}
        />
        <div className="min-w-0 flex-1">
          <div className="flex min-w-0 flex-wrap items-center gap-1.5">
            <span className="truncate font-semibold">{check.title || check.profile_id || "check"}</span>
            <Badge variant={tone}>{check.status || "unknown"}</Badge>
            {check.backend && <Badge>{check.backend}</Badge>}
            {check.routed && <Badge variant="accent">routed</Badge>}
            {check.backend_available && <Badge variant="good">backend ready</Badge>}
          </div>
          {check.detail && <p className={cn("mt-1 break-words text-muted", compact && "line-clamp-2")}>{check.detail}</p>}
          {check.next && !compact && (
            // Fold the (often multi-line) remediation so a panel of warnings stays
            // scannable; the operator opens "How to fix" on the one they're fixing.
            <Disclosure
              className="mt-1"
              summary={<span className="text-[11px] font-medium text-accent">How to fix</span>}
            >
              <p className="break-words text-foreground/75">{check.next}</p>
            </Disclosure>
          )}
        </div>
      </div>
    </div>
  );
}
