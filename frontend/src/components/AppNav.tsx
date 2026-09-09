import { useState } from "react";
import {
  MessageSquare,
  Pause,
  Play,
  Search,
  Bug,
  HelpCircle,
  Menu,
} from "lucide-react";
import { cn } from "@/app/utils";
import { postAction } from "@/app/api";
import { ConnectionChip } from "@/components/ConnectionChip";
import { AlertBell } from "@/components/AlertBell";
import { ApprovalsBell } from "@/components/ApprovalsBell";
import { NotifyToggle } from "@/components/NotifyToggle";
import { ActivityChip } from "@/components/ActivityChip";
import { useUI, type ConfirmOptions } from "@/components/ui/feedback";

type ConfirmRequest = ConfirmOptions;
import { ThemeToggle } from "@/components/ThemeToggle";
import { AdvancedToggle } from "@/components/AdvancedToggle";
import { AccentPicker } from "@/components/AccentPicker";
import { ConsoleName } from "@/components/ConsoleName";
import { NAV_GROUPS, groupForView, rowForView, type NavGroup } from "@/nav";

// Per-section accent hue (M979) — each section gets its own colour so the nav
// reads as a vivid, navigable map rather than one flat grey list. Used for the
// rail icon tint, the active section pill, and the item active state.
const SECTION_HUE: Record<string, number> = {
  talk: 255, // blue
  observe: 150, // green
  automate: 55, // amber
  govern: 25, // red-orange
  fleet: 290, // violet
  knowledge: 195, // cyan
  connect: 215, // azure
  admin: 230, // indigo
};
const sectionHue = (id: string) => SECTION_HUE[id] ?? 255;

