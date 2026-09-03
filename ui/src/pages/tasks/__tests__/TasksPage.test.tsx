import { describe, it, expect, vi, beforeEach, afterEach } from "vitest";
import { fireEvent, render, screen, waitFor } from "@testing-library/react";
import { MemoryRouter } from "react-router-dom";

import { TasksPage } from "../TasksPage";

const useScanDirectories = vi.fn();
const useRunDirectoryScan = vi.fn();
const mutateAsync = vi.fn();

vi.mock("../../../api/queries", () => ({
  useScanDirectories: () => useScanDirectories(),
  useRunDirectoryScan: () => useRunDirectoryScan(),
}));

function scanDirectoriesResult(
  data: Array<{ scannerSlug: string; directory: string }>,
  overrides: Record<string, unknown> = {},
) {
  return { data, isLoading: false, error: null, ...overrides };
}

// The repo does not wire jest-dom, so read the native property directly.
const isDisabled = (el: HTMLElement) =>
  (el as HTMLButtonElement | HTMLInputElement).disabled;

// antd's loading button prefixes the accessible name with "loading"; match on
// the stable visible label instead.
const scanButton = () =>
  screen.getByRole("button", { name: /Kick off directory scan/ });

// Open the antd Select and pick the option whose label reads exactly `label`.
// rc-select wires selection to a click on `.ant-select-item-option`, not the
// inner `role="option"` node, so target that element directly.
function selectDirectory(label: string) {
  fireEvent.mouseDown(screen.getByRole("combobox"));
  const option = Array.from(
    document.querySelectorAll(".ant-select-item-option"),
  ).find(
    (el) =>
      el.getAttribute("aria-label") === label ||
      el.getAttribute("title") === label,
  );
  if (!option) throw new Error(`no option with label "${label}"`);
  fireEvent.click(option);
}

function renderPage() {
  return render(
    <MemoryRouter>
      <TasksPage />
    </MemoryRouter>,
  );
}

describe("TasksPage", () => {
  beforeEach(() => {
    useScanDirectories.mockReset();
    useRunDirectoryScan.mockReset();
    mutateAsync.mockReset().mockResolvedValue({ scanId: "scan-123" });
    useRunDirectoryScan.mockReturnValue({ mutateAsync, isPending: false });
    useScanDirectories.mockReturnValue(
      scanDirectoriesResult([
        { scannerSlug: "movies", directory: "/media/movies" },
        { scannerSlug: "tv", directory: "/media/tv" },
      ]),
    );
    // antd's Select measures its dropdown with a ResizeObserver, which jsdom
    // does not provide (same gap WorkflowListPage stubs IntersectionObserver for).
    vi.stubGlobal(
      "ResizeObserver",
      class {
        observe() {}
        unobserve() {}
        disconnect() {}
      },
    );
  });

  afterEach(() => {
    vi.unstubAllGlobals();
    vi.restoreAllMocks();
  });

  it("populates the select from the scan-directories query with slug — directory labels", () => {
    renderPage();

    fireEvent.mouseDown(screen.getByRole("combobox"));

    expect(
      screen.getByRole("option", { name: "movies — /media/movies" }),
    ).toBeDefined();
    expect(
      screen.getByRole("option", { name: "tv — /media/tv" }),
    ).toBeDefined();
  });

  it("enables the button only once a directory is selected", () => {
    renderPage();

    expect(isDisabled(scanButton())).toBe(true);

    selectDirectory("movies — /media/movies");

    expect(isDisabled(scanButton())).toBe(false);
  });

  it("disables the controls while a request is in flight", () => {
    useRunDirectoryScan.mockReturnValue({ mutateAsync, isPending: true });
    renderPage();

    expect(isDisabled(scanButton())).toBe(true);
    expect(isDisabled(screen.getByRole("combobox"))).toBe(true);
  });

  it("calls the mutation with the selected slug and shows the scan id on success", async () => {
    renderPage();

    selectDirectory("tv — /media/tv");
    fireEvent.click(scanButton());

    expect(mutateAsync).toHaveBeenCalledWith("tv");
    await waitFor(() =>
      expect(screen.getByText(/scan id scan-123/)).toBeDefined(),
    );
  });

  it("shows the server's error message when the scan fails", async () => {
    mutateAsync.mockRejectedValueOnce(new Error("no agent is mapped"));
    renderPage();

    selectDirectory("movies — /media/movies");
    fireEvent.click(scanButton());

    await waitFor(() =>
      expect(screen.getByText("no agent is mapped")).toBeDefined(),
    );
  });

  it("disables both controls and links to the config screen when no directories are configured", () => {
    useScanDirectories.mockReturnValue(scanDirectoriesResult([]));
    renderPage();

    expect(isDisabled(screen.getByRole("combobox"))).toBe(true);
    expect(isDisabled(scanButton())).toBe(true);
    const link = screen.getByRole("link", { name: "Directory Scanner" });
    expect(link.getAttribute("href")).toBe("/system/directory-scanner");
  });
});
