import { useState } from "react";
import { postAction } from "@/lib/api";
import { Button, type ButtonProps } from "@/components/ui/button";
import { useUI, type ConfirmOptions } from "@/components/ui/feedback";

// ActionButton POSTs an allowlisted mutating command (query-arg writes: forget /
// promote / decide …) then triggers a reload so the panel reflects the change.
// Destructive variants can gate on a confirm modal, and the outcome is reported
// via toast (success / error) rather than an inline label swap.
export function ActionButton({
  label,
  path,
  params,
  onDone,
  variant,
  confirm,
  success,
}: {
  label: string;
  path: string;
  params?: Record<string, string>;
  onDone: () => void;
  variant?: ButtonProps["variant"];
  confirm?: ConfirmOptions;
  success?: string;
}) {
  const ui = useUI();
  const [busy, setBusy] = useState(false);
  return (
    <Button
      variant={variant}
      size="sm"
      disabled={busy}
      onClick={async () => {
        if (confirm && !(await ui.confirm(confirm))) return;
        setBusy(true);
        try {
          await postAction(path, params);
          if (success) ui.toast(success, "success");
          onDone();
        } catch (e) {
          ui.toast((e as Error).message, "error");
          setBusy(false);
        }
      }}
    >
      {label}
    </Button>
  );
}
