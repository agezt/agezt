import { useEffect, useState } from "react";
import { Plug, PlugZap, RefreshCw, Plus, X, Trash2, Power, PowerOff, Boxes, KeyRound } from "lucide-react";
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
  // url + transport: remote (Streamable HTTP) servers (M904). transport is
  // "stdio" | "http"; url is set only for http.
  url?: string;
  transport?: string;
  enabled?: boolean;
  description?: string;
  attached?: boolean;
  tool_count?: number;
  // env values are redacted by the backend; only the key names come back (M898).
  env_keys?: string[];
  // header values are redacted too; only the names come back for remote servers (M904).
  header_keys?: string[];
  // tool_allow: optional per-server allowlist of tool names to expose (M899).
  tool_allow?: string[];
  // lazy: collapse this server's tools into one mcp_<name> dispatcher (M906).
  lazy?: boolean;
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

// splitTools turns the form's tool-allowlist field into the wire list:
// split on whitespace or commas, blanks dropped. Pure + unit-tested (M899).
export function splitTools(s: string): string[] {
  return (s || "").split(/[\s,]+/).filter(Boolean);
}

// parseEnv turns the form's "KEY=value" lines into the wire env map (M898).
// Blank lines and lines without "=" are dropped; the value keeps any later "="
// (e.g. a base64 token). Pure + unit-tested.
export function parseEnv(s: string): Record<string, string> {
  const out: Record<string, string> = {};
  for (const line of (s || "").split("\n")) {
    const t = line.trim();
    if (!t || t.startsWith("#")) continue;
    const eq = t.indexOf("=");
    if (eq <= 0) continue;
    const key = t.slice(0, eq).trim();
    if (key) out[key] = t.slice(eq + 1).trim();
  }
  return out;
}

// parseHeaders turns the form's "Name: value" lines into the wire header map for
// a remote server (M904) — e.g. "Authorization: Bearer ...". Blank/"#" lines and
// lines without ":" are dropped; the value keeps any later ":". Pure + tested.
export function parseHeaders(s: string): Record<string, string> {
  const out: Record<string, string> = {};
  for (const line of (s || "").split("\n")) {
    const t = line.trim();
    if (!t || t.startsWith("#")) continue;
    const c = t.indexOf(":");
    if (c <= 0) continue;
    const key = t.slice(0, c).trim();
    if (key) out[key] = t.slice(c + 1).trim();
  }
  return out;
}

// urlOk is a light client-side check mirroring the kernel: http(s) with a host.
// The server re-validates; this just keeps the Register button honest. Pure.
export function urlOk(s: string): boolean {
  try {
    const u = new URL((s || "").trim());
    return (u.protocol === "http:" || u.protocol === "https:") && u.host !== "";
  } catch {
    return false;
  }
}

// CatalogCategory groups the (now ~40-entry) gallery so the operator can browse
// by intent instead of scrolling one flat grid (M912).
export type CatalogCategory = "core" | "web" | "data" | "dev" | "apps";

export const CATEGORY_LABELS: Record<CatalogCategory, string> = {
  core: "Core",
  web: "Web & search",
  data: "Databases",
  dev: "Dev & cloud",
  apps: "Apps & docs",
};

// CatalogEntry is one preset in the popular-servers gallery (M897). `args` is in
// the form's space-separated shape; `needs` flags a path/secret the operator must
// supply before it works. Names obey the kernel rule (≤16 lowercase alnum).
export interface CatalogEntry {
  name: string;
  category: CatalogCategory;
  // command/args: a stdio preset. url/headers: a remote (http) preset (M904).
  // Exactly one shape is set per entry.
  command?: string;
  args?: string;
  url?: string;
  description: string;
  needs?: string;
  // env: names of environment variables this server needs — prefilled (with
  // blank values) into the register form's env field for the operator to fill.
  env?: string[];
  // headers: names of HTTP headers a remote preset needs — prefilled (with blank
  // values, "Name: ") into the register form's headers field.
  headers?: string[];
}