// SectionNav renders the two-level navigation (section icon rail + the selected
// section's item list). It is used in BOTH the lg+ sidebar and the mobile
// drawer, so it always lays out vertically; the parent decides visibility.
export function SectionNav({
  navSection,
  setNavSection,
  activeGroupId,
  shownGroup,
  active,
  onSelect,
  unseenAlerts,
  activeRunCount,
  build,
}: {
  navSection: string;
  setNavSection: (id: string) => void;
  activeGroupId: string;
  shownGroup: NavGroup;
  active: string;
  onSelect: (id: string) => void;
  unseenAlerts: number;
  activeRunCount: number;
  build: { version?: string; revision?: string; built?: string; build_modified?: boolean } | null;
}) {
  return (
    <div className="flex min-h-0 flex-1">
      {/* Section rail — a vertical column of colourful, glassy section icons. */}
      <div className="flex shrink-0 flex-col gap-1.5 overflow-y-auto border-r border-border p-1.5">
        {NAV_GROUPS.map((g) => {
          const on = navSection === g.id;
          const isActiveSection = activeGroupId === g.id;
          const hue = sectionHue(g.id);
          const sectionBadge =
            (g.id === "observe" ? unseenAlerts : 0) + (g.id === "govern" ? activeRunCount : 0);
          return (
            <button
              key={g.id}
              onClick={() => setNavSection(g.id)}
              title={g.label}
              aria-label={g.label}
              className={cn(
                "relative flex size-11 shrink-0 flex-col items-center justify-center gap-0 rounded-xl transition-all duration-150",
                on ? "scale-[1.04] shadow-e2" : "hover:scale-[1.02] hover:bg-panel",
              )}
              style={
                on
                  ? {
                      background: `linear-gradient(155deg, oklch(0.65 0.18 ${hue} / 0.35), oklch(0.6 0.16 ${hue} / 0.08))`,
                      color: `oklch(0.62 0.18 ${hue})`,
                      boxShadow: `inset 0 0 0 1px oklch(0.62 0.16 ${hue} / 0.5), 0 0 20px -4px oklch(0.6 0.16 ${hue} / 0.4)`,
                    }
                  : { color: `oklch(0.6 0.14 ${hue})` }
              }
            >
              <g.icon className="size-5" />
              <span className="text-[9px] font-semibold leading-none tracking-normal">{g.label}</span>
              {isActiveSection && !on && (
                <span className="absolute right-1.5 top-1.5 size-1.5 rounded-full" style={{ background: `oklch(0.62 0.16 ${hue})` }} />
              )}
              {sectionBadge > 0 && (
                <span className="absolute -right-0.5 -top-0.5 inline-flex min-w-3.5 items-center justify-center rounded-full bg-bad px-0.5 text-[9px] font-bold leading-3.5 text-white">
                  {sectionBadge > 99 ? "99+" : sectionBadge}
                </span>
              )}
            </button>
          );
        })}
      </div>

      {/* Secondary item list for the selected section */}
      <div className="flex w-44 shrink-0 flex-col gap-0.5 overflow-y-auto p-2">
        <div
          className="flex items-center gap-1.5 px-2 pb-1.5 pt-1 text-xs font-bold uppercase tracking-normal"
          style={{ color: `oklch(0.6 0.14 ${sectionHue(shownGroup.id)})` }}
        >
          <shownGroup.icon className="size-3.5" />
          {shownGroup.label}
        </div>
        {shownGroup.rows.map((n) => {
          const hue = sectionHue(shownGroup.id);
          // A row is active when the active VIEW is one of its tabs, so a deep
          // link into any facet (`#models`) lights up its destination row.
          const isOn = rowForView[active]?.id === n.id;
          return (
            <button
              key={n.id}
              // Selecting a row lands on its first view; the row's tab strip
              // takes over from there.
              onClick={() => onSelect(n.views[0].id)}
              // Name the facets a row holds. The sidebar shows the destination,
              // so a view folded into it is a word the operator can no longer
              // see — and "where did Roster go?" is what that costs.
              title={n.views.length > 1 ? `${n.label}: ${n.views.map((v) => v.label).join(" · ")}` : n.label}
              className={cn(
                "relative flex shrink-0 items-center gap-2 rounded-lg px-3 py-2 text-left text-sm transition-all duration-150",
                isOn
                  ? "font-semibold before:absolute before:left-0 before:top-1/2 before:block before:h-5 before:w-[3px] before:-translate-y-1/2 before:rounded-r-full before:content-['']"
                  : "text-muted hover:bg-panel hover:text-foreground",
              )}
              style={
                isOn
                  ? {
                      background: `linear-gradient(90deg, oklch(0.6 0.16 ${hue} / 0.2), oklch(0.6 0.16 ${hue} / 0.03))`,
                      color: `oklch(0.56 0.16 ${hue})`,
                    }
                  : undefined
              }
            >
              {isOn && <span className="absolute left-0 top-1/2 block h-5 w-[3px] -translate-y-1/2 rounded-r-full" style={{ background: `oklch(0.62 0.16 ${hue})` }} />}
              <n.icon className="size-4 shrink-0" />
              <span>{n.label}</span>
              {n.id === "alerts" && unseenAlerts > 0 && (
                <span
                  className="ml-auto inline-flex min-w-4 items-center justify-center rounded-full bg-bad px-1 text-xs font-semibold leading-4 text-white"
                  title={`${unseenAlerts} new alert${unseenAlerts === 1 ? "" : "s"} — the agent flagged something`}
                  aria-label={`${unseenAlerts} unseen alerts`}
                >
                  {unseenAlerts > 99 ? "99+" : unseenAlerts}
                </span>
              )}
              {n.id === "oversight" && activeRunCount > 0 && (
                <span
                  className="ml-auto inline-flex min-w-4 items-center justify-center rounded-full bg-accent/20 px-1 text-xs font-semibold leading-4 text-accent"
                  title={`${activeRunCount} run${activeRunCount === 1 ? "" : "s"} in flight`}
                  aria-label={`${activeRunCount} active runs`}
                >
                  {activeRunCount > 99 ? "99+" : activeRunCount}
                </span>
              )}
            </button>
          );
        })}
        {build && (
          <div
            className="mt-auto px-3 pt-3 text-xs leading-tight text-muted/60"
            title={
              build.revision
                ? `Daemon build ${build.revision}${build.build_modified ? " (modified working tree)" : ""}${build.built ? ` · built ${build.built}` : ""}`
                : build.built
                  ? `Daemon built ${build.built}`
                  : "Build revision unavailable (binary built without VCS info)"
            }
          >
            v{build.version || "?"}
            {build.revision ? (
              <> · {build.revision.slice(0, 7)}{build.build_modified ? "+" : ""}</>
            ) : null}
          </div>
        )}
      </div>
    </div>
  );
}

