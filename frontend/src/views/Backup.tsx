import { useRef, useState } from "react";
import { Archive, Download, Upload, Palette, Server } from "lucide-react";
import { Button } from "@/components/ui/button";
import { useUI } from "@/components/ui/feedback";
import { downloadText } from "@/lib/export";
import { exportAppearance, parseAppearanceJSON, applyAppearanceBundle } from "@/lib/appearance";
import { parseConfigBundle, fetchConfigBundle, applyConfigBundle } from "@/lib/configbackup";

// Backup is the discoverable home for the export/import features that otherwise live
// only behind ⌘K: the per-device appearance bundle (M735) and the daemon-side config
// bundle (M738). Two cards, each with Export (download) + Import (file) buttons, so a
// user who doesn't know the palette commands can still carry their console elsewhere.
export function Backup() {
  const ui = useUI();
  const appearanceRef = useRef<HTMLInputElement>(null);
  const configRef = useRef<HTMLInputElement>(null);
  const [busy, setBusy] = useState<"config-export" | "config-import" | null>(null);

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

  async function importConfigFile(file: File) {
    setBusy("config-import");
    try {
      const applied = await applyConfigBundle(parseConfigBundle(await file.text()));
      ui.toast(`Config imported: ${applied.join(", ")}`, "success");
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
        <div className="mt-2 flex items-center gap-2">
          <Button size="sm" variant="ghost" onClick={exportConfigFile} disabled={busy === "config-export"}>
            <Download className="size-3.5" /> Export
          </Button>
          <Button size="sm" variant="ghost" onClick={() => configRef.current?.click()} disabled={busy === "config-import"}>
            <Upload className="size-3.5" /> Import
          </Button>
        </div>
      </div>

      <p className="text-[11px] text-muted">
        These same actions are available from the command palette (<kbd className="rounded border border-border px-1">⌘K</kbd>)
        as “Export / Import appearance settings” and “… configuration”.
      </p>
    </div>
  );
}
