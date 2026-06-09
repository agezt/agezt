import { useCallback, useEffect, useRef, useState } from "react";
import { Archive, Download, Upload, Palette, Server, Info, Camera } from "lucide-react";
import { Button } from "@/components/ui/button";
import { useUI } from "@/components/ui/feedback";
import { downloadText } from "@/lib/export";
import { exportAppearance, parseAppearanceJSON, applyAppearanceBundle } from "@/lib/appearance";
import { parseConfigBundle, fetchConfigBundle, applyConfigBundle } from "@/lib/configbackup";
import { fetchFullSnapshot, snapshotCounts } from "@/lib/snapshot";

// configSummary describes what a daemon-config bundle currently holds — shown so you
// know what you're about to export (and what an import will replace).
export function configSummary(c: { persona: string; prompts: unknown[]; chains: Record<string, string[]> }): string {
  const parts = [
    c.persona.trim() ? "persona set" : "no persona",
    `${c.prompts.length} prompt${c.prompts.length === 1 ? "" : "s"}`,
    `${Object.keys(c.chains).length} routing chain${Object.keys(c.chains).length === 1 ? "" : "s"}`,
  ];
  return parts.join(" · ");
}

// Backup is the discoverable home for the export/import features that otherwise live
// only behind ⌘K: the per-device appearance bundle (M735) and the daemon-side config
// bundle (M738). Two cards, each with Export (download) + Import (file) buttons, so a
// user who doesn't know the palette commands can still carry their console elsewhere.
export function Backup() {
  const ui = useUI();
  const appearanceRef = useRef<HTMLInputElement>(null);
  const configRef = useRef<HTMLInputElement>(null);
  const [busy, setBusy] = useState<"config-export" | "config-import" | "snapshot" | null>(null);
  const [summary, setSummary] = useState<string | null>(null);

  const refreshSummary = useCallback(async () => {
    try {
      const b = await fetchConfigBundle();
      setSummary(configSummary(b.config));
    } catch {
      setSummary(null);
    }
  }, []);
  useEffect(() => {
    void refreshSummary();
  }, [refreshSummary]);

  function exportAppearanceFile() {
    downloadText("agezt-appearance.json", JSON.stringify(exportAppearance(), null, 2), "application/json");
  }

  async function importAppearanceFile(file: File) {
    try {
      const bundle = parseAppearanceJSON(await file.text());
      applyAppearanceBundle(bundle);
      ui.toast(`Appearance imported (${Object.keys(bundle).join(", ")})`, "success");
    } catch (e) {
      ui.toast(`Import failed: ${(e as Error).message}`, "error");
    }
  }

  async function exportConfigFile() {
    setBusy("config-export");
    try {
      downloadText("agezt-config.json", JSON.stringify(await fetchConfigBundle(), null, 2), "application/json");
    } catch (e) {
      ui.toast(`Export failed: ${(e as Error).message}`, "error");
    } finally {
      setBusy(null);
    }
  }

  async function exportSnapshot() {
    setBusy("snapshot");
    try {
      const snap = await fetchFullSnapshot();
      downloadText("agezt-snapshot.json", JSON.stringify(snap, null, 2), "application/json");
      ui.toast(`Snapshot: ${snapshotCounts(snap)}`, "success");
    } catch (e) {
      ui.toast(`Snapshot failed: ${(e as Error).message}`, "error");
    } finally {
      setBusy(null);
    }
  }

  async function importConfigFile(file: File) {
    setBusy("config-import");
    try {
      const applied = await applyConfigBundle(parseConfigBundle(await file.text()));
      ui.toast(`Config imported: ${applied.join(", ")}`, "success");
      void refreshSummary();
    } catch (e) {
      ui.toast(`Import failed: ${(e as Error).message}`, "error");
    } finally {
      setBusy(null);
    }
  }

  return (
    <div className="space-y-4">
      <div className="flex items-center gap-2">
        <h2 className="flex items-center gap-2 text-sm font-semibold">
          <Archive className="size-4 text-accent" /> Backup &amp; Restore
        </h2>
        <span className="text-xs text-muted">carry your console to another browser or daemon</span>
      </div>

      <input
        ref={appearanceRef}
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
        ref={configRef}
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

      <div className="rounded-lg border border-border bg-card p-3">
        <h3 className="flex items-center gap-2 text-sm font-semibold">
          <Palette className="size-4 text-accent" /> Appearance
        </h3>
        <p className="mt-1 text-xs text-muted">
          Theme, accent colour and console name. A per-device preference (stored in this browser).
        </p>
        <div className="mt-2 flex items-center gap-2">
          <Button size="sm" variant="ghost" onClick={exportAppearanceFile}>
            <Download className="size-3.5" /> Export
          </Button>
          <Button size="sm" variant="ghost" onClick={() => appearanceRef.current?.click()}>
            <Upload className="size-3.5" /> Import
          </Button>
        </div>
      </div>

      <div className="rounded-lg border border-border bg-card p-3">
        <h3 className="flex items-center gap-2 text-sm font-semibold">
          <Server className="size-4 text-accent" /> Daemon configuration
        </h3>
        <p className="mt-1 text-xs text-muted">
          Global persona, the prompt library and per-task routing chains. Lives on the daemon — importing
          replaces them on whichever daemon this console is connected to.
        </p>
        {summary && (
          <p className="mt-1.5 inline-flex items-center gap-1.5 rounded-md bg-panel px-2 py-1 text-[11px] text-foreground/80">
            <Info className="size-3 text-accent" /> currently: {summary}
          </p>
        )}
        <div className="mt-2 flex items-center gap-2">
          <Button size="sm" variant="ghost" onClick={exportConfigFile} disabled={busy === "config-export"}>
            <Download className="size-3.5" /> Export
          </Button>
          <Button size="sm" variant="ghost" onClick={() => configRef.current?.click()} disabled={busy === "config-import"}>
            <Upload className="size-3.5" /> Import
          </Button>
        </div>
      </div>

      <div className="rounded-lg border border-border bg-card p-3">
        <h3 className="flex items-center gap-2 text-sm font-semibold">
          <Camera className="size-4 text-accent" /> Full snapshot
          <span className="rounded bg-panel px-1.5 py-0.5 text-[10px] font-medium uppercase tracking-wide text-muted">
            read-only
          </span>
        </h3>
        <p className="mt-1 text-xs text-muted">
          A complete record of everything customizable — persona, prompts, routing, standing orders, schedules, memory and
          the world model — in one file, for backup, audit or planning a migration. Export-only: restoring is per-domain.
        </p>
        <div className="mt-2 flex items-center gap-2">
          <Button size="sm" variant="ghost" onClick={exportSnapshot} disabled={busy === "snapshot"}>
            <Download className="size-3.5" /> Export snapshot
          </Button>
        </div>
      </div>

      <p className="text-[11px] text-muted">
        Scope: the config bundle covers <span className="text-foreground/70">persona, prompts and routing</span> only.
        Standing orders, schedules, memory and the world model aren’t included — manage those from their own views. The
        same Export/Import actions are also in the command palette (<kbd className="rounded border border-border px-1">⌘K</kbd>).
      </p>
    </div>
  );
}