// ViewTabs renders a nav row's facets as a tab strip above the view. Each tab
// navigates to that view's OWN hash — the row is presentation, the view id
// stays the address — so bookmarks, ⌘K, help topics and every
// `location.hash = "runs"` call site keep working unchanged. Renders nothing
// for a single-view row.
export function ViewTabs({
  active,
  onSelect,
}: {
  active: string;
  onSelect: (id: string) => void;
}) {
  const row = rowForView[active];
  if (!row || row.views.length < 2) return null;
  const hue = sectionHue(groupForView[active] || "talk");
  return (
    // Chrome, not content. These tabs NAVIGATE (each sets its view's own hash)
    // and they sit above the page title, so drawing them as a bordered panel
    // made them read as a second in-page tab bar — Dashboard ended up with two
    // near-identical strips, both starting with "Overview". A light underlined
    // row reads as part of the shell instead.
    <div
      className="-mt-0.5 mb-2 flex flex-wrap items-center gap-0.5 border-b border-border/70 pb-1.5"
      role="tablist"
      aria-label={`${row.label} sections`}
    >
      {row.views.map((v) => {
        const on = v.id === active;
        return (
          <button
            key={v.id}
            role="tab"
            aria-selected={on}
            onClick={() => onSelect(v.id)}
            className={cn(
              "relative inline-flex items-center gap-1.5 rounded-md px-2.5 py-1 text-xs transition-colors duration-150",
              "outline-none focus-visible:ring-2 focus-visible:ring-accent/50",
              on ? "font-semibold" : "text-muted hover:bg-panel/60 hover:text-foreground",
            )}
            style={on ? { color: `oklch(0.56 0.16 ${hue})` } : undefined}
          >
            <v.icon className="size-3.5" aria-hidden />
            {v.label}
            {on && (
              <span
                className="absolute inset-x-1.5 -bottom-1.5 h-0.5 rounded-full"
                style={{ background: `oklch(0.62 0.16 ${hue})` }}
              />
            )}
          </button>
        );
      })}
    </div>
  );
}