// transportOf reports a catalog entry's transport from its shape. Pure + tested.
export function transportOf(e: CatalogEntry): "stdio" | "http" {
  return e.url ? "http" : "stdio";
}

// filterCatalog narrows the gallery by category chip and a free-text query over
// name + description (M912). Pure + unit-tested.
export function filterCatalog(
  entries: CatalogEntry[],
  cat: CatalogCategory | "all",
  query: string,
): CatalogEntry[] {
  const q = query.trim().toLowerCase();
  return entries.filter(
    (e) =>
      (cat === "all" || e.category === cat) &&
      (q === "" || e.name.includes(q) || e.description.toLowerCase().includes(q)),
  );
}

// CATALOG: popular Model Context Protocol servers, offered as one-click examples.
// Picking one prefills the register form so the operator can review/adjust the
// path or note the credential before adding. Sourced from the maintained official
// reference servers, first-party vendor servers, and widely-used community
// servers (package names verified against npm/PyPI, 2026-06). Archived reference
// servers that still install fine (postgres, gdrive, google-maps, github stdio)
// stay listed; dead/never-published ones don't.
export const CATALOG: CatalogEntry[] = [
  // ── Core: the maintained official reference servers ──────────────────────
  { name: "everything", category: "core", command: "npx", args: "-y @modelcontextprotocol/server-everything", description: "Reference server exercising every MCP feature — ideal for a first test." },
  { name: "filesystem", category: "core", command: "npx", args: "-y @modelcontextprotocol/server-filesystem /path/to/dir", description: "Read and write files within an allowed directory.", needs: "set the directory path in args" },
  { name: "fetch", category: "core", command: "uvx", args: "mcp-server-fetch", description: "Fetch a URL and return its content as clean markdown." },
  { name: "memory", category: "core", command: "npx", args: "-y @modelcontextprotocol/server-memory", description: "Persistent knowledge-graph memory the model can read and write." },
  { name: "git", category: "core", command: "uvx", args: "mcp-server-git --repository /path/to/repo", description: "Inspect and operate a local Git repository.", needs: "set the repo path in args" },
  { name: "time", category: "core", command: "uvx", args: "mcp-server-time", description: "Current time and timezone conversions." },
  { name: "thinking", category: "core", command: "npx", args: "-y @modelcontextprotocol/server-sequential-thinking", description: "A structured step-by-step reasoning scratchpad." },
  // ── Web, search & browsing ────────────────────────────────────────────────
  { name: "playwright", category: "web", command: "npx", args: "-y @playwright/mcp@latest", description: "Browser automation via Playwright — navigate, click, fill forms, screenshot (official Microsoft)." },
  { name: "duckduckgo", category: "web", command: "uvx", args: "duckduckgo-mcp-server", description: "Free web search via DuckDuckGo — no API key required." },
  { name: "brave", category: "web", command: "npx", args: "-y @brave/brave-search-mcp-server", description: "Web, image, news and local search via the Brave Search API (official).", needs: "BRAVE_API_KEY (env)", env: ["BRAVE_API_KEY"] },
  { name: "tavily", category: "web", command: "npx", args: "-y tavily-mcp@latest", description: "AI-grade web search and page extraction via the Tavily API.", needs: "TAVILY_API_KEY (env)", env: ["TAVILY_API_KEY"] },
  { name: "exa", category: "web", command: "npx", args: "-y exa-mcp-server", description: "Neural web search built for agents — web, code, company and paper search.", needs: "EXA_API_KEY (env)", env: ["EXA_API_KEY"] },
  { name: "firecrawl", category: "web", command: "npx", args: "-y firecrawl-mcp", description: "Scrape, crawl and extract whole websites as clean markdown.", needs: "FIRECRAWL_API_KEY (env)", env: ["FIRECRAWL_API_KEY"] },
  { name: "googlemaps", category: "web", command: "npx", args: "-y @modelcontextprotocol/server-google-maps", description: "Geocoding, directions and place details via Google Maps.", needs: "GOOGLE_MAPS_API_KEY (env)", env: ["GOOGLE_MAPS_API_KEY"] },
  { name: "youtube", category: "web", command: "npx", args: "-y @kimtaeyoon83/mcp-server-youtube-transcript", description: "Fetch YouTube video transcripts/subtitles for summarizing." },
  // ── Databases & vector stores ─────────────────────────────────────────────
  { name: "postgres", category: "data", command: "npx", args: "-y @modelcontextprotocol/server-postgres postgresql://user:pass@host/db", description: "Read-only SQL queries against a PostgreSQL database.", needs: "set the connection string in args" },
  { name: "sqlite", category: "data", command: "uvx", args: "mcp-server-sqlite --db-path /path/to.db", description: "Query a local SQLite database file.", needs: "set the db path in args" },
  { name: "mongodb", category: "data", command: "npx", args: "-y mongodb-mcp-server", description: "MongoDB & Atlas — query collections, inspect schemas, run aggregations (official).", needs: "MDB_MCP_CONNECTION_STRING (env)", env: ["MDB_MCP_CONNECTION_STRING"] },
  { name: "redis", category: "data", command: "uvx", args: "--from redis-mcp-server@latest redis-mcp-server --url redis://localhost:6379/0", description: "Manage and search data in Redis (official).", needs: "set the redis url in args" },
  { name: "supabase", category: "data", command: "npx", args: "-y @supabase/mcp-server-supabase@latest --access-token sbp_YOUR_TOKEN", description: "Manage Supabase projects — SQL, migrations, logs, types (official).", needs: "Supabase personal access token in args" },
  { name: "neon", category: "data", command: "npx", args: "-y @neondatabase/mcp-server-neon start napi_YOUR_KEY", description: "Neon serverless Postgres — projects, branches, SQL (official).", needs: "Neon API key in args" },
  { name: "qdrant", category: "data", command: "uvx", args: "mcp-server-qdrant", description: "Semantic memory on the Qdrant vector database (official).", needs: "QDRANT_URL + COLLECTION_NAME (env)", env: ["QDRANT_URL", "COLLECTION_NAME"] },
  { name: "chroma", category: "data", command: "uvx", args: "chroma-mcp --client-type persistent --data-dir /path/to/data", description: "Chroma vector database — collections, embeddings, semantic search (official).", needs: "set the data dir in args" },
  { name: "pinecone", category: "data", command: "npx", args: "-y @pinecone-database/mcp", description: "Pinecone vector database — search docs, manage indexes (official).", needs: "PINECONE_API_KEY (env)", env: ["PINECONE_API_KEY"] },
  // ── Dev & cloud ───────────────────────────────────────────────────────────
  { name: "github", category: "dev", command: "npx", args: "-y @modelcontextprotocol/server-github", description: "GitHub API — issues, pull requests, repos, code search.", needs: "GITHUB_PERSONAL_ACCESS_TOKEN (env)", env: ["GITHUB_PERSONAL_ACCESS_TOKEN"] },
  { name: "githubremote", category: "dev", url: "https://api.githubcopilot.com/mcp/", description: "GitHub's hosted MCP endpoint — issues, PRs, repos, code search (no local install).", needs: "Authorization: Bearer <GitHub PAT>", headers: ["Authorization"] },
  { name: "kubernetes", category: "dev", command: "npx", args: "-y mcp-server-kubernetes", description: "Manage a Kubernetes cluster — kubectl and Helm tools over your local kubeconfig." },
  { name: "awsdocs", category: "dev", command: "uvx", args: "awslabs.aws-documentation-mcp-server@latest", description: "Search and read AWS documentation (official AWS Labs)." },
  { name: "azure", category: "dev", command: "npx", args: "-y @azure/mcp@latest server start", description: "Azure resources — storage, Cosmos DB, CLI and more (official Microsoft).", needs: "Azure credentials (az login on this machine)" },
  { name: "sentry", category: "dev", command: "npx", args: "-y @sentry/mcp-server@latest", description: "Sentry issues, stack traces and error analysis (official).", needs: "SENTRY_ACCESS_TOKEN (env)", env: ["SENTRY_ACCESS_TOKEN"] },
  { name: "deepwiki", category: "dev", url: "https://mcp.deepwiki.com/mcp", description: "Ask questions about any public GitHub repo's docs via DeepWiki (hosted, no auth)." },
  { name: "context7", category: "dev", url: "https://mcp.context7.com/mcp", description: "Up-to-date library/framework docs for coding questions (hosted by Upstash).", needs: "Authorization: Bearer <Context7 API key> (optional)", headers: ["Authorization"] },
  { name: "huggingface", category: "dev", url: "https://huggingface.co/mcp", description: "Hugging Face Hub — search models, datasets, papers and Spaces (official, hosted).", needs: "Authorization: Bearer <HF token>", headers: ["Authorization"] },
  // ── Apps, docs & productivity ─────────────────────────────────────────────
  { name: "notion", category: "apps", command: "npx", args: "-y @notionhq/notion-mcp-server", description: "Notion pages and databases — read, search, create, update (official).", needs: "NOTION_TOKEN (env)", env: ["NOTION_TOKEN"] },
  { name: "linear", category: "apps", command: "npx", args: "-y mcp-remote https://mcp.linear.app/mcp", description: "Linear issues and projects via the hosted endpoint (bridged with mcp-remote).", needs: "browser OAuth login on first attach" },
  { name: "atlassian", category: "apps", command: "uvx", args: "mcp-atlassian", description: "Jira and Confluence — issues, sprints, pages; set CONFLUENCE_* env too if you use it.", needs: "JIRA_URL + JIRA_USERNAME + JIRA_API_TOKEN (env)", env: ["JIRA_URL", "JIRA_USERNAME", "JIRA_API_TOKEN"] },
  { name: "slack", category: "apps", command: "npx", args: "-y slack-mcp-server@latest --transport stdio", description: "Slack channels, DMs and history — no workspace app approval needed.", needs: "SLACK_MCP_XOXC_TOKEN + SLACK_MCP_XOXD_TOKEN (env)", env: ["SLACK_MCP_XOXC_TOKEN", "SLACK_MCP_XOXD_TOKEN"] },
  { name: "gdrive", category: "apps", command: "npx", args: "-y @modelcontextprotocol/server-gdrive", description: "Search and read files in Google Drive.", needs: "OAuth credentials (env)" },
  { name: "airtable", category: "apps", command: "npx", args: "-y airtable-mcp-server", description: "Read and write Airtable bases, tables and records.", needs: "AIRTABLE_API_KEY (env)", env: ["AIRTABLE_API_KEY"] },
  { name: "stripe", category: "apps", command: "npx", args: "-y @stripe/mcp --tools=all --api-key=sk_YOUR_KEY", description: "Stripe — customers, products, payment links, invoices (official).", needs: "Stripe secret key in args" },
  { name: "obsidian", category: "apps", command: "npx", args: "-y mcp-obsidian /path/to/vault", description: "Read and search an Obsidian vault (markdown notes).", needs: "set the vault path in args" },
  { name: "excel", category: "apps", command: "uvx", args: "excel-mcp-server stdio", description: "Create and edit Excel workbooks — formulas, charts, pivots (no Excel install needed)." },
  { name: "arxiv", category: "apps", command: "uvx", args: "arxiv-mcp-server", description: "Search, download and read arXiv papers." },
];

