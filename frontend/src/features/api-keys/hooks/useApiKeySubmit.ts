// useApiKeySubmit.ts — small helper for the recurring "POST to /provider/keys/add
// with { env, label, value, active? }, toast, refresh, setBusy" pattern.
// The Connections page and the Models KeyManager share it; setup-wizard code
// uses inline posts because it also chains provider/model/routing reloads.

import { useCallback, useState } from "react";
import { postJSON, postAction } from "@/app/api";
import { useUI } from "@/components/ui/feedback";

export interface AddKeyInput {
  provider?: string;
  env: string;
  label: string;
  value: string;
  active?: boolean;
}

export interface UseApiKeySubmitOptions {
  /** Optional provider id to send alongside (some keys/add paths want it). */
  provider?: string;
  /** Toast a success message; defaults to a label-based one. */
  successMessage?: (input: AddKeyInput) => string;
  /** Triggered after a successful add (typically reload catalog). */
  onSuccess?: () => void | Promise<void>;
}

export function useApiKeySubmit(opts: UseApiKeySubmitOptions = {}) {
  const { toast } = useUI();
  const [busy, setBusy] = useState(false);

  const add = useCallback(
    async (input: AddKeyInput) => {
      setBusy(true);
      try {
        await postJSON("/api/provider/keys/add", {
          provider: opts.provider ?? input.provider,
          env: input.env,
          label: input.label,
          value: input.value,
          active: input.active ?? false,
        });
        const msg = opts.successMessage?.(input) ?? `Added key “${input.label}”${input.active ? " (now active)" : ""}`;
        toast(msg, "success");
        await opts.onSuccess?.();
        return true;
      } catch (e) {
        toast((e as Error).message, "error");
        return false;
      } finally {
        setBusy(false);
      }
    },
    [opts, toast],
  );

  const activate = useCallback(
    async (env: string, label: string) => {
      setBusy(true);
      try {
        await postAction("/api/provider/keys/activate", { env, label });
        toast(`“${label}” is now the active key`, "success");
        await opts.onSuccess?.();
        return true;
      } catch (e) {
        toast((e as Error).message, "error");
        return false;
      } finally {
        setBusy(false);
      }
    },
    [opts, toast],
  );

  const remove = useCallback(
    async (env: string, label: string) => {
      setBusy(true);
      try {
        await postAction("/api/provider/keys/remove", { env, label });
        toast(`Removed key “${label}”`, "success");
        await opts.onSuccess?.();
        return true;
      } catch (e) {
        toast((e as Error).message, "error");
        return false;
      } finally {
        setBusy(false);
      }
    },
    [opts, toast],
  );

  return { add, activate, remove, busy };
}
