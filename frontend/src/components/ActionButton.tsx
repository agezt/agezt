import { useState } from "react";
import { postAction } from "@/lib/api";
import { Button, type ButtonProps } from "@/components/ui/button";

// ActionButton POSTs an allowlisted mutating command (query-arg writes: forget /
// promote / decide …) then triggers a reload so the panel reflects the change.
export function ActionButton({
  label,
  path,
  params,
  onDone,
  variant,
}: {
  label: string;
  path: string;
  params?: Record<string, string>;
  onDone: () => void;
  variant?: ButtonProps["variant"];
}) {
  const [busy, setBusy] = useState(false);
  const [err, setErr] = useState<string | null>(null);
  return (
    <Button
      variant={variant}
      size="sm"
      disabled={busy}
      title={err || undefined}
      onClick={async () => {
        setBusy(true);
        setErr(null);
        try {
          await postAction(path, params);
          onDone();
        } catch (e) {
          setErr((e as Error).message);
          setBusy(false);
        }
      }}
    >
      {err ? "err" : label}
    </Button>
  );
}
