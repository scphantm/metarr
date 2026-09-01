import {
  createContext,
  use,
  useCallback,
  useEffect,
  useMemo,
  useRef,
  useState,
  type ReactNode,
} from "react";

/*
 * A generic mechanism by which whichever page is currently mounted can
 * publish context (and an optional tool-result handler) for some other
 * globally-mounted consumer to read. A page calls
 * useRegisterPageContext(pageKey, collect, applyToolResult?) on mount;
 * a consumer reads useActivePageContext() to know what's available right
 * now, calls collect() when it needs the data, and (if the page registered
 * one) calls applyToolResult() to hand a result back to the page.
 *
 * Both are supplier/handler functions, not data: calling them always
 * reflects live state rather than a snapshot taken at registration time —
 * a page can pass fresh closures on every render without that
 * re-triggering registration, because the registered functions are stable
 * per-mount wrappers that read the latest closures through refs, and those
 * refs are only ever written inside an effect (never during render, which
 * React's rules of hooks forbid).
 *
 * Only one page is ever mounted at a time (this is routing, not tabs), so
 * "the active entry" is unambiguous — a page that registers nothing (most
 * pages, today) just leaves consumers with no page context. If the user
 * navigates away before a tool result is applied, applyToolResult simply
 * becomes unavailable again — there is nothing left to apply it to.
 *
 * No current consumer is registered — this is currently unused generic
 * infrastructure, kept for a future page/feature to adopt.
 */

type ActiveEntry = {
  pageKey: string;
  collect: () => unknown;
  applyToolResult?: (toolName: string, args: unknown) => void;
};

type Registry = {
  active: ActiveEntry | null;
  register: (entry: ActiveEntry) => void;
  unregister: (pageKey: string) => void;
};

const PageContextRegistryContext = createContext<Registry | null>(null);

export function PageContextProvider({ children }: { children: ReactNode }) {
  const [active, setActive] = useState<ActiveEntry | null>(null);

  // Stable across renders (useCallback, empty deps), and each is called at
  // most once per real page mount/unmount by useRegisterPageContext's
  // properly-scoped effect below — so this never fires on every render of
  // whichever page is registering.
  const register = useCallback((entry: ActiveEntry) => {
    setActive(entry);
  }, []);

  const unregister = useCallback((pageKey: string) => {
    setActive((current) => (current?.pageKey === pageKey ? null : current));
  }, []);

  const registry = useMemo<Registry>(
    () => ({ active, register, unregister }),
    [active, register, unregister],
  );

  return (
    <PageContextRegistryContext value={registry}>
      {children}
    </PageContextRegistryContext>
  );
}

function useRegistry(): Registry {
  const registry = use(PageContextRegistryContext);
  if (!registry) {
    throw new Error("this hook must be used within a PageContextProvider");
  }
  return registry;
}

/**
 * Called by a page on mount; cleans up on unmount. collect/applyToolResult
 * can be fresh closures every render (the common case, capturing whatever
 * local state the page just rendered with) — refs keep the latest ones
 * available, and the register/unregister effect itself only runs when
 * pageKey changes, so a fresh closure identity never re-triggers
 * registration or a parent re-render. The refs are only ever written
 * inside an effect, never during render, per React's rules of hooks.
 */
export function useRegisterPageContext(
  pageKey: string,
  collect: () => unknown,
  applyToolResult?: (toolName: string, args: unknown) => void,
) {
  const { register, unregister } = useRegistry();
  const collectRef = useRef(collect);
  const applyToolResultRef = useRef(applyToolResult);

  useEffect(() => {
    collectRef.current = collect;
    applyToolResultRef.current = applyToolResult;
  });

  useEffect(() => {
    register({
      pageKey,
      collect: () => collectRef.current(),
      applyToolResult: (toolName, args) =>
        applyToolResultRef.current?.(toolName, args),
    });
    return () => unregister(pageKey);
  }, [pageKey, register, unregister]);
}

/** Called by the chat widget to know what's available right now. */
export function useActivePageContext(): ActiveEntry | null {
  return useRegistry().active;
}
