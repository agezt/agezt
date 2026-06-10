import { useEffect, useState } from "react";
import { Plug, PlugZap, RefreshCw, Plus, X, Trash2, Power, PowerOff } from "lucide-react";
import { getJSON, postAction, postJSON } from "@/lib/api";
import { cn } from "@/lib/utils";
import { Button } from "@/components/ui/button";
import { useUI, type ConfirmOptions } from "@/components/ui/feedback";
import { SkeletonList } from "@/components/ui/skeleton";
import { EmptyState } from "@/components/ui/empty";
import { Badge } from "@/components/ui/badge";
import { ErrorText } from "@/components/JsonView";

export interface MCPServer {
  id: string;
  name: string;
  command?: string;
  args?: string[];
  enabled?: boolean;
  description?: string;
  attached?: boolean;
  tool_count?: number;
}

// serverNameOk mirrors the kernel's mcp name rule: lowercase letter first,
// then letters/digits, ≤16 — deliberately NO underscore/dash, because the
// name is parsed out of mcp_<name>_<tool> by the policy mapper. Pure +
// unit-tested.
export function serverNameOk(s: string): boolean {
  return /^[a-z][a-z0-9]{0,15}$/.test(s);
}

// splitArgs turns the form's one-line args field into the wire list:
// whitespace-separated, blanks dropped. Pure + unit-tested.
export function splitArgs(s: string): string[] {
  return s.split(/\s+/).filter(Boolean);
}

const inputCls =
  "rounded-md border border-border bg-panel px-2 py-1 text-sm text-foreground outline-none focus-visible:border-accent";

// NewServerForm registers an MCP server (M797). Exported for tests and reuse
// (the M714 "creatable from UI" recipe).
export function NewServerForm({
  onCreated,
  onError,
}: {
  onCreated: (name: string) => void;
  onError: (msg: string) => void;
}) {
  const [state, setState] = useState<Record<string, string>>({});
  const [submitting, setSubmitting] = useState(false);
  const set = (k: string, v: string) => setState((s) => ({ ...s, [k]: v }));
  const name = (state.name || "").trim();
  const valid = serverNameOk(name) && (state.command || "").trim() !== "";

  async function create() {
    if (!valid) return;
    setSubmitting(true);
    try {
      const server: Record<string, unknown> = {
        name,
        command: (state.command || "").trim(),
        description: (state.description || "").trim(),
      };
      const args = splitArgs(state.args || "");
      if (args.length > 0) server.args = args;
      await postJSON("/api/mcp/add", { server });
      onCreated(name);
    } catch (e) {
      onError((e as Error).message);
    } finally {
      setSubmitting(false);
    }
  }

  return (
    <div className="rounded-lg border border-accent/30 bg-card p-3">
      <div className="grid gap-2 sm:grid-cols-2">
        <label className="flex flex-col gap-1 text-[11px] text-muted">
          Name — permanent handle (lowercase letters/digits); its tools appear as mcp_&lt;name&gt;_&lt;tool&gt;
          <input
            value={state.name || ""}
            onChange={(e) => set("name", e.target.value)}
            placeholder="e.g. everything"
            aria-label="Server name"
            className={cn(inputCls, name !== "" && !serverNameOk(name) && "border-bad")}
          />
        </label>
        <label className="flex flex-col gap-1 text-[11px] text-muted">
          Command — the stdio MCP server executable
          <input
            value={state.command || ""}
            onChange={(e) => set("command", e.target.value)}
            placeholder="e.g. npx"
            aria-label="Server command"
            className={inputCls}
          />
        </label>
        <label className="flex flex-col gap-1 text-[11px] text-muted">
          Arguments (space-separated)
          <input
            value={state.args || ""}
            onChange={(e) => set("args", e.target.value)}
            placeholder="-y @modelcontextprotocol/server-everything"
            aria-label="Server arguments"
            className={cn(inputCls, "font-mono text-xs")}
          />
        </label>
        <label className="flex flex-col gap-1 text-[11px] text-muted">
          Description (optional)
          <input
            value={state.description || ""}
            onChange={(e) => set("description", e.target.value)}
            placeholder="what this server provides"
            aria-label="Server description"
            className={inputCls}
          />
        </label>
      </div>
      <div className="mt-3 flex justify-end">
        <Button size="sm" onClick={create} disabled={!valid || submitting}>
          <Plus className="h-3.5 w-3.5" /> Register server
        </Button>
      </div>
    </div>
  );
}

