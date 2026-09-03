import { describe, it, expect, vi, beforeEach, afterEach } from "vitest";
import { fireEvent, render, screen } from "@testing-library/react";
import { MemoryRouter } from "react-router-dom";

import { WorkflowListPage } from "../WorkflowListPage";

const useWorkflowList = vi.fn();
const useDeleteWorkflow = vi.fn();
const deleteMutateAsync = vi.fn();

vi.mock("../../../api/queries", () => ({
  useWorkflowList: () => useWorkflowList(),
  useDeleteWorkflow: () => useDeleteWorkflow(),
}));

function workflow(overrides: Record<string, unknown> = {}) {
  return {
    id: "wf-1",
    version: 1,
    name: "Nightly sync",
    description: "runs every night",
    tags: ["scheduled"],
    createdAt: undefined,
    ...overrides,
  };
}

function listResult(workflows: Array<Record<string, unknown>>) {
  return {
    data: { pages: [{ workflows, nextPageToken: "" }] },
    fetchNextPage: vi.fn(),
    hasNextPage: false,
    isFetchingNextPage: false,
    isLoading: false,
    isError: false,
    error: null,
  };
}

function renderPage() {
  return render(
    <MemoryRouter>
      <WorkflowListPage />
    </MemoryRouter>,
  );
}

describe("WorkflowListPage", () => {
  beforeEach(() => {
    useWorkflowList.mockReset();
    useDeleteWorkflow.mockReset();
    deleteMutateAsync.mockReset().mockResolvedValue({});
    useDeleteWorkflow.mockReturnValue({ mutateAsync: deleteMutateAsync });
    // WorkflowListPage wires an IntersectionObserver to its scroll sentinel.
    vi.stubGlobal(
      "IntersectionObserver",
      class {
        observe() {}
        disconnect() {}
        unobserve() {}
      },
    );
  });

  afterEach(() => {
    vi.unstubAllGlobals();
    vi.restoreAllMocks();
  });

  it("keys each row by the workflow id", () => {
    useWorkflowList.mockReturnValue(listResult([workflow()]));

    renderPage();

    expect(screen.getByText("Nightly sync")).toBeDefined();
    expect(screen.getByRole("button", { name: "Delete" })).toBeDefined();
  });

  it("deletes by id after the confirmation is accepted", () => {
    useWorkflowList.mockReturnValue(listResult([workflow({ id: "wf-42" })]));
    const confirmSpy = vi.spyOn(window, "confirm").mockReturnValue(true);

    renderPage();
    fireEvent.click(screen.getByRole("button", { name: "Delete" }));

    expect(confirmSpy).toHaveBeenCalled();
    expect(deleteMutateAsync).toHaveBeenCalledWith("wf-42");
  });

  it("does not delete when the confirmation is dismissed", () => {
    useWorkflowList.mockReturnValue(listResult([workflow()]));
    vi.spyOn(window, "confirm").mockReturnValue(false);

    renderPage();
    fireEvent.click(screen.getByRole("button", { name: "Delete" }));

    expect(deleteMutateAsync).not.toHaveBeenCalled();
  });

  it("renders a delete control per workflow", () => {
    useWorkflowList.mockReturnValue(
      listResult([
        workflow({ id: "wf-1", name: "One" }),
        workflow({ id: "wf-2", name: "Two" }),
      ]),
    );
    vi.spyOn(window, "confirm").mockReturnValue(true);

    renderPage();
    const buttons = screen.getAllByRole("button", { name: "Delete" });
    expect(buttons).toHaveLength(2);

    fireEvent.click(buttons[1]);
    expect(deleteMutateAsync).toHaveBeenCalledWith("wf-2");
  });
});
