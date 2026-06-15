import { useEffect, useRef, useState } from "react";
import { useConsoleName } from "@/lib/brand";

// ConsoleName renders the header title ("<name> · console") and lets you rename it
// inline (M719): click the name to edit, Enter/blur to save, Esc to cancel. The
// accent-coloured word is the brand; "· console" stays fixed.
export function ConsoleName() {
  const { name, setName } = useConsoleName();
  const [editing, setEditing] = useState(false);
  const [draft, setDraft] = useState(name);
  const ref = useRef<HTMLInputElement>(null);

  useEffect(() => {
    if (editing) {
      const el = ref.current;
      el?.focus();
      el?.select();
    }
  }, [editing]);

  function begin() {
    setDraft(name);
    setEditing(true);
  }
  function commit() {
    setEditing(false);
    if (draft.trim() && draft.trim() !== name) setName(draft);
  }
  function cancel() {
    setEditing(false);
    setDraft(name);
  }

  return (
    <h1 className="text-base font-bold tracking-tight">
      {editing ? (
        <input
          ref={ref}
          value={draft}
          onChange={(e) => setDraft(e.target.value)}
          onBlur={commit}
          onKeyDown={(e) => {
            if (e.key === "Enter") commit();
            else if (e.key === "Escape") cancel();
          }}
          aria-label="Console name"
          className="w-28 rounded border border-accent/40 bg-panel px-1 text-sm font-semibold text-accent outline-none"
        />
      ) : (
        <button
          onClick={begin}
          title="Rename your console"
          aria-label="Rename console"
          className="text-gradient font-bold transition-opacity hover:opacity-80"
        >
          {name}
        </button>
      )}{" "}
      · console
    </h1>
  );
}