// Mcp is the MCP self-install console (M797): register a Model Context
// Protocol server, attach it at runtime (its tools go live for every run as
// mcp_<name>_<tool> — no restart), detach (kill switch), flip auto-attach,
// remove. Every transition is journaled (mcp.*).
export function Mcp() {
  const ui = useUI();
  const [servers, setServers] = useState<MCPServer[] | null>(null);
  const [err, setErr] = useState<string | null>(null);
  const [loading, setLoading] = useState(false);
  const [busy, setBusy] = useState<string | null>(null);
  const [showForm, setShowForm] = useState(false);

  async function reload() {
    setLoading(true);
    try {
      const d = await getJSON<{ servers?: MCPServer[] }>("/api/mcp");
      setServers(d.servers || []);
      setErr(null);
    } catch (e) {
      setErr((e as Error).message);
    } finally {
      setLoading(false);
    }
  }

  useEffect(() => {
    reload();
    const t = setInterval(reload, 8000);
    return () => clearInterval(t);
    // eslint-disable-next-line react-hooks/exhaustive-deps
  }, []);

  async function act(
    ref: string,
    path: string,
    params?: Record<string, string>,
    opts?: { confirm?: ConfirmOptions; success?: (res: any) => string },
  ) {
    if (opts?.confirm && !(await ui.confirm(opts.confirm))) return;
    setBusy(ref);
    try {
      const res = await postAction(path, { ref, ...params });
      if (opts?.success) ui.toast(opts.success(res), "success");
      await reload();
    } catch (e) {
      ui.toast((e as Error).message, "error");
    } finally {
      setBusy(null);
    }
  }

  const attached = (servers || []).filter((s) => s.attached).length;

  return (
    <div className="space-y-3">
      <div className="flex flex-wrap items-center justify-between gap-2">
        <div className="flex items-center gap-2">
          <Plug className="h-4 w-4 text-accent" />
          <h2 className="text-sm font-semibold">MCP servers</h2>
          {servers && (
            <span className="text-xs text-muted">
              {servers.length} server(s) · {attached} attached
            </span>
          )}
        </div>
        <div className="flex items-center gap-1.5">
          <Button size="sm" variant="ghost" onClick={reload} disabled={loading} aria-label="Refresh">
            <RefreshCw className={cn("h-3.5 w-3.5", loading && "animate-spin")} />
          </Button>
          <Button size="sm" onClick={() => setShowForm((v) => !v)}>
            {showForm ? <X className="h-3.5 w-3.5" /> : <Plus className="h-3.5 w-3.5" />}
            {showForm ? "Close" : "Register server"}
          </Button>
        </div>
      </div>

      <p className="text-xs text-muted">
        Attach a Model Context Protocol server and its tools go live for every run as{" "}
        <span className="font-mono">mcp_&lt;name&gt;_&lt;tool&gt;</span> — no restart. The spawned process gets a
        scrubbed environment (no secrets), calls are policy-gated, and detach is the kill switch. Enabled servers
        auto-attach when the daemon starts.
      </p>

      {showForm && (
        <NewServerForm
          onCreated={(name) => {
            setShowForm(false);
            ui.toast(`server ${name} registered — attach it to make its tools callable`, "success");
            reload();
          }}
          onError={(msg) => ui.toast(msg, "error")}
        />
      )}

      {err && <ErrorText>{err}</ErrorText>}
      {!servers && !err && <SkeletonList count={3} />}
      {servers && servers.length === 0 && !showForm && (
        <EmptyState
          icon={Plug}
          title="No MCP servers yet"
          hint='Register one here (e.g. command "npx", args "-y @modelcontextprotocol/server-everything"), or let an agent install its own with the mcp tool.'
        />
      )}

      <ul className="space-y-2">
        {(servers || []).map((s) => (
          <li key={s.id} className="rounded-lg border border-border bg-card p-3">
            <div className="flex flex-wrap items-center gap-2">
              <span className="font-mono text-sm text-foreground">{s.name}</span>
              {s.attached ? (
                <Badge variant="good">attached · {s.tool_count ?? 0} tools</Badge>
              ) : (
                <Badge variant="default">registered</Badge>
              )}
              {s.enabled && <Badge variant="default">auto-attach</Badge>}
              <span className="ml-auto flex items-center gap-1">
                {!s.attached && (
                  <Button
                    size="sm"
                    variant="ghost"
                    disabled={busy === s.name}
                    aria-label={`Attach ${s.name}`}
                    onClick={() =>
                      act(s.name, "/api/mcp/attach", undefined, {
                        confirm: {
                          title: `Attach ${s.name}?`,
                          message: `The daemon spawns "${s.command || ""}" now; every run will be offered its tools as mcp_${s.name}_<tool>. The process gets a scrubbed environment.`,
                          confirmLabel: "Attach",
                        },
                        success: (res) => {
                          const tools = (res?.tools as unknown[]) || [];
                          return `${s.name} attached — ${tools.length} tool(s) live`;
                        },
                      })
                    }
                  >
                    <PlugZap className="h-3.5 w-3.5" />
                  </Button>
                )}
                {s.attached && (
                  <Button
                    size="sm"
                    variant="ghost"
                    disabled={busy === s.name}
                    aria-label={`Detach ${s.name}`}
                    onClick={() =>
                      act(s.name, "/api/mcp/detach", undefined, {
                        confirm: {
                          title: `Detach ${s.name}?`,
                          message: "The server process stops and its tools vanish from the next run. You can re-attach later.",
                          confirmLabel: "Detach",
                          danger: true,
                        },
                        success: () => `${s.name} detached`,
                      })
                    }
                  >
                    <X className="h-3.5 w-3.5" />
                  </Button>
                )}
                <Button
                  size="sm"
                  variant="ghost"
                  disabled={busy === s.name}
                  aria-label={s.enabled ? `Disable auto-attach for ${s.name}` : `Enable auto-attach for ${s.name}`}
                  title={s.enabled ? "Auto-attach at daemon start: ON" : "Auto-attach at daemon start: OFF"}
                  onClick={() =>
                    act(s.name, "/api/mcp/enable", { enabled: s.enabled ? "false" : "true" }, {
                      success: () =>
                        s.enabled ? `${s.name} will not auto-attach at start` : `${s.name} will auto-attach at start`,
                    })
                  }
                >
                  {s.enabled ? <Power className="h-3.5 w-3.5" /> : <PowerOff className="h-3.5 w-3.5" />}
                </Button>
                <Button
                  size="sm"
                  variant="ghost"
                  disabled={busy === s.name}
                  aria-label={`Remove ${s.name}`}
                  onClick={() =>
                    act(s.name, "/api/mcp/remove", undefined, {
                      confirm: {
                        title: `Remove server ${s.name}?`,
                        message: "It is detached first if live, then the registration is deleted. Past activity stays in the journal.",
                        confirmLabel: "Remove",
                        danger: true,
                      },
                      success: () => `${s.name} removed`,
                    })
                  }
                >
                  <Trash2 className="h-3.5 w-3.5" />
                </Button>
              </span>
            </div>
            <div className="mt-1.5 flex flex-wrap gap-x-4 gap-y-1 text-xs text-muted">
              <span className="font-mono">
                {s.command}
                {(s.args || []).length > 0 ? " " + (s.args || []).join(" ") : ""}
              </span>
            </div>
            {s.description && <div className="mt-1 text-xs text-muted">{s.description}</div>}
          </li>
        ))}
      </ul>
    </div>
  );
}
