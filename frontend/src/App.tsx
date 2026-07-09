import { Suspense, useEffect, useMemo, useRef, useState } from "react";
import { Bot, X, RefreshCw } from "lucide-react";
import { postAction, getJSON } from "@/lib/api";
import { useEvents } from "@/lib/events";
import { ingestCouncilEvent } from "@/lib/councilStore";
import { ingestConductorEvent } from "@/lib/conductorStore";
import { attentionAlertCount } from "@/lib/alerts";
import { foldActivityEvent, summarize, type ActivityState } from "@/lib/activity";
import { CommandPalette } from "@/components/CommandPalette";
import { HelpDrawer } from "@/components/HelpDrawer";
import { Inspector, InspectorClosedBar } from "@/components/Inspector";
import { MiniChat } from "@/components/MiniChat";
import { Vitals } from "@/components/Vitals";
import { FleetNowBar } from "@/components/FleetNowBar";
import { useUI } from "@/components/ui/feedback";
import { TooltipProvider } from "@/components/ui/tooltip";
import type { CommandItem } from "@/lib/commands";
import { toggleTheme } from "@/lib/theme";
import { toggleAdvanced } from "@/lib/advanced";
import { useChat } from "@/lib/chatStore";
import { focusRun } from "@/lib/runfocus";
import { agentSlugFromHash, openAgent } from "@/lib/agentnav";
import { incidentIdFromHash } from "@/lib/incidentnav";
import { goToView } from "@/lib/nav";
import { exportAppearance, parseAppearanceJSON, applyAppearanceBundle } from "@/lib/appearance";
import { parseConfigBundle, fetchConfigBundle, applyConfigBundle } from "@/lib/configbackup";
import { downloadText } from "@/lib/export";
import { ConsoleName } from "@/components/ConsoleName";
import { anyCredentialed, type SetupCatalog } from "@/lib/setup";
import { SectionNav, Header } from "@/components/AppNav";
import {
  NAV,
  NAV_GROUPS,
  groupForView,
  sectionForView,
  viewFromHash,
  Setup,
  AgentPage,
  IncidentPage,
} from "@/nav";

