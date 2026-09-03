import { useCallback, useEffect, useState } from "react";

/*
 * The save lifecycle for one edit-in-place field.
 *
 * Config writes are synchronous now (docs/adr/0002): the RPC persists under
 * the store lock and returns the stored resource, and the mutation hook
 * splices that resource straight into the query cache. So "the request
 * succeeded" and "the value is stored" are the same moment — a field goes
 * idle -> saving -> confirmed -> idle, with no queued phase and nothing to
 * poll for. A failed write shows the error rather than reverting, because a
 * write that vanished is exactly the thing a user needs to be told about.
 */

export type SaveState = "idle" | "saving" | "confirmed" | "error";

const confirmedFlashMs = 1600;

type Options<T> = {
  // The value as the server currently reports it (from the query cache the
  // mutation hook keeps current from each write's response).
  serverValue: T;
};

type SaveStateResult<T> = {
  state: SaveState;
  error: string | null;
  // What to render: the written value while the write is in flight, the
  // server's value otherwise.
  displayValue: T;
  save: (next: T, run: () => Promise<unknown>) => Promise<void>;
  dismissError: () => void;
};

export function useSaveState<T>({
  serverValue,
}: Options<T>): SaveStateResult<T> {
  const [state, setState] = useState<SaveState>("idle");
  const [error, setError] = useState<string | null>(null);
  const [inFlight, setInFlight] = useState<{ value: T } | null>(null);

  const save = useCallback(async (next: T, run: () => Promise<unknown>) => {
    setState("saving");
    setError(null);
    setInFlight({ value: next });
    try {
      await run();
      setState("confirmed");
    } catch (cause) {
      setState("error");
      setError(cause instanceof Error ? cause.message : String(cause));
    } finally {
      setInFlight(null);
    }
  }, []);

  // Let the confirmation tick show briefly, then go quiet.
  useEffect(() => {
    if (state !== "confirmed") {
      return;
    }
    const timer = window.setTimeout(() => setState("idle"), confirmedFlashMs);
    return () => window.clearTimeout(timer);
  }, [state]);

  const dismissError = useCallback(() => {
    setState("idle");
    setError(null);
  }, []);

  return {
    state,
    error,
    displayValue: inFlight ? inFlight.value : serverValue,
    save,
    dismissError,
  };
}