export function Header({
  connected,
  chatActive,
  activeRunCount,
  inspectorOpen,
  activeLlmCount,
  onNavigate,
  onOpenNav,
  onOpenChat,
  onOpenPalette,
  onOpenHelp,
  onToggleInspector,
}: {
  connected: boolean;
  chatActive: boolean;
  activeRunCount: number;
  inspectorOpen: boolean;
  activeLlmCount: number;
  onNavigate: (id: string) => void;
  onOpenNav: () => void;
  onOpenChat: () => void;
  onOpenPalette: () => void;
  onOpenHelp: () => void;
  onToggleInspector: () => void;
}) {
  const [busy, setBusy] = useState(false);
  const ui = useUI();
  async function act(path: string, opts?: { confirm?: ConfirmRequest; success?: string }) {
    if (opts?.confirm && !(await ui.confirm(opts.confirm))) return;
    setBusy(true);
    try {
      await postAction(path);
      if (opts?.success) ui.toast(opts.success, "success");
    } catch (e) {
      ui.toast(`${path}: ${(e as Error).message}`, "error");
    } finally {
      setBusy(false);
    }
  }
  return (
    <header className="relative z-10 flex flex-wrap items-center gap-2 border-b border-border bg-panel/75 px-3 py-2 shadow-e1 backdrop-blur-md sm:gap-3 sm:px-4">
      {/* Lit accent edge under the header (M977 command-center). */}
      <div className="accent-rule pointer-events-none absolute inset-x-0 bottom-0 h-px" />
      {/* Hamburger — opens the nav drawer below lg, where the sidebar is hidden. */}
      <button
        onClick={onOpenNav}
        className="inline-flex size-8 items-center justify-center rounded-md text-muted transition-colors hover:bg-panel hover:text-foreground lg:hidden"
        title="Menu"
        aria-label="Open navigation menu"
      >
        <Menu className="size-5" />
      </button>
      <ConsoleName />
      <ConnectionChip />
      {/* Always-visible "something is working" chip: lights up whenever a run is
          in flight anywhere (chat reply, tool call, autonomous agent), so the
          background never reads as a frozen screen. */}
      <ActivityChip count={activeRunCount} onClick={() => onNavigate("overseer")} />
      <div className="ml-auto flex min-w-0 flex-wrap items-center justify-end gap-1.5 sm:gap-2">
        {/* Always-on Chat button (M985): the chat surface is the product's core
            ([[product-layer-priority]]), but the two-level nav buries it two
            clicks deep from any other section. This jumps there from anywhere. */}
        <button
          onClick={onOpenChat}
          aria-current={chatActive ? "page" : undefined}
          className={cn(
            "inline-flex h-8 items-center gap-1.5 rounded-md px-3 text-sm font-medium transition-colors",
            chatActive
              ? "bg-gradient-to-br from-accent to-accent2 text-white shadow-e1"
              : "border border-accent/40 text-accent hover:bg-accent hover:text-white",
          )}
          title="Go to Chat"
        >
          <MessageSquare className="size-4" />
          <span className="hidden sm:inline">Chat</span>
        </button>
        <NotifyToggle />
        <ApprovalsBell />
        <AlertBell />
        <button
          onClick={onOpenPalette}
          className="hidden h-8 items-center gap-1.5 rounded-md border border-border px-2.5 text-xs text-muted transition-colors hover:border-accent hover:text-foreground sm:inline-flex"
          title="Command palette"
        >
          <Search className="size-3.5" />
          <kbd className="rounded border border-border px-1 text-xs">⌘K</kbd>
        </button>
        <button
          onClick={onOpenHelp}
          className="inline-flex h-8 items-center gap-1.5 rounded-md border border-border px-2.5 text-xs text-muted transition-colors hover:border-accent hover:text-foreground"
          title="Help for this page (?)"
          aria-label="Help for this page"
        >
          <HelpCircle className="size-3.5" />
          <span className="hidden sm:inline">Help</span>
        </button>
        <button
          onClick={() =>
            act("/api/halt", {
              confirm: {
                title: "Freeze all in-flight runs?",
                message: "Every running and queued run is paused until you resume. Use this to stop the daemon fast.",
                confirmLabel: "Halt",
                danger: true,
              },
              success: "All runs halted",
            })
          }
          disabled={busy}
          className="inline-flex h-8 items-center gap-1.5 rounded-md border border-bad px-2.5 text-sm text-bad transition-colors hover:bg-bad hover:text-white disabled:opacity-50 sm:px-3"
          aria-label="Halt all runs"
        >
          <Pause className="size-4" /> <span className="hidden sm:inline">Halt</span>
        </button>
        <button
          onClick={() => act("/api/resume", { success: "Resumed" })}
          disabled={busy}
          className="inline-flex h-8 items-center gap-1.5 rounded-md border border-border px-2.5 text-sm transition-colors hover:border-accent disabled:opacity-50 sm:px-3"
          aria-label="Resume all runs"
        >
          <Play className="size-4" /> <span className="hidden sm:inline">Resume</span>
        </button>
        <button
          onClick={onToggleInspector}
          title={`Debug inspector (Ctrl+Shift+I) — ${inspectorOpen ? "close" : "open"}`}
          className={cn(
            "relative inline-flex h-8 items-center gap-1.5 rounded-md px-2.5 text-sm transition-colors",
            inspectorOpen
              ? "bg-accent/15 text-accent"
              : "text-muted hover:bg-panel hover:text-foreground",
          )}
        >
          <Bug className="size-4" />
          {activeLlmCount > 0 && (
            <span className="absolute -right-1 -top-1 inline-flex min-w-3.5 items-center justify-center rounded-full bg-accent/20 px-0.5 text-[9px] font-bold leading-3.5 text-accent">
              {activeLlmCount > 9 ? "9+" : activeLlmCount}
            </span>
          )}
        </button>
        <AccentPicker />
        <AdvancedToggle />
        <ThemeToggle />
      </div>
    </header>
  );
}