const inputCls =
  "rounded-md border border-border bg-panel px-2 py-1 text-sm text-foreground outline-none focus-visible:border-accent";

// NewServerForm registers an MCP server (M797). Exported for tests and reuse
// (the M714 "creatable from UI" recipe). `initial` pre-fills the fields — used
// by the popular-servers catalog (M897) to seed name/command/args/description.
export function NewServerForm({
  onCreated,
  onError,
  initial,
}: {
  onCreated: (name: string) => void;
  onError: (msg: string) => void;
  initial?: Record<string, string>;
}) {
  const [state, setState] = useState<Record<string, string>>(initial || {});
  const [submitting, setSubmitting] = useState(false);
  const set = (k: string, v: string) => setState((s) => ({ ...s, [k]: v }));
  const name = (state.name || "").trim();
  const remote = (state.transport || "stdio") === "http";
  const valid =
    serverNameOk(name) &&
    (remote ? urlOk(state.url || "") : (state.command || "").trim() !== "");

  async function create() {
    if (!valid) return;
    setSubmitting(true);
    try {
      const server: Record<string, unknown> = {
        name,
        description: (state.description || "").trim(),
      };
      if (remote) {
        server.url = (state.url || "").trim();
        const headers = parseHeaders(state.headers || "");
        if (Object.keys(headers).length > 0) server.headers = headers;
      } else {
        server.command = (state.command || "").trim();
        const args = splitArgs(state.args || "");
        if (args.length > 0) server.args = args;
        const env = parseEnv(state.env || "");
        if (Object.keys(env).length > 0) server.env = env;
      }
      const toolAllow = splitTools(state.tool_allow || "");
      if (toolAllow.length > 0) server.tool_allow = toolAllow;
      if (state.lazy === "true") server.lazy = true;
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
      <div className="mb-2 inline-flex rounded-md border border-border bg-panel p-0.5 text-xs" role="tablist" aria-label="Transport">
        <button
          type="button"
          role="tab"
          aria-selected={!remote}
          onClick={() => set("transport", "stdio")}
          className={cn("rounded px-2.5 py-1", !remote ? "bg-accent text-accent-foreground" : "text-muted")}
        >
          Local (stdio)
        </button>
        <button
          type="button"
          role="tab"
          aria-selected={remote}
          onClick={() => set("transport", "http")}
          className={cn("rounded px-2.5 py-1", remote ? "bg-accent text-accent-foreground" : "text-muted")}
        >
          Remote (HTTP)
        </button>
      </div>
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
        {remote ? (
          <label className="flex flex-col gap-1 text-[11px] text-muted">
            URL — the remote MCP endpoint (Streamable HTTP)
            <input
              value={state.url || ""}
              onChange={(e) => set("url", e.target.value)}
              placeholder="https://api.example.com/mcp"
              aria-label="Server URL"
              className={cn(inputCls, "font-mono text-xs", (state.url || "").trim() !== "" && !urlOk(state.url || "") && "border-bad")}
            />
          </label>
        ) : (
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
        )}
        {!remote && (
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
        )}
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
        {remote ? (
          <label className="flex flex-col gap-1 text-[11px] text-muted sm:col-span-2">
            Headers (optional) — one <span className="font-mono">Name: value</span> per line, sent on every request to
            this server (e.g. <span className="font-mono">Authorization: Bearer …</span>). Values are stored in the
            registry and never shown again, so use a dedicated low-scope token.
            <textarea
              value={state.headers || ""}
              onChange={(e) => set("headers", e.target.value)}
              placeholder={"Authorization: Bearer ..."}
              aria-label="Server headers"
              rows={2}
              className={cn(inputCls, "font-mono text-xs")}
            />
          </label>
        ) : (
          <label className="flex flex-col gap-1 text-[11px] text-muted sm:col-span-2">
            Environment (optional) — one <span className="font-mono">KEY=value</span> per line, injected only into this
            server (e.g. an API token). The base environment stays scrubbed; values are stored in the registry and never
            shown again, so use a dedicated low-scope token.
            <textarea
              value={state.env || ""}
              onChange={(e) => set("env", e.target.value)}
              placeholder={"GITHUB_PERSONAL_ACCESS_TOKEN=ghp_..."}
              aria-label="Server environment"
              rows={2}
              className={cn(inputCls, "font-mono text-xs")}
            />
          </label>
        )}
        <label className="flex flex-col gap-1 text-[11px] text-muted sm:col-span-2">
          Tools allowlist (optional) — only expose these tool names to runs (space/comma-separated); leave blank for
          all. Trims a chatty server so its schemas don’t bloat every run’s context. Attach first to discover the tool
          names, then set this.
          <input
            value={state.tool_allow || ""}
            onChange={(e) => set("tool_allow", e.target.value)}
            placeholder="create_issue search_code"
            aria-label="Server tool allowlist"
            className={cn(inputCls, "font-mono text-xs")}
          />
        </label>
        <label className="flex items-start gap-2 text-[11px] text-muted sm:col-span-2">
          <input
            type="checkbox"
            checked={state.lazy === "true"}
            onChange={(e) => set("lazy", e.target.checked ? "true" : "")}
            aria-label="Lazy load tools"
            className="mt-0.5"
          />
          <span>
            Lazy load — collapse this server’s tools into a single{" "}
            <span className="font-mono">mcp_&lt;name&gt;</span> dispatcher instead of injecting every tool’s schema into
            each run. Best for chatty servers (e.g. github’s ~30 tools); the model picks a tool and the server validates
            the arguments.
          </span>
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
  const [showCatalog, setShowCatalog] = useState(false);
  // Pre-fill values for the register form when a catalog entry is picked (M897).
  const [prefill, setPrefill] = useState<Record<string, string> | undefined>(undefined);
  // Gallery filters (M912): category chip + free-text search over name/description.
  const [catFilter, setCatFilter] = useState<CatalogCategory | "all">("all");
  const [catQuery, setCatQuery] = useState("");
  const registered = new Set((servers || []).map((s) => s.name));

  function useCatalogEntry(e: CatalogEntry) {
    const transport = transportOf(e);
    setPrefill({
      name: e.name,
      transport,
      command: e.command || "",
      args: e.args || "",
      url: e.url || "",
      description: e.description,
      // Prefill blank KEY= / "Name: " lines so the operator just pastes the secret.
      env: (e.env || []).map((k) => `${k}=`).join("\n"),
      headers: (e.headers || []).map((k) => `${k}: `).join("\n"),
    });
    setShowCatalog(false);
    setShowForm(true);
  }

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
          <Button
            size="sm"
            variant={showCatalog ? "default" : "ghost"}
            onClick={() => {
              setShowCatalog((v) => !v);
              setShowForm(false);
            }}
          >
            <Boxes className="h-3.5 w-3.5" /> Popular servers
          </Button>
          <Button
            size="sm"
            onClick={() => {
              setShowForm((v) => !v);
              if (showForm) setPrefill(undefined);
              setShowCatalog(false);
            }}
          >
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

      {showCatalog && (
        <div className="rounded-lg border border-accent/30 bg-card p-3">
          <div className="mb-2 flex items-center gap-2 text-xs text-muted">
            <Boxes className="h-3.5 w-3.5 text-accent" />
            Popular MCP servers — pick one to prefill the form, then adjust any path or credential and register.
            Most run via <span className="font-mono">npx</span>/<span className="font-mono">uvx</span> (Node/Python must be installed).
          </div>
          <div className="mb-2 flex flex-wrap items-center gap-1.5">
            {(["all", ...(Object.keys(CATEGORY_LABELS) as CatalogCategory[])] as (CatalogCategory | "all")[]).map(
              (c) => (
                <button
                  key={c}
                  type="button"
                  onClick={() => setCatFilter(c)}
                  aria-pressed={catFilter === c}
                  className={cn(
                    "rounded-full border px-2.5 py-0.5 text-[11px]",
                    catFilter === c
                      ? "border-accent bg-accent text-accent-foreground"
                      : "border-border bg-panel text-muted hover:text-foreground",
                  )}
                >
                  {c === "all" ? `All (${CATALOG.length})` : CATEGORY_LABELS[c]}
                </button>
              ),
            )}
            <input
              value={catQuery}
              onChange={(e) => setCatQuery(e.target.value)}
              placeholder="search servers…"
              aria-label="Search catalog"
              className={cn(inputCls, "ml-auto w-44 text-xs")}
            />
          </div>
          <div className="grid gap-2 sm:grid-cols-2 lg:grid-cols-3">
            {filterCatalog(CATALOG, catFilter, catQuery).map((e) => {
              const already = registered.has(e.name);
              return (
                <div key={e.name} className="flex flex-col rounded-md border border-border bg-panel/40 p-2.5">
                  <div className="flex items-center gap-2">
                    <span className="font-mono text-sm text-foreground">{e.name}</span>
                    {transportOf(e) === "http" && <Badge variant="default">remote</Badge>}
                    {already && <Badge variant="good">added</Badge>}
                    <Button
                      size="sm"
                      variant="ghost"
                      className="ml-auto"
                      disabled={already}
                      aria-label={`Use ${e.name}`}
                      onClick={() => useCatalogEntry(e)}
                    >
                      <Plus className="h-3.5 w-3.5" /> Use
                    </Button>
                  </div>
                  <p className="mt-1 text-xs text-muted">{e.description}</p>
                  <p
                    className="mt-1 truncate font-mono text-[10px] text-muted/80"
                    title={e.url || `${e.command} ${e.args}`}
                  >
                    {e.url || `${e.command} ${e.args}`}
                  </p>
                  {e.needs && (
                    <p className="mt-1 flex items-center gap-1 text-[10px] text-amber-500/90">
                      <KeyRound className="h-3 w-3" /> needs: {e.needs}
                    </p>
                  )}
                </div>
              );
            })}
          </div>
          {filterCatalog(CATALOG, catFilter, catQuery).length === 0 && (
            <p className="py-3 text-center text-xs text-muted">No presets match “{catQuery.trim()}”.</p>
          )}
          <p className="mt-2 text-[10px] text-muted/80">
            Note: servers marked “needs … (env)” get the secret injected only into that server’s process via the env
            field — the rest of the environment stays scrubbed. “Use” prefills the key names so you just paste the value.
          </p>
        </div>
      )}

      {showForm && (
        <NewServerForm
          key={prefill?.name || "blank"}
          initial={prefill}
          onCreated={(name) => {
            setShowForm(false);
            setPrefill(undefined);
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
              {s.transport === "http" && <Badge variant="default">remote</Badge>}
              {s.attached ? (
                <Badge variant="good">attached · {s.tool_count ?? 0} tools</Badge>
              ) : (
                <Badge variant="default">registered</Badge>
              )}
              {s.enabled && <Badge variant="default">auto-attach</Badge>}
              {s.lazy && (
                <Badge variant="default" title="Tools collapsed into one mcp_<name> dispatcher">
                  lazy
                </Badge>
              )}
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
                          message:
                            s.transport === "http"
                              ? `The daemon connects to "${s.url || ""}" now (Streamable HTTP); every run will be offered its tools as mcp_${s.name}_<tool>.`
                              : `The daemon spawns "${s.command || ""}" now; every run will be offered its tools as mcp_${s.name}_<tool>. The process gets a scrubbed environment.`,
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
                {s.transport === "http"
                  ? s.url
                  : `${s.command || ""}${(s.args || []).length > 0 ? " " + (s.args || []).join(" ") : ""}`}
              </span>
              {(s.env_keys || []).length > 0 && (
                <span className="flex items-center gap-1 text-[11px]">
                  <KeyRound className="h-3 w-3" /> env: {(s.env_keys || []).join(", ")}
                </span>
              )}
              {(s.header_keys || []).length > 0 && (
                <span className="flex items-center gap-1 text-[11px]">
                  <KeyRound className="h-3 w-3" /> headers: {(s.header_keys || []).join(", ")}
                </span>
              )}
              {(s.tool_allow || []).length > 0 && (
                <span className="text-[11px]" title="Only these tools are exposed to runs">
                  tools: {(s.tool_allow || []).join(", ")}
                </span>
              )}
            </div>
            {s.description && <div className="mt-1 text-xs text-muted">{s.description}</div>}
          </li>
        ))}
      </ul>
    </div>
  );
}