export default function App() {
  const [active, setActiveRaw] = useState(viewFromHash);
  const [hashKey, setHashKey] = useState(() => location.hash);
  // The agent currently addressed by `#agent/<slug>` (M960), or null on a normal
  // view. When set, the main area renders the full-page AgentPage instead of the
  // active nav view.
  const [agentSlug, setAgentSlug] = useState<string | null>(() => agentSlugFromHash(location.hash));
  const [incidentId, setIncidentId] = useState<string | null>(() => incidentIdFromHash(location.hash));
  const { newChat } = useChat();
  const [paletteOpen, setPaletteOpen] = useState(false);
  // Page-aware help drawer (M920): one global toggle, content follows `active`.
  const [helpOpen, setHelpOpen] = useState(false);
  // Collapsible debug inspector (Ctrl+Shift+I) — LLM calls, tool traces, live events.
  const [inspectorOpen, setInspectorOpen] = useState(false);
  // Recent runs offered as ⌘K "Open run" commands (fulfils the palette's promise).
  // Refreshed whenever the palette opens so the list is current without polling.
  const [recentRuns, setRecentRuns] = useState<{ correlation_id?: string; intent?: string; status?: string }[]>([]);
  // Build provenance (M971): the daemon's semver + git revision, shown in the
  // sidebar footer so it's unambiguous which build is actually running (the
  // semver alone can't distinguish two dev builds).
  const [build, setBuild] = useState<{ version?: string; revision?: string; built?: string; build_modified?: boolean } | null>(null);
  // Every roster agent offered as a ⌘K "Open agent" command, so any created
  // agent's identity page is one keystroke away from anywhere (M967).
  const [paletteAgents, setPaletteAgents] = useState<{ slug: string; name?: string; system?: boolean; retired?: boolean }[]>([]);
  // Two-level nav (M974): the section whose items the secondary list shows. It
  // follows the active view's section, but a rail click can browse another
  // section without navigating yet.
  const [navSection, setNavSection] = useState<string>(() => groupForView[viewFromHash()] || NAV_GROUPS[0].id);
  // Mobile nav drawer (below lg, where the two-level sidebar is hidden).
  const [navDrawerOpen, setNavDrawerOpen] = useState(false);
  const navDrawerRef = useRef<HTMLDivElement>(null);
  // Make the drawer a real modal (it claims aria-modal): move focus in, trap Tab,
  // lock background scroll, and restore focus to the opener on close.
  useEffect(() => {
    if (!navDrawerOpen) return;
    const prevFocused = document.activeElement as HTMLElement | null;
    const prevOverflow = document.body.style.overflow;
    document.body.style.overflow = "hidden";
    const focusables = () =>
      navDrawerRef.current
        ? Array.from(
            navDrawerRef.current.querySelectorAll<HTMLElement>(
              'button, [href], input, select, textarea, [tabindex]:not([tabindex="-1"])',
            ),
          ).filter((el) => !el.hasAttribute("disabled") && el.offsetParent !== null)
        : [];
    focusables()[0]?.focus();
    const onKey = (e: KeyboardEvent) => {
      if (e.key === "Escape") {
        setNavDrawerOpen(false);
        return;
      }
      if (e.key === "Tab") {
        const items = focusables();
        if (items.length === 0) return;
        const first = items[0];
        const last = items[items.length - 1];
        if (e.shiftKey && document.activeElement === first) {
          e.preventDefault();
          last.focus();
        } else if (!e.shiftKey && document.activeElement === last) {
          e.preventDefault();
          first.focus();
        }
      }
    };
    window.addEventListener("keydown", onKey);
    return () => {
      window.removeEventListener("keydown", onKey);
      document.body.style.overflow = prevOverflow;
      prevFocused?.focus?.();
    };
  }, [navDrawerOpen]);
  const { connected, events, subscribe } = useEvents();
  const ui = useUI();

  // Live LLM call counter for the inspector badge.
  const [activeLlmCount, setActiveLlmCount] = useState(0);
  useEffect(() => {
    return subscribe((ev) => {
      const k = ev.kind;
      if (k === "llm.request") setActiveLlmCount((c) => c + 1);
      else if (k === "llm.response") setActiveLlmCount((c) => Math.max(0, c - 1));
    });
  }, [subscribe]);

  // Feed council.* events into the module-level council store (M987) from the app
  // level, so a deliberation keeps assembling even when the Council view isn't
  // mounted — letting the operator navigate away and return mid-run.
  useEffect(() => subscribe(ingestCouncilEvent), [subscribe]);
  useEffect(() => subscribe(ingestConductorEvent), [subscribe]);

  // Keyboard shortcut: Ctrl+Shift+I toggles the debug inspector.
  useEffect(() => {
    function onKey(e: KeyboardEvent) {
      if (e.key === "I" && e.ctrlKey && e.shiftKey) {
        e.preventDefault();
        setInspectorOpen((v) => !v);
      }
    }
    window.addEventListener("keydown", onKey);
    return () => window.removeEventListener("keydown", onKey);
  }, []);

  // Fetch the daemon's build provenance once for the sidebar footer (M971).
  useEffect(() => {
    getJSON<{ version?: string; revision?: string; built?: string; build_modified?: boolean }>("/api/version")
      .then(setBuild)
      .catch(() => {});
  }, []);

  // Keep the secondary nav list pointed at the active view's section (M974), so
  // navigating (incl. via ⌘K or a deep link) reveals the right item list.
  const activeGroupId = groupForView[active] || NAV_GROUPS[0].id;
  useEffect(() => {
    setNavSection(activeGroupId);
  }, [activeGroupId]);
  const shownGroup = NAV_GROUPS.find((g) => g.id === navSection) || NAV_GROUPS[0];

  // Unseen-alert badge on the Alerts nav item (M779): count the critical/warning alerts
  // in the live buffer so the cockpit flags "something needs attention" from anywhere —
  // not only when you happen to open the Alerts tab. Opening that tab marks them seen.
  const liveAlertCount = useMemo(() => attentionAlertCount(events, { nowMs: Date.now() }), [events]);
  const [seenAlerts, setSeenAlerts] = useState(0);
  useEffect(() => {
    if (active === "alerts") setSeenAlerts(liveAlertCount);
  }, [active, liveAlertCount]);
  const unseenAlerts = Math.max(0, liveAlertCount - seenAlerts);

  // Live "active runs" badge on the Overseer nav item (M868): fold the live event
  // buffer into run state and count those still running, so the operator sees how
  // many runs are in flight from ANY view — ambient monitoring, like the alert
  // badge. Events are newest-first, so fold in reverse (chronological) order.
  // Same cold-start semantics as the alert badge: it reflects the live buffer.
  const activeRunCount = useMemo(() => {
    let state: ActivityState = {};
    for (let i = events.length - 1; i >= 0; i--) state = foldActivityEvent(state, events[i]);
    return summarize(state).running;
  }, [events]);

  const current = NAV.find((n) => n.id === active) || NAV[0];
  const View = current.render;

  // First-run setup (M816): auto-open the full-screen wizard until the catalog
  // reports at least one usable provider. API-key providers become usable when a
  // key exists; keyless/local providers become usable when they are runnable.
  // Once dismissed for this install, it stays quiet until opened from nav.
  const [needsSetup, setNeedsSetup] = useState(false);
  useEffect(() => {
    if (localStorage.getItem("agezt.setup.skipped") === "1") return;
    getJSON<SetupCatalog>("/api/catalog")
      .then((c) => setNeedsSetup(!anyCredentialed(c)))
      .catch(() => {});
  }, []);
  // Hidden inputs behind the ⌘K "Import appearance" / "Import configuration" commands.
  const appearanceFileRef = useRef<HTMLInputElement>(null);
  const configFileRef = useRef<HTMLInputElement>(null);

  async function importAppearanceFile(file: File) {
    try {
      const bundle = parseAppearanceJSON(await file.text());
      applyAppearanceBundle(bundle);
      ui.toast(`Appearance imported (${Object.keys(bundle).join(", ")})`, "success");
    } catch (e) {
      ui.toast(`Import failed: ${(e as Error).message}`, "error");
    }
  }

  // Export the daemon-side config (default identity + prompt templates + routing) as one bundle.
  async function exportConfig() {
    try {
      const bundle = await fetchConfigBundle();
      downloadText("agezt-config.json", JSON.stringify(bundle, null, 2), "application/json");
    } catch (e) {
      ui.toast(`Export failed: ${(e as Error).message}`, "error");
    }
  }

  // Restore a daemon-config bundle: apply each section it carries to the daemon.
  async function importConfigFile(file: File) {
    try {
      const applied = await applyConfigBundle(parseConfigBundle(await file.text()));
      ui.toast(`Config imported: ${applied.join(", ")}`, "success");
    } catch (e) {
      ui.toast(`Import failed: ${(e as Error).message}`, "error");
    }
  }

  // Deep-linkable views: setActive also reflects into the URL hash, so views are
  // bookmarkable and the browser back/forward buttons move between them.
  const setActive = (id: string) => {
    setActiveRaw(id);
    setAgentSlug(null); // leaving any agent detail route for a normal nav view
    setIncidentId(null); // leaving any incident detail route for a normal nav view
    // goToView is a no-op when the destination already matches the current
    // hash (and that includes the strip-`#` normalisation this branch used
    // to do inline) — so dropping the manual equality check is safe.
    goToView(id);
  };
  // Sync when the hash changes externally (back/forward, manual edit, openAgent).
  useEffect(() => {
    function onHash() {
      setActiveRaw(viewFromHash());
      setAgentSlug(agentSlugFromHash(location.hash));
      setIncidentId(incidentIdFromHash(location.hash));
      setHashKey(location.hash);
    }
    window.addEventListener("hashchange", onHash);
    return () => window.removeEventListener("hashchange", onHash);
  }, []);

  // ⌘K / Ctrl+K opens the command palette from anywhere; "?" toggles the help
  // drawer — but never while the operator is typing in a field.
  useEffect(() => {
    function onKey(e: KeyboardEvent) {
      if ((e.metaKey || e.ctrlKey) && e.key.toLowerCase() === "k") {
        e.preventDefault();
        setPaletteOpen((o) => !o);
        return;
      }
      if (e.key === "?" && !e.metaKey && !e.ctrlKey && !e.altKey) {
        const t = e.target as HTMLElement | null;
        const typing =
          t instanceof HTMLInputElement || t instanceof HTMLTextAreaElement || !!t?.isContentEditable;
        if (!typing) {
          e.preventDefault();
          setHelpOpen((o) => !o);
        }
      }
    }
    window.addEventListener("keydown", onKey);
    return () => window.removeEventListener("keydown", onKey);
  }, []);

  // Refresh the recent-runs list each time the palette opens, so "Open run …"
  // commands reflect what just happened. Best-effort — a fetch failure just leaves
  // the previous list (or none).
  useEffect(() => {
    if (!paletteOpen) return;
    let live = true;
    getJSON<{ runs?: { correlation_id?: string; intent?: string; status?: string }[] }>("/api/runs")
      .then((d) => {
        if (live) setRecentRuns((d.runs || []).filter((r) => r.correlation_id).slice(0, 8));
      })
      .catch(() => {});
    getJSON<{ profiles?: { slug: string; name?: string; system?: boolean; retired?: boolean }[] }>("/api/agents")
      .then((d) => {
        if (live) setPaletteAgents((d.profiles || []).filter((p) => p.slug));
      })
      .catch(() => {});
    return () => {
      live = false;
    };
  }, [paletteOpen]);

  const commands = useMemo<CommandItem[]>(() => {
    const views: CommandItem[] = NAV.map((n) => ({
      id: `view-${n.id}`,
      label: n.label,
      group: sectionForView[n.id] || "Go to",
      run: () => setActive(n.id),
    }));
    const actions: CommandItem[] = [
      {
        id: "act-new-chat",
        label: "New chat",
        group: "Action",
        keywords: "conversation thread compose ask message",
        run: () => {
          newChat();
          setActive("chat");
        },
      },
      {
        id: "act-halt",
        label: "Halt all runs",
        group: "Action",
        keywords: "freeze stop emergency",
        run: async () => {
          const ok = await ui.confirm({
            title: "Freeze all in-flight runs?",
            message: "Every running and queued run is paused until you resume.",
            confirmLabel: "Halt",
            danger: true,
          });
          if (ok) {
            try {
              await postAction("/api/halt");
              ui.toast("All runs halted", "success");
            } catch (e) {
              ui.toast((e as Error).message, "error");
            }
          }
        },
      },
      {
        id: "act-resume",
        label: "Resume",
        group: "Action",
        keywords: "unpause continue",
        run: () =>
          postAction("/api/resume")
            .then(() => ui.toast("Resumed", "success"))
            .catch((e) => ui.toast((e as Error).message, "error")),
      },
      {
        id: "act-help",
        label: "Help for this page",
        group: "Action",
        keywords: "guide manual docs documentation how to explain",
        run: () => setHelpOpen(true),
      },
      {
        id: "act-theme",
        label: "Toggle theme",
        group: "Action",
        keywords: "dark light appearance",
        run: () => toggleTheme(),
      },
      {
        id: "act-advanced",
        label: "Toggle Advanced mode",
        group: "Action",
        keywords: "calm detail diagnostics expert verbose simple",
        run: () => toggleAdvanced(),
      },
      {
        id: "act-appearance-export",
        label: "Export appearance settings",
        group: "Action",
        keywords: "backup theme accent console name download settings",
        run: () => downloadText("agezt-appearance.json", JSON.stringify(exportAppearance(), null, 2), "application/json"),
      },
      {
        id: "act-appearance-import",
        label: "Import appearance settings",
        group: "Action",
        keywords: "restore theme accent console name upload settings",
        run: () => appearanceFileRef.current?.click(),
      },
      {
        id: "act-config-export",
        label: "Export configuration (default identity, prompt templates, routing)",
        group: "Action",
        keywords: "backup config identity prompt templates routing download daemon profile",
        run: () => void exportConfig(),
      },
      {
        id: "act-config-import",
        label: "Import configuration (default identity, prompt templates, routing)",
        group: "Action",
        keywords: "restore config identity prompt templates routing upload daemon profile",
        run: () => configFileRef.current?.click(),
      },
    ];
    // Recent runs → "Open run …" — jump straight to a run's detail from ⌘K.
    const runCmds: CommandItem[] = recentRuns.map((r) => ({
      id: `run-${r.correlation_id}`,
      label: r.intent?.trim() || r.correlation_id || "run",
      group: "Open run",
      keywords: `run ${r.status || ""} ${r.correlation_id || ""}`,
      run: () => {
        focusRun(r.correlation_id!);
        setActive("runs");
      },
    }));
    // Roster agents → "Open agent …" — jump straight to any agent's identity
    // page from ⌘K (M967). Guardians and graveyard agents are tagged in keywords
    // so they're searchable too.
    const agentCmds: CommandItem[] = paletteAgents.map((a) => ({
      id: `agent-${a.slug}`,
      label: a.name && a.name !== a.slug ? `${a.name} (${a.slug})` : a.slug,
      group: "Open agent",
      keywords: `agent ${a.slug} ${a.name || ""} ${a.system ? "guardian system" : ""} ${a.retired ? "retired graveyard" : ""}`,
      run: () => openAgent(a.slug),
    }));
    return [...views, ...actions, ...agentCmds, ...runCmds];
    // eslint-disable-next-line react-hooks/exhaustive-deps
  }, [ui, newChat, recentRuns, paletteAgents]);

  return (
    <TooltipProvider delayDuration={200}>
    <div className="flex h-full flex-col relative">
      <CommandPalette open={paletteOpen} onClose={() => setPaletteOpen(false)} items={commands} />
      <HelpDrawer
        open={helpOpen}
        viewId={agentSlug ? "agent" : active}
        group={agentSlug ? "Agents" : sectionForView[active]}
        icon={agentSlug ? Bot : current.icon}
        onClose={() => setHelpOpen(false)}
        onNavigate={setActive}
      />
      {needsSetup && (
        <Suspense fallback={<RouteLoading label="Setup" />}>
          <Setup
            overlay
            onDone={() => {
              setNeedsSetup(false);
              setActive("chat");
            }}
            onSkip={() => {
              localStorage.setItem("agezt.setup.skipped", "1");
              setNeedsSetup(false);
            }}
          />
        </Suspense>
      )}
      <input
        ref={appearanceFileRef}
        type="file"
        accept="application/json,.json"
        className="hidden"
        aria-hidden="true"
        onChange={(e) => {
          const f = e.target.files?.[0];
          if (f) void importAppearanceFile(f);
          e.target.value = "";
        }}
      />
      <input
        ref={configFileRef}
        type="file"
        accept="application/json,.json"
        className="hidden"
        aria-hidden="true"
        onChange={(e) => {
          const f = e.target.files?.[0];
          if (f) void importConfigFile(f);
          e.target.value = "";
        }}
      />
      <MiniChat hidden={active === "chat"} onExpand={() => setActive("chat")} />
      <Header
        connected={connected}
        chatActive={active === "chat" && !agentSlug}
        activeRunCount={activeRunCount}
        inspectorOpen={inspectorOpen}
        activeLlmCount={activeLlmCount}
        onNavigate={setActive}
        onOpenNav={() => setNavDrawerOpen(true)}
        onOpenChat={() => setActive("chat")}
        onOpenPalette={() => setPaletteOpen(true)}
        onOpenHelp={() => setHelpOpen(true)}
        onToggleInspector={() => setInspectorOpen((v) => !v)}
      />
      <Vitals onNavigate={setActive} />
      <FleetNowBar onNavigate={setActive} />
      <div className="flex min-h-0 flex-1 flex-col lg:flex-row">
        {/* Two-level nav (M974): a big-icon section RAIL then a secondary LIST
            of that section's views. On lg+ it is a fixed sidebar; below lg it is
            hidden and reached through the hamburger → drawer (M1002). */}
        <nav className="hidden shrink-0 lg:flex lg:border-r lg:border-border">
          <SectionNav
            navSection={navSection}
            setNavSection={setNavSection}
            activeGroupId={activeGroupId}
            shownGroup={shownGroup}
            active={active}
            onSelect={setActive}
            unseenAlerts={unseenAlerts}
            activeRunCount={activeRunCount}
            build={build}
          />
        </nav>
        <main className="min-h-0 flex-1 overflow-auto p-3 pb-7 sm:p-4 sm:pb-7">
          {/* Keyed remount so each view fades + rises in on navigation. The
              `#agent/<slug>` detail route (M960) takes over the main area. */}
          <div
            key={incidentId ? `incident/${incidentId}` : agentSlug ? `agent/${agentSlug}` : hashKey || active}
            className="view-enter h-full"
          >
            <Suspense fallback={<RouteLoading label={incidentId ? "Incident" : agentSlug ? "Agent" : current.label} />}>
              {incidentId ? (
                <IncidentPage incidentId={incidentId} onNavigate={setActive} />
              ) : agentSlug ? (
                <AgentPage slug={agentSlug} onNavigate={setActive} />
              ) : (
                <View />
              )}
            </Suspense>
          </div>
        </main>
      </div>
    </div>
    {/* Mobile nav drawer (below lg): slides in from the left over a fading
        overlay; tapping a view navigates and closes it. */}
    {navDrawerOpen && (
      <div className="fixed inset-0 z-[120] lg:hidden" role="dialog" aria-modal="true" aria-label="Navigation">
        <div className="modal-overlay absolute inset-0 bg-black/40 backdrop-blur-[2px]" onClick={() => setNavDrawerOpen(false)} />
        <div ref={navDrawerRef} className="nav-drawer-in absolute inset-y-0 left-0 flex max-w-[85vw] flex-col border-r border-border bg-background shadow-2xl shadow-black/40">
          <div className="flex items-center justify-between border-b border-border px-3 py-2">
            <ConsoleName />
            <button
              onClick={() => setNavDrawerOpen(false)}
              className="inline-flex size-8 items-center justify-center rounded-md text-muted transition-colors hover:bg-panel hover:text-foreground"
              aria-label="Close navigation menu"
            >
              <X className="size-5" />
            </button>
          </div>
          <SectionNav
            navSection={navSection}
            setNavSection={setNavSection}
            activeGroupId={activeGroupId}
            shownGroup={shownGroup}
            active={active}
            onSelect={(id) => {
              setActive(id);
              setNavDrawerOpen(false);
            }}
            unseenAlerts={unseenAlerts}
            activeRunCount={activeRunCount}
            build={build}
          />
        </div>
      </div>
    )}
    {inspectorOpen ? (
      <div className="absolute bottom-0 left-0 right-0 z-40 flex flex-col">
        <Inspector open onClose={() => setInspectorOpen(false)} />
      </div>
    ) : (
      <div className="absolute bottom-0 left-0 right-0 z-40">
        <InspectorClosedBar onOpen={() => setInspectorOpen(true)} activeLlmCount={activeLlmCount} />
      </div>
    )}
    </TooltipProvider>
  );
}

function RouteLoading({ label }: { label: string }) {
  return (
    <div className="flex h-full min-h-[240px] items-center justify-center">
      <div className="inline-flex items-center gap-2 rounded-lg bg-card px-3 py-2 text-sm text-muted shadow-e1">
        <RefreshCw className="size-4 animate-spin" />
        Loading {label}...
      </div>
    </div>
  );
}
