import { useCallback, useEffect, useRef, useState } from "react";
import { useQueryClient } from "@tanstack/react-query";

/*
 * The save lifecycle for one edit-in-place field.
 *
 * The API accepts a write with 202 and persists it afterwards, so "the request
 * succeeded" and "the value is stored" are two different moments. Collapsing
 * them would make the UI lie: the field would show the new value, a background
 * refetch would arrive with the old one, and the edit would appear to undo
 * itself.
 *
 * So a field moves idle -> saving -> pending -> confirmed -> idle. While
 * pending it displays what was written, marks itself as not yet stored, and
 * polls until the server agrees. If the server never agrees the field says so
 * rather than quietly reverting, because a write that vanished is exactly the
 * thing a user needs to be told about.
 */

export type SaveState =
  "idle" | "saving" | "pending" | "confirmed" | "unconfirmed" | "error";

// How often to re-read while waiting, and for how long before giving up on the
// write ever landing. The listener normally persists within a second or two;
// twenty seconds is long enough that expiry means something really went wrong.
const pollIntervalMs = 1500;
const pollTimeoutMs = 20000;
const confirmedFlashMs = 1600;

type Options<T> = {
  // The value as the server currently reports it.
  serverValue: T;
  // The query to re-read while waiting for the write to land.
  queryKey: readonly unknown[];
  // Defaults to Object.is, which is wrong for the array and object shapes, so
  // those pass their own.
  isEqual?: (a: T, b: T) => boolean;
};

type SaveStateResult<T> = {
  state: SaveState;
  error: string | null;
  // What to render: the written value while it is in flight, the server's
  // otherwise. Never let a pending field show a stale server value.
  displayValue: T;
  save: (next: T, run: () => Promise<unknown>) => Promise<void>;
  dismissError: () => void;
};

export function useSaveState<T>({
  serverValue,
  queryKey,
  isEqual = Object.is,
}: Options<T>): SaveStateResult<T> {
  const queryClient = useQueryClient();
  const [state, setState] = useState<SaveState>("idle");
  const [error, setError] = useState<string | null>(null);
  const [expected, setExpected] = useState<{ value: T } | null>(null);

  // Held in a ref as well so the polling effect can compare without listing
  // isEqual among its dependencies and restarting the timer on every render.
  const isEqualRef = useRef(isEqual);
  useEffect(() => {
    isEqualRef.current = isEqual;
  }, [isEqual]);

  const save = useCallback(async (next: T, run: () => Promise<unknown>) => {
    setState("saving");
    setError(null);
    setExpected({ value: next });
    try {
      await run();
      setState("pending");
    } catch (cause) {
      setState("error");
      setExpected(null);
      setError(cause instanceof Error ? cause.message : String(cause));
    }
  }, []);

  // Poll while pending, and stop the moment the server reports what was
  // written.
  useEffect(() => {
    if (state !== "pending" || !expected) {
      return;
    }

    if (isEqualRef.current(serverValue, expected.value)) {
      setState("confirmed");
      setExpected(null);
      return;
    }

    const startedAt = Date.now();
    const timer = window.setInterval(() => {
      if (Date.now() - startedAt > pollTimeoutMs) {
        window.clearInterval(timer);
        setState("unconfirmed");
        return;
      }
      void queryClient.invalidateQueries({ queryKey });
    }, pollIntervalMs);

    return () => window.clearInterval(timer);
    // queryKey is a stable tuple from queryKeys, so stringifying it keeps the
    // effect from restarting on every render without deep-comparing.
    // eslint-disable-next-line @eslint-react/exhaustive-deps
  }, [state, expected, serverValue, queryClient, JSON.stringify(queryKey)]);

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
    displayValue: expected ? expected.value : serverValue,
    save,
    dismissError,
  };
}

// Convenience comparisons for the shapes Object.is cannot judge.
export function sameStringList(a: string[], b: string[]): boolean {
  return a.length === b.length && a.every((item, index) => item === b[index]);
}

export function sameJSON<T>(a: T, b: T): boolean {
  return JSON.stringify(a) === JSON.stringify(b);
}
