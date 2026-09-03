import { describe, it, expect, vi, beforeEach, afterEach } from "vitest";
import { renderHook, act } from "@testing-library/react";

import { useSaveState } from "../useSaveState";

describe("useSaveState", () => {
  beforeEach(() => {
    vi.useFakeTimers();
  });
  afterEach(() => {
    vi.runOnlyPendingTimers();
    vi.useRealTimers();
  });

  it("starts idle and shows the server value", () => {
    const { result } = renderHook(() => useSaveState({ serverValue: "a" }));
    expect(result.current.state).toBe("idle");
    expect(result.current.displayValue).toBe("a");
  });

  it("goes idle → saving → confirmed → idle and shows the in-flight value while saving", async () => {
    let resolve!: () => void;
    const run = vi.fn(
      () => new Promise<void>((r) => (resolve = r)),
    );
    const { result } = renderHook(() => useSaveState({ serverValue: "a" }));

    void act(() => {
      void result.current.save("b", run);
    });
    expect(result.current.state).toBe("saving");
    expect(result.current.displayValue).toBe("b");

    await act(async () => {
      resolve();
      await Promise.resolve();
    });
    expect(result.current.state).toBe("confirmed");
    // Once the write settles the field falls back to the server value, which
    // its mutation hook has spliced in by now.
    expect(result.current.displayValue).toBe("a");

    void act(() => vi.advanceTimersByTime(1600));
    expect(result.current.state).toBe("idle");
  });

  it("surfaces a rejected write and clears on dismiss", async () => {
    const run = vi.fn(() => Promise.reject(new Error("slug already exists")));
    const { result } = renderHook(() => useSaveState({ serverValue: "a" }));

    await act(async () => {
      await result.current.save("b", run);
    });
    expect(result.current.state).toBe("error");
    expect(result.current.error).toBe("slug already exists");
    // No queued/poll state, and the field is back on the server value.
    expect(result.current.displayValue).toBe("a");

    void act(() => result.current.dismissError());
    expect(result.current.state).toBe("idle");
    expect(result.current.error).toBeNull();
  });

  it("never re-reads: save runs the write exactly once", async () => {
    const run = vi.fn(() => Promise.resolve());
    const { result } = renderHook(() => useSaveState({ serverValue: 1 }));

    await act(async () => {
      await result.current.save(2, run);
    });
    expect(result.current.state).toBe("confirmed");
    expect(run).toHaveBeenCalledTimes(1);
  });
});
