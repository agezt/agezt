import { useEffect, useRef, useState } from "react";
import { Palette, Check } from "lucide-react";
import { Button } from "@/components/ui/button";
import { ACCENTS, swatchColor, useAccent } from "@/lib/accent";

// AccentPicker is the appearance knob in the header: a palette button that opens a
// popover of accent-colour swatches. Picking one recolours the whole UI instantly
// (it only shifts the accent hue; light/dark lightness is preserved) and persists.
export function AccentPicker() {
  const { hue, setHue } = useAccent();
  const [open, setOpen] = useState(false);
  const ref = useRef<HTMLDivElement>(null);

  useEffect(() => {
    if (!open) return;
    function onDown(e: MouseEvent) {
      if (ref.current && !ref.current.contains(e.target as Node)) setOpen(false);
    }
    function onKey(e: KeyboardEvent) {
      if (e.key === "Escape") setOpen(false);
    }
    window.addEventListener("mousedown", onDown);
    window.addEventListener("keydown", onKey);
    return () => {
      window.removeEventListener("mousedown", onDown);
      window.removeEventListener("keydown", onKey);
    };
  }, [open]);

  return (
    <div ref={ref} className="relative">
      <Button variant="ghost" size="icon" onClick={() => setOpen((o) => !o)} title="Accent colour" aria-label="Accent colour">
        <Palette />
      </Button>
      {open && (
        <div
          className="absolute right-0 top-full z-50 mt-1 w-44 rounded-lg border border-border bg-card p-2 shadow-xl shadow-black/30"
          role="dialog"
          aria-label="Accent colour"
        >
          <div className="mb-1.5 px-1 text-[10px] font-semibold uppercase tracking-normal text-muted">Accent</div>
          <div className="grid grid-cols-5 gap-1.5">
            {ACCENTS.map((a) => {
              const active = a.hue === hue;
              return (
                <button
                  key={a.name}
                  onClick={() => setHue(a.hue)}
                  title={a.name}
                  aria-label={a.name}
                  aria-pressed={active}
                  className="flex size-6 items-center justify-center rounded-full ring-offset-1 ring-offset-card transition-transform hover:scale-110"
                  style={{ backgroundColor: swatchColor(a.hue), boxShadow: active ? "0 0 0 2px var(--accent)" : undefined }}
                >
                  {active && <Check className="size-3.5 text-white" />}
                </button>
              );
            })}
          </div>
        </div>
      )}
    </div>
  );
}
